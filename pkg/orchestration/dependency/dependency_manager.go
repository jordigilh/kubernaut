package orchestration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// DependencyManager manages external dependencies with health monitoring and fallback mechanisms
type DependencyManager struct {
	dependencies map[string]Dependency
	healthCheck  *DependencyHealthChecker
	fallbacks    map[string]FallbackProvider
	log          *logrus.Logger
	mu           sync.RWMutex
	config       *DependencyConfig
}

// Dependency represents an external dependency
type Dependency interface {
	Name() string
	Type() DependencyType
	IsHealthy(ctx context.Context) bool
	GetHealthStatus() *DependencyHealthStatus
	Connect(ctx context.Context) error
	Disconnect() error
	GetMetrics() *DependencyMetrics
}

// DependencyType defines the type of dependency
type DependencyType string

const (
	DependencyTypeVectorDB     DependencyType = "vector_database"
	DependencyTypePatternStore DependencyType = "pattern_store"
	DependencyTypeMLLibrary    DependencyType = "ml_library"
	DependencyTypeTimeSeriesDB DependencyType = "time_series_database"
	DependencyTypeCache        DependencyType = "cache"
)

// DependencyHealthStatus represents the health status of a dependency
type DependencyHealthStatus struct {
	Name             string                 `json:"name"`
	Type             DependencyType         `json:"type"`
	IsHealthy        bool                   `json:"is_healthy"`
	LastHealthCheck  time.Time              `json:"last_health_check"`
	ResponseTime     time.Duration          `json:"response_time"`
	ErrorRate        float64                `json:"error_rate"`
	ConnectionStatus ConnectionStatus       `json:"connection_status"`
	Details          map[string]interface{} `json:"details"`
	Issues           []string               `json:"issues"`
}

// ConnectionStatus represents the connection status to a dependency
type ConnectionStatus string

const (
	ConnectionStatusConnected    ConnectionStatus = "connected"
	ConnectionStatusDisconnected ConnectionStatus = "disconnected"
	ConnectionStatusDegraded     ConnectionStatus = "degraded"
	ConnectionStatusFailed       ConnectionStatus = "failed"
)

// DependencyMetrics contains metrics about dependency usage
type DependencyMetrics struct {
	TotalRequests       int64         `json:"total_requests"`
	SuccessfulRequests  int64         `json:"successful_requests"`
	FailedRequests      int64         `json:"failed_requests"`
	AverageResponseTime time.Duration `json:"average_response_time"`
	LastRequestTime     time.Time     `json:"last_request_time"`
	CircuitBreakerState string        `json:"circuit_breaker_state"`
}

// FallbackProvider provides fallback functionality when a dependency is unavailable
type FallbackProvider interface {
	Name() string
	CanHandle(dependencyType DependencyType) bool
	ProvideFallback(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error)
	GetFallbackMetrics() *FallbackMetrics
}

// FallbackMetrics contains metrics about fallback usage
type FallbackMetrics struct {
	TotalFallbacks      int64     `json:"total_fallbacks"`
	SuccessfulFallbacks int64     `json:"successful_fallbacks"`
	FailedFallbacks     int64     `json:"failed_fallbacks"`
	LastFallback        time.Time `json:"last_fallback"`
}

// DependencyConfig configures dependency management behavior
type DependencyConfig struct {
	HealthCheckInterval     time.Duration `yaml:"health_check_interval" default:"1m"`
	ConnectionTimeout       time.Duration `yaml:"connection_timeout" default:"10s"`
	MaxRetries              int           `yaml:"max_retries" default:"3"`
	CircuitBreakerThreshold float64       `yaml:"circuit_breaker_threshold" default:"0.5"`
	EnableFallbacks         bool          `yaml:"enable_fallbacks" default:"true"`
	FallbackTimeout         time.Duration `yaml:"fallback_timeout" default:"5s"`
}

// DependencyHealthChecker monitors dependency health
type DependencyHealthChecker struct {
	manager  *DependencyManager
	stopChan chan struct{}
	running  bool
	mu       sync.Mutex
	log      *logrus.Logger
}

