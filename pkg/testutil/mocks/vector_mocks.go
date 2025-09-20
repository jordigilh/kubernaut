package mocks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// MockVectorDatabase implements vector.VectorDatabase interface for testing
type MockVectorDatabase struct {
	patterns map[string]*vector.ActionPattern
	mutex    sync.RWMutex

	// Operation tracking
	storeCalls               []StoreActionPatternCall
	findSimilarCalls         []FindSimilarPatternsCall
	updateEffectivenessCalls []UpdatePatternEffectivenessCall
	searchSemanticsCalls     []SearchBySemanticsCall
	searchVectorCalls        []SearchByVectorCall
	deleteCalls              []DeletePatternCall
	analyticsCalls           []GetPatternAnalyticsCall
	healthCheckCalls         []IsHealthyCall

	// Mock results
	storeResult              error
	findSimilarResult        []*vector.SimilarPattern
	findSimilarError         error
	updateEffectivenessError error
	searchSemanticsResult    []*vector.ActionPattern
	searchSemanticsError     error
	searchVectorResult       []*vector.ActionPattern
	searchVectorError        error
	deleteError              error
	analyticsResult          *vector.PatternAnalytics
	analyticsError           error
	healthCheckError         error
}

// Call tracking structures
type StoreActionPatternCall struct {
	Pattern *vector.ActionPattern
}

type FindSimilarPatternsCall struct {
	Pattern   *vector.ActionPattern
	Limit     int
	Threshold float64
}

type UpdatePatternEffectivenessCall struct {
	PatternID     string
	Effectiveness float64
}

type SearchBySemanticsCall struct {
	Query string
	Limit int
}

type SearchByVectorCall struct {
	Embedding []float64
	Limit     int
	Threshold float64
}

type DeletePatternCall struct {
	PatternID string
}

type GetPatternAnalyticsCall struct {
	Context context.Context
}

type IsHealthyCall struct {
	Context context.Context
}

// NewMockVectorDatabase creates a new mock vector database
func NewMockVectorDatabase() *MockVectorDatabase {
	return &MockVectorDatabase{
		patterns:                 make(map[string]*vector.ActionPattern),
		storeCalls:               []StoreActionPatternCall{},
		findSimilarCalls:         []FindSimilarPatternsCall{},
		updateEffectivenessCalls: []UpdatePatternEffectivenessCall{},
		searchSemanticsCalls:     []SearchBySemanticsCall{},
		searchVectorCalls:        []SearchByVectorCall{},
		deleteCalls:              []DeletePatternCall{},
		analyticsCalls:           []GetPatternAnalyticsCall{},
		healthCheckCalls:         []IsHealthyCall{},

		// Default success results
		storeResult:              nil,
		findSimilarResult:        []*vector.SimilarPattern{},
		findSimilarError:         nil,
		updateEffectivenessError: nil,
		searchSemanticsResult:    []*vector.ActionPattern{},
		searchSemanticsError:     nil,
		searchVectorResult:       []*vector.ActionPattern{},
		searchVectorError:        nil,
		deleteError:              nil,
		analyticsResult:          &vector.PatternAnalytics{},
		analyticsError:           nil,
		healthCheckError:         nil,
	}
}

// Mock result setters
func (m *MockVectorDatabase) SetStoreResult(err error) {
	m.storeResult = err
}

func (m *MockVectorDatabase) SetFindSimilarResult(patterns []*vector.SimilarPattern, err error) {
	m.findSimilarResult = patterns
	m.findSimilarError = err
}

func (m *MockVectorDatabase) SetUpdateEffectivenessError(err error) {
	m.updateEffectivenessError = err
}

func (m *MockVectorDatabase) SetSearchSemanticsResult(patterns []*vector.ActionPattern, err error) {
	m.searchSemanticsResult = patterns
	m.searchSemanticsError = err
}

func (m *MockVectorDatabase) SetSearchVectorResult(patterns []*vector.ActionPattern, err error) {
	m.searchVectorResult = patterns
	m.searchVectorError = err
}

func (m *MockVectorDatabase) SetDeleteError(err error) {
	m.deleteError = err
}

func (m *MockVectorDatabase) SetAnalyticsResult(analytics *vector.PatternAnalytics, err error) {
	m.analyticsResult = analytics
	m.analyticsError = err
}

func (m *MockVectorDatabase) SetHealthCheckError(err error) {
	m.healthCheckError = err
}

