package metrics

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

var _ = Describe("Metrics", func() {
	Describe("RecordAlert", func() {
		It("should increment alerts processed counter", func() {
			initial := testutil.ToFloat64(AlertsProcessedTotal)

			RecordAlert()

			after := testutil.ToFloat64(AlertsProcessedTotal)
			Expect(after).To(Equal(initial + 1.0))

			RecordAlert()

			final := testutil.ToFloat64(AlertsProcessedTotal)
			Expect(final).To(Equal(initial + 2.0))
		})
	})

	Describe("RecordAction", func() {
		It("should increment actions executed counter", func() {
			action := "test_scale_deployment"
			duration := 500 * time.Millisecond

			initialCounter := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(action))

			RecordAction(action, duration)

			finalCounter := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(action))
			Expect(finalCounter).To(Equal(initialCounter + 1.0))
		})
	})

	Describe("RecordSLMAnalysis", func() {
		It("should record duration in histogram", func() {
			duration := 2 * time.Second

			RecordSLMAnalysis(duration)

			metric := &dto.Metric{}
			err := SLMAnalysisDuration.Write(metric)
			Expect(err).NotTo(HaveOccurred())

			Expect(metric.GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		})
	})

	Describe("RecordFilteredAlert", func() {
		It("should increment filtered alerts counter", func() {
			filter := "test_severity_filter"

			initial := testutil.ToFloat64(AlertsFilteredTotal.WithLabelValues(filter))

			RecordFilteredAlert(filter)

			final := testutil.ToFloat64(AlertsFilteredTotal.WithLabelValues(filter))
			Expect(final).To(Equal(initial + 1.0))
		})
	})

	Describe("RecordActionError", func() {
		It("should increment action error counter", func() {
			action := "test_restart_pod"
			errorType := "pod_not_found"

			initial := testutil.ToFloat64(ActionExecutionErrorsTotal.WithLabelValues(action, errorType))

			RecordActionError(action, errorType)

			final := testutil.ToFloat64(ActionExecutionErrorsTotal.WithLabelValues(action, errorType))
			Expect(final).To(Equal(initial + 1.0))
		})
	})

	Describe("RecordSLMAPICall", func() {
		It("should increment SLM API calls counter", func() {
			provider := "test_localai"

			initial := testutil.ToFloat64(SLMAPICallsTotal.WithLabelValues(provider))

			RecordSLMAPICall(provider)

			final := testutil.ToFloat64(SLMAPICallsTotal.WithLabelValues(provider))
			Expect(final).To(Equal(initial + 1.0))
		})
	})

	Describe("RecordSLMAPIError", func() {
		It("should increment SLM API errors counter", func() {
			provider := "test_localai"
			errorType := "timeout"

			initial := testutil.ToFloat64(SLMAPIErrorsTotal.WithLabelValues(provider, errorType))

			RecordSLMAPIError(provider, errorType)

			final := testutil.ToFloat64(SLMAPIErrorsTotal.WithLabelValues(provider, errorType))
			Expect(final).To(Equal(initial + 1.0))
		})
	})

	Describe("RecordK8sAPICall", func() {
		It("should increment Kubernetes API calls counter", func() {
			operation := "test_get"

			initial := testutil.ToFloat64(K8sAPICallsTotal.WithLabelValues(operation))

			RecordK8sAPICall(operation)

			final := testutil.ToFloat64(K8sAPICallsTotal.WithLabelValues(operation))
			Expect(final).To(Equal(initial + 1.0))
		})
	})

	Describe("SetAlertsInCooldown", func() {
		It("should set alerts in cooldown gauge value", func() {
			SetAlertsInCooldown(5.0)

			value := testutil.ToFloat64(AlertsInCooldownTotal)
			Expect(value).To(Equal(5.0))

			SetAlertsInCooldown(3.0)

			value = testutil.ToFloat64(AlertsInCooldownTotal)
			Expect(value).To(Equal(3.0))
		})
	})

	Describe("ConcurrentActionsGauge", func() {
		It("should track concurrent actions correctly", func() {
			initial := testutil.ToFloat64(ConcurrentActionsRunning)

			IncrementConcurrentActions()
			value := testutil.ToFloat64(ConcurrentActionsRunning)
			Expect(value).To(Equal(initial + 1.0))

			IncrementConcurrentActions()
			value = testutil.ToFloat64(ConcurrentActionsRunning)
			Expect(value).To(Equal(initial + 2.0))

			DecrementConcurrentActions()
			value = testutil.ToFloat64(ConcurrentActionsRunning)
			Expect(value).To(Equal(initial + 1.0))

			DecrementConcurrentActions()
			value = testutil.ToFloat64(ConcurrentActionsRunning)
			Expect(value).To(Equal(initial))
		})
	})

	Describe("RecordWebhookRequest", func() {
		It("should increment webhook requests counter", func() {
			initialSuccess := testutil.ToFloat64(WebhookRequestsTotal.WithLabelValues("success"))
			initialError := testutil.ToFloat64(WebhookRequestsTotal.WithLabelValues("error"))

			RecordWebhookRequest("success")

			finalSuccess := testutil.ToFloat64(WebhookRequestsTotal.WithLabelValues("success"))
			Expect(finalSuccess).To(Equal(initialSuccess + 1.0))

			RecordWebhookRequest("error")

			finalError := testutil.ToFloat64(WebhookRequestsTotal.WithLabelValues("error"))
			Expect(finalError).To(Equal(initialError + 1.0))
		})
	})

	Describe("Timer", func() {
		It("should create and track elapsed time correctly", func() {
			timer := NewTimer()

			Expect(timer).ToNot(BeNil())
			Expect(timer.start.IsZero()).To(BeFalse())

			time.Sleep(10 * time.Millisecond)

			elapsed := timer.Elapsed()
			Expect(elapsed).To(BeNumerically(">=", 10*time.Millisecond))
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond))
		})

		It("should record action with timer", func() {
			timer := NewTimer()
			action := "test_timer_action"

			initialCounter := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(action))

			time.Sleep(10 * time.Millisecond)

			timer.RecordAction(action)

			finalCounter := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(action))
			Expect(finalCounter).To(Equal(initialCounter + 1.0))
		})

		It("should record SLM analysis with timer", func() {
			timer := NewTimer()

			time.Sleep(10 * time.Millisecond)

			timer.RecordSLMAnalysis()

			metric := &dto.Metric{}
			err := SLMAnalysisDuration.Write(metric)
			Expect(err).NotTo(HaveOccurred())

			Expect(metric.GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		})
	})

	Describe("MultipleActions", func() {
		It("should record multiple actions correctly", func() {
			actions := []string{"test_scale_deployment", "test_restart_pod", "test_increase_resources"}

			initialValues := make(map[string]float64)
			for _, action := range actions {
				initialValues[action] = testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(action))
			}

			for _, action := range actions {
				RecordAction(action, 100*time.Millisecond)
			}

			for _, action := range actions {
				finalValue := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(action))
				Expect(finalValue).To(Equal(initialValues[action]+1.0), "Action %s should have increased by 1", action)
			}
		})
	})

	Describe("Metrics Integration", func() {
		It("should handle complete workflow simulation correctly", func() {
			uniqueAction := "test_integration_scale"
			provider := "test_integration_localai"

			initialAlerts := testutil.ToFloat64(AlertsProcessedTotal)
			initialActions := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(uniqueAction))
			initialSLMCalls := testutil.ToFloat64(SLMAPICallsTotal.WithLabelValues(provider))
			initialWebhook := testutil.ToFloat64(WebhookRequestsTotal.WithLabelValues("success"))
			initialConcurrent := testutil.ToFloat64(ConcurrentActionsRunning)

			RecordWebhookRequest("success")

			numAlerts := 3
			for i := 0; i < numAlerts; i++ {
				RecordAlert()
				RecordSLMAPICall(provider)
				RecordSLMAnalysis(500 * time.Millisecond)
				IncrementConcurrentActions()
				RecordAction(uniqueAction, 200*time.Millisecond)
				DecrementConcurrentActions()
			}

			finalAlerts := testutil.ToFloat64(AlertsProcessedTotal)
			Expect(finalAlerts).To(Equal(initialAlerts + float64(numAlerts)))

			finalActions := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(uniqueAction))
			Expect(finalActions).To(Equal(initialActions + float64(numAlerts)))

			finalSLMCalls := testutil.ToFloat64(SLMAPICallsTotal.WithLabelValues(provider))
			Expect(finalSLMCalls).To(Equal(initialSLMCalls + float64(numAlerts)))

			finalWebhook := testutil.ToFloat64(WebhookRequestsTotal.WithLabelValues("success"))
			Expect(finalWebhook).To(Equal(initialWebhook + 1.0))

			finalConcurrent := testutil.ToFloat64(ConcurrentActionsRunning)
			Expect(finalConcurrent).To(Equal(initialConcurrent))
		})
	})

	Describe("Fake SLM Client Metrics", func() {
		It("should track fake SLM client interactions correctly", func() {
			provider := "fake"

			initialCalls := testutil.ToFloat64(SLMAPICallsTotal.WithLabelValues(provider))
			initialErrors := testutil.ToFloat64(SLMAPIErrorsTotal.WithLabelValues(provider, "connection_failed"))

			RecordSLMAPICall(provider)
			timer := NewTimer()
			time.Sleep(50 * time.Millisecond)
			timer.RecordSLMAnalysis()

			RecordSLMAPICall(provider)
			RecordSLMAPIError(provider, "connection_failed")

			finalCalls := testutil.ToFloat64(SLMAPICallsTotal.WithLabelValues(provider))
			Expect(finalCalls).To(Equal(initialCalls + 2.0))

			finalErrors := testutil.ToFloat64(SLMAPIErrorsTotal.WithLabelValues(provider, "connection_failed"))
			Expect(finalErrors).To(Equal(initialErrors + 1.0))

			metric := &dto.Metric{}
			err := SLMAnalysisDuration.Write(metric)
			Expect(err).NotTo(HaveOccurred())
			Expect(metric.GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		})
	})

	Describe("Metrics Naming", func() {
		It("should follow Prometheus naming conventions", func() {
			metricNames := []string{
				"alerts_processed_total",
				"actions_executed_total",
				"action_processing_duration_seconds",
				"slm_analysis_duration_seconds",
				"alerts_filtered_total",
				"action_execution_errors_total",
				"slm_api_calls_total",
				"slm_api_errors_total",
				"k8s_api_calls_total",
				"alerts_in_cooldown_total",
				"concurrent_actions_running",
				"webhook_requests_total",
			}

			for _, name := range metricNames {
				Expect(strings.Contains(name, "-")).To(BeFalse(), "Metric name %s should not contain hyphens", name)
				Expect(strings.Contains(name, " ")).To(BeFalse(), "Metric name %s should not contain spaces", name)

				if strings.Contains(name, "duration") {
					Expect(strings.HasSuffix(name, "_seconds")).To(BeTrue(), "Duration metric %s should end with _seconds", name)
				}

				if strings.Contains(name, "processed") || strings.Contains(name, "executed") ||
					strings.Contains(name, "filtered") || strings.Contains(name, "errors") ||
					strings.Contains(name, "calls") || strings.Contains(name, "requests") {
					Expect(strings.HasSuffix(name, "_total")).To(BeTrue(), "Counter metric %s should end with _total", name)
				}
			}
		})
	})
})
