package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/jordigilh/kubernaut/pkg/toolset"
	"github.com/jordigilh/kubernaut/pkg/toolset/configmap"
	"github.com/jordigilh/kubernaut/pkg/toolset/discovery"
	"github.com/jordigilh/kubernaut/pkg/toolset/generator"
	"github.com/jordigilh/kubernaut/pkg/toolset/metrics"
	"github.com/jordigilh/kubernaut/pkg/toolset/server/middleware"
)

// Config holds server configuration
// BR-TOOLSET-033: HTTP server configuration
type Config struct {
	Port              int
	MetricsPort       int
	ShutdownTimeout   time.Duration
	DiscoveryInterval time.Duration
}

// Server represents the Dynamic Toolset HTTP server
// BR-TOOLSET-033: HTTP server with authentication
type Server struct {
	config         *Config
	httpServer     *http.Server
	metricsServer  *http.Server
	mux            *http.ServeMux
	clientset      kubernetes.Interface
	discoverer     discovery.ServiceDiscoverer
	generator      generator.ToolsetGenerator
	configBuilder  configmap.ConfigMapBuilder
	authMiddleware *middleware.AuthMiddleware
}

// NewServer creates a new HTTP server
func NewServer(config *Config, clientset kubernetes.Interface) (*Server, error) {
	s := &Server{
		config:         config,
		clientset:      clientset,
		discoverer:     discovery.NewServiceDiscoverer(clientset),
		generator:      generator.NewHolmesGPTGenerator(),
		configBuilder:  configmap.NewConfigMapBuilder("kubernaut-toolset-config", "kubernaut-system"),
		authMiddleware: middleware.NewAuthMiddleware(clientset),
		mux:            http.NewServeMux(),
	}

	// Register detectors
	s.discoverer.RegisterDetector(discovery.NewPrometheusDetector())
	s.discoverer.RegisterDetector(discovery.NewGrafanaDetector())
	s.discoverer.RegisterDetector(discovery.NewJaegerDetector())
	s.discoverer.RegisterDetector(discovery.NewElasticsearchDetector())
	s.discoverer.RegisterDetector(discovery.NewCustomDetector())

	// Setup routes
	s.setupRoutes()

	// Create HTTP servers
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: s.mux,
	}

	// Create separate metrics server on different port with authentication
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", s.authMiddleware.Middleware(http.HandlerFunc(s.handleMetrics)))

	s.metricsServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", config.MetricsPort),
		Handler: metricsMux,
	}

	return s, nil
}

// RegisterDetector registers a custom detector (primarily for testing)
// This allows tests to inject mock detectors with custom health checkers
func (s *Server) RegisterDetector(detector discovery.ServiceDetector) {
	s.discoverer.RegisterDetector(detector)
}

// setupRoutes configures HTTP routes
func (s *Server) setupRoutes() {
	// Public endpoints (no auth)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/ready", s.handleReady)

	// Protected API endpoints (with auth)
	apiMux := http.NewServeMux()
	// Note: More specific routes must be registered first
	apiMux.HandleFunc("/api/v1/toolsets/validate", s.handleValidateToolset) // BR-TOOLSET-042: Validate toolset
	apiMux.HandleFunc("/api/v1/toolsets/generate", s.handleGenerateToolset) // BR-TOOLSET-041: Generate toolset
	apiMux.HandleFunc("/api/v1/toolsets/", s.handleToolsetsRouter)          // BR-TOOLSET-040: Router for list and get operations
	apiMux.HandleFunc("/api/v1/services", s.handleListServices)
	apiMux.HandleFunc("/api/v1/discover", s.handleDiscover)

	// Apply auth middleware to API routes
	s.mux.Handle("/api/", s.authMiddleware.Middleware(apiMux))

	// Note: Metrics endpoint is on separate server with auth (see NewServer)
}

