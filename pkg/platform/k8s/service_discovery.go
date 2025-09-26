package k8s

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// ServiceEventMetrics tracks structured metrics for service discovery events
// Business Requirement: BR-HOLMES-029 - Service discovery metrics and monitoring
// Following project guideline: use structured field values instead of interface{}
type ServiceEventMetrics struct {
	TotalEventsProcessed  int64            `json:"total_events_processed"`
	SuccessfulEvents      int64            `json:"successful_events"`
	DroppedEvents         int64            `json:"dropped_events"`
	EventTypeCounts       map[string]int64 `json:"event_type_counts"`
	AverageProcessingTime time.Duration    `json:"average_processing_time"`
	MaxProcessingTime     time.Duration    `json:"max_processing_time"`
	LastEventTimestamp    time.Time        `json:"last_event_timestamp"`
	DropRate              float64          `json:"drop_rate"`
	ProcessingLatencies   []time.Duration  `json:"processing_latencies"` // Recent latencies for calculation
}

// ServiceDiscovery provides dynamic service discovery for toolset configuration
// Business Requirement: BR-HOLMES-016 - Dynamic service discovery in Kubernetes cluster
type ServiceDiscovery struct {
	client          kubernetes.Interface
	cache           *ServiceCache
	detectors       map[string]ServiceDetector
	validators      []ServiceValidator
	eventChannel    chan ServiceEvent
	stopChannel     chan struct{}
	log             *logrus.Logger
	mu              sync.RWMutex
	discoveryConfig *ServiceDiscoveryConfig

	// Event metrics tracking - BR-HOLMES-029
	eventMetrics ServiceEventMetrics
	metricsMutex sync.RWMutex
}

// DetectedService represents a discovered service in the cluster
// Business Requirement: BR-HOLMES-017 - Service detection with metadata
type DetectedService struct {
	Name         string              `json:"name"`
	Namespace    string              `json:"namespace"`
	ServiceType  string              `json:"service_type"` // "prometheus", "grafana", "jaeger", etc.
	Endpoints    []ServiceEndpoint   `json:"endpoints"`
	Labels       map[string]string   `json:"labels"`
	Annotations  map[string]string   `json:"annotations"`
	Available    bool                `json:"available"`
	HealthStatus ServiceHealthStatus `json:"health_status"`
	LastChecked  time.Time           `json:"last_checked"`
	Priority     int                 `json:"priority"`
	Capabilities []string            `json:"capabilities"`
}

// ServiceEndpoint represents a service endpoint
type ServiceEndpoint struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Port     int32  `json:"port"`
	Protocol string `json:"protocol"`
	Path     string `json:"path,omitempty"`
}

// ServiceHealthStatus represents the health status of a service
type ServiceHealthStatus struct {
	Status        string    `json:"status"` // "healthy", "unhealthy", "unknown"
	LastCheck     time.Time `json:"last_check"`
	ResponseTime  int64     `json:"response_time"` // milliseconds
	ErrorMessage  string    `json:"error_message,omitempty"`
	CheckEndpoint string    `json:"check_endpoint"`
}

// ServiceEvent represents a service change event
type ServiceEvent struct {
	Type      string           `json:"type"` // "created", "updated", "deleted"
	Service   *DetectedService `json:"service"`
	Timestamp time.Time        `json:"timestamp"`
}

// ServiceDetector interface for service type detection
type ServiceDetector interface {
	Detect(ctx context.Context, service *corev1.Service) (*DetectedService, error)
	GetServiceType() string
	GetPriority() int
}

// ServiceValidator interface for service validation
type ServiceValidator interface {
	Validate(ctx context.Context, service *DetectedService) error
}

// ServiceDiscoveryConfig holds configuration for service discovery
// Business Requirement: BR-HOLMES-018 - Configurable service detection
type ServiceDiscoveryConfig struct {
	DiscoveryInterval   time.Duration             `yaml:"discovery_interval"`
	CacheTTL            time.Duration             `yaml:"cache_ttl"`
	HealthCheckInterval time.Duration             `yaml:"health_check_interval"`
	ServicePatterns     map[string]ServicePattern `yaml:"service_patterns"`
	Enabled             bool                      `yaml:"enabled"`
	Namespaces          []string                  `yaml:"namespaces"` // empty means all namespaces
}

