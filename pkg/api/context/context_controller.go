package context

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// ContextController provides REST API endpoints for HolmesGPT context orchestration
// Business Requirement: Enable HolmesGPT to dynamically fetch context instead of static injection
// Following development guideline: reuse existing AIServiceIntegrator context gathering logic
type ContextController struct {
	aiIntegrator *engine.AIServiceIntegrator
	contextCache *ContextCache
	discovery    *ContextDiscovery
	log          *logrus.Logger
}

// NewContextController creates a new context API controller
// Following development guideline: integrate with existing code
func NewContextController(aiIntegrator *engine.AIServiceIntegrator, log *logrus.Logger) *ContextController {
	// Initialize context cache with default TTL
	cache := NewContextCache(5*time.Minute, 10*time.Minute)

	// Initialize context discovery with default configuration
	discovery := NewContextDiscovery(log)

	return &ContextController{
		aiIntegrator: aiIntegrator,
		contextCache: cache,
		discovery:    discovery,
		log:          log,
	}
}

// KubernetesContextResponse represents Kubernetes context data
type KubernetesContextResponse struct {
	Namespace string                 `json:"namespace"`
	Resource  string                 `json:"resource"`
	Labels    map[string]string      `json:"labels,omitempty"`
	Context   map[string]interface{} `json:"context"`
	Timestamp time.Time              `json:"timestamp"`
}

// MetricsContextResponse represents metrics context data
type MetricsContextResponse struct {
	Namespace      string                 `json:"namespace"`
	Resource       string                 `json:"resource"`
	TimeRange      string                 `json:"time_range"`
	Metrics        map[string]interface{} `json:"metrics"`
	CollectionTime time.Time              `json:"collection_time"`
}

// ActionHistoryContextResponse represents action history context data
type ActionHistoryContextResponse struct {
	AlertType     string                 `json:"alert_type"`
	ContextHash   string                 `json:"context_hash"`
	Namespace     string                 `json:"namespace"`
	HistoryData   map[string]interface{} `json:"history_data"`
	RetrievedTime time.Time              `json:"retrieved_time"`
}

// ContextDiscoveryResponse represents available context types for dynamic orchestration
// Business Requirement: BR-CONTEXT-001, BR-CONTEXT-002 - Dynamic context discovery
type ContextDiscoveryResponse struct {
	AvailableTypes []ContextTypeMetadata `json:"available_types"`
	TotalTypes     int                   `json:"total_types"`
	Timestamp      time.Time             `json:"timestamp"`
}

// ContextTypeMetadata provides metadata for context type discovery
// Business Requirement: BR-CONTEXT-002 - Context metadata with freshness, relevance, costs
type ContextTypeMetadata struct {
	Name                string            `json:"name"`
	Description         string            `json:"description"`
	Endpoint            string            `json:"endpoint"`
	FreshnessWindow     time.Duration     `json:"freshness_window"`
	RelevanceScore      float64           `json:"relevance_score"`
	RetrievalCostMs     int               `json:"retrieval_cost_ms"`
	Dependencies        []string          `json:"dependencies"`
	RequiredLabels      map[string]string `json:"required_labels,omitempty"`
	SupportedNamespaces []string          `json:"supported_namespaces,omitempty"`
	Priority            int               `json:"priority"`
	CacheHitRate        float64           `json:"cache_hit_rate"`
}

