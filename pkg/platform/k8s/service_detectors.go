package k8s

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// PrometheusDetector detects Prometheus services
// Business Requirement: BR-HOLMES-017 - Automatic detection of Prometheus
type PrometheusDetector struct {
	pattern ServicePattern
	log     *logrus.Logger
}

// NewPrometheusDetector creates a new Prometheus detector
func NewPrometheusDetector(pattern ServicePattern, log *logrus.Logger) *PrometheusDetector {
	return &PrometheusDetector{
		pattern: pattern,
		log:     log,
	}
}

// Detect detects if a service is Prometheus
func (pd *PrometheusDetector) Detect(ctx context.Context, service *corev1.Service) (*DetectedService, error) {
	if !pd.pattern.Enabled {
		return nil, nil
	}

	// Check service name patterns
	if pd.matchesServiceName(service.Name) {
		return pd.createDetectedService(service), nil
	}

	// Check label selectors
	if pd.matchesLabels(service.Labels) {
		return pd.createDetectedService(service), nil
	}

	// Check required ports
	if pd.matchesPorts(service.Spec.Ports) {
		return pd.createDetectedService(service), nil
	}

	return nil, nil
}

// GetServiceType returns the service type
func (pd *PrometheusDetector) GetServiceType() string {
	return "prometheus"
}

// GetPriority returns the detector priority
func (pd *PrometheusDetector) GetPriority() int {
	return pd.pattern.Priority
}

