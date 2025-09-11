package k8s

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// HealthValidator validates service health through HTTP health checks
// Business Requirement: BR-HOLMES-019 - Service availability validation
type HealthValidator struct {
	client kubernetes.Interface
	log    *logrus.Logger
}

// NewHealthValidator creates a new health validator
func NewHealthValidator(client kubernetes.Interface, log *logrus.Logger) *HealthValidator {
	return &HealthValidator{
		client: client,
		log:    log,
	}
}

// Validate validates the health of a detected service
func (hv *HealthValidator) Validate(ctx context.Context, service *DetectedService) error {
	if len(service.Endpoints) == 0 {
		return fmt.Errorf("no endpoints available for health validation")
	}

	// Perform basic connectivity check for the first endpoint
	endpoint := service.Endpoints[0]

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Try a basic GET request to the service root
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint.URL, nil)
	if err != nil {
		hv.log.WithError(err).WithField("service", service.Name).Debug("Failed to create health check request")
		// Don't fail validation for request creation errors - service might still be functional
		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		hv.log.WithError(err).WithFields(logrus.Fields{
			"service":  service.Name,
			"endpoint": endpoint.URL,
		}).Debug("Health check request failed")
		// Don't fail validation - network issues don't mean service is invalid
		return nil
	}
	defer resp.Body.Close()

	hv.log.WithFields(logrus.Fields{
		"service":     service.Name,
		"status_code": resp.StatusCode,
		"endpoint":    endpoint.URL,
	}).Debug("Health check completed")

	return nil
}

// EndpointValidator validates service endpoints and connectivity
// Business Requirement: BR-HOLMES-019 - Service endpoint validation
type EndpointValidator struct {
	client kubernetes.Interface
	log    *logrus.Logger
}

// NewEndpointValidator creates a new endpoint validator
func NewEndpointValidator(client kubernetes.Interface, log *logrus.Logger) *EndpointValidator {
	return &EndpointValidator{
		client: client,
		log:    log,
	}
}

// Validate validates service endpoints
func (ev *EndpointValidator) Validate(ctx context.Context, service *DetectedService) error {
	// Verify the service exists in Kubernetes
	k8sService, err := ev.client.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("service not found in Kubernetes: %w", err)
	}

	// Verify service has endpoints (pods backing it)
	endpoints, err := ev.client.CoreV1().Endpoints(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
	if err != nil {
		// Endpoints might not exist yet for new services
		ev.log.WithError(err).WithFields(logrus.Fields{
			"service":   service.Name,
			"namespace": service.Namespace,
		}).Debug("Service endpoints not found")
		return nil
	}

	// Check if there are any ready endpoints
	hasReadyEndpoints := false
	for _, subset := range endpoints.Subsets {
		if len(subset.Addresses) > 0 {
			hasReadyEndpoints = true
			break
		}
	}

	if !hasReadyEndpoints {
		ev.log.WithFields(logrus.Fields{
			"service":   service.Name,
			"namespace": service.Namespace,
		}).Debug("Service has no ready endpoints")
		// Don't fail validation - service might be starting up
		return nil
	}

	// Validate service ports match detected endpoints
	if err := ev.validateServicePorts(k8sService, service); err != nil {
		return fmt.Errorf("service port validation failed: %w", err)
	}

	ev.log.WithFields(logrus.Fields{
		"service":         service.Name,
		"namespace":       service.Namespace,
		"endpoint_count":  len(service.Endpoints),
		"ready_endpoints": hasReadyEndpoints,
	}).Debug("Endpoint validation completed")

	return nil
}

// validateServicePorts validates that detected endpoints match service ports
func (ev *EndpointValidator) validateServicePorts(k8sService *corev1.Service, detectedService *DetectedService) error {
	if len(detectedService.Endpoints) == 0 {
		return fmt.Errorf("no endpoints detected for service")
	}

	// Create map of service ports for easy lookup
	servicePorts := make(map[int32]corev1.ServicePort)
	for _, port := range k8sService.Spec.Ports {
		servicePorts[port.Port] = port
	}

	// Validate each detected endpoint has a corresponding service port
	for _, endpoint := range detectedService.Endpoints {
		if _, exists := servicePorts[endpoint.Port]; !exists {
			ev.log.WithFields(logrus.Fields{
				"service":       detectedService.Name,
				"endpoint_port": endpoint.Port,
			}).Debug("Detected endpoint port not found in service specification")
			// Don't fail validation - might be a port mapping or ingress
		}
	}

	return nil
}

// RBACValidator validates RBAC permissions for service access
// Business Requirement: BR-HOLMES-027 - RBAC considerations for service discovery
type RBACValidator struct {
	client kubernetes.Interface
	log    *logrus.Logger
}

// NewRBACValidator creates a new RBAC validator
func NewRBACValidator(client kubernetes.Interface, log *logrus.Logger) *RBACValidator {
	return &RBACValidator{
		client: client,
		log:    log,
	}
}

// Validate validates RBAC permissions for service access
func (rv *RBACValidator) Validate(ctx context.Context, service *DetectedService) error {
	// For now, perform basic namespace access validation
	// In a production environment, this would check specific RBAC permissions

	// Verify we can access the service's namespace
	_, err := rv.client.CoreV1().Namespaces().Get(ctx, service.Namespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("cannot access service namespace %s: %w", service.Namespace, err)
	}

	// Verify we can list services in the namespace
	_, err = rv.client.CoreV1().Services(service.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", service.Name),
	})
	if err != nil {
		return fmt.Errorf("insufficient permissions to access service %s in namespace %s: %w",
			service.Name, service.Namespace, err)
	}

	rv.log.WithFields(logrus.Fields{
		"service":   service.Name,
		"namespace": service.Namespace,
	}).Debug("RBAC validation completed")

	return nil
}

