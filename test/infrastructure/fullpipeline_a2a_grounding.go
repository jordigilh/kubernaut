/*
Copyright 2026 Jordi Gil.

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

package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const fullPipelineA2AGroundingRulePrefix = "FullPipelineA2ASeverityGrounding_"

// SeedFullPipelineA2AGroundingRules installs namespace-scoped Prometheus alerts
// for the FullPipeline A2A RR-creation scenarios. Each scenario uses a unique
// workload namespace; omitting the shared Deployment/name labels prevents one
// scenario's alert from grounding another scenario's target. The route-skip
// label prevents the fixture from creating a second RR through Gateway.
func SeedFullPipelineA2AGroundingRules(ctx context.Context, namespace, kubeconfigPath string, targetNamespaces map[string]string, writer io.Writer) error {
	rulesYAML, ruleNames := fullPipelineA2AGroundingRuleFile(targetNamespaces)
	if len(ruleNames) == 0 {
		return nil
	}

	patch, err := json.Marshal(map[string]map[string]string{
		"data": {"fullpipeline-a2a-grounding.yml": rulesYAML},
	})
	if err != nil {
		return fmt.Errorf("marshal fullpipeline A2A grounding rules patch: %w", err)
	}

	patchCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infra
		"patch", "configmap", "prometheus-rules", "-n", namespace,
		"--type=merge", "-p", string(patch))
	patchCmd.Stdout = writer
	patchCmd.Stderr = writer
	if err := patchCmd.Run(); err != nil {
		return fmt.Errorf("patch fullpipeline A2A grounding rules: %w", err)
	}

	restartCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infra
		"rollout", "restart", "deployment/prometheus", "-n", namespace)
	restartCmd.Stdout = writer
	restartCmd.Stderr = writer
	if err := restartCmd.Run(); err != nil {
		return fmt.Errorf("restart Prometheus with fullpipeline A2A grounding rules: %w", err)
	}

	statusCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infra
		"rollout", "status", "deployment/prometheus", "-n", namespace, "--timeout=120s")
	statusCmd.Stdout = writer
	statusCmd.Stderr = writer
	if err := statusCmd.Run(); err != nil {
		return fmt.Errorf("wait for Prometheus grounding-rule rollout: %w", err)
	}

	prometheusURL := fmt.Sprintf("http://127.0.0.1:%d", PrometheusHostPort)
	for _, ruleName := range ruleNames {
		if err := WaitForPrometheusRuleState(ctx, prometheusURL, ruleName, RuleStateFiring, 60*time.Second); err != nil {
			return fmt.Errorf("wait for fullpipeline A2A grounding rule %q to fire: %w", ruleName, err)
		}
	}

	_, _ = fmt.Fprintf(writer, "  ✅ %d fullpipeline A2A severity-grounding alerts firing\n", len(ruleNames))
	return nil
}

func fullPipelineA2AGroundingRuleFile(targetNamespaces map[string]string) (string, []string) {
	scenarioKeys := make([]string, 0, len(targetNamespaces))
	for scenarioKey := range targetNamespaces {
		scenarioKeys = append(scenarioKeys, scenarioKey)
	}
	sort.Strings(scenarioKeys)

	ruleBlocks := make([]string, 0, len(scenarioKeys))
	ruleNames := make([]string, 0, len(scenarioKeys))
	for _, scenarioKey := range scenarioKeys {
		targetNamespace := targetNamespaces[scenarioKey]
		if targetNamespace == "" {
			continue
		}

		ruleName := fullPipelineA2AGroundingRulePrefix + strings.ReplaceAll(targetNamespace, "-", "_")
		ruleNames = append(ruleNames, ruleName)
		ruleBlocks = append(ruleBlocks, fmt.Sprintf(`      - alert: %s
        expr: vector(1) > 0
        for: 0s
        labels:
          severity: warning
          source: prometheus
          namespace: %s
          %s: %q
        annotations:
          summary: FullPipeline A2A severity grounding for %s`,
			ruleName,
			strconv.Quote(targetNamespace),
			SkipGatewayRouteLabelKey,
			SkipGatewayRouteLabelValue,
			scenarioKey,
		))
	}
	if len(ruleBlocks) == 0 {
		return "", nil
	}

	rulesYAML := "groups:\n  - name: fullpipeline-a2a-severity-grounding\n    interval: 10s\n    rules:\n" + strings.Join(ruleBlocks, "\n") + "\n"
	return rulesYAML, ruleNames
}
