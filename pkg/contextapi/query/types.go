// Package query contains query execution types for Context API
package query

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/contextapi/models"
	datastorage "github.com/jordigilh/kubernaut/pkg/datastorage/query" // For Vector type
)

// IncidentEventRow is an intermediate struct for scanning from database with Vector support
// This struct can properly scan pgvector types and is then converted to models.IncidentEvent
//
// BR-CONTEXT-001: Database scanning with pgvector compatibility
type IncidentEventRow struct {
	// Primary identification
	ID                   int64  `db:"id"`
	Name                 string `db:"name"`
	AlertFingerprint     string `db:"alert_fingerprint"`
	RemediationRequestID string `db:"remediation_request_id"`

	// Context
	Namespace      string `db:"namespace"`
	ClusterName    string `db:"cluster_name"`
	Environment    string `db:"environment"`
	TargetResource string `db:"target_resource"`

	// Status
	Phase      string `db:"phase"`
	Status     string `db:"status"`
	Severity   string `db:"severity"`
	ActionType string `db:"action_type"`

	// Timing
	StartTime *time.Time `db:"start_time"`
	EndTime   *time.Time `db:"end_time"`
	Duration  *int64     `db:"duration"`

	// Error tracking
	ErrorMessage *string `db:"error_message"`

	// Metadata (JSON string)
	Metadata string `db:"metadata"`

	// Vector embedding for semantic search (uses datastorage.Vector for scanning)
	Embedding datastorage.Vector `db:"embedding"`

	// Audit timestamps
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// ToIncidentEvent converts IncidentEventRow to models.IncidentEvent
// Converts datastorage.Vector to []float32 for the API model
func (r *IncidentEventRow) ToIncidentEvent() *models.IncidentEvent {
	return &models.IncidentEvent{
		ID:                   r.ID,
		Name:                 r.Name,
		AlertFingerprint:     r.AlertFingerprint,
		RemediationRequestID: r.RemediationRequestID,
		Namespace:            r.Namespace,
		ClusterName:          r.ClusterName,
		Environment:          r.Environment,
		TargetResource:       r.TargetResource,
		Phase:                r.Phase,
		Status:               r.Status,
		Severity:             r.Severity,
		ActionType:           r.ActionType,
		StartTime:            r.StartTime,
		EndTime:              r.EndTime,
		Duration:             r.Duration,
		ErrorMessage:         r.ErrorMessage,
		Metadata:             r.Metadata,
		Embedding:            []float32(r.Embedding), // Convert datastorage.Vector to []float32
		CreatedAt:            r.CreatedAt,
		UpdatedAt:            r.UpdatedAt,
	}
}