// CircuitBreaker implements circuit breaker pattern for dependencies
type CircuitBreaker struct {
	name             string
	failureThreshold float64
	resetTimeout     time.Duration
	state            CircuitState
	failures         int64
	requests         int64
	lastFailureTime  time.Time
	mu               sync.RWMutex
}

// CircuitState represents circuit breaker states
type CircuitState string

const (
	CircuitStateClosed   CircuitState = "closed"
	CircuitStateOpen     CircuitState = "open"
	CircuitStateHalfOpen CircuitState = "half_open"
)

// Concrete dependency implementations

// VectorDatabaseDependency wraps vector database operations
type VectorDatabaseDependency struct {
	name           string
	impl           engine.VectorDatabase
	circuitBreaker *CircuitBreaker
	metrics        *DependencyMetrics
	log            *logrus.Logger
	mu             sync.RWMutex
}

// PatternStoreDependency wraps pattern store operations
type PatternStoreDependency struct {
	name           string
	impl           engine.PatternStore
	circuitBreaker *CircuitBreaker
	metrics        *DependencyMetrics
	log            *logrus.Logger
	mu             sync.RWMutex
}

func NewPatternStoreDependency(name string, impl engine.PatternStore, log *logrus.Logger) *PatternStoreDependency {
	return &PatternStoreDependency{
		name:           name,
		impl:           impl,
		circuitBreaker: NewCircuitBreaker(name, 0.5, 60*time.Second),
		metrics:        &DependencyMetrics{},
		log:            log,
	}
}

func (psd *PatternStoreDependency) Name() string         { return psd.name }
func (psd *PatternStoreDependency) Type() DependencyType { return DependencyTypePatternStore }

func (psd *PatternStoreDependency) IsHealthy(ctx context.Context) bool {
	// Simple health check - try to list patterns
	_, err := psd.impl.ListPatterns(ctx, "")
	return err == nil
}

func (psd *PatternStoreDependency) GetHealthStatus() *DependencyHealthStatus {
	psd.mu.RLock()
	defer psd.mu.RUnlock()

	return &DependencyHealthStatus{
		Name:             psd.name,
		Type:             DependencyTypePatternStore,
		IsHealthy:        psd.circuitBreaker.state == CircuitStateClosed,
		LastHealthCheck:  time.Now(),
		ErrorRate:        psd.calculateErrorRate(),
		ConnectionStatus: psd.getConnectionStatus(),
		Details: map[string]interface{}{
			"circuit_breaker_state": string(psd.circuitBreaker.state),
			"total_requests":        psd.metrics.TotalRequests,
		},
	}
}

func (psd *PatternStoreDependency) Connect(ctx context.Context) error {
	psd.log.WithField("dependency", psd.name).Info("Connecting to pattern store")
	return nil
}

func (psd *PatternStoreDependency) Disconnect() error {
	psd.log.WithField("dependency", psd.name).Info("Disconnecting from pattern store")
	return nil
}

func (psd *PatternStoreDependency) GetMetrics() *DependencyMetrics {
	psd.mu.RLock()
	defer psd.mu.RUnlock()
	return psd.metrics
}

func (psd *PatternStoreDependency) calculateErrorRate() float64 {
	if psd.metrics.TotalRequests == 0 {
		return 0.0
	}
	return float64(psd.metrics.FailedRequests) / float64(psd.metrics.TotalRequests)
}

func (psd *PatternStoreDependency) getConnectionStatus() ConnectionStatus {
	switch psd.circuitBreaker.state {
	case CircuitStateClosed:
		return ConnectionStatusConnected
	case CircuitStateOpen:
		return ConnectionStatusFailed
	case CircuitStateHalfOpen:
		return ConnectionStatusDegraded
	default:
		return ConnectionStatusDisconnected
	}
}

// Fallback implementations

// InMemoryVectorFallback provides in-memory fallback for vector operations
type InMemoryVectorFallback struct {
	storage map[string]*VectorEntry
	mu      sync.RWMutex
	metrics *FallbackMetrics
	log     *logrus.Logger
}

// VectorEntry represents a vector storage entry
type VectorEntry struct {
	ID       string                 `json:"id"`
	Vector   []float64              `json:"vector"`
	Metadata map[string]interface{} `json:"metadata"`
	Created  time.Time              `json:"created"`
}

