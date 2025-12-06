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
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/opa/v1/rego"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
)

const (
	// Per BR-SP-070: P95 evaluation latency < 100ms
	// Per BR-SP-071: Fallback triggers on timeout (>100ms)
	regoEvalTimeout = 100 * time.Millisecond

	// Fallback confidence per BR-SP-071
	fallbackConfidence = 0.6
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
		// BR-SP-071: Fallback based on severity ONLY (not environment)
		p.logger.Info("Rego evaluation failed, using fallback", "error", err)
		return p.fallbackBySeverity(signal.Severity), nil
	}

	// Check for empty results
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		p.logger.Info("Rego returned no results, using fallback")
		return p.fallbackBySeverity(signal.Severity), nil
	}

	// Extract and validate Rego output
	return p.extractAndValidateResult(results, signal.Severity)
}

// buildRegoInput constructs the input map with nil checks.
// Per BR-SP-070 schema: signal, environment, namespace_labels, deployment_labels
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
		if k8sCtx.Deployment != nil {
			input["deployment_labels"] = ensureLabelsMap(k8sCtx.Deployment.Labels)
		} else {
			input["deployment_labels"] = map[string]interface{}{}
		}
	} else {
		input["namespace_labels"] = map[string]interface{}{}
		input["deployment_labels"] = map[string]interface{}{}
	}

	return input
}

// extractAndValidateResult extracts and validates Rego output.
// PE-ER-04, PE-ER-05: Validate priority is P0-P3
func (p *PriorityEngine) extractAndValidateResult(results rego.ResultSet, severity string) (*signalprocessingv1alpha1.PriorityAssignment, error) {
	resultMap, ok := results[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		p.logger.Info("Invalid Rego output type, using fallback")
		return p.fallbackBySeverity(severity), nil
	}

	priority, _ := resultMap["priority"].(string)
	policyName, _ := resultMap["policy_name"].(string)
	confidence := extractConfidence(resultMap["confidence"])

	// Validate priority is P0-P3 (PE-ER-04, PE-ER-05)
	if !validPriorities[priority] {
		return nil, fmt.Errorf("invalid priority value: %s (must be P0, P1, P2, or P3)", priority)
	}

	return &signalprocessingv1alpha1.PriorityAssignment{
		Priority:   priority,
		Confidence: confidence,
		Source:     "rego-policy",
		PolicyName: policyName,
	}, nil
}

// fallbackBySeverity returns priority based on severity only (BR-SP-071).
// Used when Rego fails - environment is NOT considered in fallback.
//
// Fallback Matrix (per BR-SP-071):
// - critical → P1 (conservative - high but not highest without context)
// - warning → P2 (standard priority for warnings)
// - info → P3 (lowest priority for informational)
// - unknown → P2 (default when severity unknown)
func (p *PriorityEngine) fallbackBySeverity(severity string) *signalprocessingv1alpha1.PriorityAssignment {
	var priority string
	switch strings.ToLower(severity) {
	case "critical":
		priority = "P1" // Conservative - high but not highest without context
	case "warning":
		priority = "P2"
	case "info":
		priority = "P3"
	default:
		priority = "P2" // Default when severity unknown
	}

	p.logger.Info("Using severity-based fallback", "severity", severity, "priority", priority)

	return &signalprocessingv1alpha1.PriorityAssignment{
		Priority:   priority,
		Confidence: fallbackConfidence,
		Source:     "fallback-severity",
	}
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

