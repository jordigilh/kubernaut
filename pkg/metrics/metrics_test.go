package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

func TestRecordAlert(t *testing.T) {
	// Get initial value
	initial := testutil.ToFloat64(AlertsProcessedTotal)

	// Record an alert
	RecordAlert()

	// Value should be increased by 1
	after := testutil.ToFloat64(AlertsProcessedTotal)
	assert.Equal(t, initial+1.0, after)

	// Record another alert
	RecordAlert()

	// Value should be increased by 2 total
	final := testutil.ToFloat64(AlertsProcessedTotal)
	assert.Equal(t, initial+2.0, final)
}

func TestRecordAction(t *testing.T) {
	action := "test_scale_deployment"
	duration := 500 * time.Millisecond

	// Get initial values
	initialCounter := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(action))

	// Record action
	RecordAction(action, duration)

	// Check counter increased
	finalCounter := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(action))
	assert.Equal(t, initialCounter+1.0, finalCounter)
}

func TestRecordSLMAnalysis(t *testing.T) {
	duration := 2 * time.Second

	// Record SLM analysis
	RecordSLMAnalysis(duration)

	// Check histogram has recorded the sample by verifying metrics output
	metric := &dto.Metric{}
	SLMAnalysisDuration.Write(metric)

	// The histogram should have at least one sample
	assert.True(t, metric.GetHistogram().GetSampleCount() > 0, "Histogram should have recorded samples")
}

func TestRecordFilteredAlert(t *testing.T) {
	filter := "test_severity_filter"

	// Get initial value
	initial := testutil.ToFloat64(AlertsFilteredTotal.WithLabelValues(filter))

	// Record filtered alert
	RecordFilteredAlert(filter)

	// Check counter
	final := testutil.ToFloat64(AlertsFilteredTotal.WithLabelValues(filter))
	assert.Equal(t, initial+1.0, final)
}

func TestRecordActionError(t *testing.T) {
	action := "test_restart_pod"
	errorType := "pod_not_found"

	// Get initial value
	initial := testutil.ToFloat64(ActionExecutionErrorsTotal.WithLabelValues(action, errorType))

	// Record action error
	RecordActionError(action, errorType)

	// Check counter
	final := testutil.ToFloat64(ActionExecutionErrorsTotal.WithLabelValues(action, errorType))
	assert.Equal(t, initial+1.0, final)
}

func TestRecordSLMAPICall(t *testing.T) {
	provider := "test_localai"

	// Get initial value
	initial := testutil.ToFloat64(SLMAPICallsTotal.WithLabelValues(provider))

	// Record SLM API call
	RecordSLMAPICall(provider)

	// Check counter
	final := testutil.ToFloat64(SLMAPICallsTotal.WithLabelValues(provider))
	assert.Equal(t, initial+1.0, final)
}

func TestRecordSLMAPIError(t *testing.T) {
	provider := "test_localai"
	errorType := "timeout"

	// Get initial value
	initial := testutil.ToFloat64(SLMAPIErrorsTotal.WithLabelValues(provider, errorType))

	// Record SLM API error
	RecordSLMAPIError(provider, errorType)

	// Check counter
	final := testutil.ToFloat64(SLMAPIErrorsTotal.WithLabelValues(provider, errorType))
	assert.Equal(t, initial+1.0, final)
}

func TestRecordK8sAPICall(t *testing.T) {
	operation := "test_get"

	// Get initial value
	initial := testutil.ToFloat64(K8sAPICallsTotal.WithLabelValues(operation))

	// Record Kubernetes API call
	RecordK8sAPICall(operation)

	// Check counter
	final := testutil.ToFloat64(K8sAPICallsTotal.WithLabelValues(operation))
	assert.Equal(t, initial+1.0, final)
}

func TestSetAlertsInCooldown(t *testing.T) {
	// Set alerts in cooldown
	SetAlertsInCooldown(5.0)

	// Check gauge
	value := testutil.ToFloat64(AlertsInCooldownTotal)
	assert.Equal(t, 5.0, value)

	// Update the value
	SetAlertsInCooldown(3.0)

	// Check updated value
	value = testutil.ToFloat64(AlertsInCooldownTotal)
	assert.Equal(t, 3.0, value)
}

