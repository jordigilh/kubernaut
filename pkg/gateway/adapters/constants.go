package adapters

// Signal source type constants for Gateway adapters
// These represent the signal type identifier used for classification, metrics, and logging.
// Per BR-GATEWAY-027: Distinguish signal types for adapter-specific metrics and classification.
//
// Valid values match OpenAPI spec: api/openapi/data-storage-v1.yaml (GatewayAuditPayload.signal_type enum)
const (
	// SourceTypePrometheusAlert represents alert signals from Prometheus AlertManager
	// Used by: PrometheusAdapter
	// BR-GATEWAY-027: Returns "prometheus-alert" for signal type classification
	SourceTypePrometheusAlert = "prometheus-alert"

	// SourceTypeKubernetesEvent represents event signals from Kubernetes API
	// Used by: KubernetesEventAdapter
	// BR-GATEWAY-027: Returns "kubernetes-event" for signal type classification
	SourceTypeKubernetesEvent = "kubernetes-event"
)