// Call tracking getters
func (m *MockVectorDatabase) GetStoreCalls() []StoreActionPatternCall {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return append([]StoreActionPatternCall{}, m.storeCalls...)
}

func (m *MockVectorDatabase) GetFindSimilarCalls() []FindSimilarPatternsCall {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return append([]FindSimilarPatternsCall{}, m.findSimilarCalls...)
}

func (m *MockVectorDatabase) GetUpdateEffectivenessCalls() []UpdatePatternEffectivenessCall {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return append([]UpdatePatternEffectivenessCall{}, m.updateEffectivenessCalls...)
}

func (m *MockVectorDatabase) GetSearchSemanticsCalls() []SearchBySemanticsCall {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return append([]SearchBySemanticsCall{}, m.searchSemanticsCalls...)
}

func (m *MockVectorDatabase) GetDeleteCalls() []DeletePatternCall {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return append([]DeletePatternCall{}, m.deleteCalls...)
}

func (m *MockVectorDatabase) GetAnalyticsCalls() []GetPatternAnalyticsCall {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return append([]GetPatternAnalyticsCall{}, m.analyticsCalls...)
}

func (m *MockVectorDatabase) GetHealthCheckCalls() []IsHealthyCall {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return append([]IsHealthyCall{}, m.healthCheckCalls...)
}

// vector.VectorDatabase interface implementation
func (m *MockVectorDatabase) StoreActionPattern(ctx context.Context, pattern *vector.ActionPattern) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.storeCalls = append(m.storeCalls, StoreActionPatternCall{Pattern: pattern})

	if m.storeResult != nil {
		return m.storeResult
	}

	// Store the pattern
	m.patterns[pattern.ID] = pattern
	return nil
}

func (m *MockVectorDatabase) FindSimilarPatterns(ctx context.Context, pattern *vector.ActionPattern, limit int, threshold float64) ([]*vector.SimilarPattern, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.findSimilarCalls = append(m.findSimilarCalls, FindSimilarPatternsCall{
		Pattern:   pattern,
		Limit:     limit,
		Threshold: threshold,
	})

	if m.findSimilarError != nil {
		return nil, m.findSimilarError
	}

	return m.findSimilarResult, nil
}

func (m *MockVectorDatabase) UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness float64) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.updateEffectivenessCalls = append(m.updateEffectivenessCalls, UpdatePatternEffectivenessCall{
		PatternID:     patternID,
		Effectiveness: effectiveness,
	})

	return m.updateEffectivenessError
}

func (m *MockVectorDatabase) SearchBySemantics(ctx context.Context, query string, limit int) ([]*vector.ActionPattern, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.searchSemanticsCalls = append(m.searchSemanticsCalls, SearchBySemanticsCall{
		Query: query,
		Limit: limit,
	})

	if m.searchSemanticsError != nil {
		return nil, m.searchSemanticsError
	}

	return m.searchSemanticsResult, nil
}

func (m *MockVectorDatabase) SearchByVector(ctx context.Context, embedding []float64, limit int, threshold float64) ([]*vector.ActionPattern, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.searchVectorCalls = append(m.searchVectorCalls, SearchByVectorCall{
		Embedding: embedding,
		Limit:     limit,
		Threshold: threshold,
	})

	if m.searchVectorError != nil {
		return nil, m.searchVectorError
	}

	return m.searchVectorResult, nil
}

func (m *MockVectorDatabase) DeletePattern(ctx context.Context, patternID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.deleteCalls = append(m.deleteCalls, DeletePatternCall{PatternID: patternID})

	if m.deleteError != nil {
		return m.deleteError
	}

	delete(m.patterns, patternID)
	return nil
}

func (m *MockVectorDatabase) GetPatternAnalytics(ctx context.Context) (*vector.PatternAnalytics, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.analyticsCalls = append(m.analyticsCalls, GetPatternAnalyticsCall{Context: ctx})

	if m.analyticsError != nil {
		return nil, m.analyticsError
	}

	return m.analyticsResult, nil
}

func (m *MockVectorDatabase) IsHealthy(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.healthCheckCalls = append(m.healthCheckCalls, IsHealthyCall{Context: ctx})

	return m.healthCheckError
}

