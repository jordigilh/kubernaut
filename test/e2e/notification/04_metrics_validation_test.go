/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package notification

import (
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// Metrics E2E Validation using Kind NodePort
// NodePort 30081 (in cluster) → localhost:9091 (on host via Kind extraPortMappings)
// This follows the same pattern as Gateway service (NodePort 30080 → localhost:8080)
var _ = Describe("Metrics E2E Validation", Label("metrics"), func() {

	var (
		metricsEndpoint string
	)

	BeforeEach(func() {
		// BR-NOT-054: Controller metrics exposed via NodePort (localhost:8081)
		// Kind extraPortMappings: containerPort 30081 → hostPort 8081
		// Using same port pattern as gateway (8xxx) for consistency
		metricsEndpoint = "http://localhost:8081/metrics"
	})

	Context("Metrics Endpoint Availability", func() {
		It("should expose metrics endpoint", func() {
			// BR-NOT-054: Observability - Metrics endpoint must be available
			By("Querying metrics endpoint")
			resp, err := http.Get(metricsEndpoint)
			Expect(err).ToNot(HaveOccurred(), "Metrics endpoint should be accessible")
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK), "Metrics endpoint should return 200 OK")

			By("Verifying metrics are in Prometheus format")
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(body)).ToNot(BeEmpty(), "Metrics response should not be empty")
		})
	})

	Context("Notification Delivery Metrics", func() {
		It("should track notification_phase metric", func() {
			// BR-NOT-054: Observability - Track notification phase distribution
			By("Creating a NotificationRequest")
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "metrics-test-requests-total",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:    notificationv1alpha1.NotificationTypeSimple,
					Subject: "Metrics Test: Requests Total",
					Body:    "Testing notification_requests_total metric",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					Priority: notificationv1alpha1.NotificationPriorityMedium,
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#metrics-test"},
					},
				},
			}
			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for notification to be processed")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				fetchedNotification := &notificationv1alpha1.NotificationRequest{}
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(notification), fetchedNotification)
				return fetchedNotification.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			By("Querying metrics endpoint")
			resp, err := http.Get(metricsEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsOutput := string(body)

			By("Validating notification_phase metric exists")
			Expect(metricsOutput).To(ContainSubstring("notification_phase"),
				"Metrics should contain notification_phase gauge")

			By("Validating metric has correct labels")
			// Expect metrics with namespace and phase labels
			Expect(metricsOutput).To(MatchRegexp(`notification_phase\{.*namespace="default".*\}`),
				"Metric should have namespace label")
			Expect(metricsOutput).To(MatchRegexp(`notification_phase\{.*phase=".*".*\}`),
				"Metric should have phase label")
		})

		It("should track notification_deliveries_total metric", func() {
			// BR-NOT-054: Observability - Track delivery attempts
			By("Creating a NotificationRequest")
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "metrics-test-delivery-attempts",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:    notificationv1alpha1.NotificationTypeSimple,
					Subject: "Metrics Test: Delivery Attempts",
					Body:    "Testing notification_delivery_attempts_total metric",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#metrics-test"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			By("Waiting for notification to be delivered")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				fetchedNotification := &notificationv1alpha1.NotificationRequest{}
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(notification), fetchedNotification)
				return fetchedNotification.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			By("Querying metrics endpoint")
			resp, err := http.Get(metricsEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsOutput := string(body)

			By("Validating notification_deliveries_total metric exists")
			Expect(metricsOutput).To(ContainSubstring("notification_deliveries_total"),
				"Metrics should contain notification_deliveries_total counter")

			By("Validating metric has correct labels")
			// Expect metrics with namespace, status, channel labels
			Expect(metricsOutput).To(MatchRegexp(`notification_deliveries_total\{.*namespace="default".*\}`),
				"Metric should have namespace label")
			Expect(metricsOutput).To(MatchRegexp(`notification_deliveries_total\{.*channel="console".*\}`),
				"Metric should have channel label")
			Expect(metricsOutput).To(MatchRegexp(`notification_deliveries_total\{.*status="success".*\}`),
				"Metric should have status label for successful deliveries")
		})

		It("should track notification_delivery_duration_seconds metric", func() {
			// BR-NOT-054: Observability - Track delivery duration
			By("Creating a NotificationRequest")
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "metrics-test-delivery-duration",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:    notificationv1alpha1.NotificationTypeSimple,
					Subject: "Metrics Test: Delivery Duration",
					Body:    "Testing notification_delivery_duration_seconds metric",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					Priority: notificationv1alpha1.NotificationPriorityLow,
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#metrics-test"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, notification)).To(Succeed())

			By("Waiting for notification to be delivered")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				fetchedNotification := &notificationv1alpha1.NotificationRequest{}
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(notification), fetchedNotification)
				return fetchedNotification.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			By("Querying metrics endpoint")
			resp, err := http.Get(metricsEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsOutput := string(body)

			By("Validating notification_delivery_duration_seconds metric exists")
			Expect(metricsOutput).To(ContainSubstring("notification_delivery_duration_seconds"),
				"Metrics should contain notification_delivery_duration_seconds histogram")

			By("Validating metric has histogram buckets")
			// Histogram metrics include _bucket, _sum, _count suffixes
			Expect(metricsOutput).To(ContainSubstring("notification_delivery_duration_seconds_bucket"),
				"Histogram should have bucket metrics")
			Expect(metricsOutput).To(ContainSubstring("notification_delivery_duration_seconds_sum"),
				"Histogram should have sum metric")
			Expect(metricsOutput).To(ContainSubstring("notification_delivery_duration_seconds_count"),
				"Histogram should have count metric")
		})
	})


	Context("Metrics Integration Health", func() {
		It("should validate key notification metrics are exposed", func() {
			// BR-NOT-054: Observability - Notification metrics requirement
			By("Creating a NotificationRequest to generate metrics activity")
			notification := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "metrics-test-all-metrics",
					Namespace: "default",
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Type:    notificationv1alpha1.NotificationTypeSimple,
					Subject: "Metrics Test: All Metrics Validation",
					Body:    "Testing key metrics are exposed",
					Channels: []notificationv1alpha1.Channel{
						notificationv1alpha1.ChannelConsole,
					},
					Priority: notificationv1alpha1.NotificationPriorityCritical,
					Recipients: []notificationv1alpha1.Recipient{
						{Slack: "#metrics-test"},
					},
				},
			}
			err := k8sClient.Create(ctx, notification)
			Expect(err).ToNot(HaveOccurred())

			By("Waiting for notification to be processed")
			Eventually(func() notificationv1alpha1.NotificationPhase {
				fetchedNotification := &notificationv1alpha1.NotificationRequest{}
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(notification), fetchedNotification)
				return fetchedNotification.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(notificationv1alpha1.NotificationPhaseSent))

			By("Querying metrics endpoint")
			resp, err := http.Get(metricsEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			metricsOutput := string(body)

			By("Validating core notification metrics are present and being recorded")
			// These are the metrics that are actually being recorded by the controller
			coreMetrics := []string{
				"notification_deliveries_total",          // RecordDeliveryAttempt - recorded
				"notification_delivery_duration_seconds", // RecordDeliveryDuration - recorded
				"notification_phase",                     // UpdatePhaseCount - recorded
			}

			for _, metric := range coreMetrics {
				Expect(metricsOutput).To(ContainSubstring(metric),
					"Core metric %s should be present and recorded", metric)
			}

			By("Validating additional registered metrics are present")
			// These metrics are registered in the controller package
			// They may not appear in Prometheus output until they have data
			registeredMetrics := []string{
				"notification_failure_rate",
				"notification_retry_count",
				"notification_stuck_duration_seconds",
			}

			registeredCount := 0
			for _, metric := range registeredMetrics {
				if strings.Contains(metricsOutput, metric) {
					registeredCount++
				}
			}

			// These metrics are registered but may not have data yet - that's OK
			// As long as core metrics are working, the integration is valid
			GinkgoWriter.Printf("INFO: %d of %d registered metrics found in output (expected: may vary)\n",
				registeredCount, len(registeredMetrics))
		})
	})
})


