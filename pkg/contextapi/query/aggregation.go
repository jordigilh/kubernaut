package query

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/contextapi/cache"
	"github.com/jordigilh/kubernaut/pkg/contextapi/datastorage"
	dsmodels "github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"go.uber.org/zap"
)

// ========================================
// BR-INTEGRATION-008, BR-INTEGRATION-009, BR-INTEGRATION-010
// AggregationService - Context API Aggregation Layer
// ========================================
//
// AggregationService aggregates success rate data from Data Storage Service
//
// ADR-033: Context API becomes Aggregation Layer between AI/LLM and Data Storage
// ADR-032: No direct PostgreSQL access - use Data Storage REST API
//
// TDD GREEN Phase: Minimal implementation to pass unit tests
// ========================================

// DataStorageClient defines the interface for Data Storage Service HTTP client
type DataStorageClient interface {
	GetSuccessRateByIncidentType(ctx context.Context, incidentType, timeRange string, minSamples int) (*dsmodels.IncidentTypeSuccessRateResponse, error)
	GetSuccessRateByPlaybook(ctx context.Context, playbookID, playbookVersion, timeRange string, minSamples int) (*dsmodels.PlaybookSuccessRateResponse, error)
	GetSuccessRateMultiDimensional(ctx context.Context, query *datastorage.MultiDimensionalQuery) (*dsmodels.MultiDimensionalSuccessRateResponse, error)
}

// AggregationService provides success rate aggregation with caching
type AggregationService struct {
	dataStorageClient DataStorageClient
	cache             cache.CacheManager
	logger            *zap.Logger
	defaultTTL        time.Duration
}

// NewAggregationService creates a new aggregation service
func NewAggregationService(
	dataStorageClient DataStorageClient,
	cacheManager cache.CacheManager,
	logger *zap.Logger,
) *AggregationService {
	return &AggregationService{
		dataStorageClient: dataStorageClient,
		cache:             cacheManager,
		logger:            logger,
		defaultTTL:        5 * time.Minute, // Default cache TTL
	}
}

// ========================================
// BR-INTEGRATION-008: Incident-Type Success Rate
// ========================================

// GetSuccessRateByIncidentType retrieves success rate for a specific incident type with caching
func (s *AggregationService) GetSuccessRateByIncidentType(
	ctx context.Context,
	incidentType string,
	timeRange string,
	minSamples int,
) (*dsmodels.IncidentTypeSuccessRateResponse, error) {
	// Validation
	if incidentType == "" {
		return nil, fmt.Errorf("incident_type cannot be empty")
	}

	// Build cache key
	cacheKey := fmt.Sprintf("incident_type:%s:%s:%d", incidentType, timeRange, minSamples)

	// Check cache first
	cachedBytes, err := s.cache.Get(ctx, cacheKey)
	if err == nil && cachedBytes != nil {
		var cachedResult dsmodels.IncidentTypeSuccessRateResponse
		if err := json.Unmarshal(cachedBytes, &cachedResult); err == nil {
			s.logger.Debug("Cache hit", zap.String("key", cacheKey))
			return &cachedResult, nil
		}
	}

	// Cache miss - call Data Storage Service
	s.logger.Debug("Cache miss - calling Data Storage Service",
		zap.String("incident_type", incidentType),
		zap.String("time_range", timeRange))

	result, err := s.dataStorageClient.GetSuccessRateByIncidentType(ctx, incidentType, timeRange, minSamples)
	if err != nil {
		return nil, fmt.Errorf("failed to get success rate from Data Storage: %w", err)
	}

	// Store in cache
	if err := s.cache.Set(ctx, cacheKey, result); err != nil {
		s.logger.Warn("Cache set error", zap.String("key", cacheKey), zap.Error(err))
	}

	return result, nil
}

// ========================================
// BR-INTEGRATION-009: Playbook Success Rate
// ========================================

// GetSuccessRateByPlaybook retrieves success rate for a specific playbook with caching
func (s *AggregationService) GetSuccessRateByPlaybook(
	ctx context.Context,
	playbookID string,
	playbookVersion string,
	timeRange string,
	minSamples int,
) (*dsmodels.PlaybookSuccessRateResponse, error) {
	// Validation
	if playbookID == "" {
		return nil, fmt.Errorf("playbook_id cannot be empty")
	}

	// Build cache key
	cacheKey := fmt.Sprintf("playbook:%s:%s:%s:%d", playbookID, playbookVersion, timeRange, minSamples)

	// Check cache first
	cachedBytes, err := s.cache.Get(ctx, cacheKey)
	if err == nil && cachedBytes != nil {
		var cachedResult dsmodels.PlaybookSuccessRateResponse
		if err := json.Unmarshal(cachedBytes, &cachedResult); err == nil {
			s.logger.Debug("Cache hit", zap.String("key", cacheKey))
			return &cachedResult, nil
		}
	}

	// Cache miss - call Data Storage Service
	s.logger.Debug("Cache miss - calling Data Storage Service",
		zap.String("playbook_id", playbookID),
		zap.String("playbook_version", playbookVersion))

	result, err := s.dataStorageClient.GetSuccessRateByPlaybook(ctx, playbookID, playbookVersion, timeRange, minSamples)
	if err != nil {
		return nil, fmt.Errorf("failed to get playbook success rate from Data Storage: %w", err)
	}

	// Store in cache
	if err := s.cache.Set(ctx, cacheKey, result); err != nil {
		s.logger.Warn("Cache set error", zap.String("key", cacheKey), zap.Error(err))
	}

	return result, nil
}

// ========================================
// BR-INTEGRATION-010: Multi-Dimensional Success Rate
// ========================================

// GetSuccessRateMultiDimensional retrieves success rate across multiple dimensions with caching
func (s *AggregationService) GetSuccessRateMultiDimensional(
	ctx context.Context,
	query *datastorage.MultiDimensionalQuery,
) (*dsmodels.MultiDimensionalSuccessRateResponse, error) {
	// Validation: at least one dimension must be specified
	if query.IncidentType == "" && query.PlaybookID == "" && query.ActionType == "" {
		return nil, fmt.Errorf("at least one dimension (incident_type, playbook_id, or action_type) must be specified")
	}

	// Build cache key
	cacheKey := fmt.Sprintf("multi:%s:%s:%s:%s:%s:%d",
		query.IncidentType,
		query.PlaybookID,
		query.PlaybookVersion,
		query.ActionType,
		query.TimeRange,
		query.MinSamples)

	// Check cache first
	cachedBytes, err := s.cache.Get(ctx, cacheKey)
	if err == nil && cachedBytes != nil {
		var cachedResult dsmodels.MultiDimensionalSuccessRateResponse
		if err := json.Unmarshal(cachedBytes, &cachedResult); err == nil {
			s.logger.Debug("Cache hit", zap.String("key", cacheKey))
			return &cachedResult, nil
		}
	}

	// Cache miss - call Data Storage Service
	s.logger.Debug("Cache miss - calling Data Storage Service",
		zap.String("incident_type", query.IncidentType),
		zap.String("playbook_id", query.PlaybookID),
		zap.String("action_type", query.ActionType))

	result, err := s.dataStorageClient.GetSuccessRateMultiDimensional(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get multi-dimensional success rate from Data Storage: %w", err)
	}

	// Store in cache
	if err := s.cache.Set(ctx, cacheKey, result); err != nil {
		s.logger.Warn("Cache set error", zap.String("key", cacheKey), zap.Error(err))
	}

	return result, nil
}