// Additional helper methods for testing
func (m *MockVectorDatabase) ClearHistory() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.storeCalls = []StoreActionPatternCall{}
	m.findSimilarCalls = []FindSimilarPatternsCall{}
	m.updateEffectivenessCalls = []UpdatePatternEffectivenessCall{}
	m.searchSemanticsCalls = []SearchBySemanticsCall{}
	m.deleteCalls = []DeletePatternCall{}
	m.analyticsCalls = []GetPatternAnalyticsCall{}
	m.healthCheckCalls = []IsHealthyCall{}
}

func (m *MockVectorDatabase) GetStoredPatterns() map[string]*vector.ActionPattern {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	result := make(map[string]*vector.ActionPattern)
	for k, v := range m.patterns {
		result[k] = v
	}
	return result
}

// MockEmbeddingGenerator implements vector.EmbeddingGenerator for testing
type MockEmbeddingGenerator struct {
	dimension     int
	embeddings    map[string][]float64
	mutex         sync.RWMutex
	generateError error
	generateCalls []GenerateEmbeddingCall
}

type GenerateEmbeddingCall struct {
	Text    string
	Context string
}

// NewMockEmbeddingGenerator creates a new mock embedding generator
func NewMockEmbeddingGenerator(dimension int) *MockEmbeddingGenerator {
	return &MockEmbeddingGenerator{
		dimension:     dimension,
		embeddings:    make(map[string][]float64),
		generateCalls: []GenerateEmbeddingCall{},
	}
}

func (m *MockEmbeddingGenerator) SetGenerateError(err error) {
	m.generateError = err
}

func (m *MockEmbeddingGenerator) SetEmbedding(text string, embedding []float64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.embeddings[text] = embedding
}

func (m *MockEmbeddingGenerator) GetGenerateCalls() []GenerateEmbeddingCall {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return append([]GenerateEmbeddingCall{}, m.generateCalls...)
}

func (m *MockEmbeddingGenerator) GenerateEmbedding(ctx context.Context, text string, options map[string]interface{}) ([]float64, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	context := ""
	if ctx != nil {
		if ctxValue := ctx.Value("context"); ctxValue != nil {
			context = ctxValue.(string)
		}
	}

	m.generateCalls = append(m.generateCalls, GenerateEmbeddingCall{
		Text:    text,
		Context: context,
	})

	if m.generateError != nil {
		return nil, m.generateError
	}

	// Return preset embedding if available
	if embedding, exists := m.embeddings[text]; exists {
		return embedding, nil
	}

	// Generate default embedding
	embedding := make([]float64, m.dimension)
	for i := range embedding {
		embedding[i] = float64(i) * 0.1 // Simple deterministic pattern
	}

	return embedding, nil
}

func (m *MockEmbeddingGenerator) GetDimension() int {
	return m.dimension
}

func (m *MockEmbeddingGenerator) ClearHistory() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.generateCalls = []GenerateEmbeddingCall{}
}

// Add methods expected by storage tests

func (m *MockEmbeddingGenerator) GenerateTextEmbedding(ctx context.Context, text string) ([]float64, error) {
	mockCallMutex.Lock()
	mockTextEmbeddingCalls = append(mockTextEmbeddingCalls, EmbeddingCall{Text: text})
	mockCallMutex.Unlock()

	if mockTextEmbeddingError != nil {
		return nil, mockTextEmbeddingError
	}
	if mockTextEmbeddingResult != nil {
		return mockTextEmbeddingResult, nil
	}
	return m.GenerateEmbedding(ctx, text, nil)
}

func (m *MockEmbeddingGenerator) GenerateActionEmbedding(ctx context.Context, actionText string, options map[string]interface{}) ([]float64, error) {
	mockCallMutex.Lock()
	mockActionEmbeddingCalls = append(mockActionEmbeddingCalls, actionText)
	mockCallMutex.Unlock()

	if mockActionEmbeddingError != nil {
		return nil, mockActionEmbeddingError
	}
	if mockActionEmbeddingResult != nil {
		return mockActionEmbeddingResult, nil
	}
	return m.GenerateEmbedding(ctx, actionText, options)
}

func (m *MockEmbeddingGenerator) GenerateContextEmbedding(ctx context.Context, contextLabels map[string]string, options map[string]interface{}) ([]float64, error) {
	// Convert context labels to string for tracking
	contextStr := fmt.Sprintf("%v", contextLabels)

	mockCallMutex.Lock()
	mockContextEmbeddingCalls = append(mockContextEmbeddingCalls, contextStr)
	mockCallMutex.Unlock()

	if mockContextEmbeddingError != nil {
		return nil, mockContextEmbeddingError
	}
	if mockContextEmbeddingResult != nil {
		return mockContextEmbeddingResult, nil
	}
	return m.GenerateEmbedding(ctx, contextStr, options)
}