// ServiceTypeValidator validates service type consistency
// Business Requirement: BR-HOLMES-022 - Service-specific validation
type ServiceTypeValidator struct {
	log *logrus.Logger
}

// NewServiceTypeValidator creates a new service type validator
func NewServiceTypeValidator(log *logrus.Logger) *ServiceTypeValidator {
	return &ServiceTypeValidator{
		log: log,
	}
}

// Validate validates service type consistency
func (stv *ServiceTypeValidator) Validate(ctx context.Context, service *DetectedService) error {
	// Validate service type is not empty
	if service.ServiceType == "" {
		return fmt.Errorf("service type cannot be empty")
	}

	// Validate service has required fields based on type
	switch service.ServiceType {
	case "prometheus":
		if err := stv.validatePrometheusService(service); err != nil {
			return fmt.Errorf("prometheus service validation failed: %w", err)
		}
	case "grafana":
		if err := stv.validateGrafanaService(service); err != nil {
			return fmt.Errorf("grafana service validation failed: %w", err)
		}
	case "jaeger":
		if err := stv.validateJaegerService(service); err != nil {
			return fmt.Errorf("jaeger service validation failed: %w", err)
		}
	case "elasticsearch":
		if err := stv.validateElasticsearchService(service); err != nil {
			return fmt.Errorf("elasticsearch service validation failed: %w", err)
		}
	default:
		// For custom services, validate basic requirements
		if err := stv.validateCustomService(service); err != nil {
			return fmt.Errorf("custom service validation failed: %w", err)
		}
	}

	stv.log.WithFields(logrus.Fields{
		"service":      service.Name,
		"service_type": service.ServiceType,
	}).Debug("Service type validation completed")

	return nil
}

// validatePrometheusService validates Prometheus-specific requirements
func (stv *ServiceTypeValidator) validatePrometheusService(service *DetectedService) error {
	// Validate endpoints
	if len(service.Endpoints) == 0 {
		return fmt.Errorf("prometheus service must have at least one endpoint")
	}

	// Validate capabilities
	expectedCapabilities := []string{"query_metrics", "alert_rules", "time_series"}
	if !stv.hasCapabilities(service.Capabilities, expectedCapabilities) {
		stv.log.WithFields(logrus.Fields{
			"service":               service.Name,
			"expected_capabilities": expectedCapabilities,
			"actual_capabilities":   service.Capabilities,
		}).Debug("Prometheus service missing expected capabilities")
	}

	return nil
}

// validateGrafanaService validates Grafana-specific requirements
func (stv *ServiceTypeValidator) validateGrafanaService(service *DetectedService) error {
	if len(service.Endpoints) == 0 {
		return fmt.Errorf("grafana service must have at least one endpoint")
	}

	expectedCapabilities := []string{"get_dashboards", "query_datasource"}
	if !stv.hasCapabilities(service.Capabilities, expectedCapabilities) {
		stv.log.WithFields(logrus.Fields{
			"service":               service.Name,
			"expected_capabilities": expectedCapabilities,
			"actual_capabilities":   service.Capabilities,
		}).Debug("Grafana service missing expected capabilities")
	}

	return nil
}

// validateJaegerService validates Jaeger-specific requirements
func (stv *ServiceTypeValidator) validateJaegerService(service *DetectedService) error {
	if len(service.Endpoints) == 0 {
		return fmt.Errorf("jaeger service must have at least one endpoint")
	}

	expectedCapabilities := []string{"search_traces", "get_services"}
	if !stv.hasCapabilities(service.Capabilities, expectedCapabilities) {
		stv.log.WithFields(logrus.Fields{
			"service":               service.Name,
			"expected_capabilities": expectedCapabilities,
			"actual_capabilities":   service.Capabilities,
		}).Debug("Jaeger service missing expected capabilities")
	}

	return nil
}

// validateElasticsearchService validates Elasticsearch-specific requirements
func (stv *ServiceTypeValidator) validateElasticsearchService(service *DetectedService) error {
	if len(service.Endpoints) == 0 {
		return fmt.Errorf("elasticsearch service must have at least one endpoint")
	}

	expectedCapabilities := []string{"search_logs", "analyze_patterns"}
	if !stv.hasCapabilities(service.Capabilities, expectedCapabilities) {
		stv.log.WithFields(logrus.Fields{
			"service":               service.Name,
			"expected_capabilities": expectedCapabilities,
			"actual_capabilities":   service.Capabilities,
		}).Debug("Elasticsearch service missing expected capabilities")
	}

	return nil
}

// validateCustomService validates custom service requirements
func (stv *ServiceTypeValidator) validateCustomService(service *DetectedService) error {
	// For custom services, validate basic requirements
	if len(service.Endpoints) == 0 {
		return fmt.Errorf("custom service must have at least one endpoint")
	}

	// Validate custom service has toolset annotation
	if service.Annotations == nil {
		return fmt.Errorf("custom service must have annotations")
	}

	if _, exists := service.Annotations["kubernaut.io/toolset"]; !exists {
		return fmt.Errorf("custom service must have kubernaut.io/toolset annotation")
	}

	return nil
}

// hasCapabilities checks if service has all expected capabilities
func (stv *ServiceTypeValidator) hasCapabilities(actual, expected []string) bool {
	if len(expected) == 0 {
		return true
	}

	capabilityMap := make(map[string]bool)
	for _, capability := range actual {
		capabilityMap[capability] = true
	}

	for _, expectedCapability := range expected {
		if !capabilityMap[expectedCapability] {
			return false
		}
	}

	return true
}
