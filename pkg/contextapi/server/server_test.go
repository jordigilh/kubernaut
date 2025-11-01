package server

import (
	"testing"
)

// ============================================================================
// DD-005: Observability Standards - Metric Cardinality Management
// TDD RED PHASE: Path normalization tests
// ============================================================================
//
// Business Requirement: BR-CONTEXT-006 (Observability)
// - Metrics must not cause Prometheus cardinality explosion
// - Dynamic path segments (IDs, UUIDs) must be normalized
//
// Test Coverage:
// 1. Static paths remain unchanged
// 2. UUID-based paths normalized to :id placeholder
// 3. Numeric IDs normalized to :id placeholder
// 4. Multiple ID segments normalized independently
// 5. Query parameters don't affect normalization (already stripped by r.URL.Path)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Static paths - should remain unchanged
		{
			name:     "health endpoint",
			input:    "/health",
			expected: "/health",
		},
		{
			name:     "ready endpoint",
			input:    "/ready",
			expected: "/ready",
		},
		{
			name:     "metrics endpoint",
			input:    "/metrics",
			expected: "/metrics",
		},
		{
			name:     "query endpoint",
			input:    "/api/v1/context/query",
			expected: "/api/v1/context/query",
		},
		{
			name:     "search endpoint",
			input:    "/api/v1/context/search",
			expected: "/api/v1/context/search",
		},

		// UUID-based paths - should be normalized
		{
			name:     "incident with UUID",
			input:    "/api/v1/incidents/550e8400-e29b-41d4-a716-446655440000",
			expected: "/api/v1/incidents/:id",
		},
		{
			name:     "incident with short UUID",
			input:    "/api/v1/incidents/abc-123-def",
			expected: "/api/v1/incidents/:id",
		},
		{
			name:     "incident with alphanumeric ID",
			input:    "/api/v1/incidents/abc123def456",
			expected: "/api/v1/incidents/:id",
		},

		// Numeric IDs - should be normalized
		{
			name:     "incident with numeric ID",
			input:    "/api/v1/incidents/12345",
			expected: "/api/v1/incidents/:id",
		},
		{
			name:     "context with numeric ID",
			input:    "/api/v1/context/67890",
			expected: "/api/v1/context/:id",
		},

		// Multiple segments with IDs
		{
			name:     "nested resource with UUID",
			input:    "/api/v1/incidents/550e8400-e29b-41d4-a716-446655440000/actions",
			expected: "/api/v1/incidents/:id/actions",
		},
		{
			name:     "nested resource with multiple IDs",
			input:    "/api/v1/incidents/abc-123/actions/def-456",
			expected: "/api/v1/incidents/:id/actions/:id",
		},

		// Edge cases
		{
			name:     "root path",
			input:    "/",
			expected: "/",
		},
		{
			name:     "trailing slash",
			input:    "/api/v1/incidents/abc-123/",
			expected: "/api/v1/incidents/:id/",
		},
		{
			name:     "path with version that looks like ID",
			input:    "/api/v1/context/query",
			expected: "/api/v1/context/query", // v1 should NOT be normalized
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePath(tt.input)
			if result != tt.expected {
				t.Errorf("normalizePath(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestNormalizePath_Idempotent verifies normalization is idempotent
func TestNormalizePath_Idempotent(t *testing.T) {
	input := "/api/v1/incidents/550e8400-e29b-41d4-a716-446655440000"

	first := normalizePath(input)
	second := normalizePath(first)

	if first != second {
		t.Errorf("normalizePath is not idempotent: first=%q, second=%q", first, second)
	}

	if second != "/api/v1/incidents/:id" {
		t.Errorf("normalizePath(%q) = %q, expected %q", input, second, "/api/v1/incidents/:id")
	}
}

// TestNormalizePath_PreservesStructure verifies path structure is preserved
func TestNormalizePath_PreservesStructure(t *testing.T) {
	tests := []struct {
		input    string
		expected int // number of path segments
	}{
		{"/health", 1},
		{"/api/v1/context/query", 4},
		{"/api/v1/incidents/abc-123", 4},
		{"/api/v1/incidents/abc-123/actions", 5},
		{"/api/v1/incidents/abc-123/actions/def-456", 6},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizePath(tt.input)

			// Count segments (split by '/', filter empty)
			resultSegments := 0
			for _, seg := range splitPath(result) {
				if seg != "" {
					resultSegments++
				}
			}

			if resultSegments != tt.expected {
				t.Errorf("normalizePath(%q) has %d segments, expected %d", tt.input, resultSegments, tt.expected)
			}
		})
	}
}

// Helper function to split path (for testing)
func splitPath(path string) []string {
	var segments []string
	var current string

	for _, ch := range path {
		if ch == '/' {
			if current != "" {
				segments = append(segments, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}

	if current != "" {
		segments = append(segments, current)
	}

	return segments
}