// InMemoryPatternFallback provides in-memory fallback for pattern storage
type InMemoryPatternFallback struct {
	patterns map[string]*types.DiscoveredPattern
	mu       sync.RWMutex
	metrics  *FallbackMetrics
	log      *logrus.Logger
}

func (impf *InMemoryPatternFallback) Name() string { return "in_memory_pattern_fallback" }

func (impf *InMemoryPatternFallback) CanHandle(dependencyType DependencyType) bool {
	return dependencyType == DependencyTypePatternStore
}

func (impf *InMemoryPatternFallback) ProvideFallback(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error) {
	impf.mu.Lock()
	defer impf.mu.Unlock()

	impf.metrics.TotalFallbacks++
	impf.metrics.LastFallback = time.Now()

	switch operation {
	case "store":
		pattern := params["pattern"].(*types.DiscoveredPattern)
		impf.patterns[pattern.ID] = pattern
		impf.metrics.SuccessfulFallbacks++
		return nil, nil

	case "get":
		filters := params["filters"].(map[string]interface{})
		patterns := make([]*types.DiscoveredPattern, 0)

		// Simple filtering - in practice would be more sophisticated
		limit := 100
		if l, ok := filters["limit"]; ok {
			if limitInt, ok := l.(int); ok {
				limit = limitInt
			}
		}

		count := 0
		for _, pattern := range impf.patterns {
			if count >= limit {
				break
			}
			patterns = append(patterns, pattern)
			count++
		}

		impf.metrics.SuccessfulFallbacks++
		return patterns, nil

	case "delete":
		patternID := params["pattern_id"].(string)
		delete(impf.patterns, patternID)
		impf.metrics.SuccessfulFallbacks++
		return nil, nil

	default:
		impf.metrics.FailedFallbacks++
		return nil, fmt.Errorf("unsupported operation: %s", operation)
	}
}

func (impf *InMemoryPatternFallback) GetFallbackMetrics() *FallbackMetrics {
	impf.mu.RLock()
	defer impf.mu.RUnlock()
	return impf.metrics
}

// NewDependencyManager creates a new dependency manager
func NewDependencyManager(config *DependencyConfig, log *logrus.Logger) *DependencyManager {
	if config == nil {
		config = &DependencyConfig{
			HealthCheckInterval:     time.Minute,
			ConnectionTimeout:       10 * time.Second,
			MaxRetries:              3,
			CircuitBreakerThreshold: 0.5,
			EnableFallbacks:         true,
			FallbackTimeout:         5 * time.Second,
		}
	}

	dm := &DependencyManager{
		dependencies: make(map[string]Dependency),
		fallbacks:    make(map[string]FallbackProvider),
		log:          log,
		config:       config,
	}

	dm.healthCheck = NewDependencyHealthChecker(dm, log)
	dm.initializeFallbacks()

	return dm
}

// RegisterDependency registers a new dependency
func (dm *DependencyManager) RegisterDependency(dependency Dependency) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	name := dependency.Name()
	if _, exists := dm.dependencies[name]; exists {
		return fmt.Errorf("dependency '%s' already registered", name)
	}

	dm.dependencies[name] = dependency
	dm.log.WithFields(logrus.Fields{
		"name": name,
		"type": dependency.Type(),
	}).Info("Dependency registered")

	return nil
}

// GetDependency retrieves a dependency by name
func (dm *DependencyManager) GetDependency(name string) (Dependency, error) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	dependency, exists := dm.dependencies[name]
	if !exists {
		return nil, fmt.Errorf("dependency '%s' not found", name)
	}

	return dependency, nil
}

// GetVectorDB returns the vector database with fallback capability
func (dm *DependencyManager) GetVectorDB() engine.VectorDatabase {
	return &ManagedVectorDB{
		manager: dm,
		log:     dm.log,
	}
}

// GetPatternStore returns the pattern store with fallback capability
func (dm *DependencyManager) GetPatternStore() engine.PatternStore {
	return &ManagedPatternStore{
		manager: dm,
		log:     dm.log,
	}
}

// StartHealthMonitoring starts dependency health monitoring
func (dm *DependencyManager) StartHealthMonitoring(ctx context.Context) error {
	return dm.healthCheck.Start(ctx)
}