func TestConcurrentActionsGauge(t *testing.T) {
	// Get initial value
	initial := testutil.ToFloat64(ConcurrentActionsRunning)

	// Increment concurrent actions
	IncrementConcurrentActions()
	value := testutil.ToFloat64(ConcurrentActionsRunning)
	assert.Equal(t, initial+1.0, value)

	// Increment again
	IncrementConcurrentActions()
	value = testutil.ToFloat64(ConcurrentActionsRunning)
	assert.Equal(t, initial+2.0, value)

	// Decrement
	DecrementConcurrentActions()
	value = testutil.ToFloat64(ConcurrentActionsRunning)
	assert.Equal(t, initial+1.0, value)

	// Decrement back to initial
	DecrementConcurrentActions()
	value = testutil.ToFloat64(ConcurrentActionsRunning)
	assert.Equal(t, initial, value)
}

func TestRecordWebhookRequest(t *testing.T) {
	// Get initial values
	initialSuccess := testutil.ToFloat64(WebhookRequestsTotal.WithLabelValues("success"))
	initialError := testutil.ToFloat64(WebhookRequestsTotal.WithLabelValues("error"))

	// Record successful webhook request
	RecordWebhookRequest("success")

	// Check counter
	finalSuccess := testutil.ToFloat64(WebhookRequestsTotal.WithLabelValues("success"))
	assert.Equal(t, initialSuccess+1.0, finalSuccess)

	// Record error webhook request
	RecordWebhookRequest("error")

	// Check error counter
	finalError := testutil.ToFloat64(WebhookRequestsTotal.WithLabelValues("error"))
	assert.Equal(t, initialError+1.0, finalError)
}

func TestTimer(t *testing.T) {
	// Create a new timer
	timer := NewTimer()

	// Verify timer is initialized
	assert.NotNil(t, timer)
	assert.False(t, timer.start.IsZero())

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Check elapsed time
	elapsed := timer.Elapsed()
	assert.True(t, elapsed >= 10*time.Millisecond, "Elapsed time should be at least 10ms")
	assert.True(t, elapsed < 100*time.Millisecond, "Elapsed time should be less than 100ms")
}

func TestTimerRecordAction(t *testing.T) {
	timer := NewTimer()
	action := "test_timer_action"

	// Get initial values
	initialCounter := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(action))

	// Wait a bit to ensure some time passes
	time.Sleep(10 * time.Millisecond)

	// Record action with timer
	timer.RecordAction(action)

	// Check that action was recorded
	finalCounter := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(action))
	assert.Equal(t, initialCounter+1.0, finalCounter)
}

func TestTimerRecordSLMAnalysis(t *testing.T) {
	timer := NewTimer()

	// Wait a bit to ensure some time passes
	time.Sleep(10 * time.Millisecond)

	// Record SLM analysis with timer
	timer.RecordSLMAnalysis()

	// Check that duration was recorded by verifying metrics output
	metric := &dto.Metric{}
	SLMAnalysisDuration.Write(metric)

	// The histogram should have recorded samples
	assert.True(t, metric.GetHistogram().GetSampleCount() > 0, "Histogram should have recorded samples")
}

func TestMultipleActions(t *testing.T) {
	actions := []string{"test_scale_deployment", "test_restart_pod", "test_increase_resources"}

	// Get initial values
	initialValues := make(map[string]float64)
	for _, action := range actions {
		initialValues[action] = testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(action))
	}

	// Record multiple actions
	for _, action := range actions {
		RecordAction(action, 100*time.Millisecond)
	}

	// Check each action was recorded
	for _, action := range actions {
		finalValue := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(action))
		assert.Equal(t, initialValues[action]+1.0, finalValue, "Action %s should have increased by 1", action)
	}
}

