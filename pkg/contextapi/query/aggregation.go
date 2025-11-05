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
// TDD REFACTOR: Helper Functions
// ========================================

// getCachedResult attempts to retrieve and unmarshal cached data
func (s *AggregationService) getCachedResult(ctx context.Context, cacheKey string, result interface{}) (bool, error) {
	cachedBytes, err := s.cache.Get(ctx, cacheKey)
	if err != nil || cachedBytes == nil {
		return false, nil
	}

	if err := json.Unmarshal(cachedBytes, result); err != nil {
		s.logger.Warn("Cache unmarshal error", zap.String("key", cacheKey), zap.Error(err))
		return false, nil
	}

	s.logger.Debug("Cache hit", zap.String("key", cacheKey))
	return true, nil
}

// setCachedResult stores result in cache with error logging
func (s *AggregationService) setCachedResult(ctx context.Context, cacheKey string, result interface{}) {
	if err := s.cache.Set(ctx, cacheKey, result); err != nil {
		s.logger.Warn("Cache set error", zap.String("key", cacheKey), zap.Error(err))
	}
}

// logCacheMiss logs cache miss with structured fields
func (s *AggregationService) logCacheMiss(fields ...zap.Field) {
	s.logger.Debug("Cache miss - calling Data Storage Service", fields...)
}

// ========================================
// BR-INTEGRATION-008: Incident-Type Success Rate
// ========================================

// GetSuccessRateByIncidentType retrieves success rate for a specific incident type with caching
// TDD REFACTOR: Extracted cache operations to helper functions
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

	// Check cache first (REFACTOR: using helper)
	var cachedResult dsmodels.IncidentTypeSuccessRateResponse
	if found, _ := s.getCachedResult(ctx, cacheKey, &cachedResult); found {
		return &cachedResult, nil
	}

	// Cache miss - call Data Storage Service (REFACTOR: using helper)
	s.logCacheMiss(
		zap.String("incident_type", incidentType),
		zap.String("time_range", timeRange),
		zap.Int("min_samples", minSamples))

	result, err := s.dataStorageClient.GetSuccessRateByIncidentType(ctx, incidentType, timeRange, minSamples)
	if err != nil {
		s.logger.Error("Data Storage Service error",
			zap.String("method", "GetSuccessRateByIncidentType"),
			zap.String("incident_type", incidentType),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get success rate from Data Storage: %w", err)
	}

	// Store in cache (REFACTOR: using helper)
	s.setCachedResult(ctx, cacheKey, result)

	return result, nil
}

// ========================================
// BR-INTEGRATION-009: Playbook Success Rate
// ========================================

// GetSuccessRateByPlaybook retrieves success rate for a specific playbook with caching
// TDD REFACTOR: Extracted cache operations to helper functions
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

	// Check cache first (REFACTOR: using helper)
	var cachedResult dsmodels.PlaybookSuccessRateResponse
	if found, _ := s.getCachedResult(ctx, cacheKey, &cachedResult); found {
		return &cachedResult, nil
	}

	// Cache miss - call Data Storage Service (REFACTOR: using helper)
	s.logCacheMiss(
		zap.String("playbook_id", playbookID),
		zap.String("playbook_version", playbookVersion),
		zap.String("time_range", timeRange),
		zap.Int("min_samples", minSamples))

	result, err := s.dataStorageClient.GetSuccessRateByPlaybook(ctx, playbookID, playbookVersion, timeRange, minSamples)
	if err != nil {
		s.logger.Error("Data Storage Service error",
			zap.String("method", "GetSuccessRateByPlaybook"),
			zap.String("playbook_id", playbookID),
			zap.String("playbook_version", playbookVersion),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get playbook success rate from Data Storage: %w", err)
	}

	// Store in cache (REFACTOR: using helper)
	s.setCachedResult(ctx, cacheKey, result)

	return result, nil
}

// ========================================
// BR-INTEGRATION-010: Multi-Dimensional Success Rate
// ========================================

// GetSuccessRateMultiDimensional retrieves success rate across multiple dimensions with caching
// TDD REFACTOR: Extracted cache operations to helper functions
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

	// Check cache first (REFACTOR: using helper)
	var cachedResult dsmodels.MultiDimensionalSuccessRateResponse
	if found, _ := s.getCachedResult(ctx, cacheKey, &cachedResult); found {
		return &cachedResult, nil
	}

	// Cache miss - call Data Storage Service (REFACTOR: using helper)
	s.logCacheMiss(
		zap.String("incident_type", query.IncidentType),
		zap.String("playbook_id", query.PlaybookID),
		zap.String("playbook_version", query.PlaybookVersion),
		zap.String("action_type", query.ActionType),
		zap.String("time_range", query.TimeRange),
		zap.Int("min_samples", query.MinSamples))

	result, err := s.dataStorageClient.GetSuccessRateMultiDimensional(ctx, query)
	if err != nil {
		s.logger.Error("Data Storage Service error",
			zap.String("method", "GetSuccessRateMultiDimensional"),
			zap.String("incident_type", query.IncidentType),
			zap.String("playbook_id", query.PlaybookID),
			zap.String("action_type", query.ActionType),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get multi-dimensional success rate from Data Storage: %w", err)
	}

	// Store in cache (REFACTOR: using helper)
	s.setCachedResult(ctx, cacheKey, result)

	return result, nil
}