// StopHealthMonitoring stops dependency health monitoring
func (dm *DependencyManager) StopHealthMonitoring() {
	dm.healthCheck.Stop()
}

// GetHealthReport returns a comprehensive health report
func (dm *DependencyManager) GetHealthReport() *DependencyHealthReport {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	report := &DependencyHealthReport{
		Timestamp:           time.Now(),
		TotalDependencies:   len(dm.dependencies),
		HealthyDependencies: 0,
		DependencyStatus:    make(map[string]*DependencyHealthStatus),
		OverallHealthy:      true,
		FallbacksActive:     make([]string, 0),
	}

	for name, dependency := range dm.dependencies {
		status := dependency.GetHealthStatus()
		report.DependencyStatus[name] = status

		if status.IsHealthy {
			report.HealthyDependencies++
		} else {
			report.OverallHealthy = false
		}
	}

	// Check for active fallbacks
	for name, fallback := range dm.fallbacks {
		metrics := fallback.GetFallbackMetrics()
		if metrics.TotalFallbacks > 0 && time.Since(metrics.LastFallback) < time.Hour {
			report.FallbacksActive = append(report.FallbacksActive, name)
		}
	}

	return report
}

// Private methods

func (dm *DependencyManager) initializeFallbacks() {
	if !dm.config.EnableFallbacks {
		return
	}

	// Initialize in-memory vector fallback
	vectorFallback := &InMemoryVectorFallback{
		storage: make(map[string]*VectorEntry),
		metrics: &FallbackMetrics{},
		log:     dm.log,
	}
	dm.fallbacks["vector_fallback"] = vectorFallback

	// Initialize in-memory pattern fallback
	patternFallback := &InMemoryPatternFallback{
		patterns: make(map[string]*types.DiscoveredPattern),
		metrics:  &FallbackMetrics{},
		log:      dm.log,
	}
	dm.fallbacks["pattern_fallback"] = patternFallback

	dm.log.Info("Fallback providers initialized")
}

// VectorDatabaseDependency implementation

func NewVectorDatabaseDependency(name string, impl engine.VectorDatabase, log *logrus.Logger) *VectorDatabaseDependency {
	return &VectorDatabaseDependency{
		name:           name,
		impl:           impl,
		circuitBreaker: NewCircuitBreaker(name, 0.5, 60*time.Second),
		metrics:        &DependencyMetrics{},
		log:            log,
	}
}

func (vdd *VectorDatabaseDependency) Name() string         { return vdd.name }
func (vdd *VectorDatabaseDependency) Type() DependencyType { return DependencyTypeVectorDB }

func (vdd *VectorDatabaseDependency) IsHealthy(ctx context.Context) bool {
	// Simple health check - try a basic operation
	_, err := vdd.impl.Search(ctx, []float64{0.1, 0.2, 0.3}, 1)
	return err == nil
}

func (vdd *VectorDatabaseDependency) GetHealthStatus() *DependencyHealthStatus {
	vdd.mu.RLock()
	defer vdd.mu.RUnlock()

	return &DependencyHealthStatus{
		Name:             vdd.name,
		Type:             DependencyTypeVectorDB,
		IsHealthy:        vdd.circuitBreaker.state == CircuitStateClosed,
		LastHealthCheck:  time.Now(),
		ErrorRate:        vdd.calculateErrorRate(),
		ConnectionStatus: vdd.getConnectionStatus(),
		Details: map[string]interface{}{
			"circuit_breaker_state": string(vdd.circuitBreaker.state),
			"total_requests":        vdd.metrics.TotalRequests,
		},
	}
}

func (vdd *VectorDatabaseDependency) Connect(ctx context.Context) error {
	// Implementation would connect to actual vector database
	vdd.log.WithField("dependency", vdd.name).Info("Connecting to vector database")
	return nil
}

func (vdd *VectorDatabaseDependency) Disconnect() error {
	vdd.log.WithField("dependency", vdd.name).Info("Disconnecting from vector database")
	return nil
}

func (vdd *VectorDatabaseDependency) GetMetrics() *DependencyMetrics {
	vdd.mu.RLock()
	defer vdd.mu.RUnlock()
	return vdd.metrics
}

