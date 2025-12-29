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

package mocks

import (
	"time"

	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// MockDB is a simple mock database for unit testing REST handlers
// V1.0: Uses structured types for type safety
type MockDB struct {
	recordCount     int
	auditEvents     []*repository.AuditEvent // V1.0: Structured types
	aggregationData map[string]interface{}   // For aggregation endpoint mocking (structured types)
}

// NewMockDB creates a new mock database
func NewMockDB() *MockDB {
	return &MockDB{
		recordCount:     0,
		auditEvents:     make([]*repository.AuditEvent, 0),
		aggregationData: make(map[string]interface{}),
	}
}

// SetRecordCount configures the mock to return a specific number of records
// Used for performance testing scenarios
func (m *MockDB) SetRecordCount(count int) {
	m.recordCount = count

	// Generate mock audit events
	m.auditEvents = make([]*repository.AuditEvent, count)
	for i := 0; i < count; i++ {
		now := time.Now()
		m.auditEvents[i] = &repository.AuditEvent{
			EventID:           uuid.New(),
			EventTimestamp:    now,
			EventDate:         repository.DateOnly(now.Truncate(24 * time.Hour)),
			EventType:         "test.event",
			Version:           "1.0",
			EventCategory:     "test",
			EventAction:       "test_action",
			EventOutcome:      "success",
			ResourceNamespace: "test-namespace",
			Severity:          "high",
		}
	}
}

// Query simulates a database query
// V1.0: Returns structured audit events
func (m *MockDB) Query(filters map[string]string, limit, offset int) ([]*repository.AuditEvent, error) {
	// BR-STORAGE-025: Return empty array for nonexistent namespaces
	if ns, ok := filters["namespace"]; ok && ns == "nonexistent" {
		return []*repository.AuditEvent{}, nil
	}

	// Simple mock: return configured audit events
	if len(m.auditEvents) == 0 {
		// Default: return 3 mock audit events
		namespace := filters["namespace"]
		if namespace == "" {
			namespace = "test-namespace"
		}

		now := time.Now()
		return []*repository.AuditEvent{
			{
				EventID:           uuid.New(),
				EventTimestamp:    now,
				EventDate:         repository.DateOnly(now.Truncate(24 * time.Hour)),
				EventType:         "test.event.1",
				Version:           "1.0",
				EventCategory:     "test",
				EventAction:       "scale_deployment",
				EventOutcome:      "success",
				ResourceNamespace: namespace,
				Severity:          "high",
			},
			{
				EventID:           uuid.New(),
				EventTimestamp:    now,
				EventDate:         repository.DateOnly(now.Truncate(24 * time.Hour)),
				EventType:         "test.event.2",
				Version:           "1.0",
				EventCategory:     "test",
				EventAction:       "restart_pod",
				EventOutcome:      "success",
				ResourceNamespace: namespace,
				Severity:          "critical",
			},
			{
				EventID:           uuid.New(),
				EventTimestamp:    now,
				EventDate:         repository.DateOnly(now.Truncate(24 * time.Hour)),
				EventType:         "test.event.3",
				Version:           "1.0",
				EventCategory:     "test",
				EventAction:       "rollback_deployment",
				EventOutcome:      "success",
				ResourceNamespace: namespace,
				Severity:          "medium",
			},
		}, nil
	}

	// Return paginated results
	start := offset
	end := offset + limit
	if start >= len(m.auditEvents) {
		return []*repository.AuditEvent{}, nil
	}
	if end > len(m.auditEvents) {
		end = len(m.auditEvents)
	}

	return m.auditEvents[start:end], nil
}

// Get simulates retrieving a single audit event by ID
// V1.0: Returns structured audit event
func (m *MockDB) Get(id int) (*repository.AuditEvent, error) {
	if id == 999999 {
		// Not found case for testing
		return nil, nil
	}

	// Return mock audit event
	now := time.Now()
	return &repository.AuditEvent{
		EventID:           uuid.New(),
		EventTimestamp:    now,
		EventDate:         repository.DateOnly(now.Truncate(24 * time.Hour)),
		EventType:         "test.event",
		Version:           "1.0",
		EventCategory:     "test",
		EventAction:       "scale_deployment",
		EventOutcome:      "success",
		ResourceNamespace: "test-namespace",
		Severity:          "high",
	}, nil
}

// CountTotal returns the total number of records matching the filters
// 🚨 FIX: Returns database count, not page size (fixes pagination bug)
// This method ensures pagination.total is accurate for the mock DB
func (m *MockDB) CountTotal(filters map[string]string) (int64, error) {
	// BR-STORAGE-025: Return 0 for nonexistent namespaces
	if ns, ok := filters["namespace"]; ok && ns == "nonexistent" {
		return 0, nil
	}

	// Simple mock: return total count based on configured audit events
	if len(m.auditEvents) == 0 {
		// Default: 3 mock audit events (same as Query default)
		return 3, nil
	}

	// Return total count of all audit events (not filtered by limit/offset)
	return int64(len(m.auditEvents)), nil
}

// SetSuccessRateData configures mock success rate aggregation
func (m *MockDB) SetSuccessRateData(data *models.SuccessRateAggregationResponse) {
	m.aggregationData["success_rate"] = data
}

// SetNamespaceAggregationData configures mock namespace aggregation
func (m *MockDB) SetNamespaceAggregationData(data *models.NamespaceAggregationResponse) {
	m.aggregationData["by_namespace"] = data
}

// SetSeverityAggregationData configures mock severity aggregation
func (m *MockDB) SetSeverityAggregationData(data *models.SeverityAggregationResponse) {
	m.aggregationData["by_severity"] = data
}

// SetTrendAggregationData configures mock trend aggregation
func (m *MockDB) SetTrendAggregationData(data *models.TrendAggregationResponse) {
	m.aggregationData["incident_trend"] = data
}

// AggregateSuccessRate calculates success rate for a workflow
// BR-STORAGE-031: Success rate aggregation
// V1.0: Returns structured type
func (m *MockDB) AggregateSuccessRate(workflowID string) (*models.SuccessRateAggregationResponse, error) {
	if data, ok := m.aggregationData["success_rate"]; ok {
		if response, ok := data.(*models.SuccessRateAggregationResponse); ok {
			// Override workflow_id with the requested one
			response.WorkflowID = workflowID
			return response, nil
		}
	}

	// Default: no data
	return &models.SuccessRateAggregationResponse{
		WorkflowID:   workflowID,
		TotalCount:   0,
		SuccessCount: 0,
		FailureCount: 0,
		SuccessRate:  0.0,
	}, nil
}

// AggregateByNamespace groups incidents by namespace
// BR-STORAGE-032: Namespace grouping aggregation
// V1.0: Returns structured type
func (m *MockDB) AggregateByNamespace() (*models.NamespaceAggregationResponse, error) {
	if data, ok := m.aggregationData["by_namespace"]; ok {
		if response, ok := data.(*models.NamespaceAggregationResponse); ok {
			return response, nil
		}
	}

	// Default: empty aggregations
	return &models.NamespaceAggregationResponse{
		Aggregations: []models.NamespaceAggregationItem{},
	}, nil
}

// AggregateBySeverity groups incidents by severity
// BR-STORAGE-033: Severity distribution aggregation
// V1.0: Returns structured type
func (m *MockDB) AggregateBySeverity() (*models.SeverityAggregationResponse, error) {
	if data, ok := m.aggregationData["by_severity"]; ok {
		if response, ok := data.(*models.SeverityAggregationResponse); ok {
			return response, nil
		}
	}

	// Default: empty aggregations
	return &models.SeverityAggregationResponse{
		Aggregations: []models.SeverityAggregationItem{},
	}, nil
}

// AggregateIncidentTrend returns incident counts over time
// BR-STORAGE-034: Incident trend aggregation
// V1.0: Returns structured type
func (m *MockDB) AggregateIncidentTrend(period string) (*models.TrendAggregationResponse, error) {
	if data, ok := m.aggregationData["incident_trend"]; ok {
		if response, ok := data.(*models.TrendAggregationResponse); ok {
			// Override period with the requested one
			response.Period = period
			return response, nil
		}
	}

	// Default: empty data points
	return &models.TrendAggregationResponse{
		Period:     period,
		DataPoints: []models.TrendDataPoint{},
	}, nil
}
