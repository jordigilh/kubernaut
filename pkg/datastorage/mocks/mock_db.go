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

// MockDB is a simple mock database for unit testing REST handlers
// This is a minimal implementation for GREEN phase - will be enhanced in REFACTOR
type MockDB struct {
	recordCount int
	incidents   []map[string]interface{}
}

// NewMockDB creates a new mock database
func NewMockDB() *MockDB {
	return &MockDB{
		recordCount: 0,
		incidents:   make([]map[string]interface{}, 0),
	}
}

// SetRecordCount configures the mock to return a specific number of records
// Used for performance testing scenarios
func (m *MockDB) SetRecordCount(count int) {
	m.recordCount = count

	// Generate mock incidents
	m.incidents = make([]map[string]interface{}, count)
	for i := 0; i < count; i++ {
		m.incidents[i] = map[string]interface{}{
			"id":          i + 1,
			"namespace":   "test-namespace",
			"action_type": "scale_deployment",
			"severity":    "high",
		}
	}
}

// Query simulates a database query
// Returns incidents based on configured recordCount
func (m *MockDB) Query(filters map[string]string, limit, offset int) ([]map[string]interface{}, error) {
	// BR-STORAGE-025: Return empty array for nonexistent namespaces
	if ns, ok := filters["namespace"]; ok && ns == "nonexistent" {
		return []map[string]interface{}{}, nil
	}

	// Simple mock: return configured incidents
	if len(m.incidents) == 0 {
		// Default: return 3 mock incidents
		return []map[string]interface{}{
			{
				"id":          1,
				"namespace":   filters["namespace"],
				"action_type": "scale_deployment",
				"severity":    "high",
			},
			{
				"id":          2,
				"namespace":   filters["namespace"],
				"action_type": "restart_pod",
				"severity":    "critical",
			},
			{
				"id":          3,
				"namespace":   filters["namespace"],
				"action_type": "rollback_deployment",
				"severity":    "medium",
			},
		}, nil
	}

	// Return paginated results
	start := offset
	end := offset + limit
	if start >= len(m.incidents) {
		return []map[string]interface{}{}, nil
	}
	if end > len(m.incidents) {
		end = len(m.incidents)
	}

	return m.incidents[start:end], nil
}

// Get simulates retrieving a single incident by ID
func (m *MockDB) Get(id int) (map[string]interface{}, error) {
	if id == 999999 {
		// Not found case for testing
		return nil, nil
	}

	return map[string]interface{}{
		"id":          id,
		"namespace":   "test-namespace",
		"action_type": "scale_deployment",
		"severity":    "high",
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

	// Simple mock: return total count based on configured incidents
	if len(m.incidents) == 0 {
		// Default: 3 mock incidents (same as Query default)
		return 3, nil
	}

	// Return total count of all incidents (not filtered by limit/offset)
	return int64(len(m.incidents)), nil
}
