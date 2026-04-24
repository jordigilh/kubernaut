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

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/routing"
)

func newTeamsReceiver(name, credRef string) *routing.Receiver {
	return &routing.Receiver{
		Name: name,
		TeamsConfigs: []routing.TeamsConfig{
			{CredentialRef: credRef},
		},
	}
}

var _ = Describe("Microsoft Teams Delivery Channel (#593)", func() {

	Describe("Routing Config Validation (Phase 0 regression guards)", func() {

		It("UT-NOT-593-013: QualifiedChannels returns teams:<receiver> for Teams with CredentialRef", func() {
			receiver := newTeamsReceiver("sre-alerts", "teams-webhook-url")
			channels := receiver.QualifiedChannels()
			Expect(channels).To(ContainElement("teams:sre-alerts"))
		})

		It("UT-NOT-593-014: ValidateCredentialRefs fails when TeamsConfig has empty CredentialRef", func() {
			config := &routing.Config{
				Route: &routing.Route{Receiver: "teams-missing-cred"},
				Receivers: []*routing.Receiver{
					{
						Name: "teams-missing-cred",
						TeamsConfigs: []routing.TeamsConfig{
							{CredentialRef: ""},
						},
					},
				},
			}
			err := config.ValidateCredentialRefs()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("credentialRef"))
			Expect(err.Error()).To(ContainSubstring("teamsConfigs"))
		})
	})

	Describe("Adaptive Card Construction", func() {

		It("UT-NOT-593-001: Workflows format: outer type=message, Adaptive Card content type, version 1.0, schema", func() {
			notification := newTestNotification("teams-format", "prod", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityMedium)

			msg := delivery.BuildTeamsPayload(notification)

			Expect(msg.Type).To(Equal("message"))
			Expect(msg.Attachments).To(HaveLen(1))
			Expect(msg.Attachments[0].ContentType).To(Equal("application/vnd.microsoft.card.adaptive"))

			card := msg.Attachments[0].Content
			Expect(card.Type).To(Equal("AdaptiveCard"))
			Expect(card.Version).To(Equal("1.0"))
			Expect(card.Schema).To(Equal("http://adaptivecards.io/schemas/adaptive-card.json"))

			raw, err := json.Marshal(msg)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(raw)).NotTo(ContainSubstring("MessageCard"))
			Expect(string(raw)).NotTo(ContainSubstring("@type"))
			Expect(string(raw)).NotTo(ContainSubstring("@context"))
		})

		It("UT-NOT-593-002: Card body has TextBlocks with subject, RCA summary, affected resource, confidence", func() {
			notification := newTestNotification("teams-content", "prod", notificationv1alpha1.NotificationTypeApproval, notificationv1alpha1.NotificationPriorityHigh)
			notification.Spec.Context = &notificationv1alpha1.NotificationContext{
				Analysis: &notificationv1alpha1.AnalysisContext{
					RootCause: "Memory leak in connection pool",
				},
				Workflow: &notificationv1alpha1.WorkflowContext{
					Confidence: "0.88",
				},
				Target: &notificationv1alpha1.TargetContext{
					TargetResource: "Deployment/backend-api",
				},
			}

			msg := delivery.BuildTeamsPayload(notification)
			raw, err := json.Marshal(msg)
			Expect(err).NotTo(HaveOccurred())
			body := string(raw)

			Expect(body).To(ContainSubstring(notification.Spec.Subject))
			Expect(body).To(ContainSubstring("Memory leak in connection pool"))
			Expect(body).To(ContainSubstring("Deployment/backend-api"))
			Expect(body).To(ContainSubstring("0.88"))
		})

		It("UT-NOT-593-003: Approval card has kubectl command", func() {
			notification := newTestNotification("teams-approval", "monitoring", notificationv1alpha1.NotificationTypeApproval, notificationv1alpha1.NotificationPriorityCritical)
			notification.Spec.Context = &notificationv1alpha1.NotificationContext{
				Lineage: &notificationv1alpha1.LineageContext{
					RemediationRequest: "fix-oom-rr",
				},
			}

			msg := delivery.BuildTeamsPayload(notification)
			raw, err := json.Marshal(msg)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(raw)).To(ContainSubstring("kubectl kubernaut chat rar/fix-oom-rr -n monitoring"))
		})

		It("UT-NOT-593-004: Status-update card has verification context", func() {
			notification := newTestNotification("teams-status", "prod", notificationv1alpha1.NotificationTypeStatusUpdate, notificationv1alpha1.NotificationPriorityMedium)
			notification.Spec.Context = &notificationv1alpha1.NotificationContext{
				Verification: &notificationv1alpha1.VerificationContext{
					Assessed: true,
					Outcome:  "partial",
					Reason:   "Metrics stabilized but latency elevated",
				},
			}

			msg := delivery.BuildTeamsPayload(notification)
			raw, err := json.Marshal(msg)
			Expect(err).NotTo(HaveOccurred())
			body := string(raw)

			Expect(body).To(ContainSubstring("partial"))
			Expect(body).To(ContainSubstring("Metrics stabilized but latency elevated"))
		})

		It("UT-NOT-593-005: Escalation card has urgency indicators", func() {
			notification := newTestNotification("teams-esc", "prod", notificationv1alpha1.NotificationTypeEscalation, notificationv1alpha1.NotificationPriorityCritical)

			msg := delivery.BuildTeamsPayload(notification)
			raw, err := json.Marshal(msg)
			Expect(err).NotTo(HaveOccurred())
			body := string(raw)

			Expect(body).To(ContainSubstring("CRITICAL"))
		})

		It("UT-NOT-593-015: Completion card has verification outcome", func() {
			notification := newTestNotification("teams-complete", "prod", notificationv1alpha1.NotificationTypeCompletion, notificationv1alpha1.NotificationPriorityLow)
			notification.Spec.Context = &notificationv1alpha1.NotificationContext{
				Verification: &notificationv1alpha1.VerificationContext{
					Assessed: true,
					Outcome:  "passed",
					Reason:   "All health checks green",
				},
			}

			msg := delivery.BuildTeamsPayload(notification)
			raw, err := json.Marshal(msg)
			Expect(err).NotTo(HaveOccurred())
			body := string(raw)

			Expect(body).To(ContainSubstring("passed"))
			Expect(body).To(ContainSubstring("All health checks green"))
		})
	})

	Describe("Deliver Method", func() {

		It("UT-NOT-593-006: HTTP 500/502/503/429 are retryable errors", func() {
			retryableCodes := []int{500, 502, 503, 429}
			for _, code := range retryableCodes {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(code)
					fmt.Fprint(w, "server error")
				}))

				svc := delivery.NewTeamsDeliveryService(server.URL, 5*time.Second)
				notification := newTestNotification("teams-retry", "ns1", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityMedium)

				err := svc.Deliver(context.Background(), notification)
				Expect(err).To(HaveOccurred(), "code %d should return error", code)
				Expect(delivery.IsRetryableError(err)).To(BeTrue(), "code %d should be retryable", code)

				server.Close()
			}
		})

		It("UT-NOT-593-007: HTTP 400/401/403/404 are permanent errors", func() {
			permanentCodes := []int{400, 401, 403, 404}
			for _, code := range permanentCodes {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(code)
					fmt.Fprint(w, "client error")
				}))

				svc := delivery.NewTeamsDeliveryService(server.URL, 5*time.Second)
				notification := newTestNotification("teams-perm", "ns1", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityMedium)

				err := svc.Deliver(context.Background(), notification)
				Expect(err).To(HaveOccurred(), "code %d should return error", code)
				Expect(delivery.IsRetryableError(err)).To(BeFalse(), "code %d should be permanent", code)

				server.Close()
			}
		})

		It("UT-NOT-593-008: TLS certificate error is permanent (BR-NOT-058)", func() {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			svc := delivery.NewTeamsDeliveryService(server.URL, 5*time.Second)
			notification := newTestNotification("teams-tls", "ns1", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityMedium)

			err := svc.Deliver(context.Background(), notification)
			Expect(err).To(HaveOccurred())
			Expect(delivery.IsRetryableError(err)).To(BeFalse(), "TLS errors should be permanent")
		})
	})

	Describe("Size Guard", func() {

		It("UT-NOT-593-009: Payload >28KB triggers truncation; result <=28KB", func() {
			notification := newTestNotification("teams-big", "ns1", notificationv1alpha1.NotificationTypeEscalation, notificationv1alpha1.NotificationPriorityCritical)
			notification.Spec.Body = strings.Repeat("X", 40*1024)

			msg := delivery.BuildTeamsPayload(notification)
			raw, err := json.Marshal(msg)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(raw)).To(BeNumerically(">", 28*1024), "pre-truncation should exceed 28KB")

			truncated := delivery.TruncateTeamsPayload(msg, 28*1024)
			truncRaw, err := json.Marshal(truncated)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(truncRaw)).To(BeNumerically("<=", 28*1024))
		})

		It("UT-NOT-593-010: Truncated body has marker and correlation ID", func() {
			notification := newTestNotification("teams-trunc-marker", "ns1", notificationv1alpha1.NotificationTypeEscalation, notificationv1alpha1.NotificationPriorityCritical)
			notification.Spec.Body = strings.Repeat("Y", 40*1024)

			msg := delivery.BuildTeamsPayload(notification)
			truncated := delivery.TruncateTeamsPayload(msg, 28*1024)

			raw, err := json.Marshal(truncated)
			Expect(err).NotTo(HaveOccurred())
			body := string(raw)
			Expect(body).To(ContainSubstring("[truncated -- full details in audit trail]"))
			Expect(body).To(ContainSubstring("teams-trunc-marker"))
		})
	})

	Describe("Edge Cases", func() {

		It("UT-NOT-593-011: Cancelled context returns immediate error", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			svc := delivery.NewTeamsDeliveryService(server.URL, 5*time.Second)
			notification := newTestNotification("teams-cancel", "ns1", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityLow)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := svc.Deliver(ctx, notification)
			Expect(err).To(HaveOccurred())
		})

		It("UT-NOT-593-012: Empty webhook URL returns descriptive error", func() {
			svc := delivery.NewTeamsDeliveryService("", 5*time.Second)
			notification := newTestNotification("teams-no-url", "ns1", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityLow)

			err := svc.Deliver(context.Background(), notification)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("teams webhook URL is empty"))
		})
	})
})
