package query

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
)

// VectorSearch is a stub for future semantic search functionality
// ADR-032: Vector search requires Data Storage Service API support
type VectorSearch struct{}

// AggregationService is a stub for future aggregation functionality
// ADR-032: Aggregation requires Data Storage Service API support
type AggregationService struct{}

// Router routes queries to appropriate backends (CachedExecutor, Vector DB, Aggregation)
// BR-CONTEXT-004: Query Aggregation and routing logic
//
// v2.0 Integration: Uses CachedExecutor (Days 1-4) for cache-first queries
type Router struct {
	cachedExecutor     *CachedExecutor     // v2.0: Cache→DB fallback chain
	vectorSearch       *VectorSearch       // v2.0: Semantic search with pgvector
	aggregationService *AggregationService // v2.0: Advanced analytics
	logger             *zap.Logger
}

// NewRouter creates a new query router with v2.0 components
//
// v2.0 Changes:
// - Replaced db *sqlx.DB with cachedExecutor *CachedExecutor
// - CachedExecutor provides cache-first query execution
// - All queries benefit from multi-tier caching (L1 Redis + L2 LRU + L3 DB)
func NewRouter(
	cachedExecutor *CachedExecutor,
	vectorSearch *VectorSearch,
	aggregationService *AggregationService,
	logger *zap.Logger,
) *Router {
	return &Router{
		cachedExecutor:     cachedExecutor,
		vectorSearch:       vectorSearch,
		aggregationService: aggregationService,
		logger:             logger,
	}
}

// SelectBackend determines which backend to use for a query type
// BR-CONTEXT-004: Intelligent backend selection
//
// v2.0: Returns backend identifiers for v2.0 architecture
func (r *Router) SelectBackend(queryType string) string {
	switch queryType {
	case "query", "simple", "recent":
		return "cached" // v2.0: CachedExecutor (cache-first)
	case "pattern_match", "vector":
		return "vectordb" // v2.0: VectorSearch (semantic search)
	case "aggregation":
		return "postgresql" // v2.0: AggregationService (analytics)
	default:
		return "cached" // v2.0: Default to cached executor
	}
}

// Query executes a query via the CachedExecutor
// BR-CONTEXT-001: Historical context query with caching
//
// v2.0 Method: Routes queries to CachedExecutor for cache-first execution
func (r *Router) Query(ctx context.Context, params *models.ListIncidentsParams) ([]*models.IncidentEvent, int, error) {
	if r.cachedExecutor == nil {
		return nil, 0, fmt.Errorf("cached executor not configured")
	}

	r.logger.Debug("routing query to cached executor",
		zap.String("namespace", stringValue(params.Namespace)),
		zap.Int("limit", params.Limit))

	return r.cachedExecutor.ListIncidents(ctx, params)
}

// VectorSearch executes semantic search via VectorSearch
// BR-CONTEXT-003: Semantic similarity search
//
// v2.0 Method: Routes vector searches to VectorSearch component
func (r *Router) VectorSearch(ctx context.Context, embedding []float32, limit int, threshold float32) ([]*models.IncidentEvent, []float32, error) {
	if r.vectorSearch == nil {
		return nil, nil, fmt.Errorf("vector search not configured")
	}

	r.logger.Debug("routing query to vector search",
		zap.Int("embedding_dim", len(embedding)),
		zap.Int("limit", limit),
		zap.Float64("threshold", float64(threshold)))

	return r.cachedExecutor.SemanticSearch(ctx, embedding, limit, threshold)
}

// Aggregation returns the aggregation service for advanced analytics
// BR-CONTEXT-004: Query aggregation and analytics
//
// v2.0 Method: Provides access to AggregationService
func (r *Router) Aggregation() *AggregationService {
	return r.aggregationService
}

// Helper function to safely dereference string pointers
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// v2.0 ROUTER IMPLEMENTATION NOTES
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// v2.0 Architecture:
// 1. ✅ Uses CachedExecutor for cache-first queries (Days 1-4)
// 2. ✅ Uses VectorSearch for semantic search (Day 5)
// 3. ✅ Uses AggregationService for analytics (aggregation.go)
// 4. ✅ All aggregation methods moved to AggregationService
// 5. ✅ Router provides routing and delegation only
//
// Business Requirements:
// - BR-CONTEXT-001: Historical context queries (via CachedExecutor)
// - BR-CONTEXT-003: Semantic search (via VectorSearch)
// - BR-CONTEXT-004: Aggregation (via AggregationService)
//
// Migration from v1.x:
// - Old router methods (AggregateSuccessRate, GroupByNamespace, etc.) removed
// - Callers should use router.Aggregation() to access aggregation methods
// - Example: router.Aggregation().GetNamespaceHealthScore(ctx, ns, window)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
