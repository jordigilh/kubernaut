/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// ============================================================================
// TYPES
// ============================================================================

// EffectivenessScoreResponse is the response for GET /api/v1/effectiveness/{correlation_id}.
// Per ADR-EM-001 Principle 5 and DD-017 v2.1 scoring formula.
type EffectivenessScoreResponse struct {
	CorrelationID    string                   `json:"correlation_id"`
	Score            *float64                 `json:"score"`
	Components       EffectivenessComponents  `json:"components"`
	HashComparison   HashComparisonData       `json:"hash_comparison,omitempty"`
	AssessmentStatus string                   `json:"assessment_status"`
	ComputedAt       time.Time                `json:"computed_at"`
}

// EffectivenessComponents holds individual component assessment scores.
type EffectivenessComponents struct {
	HealthAssessed  bool     `json:"health_assessed"`
	HealthScore     *float64 `json:"health_score"`
	HealthDetails   string   `json:"health_details,omitempty"`
	AlertAssessed   bool     `json:"alert_assessed"`
	AlertScore      *float64 `json:"alert_score"`
	AlertDetails    string   `json:"alert_details,omitempty"`
	MetricsAssessed bool     `json:"metrics_assessed"`
	MetricsScore    *float64 `json:"metrics_score"`
	MetricsDetails  string   `json:"metrics_details,omitempty"`
}

// HashComparisonData holds pre/post remediation spec hash comparison.
// Supplementary signal, not part of the scoring formula (DD-EM-002).
type HashComparisonData struct {
	PreHash  string `json:"pre_remediation_spec_hash,omitempty"`
	PostHash string `json:"post_remediation_spec_hash,omitempty"`
	Match    *bool  `json:"hash_match,omitempty"`
}

// EffectivenessEvent represents a parsed audit event for effectiveness scoring.
type EffectivenessEvent struct {
	EventData map[string]interface{}
}

// ============================================================================
// DD-017 v2.1 WEIGHTED SCORING
// ============================================================================

// Component base weights per DD-017 v2.1.
const (
	weightHealth  = 0.40
	weightAlert   = 0.35
	weightMetrics = 0.25
)

// ComputeWeightedScore computes the weighted effectiveness score from component scores.
// Per DD-017 v2.1: score = sum(score_i * weight_i) / sum(weight_i)
// where weight_i is the base weight of each assessed component with a non-nil score.
// Missing components have their weight redistributed proportionally.
// Returns nil if no components have scores.
func ComputeWeightedScore(c *EffectivenessComponents) *float64 {
	var totalWeight float64
	var weightedSum float64

	if c.HealthAssessed && c.HealthScore != nil {
		totalWeight += weightHealth
		weightedSum += *c.HealthScore * weightHealth
	}
	if c.AlertAssessed && c.AlertScore != nil {
		totalWeight += weightAlert
		weightedSum += *c.AlertScore * weightAlert
	}
	if c.MetricsAssessed && c.MetricsScore != nil {
		totalWeight += weightMetrics
		weightedSum += *c.MetricsScore * weightMetrics
	}

	if totalWeight == 0 {
		return nil
	}

	score := weightedSum / totalWeight
	return &score
}

// ============================================================================
// RESPONSE BUILDER
// ============================================================================

// BuildEffectivenessResponse constructs the response from audit events.
// This is pure logic (no I/O) - extracts component scores from event payloads
// and applies the DD-017 v2.1 weighted scoring formula.
func BuildEffectivenessResponse(correlationID string, events []*EffectivenessEvent) *EffectivenessScoreResponse {
	resp := &EffectivenessScoreResponse{
		CorrelationID:    correlationID,
		AssessmentStatus: "no_data",
		ComputedAt:       time.Now().UTC(),
	}

	if len(events) == 0 {
		return resp
	}

	components := &resp.Components

	for _, event := range events {
		eventData := event.EventData
		if eventData == nil {
			continue
		}

		eventType, _ := eventData["event_type"].(string)

		switch eventType {
		case "effectiveness.health.assessed":
			if assessed, ok := eventData["assessed"].(bool); ok && assessed {
				components.HealthAssessed = true
			}
			if score, ok := eventData["score"].(float64); ok {
				components.HealthScore = &score
			}
			if details, ok := eventData["details"].(string); ok {
				components.HealthDetails = details
			}

		case "effectiveness.alert.assessed":
			if assessed, ok := eventData["assessed"].(bool); ok && assessed {
				components.AlertAssessed = true
			}
			if score, ok := eventData["score"].(float64); ok {
				components.AlertScore = &score
			}
			if details, ok := eventData["details"].(string); ok {
				components.AlertDetails = details
			}

		case "effectiveness.metrics.assessed":
			if assessed, ok := eventData["assessed"].(bool); ok && assessed {
				components.MetricsAssessed = true
			}
			if score, ok := eventData["score"].(float64); ok {
				components.MetricsScore = &score
			}
			if details, ok := eventData["details"].(string); ok {
				components.MetricsDetails = details
			}

		case "effectiveness.hash.computed":
			var hashData HashComparisonData
			if postHash, ok := eventData["post_remediation_spec_hash"].(string); ok {
				hashData.PostHash = postHash
			}
			if preHash, ok := eventData["pre_remediation_spec_hash"].(string); ok {
				hashData.PreHash = preHash
			}
			if match, ok := eventData["hash_match"].(bool); ok {
				hashData.Match = &match
			}
			if hashData.PreHash != "" || hashData.PostHash != "" {
				resp.HashComparison = hashData
			}

		case "effectiveness.assessment.completed":
			if reason, ok := eventData["reason"].(string); ok {
				// DD-EM-002 v1.1: spec_drift is terminal and takes priority over
				// all other reasons. When multiple completed events exist (e.g.,
				// "full" followed by "spec_drift" after EA re-assessment), spec_drift
				// must not be overwritten by an earlier reason that sorts later.
				if resp.AssessmentStatus != "spec_drift" {
					resp.AssessmentStatus = reason
				}
			}
		}
	}

	// DD-EM-002 v1.1: Spec drift means remediation was unsuccessful.
	// Short-circuit to score 0.0 — component scores are unreliable because
	// the target resource spec was modified (likely by another remediation).
	if resp.AssessmentStatus == "spec_drift" {
		score := 0.0
		resp.Score = &score
		return resp
	}

	// Compute weighted score from available components
	resp.Score = ComputeWeightedScore(components)

	// Determine assessment status if not already set by completed event
	if resp.AssessmentStatus == "no_data" {
		anyAssessed := components.HealthAssessed || components.AlertAssessed || components.MetricsAssessed
		if anyAssessed {
			resp.AssessmentStatus = "in_progress"
		}
	}

	return resp
}

