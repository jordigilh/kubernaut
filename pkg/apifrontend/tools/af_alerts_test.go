package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	Describe("PrioritizeAlerts (#1412)", func() {
		var now time.Time

		BeforeEach(func() {
			now = time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
		})

		It("UT-AF-1412-010: single alert — SelectedIndex is 0, TiedIndices and AlsoActiveStart empty", func() {
			alerts := []tools.AlertSummary{
				{Labels: map[string]string{"alertname": "HighCPU", "severity": "critical"}, State: "firing", ActiveAt: now},
			}
			result := tools.PrioritizeAlerts(alerts)
			Expect(result.SelectedIndex).To(Equal(0))
			Expect(alerts[result.SelectedIndex].Labels["alertname"]).To(Equal("HighCPU"))
			Expect(result.TiedIndices).To(BeEmpty())
			Expect(result.AlsoActiveStart).To(Equal(1))
		})

		It("UT-AF-1412-020: multiple severities — highest selected, others after AlsoActiveStart", func() {
			alerts := []tools.AlertSummary{
				{Labels: map[string]string{"alertname": "LowDisk", "severity": "warning"}, State: "firing", ActiveAt: now},
				{Labels: map[string]string{"alertname": "HighCPU", "severity": "critical"}, State: "firing", ActiveAt: now},
				{Labels: map[string]string{"alertname": "InfoAlert", "severity": "info"}, State: "firing", ActiveAt: now},
			}
			result := tools.PrioritizeAlerts(alerts)
			Expect(result.SelectedIndex).To(Equal(0))
			Expect(alerts[result.SelectedIndex].Labels["alertname"]).To(Equal("HighCPU"))
			Expect(alerts[result.SelectedIndex].Labels["severity"]).To(Equal("critical"))
			Expect(result.TiedIndices).To(BeEmpty())
			Expect(result.AlsoActiveStart).To(Equal(1))
			Expect(len(alerts[result.AlsoActiveStart:])).To(Equal(2))
		})

		It("UT-AF-1412-030: tie at same severity — FIFO (oldest ActiveAt) wins, others in TiedIndices", func() {
			alerts := []tools.AlertSummary{
				{Labels: map[string]string{"alertname": "NewerAlert", "severity": "critical"}, State: "firing", ActiveAt: now.Add(5 * time.Minute)},
				{Labels: map[string]string{"alertname": "OlderAlert", "severity": "critical"}, State: "firing", ActiveAt: now},
				{Labels: map[string]string{"alertname": "InfoAlert", "severity": "info"}, State: "firing", ActiveAt: now},
			}
			result := tools.PrioritizeAlerts(alerts)
			Expect(result.SelectedIndex).To(Equal(0))
			Expect(alerts[result.SelectedIndex].Labels["alertname"]).To(Equal("OlderAlert"), "FIFO: oldest ActiveAt wins")
			Expect(result.TiedIndices).To(Equal([]int{1}))
			Expect(alerts[result.TiedIndices[0]].Labels["alertname"]).To(Equal("NewerAlert"))
			Expect(result.AlsoActiveStart).To(Equal(2))
			Expect(alerts[result.AlsoActiveStart].Labels["alertname"]).To(Equal("InfoAlert"))
		})

		It("UT-AF-1412-040: all same severity — oldest selected, rest in TiedIndices", func() {
			alerts := []tools.AlertSummary{
				{Labels: map[string]string{"alertname": "C", "severity": "warning"}, State: "firing", ActiveAt: now.Add(10 * time.Minute)},
				{Labels: map[string]string{"alertname": "A", "severity": "warning"}, State: "firing", ActiveAt: now},
				{Labels: map[string]string{"alertname": "B", "severity": "warning"}, State: "firing", ActiveAt: now.Add(5 * time.Minute)},
			}
			result := tools.PrioritizeAlerts(alerts)
			Expect(result.SelectedIndex).To(Equal(0))
			Expect(alerts[result.SelectedIndex].Labels["alertname"]).To(Equal("A"), "FIFO: oldest ActiveAt wins")
			Expect(result.TiedIndices).To(HaveLen(2))
			Expect(result.AlsoActiveStart).To(Equal(3))
		})

		It("UT-AF-1412-050: empty alerts — SelectedIndex is 0, no indices", func() {
			result := tools.PrioritizeAlerts([]tools.AlertSummary{})
			Expect(result.SelectedIndex).To(Equal(0))
			Expect(result.TiedIndices).To(BeEmpty())
			Expect(result.AlsoActiveStart).To(Equal(0))
		})

		It("UT-AF-1412-060: warning vs info — warning is selected (not info)", func() {
			alerts := []tools.AlertSummary{
				{Labels: map[string]string{"alertname": "InfoAlert", "severity": "info"}, State: "firing", ActiveAt: now},
				{Labels: map[string]string{"alertname": "WarnAlert", "severity": "warning"}, State: "firing", ActiveAt: now.Add(time.Minute)},
			}
			result := tools.PrioritizeAlerts(alerts)
			Expect(result.SelectedIndex).To(Equal(0))
			Expect(alerts[result.SelectedIndex].Labels["alertname"]).To(Equal("WarnAlert"), "warning should outrank info")
			Expect(result.AlsoActiveStart).To(Equal(1))
			Expect(alerts[result.AlsoActiveStart].Labels["alertname"]).To(Equal("InfoAlert"))
		})
	})

	Describe("HandleListAlerts prioritization wiring (IT-AF-1412)", func() {
		It("IT-AF-1412-001: HandleListAlerts returns Prioritized.SelectedIndex as highest-severity alert", func() {
			alerts := []prom.Alert{
				{Labels: map[string]string{"alertname": "LowDisk", "namespace": "prod", "severity": "warning"}, State: "firing", ActiveAt: time.Now().Add(-10 * time.Minute)},
				{Labels: map[string]string{"alertname": "HighCPU", "namespace": "prod", "severity": "critical"}, State: "firing", ActiveAt: time.Now()},
				{Labels: map[string]string{"alertname": "InfoAlert", "namespace": "prod", "severity": "info"}, State: "firing", ActiveAt: time.Now()},
			}
			client := &mockAlertPromClient{alerts: alerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{Namespace: "prod"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Prioritized).NotTo(BeNil(), "Prioritized field must be populated by HandleListAlerts")
			Expect(result.Prioritized.SelectedIndex).To(Equal(0))
			Expect(result.Alerts[result.Prioritized.SelectedIndex].Labels["alertname"]).To(Equal("HighCPU"))
			Expect(result.Alerts[result.Prioritized.SelectedIndex].Labels["severity"]).To(Equal("critical"))
			Expect(result.Prioritized.AlsoActiveStart).To(Equal(1))
		})

		It("IT-AF-1412-002: HandleListAlerts returns tied indices for same-severity alerts", func() {
			alerts := []prom.Alert{
				{Labels: map[string]string{"alertname": "A", "namespace": "prod", "severity": "critical"}, State: "firing", ActiveAt: time.Now().Add(-5 * time.Minute)},
				{Labels: map[string]string{"alertname": "B", "namespace": "prod", "severity": "critical"}, State: "firing", ActiveAt: time.Now()},
			}
			client := &mockAlertPromClient{alerts: alerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{Namespace: "prod"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Prioritized).NotTo(BeNil(), "Prioritized field must be populated")
			Expect(result.Prioritized.SelectedIndex).To(Equal(0))
			Expect(result.Alerts[0].Labels["alertname"]).To(Equal("A"), "FIFO: older alert selected")
			Expect(result.Prioritized.TiedIndices).To(Equal([]int{1}))
		})
	})

	Describe("Index-based prioritization and trimming (#1434)", func() {
		It("UT-AF-1434-001: alerts[] is sorted by priority (severity desc, FIFO asc)", func() {
			now := time.Now()
			alerts := []prom.Alert{
				{Labels: map[string]string{"alertname": "InfoAlert", "namespace": "prod", "severity": "info"}, State: "firing", ActiveAt: now},
				{Labels: map[string]string{"alertname": "Critical1", "namespace": "prod", "severity": "critical"}, State: "firing", ActiveAt: now.Add(-5 * time.Minute)},
				{Labels: map[string]string{"alertname": "Warning1", "namespace": "prod", "severity": "warning"}, State: "firing", ActiveAt: now},
				{Labels: map[string]string{"alertname": "Critical2", "namespace": "prod", "severity": "critical"}, State: "firing", ActiveAt: now},
			}
			client := &mockAlertPromClient{alerts: alerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Alerts[0].Labels["alertname"]).To(Equal("Critical1"), "highest severity, oldest first")
			Expect(result.Alerts[1].Labels["alertname"]).To(Equal("Critical2"), "same severity, newer")
			Expect(result.Alerts[2].Labels["alertname"]).To(Equal("Warning1"), "next severity tier")
			Expect(result.Alerts[3].Labels["alertname"]).To(Equal("InfoAlert"), "lowest severity")
		})

		It("UT-AF-1434-002: total_count reflects pre-truncation count", func() {
			largeAlerts := make([]prom.Alert, 50)
			for i := range largeAlerts {
				largeAlerts[i] = prom.Alert{
					Labels:      map[string]string{"alertname": fmt.Sprintf("Alert%d", i), "namespace": "prod", "severity": "warning", "pod": fmt.Sprintf("pod-with-long-name-%d-suffix", i), "container": "my-container", "job": "kube-state-metrics", "service": "kube-state-metrics", "instance": "[HOST_REDACTED]", "prometheus": "monitoring/k8s", "endpoint": "https-main"},
					Annotations: map[string]string{"summary": fmt.Sprintf("Alert %d is firing with a reasonably long summary description for testing purposes", i), "description": fmt.Sprintf("Detailed description for alert %d that contributes to the overall payload size", i), "runbook_url": "[URL_REDACTED]"},
					State:       "firing",
					ActiveAt:    time.Now().Add(-time.Duration(i) * time.Minute),
				}
			}
			client := &mockAlertPromClient{alerts: largeAlerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.TotalCount).To(Equal(50), "total_count must reflect all matching alerts before truncation")
			Expect(result.Count).To(BeNumerically("<", 50), "count should be less after truncation")
			Expect(result.Truncated).To(BeTrue())
		})

		It("UT-AF-1434-003: 5 realistic alerts fit within maxToolOutputBytes (16384)", func() {
			now := time.Now()
			alerts := []prom.Alert{
				{Labels: map[string]string{"alertname": "KubePodNotReady", "namespace": "production-payments", "severity": "critical", "pod": "payment-service-7d4f5b9c8-xk2nq", "container": "payment-processor", "instance": "10.128.0.45:9090", "job": "kube-state-metrics", "service": "kube-state-metrics", "prometheus": "monitoring/k8s", "endpoint": "https-main"}, Annotations: map[string]string{"summary": "Pod production-payments/payment-service-7d4f5b9c8-xk2nq has been in a non-ready state for longer than 15 minutes.", "description": "Pod payment-service-7d4f5b9c8-xk2nq in namespace production-payments has been in a non-ready state for longer than 15 minutes.", "runbook_url": "https://runbook.internal.corp/cpu"}, State: "firing", ActiveAt: now},
				{Labels: map[string]string{"alertname": "KubeDeploymentReplicasMismatch", "namespace": "production-payments", "severity": "critical", "deployment": "payment-service", "instance": "10.128.0.45:9090", "job": "kube-state-metrics", "service": "kube-state-metrics", "prometheus": "monitoring/k8s", "endpoint": "https-main"}, Annotations: map[string]string{"summary": "Deployment production-payments/payment-service has not matched the expected number of replicas for longer than 15 minutes.", "description": "Deployment production-payments/payment-service has 1/3 replicas available.", "runbook_url": "https://runbook.internal.corp/deploy"}, State: "firing", ActiveAt: now.Add(-5 * time.Minute)},
				{Labels: map[string]string{"alertname": "etcdHighCommitDurations", "namespace": "openshift-etcd", "severity": "warning", "pod": "etcd-master-0", "instance": "10.128.0.10:2379", "job": "etcd", "service": "etcd-metrics", "prometheus": "monitoring/k8s", "endpoint": "metrics"}, Annotations: map[string]string{"summary": "etcd cluster openshift-etcd has high commit durations exceeding 500ms on 3 members.", "description": "etcd cluster openshift-etcd commit durations 99th percentile is 678ms on etcd-master-0.", "runbook_url": "https://runbook.internal.corp/etcd"}, State: "firing", ActiveAt: now.Add(-15 * time.Minute)},
				{Labels: map[string]string{"alertname": "NodeFilesystemSpaceFillingUp", "severity": "warning", "node": "worker-node-03.cluster.local", "device": "/dev/sda1", "mountpoint": "/var/lib/containers", "fstype": "xfs", "instance": "10.128.0.33:9100", "job": "node-exporter", "prometheus": "monitoring/k8s"}, Annotations: map[string]string{"summary": "Filesystem on worker-node-03.cluster.local at /var/lib/containers is predicted to run out of space within 24 hours.", "description": "Filesystem on worker-node-03.cluster.local mounted at /var/lib/containers is filling up.", "runbook_url": "https://runbook.internal.corp/disk"}, State: "firing", ActiveAt: now.Add(-30 * time.Minute)},
				{Labels: map[string]string{"alertname": "KubeContainerWaiting", "namespace": "staging-analytics", "severity": "warning", "pod": "analytics-worker-5f8b9d7c6-m2kpv", "container": "worker", "instance": "10.128.0.45:9090", "job": "kube-state-metrics", "service": "kube-state-metrics", "prometheus": "monitoring/k8s"}, Annotations: map[string]string{"summary": "Container worker in pod analytics-worker-5f8b9d7c6-m2kpv has been in waiting state for over 1 hour.", "description": "Container worker in pod staging-analytics/analytics-worker-5f8b9d7c6-m2kpv is in CrashLoopBackOff.", "runbook_url": "https://runbook.internal.corp/crashloop"}, State: "firing", ActiveAt: now.Add(-60 * time.Minute)},
			}
			client := &mockAlertPromClient{alerts: alerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Count).To(Equal(5), "all 5 alerts must fit")
			Expect(result.Truncated).To(BeFalse(), "5 alerts should not trigger truncation")

			resultJSON, jsonErr := json.Marshal(result)
			Expect(jsonErr).NotTo(HaveOccurred())
			Expect(len(resultJSON)).To(BeNumerically("<=", 16384),
				"full ListAlertsResult with 5 realistic alerts must fit in 16384 bytes, got %d", len(resultJSON))
		})

		It("UT-AF-1434-004: prioritized uses indices not full alert copies", func() {
			now := time.Now()
			alerts := []prom.Alert{
				{Labels: map[string]string{"alertname": "A", "namespace": "prod", "severity": "critical"}, State: "firing", ActiveAt: now.Add(-5 * time.Minute)},
				{Labels: map[string]string{"alertname": "B", "namespace": "prod", "severity": "critical"}, State: "firing", ActiveAt: now},
				{Labels: map[string]string{"alertname": "C", "namespace": "prod", "severity": "warning"}, State: "firing", ActiveAt: now},
			}
			client := &mockAlertPromClient{alerts: alerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Prioritized).NotTo(BeNil())
			Expect(result.Prioritized.SelectedIndex).To(Equal(0))
			Expect(result.Prioritized.TiedIndices).To(Equal([]int{1}))
			Expect(result.Prioritized.AlsoActiveStart).To(Equal(2))
		})

		It("UT-AF-1434-005: trimResultToFit trims lowest-priority alerts from tail", func() {
			largeAlerts := make([]prom.Alert, 50)
			for i := range largeAlerts {
				largeAlerts[i] = prom.Alert{
					Labels:      map[string]string{"alertname": fmt.Sprintf("Alert%d", i), "namespace": "prod", "severity": "warning", "pod": fmt.Sprintf("pod-with-long-name-%d-suffix", i), "container": "my-container", "job": "kube-state-metrics", "service": "kube-state-metrics", "instance": "[HOST_REDACTED]", "prometheus": "monitoring/k8s", "endpoint": "https-main"},
					Annotations: map[string]string{"summary": fmt.Sprintf("Alert %d is firing with a reasonably long summary description for testing", i), "description": fmt.Sprintf("Detailed description for alert %d contributes to overall size", i), "runbook_url": "[URL_REDACTED]"},
					State:       "firing",
					ActiveAt:    time.Now().Add(-time.Duration(i) * time.Minute),
				}
			}
			client := &mockAlertPromClient{alerts: largeAlerts}
			result, err := tools.HandleListAlerts(context.Background(), client, tools.ListAlertsArgs{})
			Expect(err).NotTo(HaveOccurred())

			resultJSON, jsonErr := json.Marshal(result)
			Expect(jsonErr).NotTo(HaveOccurred())
			Expect(len(resultJSON)).To(BeNumerically("<=", 16384),
				"trimmed result must fit in 16384 bytes, got %d", len(resultJSON))
			Expect(result.Count).To(BeNumerically(">=", 3), "should retain at least 3 alerts after trimming")
		})
	})
})