// matchesServiceName checks if service name matches Prometheus patterns
func (pd *PrometheusDetector) matchesServiceName(name string) bool {
	name = strings.ToLower(name)
	for _, pattern := range pd.pattern.ServiceNames {
		if strings.Contains(name, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// matchesLabels checks if service labels match Prometheus patterns
func (pd *PrometheusDetector) matchesLabels(labels map[string]string) bool {
	if labels == nil {
		return false
	}

	for _, selector := range pd.pattern.Selectors {
		if pd.matchesSelector(labels, selector) {
			return true
		}
	}
	return false
}

// matchesSelector checks if labels match a specific selector
func (pd *PrometheusDetector) matchesSelector(labels map[string]string, selector map[string]string) bool {
	for key, value := range selector {
		if labelValue, exists := labels[key]; !exists || labelValue != value {
			return false
		}
	}
	return true
}

// matchesPorts checks if service ports match Prometheus patterns
func (pd *PrometheusDetector) matchesPorts(ports []corev1.ServicePort) bool {
	if len(pd.pattern.RequiredPorts) == 0 {
		return false
	}

	for _, requiredPort := range pd.pattern.RequiredPorts {
		for _, port := range ports {
			if port.Port == requiredPort {
				return true
			}
		}
	}
	return false
}

// createDetectedService creates a DetectedService for Prometheus
func (pd *PrometheusDetector) createDetectedService(service *corev1.Service) *DetectedService {
	endpoints := pd.createEndpoints(service)

	return &DetectedService{
		Name:         service.Name,
		Namespace:    service.Namespace,
		ServiceType:  "prometheus",
		Endpoints:    endpoints,
		Labels:       service.Labels,
		Annotations:  service.Annotations,
		Available:    true, // Will be validated later
		Priority:     pd.pattern.Priority,
		Capabilities: pd.pattern.Capabilities,
	}
}

// createEndpoints creates service endpoints for Prometheus
func (pd *PrometheusDetector) createEndpoints(service *corev1.Service) []ServiceEndpoint {
	var endpoints []ServiceEndpoint

	for _, port := range service.Spec.Ports {
		protocol := "http"
		if port.Port == 443 || strings.Contains(strings.ToLower(port.Name), "https") {
			protocol = "https"
		}

		url := fmt.Sprintf("%s://%s.%s.svc.cluster.local:%d",
			protocol, service.Name, service.Namespace, port.Port)

		endpoints = append(endpoints, ServiceEndpoint{
			Name:     port.Name,
			URL:      url,
			Port:     port.Port,
			Protocol: protocol,
		})
	}

	return endpoints
}

// GrafanaDetector detects Grafana services
// Business Requirement: BR-HOLMES-017 - Automatic detection of Grafana
type GrafanaDetector struct {
	pattern ServicePattern
	log     *logrus.Logger
}

// NewGrafanaDetector creates a new Grafana detector
func NewGrafanaDetector(pattern ServicePattern, log *logrus.Logger) *GrafanaDetector {
	return &GrafanaDetector{
		pattern: pattern,
		log:     log,
	}
}

// Detect detects if a service is Grafana
func (gd *GrafanaDetector) Detect(ctx context.Context, service *corev1.Service) (*DetectedService, error) {
	if !gd.pattern.Enabled {
		return nil, nil
	}

	if gd.matchesServiceName(service.Name) || gd.matchesLabels(service.Labels) || gd.matchesPorts(service.Spec.Ports) {
		return gd.createDetectedService(service), nil
	}

	return nil, nil
}

// GetServiceType returns the service type
func (gd *GrafanaDetector) GetServiceType() string {
	return "grafana"
}

// GetPriority returns the detector priority
func (gd *GrafanaDetector) GetPriority() int {
	return gd.pattern.Priority
}

// matchesServiceName checks if service name matches Grafana patterns
func (gd *GrafanaDetector) matchesServiceName(name string) bool {
	name = strings.ToLower(name)
	for _, pattern := range gd.pattern.ServiceNames {
		if strings.Contains(name, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// matchesLabels checks if service labels match Grafana patterns
func (gd *GrafanaDetector) matchesLabels(labels map[string]string) bool {
	if labels == nil {
		return false
	}

	for _, selector := range gd.pattern.Selectors {
		if gd.matchesSelector(labels, selector) {
			return true
		}
	}
	return false
}

// matchesSelector checks if labels match a specific selector
func (gd *GrafanaDetector) matchesSelector(labels map[string]string, selector map[string]string) bool {
	for key, value := range selector {
		if labelValue, exists := labels[key]; !exists || labelValue != value {
			return false
		}
	}
	return true
}

// matchesPorts checks if service ports match Grafana patterns
func (gd *GrafanaDetector) matchesPorts(ports []corev1.ServicePort) bool {
	if len(gd.pattern.RequiredPorts) == 0 {
		return false
	}

	for _, requiredPort := range gd.pattern.RequiredPorts {
		for _, port := range ports {
			if port.Port == requiredPort {
				return true
			}
		}
	}
	return false
}

// createDetectedService creates a DetectedService for Grafana
func (gd *GrafanaDetector) createDetectedService(service *corev1.Service) *DetectedService {
	endpoints := gd.createEndpoints(service)

	return &DetectedService{
		Name:         service.Name,
		Namespace:    service.Namespace,
		ServiceType:  "grafana",
		Endpoints:    endpoints,
		Labels:       service.Labels,
		Annotations:  service.Annotations,
		Available:    true,
		Priority:     gd.pattern.Priority,
		Capabilities: gd.pattern.Capabilities,
	}
}

// createEndpoints creates service endpoints for Grafana
func (gd *GrafanaDetector) createEndpoints(service *corev1.Service) []ServiceEndpoint {
	var endpoints []ServiceEndpoint

	for _, port := range service.Spec.Ports {
		protocol := "http"
		if port.Port == 443 || strings.Contains(strings.ToLower(port.Name), "https") {
			protocol = "https"
		}

		url := fmt.Sprintf("%s://%s.%s.svc.cluster.local:%d",
			protocol, service.Name, service.Namespace, port.Port)

		endpoints = append(endpoints, ServiceEndpoint{
			Name:     port.Name,
			URL:      url,
			Port:     port.Port,
			Protocol: protocol,
		})
	}

	return endpoints
}

// JaegerDetector detects Jaeger services
// Business Requirement: BR-HOLMES-017 - Automatic detection of Jaeger
type JaegerDetector struct {
	pattern ServicePattern
	log     *logrus.Logger
}

// NewJaegerDetector creates a new Jaeger detector
func NewJaegerDetector(pattern ServicePattern, log *logrus.Logger) *JaegerDetector {
	return &JaegerDetector{
		pattern: pattern,
		log:     log,
	}
}

// Detect detects if a service is Jaeger
func (jd *JaegerDetector) Detect(ctx context.Context, service *corev1.Service) (*DetectedService, error) {
	if !jd.pattern.Enabled {
		return nil, nil
	}

	if jd.matchesServiceName(service.Name) || jd.matchesLabels(service.Labels) || jd.matchesPorts(service.Spec.Ports) {
		return jd.createDetectedService(service), nil
	}

	return nil, nil
}

// GetServiceType returns the service type
func (jd *JaegerDetector) GetServiceType() string {
	return "jaeger"
}

// GetPriority returns the detector priority
func (jd *JaegerDetector) GetPriority() int {
	return jd.pattern.Priority
}

// matchesServiceName checks if service name matches Jaeger patterns
func (jd *JaegerDetector) matchesServiceName(name string) bool {
	name = strings.ToLower(name)
	for _, pattern := range jd.pattern.ServiceNames {
		if strings.Contains(name, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// matchesLabels checks if service labels match Jaeger patterns
func (jd *JaegerDetector) matchesLabels(labels map[string]string) bool {
	if labels == nil {
		return false
	}

	for _, selector := range jd.pattern.Selectors {
		if jd.matchesSelector(labels, selector) {
			return true
		}
	}
	return false
}

// matchesSelector checks if labels match a specific selector
func (jd *JaegerDetector) matchesSelector(labels map[string]string, selector map[string]string) bool {
	for key, value := range selector {
		if labelValue, exists := labels[key]; !exists || labelValue != value {
			return false
		}
	}
	return true
}

// matchesPorts checks if service ports match Jaeger patterns
func (jd *JaegerDetector) matchesPorts(ports []corev1.ServicePort) bool {
	if len(jd.pattern.RequiredPorts) == 0 {
		return false
	}

	for _, requiredPort := range jd.pattern.RequiredPorts {
		for _, port := range ports {
			if port.Port == requiredPort {
				return true
			}
		}
	}
	return false
}

// createDetectedService creates a DetectedService for Jaeger
func (jd *JaegerDetector) createDetectedService(service *corev1.Service) *DetectedService {
	endpoints := jd.createEndpoints(service)

	return &DetectedService{
		Name:         service.Name,
		Namespace:    service.Namespace,
		ServiceType:  "jaeger",
		Endpoints:    endpoints,
		Labels:       service.Labels,
		Annotations:  service.Annotations,
		Available:    true,
		Priority:     jd.pattern.Priority,
		Capabilities: jd.pattern.Capabilities,
	}
}

// createEndpoints creates service endpoints for Jaeger
func (jd *JaegerDetector) createEndpoints(service *corev1.Service) []ServiceEndpoint {
	var endpoints []ServiceEndpoint

	for _, port := range service.Spec.Ports {
		protocol := "http"
		if port.Port == 443 || strings.Contains(strings.ToLower(port.Name), "https") {
			protocol = "https"
		}

		url := fmt.Sprintf("%s://%s.%s.svc.cluster.local:%d",
			protocol, service.Name, service.Namespace, port.Port)

		endpoints = append(endpoints, ServiceEndpoint{
			Name:     port.Name,
			URL:      url,
			Port:     port.Port,
			Protocol: protocol,
		})
	}

	return endpoints
}

// ElasticsearchDetector detects Elasticsearch services
// Business Requirement: BR-HOLMES-017 - Automatic detection of Elasticsearch
type ElasticsearchDetector struct {
	pattern ServicePattern
	log     *logrus.Logger
}

// NewElasticsearchDetector creates a new Elasticsearch detector
func NewElasticsearchDetector(pattern ServicePattern, log *logrus.Logger) *ElasticsearchDetector {
	return &ElasticsearchDetector{
		pattern: pattern,
		log:     log,
	}
}

// Detect detects if a service is Elasticsearch
func (ed *ElasticsearchDetector) Detect(ctx context.Context, service *corev1.Service) (*DetectedService, error) {
	if !ed.pattern.Enabled {
		return nil, nil
	}

	if ed.matchesServiceName(service.Name) || ed.matchesLabels(service.Labels) || ed.matchesPorts(service.Spec.Ports) {
		return ed.createDetectedService(service), nil
	}

	return nil, nil
}

// GetServiceType returns the service type
func (ed *ElasticsearchDetector) GetServiceType() string {
	return "elasticsearch"
}

// GetPriority returns the detector priority
func (ed *ElasticsearchDetector) GetPriority() int {
	return ed.pattern.Priority
}

// matchesServiceName checks if service name matches Elasticsearch patterns
func (ed *ElasticsearchDetector) matchesServiceName(name string) bool {
	name = strings.ToLower(name)
	for _, pattern := range ed.pattern.ServiceNames {
		if strings.Contains(name, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// matchesLabels checks if service labels match Elasticsearch patterns
func (ed *ElasticsearchDetector) matchesLabels(labels map[string]string) bool {
	if labels == nil {
		return false
	}

	for _, selector := range ed.pattern.Selectors {
		if ed.matchesSelector(labels, selector) {
			return true
		}
	}
	return false
}

// matchesSelector checks if labels match a specific selector
func (ed *ElasticsearchDetector) matchesSelector(labels map[string]string, selector map[string]string) bool {
	for key, value := range selector {
		if labelValue, exists := labels[key]; !exists || labelValue != value {
			return false
		}
	}
	return true
}

// matchesPorts checks if service ports match Elasticsearch patterns
func (ed *ElasticsearchDetector) matchesPorts(ports []corev1.ServicePort) bool {
	if len(ed.pattern.RequiredPorts) == 0 {
		return false
	}

	for _, requiredPort := range ed.pattern.RequiredPorts {
		for _, port := range ports {
			if port.Port == requiredPort {
				return true
			}
		}
	}
	return false
}

// createDetectedService creates a DetectedService for Elasticsearch
func (ed *ElasticsearchDetector) createDetectedService(service *corev1.Service) *DetectedService {
	endpoints := ed.createEndpoints(service)

	return &DetectedService{
		Name:         service.Name,
		Namespace:    service.Namespace,
		ServiceType:  "elasticsearch",
		Endpoints:    endpoints,
		Labels:       service.Labels,
		Annotations:  service.Annotations,
		Available:    true,
		Priority:     ed.pattern.Priority,
		Capabilities: ed.pattern.Capabilities,
	}
}

// createEndpoints creates service endpoints for Elasticsearch
func (ed *ElasticsearchDetector) createEndpoints(service *corev1.Service) []ServiceEndpoint {
	var endpoints []ServiceEndpoint

	for _, port := range service.Spec.Ports {
		protocol := "http"
		if port.Port == 443 || strings.Contains(strings.ToLower(port.Name), "https") {
			protocol = "https"
		}

		url := fmt.Sprintf("%s://%s.%s.svc.cluster.local:%d",
			protocol, service.Name, service.Namespace, port.Port)

		endpoints = append(endpoints, ServiceEndpoint{
			Name:     port.Name,
			URL:      url,
			Port:     port.Port,
			Protocol: protocol,
		})
	}

	return endpoints
}

// CustomServiceDetector detects custom services based on annotations
// Business Requirement: BR-HOLMES-018 - Custom service detection through annotations
type CustomServiceDetector struct {
	pattern ServicePattern
	log     *logrus.Logger
}

// NewCustomServiceDetector creates a new custom service detector
func NewCustomServiceDetector(pattern ServicePattern, log *logrus.Logger) *CustomServiceDetector {
	return &CustomServiceDetector{
		pattern: pattern,
		log:     log,
	}
}

// Detect detects custom services based on annotations
func (csd *CustomServiceDetector) Detect(ctx context.Context, service *corev1.Service) (*DetectedService, error) {
	if !csd.pattern.Enabled {
		return nil, nil
	}

	if service.Annotations == nil {
		return nil, nil
	}

	// Check for custom toolset annotation
	toolsetType, exists := service.Annotations["kubernaut.io/toolset"]
	if !exists {
		return nil, nil
	}

	return csd.createDetectedService(service, toolsetType), nil
}

// GetServiceType returns the service type
func (csd *CustomServiceDetector) GetServiceType() string {
	return "custom"
}

// GetPriority returns the detector priority
func (csd *CustomServiceDetector) GetPriority() int {
	return csd.pattern.Priority
}

// createDetectedService creates a DetectedService for custom service
func (csd *CustomServiceDetector) createDetectedService(service *corev1.Service, toolsetType string) *DetectedService {
	endpoints := csd.createEndpoints(service)
	capabilities := csd.parseCapabilities(service.Annotations)

	return &DetectedService{
		Name:         service.Name,
		Namespace:    service.Namespace,
		ServiceType:  toolsetType, // Use the custom toolset type
		Endpoints:    endpoints,
		Labels:       service.Labels,
		Annotations:  service.Annotations,
		Available:    true,
		Priority:     csd.pattern.Priority,
		Capabilities: capabilities,
	}
}

// createEndpoints creates service endpoints for custom service
func (csd *CustomServiceDetector) createEndpoints(service *corev1.Service) []ServiceEndpoint {
	var endpoints []ServiceEndpoint

	// Check for custom endpoints annotation
	if endpointsStr, exists := service.Annotations["kubernaut.io/endpoints"]; exists {
		endpoints = csd.parseEndpoints(service, endpointsStr)
	}

	// Fallback to standard service ports if no custom endpoints
	if len(endpoints) == 0 {
		for _, port := range service.Spec.Ports {
			protocol := "http"
			if port.Port == 443 || strings.Contains(strings.ToLower(port.Name), "https") {
				protocol = "https"
			}

			url := fmt.Sprintf("%s://%s.%s.svc.cluster.local:%d",
				protocol, service.Name, service.Namespace, port.Port)

			endpoints = append(endpoints, ServiceEndpoint{
				Name:     port.Name,
				URL:      url,
				Port:     port.Port,
				Protocol: protocol,
			})
		}
	}

	return endpoints
}

// parseEndpoints parses custom endpoints from annotation
// Format: "metrics:9090,logs:3100,api:8080"
func (csd *CustomServiceDetector) parseEndpoints(service *corev1.Service, endpointsStr string) []ServiceEndpoint {
	var endpoints []ServiceEndpoint

	endpointPairs := strings.Split(endpointsStr, ",")
	for _, pair := range endpointPairs {
		parts := strings.Split(strings.TrimSpace(pair), ":")
		if len(parts) != 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		portStr := strings.TrimSpace(parts[1])

		port, err := strconv.ParseInt(portStr, 10, 32)
		if err != nil {
			csd.log.WithError(err).WithField("port", portStr).Debug("Failed to parse custom endpoint port")
			continue
		}

		protocol := "http"
		if port == 443 {
			protocol = "https"
		}

		url := fmt.Sprintf("%s://%s.%s.svc.cluster.local:%d",
			protocol, service.Name, service.Namespace, port)

		endpoints = append(endpoints, ServiceEndpoint{
			Name:     name,
			URL:      url,
			Port:     int32(port),
			Protocol: protocol,
		})
	}

	return endpoints
}

// parseCapabilities parses custom capabilities from annotation
// Format: "query_logs,analyze_patterns,custom_metric"
func (csd *CustomServiceDetector) parseCapabilities(annotations map[string]string) []string {
	capabilitiesStr, exists := annotations["kubernaut.io/capabilities"]
	if !exists {
		return []string{}
	}

	capabilities := strings.Split(capabilitiesStr, ",")
	for i, capability := range capabilities {
		capabilities[i] = strings.TrimSpace(capability)
	}

	return capabilities
}
