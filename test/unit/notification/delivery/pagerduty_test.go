package delivery_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

func newPagerDutyReceiver(name, credRef string) *routing.Receiver {
	return &routing.Receiver{
		Name: name,
		PagerDutyConfigs: []routing.PagerDutyConfig{
			{CredentialRef: credRef},
		},
	}
}

func newTestNotification(name, namespace string, nType notificationv1alpha1.NotificationType, priority notificationv1alpha1.NotificationPriority) *notificationv1alpha1.NotificationRequest {
	return &notificationv1alpha1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: notificationv1alpha1.NotificationRequestSpec{
			Type:     nType,
			Priority: priority,
			Subject:  "OOMKill detected in pod web-api-7d9b6c5f4-x2k9l",
			Body:     "Container memory limit exceeded. Root cause analysis suggests increasing memory limits.",
		},
	}
}

var _ = Describe("PagerDuty Delivery Channel (#60)", func() {

	Describe("Routing Config Validation (Phase 0 regression guards)", func() {

		It("UT-NOT-060-013: QualifiedChannels returns pagerduty:<receiver> for PD with CredentialRef", func() {
			receiver := newPagerDutyReceiver("oncall-critical", "pd-routing-key")
			channels := receiver.QualifiedChannels()
			Expect(channels).To(ContainElement("pagerduty:oncall-critical"))
		})

		It("UT-NOT-060-014: ValidateCredentialRefs fails when PagerDutyConfig has empty CredentialRef", func() {
			config := &routing.Config{
				Route: &routing.Route{Receiver: "pd-missing-cred"},
				Receivers: []*routing.Receiver{
					{
						Name: "pd-missing-cred",
						PagerDutyConfigs: []routing.PagerDutyConfig{
							{CredentialRef: ""},
						},
					},
				},
			}
			err := config.ValidateCredentialRefs()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credentialRef"))
			Expect(err.Error()).To(ContainSubstring("pagerdutyConfigs"))
		})
	})

	Describe("Payload Construction", func() {

		It("UT-NOT-060-001: Payload has routing_key, event_action=trigger, severity, summary, source, component", func() {
			notification := newTestNotification("oom-alert-001", "production", notificationv1alpha1.NotificationTypeEscalation, notificationv1alpha1.NotificationPriorityCritical)

			event := delivery.BuildPagerDutyPayload("test-routing-key", notification)

			Expect(event.RoutingKey).To(Equal("test-routing-key"))
			Expect(event.EventAction).To(Equal("trigger"))
			Expect(event.DedupKey).To(Equal("oom-alert-001"))
			Expect(event.Payload.Severity).To(Equal("critical"))
			Expect(event.Payload.Summary).To(Equal(notification.Spec.Subject))
			Expect(event.Payload.Source).To(Equal("kubernaut"))
			Expect(event.Payload.Component).To(Equal("production"))

			raw, err := json.Marshal(event)
			Expect(err).NotTo(HaveOccurred())
			Expect(raw).NotTo(BeEmpty())
		})

		It("UT-NOT-060-005: dedup_key equals notification.Name", func() {
			notification := newTestNotification("my-unique-nr-name", "ns1", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityLow)

			event := delivery.BuildPagerDutyPayload("key", notification)

			Expect(event.DedupKey).To(Equal("my-unique-nr-name"))
		})

		It("UT-NOT-060-003: Severity mapping covers all priorities", func() {
			cases := []struct {
				priority notificationv1alpha1.NotificationPriority
				expected string
			}{
				{notificationv1alpha1.NotificationPriorityCritical, "critical"},
				{notificationv1alpha1.NotificationPriorityHigh, "error"},
				{notificationv1alpha1.NotificationPriorityMedium, "warning"},
				{notificationv1alpha1.NotificationPriorityLow, "info"},
			}
			for _, tc := range cases {
				notification := newTestNotification("sev-test", "ns", notificationv1alpha1.NotificationTypeSimple, tc.priority)
				event := delivery.BuildPagerDutyPayload("key", notification)
				Expect(event.Payload.Severity).To(Equal(tc.expected), "priority %s should map to severity %s", tc.priority, tc.expected)
			}
		})

		It("UT-NOT-060-002: custom_details contains RCA summary, confidence, affected resource", func() {
			notification := newTestNotification("rca-test", "prod", notificationv1alpha1.NotificationTypeApproval, notificationv1alpha1.NotificationPriorityHigh)
			notification.Spec.Context = &notificationv1alpha1.NotificationContext{
				Analysis: &notificationv1alpha1.AnalysisContext{
					RootCause: "OOMKill due to memory leak in request handler",
				},
				Workflow: &notificationv1alpha1.WorkflowContext{
					Confidence: "0.92",
				},
				Target: &notificationv1alpha1.TargetContext{
					TargetResource: "Deployment/web-api",
				},
			}

			event := delivery.BuildPagerDutyPayload("key", notification)

			Expect(event.Payload.CustomDetails).To(HaveKeyWithValue("rca_summary", "OOMKill due to memory leak in request handler"))
			Expect(event.Payload.CustomDetails).To(HaveKeyWithValue("confidence", "0.92"))
			Expect(event.Payload.CustomDetails).To(HaveKeyWithValue("affected_resource", "Deployment/web-api"))
		})

		It("UT-NOT-060-004: custom_details includes kubectl command with RR lineage", func() {
			notification := newTestNotification("cmd-test", "monitoring", notificationv1alpha1.NotificationTypeApproval, notificationv1alpha1.NotificationPriorityCritical)
			notification.Spec.Context = &notificationv1alpha1.NotificationContext{
				Lineage: &notificationv1alpha1.LineageContext{
					RemediationRequest: "oom-recovery-rr",
				},
			}

			event := delivery.BuildPagerDutyPayload("key", notification)

			Expect(event.Payload.CustomDetails).To(HaveKeyWithValue("kubectl_command", "kubectl kubernaut chat rar/oom-recovery-rr -n monitoring"))
		})
	})

	Describe("Deliver Method", func() {

		It("UT-NOT-060-015: HTTP 202 returns success (nil error)", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))
				w.WriteHeader(http.StatusAccepted)
			}))
			defer server.Close()

			svc := delivery.NewPagerDutyDeliveryService(server.URL, "test-routing-key", 5*time.Second)
			notification := newTestNotification("pd-success", "ns1", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityMedium)

			err := svc.Deliver(context.Background(), notification)
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-NOT-060-006: HTTP 500/502/503/429 are retryable errors", func() {
			retryableCodes := []int{500, 502, 503, 429}
			for _, code := range retryableCodes {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(code)
					fmt.Fprint(w, "server error")
				}))

				svc := delivery.NewPagerDutyDeliveryService(server.URL, "key", 5*time.Second)
				notification := newTestNotification("pd-retry", "ns1", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityMedium)

				err := svc.Deliver(context.Background(), notification)
				Expect(err).To(HaveOccurred(), "code %d should return error", code)
				Expect(delivery.IsRetryableError(err)).To(BeTrue(), "code %d should be retryable", code)

				server.Close()
			}
		})

		It("UT-NOT-060-007: HTTP 400/401/403/404 are permanent errors", func() {
			permanentCodes := []int{400, 401, 403, 404}
			for _, code := range permanentCodes {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(code)
					fmt.Fprint(w, "client error")
				}))

				svc := delivery.NewPagerDutyDeliveryService(server.URL, "key", 5*time.Second)
				notification := newTestNotification("pd-perm", "ns1", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityMedium)

				err := svc.Deliver(context.Background(), notification)
				Expect(err).To(HaveOccurred(), "code %d should return error", code)
				Expect(delivery.IsRetryableError(err)).To(BeFalse(), "code %d should be permanent", code)

				server.Close()
			}
		})

		It("UT-NOT-060-008: TLS certificate error is permanent (BR-NOT-058)", func() {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			svc := delivery.NewPagerDutyDeliveryService(server.URL, "key", 5*time.Second)
			notification := newTestNotification("pd-tls", "ns1", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityMedium)

			err := svc.Deliver(context.Background(), notification)
			Expect(err).To(HaveOccurred())
			Expect(delivery.IsRetryableError(err)).To(BeFalse(), "TLS errors should be permanent")
		})
	})

	Describe("Size Guard", func() {

		It("UT-NOT-060-009: Payload >512KB triggers truncation; result <=512KB", func() {
			notification := newTestNotification("big-payload", "ns1", notificationv1alpha1.NotificationTypeEscalation, notificationv1alpha1.NotificationPriorityCritical)
			notification.Spec.Context = &notificationv1alpha1.NotificationContext{
				Analysis: &notificationv1alpha1.AnalysisContext{
					RootCause: strings.Repeat("A", 600*1024),
				},
			}

			event := delivery.BuildPagerDutyPayload("key", notification)
			raw, err := json.Marshal(event)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(raw)).To(BeNumerically(">", 512*1024), "pre-truncation should exceed 512KB")

			truncated := delivery.TruncatePagerDutyPayload(event, 512*1024)
			truncRaw, err := json.Marshal(truncated)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(truncRaw)).To(BeNumerically("<=", 512*1024))
		})

		It("UT-NOT-060-010: Truncated payload has marker and correlation_id", func() {
			notification := newTestNotification("trunc-marker", "ns1", notificationv1alpha1.NotificationTypeEscalation, notificationv1alpha1.NotificationPriorityCritical)
			notification.Spec.Context = &notificationv1alpha1.NotificationContext{
				Analysis: &notificationv1alpha1.AnalysisContext{
					RootCause: strings.Repeat("B", 600*1024),
				},
			}

			event := delivery.BuildPagerDutyPayload("key", notification)
			truncated := delivery.TruncatePagerDutyPayload(event, 512*1024)

			Expect(truncated.Payload.CustomDetails["rca_summary"]).To(ContainSubstring("[truncated -- full details in audit trail]"))
			Expect(truncated.Payload.CustomDetails).To(HaveKeyWithValue("correlation_id", "trunc-marker"))
		})
	})

	Describe("Edge Cases", func() {

		It("UT-NOT-060-011: Cancelled context returns immediate error", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			}))
			defer server.Close()

			svc := delivery.NewPagerDutyDeliveryService(server.URL, "key", 5*time.Second)
			notification := newTestNotification("pd-cancel", "ns1", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityLow)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := svc.Deliver(ctx, notification)
			Expect(err).To(HaveOccurred())
		})

		It("UT-NOT-060-012: Empty routing key returns descriptive error", func() {
			svc := delivery.NewPagerDutyDeliveryService("https://events.pagerduty.com", "", 5*time.Second)
			notification := newTestNotification("pd-no-key", "ns1", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityLow)

			err := svc.Deliver(context.Background(), notification)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("pagerduty routing key is empty"))
		})
	})
})