// ServicePattern defines how to detect a specific service type
type ServicePattern struct {
	Enabled       bool                `yaml:"enabled"`
	Selectors     []map[string]string `yaml:"selectors"`
	ServiceNames  []string            `yaml:"service_names"`
	RequiredPorts []int32             `yaml:"required_ports"`
	HealthCheck   HealthCheckConfig   `yaml:"health_check"`
	Priority      int                 `yaml:"priority"`
	Capabilities  []string            `yaml:"capabilities"`
}

// HealthCheckConfig defines health check parameters
type HealthCheckConfig struct {
	Endpoint string        `yaml:"endpoint"`
	Timeout  time.Duration `yaml:"timeout"`
	Retries  int           `yaml:"retries"`
	Method   string        `yaml:"method"` // "GET", "POST", etc.
}

// NewServiceDiscovery creates a new service discovery instance
// Following development guideline: reuse existing UnifiedClient pattern
func NewServiceDiscovery(client kubernetes.Interface, config *ServiceDiscoveryConfig, log *logrus.Logger) *ServiceDiscovery {
	if config == nil {
		// Provide default configuration
		config = &ServiceDiscoveryConfig{
			DiscoveryInterval:   5 * time.Minute,
			CacheTTL:            10 * time.Minute,
			HealthCheckInterval: 30 * time.Second,
			Enabled:             true,
			ServicePatterns:     getDefaultServicePatterns(),
		}
	}

	sd := &ServiceDiscovery{
		client:          client,
		cache:           NewServiceCache(config.CacheTTL, config.CacheTTL*2),
		detectors:       make(map[string]ServiceDetector),
		validators:      make([]ServiceValidator, 0),
		eventChannel:    make(chan ServiceEvent, 100),
		stopChannel:     make(chan struct{}),
		log:             log,
		discoveryConfig: config,
		// Initialize metrics tracking - BR-HOLMES-029
		eventMetrics: ServiceEventMetrics{
			EventTypeCounts:     make(map[string]int64),
			ProcessingLatencies: make([]time.Duration, 0, 100), // Keep recent 100 latencies
		},
	}

	// Initialize default detectors
	// Business Requirement: BR-HOLMES-017 - Well-known service detection
	sd.registerDefaultDetectors()
	sd.registerDefaultValidators()

	return sd
}

// Start begins the service discovery process
// Business Requirement: BR-HOLMES-020 - Real-time service updates
func (sd *ServiceDiscovery) Start(ctx context.Context) error {
	if !sd.discoveryConfig.Enabled {
		sd.log.Info("Service discovery is disabled")
		return nil
	}

	sd.log.Info("Starting dynamic service discovery for toolset configuration")

	// Initial discovery scan
	if err := sd.performDiscoveryScan(ctx); err != nil {
		sd.log.WithError(err).Error("Initial service discovery scan failed")
		return fmt.Errorf("initial service discovery failed: %w", err)
	}

	// Start periodic discovery
	go sd.periodicDiscovery(ctx)

	// Start service watcher for real-time updates
	go sd.watchServices(ctx)

	// Start health monitoring
	go sd.healthMonitoring(ctx)

	sd.log.Info("Service discovery started successfully")
	return nil
}

// Stop stops the service discovery process
func (sd *ServiceDiscovery) Stop() {
	sd.log.Info("Stopping service discovery")

	// Safely close stop channel only once to prevent panic
	select {
	case <-sd.stopChannel:
		// Channel already closed
		sd.log.Debug("Service discovery already stopped")
		return
	default:
		// Channel not closed, safe to close it
		close(sd.stopChannel)
		sd.log.Debug("Service discovery stop channel closed")
	}
}

// GetDiscoveredServices returns all discovered services
// Business Requirement: BR-HOLMES-021 - Cached service discovery results
func (sd *ServiceDiscovery) GetDiscoveredServices() map[string]*DetectedService {
	return sd.cache.GetAllServices()
}

// GetServicesByType returns services of a specific type
func (sd *ServiceDiscovery) GetServicesByType(serviceType string) []*DetectedService {
	allServices := sd.cache.GetAllServices()
	var services []*DetectedService

	for _, service := range allServices {
		if service.ServiceType == serviceType {
			services = append(services, service)
		}
	}

	return services
}

