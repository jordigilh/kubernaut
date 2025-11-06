package server

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestServerPathNormalization(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Server Path Normalization Suite")
}

// ============================================================================
// BR-CONTEXT-006: Observability - Metric Cardinality Management
// DD-005 § 3.1: Metrics Cardinality Management
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

var _ = Describe("BR-CONTEXT-006: Path Normalization for Metrics Cardinality", func() {
	
	Context("Static paths (no IDs)", func() {
		DescribeTable("Should preserve static endpoint paths unchanged",
			func(input, expected string) {
				result := normalizePath(input)
				Expect(result).To(Equal(expected))
			},
			Entry("health endpoint", "/health", "/health"),
			Entry("ready endpoint", "/ready", "/ready"),
			Entry("metrics endpoint", "/metrics", "/metrics"),
			Entry("query endpoint", "/api/v1/context/query", "/api/v1/context/query"),
			Entry("search endpoint", "/api/v1/context/search", "/api/v1/context/search"),
			Entry("root path", "/", "/"),
		)
	})

	Context("UUID-based paths", func() {
		DescribeTable("Should normalize UUID segments to :id placeholder",
			func(input, expected string) {
				result := normalizePath(input)
				Expect(result).To(Equal(expected))
			},
			Entry("full UUID", 
				"/api/v1/incidents/550e8400-e29b-41d4-a716-446655440000",
				"/api/v1/incidents/:id"),
			Entry("short UUID with hyphens",
				"/api/v1/incidents/abc-123-def",
				"/api/v1/incidents/:id"),
			Entry("alphanumeric ID",
				"/api/v1/incidents/abc123def456",
				"/api/v1/incidents/:id"),
		)
	})

	Context("Numeric IDs", func() {
		DescribeTable("Should normalize numeric ID segments to :id placeholder",
			func(input, expected string) {
				result := normalizePath(input)
				Expect(result).To(Equal(expected))
			},
			Entry("incident with numeric ID",
				"/api/v1/incidents/12345",
				"/api/v1/incidents/:id"),
			Entry("context with numeric ID",
				"/api/v1/context/67890",
				"/api/v1/context/:id"),
		)
	})

	Context("Nested resources with multiple IDs", func() {
		DescribeTable("Should normalize each ID segment independently",
			func(input, expected string) {
				result := normalizePath(input)
				Expect(result).To(Equal(expected))
			},
			Entry("nested resource with single UUID",
				"/api/v1/incidents/550e8400-e29b-41d4-a716-446655440000/actions",
				"/api/v1/incidents/:id/actions"),
			Entry("nested resource with multiple IDs",
				"/api/v1/incidents/abc-123/actions/def-456",
				"/api/v1/incidents/:id/actions/:id"),
		)
	})

	Context("Edge cases", func() {
		DescribeTable("Should handle edge cases correctly",
			func(input, expected string) {
				result := normalizePath(input)
				Expect(result).To(Equal(expected))
			},
			Entry("trailing slash preserved",
				"/api/v1/incidents/abc-123/",
				"/api/v1/incidents/:id/"),
			Entry("version segments not normalized",
				"/api/v1/context/query",
				"/api/v1/context/query"),
		)
	})

	Context("Idempotency", func() {
		It("should be idempotent (normalizing twice produces same result)", func() {
			input := "/api/v1/incidents/550e8400-e29b-41d4-a716-446655440000"
			
			first := normalizePath(input)
			second := normalizePath(first)
			
			Expect(first).To(Equal(second))
			Expect(second).To(Equal("/api/v1/incidents/:id"))
		})
	})

	Context("Path structure preservation", func() {
		DescribeTable("Should preserve number of path segments",
			func(input string, expectedSegments int) {
				result := normalizePath(input)
				
				// Count non-empty segments
				segments := splitPath(result)
				count := 0
				for _, seg := range segments {
					if seg != "" {
						count++
					}
				}
				
				Expect(count).To(Equal(expectedSegments))
			},
			Entry("single segment", "/health", 1),
			Entry("four segments", "/api/v1/context/query", 4),
			Entry("four segments with ID", "/api/v1/incidents/abc-123", 4),
			Entry("five segments", "/api/v1/incidents/abc-123/actions", 5),
			Entry("six segments", "/api/v1/incidents/abc-123/actions/def-456", 6),
		)
	})
})

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
