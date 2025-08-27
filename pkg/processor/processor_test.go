package processor

import (
	"context"
	"fmt"
	"testing"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/executor"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// FakeSLMClient implements the slm.Client interface for testing
type FakeSLMClient struct {
	recommendation *types.ActionRecommendation
	err            error
	healthy        bool
	callCount      int
}

func NewFakeSLMClient(healthy bool) *FakeSLMClient {
	return &FakeSLMClient{
		healthy: healthy,
		recommendation: &types.ActionRecommendation{
			Action:     "notify_only",
			Confidence: 0.5,
			Reasoning:  "Fake SLM response for testing",
			Parameters: map[string]interface{}{},
		},
	}
}

func (f *FakeSLMClient) AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
	f.callCount++
	if f.err != nil {
		return nil, f.err
	}
	return f.recommendation, nil
}

func (f *FakeSLMClient) IsHealthy() bool {
	return f.healthy
}

func (f *FakeSLMClient) SetError(err error) {
	f.err = err
}

func (f *FakeSLMClient) SetRecommendation(rec *types.ActionRecommendation) {
	f.recommendation = rec
}

func (f *FakeSLMClient) GetCallCount() int {
	return f.callCount
}

// FakeExecutor implements the executor.Executor interface for testing
type FakeExecutor struct {
	err       error
	callCount int
	healthy   bool
}

func NewFakeExecutor(healthy bool) *FakeExecutor {
	return &FakeExecutor{
		healthy: healthy,
	}
}

func (f *FakeExecutor) Execute(ctx context.Context, action *types.ActionRecommendation, alert types.Alert) error {
	f.callCount++
	return f.err
}

func (f *FakeExecutor) IsHealthy() bool {
	return f.healthy
}

func (f *FakeExecutor) SetError(err error) {
	f.err = err
}

func (f *FakeExecutor) GetCallCount() int {
	return f.callCount
}

func (f *FakeExecutor) GetActionRegistry() *executor.ActionRegistry {
	return executor.NewActionRegistry()
}

func TestNewProcessor(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)
	filters := []config.FilterConfig{}

	processor := NewProcessor(slmClient, executor, filters, logger)

	assert.NotNil(t, processor)
	assert.Implements(t, (*Processor)(nil), processor)
}

func TestProcessor_ProcessAlert_Success(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)

	recommendation := &types.ActionRecommendation{
		Action:     "scale_deployment",
		Confidence: 0.8,
		Reasoning:  "High CPU usage detected",
		Parameters: map[string]interface{}{
			"replicas": 5,
		},
	}
	slmClient.SetRecommendation(recommendation)

	processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, logger)
	ctx := context.Background()

	alert := types.Alert{
		Name:      "HighCPUUsage",
		Status:    "firing",
		Severity:  "warning",
		Namespace: "production",
	}

	err := processor.ProcessAlert(ctx, alert)

	assert.NoError(t, err)
	assert.Equal(t, 1, slmClient.GetCallCount())
	assert.Equal(t, 1, executor.GetCallCount())
}

func TestProcessor_ProcessAlert_NonFiringStatus(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)

	processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, logger)
	ctx := context.Background()

	alert := types.Alert{
		Name:      "ResolvedAlert",
		Status:    "resolved",
		Severity:  "warning",
		Namespace: "production",
	}

	err := processor.ProcessAlert(ctx, alert)

	assert.NoError(t, err)
	// Should not call SLM or executor for non-firing alerts
	assert.Equal(t, 0, slmClient.GetCallCount())
	assert.Equal(t, 0, executor.GetCallCount())
}

func TestProcessor_ProcessAlert_SLMError(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)

	slmError := fmt.Errorf("SLM analysis failed")
	slmClient.SetError(slmError)

	processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, logger)
	ctx := context.Background()

	alert := types.Alert{
		Name:      "HighCPUUsage",
		Status:    "firing",
		Severity:  "warning",
		Namespace: "production",
	}

	err := processor.ProcessAlert(ctx, alert)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to analyze alert with SLM")
	assert.Equal(t, 1, slmClient.GetCallCount())
	assert.Equal(t, 0, executor.GetCallCount()) // Should not execute if SLM fails
}

func TestProcessor_ProcessAlert_ExecutorError(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)

	executorError := fmt.Errorf("execution failed")
	executor.SetError(executorError)

	processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, logger)
	ctx := context.Background()

	alert := types.Alert{
		Name:      "HighCPUUsage",
		Status:    "firing",
		Severity:  "warning",
		Namespace: "production",
	}

	err := processor.ProcessAlert(ctx, alert)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute action")
	assert.Equal(t, 1, slmClient.GetCallCount())
	assert.Equal(t, 1, executor.GetCallCount())
}