// GetEventChannel returns the service event channel for real-time updates
func (sd *ServiceDiscovery) GetEventChannel() <-chan ServiceEvent {
	return sd.eventChannel
}

// GetEventMetrics returns structured event processing metrics
// Business Requirement: BR-HOLMES-029 - Service discovery metrics and monitoring
func (sd *ServiceDiscovery) GetEventMetrics() ServiceEventMetrics {
	sd.metricsMutex.RLock()
	defer sd.metricsMutex.RUnlock()

	// Calculate drop rate
	total := sd.eventMetrics.TotalEventsProcessed + sd.eventMetrics.DroppedEvents
	dropRate := 0.0
	if total > 0 {
		dropRate = float64(sd.eventMetrics.DroppedEvents) / float64(total)
	}

	// Calculate average processing time from recent latencies
	avgProcessingTime := time.Duration(0)
	if len(sd.eventMetrics.ProcessingLatencies) > 0 {
		var totalDuration time.Duration
		for _, latency := range sd.eventMetrics.ProcessingLatencies {
			totalDuration += latency
		}
		avgProcessingTime = totalDuration / time.Duration(len(sd.eventMetrics.ProcessingLatencies))
	}

	// Create a copy to avoid race conditions
	eventTypeCounts := make(map[string]int64)
	for k, v := range sd.eventMetrics.EventTypeCounts {
		eventTypeCounts[k] = v
	}

	return ServiceEventMetrics{
		TotalEventsProcessed:  sd.eventMetrics.TotalEventsProcessed,
		SuccessfulEvents:      sd.eventMetrics.SuccessfulEvents,
		DroppedEvents:         sd.eventMetrics.DroppedEvents,
		EventTypeCounts:       eventTypeCounts,
		AverageProcessingTime: avgProcessingTime,
		MaxProcessingTime:     sd.eventMetrics.MaxProcessingTime,
		LastEventTimestamp:    sd.eventMetrics.LastEventTimestamp,
		DropRate:              dropRate,
		ProcessingLatencies:   nil, // Don't expose internal latency slice
	}
}

// performDiscoveryScan performs a full service discovery scan
// Business Requirement: BR-HOLMES-016 - Cluster service discovery
func (sd *ServiceDiscovery) performDiscoveryScan(ctx context.Context) error {
	sd.log.Debug("Starting service discovery scan")

	// Safety check: ensure client is initialized before proceeding
	if sd.client == nil {
		sd.log.Error("Kubernetes client is not initialized")
		return fmt.Errorf("kubernetes client is not initialized - cannot perform service discovery")
	}

	namespaces := sd.discoveryConfig.Namespaces
	if len(namespaces) == 0 {
		// Get all namespaces
		nsList, err := sd.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if err != nil {
			sd.log.WithError(err).Error("Failed to list namespaces")
			return fmt.Errorf("failed to list namespaces: %w", err)
		}

		for _, ns := range nsList.Items {
			namespaces = append(namespaces, ns.Name)
		}
	}

	var discoveredServices []*DetectedService

	for _, namespace := range namespaces {
		services, err := sd.discoverServicesInNamespace(ctx, namespace)
		if err != nil {
			sd.log.WithError(err).WithField("namespace", namespace).Error("Failed to discover services in namespace")
			continue
		}
		discoveredServices = append(discoveredServices, services...)
	}

	// Update cache with discovered services
	sd.updateServiceCache(discoveredServices)

	sd.log.WithField("discovered_count", len(discoveredServices)).Info("Service discovery scan completed")
	return nil
}

// discoverServicesInNamespace discovers services in a specific namespace
func (sd *ServiceDiscovery) discoverServicesInNamespace(ctx context.Context, namespace string) ([]*DetectedService, error) {
	serviceList, err := sd.client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services in namespace %s: %w", namespace, err)
	}

	var discoveredServices []*DetectedService

	for _, service := range serviceList.Items {
		detectedService := sd.detectServiceType(ctx, &service)

		if detectedService != nil {
			discoveredServices = append(discoveredServices, detectedService)
		}
	}

	return discoveredServices, nil
}

