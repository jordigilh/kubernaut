package adapters

// Signal source type constants for Gateway adapters.
// All adapters normalize to "alert" as the generic signal type.
// Adapter identity is preserved via NormalizedSignal.Source (e.g., "prometheus", "kubernetes-events").
//
// Valid values match OpenAPI spec: api/openapi/data-storage-v1.yaml (GatewayAuditPayload.signal_type enum)
const (
	// SourceTypeAlert is the normalized signal type for all alert-based adapters.
	// Individual adapter identity is carried in NormalizedSignal.Source, not SourceType.
	SourceTypeAlert = "alert"

	// Deprecated: Use SourceTypeAlert. Kept as aliases during migration.
	SourceTypePrometheusAlert = SourceTypeAlert
	SourceTypeKubernetesEvent = SourceTypeAlert
)
