package notification_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
)

func TestSlackDelivery(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Slack Delivery Suite")
}

var _ = Describe("BR-NOT-053: Slack Delivery Service", func() {
	var (
		ctx     context.Context
		service *delivery.SlackDeliveryService
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ⭐ TABLE-DRIVEN: Webhook response handling
	DescribeTable("should handle webhook responses correctly",
		func(statusCode int, expectError bool, expectRetry bool) {
			// Create mock Slack webhook server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode)
			}))
			defer server.Close()

			service = delivery.NewSlackDeliveryService(server.URL)

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-notification",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  "Test",
					Body:     "Test message",
					Priority: notificationv1alpha1.NotificationPriorityHigh,
				},
			}

			err := service.Deliver(ctx, notification)

			if expectError {
				Expect(err).To(HaveOccurred())
				if expectRetry {
					Expect(delivery.IsRetryableError(err)).To(BeTrue(), "Expected retryable error for status %d", statusCode)
				} else {
					Expect(delivery.IsRetryableError(err)).To(BeFalse(), "Expected permanent error for status %d", statusCode)
				}
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		},
		Entry("200 OK - success", http.StatusOK, false, false),
		Entry("204 No Content - success", http.StatusNoContent, false, false),
		Entry("503 Service Unavailable - retryable", http.StatusServiceUnavailable, true, true),
		Entry("500 Internal Server Error - retryable", http.StatusInternalServerError, true, true),
		Entry("502 Bad Gateway - retryable", http.StatusBadGateway, true, true),
		Entry("401 Unauthorized - permanent failure", http.StatusUnauthorized, true, false),
		Entry("404 Not Found - permanent failure", http.StatusNotFound, true, false),
		Entry("400 Bad Request - permanent failure", http.StatusBadRequest, true, false),
	)

	Context("when formatting Slack message", func() {
		It("should create valid Block Kit JSON", func() {
			// Test Block Kit formatting
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-notification",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  "Test Subject",
					Body:     "Test Body",
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Type:     notificationv1alpha1.NotificationTypeEscalation,
				},
			}

			payload := delivery.FormatSlackPayload(notification)
			
			// Validate Block Kit structure
			Expect(payload).To(HaveKey("blocks"))
			Expect(payload["blocks"]).To(BeAssignableToTypeOf([]interface{}{}))
			
			blocks := payload["blocks"].([]interface{})
			Expect(blocks).To(HaveLen(3), "Expected header, section, and context blocks")
			
			// Validate header block
			headerBlock := blocks[0].(map[string]interface{})
			Expect(headerBlock["type"]).To(Equal("header"))
			Expect(headerBlock).To(HaveKey("text"))
			
			// Validate section block (message body)
			sectionBlock := blocks[1].(map[string]interface{})
			Expect(sectionBlock["type"]).To(Equal("section"))
			Expect(sectionBlock).To(HaveKey("text"))
			
			// Validate context block (metadata)
			contextBlock := blocks[2].(map[string]interface{})
			Expect(contextBlock["type"]).To(Equal("context"))
			Expect(contextBlock).To(HaveKey("elements"))
		})

		It("should include priority emoji in subject", func() {
			notifications := []struct {
				priority      notificationv1alpha1.NotificationPriority
				expectedEmoji string
			}{
				{notificationv1alpha1.NotificationPriorityCritical, "🚨"},
				{notificationv1alpha1.NotificationPriorityHigh, "⚠️"},
				{notificationv1alpha1.NotificationPriorityMedium, "ℹ️"},
				{notificationv1alpha1.NotificationPriorityLow, "💬"},
			}

			for _, n := range notifications {
				notification := &notificationv1alpha1.NotificationRequest{
					Spec: notificationv1alpha1.NotificationRequestSpec{
						Subject:  "Test",
						Body:     "Body",
						Priority: n.priority,
					},
				}

				payload := delivery.FormatSlackPayload(notification)
				blocks := payload["blocks"].([]interface{})
				headerBlock := blocks[0].(map[string]interface{})
				headerText := headerBlock["text"].(map[string]interface{})
				text := headerText["text"].(string)

				Expect(text).To(ContainSubstring(n.expectedEmoji), 
					"Expected emoji %s for priority %s", n.expectedEmoji, n.priority)
			}
		})
	})

	Context("when webhook URL is invalid", func() {
		It("should return retryable error for network failures", func() {
			service = delivery.NewSlackDeliveryService("http://invalid-url-that-does-not-exist:9999")

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-notification",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test",
					Body:    "Test message",
				},
			}

			err := service.Deliver(ctx, notification)

			Expect(err).To(HaveOccurred())
			Expect(delivery.IsRetryableError(err)).To(BeTrue(), "Network errors should be retryable")
		})
	})

	Context("when context is cancelled", func() {
		It("should respect context cancellation", func() {
			// Create a server that delays response
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Intentionally delay to allow context cancellation
				<-r.Context().Done()
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			service = delivery.NewSlackDeliveryService(server.URL)

			// Create cancellable context
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-notification",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject: "Test",
					Body:    "Test message",
				},
			}

			err := service.Deliver(ctx, notification)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})
	})
})