// detectServiceType attempts to detect the service type using registered detectors
func (sd *ServiceDiscovery) detectServiceType(ctx context.Context, service *corev1.Service) *DetectedService {
	sd.mu.RLock()
	detectors := sd.detectors
	sd.mu.RUnlock()

	for _, detector := range detectors {
		detectedService, err := detector.Detect(ctx, service)
		if err != nil {
			sd.log.WithError(err).WithField("detector", detector.GetServiceType()).Debug("Detector failed")
			continue
		}

		if detectedService != nil {
			sd.log.WithFields(logrus.Fields{
				"service":      service.Name,
				"namespace":    service.Namespace,
				"service_type": detectedService.ServiceType,
			}).Debug("Service detected")

			// Validate the detected service
			if err := sd.validateService(ctx, detectedService); err != nil {
				sd.log.WithError(err).WithField("service", detectedService.Name).Debug("Service validation failed")
				continue
			}

			return detectedService
		}
	}

	return nil // Service not recognized by any detector
}

// validateService validates a detected service using registered validators
// Business Requirement: BR-HOLMES-019 - Service availability validation
func (sd *ServiceDiscovery) validateService(ctx context.Context, service *DetectedService) error {
	for _, validator := range sd.validators {
		if err := validator.Validate(ctx, service); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}
	return nil
}

// updateServiceCache updates the service cache with discovered services
func (sd *ServiceDiscovery) updateServiceCache(services []*DetectedService) {
	for _, service := range services {
		sd.cache.SetService(service)

		// Emit service event
		select {
		case sd.eventChannel <- ServiceEvent{
			Type:      "updated",
			Service:   service,
			Timestamp: time.Now(),
		}:
		default:
			sd.log.Warn("Service event channel is full, dropping event")
		}
	}
}

// registerDefaultDetectors registers the default service detectors
func (sd *ServiceDiscovery) registerDefaultDetectors() {
	sd.registerDetector(NewPrometheusDetector(sd.discoveryConfig.ServicePatterns["prometheus"], sd.log))
	sd.registerDetector(NewGrafanaDetector(sd.discoveryConfig.ServicePatterns["grafana"], sd.log))
	sd.registerDetector(NewJaegerDetector(sd.discoveryConfig.ServicePatterns["jaeger"], sd.log))
	sd.registerDetector(NewElasticsearchDetector(sd.discoveryConfig.ServicePatterns["elasticsearch"], sd.log))
	sd.registerDetector(NewCustomServiceDetector(sd.discoveryConfig.ServicePatterns["custom"], sd.log))
}

// registerDefaultValidators registers the default service validators
func (sd *ServiceDiscovery) registerDefaultValidators() {
	sd.validators = append(sd.validators, NewHealthValidator(sd.client, sd.log))
	sd.validators = append(sd.validators, NewEndpointValidator(sd.client, sd.log))
}

// registerDetector registers a service detector
func (sd *ServiceDiscovery) registerDetector(detector ServiceDetector) {
	if detector == nil {
		return
	}

	sd.mu.Lock()
	defer sd.mu.Unlock()

	sd.detectors[detector.GetServiceType()] = detector
	sd.log.WithField("service_type", detector.GetServiceType()).Debug("Registered service detector")
}

// periodicDiscovery performs periodic service discovery
func (sd *ServiceDiscovery) periodicDiscovery(ctx context.Context) {
	// Ensure minimum discovery interval to prevent ticker panic
	discoveryInterval := sd.discoveryConfig.DiscoveryInterval
	if discoveryInterval <= 0 {
		discoveryInterval = 5 * time.Minute // Default minimum discovery interval
	}

	ticker := time.NewTicker(discoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := sd.performDiscoveryScan(ctx); err != nil {
				sd.log.WithError(err).Error("Periodic service discovery scan failed")
			}
		case <-sd.stopChannel:
			return
		case <-ctx.Done():
			return
		}
	}
}

// watchServices watches for real-time service changes
// Business Requirement: BR-HOLMES-020 - Real-time toolset configuration updates
func (sd *ServiceDiscovery) watchServices(ctx context.Context) {
	for _, namespace := range sd.getWatchNamespaces() {
		go sd.watchServicesInNamespace(ctx, namespace)
	}
}