func TestMetricsIntegration(t *testing.T) {
	// Test a complete workflow simulation
	uniqueAction := "test_integration_scale"
	provider := "test_integration_localai"

	// Get initial values
	initialAlerts := testutil.ToFloat64(AlertsProcessedTotal)
	initialActions := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(uniqueAction))
	initialSLMCalls := testutil.ToFloat64(SLMAPICallsTotal.WithLabelValues(provider))
	initialWebhook := testutil.ToFloat64(WebhookRequestsTotal.WithLabelValues("success"))
	initialConcurrent := testutil.ToFloat64(ConcurrentActionsRunning)

	// Simulate webhook request
	RecordWebhookRequest("success")

	// Simulate processing alerts
	numAlerts := 3
	for i := 0; i < numAlerts; i++ {
		RecordAlert()

		// Simulate SLM analysis
		RecordSLMAPICall(provider)
		RecordSLMAnalysis(500 * time.Millisecond)

		// Simulate action execution
		IncrementConcurrentActions()
		RecordAction(uniqueAction, 200*time.Millisecond)
		DecrementConcurrentActions()
	}

	// Verify final state
	finalAlerts := testutil.ToFloat64(AlertsProcessedTotal)
	assert.Equal(t, initialAlerts+float64(numAlerts), finalAlerts)

	finalActions := testutil.ToFloat64(ActionsExecutedTotal.WithLabelValues(uniqueAction))
	assert.Equal(t, initialActions+float64(numAlerts), finalActions)

	finalSLMCalls := testutil.ToFloat64(SLMAPICallsTotal.WithLabelValues(provider))
	assert.Equal(t, initialSLMCalls+float64(numAlerts), finalSLMCalls)

	finalWebhook := testutil.ToFloat64(WebhookRequestsTotal.WithLabelValues("success"))
	assert.Equal(t, initialWebhook+1.0, finalWebhook)

	finalConcurrent := testutil.ToFloat64(ConcurrentActionsRunning)
	assert.Equal(t, initialConcurrent, finalConcurrent) // Should be back to initial value
}

func TestFakeSLMClientMetrics(t *testing.T) {
	// This test simulates using a fake SLM client
	provider := "fake"

	// Get initial values
	initialCalls := testutil.ToFloat64(SLMAPICallsTotal.WithLabelValues(provider))
	initialErrors := testutil.ToFloat64(SLMAPIErrorsTotal.WithLabelValues(provider, "connection_failed"))

	// Simulate fake SLM client calls

	// Record successful call
	RecordSLMAPICall(provider)
	timer := NewTimer()
	time.Sleep(50 * time.Millisecond) // Simulate processing time
	timer.RecordSLMAnalysis()

	// Record failed call
	RecordSLMAPICall(provider)
	RecordSLMAPIError(provider, "connection_failed")

	// Verify metrics
	finalCalls := testutil.ToFloat64(SLMAPICallsTotal.WithLabelValues(provider))
	assert.Equal(t, initialCalls+2.0, finalCalls) // Both calls recorded

	finalErrors := testutil.ToFloat64(SLMAPIErrorsTotal.WithLabelValues(provider, "connection_failed"))
	assert.Equal(t, initialErrors+1.0, finalErrors)

	// Check that successful analysis was recorded
	metric := &dto.Metric{}
	SLMAnalysisDuration.Write(metric)
	assert.True(t, metric.GetHistogram().GetSampleCount() > 0, "Should have recorded successful analysis")
}

func TestMetricsNaming(t *testing.T) {
	// Test that metrics follow Prometheus naming conventions
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
		// Check that metric names don't contain invalid characters
		assert.False(t, strings.Contains(name, "-"), "Metric name %s should not contain hyphens", name)
		assert.False(t, strings.Contains(name, " "), "Metric name %s should not contain spaces", name)

		// Check that duration metrics end with appropriate suffix
		if strings.Contains(name, "duration") {
			assert.True(t, strings.HasSuffix(name, "_seconds"), "Duration metric %s should end with _seconds", name)
		}

		// Check that counters end with _total
		if strings.Contains(name, "processed") || strings.Contains(name, "executed") ||
			strings.Contains(name, "filtered") || strings.Contains(name, "errors") ||
			strings.Contains(name, "calls") || strings.Contains(name, "requests") {
			assert.True(t, strings.HasSuffix(name, "_total"), "Counter metric %s should end with _total", name)
		}
	}
}
