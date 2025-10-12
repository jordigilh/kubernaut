package discovery

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// Detector utilities - shared logic extracted from individual detectors
// Purpose: DRY principle - eliminate duplicate detection patterns across detectors

// ========================================
// Label Matching Utilities
// ========================================

// HasLabel checks if a service has a specific label with a specific value
func HasLabel(svc *corev1.Service, key, value string) bool {
	if svc.Labels == nil {
		return false
	}
	labelValue, ok := svc.Labels[key]
	return ok && labelValue == value
}

// HasAnyLabel checks if a service has any of the provided label key-value pairs
func HasAnyLabel(svc *corev1.Service, labels map[string]string) bool {
	if svc.Labels == nil {
		return false
	}

	for key, value := range labels {
		if labelValue, ok := svc.Labels[key]; ok && labelValue == value {
			return true
		}
	}
	return false
}

// HasStandardAppLabels checks for common app label patterns
// Checks both "app" and "app.kubernetes.io/name" labels for the given app name
func HasStandardAppLabels(svc *corev1.Service, appName string) bool {
	return HasLabel(svc, "app", appName) ||
		HasLabel(svc, "app.kubernetes.io/name", appName)
}

// ========================================
// Service Name Matching Utilities
// ========================================

// ServiceNameContains checks if the service name contains the given substring (case-insensitive)
func ServiceNameContains(svc *corev1.Service, substring string) bool {
	return strings.Contains(strings.ToLower(svc.Name), strings.ToLower(substring))
}

// ========================================
// Port Matching Utilities
// ========================================

// FindPort searches for a port by name or port number
// Returns nil if not found
// If portName is empty, only matches by port number
func FindPort(svc *corev1.Service, portName string, portNumber int32) *corev1.ServicePort {
	for i := range svc.Spec.Ports {
		port := &svc.Spec.Ports[i]
		// If portName is empty, only match by port number
		if portName == "" {
			if port.Port == portNumber {
				return port
			}
		} else {
			// Match by either name or port number
			if port.Name == portName || port.Port == portNumber {
				return port
			}
		}
	}
	return nil
}

// FindPortByName searches for a port by name
// Returns nil if not found
func FindPortByName(svc *corev1.Service, portName string) *corev1.ServicePort {
	for i := range svc.Spec.Ports {
		port := &svc.Spec.Ports[i]
		if port.Name == portName {
			return port
		}
	}
	return nil
}

// FindPortByNumber searches for a port by port number
// Returns nil if not found
func FindPortByNumber(svc *corev1.Service, portNumber int32) *corev1.ServicePort {
	for i := range svc.Spec.Ports {
		port := &svc.Spec.Ports[i]
		if port.Port == portNumber {
			return port
		}
	}
	return nil
}

// FindPortByAnyName searches for a port matching any of the provided names
// Returns the first match, or nil if none found
func FindPortByAnyName(svc *corev1.Service, portNames ...string) *corev1.ServicePort {
	for _, name := range portNames {
		if port := FindPortByName(svc, name); port != nil {
			return port
		}
	}
	return nil
}

// GetPortNumber returns the port number from a service using a priority-based search
// Priority: named ports → specific port number → first port → fallback
func GetPortNumber(svc *corev1.Service, portNames []string, targetPort int32, fallbackPort int32) int32 {
	// Strategy 1: Look for named ports
	if len(portNames) > 0 {
		if port := FindPortByAnyName(svc, portNames...); port != nil {
			return port.Port
		}
	}

	// Strategy 2: Look for specific port number
	if targetPort > 0 {
		if port := FindPortByNumber(svc, targetPort); port != nil {
			return port.Port
		}
	}

	// Strategy 3: Use first port if available
	if len(svc.Spec.Ports) > 0 {
		return svc.Spec.Ports[0].Port
	}

	// Strategy 4: Fallback port
	return fallbackPort
}

// HasPort checks if a service has a specific port (by name or number)
func HasPort(svc *corev1.Service, portName string, portNumber int32) bool {
	return FindPort(svc, portName, portNumber) != nil
}

// ========================================
// Endpoint Construction Utilities
// ========================================

// BuildEndpoint constructs a Kubernetes service endpoint URL in cluster.local format
// Format: http://service-name.namespace.svc.cluster.local:port
func BuildEndpoint(serviceName, namespace string, port int32) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", serviceName, namespace, port)
}

// BuildHTTPSEndpoint constructs a secure Kubernetes service endpoint URL
// Format: https://service-name.namespace.svc.cluster.local:port
func BuildHTTPSEndpoint(serviceName, namespace string, port int32) string {
	return fmt.Sprintf("https://%s.%s.svc.cluster.local:%d", serviceName, namespace, port)
}

// ========================================
// Multi-Strategy Detection Utilities
// ========================================

// DetectionStrategy defines a service detection strategy
type DetectionStrategy func(*corev1.Service) bool

// DetectByAnyStrategy runs multiple detection strategies and returns true if any match
func DetectByAnyStrategy(svc *corev1.Service, strategies ...DetectionStrategy) bool {
	for _, strategy := range strategies {
		if strategy(svc) {
			return true
		}
	}
	return false
}

// CreateLabelStrategy creates a detection strategy based on standard app labels
func CreateLabelStrategy(appName string) DetectionStrategy {
	return func(svc *corev1.Service) bool {
		return HasStandardAppLabels(svc, appName)
	}
}

// CreateNameStrategy creates a detection strategy based on service name matching
func CreateNameStrategy(substring string) DetectionStrategy {
	return func(svc *corev1.Service) bool {
		return ServiceNameContains(svc, substring)
	}
}

// CreatePortStrategy creates a detection strategy based on port matching
func CreatePortStrategy(portName string, portNumber int32) DetectionStrategy {
	return func(svc *corev1.Service) bool {
		return HasPort(svc, portName, portNumber)
	}
}
