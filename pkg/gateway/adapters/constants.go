package adapters

// Signal source type constants for Gateway adapters
// These represent the origin/protocol of incoming signals and are used for:
// - Signal classification and routing
// - Metrics and observability
// - Audit event categorization (mapped to OpenAPI audit enums)
//
// Valid values match OpenAPI spec: api/openapi/data-storage-v1.yaml (GatewayAuditPayload.signal_type enum)
const (
	// SourceTypeAlertManager represents signals from Prometheus AlertManager webhooks
	// Used by: PrometheusAdapter (processes AlertManager webhook format)
	SourceTypeAlertManager = "alertmanager"

	// SourceTypeWebhook represents signals from generic webhooks
	// Used by: KubernetesEventAdapter (K8s events via webhook/API), custom webhook adapters
	SourceTypeWebhook = "webhook"
)
