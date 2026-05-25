package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Severity Triage Pipeline (G12)", Label("e2e", "phase4", "g12"), func() {
	var authToken string

	BeforeEach(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
		Expect(authToken).NotTo(BeEmpty())

		kubeconfigPath := os.Getenv("HOME") + "/.kube/apifrontend-e2e-config"
		for _, ns := range []string{"sev-tier2-ns", "no-data-ns", "no-rules-ns"} {
			out, nsErr := exec.CommandContext(context.Background(), "kubectl",
				"--kubeconfig", kubeconfigPath,
				"create", "namespace", ns).CombinedOutput()
			if nsErr != nil && !strings.Contains(string(out), "already exists") {
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: failed to create namespace %s: %s\n", ns, string(out))
			}
		}
	})

	a2aCreateRR := func(namespace, deployName string, extraFields map[string]interface{}) (string, error) {
		severity := ""
		if s, ok := extraFields["severity"].(string); ok {
			severity = fmt.Sprintf(" with severity %s", s)
		}
		prompt := fmt.Sprintf("Create a remediation request for deployment %s in %s namespace%s", deployName, namespace, severity)
		resp, err := a2aInvoke(httpClient, baseURL, authToken, a2aTasksSend(
			fmt.Sprintf("g12-sev-%s-%d", deployName, time.Now().UnixNano()), prompt))
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return "", fmt.Errorf("A2A %d: %s", resp.StatusCode, string(body))
		}
		rpc, err := parseRPCResponse(resp)
		if err != nil {
			return "", err
		}
		if rpc.Error != nil {
			return "", fmt.Errorf("A2A error %d: %s", rpc.Error.Code, rpc.Error.Message)
		}
		return extractA2AToolJSON(rpc.Result, "rr_id"), nil
	}

	expectSeverityAndSource := func(text, wantSeverity, wantSource string) {
		Expect(text).To(ContainSubstring("severity"), "tool JSON should include severity")
		Expect(strings.ToLower(text)).To(ContainSubstring(strings.ToLower(wantSeverity)))
		Expect(text).To(ContainSubstring("severity_source"))
		Expect(parseJSONStringField(text, "severity_source")).To(Equal(wantSource))
		if wantSeverity != "" {
			Expect(parseJSONStringField(text, "severity")).To(Equal(wantSeverity))
		}
	}

	expectSeveritySource := func(text, wantSource string) {
		Expect(parseJSONStringField(text, "severity_source")).To(Equal(wantSource))
	}

	It("TC-E2E-SEV-01: Tier 1 — Firing alert", func() {
		text, err := a2aCreateRR("default", "test-firing-target", nil)
		Expect(err).NotTo(HaveOccurred(), text)
		expectSeverityAndSource(text, "critical", "firing_alert")
	})

	It("TC-E2E-SEV-02: Tier 1.5 — Pending alert", func() {
		text, err := a2aCreateRR("default", "test-pending-target", nil)
		Expect(err).NotTo(HaveOccurred(), text)
		src := parseJSONStringField(text, "severity_source")
		Expect(src).To(BeElementOf("pending_alert", "firing_alert", "llm_rule_informed", "llm_triage"),
			"expected Prometheus-informed source, got: %s (full: %s)", src, text)
	})

	It("TC-E2E-SEV-03: Tier 2 — Inactive rule with live data", func() {
		promURL := "http://localhost:9190"
		if envProm := os.Getenv("AF_E2E_PROMETHEUS_URL"); envProm != "" {
			promURL = envProm
		}
		ctx := context.Background()
		err := injectMetricForTier2(ctx, promURL, "e2e_disk_usage_percent", 80, map[string]string{
			"namespace": "sev-tier2-ns", "kind": "Deployment", "name": "test-inactive-target",
		})
		Expect(err).NotTo(HaveOccurred(), "disk metric injection must succeed for Tier 2 test")

		text, toolErr := a2aCreateRR("sev-tier2-ns", "test-inactive-target", nil)
		Expect(toolErr).NotTo(HaveOccurred(), text)
		src := parseJSONStringField(text, "severity_source")
		Expect(src).To(BeElementOf("rule_evaluation", "llm_rule_informed", "llm_triage"),
			"expected Tier 2 or Tier 2.5 source, got: %s (full: %s)", src, text)
	})

	It("TC-E2E-SEV-04: Tier 2.5 — Inactive rule, no data", func() {
		text, err := a2aCreateRR("no-data-ns", "test-nodata-target", nil)
		Expect(err).NotTo(HaveOccurred(), text)
		src := parseJSONStringField(text, "severity_source")
		Expect(src).To(BeElementOf("llm_rule_informed", "llm_triage"),
			"expected Tier 2.5 or Tier 3 source, got: %s (full: %s)", src, text)
	})

	It("TC-E2E-SEV-05: Tier 3 — No rules", func() {
		text, err := a2aCreateRR("no-rules-ns", "test-norules-target", nil)
		Expect(err).NotTo(HaveOccurred(), text)
		expectSeveritySource(text, "llm_triage")
	})

	It("TC-E2E-SEV-06: User-supplied severity bypasses triage", func() {
		text, err := a2aCreateRR("default", "test-user-severity-bypass", map[string]interface{}{
			"severity": "low",
		})
		Expect(err).NotTo(HaveOccurred(), text)

		var parsed map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &parsed)).To(Succeed())
		Expect(parsed).To(HaveKey("severity"))
		Expect(parsed["severity"]).To(Equal("low"))
		Expect(parsed).NotTo(HaveKey("severity_source"))
	})

})

// injectMetricForTier2 injects a metric into Prometheus via OTLP for Tier 2 testing.
func injectMetricForTier2(ctx context.Context, promURL, metricName string, value float64, labels map[string]string) error {
	labelAttrs := make([]map[string]interface{}, 0, len(labels))
	for k, v := range labels {
		labelAttrs = append(labelAttrs, map[string]interface{}{
			"key":   k,
			"value": map[string]string{"stringValue": v},
		})
	}

	payload := map[string]interface{}{
		"resourceMetrics": []map[string]interface{}{
			{
				"resource": map[string]interface{}{
					"attributes": []map[string]interface{}{
						{"key": "service.name", "value": map[string]string{"stringValue": "e2e-sev-test"}},
					},
				},
				"scopeMetrics": []map[string]interface{}{
					{
						"scope": map[string]interface{}{"name": "e2e-sev-test"},
						"metrics": []map[string]interface{}{
							{
								"name": metricName,
								"gauge": map[string]interface{}{
									"dataPoints": []map[string]interface{}{
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
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		promURL+"/api/v1/otlp/v1/metrics", strings.NewReader(string(jsonPayload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OTLP inject failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}
