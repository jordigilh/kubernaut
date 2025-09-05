package patterns

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// collectHistoricalDataSimple collects historical workflow execution data (simplified implementation)
func (pde *PatternDiscoveryEngine) collectHistoricalDataSimple(ctx context.Context, request *PatternAnalysisRequest) ([]*engine.WorkflowExecutionData, error) {
	pde.log.WithFields(logrus.Fields{
		"analysis_type": request.AnalysisType,
		"time_range":    fmt.Sprintf("%v to %v", request.TimeRange.Start, request.TimeRange.End),
		"pattern_types": request.PatternTypes,
	}).Info("Collecting historical data for pattern analysis")

	// Create mock historical data for testing and development
	// In production, this would query the actual repository
	mockData := pde.generateMockHistoricalData(request)

	pde.log.WithField("data_points", len(mockData)).Info("Historical data collection completed")

	return mockData, nil
}

// generateMockHistoricalData creates realistic mock data for testing
func (pde *PatternDiscoveryEngine) generateMockHistoricalData(request *PatternAnalysisRequest) []*engine.WorkflowExecutionData {
	now := time.Now()
	data := make([]*engine.WorkflowExecutionData, 0)

	// Generate mock data for different alert types
	alertTypes := []string{"HighMemoryUsage", "PodCrashLoop", "NodeNotReady", "DiskSpaceCritical"}
	namespaces := []string{"default", "kube-system", "monitoring", "production"}

	for i := 0; i < 50; i++ {
		alertType := alertTypes[i%len(alertTypes)]
		namespace := namespaces[i%len(namespaces)]

		// Vary success rates based on alert type to create patterns
		successRate := 0.8
		if alertType == "NodeNotReady" {
			successRate = 0.6 // More challenging alerts
		} else if alertType == "PodCrashLoop" {
			successRate = 0.7
		}

		success := float64(i%10) < successRate*10

		execData := &engine.WorkflowExecutionData{
			ExecutionID: fmt.Sprintf("mock-exec-%d", i),
			WorkflowID:  fmt.Sprintf("template-%s", alertType),
			Timestamp:   now.Add(time.Duration(-i) * time.Hour),
			Duration:    time.Duration(30+i%120) * time.Second,
			Success:     success,
			Metrics: map[string]float64{
				"cpu_usage":     0.2 + float64(i%30)/100.0,
				"memory_usage":  0.3 + float64(i%40)/100.0,
				"network_usage": 0.1 + float64(i%20)/100.0,
				"storage_usage": 0.15 + float64(i%25)/100.0,
			},
			Metadata: map[string]interface{}{
				"alert": &types.Alert{
					Name:      alertType,
					Severity:  pde.randomSeverity(i),
					Namespace: namespace,
					Resource:  pde.randomResourceType(i),
					Labels: map[string]string{
						"app":     fmt.Sprintf("app-%d", i%5),
						"version": fmt.Sprintf("v%d.%d", (i%3)+1, (i%5)+1),
					},
				},
				"execution_result": map[string]interface{}{
					"success":         success,
					"steps_completed": 3 + (i % 5),
					"duration":        time.Duration(30+i%120) * time.Second,
					"error_message":   pde.generateErrorMessage(success, alertType),
				},
				"historical_data": map[string]interface{}{
					"recent_failures":      i % 3,
					"average_success_rate": successRate + (float64(i%10)-5)/20.0,
					"last_execution_time":  now.Add(time.Duration(-(i + 1)) * time.Hour),
				},
				"workflow_template": map[string]interface{}{
					"step_count":       5 + (i % 3),
					"dependency_depth": 2 + (i % 2),
					"parallel_steps":   1 + (i % 2),
				},
				"environment_metrics": map[string]interface{}{
					"cluster_size":      10 + (i % 20),
					"cluster_load":      0.4 + float64(i%30)/100.0,
					"resource_pressure": 0.2 + float64(i%40)/100.0,
				},
			},
		}

		data = append(data, execData)
	}

	return data
}

// Helper methods for mock data generation
func (pde *PatternDiscoveryEngine) randomSeverity(seed int) string {
	severities := []string{"critical", "warning", "info"}
	return severities[seed%len(severities)]
}

func (pde *PatternDiscoveryEngine) randomResourceType(seed int) string {
	resourceTypes := []string{"deployment", "pod", "service", "node", "pvc"}
	return resourceTypes[seed%len(resourceTypes)]
}

func (pde *PatternDiscoveryEngine) generateErrorMessage(success bool, alertType string) *string {
	if success {
		return nil
	}

	messages := map[string]string{
		"HighMemoryUsage":   "Failed to scale deployment: resource limits exceeded",
		"PodCrashLoop":      "Pod restart failed: image pull error",
		"NodeNotReady":      "Node cordoning failed: insufficient permissions",
		"DiskSpaceCritical": "Cleanup failed: no removable files found",
	}

	if msg, exists := messages[alertType]; exists {
		return &msg
	}
	return stringPtr("Unknown error occurred")
}

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}