func (vdd *VectorDatabaseDependency) calculateErrorRate() float64 {
	if vdd.metrics.TotalRequests == 0 {
		return 0.0
	}
	return float64(vdd.metrics.FailedRequests) / float64(vdd.metrics.TotalRequests)
}

func (vdd *VectorDatabaseDependency) getConnectionStatus() ConnectionStatus {
	switch vdd.circuitBreaker.state {
	case CircuitStateClosed:
		return ConnectionStatusConnected
	case CircuitStateOpen:
		return ConnectionStatusFailed
	case CircuitStateHalfOpen:
		return ConnectionStatusDegraded
	default:
		return ConnectionStatusDisconnected
	}
}

// InMemoryVectorFallback implementation

func (imvf *InMemoryVectorFallback) Name() string { return "in_memory_vector_fallback" }

func (imvf *InMemoryVectorFallback) CanHandle(dependencyType DependencyType) bool {
	return dependencyType == DependencyTypeVectorDB
}

func (imvf *InMemoryVectorFallback) ProvideFallback(ctx context.Context, operation string, params map[string]interface{}) (interface{}, error) {
	imvf.mu.Lock()
	defer imvf.mu.Unlock()

	imvf.metrics.TotalFallbacks++
	imvf.metrics.LastFallback = time.Now()

	switch operation {
	case "store":
		id := params["id"].(string)
		vector := params["vector"].([]float64)
		metadata := params["metadata"].(map[string]interface{})

		entry := &VectorEntry{
			ID:       id,
			Vector:   vector,
			Metadata: metadata,
			Created:  time.Now(),
		}
		imvf.storage[id] = entry
		imvf.metrics.SuccessfulFallbacks++
		return nil, nil

	case "search":
		queryVector := params["vector"].([]float64)
		limit := params["limit"].(int)

		results := imvf.searchSimilar(queryVector, limit)
		imvf.metrics.SuccessfulFallbacks++
		return results, nil

	default:
		imvf.metrics.FailedFallbacks++
		return nil, fmt.Errorf("unsupported operation: %s", operation)
	}
}

func (imvf *InMemoryVectorFallback) GetFallbackMetrics() *FallbackMetrics {
	imvf.mu.RLock()
	defer imvf.mu.RUnlock()
	return imvf.metrics
}

func (imvf *InMemoryVectorFallback) searchSimilar(queryVector []float64, limit int) []*engine.VectorSearchResult {
	results := make([]*engine.VectorSearchResult, 0)

	for id, entry := range imvf.storage {
		similarity := imvf.calculateSimilarity(queryVector, entry.Vector)
		results = append(results, &engine.VectorSearchResult{
			ID:       id,
			Score:    similarity,
			Metadata: entry.Metadata,
		})
	}

	// Sort by similarity (descending) and limit results
	// Simple implementation - would use proper sorting in practice
	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

func (imvf *InMemoryVectorFallback) calculateSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	dot := 0.0
	normA := 0.0
	normB := 0.0

	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dot / (normA * normB)
}

// ManagedVectorDB provides vector database operations with fallback
type ManagedVectorDB struct {
	manager *DependencyManager
	log     *logrus.Logger
}

func (mvdb *ManagedVectorDB) Store(ctx context.Context, id string, vector []float64, metadata map[string]interface{}) error {
	// Try primary vector database first
	if dep, err := mvdb.manager.GetDependency("vector_db"); err == nil {
		if vdb, ok := dep.(*VectorDatabaseDependency); ok && vdb.IsHealthy(ctx) {
			return vdb.impl.Store(ctx, id, vector, metadata)
		}
	}

	// Use fallback
	if mvdb.manager.config.EnableFallbacks {
		fallback := mvdb.manager.fallbacks["vector_fallback"]
		params := map[string]interface{}{
			"id":       id,
			"vector":   vector,
			"metadata": metadata,
		}
		_, err := fallback.ProvideFallback(ctx, "store", params)
		if err == nil {
			mvdb.log.Debug("Used vector database fallback for store operation")
		}
		return err
	}

	return fmt.Errorf("vector database unavailable and fallback disabled")
}