// ============================================================================
// HTTP HANDLER
// ============================================================================

// handleGetEffectivenessScore handles GET /api/v1/effectiveness/{correlation_id}.
// It queries audit events and computes the weighted score on demand.
//
// Business Requirements: BR-EM-001 to BR-EM-004
// Architecture: ADR-EM-001 Principle 5, DD-017 v2.1
func (s *Server) handleGetEffectivenessScore(w http.ResponseWriter, r *http.Request) {
	correlationID := chi.URLParam(r, "correlation_id")
	if correlationID == "" {
		writeValidationRFC7807Error(w, &validation.RFC7807Problem{
			Type:     "https://kubernaut.ai/problems/effectiveness/missing-correlation-id",
			Title:    "Missing Correlation ID",
			Status:   http.StatusBadRequest,
			Detail:   "correlation_id path parameter is required",
			Instance: r.URL.Path,
		}, s)
		return
	}

	// Query effectiveness audit events from the database
	events, err := s.queryEffectivenessEvents(r.Context(), correlationID)
	if err != nil {
		s.logger.Error(err, "Failed to query effectiveness events",
			"correlation_id", correlationID)
		writeValidationRFC7807Error(w, &validation.RFC7807Problem{
			Type:     "https://kubernaut.ai/problems/effectiveness/query-error",
			Title:    "Effectiveness Query Error",
			Status:   http.StatusInternalServerError,
			Detail:   "Failed to query effectiveness events: " + err.Error(),
			Instance: r.URL.Path,
		}, s)
		return
	}

	if len(events) == 0 {
		writeValidationRFC7807Error(w, &validation.RFC7807Problem{
			Type:     "https://kubernaut.ai/problems/effectiveness/not-found",
			Title:    "Effectiveness Assessment Not Found",
			Status:   http.StatusNotFound,
			Detail:   "No effectiveness assessment events found for correlation_id: " + correlationID,
			Instance: r.URL.Path,
		}, s)
		return
	}

	// Build response from component events
	resp := BuildEffectivenessResponse(correlationID, events)

	s.logger.Info("Effectiveness score computed",
		"correlation_id", correlationID,
		"score", resp.Score,
		"status", resp.AssessmentStatus,
		"health_assessed", resp.Components.HealthAssessed,
		"alert_assessed", resp.Components.AlertAssessed,
		"metrics_assessed", resp.Components.MetricsAssessed,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.logger.Error(err, "failed to encode effectiveness response")
	}
}

// queryEffectivenessEvents queries audit events for a given correlation ID.
// Returns events filtered by event_category='effectiveness'.
//
// Convention (#211): All audit_events ORDER BY clauses MUST include event_id
// as a deterministic tiebreaker. Without it, same-timestamp events return in
// non-deterministic order, causing flaky tests and wrong assessment status.
func (s *Server) queryEffectivenessEvents(_ /* ctx */ interface{}, correlationID string) ([]*EffectivenessEvent, error) {
	// Query audit events from the database
	query := `SELECT event_data FROM audit_events
		WHERE correlation_id = $1
		AND event_category = 'effectiveness'
		ORDER BY event_timestamp ASC, event_id ASC`

	rows, err := s.db.Query(query, correlationID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			s.logger.Error(cerr, "Failed to close effectiveness query rows")
		}
	}()

	var events []*EffectivenessEvent
	for rows.Next() {
		var eventDataJSON []byte
		if err := rows.Scan(&eventDataJSON); err != nil {
			return nil, err
		}
		var eventData map[string]interface{}
		if err := json.Unmarshal(eventDataJSON, &eventData); err != nil {
			s.logger.Error(err, "Failed to unmarshal event data")
			continue
		}
		events = append(events, &EffectivenessEvent{EventData: eventData})
	}

	return events, rows.Err()
}