func TestProcessor_ShouldProcess_NoFilters(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)

	// No filters configured - should process all alerts
	processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, logger)

	alert := types.Alert{
		Name:      "TestAlert",
		Namespace: "any-namespace",
		Severity:  "critical",
	}

	shouldProcess := processor.ShouldProcess(alert)
	assert.True(t, shouldProcess)
}

func TestProcessor_ShouldProcess_WithFilters(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)

	filters := []config.FilterConfig{
		{
			Name: "production-filter",
			Conditions: map[string][]string{
				"namespace": {"production", "staging"},
				"severity":  {"critical", "warning"},
			},
		},
		{
			Name: "critical-only",
			Conditions: map[string][]string{
				"severity": {"critical"},
			},
		},
	}

	processor := NewProcessor(slmClient, executor, filters, logger)

	tests := []struct {
		name           string
		alert          types.Alert
		expectedResult bool
	}{
		{
			name: "matches first filter",
			alert: types.Alert{
				Name:      "ProdAlert",
				Namespace: "production",
				Severity:  "critical",
			},
			expectedResult: true,
		},
		{
			name: "matches second filter",
			alert: types.Alert{
				Name:      "CriticalAlert",
				Namespace: "development",
				Severity:  "critical",
			},
			expectedResult: true,
		},
		{
			name: "doesn't match any filter",
			alert: types.Alert{
				Name:      "InfoAlert",
				Namespace: "development",
				Severity:  "info",
			},
			expectedResult: false,
		},
		{
			name: "partial match on first filter",
			alert: types.Alert{
				Name:      "ProdAlert",
				Namespace: "production",
				Severity:  "info", // Wrong severity
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldProcess := processor.ShouldProcess(tt.alert)
			assert.Equal(t, tt.expectedResult, shouldProcess)
		})
	}
}

func TestProcessor_ProcessAlert_Filtered(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)

	filters := []config.FilterConfig{
		{
			Name: "production-only",
			Conditions: map[string][]string{
				"namespace": {"production"},
			},
		},
	}

	processor := NewProcessor(slmClient, executor, filters, logger)
	ctx := context.Background()

	alert := types.Alert{
		Name:      "DevAlert",
		Status:    "firing",
		Namespace: "development", // Doesn't match filter
		Severity:  "critical",
	}

	err := processor.ProcessAlert(ctx, alert)

	assert.NoError(t, err) // No error, but alert is filtered out
	assert.Equal(t, 0, slmClient.GetCallCount())
	assert.Equal(t, 0, executor.GetCallCount())
}

