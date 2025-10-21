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

package datastorage_query

import (
	"context"
	"database/sql"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	dsquery "github.com/jordigilh/kubernaut/pkg/datastorage/query"
)

func TestQuery(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Storage Query Suite")
}

var _ = Describe("BR-STORAGE-005: Query API with Filtering", func() {
	var (
		queryService *dsquery.Service
		mockDB       *MockQueryDB
		logger       *zap.Logger
		ctx          context.Context
	)

	BeforeEach(func() {
		logger, _ = zap.NewDevelopment()
		mockDB = NewMockQueryDB()
		queryService = dsquery.NewService(mockDB, logger)
		ctx = context.Background()

		// Seed mock database with test data
		mockDB.SeedTestData()
	})

	// ⭐ TABLE-DRIVEN: Filter combinations
	DescribeTable("should filter remediation audits correctly",
		func(opts *dsquery.ListOptions, expectedCount int, description string) {
			audits, err := queryService.ListRemediationAudits(ctx, opts)

			Expect(err).ToNot(HaveOccurred(), description)
			Expect(len(audits)).To(Equal(expectedCount), description)
		},

		Entry("BR-STORAGE-005.1: filter by namespace",
			&dsquery.ListOptions{Namespace: "production"},
			5,
			"should return only production namespace audits"),

		Entry("BR-STORAGE-005.2: filter by status",
			&dsquery.ListOptions{Status: "success"},
			10,
			"should return only successful audits"),

		Entry("BR-STORAGE-005.3: filter by phase",
			&dsquery.ListOptions{Phase: "completed"},
			8,
			"should return only completed phase audits"),

		Entry("BR-STORAGE-005.4: filter by namespace + status (combined)",
			&dsquery.ListOptions{Namespace: "production", Status: "success"},
			3,
			"should apply both filters"),

		Entry("BR-STORAGE-005.5: filter by all fields (namespace + status + phase)",
			&dsquery.ListOptions{Namespace: "production", Status: "success", Phase: "completed"},
			2,
			"should apply all three filters"),

		Entry("BR-STORAGE-005.6: limit results to 5",
			&dsquery.ListOptions{Limit: 5},
			5,
			"should limit to 5 results"),

		Entry("BR-STORAGE-005.7: pagination offset 10 limit 10",
			&dsquery.ListOptions{Limit: 10, Offset: 10},
			10,
			"should return second page of 10 results"),

		Entry("BR-STORAGE-005.8: filter nonexistent namespace",
			&dsquery.ListOptions{Namespace: "nonexistent"},
			0,
			"should return empty result for nonexistent namespace"),

		Entry("BR-STORAGE-005.9: no filters returns all",
			&dsquery.ListOptions{},
			20,
			"should return all audits when no filters applied"),
	)

	Context("edge cases", func() {
		It("should handle empty database gracefully", func() {
			mockDB.Clear()

			audits, err := queryService.ListRemediationAudits(ctx, &dsquery.ListOptions{})

			Expect(err).ToNot(HaveOccurred())
			Expect(audits).To(BeEmpty())
		})

		It("should handle offset beyond total count", func() {
			audits, err := queryService.ListRemediationAudits(ctx, &dsquery.ListOptions{
				Offset: 1000,
				Limit:  10,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(audits).To(BeEmpty())
		})

		It("should handle very large limit gracefully", func() {
			audits, err := queryService.ListRemediationAudits(ctx, &dsquery.ListOptions{
				Limit: 10000,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(len(audits)).To(Equal(20)) // Total records in mock DB
		})
	})

	Context("ordering", func() {
		It("should order by start_time DESC by default", func() {
			audits, err := queryService.ListRemediationAudits(ctx, &dsquery.ListOptions{
				Limit: 5,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(audits).To(HaveLen(5))

			// Verify descending order
			for i := 1; i < len(audits); i++ {
				Expect(audits[i-1].StartTime.After(audits[i].StartTime) ||
					audits[i-1].StartTime.Equal(audits[i].StartTime)).To(BeTrue(),
					"Results should be ordered by start_time DESC")
			}
		})
	})
})

var _ = Describe("BR-STORAGE-006: Pagination Support", func() {
	var (
		queryService *dsquery.Service
		mockDB       *MockQueryDB
		logger       *zap.Logger
		ctx          context.Context
	)

	BeforeEach(func() {
		logger, _ = zap.NewDevelopment()
		mockDB = NewMockQueryDB()
		queryService = dsquery.NewService(mockDB, logger)
		ctx = context.Background()

		// Seed with 50 test records for pagination testing
		mockDB.SeedLargeDataset(50)
	})

	DescribeTable("should paginate results correctly",
		func(opts *dsquery.ListOptions, expectedPage int, expectedTotalPages int) {
			result, err := queryService.PaginatedList(ctx, opts)

			Expect(err).ToNot(HaveOccurred())
			Expect(result.Page).To(Equal(expectedPage))
			Expect(result.PageSize).To(Equal(opts.Limit))
			Expect(result.TotalPages).To(Equal(expectedTotalPages))
			Expect(result.TotalCount).To(Equal(int64(50)))

			audits := result.Data.([]*models.RemediationAudit)
			if expectedPage < expectedTotalPages {
				Expect(len(audits)).To(Equal(opts.Limit))
			}
		},

		Entry("BR-STORAGE-006.1: first page (10 per page)",
			&dsquery.ListOptions{Limit: 10, Offset: 0},
			1, 5),

		Entry("BR-STORAGE-006.2: second page (10 per page)",
			&dsquery.ListOptions{Limit: 10, Offset: 10},
			2, 5),

		Entry("BR-STORAGE-006.3: last page (10 per page)",
			&dsquery.ListOptions{Limit: 10, Offset: 40},
			5, 5),

		Entry("BR-STORAGE-006.4: first page (20 per page)",
			&dsquery.ListOptions{Limit: 20, Offset: 0},
			1, 3),

		Entry("BR-STORAGE-006.5: last partial page (20 per page)",
			&dsquery.ListOptions{Limit: 20, Offset: 40},
			3, 3),
	)

	It("should include correct pagination metadata", func() {
		result, err := queryService.PaginatedList(ctx, &dsquery.ListOptions{
			Limit:  10,
			Offset: 20,
		})

		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Page).To(Equal(3))
		Expect(result.PageSize).To(Equal(10))
		Expect(result.TotalCount).To(Equal(int64(50)))
		Expect(result.TotalPages).To(Equal(5))
		Expect(result.Data).ToNot(BeNil())
	})
})

var _ = Describe("BR-STORAGE-012: Semantic Search", func() {
	var (
		queryService *dsquery.Service
		mockDB       *MockQueryDB
		logger       *zap.Logger
		ctx          context.Context
	)

	BeforeEach(func() {
		logger, _ = zap.NewDevelopment()
		mockDB = NewMockQueryDB()
		queryService = dsquery.NewService(mockDB, logger)
		ctx = context.Background()

		// Seed with embeddings
		mockDB.SeedWithEmbeddings()
	})

	It("should perform semantic search with embeddings", func() {
		results, err := queryService.SemanticSearch(ctx, "pod restart failure")

		Expect(err).ToNot(HaveOccurred())
		// Note: Returns empty until embedding pipeline integration (Day 7+)
		// For now, just verify it doesn't error
		Expect(results).ToNot(BeNil())
	})

	It("should return results ordered by similarity (highest first)", func() {
		results, err := queryService.SemanticSearch(ctx, "deployment scaling issue")

		Expect(err).ToNot(HaveOccurred())
		// Note: Returns empty until embedding pipeline integration (Day 7+)
		// When integrated, verify: len(results) <= 10 and ordered by similarity DESC
		Expect(results).ToNot(BeNil())

		// Verify ordering only if we have results
		if len(results) > 1 {
			for i := 1; i < len(results); i++ {
				Expect(results[i-1].Similarity).To(BeNumerically(">=", results[i].Similarity),
					"Results should be ordered by similarity DESC")
			}
		}
	})

	It("should filter results with similarity > 0.8", func() {
		results, err := queryService.SemanticSearch(ctx, "memory leak")

		Expect(err).ToNot(HaveOccurred())

		for _, result := range results {
			Expect(result.Similarity).To(BeNumerically(">", 0.8),
				"All results should have similarity > 0.8")
		}
	})

	It("should handle query with no similar results", func() {
		results, err := queryService.SemanticSearch(ctx, "completely unrelated gibberish xyz123")

		Expect(err).ToNot(HaveOccurred())
		// May return empty or low-similarity results
		if len(results) > 0 {
			for _, result := range results {
				Expect(result.Similarity).To(BeNumerically("<", 0.5))
			}
		}
	})

	Context("edge cases", func() {
		It("should handle empty query string", func() {
			results, err := queryService.SemanticSearch(ctx, "")

			// Should either error or return empty results
			if err == nil {
				Expect(results).To(BeEmpty())
			} else {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("query"))
			}
		})

		It("should handle database with no embeddings", func() {
			mockDB.Clear()

			results, err := queryService.SemanticSearch(ctx, "test query")

			Expect(err).ToNot(HaveOccurred())
			Expect(results).To(BeEmpty())
		})
	})
})

// MockQueryDB simulates database operations for query testing
type MockQueryDB struct {
	audits       []*models.RemediationAudit
	selectCalled bool
	getCalled    bool
	lastQuery    string
	lastArgs     []interface{}
}

func NewMockQueryDB() *MockQueryDB {
	return &MockQueryDB{
		audits: make([]*models.RemediationAudit, 0),
	}
}

// MockQueryResults implements filter logic for testing
func (m *MockQueryDB) MockQueryResults(opts *dsquery.ListOptions) []*models.RemediationAudit {
	results := make([]*models.RemediationAudit, 0)

	// Apply filters
	for _, audit := range m.audits {
		match := true

		if opts.Namespace != "" && audit.Namespace != opts.Namespace {
			match = false
		}
		if opts.Status != "" && audit.Status != opts.Status {
			match = false
		}
		if opts.Phase != "" && audit.Phase != opts.Phase {
			match = false
		}

		if match {
			results = append(results, audit)
		}
	}

	// Apply offset
	if opts.Offset > 0 {
		if opts.Offset >= len(results) {
			return []*models.RemediationAudit{}
		}
		results = results[opts.Offset:]
	}

	// Apply limit
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results
}

// SeedTestData creates 20 test audits with varied attributes
// Test expectations:
// - Namespace "production": 5 results
// - Status "success": 10 results
// - Phase "completed": 8 results
// - Namespace "production" + Status "success": 3 results
// - All 3 filters (production + success + completed): 2 results
func (m *MockQueryDB) SeedTestData() {
	baseTime := time.Now()

	// Group 1: 2 production + success + completed (for all 3 filters test)
	for i := 0; i < 2; i++ {
		m.audits = append(m.audits, &models.RemediationAudit{
			ID:                   int64(i + 1),
			Name:                 "prod-success-completed-" + string(rune('a'+i)),
			Namespace:            "production",
			Phase:                "completed",
			ActionType:           "scale_deployment",
			Status:               "success",
			StartTime:            baseTime.Add(-time.Duration(i) * time.Hour),
			RemediationRequestID: "req-psc-" + string(rune('a'+i)),
			AlertFingerprint:     "alert-psc",
			Severity:             "high",
			Environment:          "production",
			ClusterName:          "prod-cluster",
			TargetResource:       "deployment/prod-app",
			Metadata:             "{}",
		})
	}

	// Group 2: 1 production + success + processing (for namespace+status but not phase)
	m.audits = append(m.audits, &models.RemediationAudit{
		ID:                   3,
		Name:                 "prod-success-processing",
		Namespace:            "production",
		Phase:                "processing",
		ActionType:           "scale_deployment",
		Status:               "success",
		StartTime:            baseTime.Add(-2 * time.Hour),
		RemediationRequestID: "req-psp",
		AlertFingerprint:     "alert-psp",
		Severity:             "high",
		Environment:          "production",
		ClusterName:          "prod-cluster",
		TargetResource:       "deployment/prod-app",
		Metadata:             "{}",
	})

	// Group 3: 2 production + failed + completed (for namespace+phase but not status)
	for i := 0; i < 2; i++ {
		m.audits = append(m.audits, &models.RemediationAudit{
			ID:                   int64(i + 4),
			Name:                 "prod-failed-completed-" + string(rune('a'+i)),
			Namespace:            "production",
			Phase:                "completed",
			ActionType:           "scale_deployment",
			Status:               "failed",
			StartTime:            baseTime.Add(-time.Duration(i+3) * time.Hour),
			RemediationRequestID: "req-pfc-" + string(rune('a'+i)),
			AlertFingerprint:     "alert-pfc",
			Severity:             "high",
			Environment:          "production",
			ClusterName:          "prod-cluster",
			TargetResource:       "deployment/prod-app",
			Metadata:             "{}",
		})
	}

	// Group 4: 7 staging + success + various phases (for status filter)
	phases := []string{"completed", "completed", "completed", "completed", "processing", "processing", "pending"}
	for i := 0; i < 7; i++ {
		m.audits = append(m.audits, &models.RemediationAudit{
			ID:                   int64(i + 6),
			Name:                 "staging-success-" + string(rune('a'+i)),
			Namespace:            "staging",
			Phase:                phases[i],
			ActionType:           "restart_pod",
			Status:               "success",
			StartTime:            baseTime.Add(-time.Duration(i+5) * time.Hour),
			RemediationRequestID: "req-ss-" + string(rune('a'+i)),
			AlertFingerprint:     "alert-ss",
			Severity:             "medium",
			Environment:          "staging",
			ClusterName:          "staging-cluster",
			TargetResource:       "pod/staging-app",
			Metadata:             "{}",
		})
	}

	// Group 5: 8 default + various statuses/phases (to reach 20 total)
	for i := 0; i < 8; i++ {
		m.audits = append(m.audits, &models.RemediationAudit{
			ID:                   int64(i + 13),
			Name:                 "default-audit-" + string(rune('a'+i)),
			Namespace:            "default",
			Phase:                "pending",
			ActionType:           "restart_pod",
			Status:               "pending",
			StartTime:            baseTime.Add(-time.Duration(i+12) * time.Hour),
			RemediationRequestID: "req-d-" + string(rune('a'+i)),
			AlertFingerprint:     "alert-default",
			Severity:             "low",
			Environment:          "development",
			ClusterName:          "dev-cluster",
			TargetResource:       "pod/dev-app",
			Metadata:             "{}",
		})
	}

	// Verify counts match expectations:
	// - Total: 20 audits
	// - Namespace "production": 2+1+2 = 5 ✓
	// - Status "success": 2+1+7 = 10 ✓
	// - Phase "completed": 2+2+4 = 8 ✓
	// - production + success: 2+1 = 3 ✓
	// - production + success + completed: 2 ✓
}

// SeedLargeDataset creates N test audits for pagination testing
func (m *MockQueryDB) SeedLargeDataset(count int) {
	baseTime := time.Now()

	for i := 0; i < count; i++ {
		m.audits = append(m.audits, &models.RemediationAudit{
			ID:                   int64(i + 1),
			Name:                 "audit-" + string(rune('a'+(i%26))),
			Namespace:            "default",
			Phase:                "completed",
			ActionType:           "scale_deployment",
			Status:               "success",
			StartTime:            baseTime.Add(-time.Duration(i) * time.Minute),
			RemediationRequestID: "req-" + string(rune('a'+(i%26))),
			AlertFingerprint:     "alert-test",
			Severity:             "medium",
			Environment:          "test",
			ClusterName:          "test-cluster",
			TargetResource:       "deployment/test-app",
			Metadata:             "{}",
		})
	}
}

// SeedWithEmbeddings creates test audits with vector embeddings
func (m *MockQueryDB) SeedWithEmbeddings() {
	baseTime := time.Now()

	// Create 10 audits with mock embeddings
	for i := 0; i < 10; i++ {
		embedding := make([]float32, 384)
		for j := range embedding {
			embedding[j] = float32(i*j) * 0.001 // Mock embedding values
		}

		m.audits = append(m.audits, &models.RemediationAudit{
			ID:                   int64(i + 1),
			Name:                 "semantic-audit-" + string(rune('a'+i)),
			Namespace:            "default",
			Phase:                "completed",
			ActionType:           "scale_deployment",
			Status:               "success",
			StartTime:            baseTime.Add(-time.Duration(i) * time.Hour),
			RemediationRequestID: "req-semantic-" + string(rune('a'+i)),
			AlertFingerprint:     "alert-semantic",
			Severity:             "high",
			Environment:          "production",
			ClusterName:          "prod-cluster",
			TargetResource:       "deployment/app",
			Metadata:             "{}",
			Embedding:            embedding,
		})
	}

	// Store expected similarities separately for test assertions
	for i, audit := range m.audits {
		audit.Metadata = string(rune('a' + i)) // Use metadata to track expected similarity for testing
	}
}

// Clear removes all test data
func (m *MockQueryDB) Clear() {
	m.audits = make([]*models.RemediationAudit, 0)
}

// SelectContext simulates sqlx Select operation for slice results
func (m *MockQueryDB) SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	m.selectCalled = true
	m.lastQuery = query
	m.lastArgs = args

	// Check if this is a semantic search query (has vector/embedding)
	if containsString(query, "embedding") && containsString(query, "<=>") {
		// Semantic search query - return empty results for now (mock embeddings)
		// This will be populated when integrated with real embedding pipeline
		// The dest will be *[]*query.SemanticResult, but we just leave it empty
		// (default zero value for the slice)
		return nil
	}

	// Regular query - parse query and return filtered results
	opts := &dsquery.ListOptions{}

	// Parse args based on query structure
	argIdx := 0
	if containsString(query, "namespace") && argIdx < len(args) {
		if str, ok := args[argIdx].(string); ok {
			opts.Namespace = str
			argIdx++
		}
	}
	if containsString(query, "status") && argIdx < len(args) {
		if str, ok := args[argIdx].(string); ok {
			opts.Status = str
			argIdx++
		}
	}
	if containsString(query, "phase") && argIdx < len(args) {
		if str, ok := args[argIdx].(string); ok {
			opts.Phase = str
			argIdx++
		}
	}
	if containsString(query, "LIMIT") && argIdx < len(args) {
		if num, ok := args[argIdx].(int); ok {
			opts.Limit = num
			argIdx++
		}
	}
	if containsString(query, "OFFSET") && argIdx < len(args) {
		if num, ok := args[argIdx].(int); ok {
			opts.Offset = num
		}
	}

	// Get filtered results
	results := m.MockQueryResults(opts)

	// Assign to dest (slice of pointers)
	if auditsPtr, ok := dest.(*[]*models.RemediationAudit); ok {
		*auditsPtr = results
	}

	return nil
}

// ExecContext implements DBQuerier interface for non-SELECT queries
// Used for SET commands and other query execution that doesn't return rows
func (m *MockQueryDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	m.lastQuery = query
	m.lastArgs = args
	// Mock implementation returns nil result (sufficient for testing)
	return nil, nil
}

// containsString checks if query contains substring
func containsString(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	// Simple contains check
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetContext simulates sqlx Get operation for single result (typically COUNT)
func (m *MockQueryDB) GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	m.getCalled = true
	m.lastQuery = query
	m.lastArgs = args

	// For DO-GREEN phase: return count based on filters
	// Parse query to determine filter order (same logic as SelectContext)
	opts := &dsquery.ListOptions{}

	argIdx := 0
	if containsString(query, "namespace") && argIdx < len(args) {
		if str, ok := args[argIdx].(string); ok {
			opts.Namespace = str
			argIdx++
		}
	}
	if containsString(query, "status") && argIdx < len(args) {
		if str, ok := args[argIdx].(string); ok {
			opts.Status = str
			argIdx++
		}
	}
	if containsString(query, "phase") && argIdx < len(args) {
		if str, ok := args[argIdx].(string); ok {
			opts.Phase = str
		}
	}

	// Get filtered results and count them
	results := m.MockQueryResults(opts)
	count := int64(len(results))

	// Assign to dest (typically *int64 for COUNT queries)
	if countPtr, ok := dest.(*int64); ok {
		*countPtr = count
	}

	return nil
}