func (mvdb *ManagedVectorDB) Search(ctx context.Context, vector []float64, limit int) ([]*engine.VectorSearchResult, error) {
	// Try primary vector database first
	if dep, err := mvdb.manager.GetDependency("vector_db"); err == nil {
		if vdb, ok := dep.(*VectorDatabaseDependency); ok && vdb.IsHealthy(ctx) {
			return vdb.impl.Search(ctx, vector, limit)
		}
	}

	// Use fallback
	if mvdb.manager.config.EnableFallbacks {
		fallback := mvdb.manager.fallbacks["vector_fallback"]
		params := map[string]interface{}{
			"vector": vector,
			"limit":  limit,
		}
		result, err := fallback.ProvideFallback(ctx, "search", params)
		if err == nil {
			mvdb.log.Debug("Used vector database fallback for search operation")
			return result.([]*engine.VectorSearchResult), nil
		}
	}

	return nil, fmt.Errorf("vector database unavailable and fallback disabled")
}

func (mvdb *ManagedVectorDB) Update(ctx context.Context, id string, vector []float64, metadata map[string]interface{}) error {
	// Similar implementation to Store
	return mvdb.Store(ctx, id, vector, metadata)
}

func (mvdb *ManagedVectorDB) Delete(ctx context.Context, id string) error {
	// Try primary vector database first
	if dep, err := mvdb.manager.GetDependency("vector_db"); err == nil {
		if vdb, ok := dep.(*VectorDatabaseDependency); ok && vdb.IsHealthy(ctx) {
			return vdb.impl.Delete(ctx, id)
		}
	}

	// For fallback, we don't support delete - return error
	return fmt.Errorf("vector database unavailable for delete operation")
}

// ManagedPatternStore provides pattern store operations with fallback
type ManagedPatternStore struct {
	manager *DependencyManager
	log     *logrus.Logger
}

func (mps *ManagedPatternStore) StorePattern(ctx context.Context, pattern *types.DiscoveredPattern) error {
	// Try primary pattern store first
	if dep, err := mps.manager.GetDependency("pattern_store"); err == nil {
		if psd, ok := dep.(*PatternStoreDependency); ok && psd.IsHealthy(ctx) {
			return psd.impl.StorePattern(ctx, pattern)
		}
	}

	// Use fallback
	if mps.manager.config.EnableFallbacks {
		fallback := mps.manager.fallbacks["pattern_fallback"]
		params := map[string]interface{}{
			"pattern": pattern,
		}
		_, err := fallback.ProvideFallback(ctx, "store", params)
		if err == nil {
			mps.log.Debug("Used pattern store fallback for store operation")
		}
		return err
	}

	return fmt.Errorf("pattern store unavailable and fallback disabled")
}

func (mps *ManagedPatternStore) ListPatterns(ctx context.Context, patternType string) ([]*types.DiscoveredPattern, error) {
	// Try primary pattern store first
	if dep, err := mps.manager.GetDependency("pattern_store"); err == nil {
		if psd, ok := dep.(*PatternStoreDependency); ok && psd.IsHealthy(ctx) {
			return psd.impl.ListPatterns(ctx, patternType)
		}
	}

	// Use fallback
	if mps.manager.config.EnableFallbacks {
		fallback := mps.manager.fallbacks["pattern_fallback"]
		params := map[string]interface{}{
			"filters": map[string]interface{}{"pattern_type": patternType},
		}
		result, err := fallback.ProvideFallback(ctx, "get", params)
		if err == nil {
			mps.log.Debug("Used pattern store fallback for get operation")
			return result.([]*types.DiscoveredPattern), nil
		}
	}

	return nil, fmt.Errorf("pattern store unavailable and fallback disabled")
}

func (mps *ManagedPatternStore) GetPattern(ctx context.Context, patternID string) (*types.DiscoveredPattern, error) {
	// Try primary pattern store first
	if dep, err := mps.manager.GetDependency("pattern_store"); err == nil {
		if psd, ok := dep.(*PatternStoreDependency); ok && psd.IsHealthy(ctx) {
			return psd.impl.GetPattern(ctx, patternID)
		}
	}

	// For fallback, we don't support get by ID - return error
	return nil, fmt.Errorf("pattern store unavailable for get operation")
}

