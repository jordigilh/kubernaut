/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package classifier provides Rego-based environment and business classification.
//
// Per IMPLEMENTATION_PLAN_V1.23.md Day 5 specification:
// PriorityEngine determines priority using Rego policies with K8s + business context.
// BR-SP-070: Rego policies with rich context
// BR-SP-071: Severity-based fallback on timeout/error
// BR-SP-072: Hot-reload from mounted ConfigMap via fsnotify
package classifier

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/opa/v1/rego"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
)

const (
	// Per BR-SP-070: P95 evaluation latency < 100ms
	// Per BR-SP-071: Fallback triggers on timeout (>100ms)
	regoEvalTimeout = 100 * time.Millisecond
)

// validPriorities defines the valid priority levels per BR-SP-071
var validPriorities = map[string]bool{
	"P0": true,
	"P1": true,
	"P2": true,
	"P3": true,
}

// PriorityEngine determines priority using Rego policy.
// Per IMPLEMENTATION_PLAN_V1.23.md Day 5 specification.
//
// BR-SP-070: Rego policies with rich context
// BR-SP-071: Severity-based fallback on timeout/error
// BR-SP-072: Hot-reload from mounted ConfigMap via fsnotify
type PriorityEngine struct {
	regoQuery   *rego.PreparedEvalQuery
	fileWatcher *hotreload.FileWatcher // Per DD-INFRA-001: fsnotify-based
	policyPath  string                 // Path to mounted ConfigMap file
	logger      logr.Logger            // DD-005 v2.0: logr.Logger (not *zap.Logger)
	mu          sync.RWMutex           // Protects regoQuery during hot-reload
}

// NewPriorityEngine creates a new Rego-based priority engine.
// Per BR-SP-070, BR-SP-071, BR-SP-072 specifications.
//
// DD-005 v2.0: Uses logr.Logger
func NewPriorityEngine(ctx context.Context, policyPath string, logger logr.Logger) (*PriorityEngine, error) {
	log := logger.WithName("priority-engine")

	// Read and compile initial policy
	policyContent, err := os.ReadFile(policyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy file %s: %w", policyPath, err)
	}

	query, err := rego.New(
		rego.Query("data.signalprocessing.priority.result"),
		rego.Module(filepath.Base(policyPath), string(policyContent)),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to compile Rego policy: %w", err)
	}

	return &PriorityEngine{
		regoQuery:  &query,
		policyPath: policyPath,
		logger:     log,
	}, nil
}

// Assign determines priority using Rego policy with K8s + business context.
//
// BR-SP-070: Rego policies with rich context
// BR-SP-071: Fallback on timeout (>100ms) or error
// Input schema per BR-SP-070 (no replicas/minReplicas/conditions)
func (p *PriorityEngine) Assign(ctx context.Context, k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification, signal *signalprocessingv1alpha1.SignalData) (*signalprocessingv1alpha1.PriorityAssignment, error) {
	// Validate inputs (PE-ER-03)
	if envClass == nil {
		return nil, fmt.Errorf("environment classification is required")
	}
	if signal == nil {
		return nil, fmt.Errorf("signal data is required")
	}

	// Build input per BR-SP-070 schema with nil checks
	input := p.buildRegoInput(k8sCtx, envClass, signal)

	// Add timeout per BR-SP-071 (>100ms triggers fallback)
	timeoutCtx, cancel := context.WithTimeout(ctx, regoEvalTimeout)
	defer cancel()

	p.mu.RLock()
	query := p.regoQuery
	p.mu.RUnlock()

	results, err := query.Eval(timeoutCtx, rego.EvalInput(input))
	if err != nil {
		// BR-SP-071: DEPRECATED - Go fallback removed (2025-12-20)
		// Rego policy evaluation failures are now ERRORS, not fallbacks.
		// Operators must ensure their policies are valid and have `default` rules.
		p.logger.Error(err, "Rego evaluation failed - policy is mandatory, no Go fallback",
			"hint", "Ensure your priority.rego policy has a `default` rule for catch-all cases")
		return nil, fmt.Errorf("priority policy evaluation failed: %w", err)
	}

	// Check for empty results - this is a policy bug (missing default rule)
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		p.logger.Error(nil, "Rego returned no results - policy missing `default` rule",
			"hint", "Add a `default result := {...}` rule to your priority.rego policy")
		return nil, fmt.Errorf("priority policy returned no results - add a `default` rule")
	}

	// Extract and validate Rego output
	return p.extractAndValidateResult(results, signal.Severity)
}