// watchServicesInNamespace watches services in a specific namespace
func (sd *ServiceDiscovery) watchServicesInNamespace(ctx context.Context, namespace string) {
	for {
		select {
		case <-sd.stopChannel:
			return
		case <-ctx.Done():
			return
		default:
		}

		watchInterface, err := sd.client.CoreV1().Services(namespace).Watch(ctx, metav1.ListOptions{
			Watch: true,
		})
		if err != nil {
			sd.log.WithError(err).WithField("namespace", namespace).Error("Failed to start service watch")
			time.Sleep(30 * time.Second) // Wait before retrying
			continue
		}

		sd.handleServiceWatchEvents(ctx, watchInterface, namespace)
		watchInterface.Stop()
	}
}

// handleServiceWatchEvents handles service watch events
func (sd *ServiceDiscovery) handleServiceWatchEvents(ctx context.Context, watchInterface watch.Interface, namespace string) {
	for {
		select {
		case event, ok := <-watchInterface.ResultChan():
			if !ok {
				sd.log.WithField("namespace", namespace).Debug("Service watch channel closed")
				return
			}

			service, ok := event.Object.(*corev1.Service)
			if !ok {
				continue
			}

			sd.handleServiceEvent(ctx, event.Type, service)

		case <-sd.stopChannel:
			return
		case <-ctx.Done():
			return
		}
	}
}

// handleServiceEvent handles a single service event
func (sd *ServiceDiscovery) handleServiceEvent(ctx context.Context, eventType watch.EventType, service *corev1.Service) {
	switch eventType {
	case watch.Added, watch.Modified:
		detectedService := sd.detectServiceType(ctx, service)

		if detectedService != nil {
			sd.cache.SetService(detectedService)

			eventTypeStr := "created"
			if eventType == watch.Modified {
				eventTypeStr = "updated"
			}

			select {
			case sd.eventChannel <- ServiceEvent{
				Type:      eventTypeStr,
				Service:   detectedService,
				Timestamp: time.Now(),
			}:
			default:
				sd.log.Warn("Service event channel is full, dropping event")
			}
		}

	case watch.Deleted:
		serviceKey := fmt.Sprintf("%s/%s", service.Namespace, service.Name)
		if cachedService := sd.cache.GetService(serviceKey); cachedService != nil {
			sd.cache.RemoveService(serviceKey)

			select {
			case sd.eventChannel <- ServiceEvent{
				Type:      "deleted",
				Service:   cachedService,
				Timestamp: time.Now(),
			}:
			default:
				sd.log.Warn("Service event channel is full, dropping event")
			}
		}
	}
}

// healthMonitoring performs periodic health monitoring of discovered services
// Business Requirement: BR-HOLMES-026 - Service discovery health checks
func (sd *ServiceDiscovery) healthMonitoring(ctx context.Context) {
	// Ensure minimum health check interval to prevent ticker panic
	healthCheckInterval := sd.discoveryConfig.HealthCheckInterval
	if healthCheckInterval <= 0 {
		healthCheckInterval = 60 * time.Second // Default minimum health check interval
	}

	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sd.performHealthChecks(ctx)
		case <-sd.stopChannel:
			return
		case <-ctx.Done():
			return
		}
	}
}

// performHealthChecks performs health checks on all discovered services
func (sd *ServiceDiscovery) performHealthChecks(ctx context.Context) {
	services := sd.cache.GetAllServices()

	for _, service := range services {
		if err := sd.checkServiceHealth(ctx, service); err != nil {
			sd.log.WithError(err).WithField("service", service.Name).Debug("Health check failed")
		}
	}
}

// checkServiceHealth checks the health of a specific service
func (sd *ServiceDiscovery) checkServiceHealth(ctx context.Context, service *DetectedService) error {
	// Find the service pattern configuration
	pattern, exists := sd.discoveryConfig.ServicePatterns[service.ServiceType]
	if !exists {
		return fmt.Errorf("no pattern configuration for service type: %s", service.ServiceType)
	}

	if pattern.HealthCheck.Endpoint == "" {
		// No health check configured
		service.HealthStatus = ServiceHealthStatus{
			Status:    "unknown",
			LastCheck: time.Now(),
		}
		return nil
	}

	// Perform health check
	start := time.Now()
	err := sd.performHealthCheckRequest(ctx, service, pattern.HealthCheck)
	responseTime := time.Since(start).Milliseconds()

	if err != nil {
		service.HealthStatus = ServiceHealthStatus{
			Status:        "unhealthy",
			LastCheck:     time.Now(),
			ResponseTime:  responseTime,
			ErrorMessage:  err.Error(),
			CheckEndpoint: pattern.HealthCheck.Endpoint,
		}
		service.Available = false
		return err
	}

	service.HealthStatus = ServiceHealthStatus{
		Status:        "healthy",
		LastCheck:     time.Now(),
		ResponseTime:  responseTime,
		CheckEndpoint: pattern.HealthCheck.Endpoint,
	}
	service.Available = true

	// Update cache
	sd.cache.SetService(service)
	return nil
}

