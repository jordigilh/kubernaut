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

package infrastructure

// ============================================================================
// AF Severity Triage Prometheus Infrastructure
//
// Deploys Prometheus with AF-specific alert rules for the 5-tier severity
// triage pipeline E2E tests. Builds on the shared DeployPrometheus helper.
//
// Provides:
//   - DeployPrometheusForSeverityTriage: orchestrates Prometheus + AF rules
//   - SeedTriageAlertRules: patches Prometheus ConfigMap with AF fixtures
//   - WaitForPrometheusRuleState: polls /api/v1/rules for desired state
//   - AFInjectOTLPMetrics: single-metric OTLP injection (wraps InjectMetrics)
//   - SeverityTriageAlertRulesYAML: PromQL alert rule fixtures
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// DeployPrometheusForSeverityTriage deploys Prometheus in the E2E cluster and
// seeds AF-specific alert rules for the 5-tier severity triage pipeline tests.
//
// This delegates to kubernaut's canonical DeployPrometheus (DD-TEST-001 v2.8)
// and then patches the rules ConfigMap with AF's triage fixtures.
//
// Ref: Prometheus OTLP receiver -- https://prometheus.io/docs/guides/opentelemetry/
func DeployPrometheusForSeverityTriage(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "Deploying Prometheus for severity triage testing...")

	if err := DeployPrometheus(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("deploy Prometheus: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "Seeding AF severity triage alert rules...")

	if err := SeedTriageAlertRules(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("seed triage alert rules: %w", err)
	}

	return nil
}

// SeedTriageAlertRules patches the Prometheus rules ConfigMap with AF-specific
// alert rules for the 5-tier severity triage pipeline. After patching, it
// triggers a Prometheus config reload.
func SeedTriageAlertRules(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	rulesYAML := strings.TrimSpace(SeverityTriageAlertRulesYAML)

	patchJSON := fmt.Sprintf(`{"data":{"af-severity-triage.yml":%q}}`, rulesYAML)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infra
		"patch", "configmap", "prometheus-rules",
		"-n", namespace,
		"--type=merge",
		"-p", patchJSON)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("patch prometheus-rules ConfigMap: %w", err)
	}

	restartCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"rollout", "restart", "deployment/prometheus",
		"-n", namespace)
	restartCmd.Stdout = writer
	restartCmd.Stderr = writer
	if err := restartCmd.Run(); err != nil {
		return fmt.Errorf("restart Prometheus: %w", err)
	}

	waitCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"rollout", "status", "deployment/prometheus",
		"-n", namespace,
		"--timeout=60s")
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("prometheus not ready after restart: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Prometheus ready with AF severity triage alert rules")
	return nil
}

// PrometheusRuleState represents the state of a Prometheus alerting rule.
type PrometheusRuleState string

const (
	// RuleStateFiring means the alerting rule is actively firing.
	RuleStateFiring PrometheusRuleState = "firing"
	// RuleStatePending means the rule is pending (threshold met, not yet firing).
	RuleStatePending PrometheusRuleState = "pending"
	// RuleStateInactive means the rule is not pending or firing.
	RuleStateInactive PrometheusRuleState = "inactive"
)

type prometheusRulesResponse struct {
	Status string `json:"status"`
	Data   struct {
		Groups []struct {
			Name  string `json:"name"`
			Rules []struct {
				Name  string `json:"name"`
				State string `json:"state"`
				Type  string `json:"type"`
			} `json:"rules"`
		} `json:"groups"`
	} `json:"data"`
}

// WaitForPrometheusRuleState polls Prometheus /api/v1/rules until the named
// alert rule reaches the desired state or the timeout expires.
func WaitForPrometheusRuleState(ctx context.Context, prometheusURL, ruleName string, desired PrometheusRuleState, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, prometheusURL+"/api/v1/rules", http.NoBody)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		var rulesResp prometheusRulesResponse
		if err := json.Unmarshal(body, &rulesResp); err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		for _, group := range rulesResp.Data.Groups {
			for _, rule := range group.Rules {
				if rule.Name == ruleName && PrometheusRuleState(rule.State) == desired {
					return nil
				}
			}
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("rule %q did not reach state %q within %v", ruleName, desired, timeout)
}

// AFInjectOTLPMetrics sends a single gauge metric to Prometheus via the OTLP HTTP endpoint.
// This is a convenience wrapper around InjectMetrics for AF E2E tests that inject one
// metric at a time with simple label maps.
func AFInjectOTLPMetrics(ctx context.Context, prometheusURL, metricName string, value float64, labels map[string]string) error {
	_ = ctx // kept for API compatibility; InjectMetrics is context-free
	return InjectMetrics(prometheusURL, []TestMetric{
		{
			Name:   metricName,
			Value:  value,
			Labels: labels,
		},
	})
}

// SeverityTriageAlertRulesYAML is the Prometheus alert rules YAML for seeding
// the E2E severity triage pipeline tests.
//
// Each rule exercises a specific tier:
//   - HighCPU: for:0s -> fires immediately when metric is present (tier 1)
//   - HighMemory: for:1h -> stays pending when metric present (tier 1.5)
//   - DiskPressure: for:0s + metric injected -> inactive then evaluates live data (tier 2)
//   - NetworkLatency: query matches no-data-ns target but no metric exists -> inactive no data (tier 2.5)
//
// PromQL expressions include label selectors for namespace/kind/name because the
// triage pipeline's Tier 1.5 and Tier 2 use ExtractLabelMatchers(query)
// + MatchesResource to correlate rules with the target resource.
const SeverityTriageAlertRulesYAML = `
groups:
  - name: e2e-severity-triage
    interval: 5s
    rules:
      - alert: HighCPU
        expr: e2e_cpu_usage_percent{namespace="default",kind="Deployment",name="test-firing-target"} > 90
        for: 0s
        labels:
          severity: critical
          source: prometheus
        annotations:
          summary: "CPU usage is critically high"
      - alert: HighMemory
        expr: e2e_memory_usage_percent{namespace="default",kind="Deployment",name="test-pending-target"} > 85
        for: 1h
        labels:
          severity: high
          source: prometheus
        annotations:
          summary: "Memory usage is high"
      - alert: DiskPressure
        expr: e2e_disk_usage_percent{namespace="sev-tier2-ns",kind="Deployment",name="test-inactive-target"} > 90
        for: 0s
        labels:
          severity: medium
          source: prometheus
        annotations:
          summary: "Disk usage is elevated"
      - alert: NetworkLatency
        expr: e2e_network_latency_ms{namespace="no-data-ns",kind="Deployment",name="test-nodata-target"} > 100
        for: 0s
        labels:
          severity: high
          source: prometheus
        annotations:
          summary: "Network latency is high"
`