func (m *MockEmbeddingGenerator) GetEmbeddingDimension() int {
	return m.dimension
}

func (m *MockEmbeddingGenerator) CombineEmbeddings(embeddings ...[]float64) []float64 {
	if len(embeddings) == 0 {
		return nil
	}
	if len(embeddings) == 1 {
		return embeddings[0]
	}

	// For multiple embeddings, average them
	dimension := len(embeddings[0])
	result := make([]float64, dimension)
	for _, embedding := range embeddings {
		if len(embedding) != dimension {
			return nil
		}
		for i := range result {
			result[i] += embedding[i]
		}
	}

	// Average
	for i := range result {
		result[i] /= float64(len(embeddings))
	}
	return result
}

// Storage test-specific mock state
type EmbeddingCall struct {
	Text string
}

var (
	mockTextEmbeddingResult    []float64
	mockTextEmbeddingError     error
	mockActionEmbeddingResult  []float64
	mockActionEmbeddingError   error
	mockContextEmbeddingResult []float64
	mockContextEmbeddingError  error
	mockTextEmbeddingCalls     []EmbeddingCall
	mockActionEmbeddingCalls   []string
	mockContextEmbeddingCalls  []string
	mockCallMutex              sync.RWMutex
)

func (m *MockEmbeddingGenerator) SetTextEmbeddingResult(result []float64, err error) {
	mockCallMutex.Lock()
	defer mockCallMutex.Unlock()
	mockTextEmbeddingResult = result
	mockTextEmbeddingError = err
}

func (m *MockEmbeddingGenerator) SetActionEmbeddingResult(result []float64, err error) {
	mockCallMutex.Lock()
	defer mockCallMutex.Unlock()
	mockActionEmbeddingResult = result
	mockActionEmbeddingError = err
}

func (m *MockEmbeddingGenerator) SetContextEmbeddingResult(result []float64, err error) {
	mockCallMutex.Lock()
	defer mockCallMutex.Unlock()
	mockContextEmbeddingResult = result
	mockContextEmbeddingError = err
}

func (m *MockEmbeddingGenerator) GetTextEmbeddingCalls() []EmbeddingCall {
	mockCallMutex.RLock()
	defer mockCallMutex.RUnlock()
	result := make([]EmbeddingCall, len(mockTextEmbeddingCalls))
	copy(result, mockTextEmbeddingCalls)
	return result
}

func (m *MockEmbeddingGenerator) GetActionEmbeddingCalls() []string {
	mockCallMutex.RLock()
	defer mockCallMutex.RUnlock()
	result := make([]string, len(mockActionEmbeddingCalls))
	copy(result, mockActionEmbeddingCalls)
	return result
}

func (m *MockEmbeddingGenerator) GetContextEmbeddingCalls() []string {
	mockCallMutex.RLock()
	defer mockCallMutex.RUnlock()
	result := make([]string, len(mockContextEmbeddingCalls))
	copy(result, mockContextEmbeddingCalls)
	return result
}

// ResetGlobalMockState resets all global mock state variables
// This should be called in BeforeEach to avoid test interference
func ResetGlobalMockState() {
	mockCallMutex.Lock()
	defer mockCallMutex.Unlock()

	mockTextEmbeddingResult = nil
	mockTextEmbeddingError = nil
	mockActionEmbeddingResult = nil
	mockActionEmbeddingError = nil
	mockContextEmbeddingResult = nil
	mockContextEmbeddingError = nil
	mockTextEmbeddingCalls = []EmbeddingCall{}
	mockActionEmbeddingCalls = []string{}
	mockContextEmbeddingCalls = []string{}
}

// MockEmbeddingCache implements caching interface for testing
type MockEmbeddingCache struct {
	getCalls  []CacheGetCall
	setCalls  []CacheSetCall
	getResult []float64
	getFound  bool
	getError  error
	setError  error
	callMutex sync.RWMutex
}

type CacheGetCall struct {
	Key string
}

type CacheSetCall struct {
	Key        string
	Value      []float64
	Expiration time.Duration
	TTL        time.Duration // Alias for tests that expect TTL
}

// NewMockEmbeddingCache creates a new mock embedding cache
func NewMockEmbeddingCache() *MockEmbeddingCache {
	return &MockEmbeddingCache{
		getCalls: make([]CacheGetCall, 0),
		setCalls: make([]CacheSetCall, 0),
	}
}