// Start starts the HTTP server and metrics server
func (s *Server) Start(ctx context.Context) error {
	// Start discovery loop in background
	go func() {
		_ = s.discoverer.Start(ctx)
	}()

	// Start metrics server in background
	go func() {
		if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Metrics server error: %v\n", err)
		}
	}()

	// Start main HTTP server (blocking)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down both servers
func (s *Server) Shutdown(ctx context.Context) error {
	// Stop discovery loop
	if err := s.discoverer.Stop(); err != nil {
		return fmt.Errorf("failed to stop discoverer: %w", err)
	}

	// Shutdown metrics server
	if err := s.metricsServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown metrics server: %w", err)
	}

	// Shutdown main HTTP server
	return s.httpServer.Shutdown(ctx)
}

// ServeHTTP implements http.Handler for testing
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status": "ok",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleReady handles readiness check requests
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	// Check Kubernetes API connectivity
	_, err := s.clientset.Discovery().ServerVersion()
	k8sReady := err == nil

	response := map[string]interface{}{
		"kubernetes": k8sReady,
	}

	status := http.StatusOK
	if !k8sReady {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}

// handleListToolsets handles GET /api/v1/toolsets with optional filtering
// BR-TOOLSET-037: Renamed from handleGetToolset to match api-specification.md (plural)
// BR-TOOLSET-039: List toolsets with query parameter filtering
func (s *Server) handleListToolsets(w http.ResponseWriter, r *http.Request) {
	timer := prometheus.NewTimer(metrics.APIRequestDuration.WithLabelValues("/api/v1/toolsets", "GET"))
	defer timer.ObserveDuration()

	if r.Method != http.MethodGet {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets", "GET", "405").Inc()
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters for filtering
	enabledFilter, err := parseOptionalBool(r.URL.Query().Get("enabled"))
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets", "GET", "400").Inc()
		http.Error(w, "Invalid enabled parameter", http.StatusBadRequest)
		return
	}

	healthyFilter, err := parseOptionalBool(r.URL.Query().Get("healthy"))
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets", "GET", "400").Inc()
		http.Error(w, "Invalid healthy parameter", http.StatusBadRequest)
		return
	}

	// Discover services
	services, err := s.discoverer.DiscoverServices(r.Context())
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets", "GET", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolsets", "discovery_failed").Inc()
		http.Error(w, "Failed to discover services", http.StatusInternalServerError)
		return
	}

	// Convert to toolset responses
	toolsets := s.servicesToToolsets(services)

	// Apply filters
	filtered := s.filterToolsets(toolsets, enabledFilter, healthyFilter)

	// Build response
	response := toolset.ToolsetsListResponse{
		Toolsets:      filtered,
		Total:         len(filtered),
		LastDiscovery: time.Now().Format(time.RFC3339),
	}

	metrics.APIRequests.WithLabelValues("/api/v1/toolsets", "GET", "200").Inc()
	metrics.ToolsInToolset.Set(float64(len(filtered)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleToolsetsRouter routes between list and get operations
// BR-TOOLSET-040: Route toolsets API calls based on path
func (s *Server) handleToolsetsRouter(w http.ResponseWriter, r *http.Request) {
	// Extract path after /api/v1/toolsets/
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/toolsets")
	path = strings.Trim(path, "/")

	if path == "" {
		// GET /api/v1/toolsets - list all
		s.handleListToolsets(w, r)
		return
	}

	// GET /api/v1/toolsets/{name} - get specific toolset
	s.handleGetToolset(w, r, path)
}

// handleGetToolset gets a specific toolset by name or type
// BR-TOOLSET-040: Get toolset by name
func (s *Server) handleGetToolset(w http.ResponseWriter, r *http.Request, name string) {
	timer := prometheus.NewTimer(metrics.APIRequestDuration.WithLabelValues("/api/v1/toolsets/{name}", "GET"))
	defer timer.ObserveDuration()

	if r.Method != http.MethodGet {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/{name}", "GET", "405").Inc()
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Discover services
	services, err := s.discoverer.DiscoverServices(r.Context())
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/{name}", "GET", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolsets/{name}", "discovery_failed").Inc()
		http.Error(w, "Failed to discover services", http.StatusInternalServerError)
		return
	}

	// Convert to toolset responses
	toolsets := s.servicesToToolsets(services)

	// Find matching toolset (prefer name match, fallback to type match)
	for _, t := range toolsets {
		if t.Name == name || t.Type == name {
			metrics.APIRequests.WithLabelValues("/api/v1/toolsets/{name}", "GET", "200").Inc()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(t)
			return
		}
	}

	// Not found
	metrics.APIRequests.WithLabelValues("/api/v1/toolsets/{name}", "GET", "404").Inc()
	http.Error(w, fmt.Sprintf("Toolset %s not found", name), http.StatusNotFound)
}

// handleGenerateToolset handles POST /api/v1/toolsets/generate
// BR-TOOLSET-041: Generate toolset with discovery and ConfigMap update
func (s *Server) handleGenerateToolset(w http.ResponseWriter, r *http.Request) {
	timer := prometheus.NewTimer(metrics.APIRequestDuration.WithLabelValues("/api/v1/toolsets/generate", "POST"))
	defer timer.ObserveDuration()

	if r.Method != http.MethodPost {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "405").Inc()
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Trigger service discovery
	services, err := s.discoverer.DiscoverServices(r.Context())
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolsets/generate", "discovery_failed").Inc()
		http.Error(w, "Failed to discover services", http.StatusInternalServerError)
		return
	}

	// Convert to toolset responses
	toolsets := s.servicesToToolsets(services)

	// Generate toolset JSON
	servicePointers := make([]*toolset.DiscoveredService, len(services))
	for i := range services {
		servicePointers[i] = &services[i]
	}

	toolsetJSON, err := s.generator.GenerateToolset(r.Context(), servicePointers)
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolsets/generate", "generation_failed").Inc()
		http.Error(w, "Failed to generate toolset", http.StatusInternalServerError)
		return
	}

	// Build ConfigMap
	cm, err := s.configBuilder.BuildConfigMap(r.Context(), toolsetJSON)
	if err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "500").Inc()
		metrics.APIErrors.WithLabelValues("/api/v1/toolsets/generate", "configmap_failed").Inc()
		http.Error(w, "Failed to build ConfigMap", http.StatusInternalServerError)
		return
	}

	// Create or update ConfigMap in cluster
	existingCM, err := s.clientset.CoreV1().ConfigMaps(cm.Namespace).Get(r.Context(), cm.Name, metav1.GetOptions{})
	if err != nil {
		// ConfigMap doesn't exist, create it
		cm, err = s.clientset.CoreV1().ConfigMaps(cm.Namespace).Create(r.Context(), cm, metav1.CreateOptions{})
		if err != nil {
			metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "500").Inc()
			metrics.APIErrors.WithLabelValues("/api/v1/toolsets/generate", "configmap_create_failed").Inc()
			http.Error(w, "Failed to create ConfigMap", http.StatusInternalServerError)
			return
		}
	} else {
		// ConfigMap exists, update it
		cm.ResourceVersion = existingCM.ResourceVersion
		cm, err = s.clientset.CoreV1().ConfigMaps(cm.Namespace).Update(r.Context(), cm, metav1.UpdateOptions{})
		if err != nil {
			metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "500").Inc()
			metrics.APIErrors.WithLabelValues("/api/v1/toolsets/generate", "configmap_update_failed").Inc()
			http.Error(w, "Failed to update ConfigMap", http.StatusInternalServerError)
			return
		}
	}

	// Build response
	response := toolset.ToolsetsListResponse{
		Toolsets:         toolsets,
		Total:            len(toolsets),
		LastDiscovery:    time.Now().Format(time.RFC3339),
		ConfigMapVersion: cm.ResourceVersion,
	}

	metrics.APIRequests.WithLabelValues("/api/v1/toolsets/generate", "POST", "200").Inc()
	metrics.ToolsInToolset.Set(float64(len(toolsets)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleListServices handles GET /api/v1/services
// BR-TOOLSET-034: List discovered services
func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Discover services
	services, err := s.discoverer.DiscoverServices(r.Context())
	if err != nil {
		http.Error(w, "Failed to discover services", http.StatusInternalServerError)
		return
	}

	// Filter by type if specified
	serviceType := r.URL.Query().Get("type")
	if serviceType != "" {
		filtered := []toolset.DiscoveredService{}
		for _, svc := range services {
			if svc.Type == serviceType {
				filtered = append(filtered, svc)
			}
		}
		services = filtered
	}

	response := map[string]interface{}{
		"services": services,
		"count":    len(services),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleDiscover handles POST /api/v1/discover
// BR-TOOLSET-034: Trigger discovery manually
func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Trigger discovery (async)
	go func() {
		_, _ = s.discoverer.DiscoverServices(context.Background())
	}()

	response := map[string]interface{}{
		"message": "Discovery triggered successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)
}

// handleMetrics handles GET /metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

// handleValidateToolset handles POST /api/v1/toolsets/validate
// BR-TOOLSET-042: Validate toolset JSON structure
func (s *Server) handleValidateToolset(w http.ResponseWriter, r *http.Request) {
	timer := prometheus.NewTimer(metrics.APIRequestDuration.WithLabelValues("/api/v1/toolsets/validate", "POST"))
	defer timer.ObserveDuration()

	if r.Method != http.MethodPost {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/validate", "POST", "405").Inc()
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	var toolsetData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&toolsetData); err != nil {
		metrics.APIRequests.WithLabelValues("/api/v1/toolsets/validate", "POST", "400").Inc()
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate toolset structure
	validationErrors := validateToolsetStructure(toolsetData)

	// Build response
	response := toolset.ValidationResponse{
		Valid:  len(validationErrors) == 0,
		Errors: validationErrors,
	}

	metrics.APIRequests.WithLabelValues("/api/v1/toolsets/validate", "POST", "200").Inc()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// parseOptionalBool parses an optional boolean query parameter
// BR-TOOLSET-039: Query parameter parsing for filtering
func parseOptionalBool(value string) (*bool, error) {
	if value == "" {
		return nil, nil
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// servicesToToolsets converts discovered services to toolset responses
// BR-TOOLSET-039: Convert services to API response format
func (s *Server) servicesToToolsets(services []toolset.DiscoveredService) []toolset.ToolsetResponse {
	toolsets := make([]toolset.ToolsetResponse, 0, len(services))
	for _, svc := range services {
		toolsets = append(toolsets, toolset.ToolsetResponse{
			Name:            svc.Name,
			Type:            svc.Type,
			Enabled:         true, // All discovered services are enabled
			Healthy:         svc.Healthy,
			ServiceEndpoint: svc.Endpoint,
			DiscoveredAt:    svc.DiscoveredAt.Format(time.RFC3339),
			LastHealthCheck: svc.LastCheck.Format(time.RFC3339),
			Config: map[string]interface{}{
				"url": svc.Endpoint,
			},
		})
	}
	return toolsets
}

// filterToolsets applies enabled and healthy filters to toolset list
// BR-TOOLSET-039: Filter toolsets by enabled and healthy status
func (s *Server) filterToolsets(toolsets []toolset.ToolsetResponse, enabled, healthy *bool) []toolset.ToolsetResponse {
	filtered := make([]toolset.ToolsetResponse, 0, len(toolsets))
	for _, t := range toolsets {
		// Apply enabled filter
		if enabled != nil && t.Enabled != *enabled {
			continue
		}
		// Apply healthy filter
		if healthy != nil && t.Healthy != *healthy {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

// validateToolsetStructure validates toolset JSON structure
// BR-TOOLSET-042: Toolset validation logic
func validateToolsetStructure(data map[string]interface{}) []toolset.ValidationError {
	// Validate tools array exists and is valid
	tools, err := extractToolsArray(data)
	if err != nil {
		return []toolset.ValidationError{*err}
	}

	// Validate individual tools
	return validateTools(tools)
}

// extractToolsArray extracts and validates the tools array from toolset data
func extractToolsArray(data map[string]interface{}) ([]interface{}, *toolset.ValidationError) {
	// Check if tools array exists
	toolsInterface, ok := data["tools"]
	if !ok {
		return nil, &toolset.ValidationError{
			Field:   "tools",
			Message: "tools array is required",
		}
	}

	// Check if tools is an array
	tools, ok := toolsInterface.([]interface{})
	if !ok {
		return nil, &toolset.ValidationError{
			Field:   "tools",
			Message: "tools must be an array",
		}
	}

	// Check if tools array is empty
	if len(tools) == 0 {
		return nil, &toolset.ValidationError{
			Field:   "tools",
			Message: "tools array cannot be empty",
		}
	}

	return tools, nil
}

// validateTools validates each tool in the tools array
func validateTools(tools []interface{}) []toolset.ValidationError {
	var errors []toolset.ValidationError
	toolNames := make(map[string]bool)

	for i, toolInterface := range tools {
		tool, ok := toolInterface.(map[string]interface{})
		if !ok {
			errors = append(errors, toolset.ValidationError{
				Field:   fmt.Sprintf("tools[%d]", i),
				Message: "tool must be an object",
			})
			continue
		}

		// Validate individual tool fields
		toolErrors := validateTool(tool, i, toolNames)
		errors = append(errors, toolErrors...)
	}

	return errors
}

// validateTool validates a single tool's fields
func validateTool(tool map[string]interface{}, index int, seenNames map[string]bool) []toolset.ValidationError {
	var errors []toolset.ValidationError

	// Validate name
	if err := validateToolName(tool, index, seenNames); err != nil {
		errors = append(errors, *err)
	}

	// Validate type
	if err := validateToolType(tool, index); err != nil {
		errors = append(errors, *err)
	}

	// Validate endpoint
	if err := validateToolEndpoint(tool, index); err != nil {
		errors = append(errors, *err)
	}

	return errors
}

// validateToolName validates the tool name field
func validateToolName(tool map[string]interface{}, index int, seenNames map[string]bool) *toolset.ValidationError {
	name, ok := tool["name"].(string)
	if !ok || name == "" {
		return &toolset.ValidationError{
			Field:   fmt.Sprintf("tools[%d].name", index),
			Message: "name is required and must be a non-empty string",
		}
	}

	// Check for duplicate names
	if seenNames[name] {
		return &toolset.ValidationError{
			Field:   fmt.Sprintf("tools[%d].name", index),
			Message: fmt.Sprintf("duplicate tool name: %s", name),
		}
	}

	seenNames[name] = true
	return nil
}

// validateToolType validates the tool type field
func validateToolType(tool map[string]interface{}, index int) *toolset.ValidationError {
	toolType, ok := tool["type"].(string)
	if !ok || toolType == "" {
		return &toolset.ValidationError{
			Field:   fmt.Sprintf("tools[%d].type", index),
			Message: "type is required and must be a non-empty string",
		}
	}
	return nil
}

// validateToolEndpoint validates the tool endpoint field
func validateToolEndpoint(tool map[string]interface{}, index int) *toolset.ValidationError {
	endpoint, ok := tool["endpoint"].(string)
	if !ok || endpoint == "" {
		return &toolset.ValidationError{
			Field:   fmt.Sprintf("tools[%d].endpoint", index),
			Message: "endpoint is required and must be a non-empty string",
		}
	}

	// Validate endpoint is a valid URL
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return &toolset.ValidationError{
			Field:   fmt.Sprintf("tools[%d].endpoint", index),
			Message: "endpoint must be a valid HTTP or HTTPS URL",
		}
	}

	return nil
}
