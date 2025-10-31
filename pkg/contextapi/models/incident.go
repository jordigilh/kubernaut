// Package models defines data models for Context API
package models

import "time"

// IncidentEvent represents a remediation audit record from Data Storage Service
// Maps to resource_action_traces table in PostgreSQL (DD-SCHEMA-001)
//
// BR-CONTEXT-001: Query incident audit data
// BR-CONTEXT-004: Namespace/cluster/severity filtering
type IncidentEvent struct {
	// Primary identification
	ID                   int64  `db:"id" json:"id"`
	Name                 string `db:"name" json:"name"`                                     // Alert name
	AlertFingerprint     string `db:"alert_fingerprint" json:"alert_fingerprint"`           // Unique alert ID
	RemediationRequestID string `db:"remediation_request_id" json:"remediation_request_id"` // Unique request ID

	// Context
	Namespace      string `db:"namespace" json:"namespace"`
	ClusterName    string `db:"cluster_name" json:"cluster_name"`
	Environment    string `db:"environment" json:"environment"`
	TargetResource string `db:"target_resource" json:"target_resource"`

	// Status
	Phase      string `db:"phase" json:"phase"` // pending, processing, completed, failed
	Status     string `db:"status" json:"status"`
	Severity   string `db:"severity" json:"severity"`       // critical, warning, info
	ActionType string `db:"action_type" json:"action_type"` // scale, restart, delete, etc.

	// Timing
	StartTime *time.Time `db:"start_time" json:"start_time"`
	EndTime   *time.Time `db:"end_time" json:"end_time,omitempty"`
	Duration  *int64     `db:"duration" json:"duration,omitempty"` // milliseconds

	// Error tracking
	ErrorMessage *string `db:"error_message" json:"error_message,omitempty"`

	// Metadata (JSON string)
	Metadata string `db:"metadata" json:"metadata"`

	// Vector embedding for semantic search
	// BR-CONTEXT-002: Semantic search on embeddings
	Embedding []float32 `db:"embedding" json:"embedding,omitempty"`

	// Audit timestamps
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// ListIncidentsParams defines query parameters for listing incidents
//
// BR-CONTEXT-004: Namespace/cluster/severity filtering
// BR-CONTEXT-007: Pagination support
type ListIncidentsParams struct {
	// Filters
	Name             *string `json:"name,omitempty"`
	AlertFingerprint *string `json:"alert_fingerprint,omitempty"`
	Namespace        *string `json:"namespace,omitempty"`
	Phase            *string `json:"phase,omitempty"`
	Status           *string `json:"status,omitempty"`
	Severity         *string `json:"severity,omitempty"`
	ClusterName      *string `json:"cluster_name,omitempty"`
	Environment      *string `json:"environment,omitempty"`
	ActionType       *string `json:"action_type,omitempty"`

	// Pagination
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// SemanticSearchParams defines parameters for semantic search
//
// BR-CONTEXT-002: Semantic search on embeddings
type SemanticSearchParams struct {
	// Query embedding vector
	QueryEmbedding []float32 `json:"query_embedding"`

	// Alternative field name for compatibility
	Embedding []float32 `json:"embedding,omitempty"`

	// Similarity threshold (0.0-1.0)
	Threshold float32 `json:"threshold,omitempty"`

	// Optional filters
	Namespace *string `json:"namespace,omitempty"`
	Severity  *string `json:"severity,omitempty"`

	// Result limit
	Limit int `json:"limit"`
}

// PatternMatchQuery represents a pattern matching query with natural language
//
// BR-CONTEXT-002: Semantic search on embeddings
type PatternMatchQuery struct {
	// Natural language query
	Query string `json:"query"`

	// Query embedding vector (generated from Query)
	Embedding []float32 `json:"embedding,omitempty"`

	// Similarity threshold (0.0-1.0, default 0.7)
	Threshold float32 `json:"threshold"`

	// Optional filters
	Namespace *string `json:"namespace,omitempty"`
	Severity  *string `json:"severity,omitempty"`

	// Result limit (default 10, max 50)
	Limit int `json:"limit"`
}

// SimilarIncident represents an incident with similarity score
//
// BR-CONTEXT-002: Semantic search results
type SimilarIncident struct {
	IncidentEvent
	Similarity float32 `json:"similarity"` // Cosine similarity (0.0-1.0)
}

// ListIncidentsResponse represents the API response for listing incidents
//
// BR-CONTEXT-008: REST API for LLM context
type ListIncidentsResponse struct {
	Incidents []*IncidentEvent `json:"incidents"`
	Total     int              `json:"total"`
	Limit     int              `json:"limit"`
	Offset    int              `json:"offset"`
}

// SemanticSearchResponse represents the API response for semantic search
//
// BR-CONTEXT-002: Semantic search on embeddings
// BR-CONTEXT-008: REST API for LLM context
type SemanticSearchResponse struct {
	Incidents []*IncidentEvent `json:"incidents"`
	Scores    []float32        `json:"scores"` // Similarity scores
	Limit     int              `json:"limit"`
}

// HealthResponse represents the health check response
//
// BR-CONTEXT-006: Health checks & metrics
type HealthResponse struct {
	Status  string `json:"status"`  // "healthy" or "unhealthy"
	Message string `json:"message"` // Optional message
}

// Validate validates ListIncidentsParams
func (p *ListIncidentsParams) Validate() error {
	// Validate limit
	if p.Limit < 0 {
		return ErrInvalidLimit
	}
	if p.Limit == 0 {
		p.Limit = 10 // Default limit
	}
	if p.Limit > 100 {
		return ErrLimitTooLarge
	}

	// Validate offset
	if p.Offset < 0 {
		return ErrInvalidOffset
	}

	// Validate phase values
	if p.Phase != nil {
		phase := *p.Phase
		if phase != "pending" && phase != "processing" && phase != "completed" && phase != "failed" {
			return ErrInvalidPhase
		}
	}

	// Validate severity values
	if p.Severity != nil {
		severity := *p.Severity
		if severity != "critical" && severity != "warning" && severity != "info" {
			return ErrInvalidSeverity
		}
	}

	return nil
}

// Validate validates SemanticSearchParams
func (p *SemanticSearchParams) Validate() error {
	// Use Embedding if QueryEmbedding is not set
	if len(p.QueryEmbedding) == 0 && len(p.Embedding) > 0 {
		p.QueryEmbedding = p.Embedding
	}

	// Validate embedding vector
	if len(p.QueryEmbedding) == 0 {
		return ErrMissingEmbedding
	}
	if len(p.QueryEmbedding) != 384 {
		return ErrInvalidEmbeddingDimension
	}

	// Validate limit
	if p.Limit < 0 {
		return ErrInvalidLimit
	}
	if p.Limit == 0 {
		p.Limit = 10 // Default limit
	}
	if p.Limit > 50 {
		return ErrLimitTooLarge
	}

	// Validate severity if provided
	if p.Severity != nil {
		severity := *p.Severity
		if severity != "critical" && severity != "warning" && severity != "info" {
			return ErrInvalidSeverity
		}
	}

	return nil
}

// Validate validates PatternMatchQuery
func (q *PatternMatchQuery) Validate() error {
	// Validate query text or embedding
	if q.Query == "" && len(q.Embedding) == 0 {
		return ErrMissingQuery
	}

	// Validate embedding dimension if provided
	if len(q.Embedding) > 0 && len(q.Embedding) != 384 {
		return ErrInvalidEmbeddingDimension
	}

	// Validate threshold
	if q.Threshold < 0.0 || q.Threshold > 1.0 {
		return ErrInvalidThreshold
	}
	if q.Threshold == 0.0 {
		q.Threshold = 0.7 // Default threshold
	}

	// Validate limit
	if q.Limit < 0 {
		return ErrInvalidLimit
	}
	if q.Limit == 0 {
		q.Limit = 10 // Default limit
	}
	if q.Limit > 50 {
		return ErrLimitTooLarge
	}

	// Validate severity if provided
	if q.Severity != nil {
		severity := *q.Severity
		if severity != "critical" && severity != "warning" && severity != "info" {
			return ErrInvalidSeverity
		}
	}

	return nil
}
