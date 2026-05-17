package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Severity Triage Pipeline (G12)", Ordered, ContinueOnFailure, Label("e2e", "phase4", "g12"), func() {
	var authToken, mcpSessionID string
	var prometheusReachable bool

	BeforeAll(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
		Expect(authToken).NotTo(BeEmpty())

		mcpSessionID, err = initMCPSession(authToken)
		Expect(err).NotTo(HaveOccurred(), "MCP initialize")

		// Check if Prometheus is accessible from the test host (for pre-flight validation)
		promURL := "http://localhost:9190"
		if envProm := os.Getenv("AF_E2E_PROMETHEUS_URL"); envProm != "" {
			promURL = envProm
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, promURL+"/api/v1/rules", http.NoBody)
		if reqErr == nil {
			resp, doErr := (&http.Client{Timeout: 5 * time.Second}).Do(req)
			if doErr == nil {
				_ = resp.Body.Close()
				prometheusReachable = resp.StatusCode == http.StatusOK
			}
		}
	})

	skipIfNoPrometheus := func() {
		if !prometheusReachable {
			Skip("Prometheus not reachable from test host — triage pipeline tests require Prometheus infrastructure")
		}
	}

	mcpToolCall := func(toolName string, args map[string]interface{}) (string, error) {
		callBody := buildJSONRPC(fmt.Sprintf("g12-%s-%d", toolName, time.Now().UnixNano()),
			"tools/call", map[string]interface{}{
				"name":      toolName,
				"arguments": args,
			})
		raw, code, err := mcpPOST(authToken, mcpSessionID, callBody)
		if err != nil {
			return "", err
		}
		if code >= http.StatusBadRequest {
			return "", fmt.Errorf("HTTP %d: %s", code, string(raw))
		}
		payload := unwrapSSEDataLine(raw)
		text, toolErr, parseErr := parseMCPToolPayload(payload)
		if parseErr != nil {
			return text, parseErr
		}
		if toolErr {
			return text, fmt.Errorf("%s", text)
		}
		return text, nil
	}

	createRRArgs := func(namespace, deployName string, extra map[string]interface{}) map[string]interface{} {
		args := map[string]interface{}{
			"namespace":   namespace,
			"name":        deployName,
			"kind":        "Deployment",
			"description": fmt.Sprintf("E2E G12 severity triage — %s/%s", namespace, deployName),
		}
		for k, v := range extra {
			args[k] = v
		}
		return args
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

	e2eKubeconfigPath := func() string {
		return os.Getenv("HOME") + "/.kube/apifrontend-e2e-config"
	}

	sumSeverityTriageTotal := func(metricsText string) float64 {
		var sum float64
		for _, line := range strings.Split(metricsText, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !strings.HasPrefix(line, "af_severity_triage_total") {
				continue
			}
			if strings.HasPrefix(line, "af_severity_triage_total_created") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
			if err != nil {
				continue
			}
			sum += v
		}
		return sum
	}

	It("TC-E2E-SEV-01: Tier 1 — Firing alert", func() {
		// Pre-check: Verify Prometheus has the HighCPU alert in firing state.
		// OTLP injection is best-effort in BeforeSuite; skip if alert infra isn't ready.
		promURL := "http://localhost:9190"
		if envProm := os.Getenv("AF_E2E_PROMETHEUS_URL"); envProm != "" {
			promURL = envProm
		}
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, promURL+"/api/v1/rules", http.NoBody)
			if err != nil {
				Skip("Prometheus not reachable from test host — cannot verify firing alert: " + err.Error())
			}
			resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
			if err != nil {
				Skip("Prometheus not reachable from test host — cannot verify firing alert: " + err.Error())
			}
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), `"firing"`) || !strings.Contains(string(body), "HighCPU") {
				Skip("HighCPU alert not in firing state — Prometheus triage infra not ready")
			}
		}()

		text, err := mcpToolCall("af_create_rr", createRRArgs("default", "test-firing-target", nil))
		Expect(err).NotTo(HaveOccurred(), text)
		expectSeverityAndSource(text, "critical", "firing_alert")
	})

	It("TC-E2E-SEV-02: Tier 1.5 — Pending alert", func() {
		skipIfNoPrometheus()
		text, err := mcpToolCall("af_create_rr", createRRArgs("default", "test-pending-target", nil))
		Expect(err).NotTo(HaveOccurred(), text)
		expectSeveritySource(text, "pending_alert")
	})

	It("TC-E2E-SEV-03: Tier 2 — Inactive rule with live data", func() {
		skipIfNoPrometheus()

		// Inject the disk metric just before calling af_create_rr.
		// The rule evaluation interval is 5s, so the DiskPressure rule is still "inactive"
		// from the previous cycle, but AF's InstantQuery will find live data.
		promURL := "http://localhost:9190"
		if envProm := os.Getenv("AF_E2E_PROMETHEUS_URL"); envProm != "" {
			promURL = envProm
		}
		ctx := context.Background()
		err := injectMetricForTier2(ctx, promURL, "e2e_disk_usage_percent", 95, map[string]string{
			"namespace": "default", "kind": "Deployment", "name": "test-inactive-target",
		})
		if err != nil {
			Skip("Could not inject disk metric for Tier 2 test: " + err.Error())
		}

		text, toolErr := mcpToolCall("af_create_rr", createRRArgs("default", "test-inactive-target", nil))
		Expect(toolErr).NotTo(HaveOccurred(), text)
		// Accept either rule_evaluation (Tier 2 hit the timing window) or
		// llm_rule_informed (rule re-evaluated and fired before AF queried).
		src := parseJSONStringField(text, "severity_source")
		Expect(src).To(BeElementOf("rule_evaluation", "llm_rule_informed"),
			"expected Tier 2 or Tier 2.5 source, got: %s (full: %s)", src, text)
	})

	It("TC-E2E-SEV-04: Tier 2.5 — Inactive rule, no data", func() {
		skipIfNoPrometheus()
		text, err := mcpToolCall("af_create_rr", createRRArgs("no-data-ns", "test-nodata-target", nil))
		Expect(err).NotTo(HaveOccurred(), text)
		expectSeveritySource(text, "llm_rule_informed")
	})

	It("TC-E2E-SEV-05: Tier 3 — No rules", func() {
		text, err := mcpToolCall("af_create_rr", createRRArgs("no-rules-ns", "test-norules-target", nil))
		Expect(err).NotTo(HaveOccurred(), text)
		expectSeveritySource(text, "llm_triage")
	})

	It("TC-E2E-SEV-06: User-supplied severity bypasses triage", func() {
		text, err := mcpToolCall("af_create_rr", createRRArgs("default", "test-user-severity-bypass", map[string]interface{}{
			"severity": "low",
		}))
		Expect(err).NotTo(HaveOccurred(), text)

		var parsed map[string]interface{}
		Expect(json.Unmarshal([]byte(text), &parsed)).To(Succeed())
		Expect(parsed).To(HaveKey("severity"))
		Expect(parsed["severity"]).To(Equal("low"))
		Expect(parsed).NotTo(HaveKey("severity_source"))
	})

	It("TC-E2E-SEV-07: Prometheus unavailable — LLM triage fallback", func() {
		kubeconfigPath := e2eKubeconfigPath()

		DeferCleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
				"scale", "deployment/prometheus", fmt.Sprintf("--replicas=%d", 1),
				"-n", e2eNamespace)
			out, err := cmd.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "restore Prometheus: %s", string(out))

			wait := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
				"rollout", "status", "deployment/prometheus", "-n", e2eNamespace, "--timeout=120s")
			wout, werr := wait.CombinedOutput()
			Expect(werr).NotTo(HaveOccurred(), "prometheus rollout: %s", string(wout))
		})

		scaleDownCtx, scaleDownCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer scaleDownCancel()
		sd := exec.CommandContext(scaleDownCtx, "kubectl", "--kubeconfig", kubeconfigPath,
			"scale", "deployment/prometheus", "--replicas=0", "-n", e2eNamespace)
		sdOut, sdErr := sd.CombinedOutput()
		Expect(sdErr).NotTo(HaveOccurred(), "scale prometheus down: %s", string(sdOut))

		// Allow Kubernetes to terminate Prometheus before triage runs.
		time.Sleep(5 * time.Second)

		text, err := mcpToolCall("af_create_rr", createRRArgs("default", "test-prom-down-target", nil))
		Expect(err).NotTo(HaveOccurred(), text)
		expectSeveritySource(text, "llm_triage")
	})

	It("TC-E2E-SEV-08: Triage metrics present on /metrics", func() {
		body := scrapeMetrics()
		Expect(sumSeverityTriageTotal(body)).To(BeNumerically(">", 0),
			"af_severity_triage_total should be incremented after triage calls")
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
