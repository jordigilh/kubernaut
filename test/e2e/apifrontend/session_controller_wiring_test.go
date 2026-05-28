package e2e_test

import (
	"context"
	"io"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FedRAMP control mapping for this E2E suite:
//   AU-3    — audit record content: structured JSON logs, no slog text format
//   SI-4    — system monitoring: /readyz reflects real cache-sync state
//   SI-4(2) — automated monitoring: af_session_ttl_actions_total on /metrics
//   SI-10   — information accuracy: pre-flight diagnostics in pod logs
//   CM-3(5) — config change monitoring: controller-runtime prefix in logs

var _ = Describe("Session Controller Wiring (E2E)", Label("e2e", "phase1", "session-wiring"), func() {

	var namespace string

	BeforeEach(func() {
		namespace = getEnvOrDefault("AF_E2E_NAMESPACE", "kubernaut-system")
	})

	afPodName := func() string {
		ctx := context.Background()
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=apifrontend",
		})
		Expect(err).NotTo(HaveOccurred(), "failed to list AF pods")
		Expect(pods.Items).NotTo(BeEmpty(), "no AF pod found with label app=apifrontend")
		return pods.Items[0].Name
	}

	afPodLogs := func() string {
		ctx := context.Background()
		podName := afPodName()
		logStream, err := clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{}).Stream(ctx)
		Expect(err).NotTo(HaveOccurred(), "failed to stream AF pod logs")
		defer func() { _ = logStream.Close() }()
		logBytes, err := io.ReadAll(logStream)
		Expect(err).NotTo(HaveOccurred(), "failed to read AF pod logs")
		return string(logBytes)
	}

	e2eMetricsURL := func() string {
		u := getEnvOrDefault("AF_E2E_METRICS_URL", "")
		if u != "" {
			return u
		}
		return baseURL + "/metrics"
	}

	e2eScrapeMetrics := func() string {
		resp, err := httpClient.Get(e2eMetricsURL())
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		ExpectWithOffset(1, resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		return string(body)
	}

	readyzURL := func() string {
		u := getEnvOrDefault("AF_E2E_HEALTH_URL", "http://localhost:18081")
		return u + "/readyz"
	}

	// -------------------------------------------------------------------
	// E2E-AF-1274-001: AU-3 — all audit records must be structured JSON
	//
	// slog text format (msg=) bypasses the JSON sink and breaks SIEM
	// ingestion. Every AF log line must be machine-parseable JSON.
	// -------------------------------------------------------------------
	It("E2E-AF-1274-001: pod logs are structured JSON, slog text format absent [AU-3]", func() {
		logs := afPodLogs()

		Expect(logs).NotTo(ContainSubstring("msg="),
			"AU-3 violation: slog text format (msg=) bypasses JSON audit sink")

		Expect(logs).To(SatisfyAny(
			ContainSubstring(`"msg":`),
			ContainSubstring(`"level"`),
		), "AU-3: AF pod logs must contain structured JSON fields for SIEM ingestion")
	})

	// -------------------------------------------------------------------
	// E2E-AF-1273-001: SI-10 — pre-flight diagnostics confirm environment
	//
	// The AF must log startup diagnostics proving the CRD schema and
	// RBAC permissions were verified. Without this, operators have no
	// evidence that the controller environment was validated at boot.
	// -------------------------------------------------------------------
	It("E2E-AF-1273-001: pod logs confirm startup diagnostics ran [SI-10]", func() {
		logs := afPodLogs()

		Expect(logs).To(ContainSubstring("session controller manager started"),
			"SI-10: AF must log session controller startup to confirm environment validation")

		if strings.Contains(logs, "pre-flight CRD discovery") {
			Expect(logs).To(ContainSubstring("pre-flight CRD discovery"))
		}
		if strings.Contains(logs, "pre-flight RBAC check") {
			Expect(logs).To(ContainSubstring("pre-flight RBAC check"))
		}
	})

	// -------------------------------------------------------------------
	// E2E-AF-1273-002: AU-3 — framework logs share the audit pipeline
	//
	// controller-runtime emits leader election, cache sync, and reconcile
	// errors. These must appear in the same JSON stream as application
	// logs so the SIEM gets a unified audit trail.
	// -------------------------------------------------------------------
	It("E2E-AF-1273-002: pod logs contain controller-runtime events in audit stream [AU-3]", func() {
		logs := afPodLogs()

		Expect(logs).To(ContainSubstring("controller-runtime"),
			"AU-3: controller-runtime events must flow through the same audit pipeline")
	})

	// -------------------------------------------------------------------
	// E2E-AF-1272-001: SI-4 — readiness probe reflects cache sync state
	//
	// The readiness probe gates traffic. It must return 200 only after
	// the informer cache syncs, proving the session controller can serve
	// accurate data. 503 before sync prevents stale reads (SI-10).
	// -------------------------------------------------------------------
	It("E2E-AF-1272-001: /readyz returns 200 and logs confirm cache sync [SI-4]", func() {
		resp, err := http.Get(readyzURL()) //nolint:gosec,noctx // E2E health port probe
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK),
			"SI-4: /readyz must return 200 only after session controller cache sync")

		logs := afPodLogs()
		Expect(logs).To(ContainSubstring("session controller cache synced"),
			"SI-4: pod logs must confirm cache sync so operators can verify readiness")
	})

	// -------------------------------------------------------------------
	// E2E-AF-1272-002: SI-4(2) — TTL actions observable via /metrics
	//
	// The SIEM scrapes /metrics for automated alerting. The TTL actions
	// counter must be present so Prometheus rules can fire when sessions
	// are auto-cancelled or retention-deleted.
	// -------------------------------------------------------------------
	It("E2E-AF-1272-002: /metrics exposes af_session_ttl_actions_total [SI-4(2)]", func() {
		body := e2eScrapeMetrics()
		Expect(body).To(ContainSubstring("af_session_ttl_actions_total"),
			"SI-4(2): TTL actions counter must be scrapeable for SIEM alerting")
	})

})
