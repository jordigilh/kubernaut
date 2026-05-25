package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

var _ = Describe("Severity Triage Pipeline (G12)", Label("e2e", "phase4", "g12"), func() {
	var authToken string
	var prometheusReachable, prometheusAlertsReady bool

	BeforeEach(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
		Expect(authToken).NotTo(BeEmpty())

		for _, nsName := range []string{"sev-tier2-ns", "no-data-ns", "no-rules-ns"} {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			err := k8sClient.Create(context.Background(), ns)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: failed to create namespace %s: %v\n", nsName, err)
			}
		}

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
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				prometheusReachable = resp.StatusCode == http.StatusOK
				prometheusAlertsReady = strings.Contains(string(body), `"firing"`) && strings.Contains(string(body), "HighCPU")
			}
		}
	})

	skipIfNoPrometheus := func() {
		if !prometheusReachable {
			Skip("Prometheus not reachable from test host — triage pipeline tests require Prometheus infrastructure")
		}
	}

	skipIfNoAlerts := func() {
		skipIfNoPrometheus()
		if !prometheusAlertsReady {
			Skip("Prometheus alerts not firing — OTLP metric injection timing issue")
		}
	}

	// a2aCreateRR sends an A2A request to create an RR and returns the full response text.
	a2aCreateRR := func(namespace, deployName string) (string, error) {
		prompt := fmt.Sprintf("Create a remediation request for deployment %s in %s namespace", deployName, namespace)
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
		return string(rpc.Result), nil
	}

	// readRRFromCRD finds the most recently created RR whose name matches the
	// pattern "rr-deployment-{deployName}-*" in the given namespace and returns
	// severity, severitySource and signalName from its spec.
	readRRFromCRD := func(namespace, deployName string) (severity, severitySource, signalName string, err error) {
		prefix := fmt.Sprintf("rr-deployment-%s-", strings.ToLower(deployName))
		rrList := &remediationv1alpha1.RemediationRequestList{}
		if listErr := k8sClient.List(context.Background(), rrList, client.InNamespace(namespace)); listErr != nil {
			return "", "", "", fmt.Errorf("list RemediationRequests failed: %w", listErr)
		}

		items := append([]remediationv1alpha1.RemediationRequest(nil), rrList.Items...)
		sort.Slice(items, func(i, j int) bool {
			return items[i].CreationTimestamp.Before(&items[j].CreationTimestamp)
		})

		// Walk in reverse (sorted by creation) to find the most recent matching RR.
		for i := len(items) - 1; i >= 0; i-- {
			item := items[i]
			if strings.HasPrefix(item.Name, prefix) {
				return item.Spec.Severity,
					item.Spec.SignalLabels["severity_source"],
					item.Spec.SignalName,
					nil
			}
		}
		return "", "", "", fmt.Errorf("no RR matching prefix %q found in namespace %s", prefix, namespace)
	}

	It("TC-E2E-SEV-01: Tier 1 — Firing alert", func() {
		skipIfNoAlerts()
		text, err := a2aCreateRR("default", "test-firing-target")
		Expect(err).NotTo(HaveOccurred(), text)
		Expect(strings.ToLower(text)).To(ContainSubstring("remediation request"),
			"A2A response should confirm RR creation")

		sev, src, _, rrErr := readRRFromCRD("default", "test-firing-target")
		Expect(rrErr).NotTo(HaveOccurred())
		Expect(sev).To(Equal("critical"))
		Expect(src).To(Equal("firing_alert"))
	})

	It("TC-E2E-SEV-02: Tier 1.5 — Pending alert", func() {
		skipIfNoAlerts()
		text, err := a2aCreateRR("default", "test-pending-target")
		Expect(err).NotTo(HaveOccurred(), text)

		_, src, _, rrErr := readRRFromCRD("default", "test-pending-target")
		Expect(rrErr).NotTo(HaveOccurred())
		Expect(src).To(BeElementOf("pending_alert", "firing_alert", "llm_rule_informed", "llm_triage"),
			"expected Prometheus-informed source, got: %s", src)
	})

	It("TC-E2E-SEV-03: Tier 2 — Inactive rule with live data", func() {
		skipIfNoAlerts()

		promURL := "http://localhost:9190"
		if envProm := os.Getenv("AF_E2E_PROMETHEUS_URL"); envProm != "" {
			promURL = envProm
		}
		ctx := context.Background()
		err := injectMetricForTier2(ctx, promURL, "e2e_disk_usage_percent", 80, map[string]string{
			"namespace": "sev-tier2-ns", "kind": "Deployment", "name": "test-inactive-target",
		})
		if err != nil {
			Skip("Could not inject disk metric for Tier 2 test: " + err.Error())
		}

		text, toolErr := a2aCreateRR("sev-tier2-ns", "test-inactive-target")
		Expect(toolErr).NotTo(HaveOccurred(), text)

		_, src, _, rrErr := readRRFromCRD("sev-tier2-ns", "test-inactive-target")
		Expect(rrErr).NotTo(HaveOccurred())
		Expect(src).To(BeElementOf("rule_evaluation", "llm_rule_informed", "llm_triage"),
			"expected Tier 2 or Tier 2.5 source, got: %s", src)
	})

	It("TC-E2E-SEV-04: Tier 2.5 — Inactive rule, no data", func() {
		skipIfNoAlerts()
		text, err := a2aCreateRR("no-data-ns", "test-nodata-target")
		Expect(err).NotTo(HaveOccurred(), text)

		_, src, _, rrErr := readRRFromCRD("no-data-ns", "test-nodata-target")
		Expect(rrErr).NotTo(HaveOccurred())
		Expect(src).To(BeElementOf("llm_rule_informed", "llm_triage"),
			"expected Tier 2.5 or Tier 3 source, got: %s", src)
	})

	It("TC-E2E-SEV-05: Tier 3 — No rules", func() {
		text, err := a2aCreateRR("no-rules-ns", "test-norules-target")
		Expect(err).NotTo(HaveOccurred(), text)

		_, src, _, rrErr := readRRFromCRD("no-rules-ns", "test-norules-target")
		Expect(rrErr).NotTo(HaveOccurred())
		Expect(src).To(Equal("llm_triage"))
	})

	// TC-E2E-SEV-06: Removed post-#1282. CreateRRArgs no longer accepts
	// severity — AF always resolves severity via the triage pipeline.
	// User-supplied severity bypass is no longer a supported path.

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
