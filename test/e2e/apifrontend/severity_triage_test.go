package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	kinfra "github.com/jordigilh/kubernaut/test/infrastructure"
)

var _ = Describe("Severity Triage Pipeline (G12)", Label("e2e", "phase4", "g12"), func() {
	var authToken string

	BeforeEach(func() {
		var err error
		authToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
		Expect(authToken).NotTo(BeEmpty())

		for _, nsName := range []string{"sev-tier2-ns", "no-data-ns", "no-rules-ns"} {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
			err = k8sClient.Create(context.Background(), ns)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: failed to create namespace %s: %v\n", nsName, err)
			}
		}
	})

	a2aCreateRRAndWait := func(namespace, deployName string, promptSuffix string) {
		prompt := fmt.Sprintf("Create a remediation request for deployment %s in %s namespace%s", deployName, namespace, promptSuffix)
		resp, err := a2aInvoke(httpClient, baseURL, authToken, a2aTasksSend(
			fmt.Sprintf("g12-sev-%s-%d", deployName, time.Now().UnixNano()), prompt))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			Fail(fmt.Sprintf("A2A %d: %s", resp.StatusCode, string(body)))
		}
		rpc, err := parseRPCResponse(resp)
		Expect(err).NotTo(HaveOccurred())
		if rpc.Error != nil {
			Fail(fmt.Sprintf("A2A error %d: %s", rpc.Error.Code, rpc.Error.Message))
		}

		if rpc.Result != nil {
			task, taskErr := extractTaskFromResult(rpc.Result)
			if taskErr == nil && task.Status.State == "failed" {
				Fail(fmt.Sprintf("A2A task failed: %s", string(task.Status.Message)))
			}
			if taskErr == nil && task.Status.Message != nil {
				msgStr := string(task.Status.Message)
				Expect(msgStr).NotTo(ContainSubstring("circuit breaker"),
					"kubernaut_remediate failed due to K8s circuit breaker — cluster not ready")
				Expect(msgStr).NotTo(ContainSubstring("ErrK8sUnavailable"),
					"kubernaut_remediate failed — K8s client unavailable")
			}
		}
	}

	findRRByTarget := func(namespace, deployName string) *remediationv1alpha1.RemediationRequest {
		var found *remediationv1alpha1.RemediationRequest
		Eventually(func(g Gomega) {
			list := &remediationv1alpha1.RemediationRequestList{}
			g.Expect(k8sClient.List(context.Background(), list, client.InNamespace(e2eNamespace))).To(Succeed())
			for i := range list.Items {
				rr := &list.Items[i]
				if rr.Spec.TargetResource.Name == deployName &&
					rr.Spec.TargetResource.Namespace == namespace {
					found = rr
					return
				}
			}
			g.Expect(found).NotTo(BeNil(), "RR for %s/%s not found in %s", namespace, deployName, e2eNamespace)
		}, 30*time.Second, 1*time.Second).Should(Succeed())
		return found
	}

	It("TC-E2E-SEV-01: Tier 1 — Firing alert", func() {
		promURL := "http://localhost:9190"
		if envProm := os.Getenv("AF_E2E_PROMETHEUS_URL"); envProm != "" {
			promURL = envProm
		}
		ctx := context.Background()
		Expect(injectMetricForTier2(ctx, promURL, "e2e_cpu_usage_percent", 95, map[string]string{
			"namespace": "default", "kind": "Deployment", "name": "test-firing-target",
		})).To(Succeed(), "CPU metric re-injection must succeed for Tier 1")

		Eventually(func() error {
			return kinfra.WaitForPrometheusRuleState(ctx, promURL, "HighCPU", kinfra.RuleStateFiring, 5*time.Second)
		}, 60*time.Second, 2*time.Second).Should(Succeed(),
			"HighCPU alert must be firing before RR creation")

		a2aCreateRRAndWait("default", "test-firing-target", "")
		rr := findRRByTarget("default", "test-firing-target")
		Expect(rr.Spec.Severity).To(Equal("critical"), "spec.severity")
		Expect(rr.Spec.SignalLabels).To(HaveKeyWithValue("severity_source", "firing_alert"))
	})

	It("TC-E2E-SEV-02: Tier 1.5 — Pending alert", func() {
		a2aCreateRRAndWait("default", "test-pending-target", "")
		rr := findRRByTarget("default", "test-pending-target")
		Expect(rr.Spec.Severity).NotTo(BeEmpty(), "severity must be set by triage pipeline")
		src := rr.Spec.SignalLabels["severity_source"]
		Expect(src).To(BeElementOf("pending_alert", "firing_alert", "llm_rule_informed", "llm_triage"),
			"expected Prometheus-informed source, got: %s", src)
	})

	It("TC-E2E-SEV-03: Tier 2 — Inactive rule with live data", func() {
		promURL := "http://localhost:9190"
		if envProm := os.Getenv("AF_E2E_PROMETHEUS_URL"); envProm != "" {
			promURL = envProm
		}
		ctx := context.Background()
		Expect(injectMetricForTier2(ctx, promURL, "e2e_disk_usage_percent", 80, map[string]string{
			"namespace": "sev-tier2-ns", "kind": "Deployment", "name": "test-inactive-target",
		})).To(Succeed(), "disk metric injection must succeed for Tier 2 test")

		a2aCreateRRAndWait("sev-tier2-ns", "test-inactive-target", "")
		rr := findRRByTarget("sev-tier2-ns", "test-inactive-target")
		src := rr.Spec.SignalLabels["severity_source"]
		Expect(src).To(BeElementOf("rule_evaluation", "llm_rule_informed", "llm_triage"),
			"expected Tier 2 or Tier 2.5 source, got: %s", src)
	})

	It("TC-E2E-SEV-04: Tier 2.5 — Inactive rule, no data", func() {
		a2aCreateRRAndWait("no-data-ns", "test-nodata-target", "")
		rr := findRRByTarget("no-data-ns", "test-nodata-target")
		src := rr.Spec.SignalLabels["severity_source"]
		Expect(src).To(BeElementOf("llm_rule_informed", "llm_triage"),
			"expected Tier 2.5 or Tier 3 source, got: %s", src)
	})

	It("TC-E2E-SEV-05: Tier 3 — No rules", func() {
		a2aCreateRRAndWait("no-rules-ns", "test-norules-target", "")
		rr := findRRByTarget("no-rules-ns", "test-norules-target")
		src := rr.Spec.SignalLabels["severity_source"]
		Expect(src).To(Equal("llm_triage"), "Tier 3: no rules => pure LLM triage")
	})

	It("TC-E2E-SEV-06: User severity hint does not bypass triage pipeline", func() {
		a2aCreateRRAndWait("default", "test-user-severity-bypass", " with severity low")
		rr := findRRByTarget("default", "test-user-severity-bypass")
		Expect(rr.Spec.Severity).NotTo(BeEmpty(), "severity must be set by triage pipeline")
		Expect(rr.Spec.SignalLabels).To(HaveKey("severity_source"),
			"triage pipeline must set severity_source even when user supplies a hint")
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