func (mps *ManagedPatternStore) UpdatePattern(ctx context.Context, pattern *types.DiscoveredPattern) error {
	return mps.StorePattern(ctx, pattern) // Simplified implementation
}

func (mps *ManagedPatternStore) DeletePattern(ctx context.Context, patternID string) error {
	// Try primary pattern store first
	if dep, err := mps.manager.GetDependency("pattern_store"); err == nil {
		if psd, ok := dep.(*PatternStoreDependency); ok && psd.IsHealthy(ctx) {
			return psd.impl.DeletePattern(ctx, patternID)
		}
	}

	// Use fallback
	if mps.manager.config.EnableFallbacks {
		fallback := mps.manager.fallbacks["pattern_fallback"]
		params := map[string]interface{}{
			"pattern_id": patternID,
		}
		_, err := fallback.ProvideFallback(ctx, "delete", params)
		return err
	}

	return fmt.Errorf("pattern store unavailable and fallback disabled")
}

// Supporting types

type DependencyHealthReport struct {
	Timestamp           time.Time                          `json:"timestamp"`
	TotalDependencies   int                                `json:"total_dependencies"`
	HealthyDependencies int                                `json:"healthy_dependencies"`
	DependencyStatus    map[string]*DependencyHealthStatus `json:"dependency_status"`
	OverallHealthy      bool                               `json:"overall_healthy"`
	FallbacksActive     []string                           `json:"fallbacks_active"`
}

// CircuitBreaker implementation

func NewCircuitBreaker(name string, failureThreshold float64, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:             name,
		failureThreshold: failureThreshold,
		resetTimeout:     resetTimeout,
		state:            CircuitStateClosed,
	}
}

func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.requests++

	if cb.state == CircuitStateOpen {
		if time.Since(cb.lastFailureTime) < cb.resetTimeout {
			return fmt.Errorf("circuit breaker is open")
		}
		cb.state = CircuitStateHalfOpen
	}

	err := fn()
	if err != nil {
		cb.failures++
		cb.lastFailureTime = time.Now()

		if cb.state == CircuitStateHalfOpen || float64(cb.failures)/float64(cb.requests) >= cb.failureThreshold {
			cb.state = CircuitStateOpen
		}
		return err
	}

	if cb.state == CircuitStateHalfOpen {
		cb.state = CircuitStateClosed
		cb.failures = 0
	}

	return nil
}

// DependencyHealthChecker implementation

func NewDependencyHealthChecker(manager *DependencyManager, log *logrus.Logger) *DependencyHealthChecker {
	return &DependencyHealthChecker{
		manager:  manager,
		log:      log,
		stopChan: make(chan struct{}),
	}
}

func (dhc *DependencyHealthChecker) Start(ctx context.Context) error {
	dhc.mu.Lock()
	if dhc.running {
		dhc.mu.Unlock()
		return fmt.Errorf("health checker is already running")
	}
	dhc.running = true
	dhc.mu.Unlock()

	go dhc.healthCheckLoop(ctx)
	dhc.log.Info("Dependency health checker started")
	return nil
}

func (dhc *DependencyHealthChecker) Stop() {
	dhc.mu.Lock()
	if !dhc.running {
		dhc.mu.Unlock()
		return
	}
	dhc.running = false
	dhc.mu.Unlock()

	close(dhc.stopChan)
	dhc.log.Info("Dependency health checker stopped")
}

func (dhc *DependencyHealthChecker) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(dhc.manager.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-dhc.stopChan:
			return
		case <-ticker.C:
			dhc.performHealthChecks(ctx)
		}
	}
}

func (dhc *DependencyHealthChecker) performHealthChecks(ctx context.Context) {
	dhc.manager.mu.RLock()
	dependencies := make(map[string]Dependency)
	for name, dep := range dhc.manager.dependencies {
		dependencies[name] = dep
	}
	dhc.manager.mu.RUnlock()

	for name, dependency := range dependencies {
		go func(name string, dep Dependency) {
			healthy := dep.IsHealthy(ctx)
			dhc.log.WithFields(logrus.Fields{
				"dependency": name,
				"type":       dep.Type(),
				"healthy":    healthy,
			}).Debug("Health check completed")
		}(name, dependency)
	}
}