// performHealthCheckRequest performs the actual HTTP health check request
func (sd *ServiceDiscovery) performHealthCheckRequest(ctx context.Context, service *DetectedService, healthCheck HealthCheckConfig) error {
	if len(service.Endpoints) == 0 {
		return fmt.Errorf("no endpoints available for service %s", service.Name)
	}

	// Use the first endpoint for health check
	endpoint := service.Endpoints[0]
	healthURL := fmt.Sprintf("%s%s", endpoint.URL, healthCheck.Endpoint)

	client := &http.Client{
		Timeout: healthCheck.Timeout,
	}

	method := healthCheck.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	// Guideline #6: Proper error handling - explicitly handle or log defer errors
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logrus.WithError(closeErr).Debug("Failed to close response body during health check")
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check failed with status: %d", resp.StatusCode)
	}

	return nil
}

// getWatchNamespaces returns the namespaces to watch for services
func (sd *ServiceDiscovery) getWatchNamespaces() []string {
	if len(sd.discoveryConfig.Namespaces) > 0 {
		return sd.discoveryConfig.Namespaces
	}
	return []string{metav1.NamespaceAll} // Watch all namespaces
}

// getDefaultServicePatterns returns the default service patterns
// Business Requirement: BR-HOLMES-017 - Well-known service detection patterns
func getDefaultServicePatterns() map[string]ServicePattern {
	return map[string]ServicePattern{
		"prometheus": {
			Enabled: true,
			Selectors: []map[string]string{
				{"app.kubernetes.io/name": "prometheus"},
				{"app": "prometheus"},
			},
			ServiceNames:  []string{"prometheus", "prometheus-server"},
			RequiredPorts: []int32{9090},
			HealthCheck: HealthCheckConfig{
				Endpoint: "/api/v1/status/buildinfo",
				Timeout:  2 * time.Second,
				Retries:  3,
				Method:   "GET",
			},
			Priority: 80,
			Capabilities: []string{
				"query_metrics",
				"alert_rules",
				"time_series",
				"resource_usage_analysis",
			},
		},
		"grafana": {
			Enabled: true,
			Selectors: []map[string]string{
				{"app.kubernetes.io/name": "grafana"},
			},
			ServiceNames:  []string{"grafana"},
			RequiredPorts: []int32{3000},
			HealthCheck: HealthCheckConfig{
				Endpoint: "/api/health",
				Timeout:  2 * time.Second,
				Retries:  3,
				Method:   "GET",
			},
			Priority: 70,
			Capabilities: []string{
				"get_dashboards",
				"query_datasource",
				"get_alerts",
				"visualization",
			},
		},
		"jaeger": {
			Enabled: true,
			Selectors: []map[string]string{
				{"app.kubernetes.io/name": "jaeger"},
			},
			ServiceNames:  []string{"jaeger-query"},
			RequiredPorts: []int32{16686},
			HealthCheck: HealthCheckConfig{
				Endpoint: "/api/services",
				Timeout:  2 * time.Second,
				Retries:  3,
				Method:   "GET",
			},
			Priority: 60,
			Capabilities: []string{
				"search_traces",
				"get_services",
				"analyze_latency",
				"distributed_tracing",
			},
		},
		"elasticsearch": {
			Enabled: true,
			Selectors: []map[string]string{
				{"app.kubernetes.io/name": "elasticsearch"},
			},
			ServiceNames:  []string{"elasticsearch"},
			RequiredPorts: []int32{9200},
			HealthCheck: HealthCheckConfig{
				Endpoint: "/_cluster/health",
				Timeout:  2 * time.Second,
				Retries:  3,
				Method:   "GET",
			},
			Priority: 50,
			Capabilities: []string{
				"search_logs",
				"analyze_patterns",
				"aggregation",
				"log_analysis",
			},
		},
		"custom": {
			Enabled:  true,
			Priority: 30,
		},
	}
}
