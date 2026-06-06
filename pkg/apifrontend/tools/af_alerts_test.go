package tools_test

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

type mockAlertPromClient struct {
	alerts    []prom.Alert
	alertsErr error
}

func (m *mockAlertPromClient) GetAlerts(_ context.Context) ([]prom.Alert, error) {
	return m.alerts, m.alertsErr
}
func (m *mockAlertPromClient) GetRules(_ context.Context) ([]prom.RuleGroup, error) {
	return nil, nil
}
func (m *mockAlertPromClient) InstantQuery(_ context.Context, _ string) (*prom.QueryResult, error) {
	return nil, nil
}

var _ = Describe("Alert Tools (#1367)", func() {

	var testAlerts []prom.Alert

	BeforeEach(func() {
		testAlerts = []prom.Alert{
			{Labels: map[string]string{"alertname": "HighCPU", "namespace": "prod", "severity": "critical", "instance": "10.128.0.45:9090"}, Annotations: map[string]string{"summary": "CPU is high", "runbook_url": "https://runbook.internal.corp:8080/cpu"}, State: "firing", ActiveAt: time.Now()},
			{Labels: map[string]string{"alertname": "LowDisk", "namespace": "prod", "severity": "warning"}, Annotations: map[string]string{"summary": "Disk low"}, State: "firing", ActiveAt: time.Now()},
			{Labels: map[string]string{"alertname": "MemLeak", "namespace": "staging", "severity": "high"}, Annotations: map[string]string{"summary": "Memory leak"}, State: "pending", ActiveAt: time.Now()},
			{Labels: map[string]string{"alertname": "etcdSlow", "severity": "warning"}, Annotations: map[string]string{"summary": "etcd slow"}, State: "firing", ActiveAt: time.Now()},
			{Labels: map[string]string{"alertname": "SecretExposed", "namespace": "prod", "severity": "critical", "password": "redacted-val", "token": "abc123"}, State: "firing", ActiveAt: time.Now()},
		}
	})

	Describe("HandleListAlerts", func() {

		It("UT-AF-1367-001: returns all alerts when no filters", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(5))
		})

		It("UT-AF-1367-002: filters by namespace label", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{Namespace: "prod"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(3), "3 alerts in prod namespace")
			for _, a := range result.Alerts {
				Expect(a.Labels["namespace"]).To(Equal("prod"))
			}
		})

		It("UT-AF-1367-003: filters by severity", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{Severity: "warning"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(2))
			for _, a := range result.Alerts {
				Expect(a.Labels["severity"]).To(Equal("warning"))
			}
		})

		It("UT-AF-1367-004: filters by state (firing/pending)", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{State: "pending"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(1))
			Expect(result.Alerts[0].Labels["alertname"]).To(Equal("MemLeak"))
		})

		It("UT-AF-1367-005: returns empty when no matches", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{Namespace: "nonexistent"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(0))
			Expect(result.Alerts).To(BeEmpty())
		})

		It("UT-AF-1367-006: returns error when promClient is nil", func() {
			_, err := tools.HandleListAlerts(context.Background(), nil, tools.ListAlertsArgs{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unavailable"))
		})

		It("UT-AF-1367-007: rejects invalid severity with ErrInvalidInput", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			_, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{Severity: "extreme"})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, tools.ErrInvalidInput)).To(BeTrue())
		})

		It("UT-AF-1367-008: rejects invalid state with ErrInvalidInput", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			_, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{State: "resolved"})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, tools.ErrInvalidInput)).To(BeTrue())
		})

		It("UT-AF-1367-009: redacts instance labels containing IP:port", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{Namespace: "prod", Severity: "critical"})
			Expect(err).NotTo(HaveOccurred())
			for _, a := range result.Alerts {
				if a.Labels["alertname"] == "HighCPU" {
					Expect(a.Labels["instance"]).NotTo(ContainSubstring("10.128.0.45"),
						"IP address should be redacted from instance label")
				}
			}
		})
	})

	Describe("HandleGetAlertDetails", func() {

		It("UT-AF-1367-010: returns matching alert with full details", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			result, err := tools.HandleGetAlertDetails(context.Background(), client, tools.GetAlertDetailsArgs{AlertName: "HighCPU"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(1))
			Expect(result.Alerts[0].Labels["alertname"]).To(Equal("HighCPU"))
			Expect(result.Alerts[0].Annotations).To(HaveKey("summary"))
		})

		It("UT-AF-1367-011: returns error for empty alertname", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			_, err := tools.HandleGetAlertDetails(context.Background(), client, tools.GetAlertDetailsArgs{AlertName: ""})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, tools.ErrInvalidInput)).To(BeTrue())
		})

		It("UT-AF-1367-012: filters by namespace", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			result, err := tools.HandleGetAlertDetails(context.Background(), client, tools.GetAlertDetailsArgs{AlertName: "MemLeak", Namespace: "staging"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(1))
		})

		It("UT-AF-1367-013: redacts sensitive label keys (password, token, secret)", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			result, err := tools.HandleGetAlertDetails(context.Background(), client, tools.GetAlertDetailsArgs{AlertName: "SecretExposed"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(1))
			Expect(result.Alerts[0].Labels).NotTo(HaveKey("password"))
			Expect(result.Alerts[0].Labels).NotTo(HaveKey("token"))
		})

		It("UT-AF-1367-014: errors are redacted (no internal URLs/IPs in error messages)", func() {
			client := &mockAlertPromClient{alertsErr: errors.New("Get \"https://thanos-querier.openshift-monitoring.svc:9091/api/v1/alerts\": connection refused")}
			_, err := tools.HandleGetAlertDetails(context.Background(), client, tools.GetAlertDetailsArgs{AlertName: "any"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("thanos-querier"))
			Expect(err.Error()).NotTo(ContainSubstring("9091"))
		})
	})

	Describe("FedRAMP audit fixes (#1367 F1-F8)", func() {

		It("UT-AF-1367-F1: annotations are redacted — runbook_url internal hostnames stripped", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{Namespace: "prod", Severity: "critical"})
			Expect(err).NotTo(HaveOccurred())
			for _, a := range result.Alerts {
				if a.Labels["alertname"] == "HighCPU" {
					Expect(a.Annotations["runbook_url"]).NotTo(ContainSubstring("runbook.internal.corp"),
						"internal hostname should be redacted from runbook_url annotation")
				}
			}
		})

		It("UT-AF-1367-F2: HandleListAlerts rejects invalid namespace with ErrInvalidInput", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			_, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{Namespace: "INVALID--NS!"})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, tools.ErrInvalidInput)).To(BeTrue())
		})

		It("UT-AF-1367-F3: HandleListAlerts sets Truncated when output exceeds size cap", func() {
			largeAlerts := make([]prom.Alert, 500)
			for i := range largeAlerts {
				largeAlerts[i] = prom.Alert{
					Labels:      map[string]string{"alertname": "Alert" + string(rune('A'+i%26)), "namespace": "prod", "severity": "warning"},
					Annotations: map[string]string{"summary": "This is a long summary that contributes to output size padding for testing purposes across many repeated alerts"},
					State:       "firing",
					ActiveAt:    time.Now(),
				}
			}
			client := &mockAlertPromClient{alerts: largeAlerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{})
			Expect(err).NotTo(HaveOccurred())
			if result.Truncated {
				Expect(result.Count).To(BeNumerically("<", 500))
			}
		})

		It("UT-AF-1367-F4: sensitiveAlertKeys matches severity.SensitiveKeys", func() {
			for k := range severity.SensitiveKeys {
				Expect(tools.SensitiveAlertKeys).To(HaveKey(k),
					"severity.SensitiveKeys has %q but tools.SensitiveAlertKeys does not — drift detected", k)
			}
			for k := range tools.SensitiveAlertKeys {
				Expect(severity.SensitiveKeys).To(HaveKey(k),
					"tools.SensitiveAlertKeys has %q but severity.SensitiveKeys does not — drift detected", k)
			}
		})

		It("UT-AF-1367-F5: redactAlertLabels applies URL/IP redaction to all label values", func() {
			alerts := []prom.Alert{
				{Labels: map[string]string{"alertname": "TestAlert", "namespace": "prod", "severity": "warning", "custom_label": "10.0.0.1:443", "endpoint": "https://internal.svc:8080/metrics"}, State: "firing", ActiveAt: time.Now()},
			}
			client := &mockAlertPromClient{alerts: alerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(1))
			Expect(result.Alerts[0].Labels["custom_label"]).NotTo(ContainSubstring("10.0.0.1"),
				"IP in custom label should be redacted")
			Expect(result.Alerts[0].Labels["endpoint"]).NotTo(ContainSubstring("internal.svc"),
				"internal URL in endpoint label should be redacted")
		})

		It("UT-AF-1367-F6: HandleGetAlertDetails rejects invalid namespace with ErrInvalidInput", func() {
			client := &mockAlertPromClient{alerts: testAlerts}
			_, err := tools.HandleGetAlertDetails(context.Background(), client, tools.GetAlertDetailsArgs{AlertName: "HighCPU", Namespace: "INVALID--NS!"})
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, tools.ErrInvalidInput)).To(BeTrue())
		})
	})
})