func TestProcessor_GetAlertValue_StandardFields(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)
	processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, logger).(*processor)

	alert := types.Alert{
		Name:      "TestAlert",
		Namespace: "test-namespace",
		Severity:  "critical",
		Status:    "firing",
		Resource:  "test-resource",
	}

	tests := []struct {
		name      string
		condition string
		expected  string
	}{
		{
			name:      "severity",
			condition: "severity",
			expected:  "critical",
		},
		{
			name:      "severity uppercase",
			condition: "SEVERITY",
			expected:  "critical",
		},
		{
			name:      "namespace",
			condition: "namespace",
			expected:  "test-namespace",
		},
		{
			name:      "status",
			condition: "status",
			expected:  "firing",
		},
		{
			name:      "alertname",
			condition: "alertname",
			expected:  "TestAlert",
		},
		{
			name:      "alert_name",
			condition: "alert_name",
			expected:  "TestAlert",
		},
		{
			name:      "name",
			condition: "name",
			expected:  "TestAlert",
		},
		{
			name:      "resource",
			condition: "resource",
			expected:  "test-resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.getAlertValue(alert, tt.condition)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessor_GetAlertValue_Labels(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)
	processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, logger).(*processor)

	alert := types.Alert{
		Name: "TestAlert",
		Labels: map[string]string{
			"app":        "my-app",
			"deployment": "my-deployment",
			"pod":        "my-pod-123",
			"custom":     "custom-value",
		},
	}

	tests := []struct {
		name      string
		condition string
		expected  string
	}{
		{
			name:      "app label",
			condition: "app",
			expected:  "my-app",
		},
		{
			name:      "deployment label",
			condition: "deployment",
			expected:  "my-deployment",
		},
		{
			name:      "pod label",
			condition: "pod",
			expected:  "my-pod-123",
		},
		{
			name:      "custom label",
			condition: "custom",
			expected:  "custom-value",
		},
		{
			name:      "non-existent label",
			condition: "non-existent",
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.getAlertValue(alert, tt.condition)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessor_GetAlertValue_Annotations(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)
	processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, logger).(*processor)

	alert := types.Alert{
		Name: "TestAlert",
		Annotations: map[string]string{
			"description": "Test alert description",
			"runbook":     "https://runbook.example.com",
			"summary":     "High CPU usage detected",
			"custom":      "annotation-value",
		},
	}

	tests := []struct {
		name      string
		condition string
		expected  string
	}{
		{
			name:      "description annotation",
			condition: "description",
			expected:  "Test alert description",
		},
		{
			name:      "runbook annotation",
			condition: "runbook",
			expected:  "https://runbook.example.com",
		},
		{
			name:      "summary annotation",
			condition: "summary",
			expected:  "High CPU usage detected",
		},
		{
			name:      "custom annotation",
			condition: "custom",
			expected:  "annotation-value",
		},
		{
			name:      "non-existent annotation",
			condition: "non-existent",
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.getAlertValue(alert, tt.condition)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessor_GetAlertValue_LabelsPriorityOverAnnotations(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)
	processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, logger).(*processor)

	alert := types.Alert{
		Name: "TestAlert",
		Labels: map[string]string{
			"shared_key": "label-value",
		},
		Annotations: map[string]string{
			"shared_key": "annotation-value",
		},
	}

	// Labels should take priority over annotations
	result := processor.getAlertValue(alert, "shared_key")
	assert.Equal(t, "label-value", result)
}

func TestProcessor_GetAlertValue_EmptyValues(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)
	processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, logger).(*processor)

	alert := types.Alert{
		Name:        "TestAlert",
		Namespace:   "",
		Severity:    "",
		Status:      "",
		Resource:    "",
		Labels:      map[string]string{},
		Annotations: map[string]string{},
	}

	tests := []struct {
		name      string
		condition string
		expected  string
	}{
		{
			name:      "empty severity",
			condition: "severity",
			expected:  "",
		},
		{
			name:      "empty namespace",
			condition: "namespace",
			expected:  "",
		},
		{
			name:      "empty status",
			condition: "status",
			expected:  "",
		},
		{
			name:      "empty resource",
			condition: "resource",
			expected:  "",
		},
		{
			name:      "non-existent condition",
			condition: "unknown",
			expected:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.getAlertValue(alert, tt.condition)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessor_GetAlertValue_MixedCaseConditions(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)
	processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, logger).(*processor)

	alert := types.Alert{
		Name:      "TestAlert",
		Namespace: "test-namespace",
		Severity:  "critical",
		Status:    "firing",
		Resource:  "test-resource",
	}

	tests := []struct {
		name      string
		condition string
		expected  string
	}{
		{
			name:      "SEVERITY uppercase",
			condition: "SEVERITY",
			expected:  "critical",
		},
		{
			name:      "Namespace mixed case",
			condition: "Namespace",
			expected:  "test-namespace",
		},
		{
			name:      "STATUS uppercase",
			condition: "STATUS",
			expected:  "firing",
		},
		{
			name:      "ALERTNAME uppercase",
			condition: "ALERTNAME",
			expected:  "TestAlert",
		},
		{
			name:      "Alert_Name mixed case",
			condition: "Alert_Name",
			expected:  "TestAlert",
		},
		{
			name:      "NAME uppercase",
			condition: "NAME",
			expected:  "TestAlert",
		},
		{
			name:      "RESOURCE uppercase",
			condition: "RESOURCE",
			expected:  "test-resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.getAlertValue(alert, tt.condition)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessor_GetAlertValue_NilMaps(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel)

	slmClient := NewFakeSLMClient(true)
	executor := NewFakeExecutor(true)
	processor := NewProcessor(slmClient, executor, []config.FilterConfig{}, logger).(*processor)

	alert := types.Alert{
		Name:        "TestAlert",
		Labels:      nil,
		Annotations: nil,
	}

	// Should not panic when accessing nil maps
	result := processor.getAlertValue(alert, "non-existent")
	assert.Equal(t, "", result)

	// Should still work for standard fields
	result = processor.getAlertValue(alert, "name")
	assert.Equal(t, "TestAlert", result)
}