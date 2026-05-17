package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	kinfra "github.com/jordigilh/kubernaut/test/infrastructure"
)

// DeployPrometheusForSeverityTriage deploys Prometheus in the E2E cluster and
// seeds AF-specific alert rules for the 5-tier severity triage pipeline tests.
//
// This delegates to kubernaut's canonical DeployPrometheus (DD-TEST-001 v2.8)
// and then patches the rules ConfigMap with AF's triage fixtures.
//
// Ref: Prometheus OTLP receiver — https://prometheus.io/docs/guides/opentelemetry/
func DeployPrometheusForSeverityTriage(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "Deploying Prometheus for severity triage testing...")

	if err := kinfra.DeployPrometheus(ctx, namespace, kubeconfigPath, writer); err != nil {
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

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infra, args from test constants
		"patch", "configmap", "prometheus-rules",
		"-n", namespace,
		"--type=merge",
		"-p", patchJSON)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("patch prometheus-rules ConfigMap: %w", err)
	}

	// Restart Prometheus to pick up new rules
	restartCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"rollout", "restart", "deployment/prometheus",
		"-n", namespace)
	restartCmd.Stdout = writer
	restartCmd.Stderr = writer
	if err := restartCmd.Run(); err != nil {
		return fmt.Errorf("restart Prometheus: %w", err)
	}

	// Wait for Prometheus to be ready again
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

// prometheusRulesResponse models the /api/v1/rules response.
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

// InjectOTLPMetrics sends metrics to Prometheus via the OTLP HTTP endpoint.
// Requires Prometheus started with --web.enable-otlp-receiver.
func InjectOTLPMetrics(ctx context.Context, prometheusURL, metricName string, value float64, labels map[string]string) error {
	labelAttrs := make([]map[string]any, 0, len(labels))
	for k, v := range labels {
		labelAttrs = append(labelAttrs, map[string]any{
			"key":   k,
			"value": map[string]string{"stringValue": v},
		})
	}

	payload := map[string]any{
		"resourceMetrics": []map[string]any{
			{
				"resource": map[string]any{
					"attributes": []map[string]any{
						{"key": "service.name", "value": map[string]string{"stringValue": "e2e-test"}},
					},
				},
				"scopeMetrics": []map[string]any{
					{
						"scope": map[string]any{"name": "e2e-test"},
						"metrics": []map[string]any{
							{
								"name": metricName,
								"gauge": map[string]any{
									"dataPoints": []map[string]any{
										{
											"asDouble":          value,
											"timeUnixNano":      fmt.Sprintf("%d", time.Now().UnixNano()),
											"startTimeUnixNano": fmt.Sprintf("%d", time.Now().Add(-10*time.Second).UnixNano()),
											"attributes":        labelAttrs,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal OTLP payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		prometheusURL+"/api/v1/otlp/v1/metrics",
		strings.NewReader(string(jsonPayload)))
	if err != nil {
		return fmt.Errorf("create OTLP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send OTLP metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OTLP inject failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}
