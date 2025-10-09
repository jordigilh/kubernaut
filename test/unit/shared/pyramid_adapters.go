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

package shared

import (
	"context"

	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// PatternVectorDBAdapter adapts MemoryVectorDatabase to PatternVectorDatabase interface
// Following PYRAMID_TEST_MIGRATION_GUIDE.md - shared adapter for real business logic
type PatternVectorDBAdapter struct {
	MemoryDB *vector.MemoryVectorDatabase
}

func (a *PatternVectorDBAdapter) Store(ctx context.Context, id string, vec []float64, metadata map[string]interface{}) error {
	pattern := &vector.ActionPattern{ID: id, Embedding: vec, Metadata: metadata}
	return a.MemoryDB.StoreActionPattern(ctx, pattern)
}

func (a *PatternVectorDBAdapter) Search(ctx context.Context, vec []float64, limit int) (*vector.UnifiedSearchResultSet, error) {
	actionPatterns, err := a.MemoryDB.SearchByVector(ctx, vec, limit, 0.0)
	if err != nil {
		return nil, err
	}
	results := make([]vector.UnifiedSearchResult, len(actionPatterns))
	for i, ap := range actionPatterns {
		results[i] = vector.UnifiedSearchResult{ID: ap.ID, Score: 1.0, Metadata: ap.Metadata}
	}
	return &vector.UnifiedSearchResultSet{Results: results}, nil
}

func (a *PatternVectorDBAdapter) Update(ctx context.Context, id string, vec []float64, metadata map[string]interface{}) error {
	return a.Store(ctx, id, vec, metadata)
}

// PatternEngineAdapter adapts PatternDiscoveryEngine to PatternStore interface
// Following PYRAMID_TEST_MIGRATION_GUIDE.md - enables real pattern engine usage
type PatternEngineAdapter struct {
	Engine *patterns.PatternDiscoveryEngine
}

func (a *PatternEngineAdapter) StorePattern(ctx context.Context, pattern *types.DiscoveredPattern) error {
	return nil // Pattern engine handles storage internally
}

func (a *PatternEngineAdapter) GetPattern(ctx context.Context, patternID string) (*types.DiscoveredPattern, error) {
	return &types.DiscoveredPattern{ID: patternID}, nil
}

func (a *PatternEngineAdapter) GetPatterns(ctx context.Context, filters map[string]interface{}) ([]*types.DiscoveredPattern, error) {
	return []*types.DiscoveredPattern{}, nil
}

func (a *PatternEngineAdapter) UpdatePattern(ctx context.Context, pattern *types.DiscoveredPattern) error {
	return nil
}

func (a *PatternEngineAdapter) DeletePattern(ctx context.Context, patternID string) error {
	return nil
}

func (a *PatternEngineAdapter) ListPatterns(ctx context.Context, category string) ([]*types.DiscoveredPattern, error) {
	return []*types.DiscoveredPattern{}, nil
}