// buildRegoInput constructs the input map with nil checks.
// Per BR-SP-070 schema: signal, environment, namespace_labels, workload_labels
func (p *PriorityEngine) buildRegoInput(k8sCtx *signalprocessingv1alpha1.KubernetesContext, envClass *signalprocessingv1alpha1.EnvironmentClassification, signal *signalprocessingv1alpha1.SignalData) map[string]interface{} {
	input := map[string]interface{}{
		"signal": map[string]interface{}{
			"severity": signal.Severity,
			"source":   signal.Source,
		},
		"environment": envClass.Environment,
	}

	// Nil checks for nested K8s context
	if k8sCtx != nil {
		if k8sCtx.Namespace != nil {
			input["namespace_labels"] = ensureLabelsMap(k8sCtx.Namespace.Labels)
		} else {
			input["namespace_labels"] = map[string]interface{}{}
		}
		if k8sCtx.Workload != nil {
			input["workload_labels"] = ensureLabelsMap(k8sCtx.Workload.Labels)
		} else {
			input["workload_labels"] = map[string]interface{}{}
		}
	} else {
		input["namespace_labels"] = map[string]interface{}{}
		input["workload_labels"] = map[string]interface{}{}
	}

	return input
}

// extractAndValidateResult extracts and validates Rego output.
// PE-ER-04, PE-ER-05: Validate priority is P0-P3
func (p *PriorityEngine) extractAndValidateResult(results rego.ResultSet, severity string) (*signalprocessingv1alpha1.PriorityAssignment, error) {
	resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		// BR-SP-071: DEPRECATED - Go fallback removed (2025-12-20)
		// Invalid output structure is a policy bug, not a runtime fallback scenario
		p.logger.Error(nil, "Invalid Rego output type - policy returns wrong structure",
			"hint", "Ensure priority.rego returns a map with 'priority' and 'policy_name' fields")
		return nil, fmt.Errorf("priority policy returned invalid output structure (expected map)")
	}

	priority, _ := resultMap["priority"].(string)
	policyName, _ := resultMap["policy_name"].(string)
	// Note: Confidence field removed per DD-SP-001 V1.1

	// Validate priority is P0-P3 (PE-ER-04, PE-ER-05)
	if !validPriorities[priority] {
		return nil, fmt.Errorf("invalid priority value: %s (must be P0, P1, P2, or P3)", priority)
	}

	return &signalprocessingv1alpha1.PriorityAssignment{
		Priority:   priority,
		Source:     "rego-policy",
		PolicyName: policyName,
		AssignedAt: metav1.Now(), // Set timestamp in Go, not Rego
	}, nil
}

// StartHotReload starts the hot-reload mechanism for Rego policies.
// BR-SP-072: Hot-reload from mounted ConfigMap via fsnotify
// Per DD-INFRA-001: Uses shared FileWatcher component
func (p *PriorityEngine) StartHotReload(ctx context.Context) error {
	var err error
	p.fileWatcher, err = hotreload.NewFileWatcher(
		p.policyPath, // e.g., "/etc/kubernaut/policies/priority.rego"
		func(content string) error {
			// Compile new Rego policy
			newQuery, err := rego.New(
				rego.Query("data.signalprocessing.priority.result"),
				rego.Module("priority.rego", content),
			).PrepareForEval(ctx)
			if err != nil {
				return fmt.Errorf("rego compilation failed: %w", err)
			}

			// Atomically swap policy
			p.mu.Lock()
			p.regoQuery = &newQuery
			p.mu.Unlock()

			p.logger.Info("Rego policy hot-reloaded successfully",
				"hash", p.fileWatcher.GetLastHash())
			return nil
		},
		p.logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	return p.fileWatcher.Start(ctx)
}

// Stop gracefully stops the hot-reloader.
func (p *PriorityEngine) Stop() {
	if p.fileWatcher != nil {
		p.fileWatcher.Stop()
	}
}

// GetPolicyHash returns the current policy hash (for monitoring/debugging).
func (p *PriorityEngine) GetPolicyHash() string {
	if p.fileWatcher != nil {
		return p.fileWatcher.GetLastHash()
	}
	return ""
}
