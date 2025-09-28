package alert

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// AlertService defines the interface for alert processing operations
// Business Requirements: BR-ALERT-001 to BR-ALERT-006
// Single Responsibility: Alert ingestion, validation, routing, and enrichment
type AlertService interface {
	// Core alert processing
	ProcessAlert(ctx context.Context, alert types.Alert) (*ProcessResult, error)

	// BR-ALERT-001: Alert validation
	ValidateAlert(alert types.Alert) map[string]interface{}

	// BR-ALERT-002: Alert routing and filtering
	RouteAlert(ctx context.Context, alert types.Alert) map[string]interface{}

	// BR-ALERT-003: Alert deduplication
	GetDeduplicationStats() map[string]interface{}

	// BR-ALERT-004: Alert enrichment
	EnrichAlert(ctx context.Context, alert types.Alert) map[string]interface{}

	// BR-ALERT-005: Alert persistence
	PersistAlert(ctx context.Context, alert types.Alert) map[string]interface{}
	GetAlertHistory(namespace string, duration time.Duration) map[string]interface{}

	// BR-ALERT-006: Service metrics
	GetAlertMetrics() map[string]interface{}

	// Service health
	Health() map[string]interface{}
}

// ProcessResult represents the result of alert processing
type ProcessResult struct {
	Success            bool                   `json:"success"`
	Skipped            bool                   `json:"skipped,omitempty"`
	Reason             string                 `json:"reason,omitempty"`
	AlertID            string                 `json:"alert_id,omitempty"`
	ProcessingTime     time.Duration          `json:"processing_time"`
	ValidationResult   map[string]interface{} `json:"validation_result,omitempty"`
	EnrichmentResult   map[string]interface{} `json:"enrichment_result,omitempty"`
	RoutingResult      map[string]interface{} `json:"routing_result,omitempty"`
	DeduplicationCheck bool                   `json:"deduplication_check"`
	PersistenceResult  map[string]interface{} `json:"persistence_result,omitempty"`
}

// AlertProcessor handles the core alert processing logic
type AlertProcessor interface {
	// Core processing operations
	Process(ctx context.Context, alert types.Alert) (*ProcessResult, error)
	ShouldProcess(alert types.Alert) bool

	// Validation operations
	Validate(alert types.Alert) (bool, []string)

	// Enrichment operations
	Enrich(ctx context.Context, alert types.Alert) (map[string]interface{}, error)

	// Routing operations
	Route(ctx context.Context, alert types.Alert) (string, error)

	// Deduplication operations
	CheckDuplicate(alert types.Alert) bool

	// Persistence operations
	Persist(ctx context.Context, alert types.Alert) (string, error)
}

// AlertEnricher handles alert enrichment with AI analysis
type AlertEnricher interface {
	EnrichWithAI(ctx context.Context, alert types.Alert) (map[string]interface{}, error)
	EnrichWithMetadata(alert types.Alert) map[string]interface{}
	IsHealthy() bool
}

// AlertRouter handles alert routing decisions
type AlertRouter interface {
	DetermineRoute(alert types.Alert) string
	GetAvailableRoutes() []string
	IsRouteHealthy(route string) bool
}

// AlertValidator handles alert validation
type AlertValidator interface {
	ValidateStructure(alert types.Alert) (bool, []string)
	ValidateContent(alert types.Alert) (bool, []string)
	ValidateBusinessRules(alert types.Alert) (bool, []string)
}

// AlertDeduplicator handles alert deduplication
type AlertDeduplicator interface {
	IsDuplicate(alert types.Alert) bool
	GetDuplicateWindow() time.Duration
	GetStats() map[string]interface{}
}

// AlertPersister handles alert persistence
type AlertPersister interface {
	Save(ctx context.Context, alert types.Alert) (string, error)
	GetHistory(namespace string, duration time.Duration) ([]types.Alert, error)
	GetMetrics() map[string]interface{}
}