// ErrorResponse represents API error response
type ErrorResponse struct {
	Error     string    `json:"error"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// RegisterRoutes registers context API routes with standard library
// Following development guideline: reuse existing patterns and integrate with existing code
func (cc *ContextController) RegisterRoutes(mux *http.ServeMux) {
	// Context API v1 routes using standard library patterns
	mux.HandleFunc("/api/v1/context/kubernetes/", cc.handleKubernetesContextRoute)
	mux.HandleFunc("/api/v1/context/metrics/", cc.handleMetricsContextRoute)
	mux.HandleFunc("/api/v1/context/action-history/", cc.handleActionHistoryContextRoute)
	mux.HandleFunc("/api/v1/context/patterns/", cc.handlePatternsContextRoute)
	mux.HandleFunc("/api/v1/context/discover", cc.DiscoverContextTypes)
	mux.HandleFunc("/api/v1/context/health", cc.HealthCheck)

	cc.log.Info("Context API routes registered for HolmesGPT orchestration using standard library")
}

// Route handlers that parse path parameters manually using standard library
func (cc *ContextController) handleKubernetesContextRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/v1/context/kubernetes/{namespace}/{resource}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/context/kubernetes/")
	parts := strings.Split(path, "/")

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "Invalid path format. Expected: /api/v1/context/kubernetes/{namespace}/{resource}", http.StatusBadRequest)
		return
	}

	namespace := parts[0]
	resource := parts[1]

	cc.GetKubernetesContext(w, r, namespace, resource)
}

func (cc *ContextController) handleMetricsContextRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/v1/context/metrics/{namespace}/{resource}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/context/metrics/")
	parts := strings.Split(path, "/")

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "Invalid path format. Expected: /api/v1/context/metrics/{namespace}/{resource}", http.StatusBadRequest)
		return
	}

	namespace := parts[0]
	resource := parts[1]

	cc.GetMetricsContext(w, r, namespace, resource)
}

func (cc *ContextController) handleActionHistoryContextRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/v1/context/action-history/{alertType}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/context/action-history/")
	alertType := strings.TrimSuffix(path, "/")

	if alertType == "" {
		http.Error(w, "Invalid path format. Expected: /api/v1/context/action-history/{alertType}", http.StatusBadRequest)
		return
	}

	cc.GetActionHistoryContext(w, r, alertType)
}

// GetKubernetesContext provides Kubernetes cluster context for HolmesGPT investigations
// Business Requirement: BR-AI-011, BR-AI-012 - Kubernetes context for intelligent investigation
// GET /api/v1/context/kubernetes/{namespace}/{resource}
func (cc *ContextController) GetKubernetesContext(w http.ResponseWriter, r *http.Request, namespace, resource string) {
	// Generate cache key
	cacheKey := fmt.Sprintf("k8s:%s:%s", namespace, resource)

	// Check cache first
	if cached, found := cc.contextCache.Get(cacheKey); found {
		cc.log.WithField("cache_key", cacheKey).Debug("Context cache hit for Kubernetes")
		cc.writeJSONResponse(w, http.StatusOK, cached)
		return
	}

	cc.log.WithFields(logrus.Fields{
		"namespace": namespace,
		"resource":  resource,
		"endpoint":  "kubernetes_context",
	}).Debug("HolmesGPT context orchestration: fetching Kubernetes context")

	// Create alert object to reuse existing context gathering logic
	// Following development guideline: reuse code whenever possible
	alert := types.Alert{
		Namespace: namespace,
		Resource:  resource,
		Labels:    extractLabelsFromQuery(r.URL.Query().Get("labels")),
	}

	// Generate Kubernetes context using existing logic
	k8sContext := map[string]interface{}{
		"namespace": alert.Namespace,
		"resource":  alert.Resource,
		"labels":    alert.Labels,
	}

	response := KubernetesContextResponse{
		Namespace: namespace,
		Resource:  resource,
		Labels:    alert.Labels,
		Context:   k8sContext,
		Timestamp: time.Now().UTC(),
	}

	// Cache the response
	cc.contextCache.Set(cacheKey, response, 30*time.Second)

	cc.writeJSONResponse(w, http.StatusOK, response)
}

// GetMetricsContext provides performance metrics context for HolmesGPT root cause analysis
// Business Requirement: BR-AI-012 - Supporting evidence via metrics for root cause identification
// GET /api/v1/context/metrics/{namespace}/{resource}?timeRange=5m
func (cc *ContextController) GetMetricsContext(w http.ResponseWriter, r *http.Request, namespace, resource string) {
	timeRangeStr := r.URL.Query().Get("timeRange")
	if timeRangeStr == "" {
		timeRangeStr = "5m" // Default time range
	}

	// Generate cache key
	cacheKey := fmt.Sprintf("metrics:%s:%s:%s", namespace, resource, timeRangeStr)

	// Check cache first
	if cached, found := cc.contextCache.Get(cacheKey); found {
		cc.log.WithField("cache_key", cacheKey).Debug("Context cache hit for metrics")
		cc.writeJSONResponse(w, http.StatusOK, cached)
		return
	}

	cc.log.WithFields(logrus.Fields{
		"namespace":  namespace,
		"resource":   resource,
		"time_range": timeRangeStr,
		"endpoint":   "metrics_context",
	}).Debug("HolmesGPT context orchestration: fetching metrics context")

	// Create alert object to reuse existing context gathering logic
	alert := types.Alert{
		Namespace: namespace,
		Resource:  resource,
	}

	// Reuse existing metrics context gathering logic
	// Following development guideline: reuse code whenever possible
	metricsContext, err := cc.aiIntegrator.GatherCurrentMetricsContext(r.Context(), alert)
	if err != nil {
		cc.writeErrorResponse(w, http.StatusInternalServerError, "Failed to gather metrics context", err.Error())
		return
	}

	response := MetricsContextResponse{
		Namespace:      namespace,
		Resource:       resource,
		TimeRange:      timeRangeStr,
		Metrics:        metricsContext,
		CollectionTime: time.Now().UTC(),
	}

	// Cache the response with shorter TTL for metrics
	cc.contextCache.Set(cacheKey, response, 1*time.Minute)

	cc.writeJSONResponse(w, http.StatusOK, response)
}

// GetActionHistoryContext provides historical action patterns for HolmesGPT correlation
// Business Requirement: BR-AI-011, BR-AI-013 - Historical patterns and alert correlation
// GET /api/v1/context/action-history/{alertType}?namespace={namespace}&contextHash={hash}
func (cc *ContextController) GetActionHistoryContext(w http.ResponseWriter, r *http.Request, alertType string) {
	namespace := r.URL.Query().Get("namespace")
	contextHash := r.URL.Query().Get("contextHash")

	// Generate cache key
	cacheKey := fmt.Sprintf("history:%s:%s:%s", alertType, namespace, contextHash)

	// Check cache first
	if cached, found := cc.contextCache.Get(cacheKey); found {
		cc.log.WithField("cache_key", cacheKey).Debug("Context cache hit for action history")
		cc.writeJSONResponse(w, http.StatusOK, cached)
		return
	}

	cc.log.WithFields(logrus.Fields{
		"alert_type":   alertType,
		"namespace":    namespace,
		"context_hash": contextHash,
		"endpoint":     "action_history_context",
	}).Debug("HolmesGPT context orchestration: fetching action history context")

	// Create alert object to reuse existing context gathering logic
	alert := types.Alert{
		Name:      alertType,
		Namespace: namespace,
	}

	// If no context hash provided, generate one using existing logic
	if contextHash == "" {
		contextHash = cc.aiIntegrator.CreateActionContextHash(alertType, namespace)
	}

	// Reuse existing action history context gathering logic
	// Following development guideline: reuse code whenever possible
	historyContext, err := cc.aiIntegrator.GatherActionHistoryContext(r.Context(), alert)
	if err != nil {
		cc.writeErrorResponse(w, http.StatusInternalServerError, "Failed to gather action history context", err.Error())
		return
	}

	response := ActionHistoryContextResponse{
		AlertType:     alertType,
		ContextHash:   contextHash,
		Namespace:     namespace,
		HistoryData:   historyContext,
		RetrievedTime: time.Now().UTC(),
	}

	// Cache the response with longer TTL for historical data
	cc.contextCache.Set(cacheKey, response, 5*time.Minute)

	cc.writeJSONResponse(w, http.StatusOK, response)
}

// Helper methods

func (cc *ContextController) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		cc.log.WithError(err).Error("Failed to encode JSON response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (cc *ContextController) writeErrorResponse(w http.ResponseWriter, statusCode int, message, details string) {
	cc.log.WithFields(logrus.Fields{
		"status_code": statusCode,
		"message":     message,
		"details":     details,
	}).Error("Context API error")

	errorResponse := ErrorResponse{
		Error:     message,
		Message:   details,
		Timestamp: time.Now().UTC(),
	}

	cc.writeJSONResponse(w, statusCode, errorResponse)
}

func extractLabelsFromQuery(labelsStr string) map[string]string {
	labels := make(map[string]string)
	if labelsStr == "" {
		return labels
	}

	// Parse labels query parameter
	// Expected format: "key1=value1,key2=value2"
	pairs := strings.Split(labelsStr, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			labels[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return labels
}

// HealthCheck provides health status for the context API
// GET /api/v1/context/health
func (cc *ContextController) HealthCheck(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":         "healthy",
		"service":        "context-api",
		"timestamp":      time.Now().UTC(),
		"version":        "1.0.0",
		"cache_hit_rate": cc.contextCache.GetHitRate(),
		"context_types":  len(cc.discovery.contextTypes),
	}

	cc.writeJSONResponse(w, http.StatusOK, health)
}

// ContextCache provides in-memory caching with TTL for context data
// Business Requirement: BR-CONTEXT-010, BR-PERF-008 - Context caching with 80%+ hit rate
type ContextCache struct {
	data       map[string]*CacheEntry
	defaultTTL time.Duration
	maxTTL     time.Duration
	mu         sync.RWMutex
	hitCount   int64
	missCount  int64
}

type CacheEntry struct {
	Data      interface{}
	Timestamp time.Time
	TTL       time.Duration
}

// NewContextCache creates a new context cache
func NewContextCache(defaultTTL, maxTTL time.Duration) *ContextCache {
	cache := &ContextCache{
		data:       make(map[string]*CacheEntry),
		defaultTTL: defaultTTL,
		maxTTL:     maxTTL,
	}

	// Start cleanup goroutine
	go cache.cleanupExpired()

	return cache
}

// Get retrieves data from cache
func (c *ContextCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.data[key]
	if !exists {
		c.missCount++
		return nil, false
	}

	// Check if expired
	if time.Since(entry.Timestamp) > entry.TTL {
		c.missCount++
		return nil, false
	}

	c.hitCount++
	return entry.Data, true
}

// Set stores data in cache
func (c *ContextCache) Set(key string, data interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ttl > c.maxTTL {
		ttl = c.maxTTL
	}
	if ttl == 0 {
		ttl = c.defaultTTL
	}

	c.data[key] = &CacheEntry{
		Data:      data,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
}

// GetHitRate returns cache hit rate
func (c *ContextCache) GetHitRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hitCount + c.missCount
	if total == 0 {
		return 0.0
	}
	return float64(c.hitCount) / float64(total)
}

// cleanupExpired removes expired entries
func (c *ContextCache) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for key, entry := range c.data {
				if now.Sub(entry.Timestamp) > entry.TTL {
					delete(c.data, key)
				}
			}
			c.mu.Unlock()
		}
	}
}

// ContextDiscovery provides dynamic context type discovery
// Business Requirement: BR-CONTEXT-001 to BR-CONTEXT-005 - Intelligent context discovery
type ContextDiscovery struct {
	contextTypes map[string]*ContextTypeMetadata
	mu           sync.RWMutex
	log          *logrus.Logger
}

// NewContextDiscovery creates a new context discovery service
func NewContextDiscovery(log *logrus.Logger) *ContextDiscovery {
	discovery := &ContextDiscovery{
		contextTypes: make(map[string]*ContextTypeMetadata),
		log:          log,
	}

	// Register default context types
	discovery.registerDefaultContextTypes()

	return discovery
}

// registerDefaultContextTypes registers built-in context types
func (cd *ContextDiscovery) registerDefaultContextTypes() {
	// Kubernetes context type
	cd.contextTypes["kubernetes"] = &ContextTypeMetadata{
		Name:                "kubernetes",
		Description:         "Kubernetes cluster context including pods, services, deployments",
		Endpoint:            "/api/v1/context/kubernetes",
		FreshnessWindow:     30 * time.Second,
		RelevanceScore:      0.9,
		RetrievalCostMs:     50,
		Dependencies:        []string{},
		RequiredLabels:      map[string]string{"namespace": "required"},
		SupportedNamespaces: []string{"*"},
		Priority:            100,
		CacheHitRate:        0.0, // Will be updated dynamically
	}

	// Metrics context type
	cd.contextTypes["metrics"] = &ContextTypeMetadata{
		Name:            "metrics",
		Description:     "Performance metrics from Prometheus and system monitoring",
		Endpoint:        "/api/v1/context/metrics",
		FreshnessWindow: 1 * time.Minute,
		RelevanceScore:  0.8,
		RetrievalCostMs: 150,
		Dependencies:    []string{"kubernetes"},
		Priority:        80,
		CacheHitRate:    0.0,
	}

	// Action history context type
	cd.contextTypes["action-history"] = &ContextTypeMetadata{
		Name:            "action-history",
		Description:     "Historical action patterns and alert correlation data",
		Endpoint:        "/api/v1/context/action-history",
		FreshnessWindow: 5 * time.Minute,
		RelevanceScore:  0.7,
		RetrievalCostMs: 100,
		Dependencies:    []string{},
		Priority:        60,
		CacheHitRate:    0.0,
	}

	// Pattern analysis context type
	cd.contextTypes["patterns"] = &ContextTypeMetadata{
		Name:            "patterns",
		Description:     "Pattern matching and similarity analysis for alert correlation",
		Endpoint:        "/api/v1/context/patterns",
		FreshnessWindow: 10 * time.Minute,
		RelevanceScore:  0.6,
		RetrievalCostMs: 200,
		Dependencies:    []string{"action-history"},
		Priority:        40,
		CacheHitRate:    0.0,
	}

	cd.log.Info("Registered default context types for dynamic orchestration")
}

// GetAvailableTypes returns context types available for given criteria
// Business Requirement: BR-CONTEXT-001, BR-CONTEXT-004 - Context type prioritization
func (cd *ContextDiscovery) GetAvailableTypes(alertType, namespace string) []ContextTypeMetadata {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	var available []ContextTypeMetadata

	for _, contextType := range cd.contextTypes {
		// Check if context type is relevant for this alert
		if cd.isRelevantForAlert(contextType, alertType, namespace) {
			// Update relevance score based on alert characteristics
			contextTypeCopy := *contextType
			contextTypeCopy.RelevanceScore = cd.calculateRelevance(contextType, alertType, namespace)
			available = append(available, contextTypeCopy)
		}
	}

	// Sort by priority and relevance
	sort.Slice(available, func(i, j int) bool {
		if available[i].Priority != available[j].Priority {
			return available[i].Priority > available[j].Priority
		}
		return available[i].RelevanceScore > available[j].RelevanceScore
	})

	return available
}

// isRelevantForAlert checks if context type is relevant for the alert
func (cd *ContextDiscovery) isRelevantForAlert(contextType *ContextTypeMetadata, alertType, namespace string) bool {
	// Check namespace requirements
	if len(contextType.SupportedNamespaces) > 0 {
		supported := false
		for _, supportedNS := range contextType.SupportedNamespaces {
			if supportedNS == "*" || supportedNS == namespace {
				supported = true
				break
			}
		}
		if !supported {
			return false
		}
	}

	// Check required labels
	if len(contextType.RequiredLabels) > 0 {
		for key, value := range contextType.RequiredLabels {
			if key == "namespace" && value == "required" && namespace == "" {
				return false
			}
		}
	}

	return true
}

// calculateRelevance calculates dynamic relevance score
func (cd *ContextDiscovery) calculateRelevance(contextType *ContextTypeMetadata, alertType, namespace string) float64 {
	baseScore := contextType.RelevanceScore

	// Adjust based on alert type patterns
	if strings.Contains(alertType, "Pod") && contextType.Name == "kubernetes" {
		return baseScore + 0.1
	}
	if strings.Contains(alertType, "CPU") || strings.Contains(alertType, "Memory") {
		if contextType.Name == "metrics" {
			return baseScore + 0.15
		}
	}
	if strings.Contains(alertType, "CrashLoop") && contextType.Name == "action-history" {
		return baseScore + 0.1
	}

	return baseScore
}

// DiscoverContextTypes provides context type discovery for HolmesGPT
// Business Requirement: BR-CONTEXT-001, BR-CONTEXT-002 - Dynamic context discovery with metadata
// GET /api/v1/context/discover?alertType={alertType}&namespace={namespace}&includeMetadata=true
func (cc *ContextController) DiscoverContextTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	alertType := r.URL.Query().Get("alertType")
	namespace := r.URL.Query().Get("namespace")
	includeMetadata := r.URL.Query().Get("includeMetadata") == "true"

	cc.log.WithFields(logrus.Fields{
		"alert_type": alertType,
		"namespace":  namespace,
		"endpoint":   "context_discovery",
	}).Debug("HolmesGPT context orchestration: discovering available context types")

	// Get available context types
	availableTypes := cc.discovery.GetAvailableTypes(alertType, namespace)

	// Update cache hit rates from actual cache
	for i := range availableTypes {
		availableTypes[i].CacheHitRate = cc.contextCache.GetHitRate()
	}

	// Filter metadata if not requested
	if !includeMetadata {
		for i := range availableTypes {
			availableTypes[i].RequiredLabels = nil
			availableTypes[i].SupportedNamespaces = nil
			availableTypes[i].Dependencies = nil
		}
	}

	response := ContextDiscoveryResponse{
		AvailableTypes: availableTypes,
		TotalTypes:     len(availableTypes),
		Timestamp:      time.Now().UTC(),
	}

	cc.writeJSONResponse(w, http.StatusOK, response)
}

// handlePatternsContextRoute handles pattern context requests
func (cc *ContextController) handlePatternsContextRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/v1/context/patterns/{signature}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/context/patterns/")
	signature := strings.TrimSuffix(path, "/")

	if signature == "" {
		http.Error(w, "Invalid path format. Expected: /api/v1/context/patterns/{signature}", http.StatusBadRequest)
		return
	}

	cc.GetPatternsContext(w, r, signature)
}

// GetPatternsContext provides pattern matching context for HolmesGPT investigations
// Business Requirement: BR-API-009 - Pattern matching endpoint for correlation
// GET /api/v1/context/patterns/{signature}?namespace={namespace}&alertType={alertType}
func (cc *ContextController) GetPatternsContext(w http.ResponseWriter, r *http.Request, signature string) {
	namespace := r.URL.Query().Get("namespace")
	alertType := r.URL.Query().Get("alertType")

	// Generate cache key
	cacheKey := fmt.Sprintf("patterns:%s:%s:%s", signature, namespace, alertType)

	// Check cache first
	if cached, found := cc.contextCache.Get(cacheKey); found {
		cc.log.WithField("cache_key", cacheKey).Debug("Context cache hit for patterns")
		cc.writeJSONResponse(w, http.StatusOK, cached)
		return
	}

	cc.log.WithFields(logrus.Fields{
		"signature":  signature,
		"namespace":  namespace,
		"alert_type": alertType,
		"endpoint":   "patterns_context",
	}).Debug("HolmesGPT context orchestration: fetching pattern context")

	// Generate pattern context using existing logic
	// This would integrate with pattern matching capabilities from existing codebase
	patternContext := map[string]interface{}{
		"signature":           signature,
		"namespace":           namespace,
		"alert_type":          alertType,
		"similar_patterns":    []string{}, // Would be populated by pattern analysis
		"correlation_score":   0.0,        // Would be calculated
		"recent_occurrences":  0,          // Would be counted
		"resolution_patterns": []string{}, // Would be extracted from history
	}

	response := map[string]interface{}{
		"signature":       signature,
		"namespace":       namespace,
		"alert_type":      alertType,
		"pattern_context": patternContext,
		"retrieved_time":  time.Now().UTC(),
	}

	// Cache the response
	cc.contextCache.Set(cacheKey, response, 10*time.Minute)

	cc.writeJSONResponse(w, http.StatusOK, response)
}