func (m *MockEmbeddingCache) Get(ctx context.Context, key string) ([]float64, bool, error) {
	m.callMutex.Lock()
	m.getCalls = append(m.getCalls, CacheGetCall{Key: key})
	m.callMutex.Unlock()

	return m.getResult, m.getFound, m.getError
}

func (m *MockEmbeddingCache) Set(ctx context.Context, key string, value []float64, expiration time.Duration) error {
	m.callMutex.Lock()
	m.setCalls = append(m.setCalls, CacheSetCall{
		Key:        key,
		Value:      value,
		Expiration: expiration,
		TTL:        expiration, // Set TTL to same as Expiration for compatibility
	})
	m.callMutex.Unlock()

	return m.setError
}

func (m *MockEmbeddingCache) Delete(ctx context.Context, key string) error {
	// Mock implementation
	return nil
}

func (m *MockEmbeddingCache) Close() error {
	// Mock implementation - just clear everything
	return m.Clear(context.Background())
}

func (m *MockEmbeddingCache) GetStats(ctx context.Context) vector.CacheStats {
	// Mock implementation - return empty stats
	stats := vector.CacheStats{
		Hits:      0,
		Misses:    0,
		HitRate:   0.0,
		TotalKeys: 0,
		CacheType: "mock",
	}
	stats.CalculateHitRate()
	return stats
}

func (m *MockEmbeddingCache) Clear(ctx context.Context) error {
	m.callMutex.Lock()
	defer m.callMutex.Unlock()
	// Clear internal state
	m.getCalls = make([]CacheGetCall, 0)
	m.setCalls = make([]CacheSetCall, 0)
	return nil
}

func (m *MockEmbeddingCache) Reset() {
	m.callMutex.Lock()
	defer m.callMutex.Unlock()
	m.getCalls = make([]CacheGetCall, 0)
	m.setCalls = make([]CacheSetCall, 0)
	m.getResult = nil
	m.getFound = false
	m.getError = nil
	m.setError = nil
}

// Mock control methods
func (m *MockEmbeddingCache) SetGetResult(result []float64, found bool, err error) {
	m.getResult = result
	m.getFound = found
	m.getError = err
}

func (m *MockEmbeddingCache) SetSetResult(err error) {
	m.setError = err
}

func (m *MockEmbeddingCache) GetGetCalls() []CacheGetCall {
	m.callMutex.RLock()
	defer m.callMutex.RUnlock()
	result := make([]CacheGetCall, len(m.getCalls))
	copy(result, m.getCalls)
	return result
}

func (m *MockEmbeddingCache) GetSetCalls() []CacheSetCall {
	m.callMutex.RLock()
	defer m.callMutex.RUnlock()
	result := make([]CacheSetCall, len(m.setCalls))
	copy(result, m.setCalls)
	return result
}

// CreateTestActionPattern creates a simple test action pattern with just an ID
func CreateTestActionPattern(id string) *vector.ActionPattern {
	return &vector.ActionPattern{
		ID:               id,
		ActionType:       "scale_deployment",
		AlertName:        "HighMemoryUsage", // Updated to match business requirement tests
		AlertSeverity:    "critical",        // Updated to match business requirement tests
		Namespace:        "production",      // Updated to match business requirement tests
		ResourceType:     "deployment",      // Updated to match business requirement tests
		ResourceName:     "web-server",      // Updated to match business requirement tests
		ActionParameters: map[string]interface{}{"replicas": 3},
		ContextLabels:    map[string]string{"app": "test", "version": "v1.0"},
		PreConditions:    map[string]interface{}{"current_replicas": 1},
		PostConditions:   map[string]interface{}{"target_replicas": 3},
		Embedding:        []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		CreatedAt:        time.Now().Add(-time.Hour),
		UpdatedAt:        time.Now(),
		EffectivenessData: &vector.EffectivenessData{
			Score:                0.85,
			SuccessCount:         8,
			FailureCount:         2,
			AverageExecutionTime: 5 * time.Minute,
			SideEffectsCount:     0,
			RecurrenceRate:       0.1,
			ContextualFactors:    map[string]float64{"load": 0.7},
			LastAssessed:         time.Now().Add(-30 * time.Minute),
		},
		Metadata: map[string]interface{}{"test": true, "source": "mock_generator"},
	}
}
