package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
)

// ========================================
// SOC2 Gap #8: Legal Hold & Retention
// BR-AUDIT-006: Legal hold capability for Sarbanes-Oxley and HIPAA compliance
// ========================================

// PlaceLegalHoldRequest represents a legal hold placement request
type PlaceLegalHoldRequest struct {
	CorrelationID string `json:"correlation_id"`
	Reason        string `json:"reason"`
}

// PlaceLegalHoldResponse represents the result of placing a legal hold
type PlaceLegalHoldResponse struct {
	CorrelationID  string    `json:"correlation_id"`
	EventsAffected int       `json:"events_affected"`
	PlacedBy       string    `json:"placed_by"`
	PlacedAt       time.Time `json:"placed_at"`
	Reason         string    `json:"reason"`
}

// ReleaseLegalHoldRequest represents a legal hold release request
type ReleaseLegalHoldRequest struct {
	ReleaseReason string `json:"release_reason"`
}

// ReleaseLegalHoldResponse represents the result of releasing a legal hold
type ReleaseLegalHoldResponse struct {
	CorrelationID  string    `json:"correlation_id"`
	EventsReleased int       `json:"events_released"`
	ReleasedBy     string    `json:"released_by"`
	ReleasedAt     time.Time `json:"released_at"`
}

// LegalHold represents an active legal hold
type LegalHold struct {
	CorrelationID  string    `json:"correlation_id"`
	EventsAffected int       `json:"events_affected"`
	PlacedBy       string    `json:"placed_by"`
	PlacedAt       time.Time `json:"placed_at"`
	Reason         string    `json:"reason"`
}

// ListLegalHoldsResponse represents a list of active legal holds
type ListLegalHoldsResponse struct {
	Holds []LegalHold `json:"holds"`
	Total int         `json:"total"`
}

// HandlePlaceLegalHold places a legal hold on all events with a correlation_id
// POST /api/v1/audit/legal-hold
func (s *Server) HandlePlaceLegalHold(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1. Parse request body
	var req PlaceLegalHoldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.metrics.LegalHoldFailures.WithLabelValues("invalid_request").Inc()
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid-request", "Invalid Request",
			fmt.Sprintf("Invalid request body: %v", err), s.logger)
		return
	}

	// 2. Validate correlation_id
	if req.CorrelationID == "" {
		s.metrics.LegalHoldFailures.WithLabelValues("missing_correlation_id").Inc()
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-correlation-id", "Missing Correlation ID",
			"correlation_id is required", s.logger)
		return
	}

	if req.Reason == "" {
		s.metrics.LegalHoldFailures.WithLabelValues("missing_reason").Inc()
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-reason", "Missing Reason",
			"reason is required", s.logger)
		return
	}

	// 3. Extract X-Auth-Request-User header (placed_by) - REQUIRED for SOC2 compliance
	// DD-AUTH-004: OAuth-proxy injects this header after validating JWT token + SAR
	// DD-AUTH-005: All services authenticate via oauth-proxy, which sets this header
	placedBy := r.Header.Get("X-Auth-Request-User")
	if placedBy == "" {
		s.metrics.LegalHoldFailures.WithLabelValues("unauthorized").Inc()
		response.WriteRFC7807Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized",
			"X-Auth-Request-User header is required for legal hold operations (missing authentication)", s.logger)
		return
	}

	// 4. Check if correlation_id exists
	var eventCount int
	checkQuery := `SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1`
	err := s.db.QueryRowContext(ctx, checkQuery, req.CorrelationID).Scan(&eventCount)
	if err != nil {
		s.logger.Error(err, "Failed to check correlation_id existence",
			"correlation_id", req.CorrelationID)
		s.metrics.LegalHoldFailures.WithLabelValues("db_error").Inc()
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to check correlation_id", s.logger)
		return
	}

	if eventCount == 0 {
		s.metrics.LegalHoldFailures.WithLabelValues("correlation_id_not_found").Inc()
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
			fmt.Sprintf("No events found for correlation_id: %s", req.CorrelationID), s.logger)
		return
	}

	// 5. Place legal hold on all events with correlation_id
	placedAt := time.Now()
	updateQuery := `
		UPDATE audit_events
		SET legal_hold = TRUE,
		    legal_hold_reason = $1,
		    legal_hold_placed_by = $2,
		    legal_hold_placed_at = $3
		WHERE correlation_id = $4
	`
	result, err := s.db.ExecContext(ctx, updateQuery, req.Reason, placedBy, placedAt, req.CorrelationID)
	if err != nil {
		s.logger.Error(err, "Failed to place legal hold",
			"correlation_id", req.CorrelationID,
			"placed_by", placedBy)
		s.metrics.LegalHoldFailures.WithLabelValues("update_failed").Inc()
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to place legal hold", s.logger)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		s.logger.Error(err, "Failed to get rows affected", "correlation_id", req.CorrelationID)
	}

	// 6. Log success
	s.logger.Info("Legal hold placed",
		"correlation_id", req.CorrelationID,
		"events_affected", rowsAffected,
		"placed_by", placedBy,
		"reason", req.Reason)

	s.metrics.LegalHoldSuccesses.WithLabelValues("place").Inc()

	// 7. Return response
	response := PlaceLegalHoldResponse{
		CorrelationID:  req.CorrelationID,
		EventsAffected: int(rowsAffected),
		PlacedBy:       placedBy,
		PlacedAt:       placedAt,
		Reason:         req.Reason,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error(err, "Failed to write place legal hold response")
	}
}

// HandleReleaseLegalHold releases a legal hold on all events with a correlation_id
// DELETE /api/v1/audit/legal-hold/{correlation_id}
func (s *Server) HandleReleaseLegalHold(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1. Extract correlation_id from URL path
	correlationID := chi.URLParam(r, "correlation_id")
	if correlationID == "" {
		s.metrics.LegalHoldFailures.WithLabelValues("missing_correlation_id").Inc()
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-correlation-id", "Missing Correlation ID",
			"correlation_id is required in URL path", s.logger)
		return
	}

	// 2. Parse request body (release_reason)
	var req ReleaseLegalHoldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.metrics.LegalHoldFailures.WithLabelValues("invalid_request").Inc()
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid-request", "Invalid Request",
			fmt.Sprintf("Invalid request body: %v", err), s.logger)
		return
	}

	if req.ReleaseReason == "" {
		s.metrics.LegalHoldFailures.WithLabelValues("missing_release_reason").Inc()
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-release-reason", "Missing Release Reason",
			"release_reason is required", s.logger)
		return
	}

	// 3. Extract X-Auth-Request-User header (released_by) - REQUIRED for SOC2 compliance
	// DD-AUTH-004: OAuth-proxy injects this header after validating JWT token + SAR
	// DD-AUTH-005: All services authenticate via oauth-proxy, which sets this header
	releasedBy := r.Header.Get("X-Auth-Request-User")
	if releasedBy == "" {
		s.metrics.LegalHoldFailures.WithLabelValues("unauthorized").Inc()
		response.WriteRFC7807Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized",
			"X-Auth-Request-User header is required for legal hold operations (missing authentication)", s.logger)
		return
	}

	// 4. Check if correlation_id has any events with legal hold
	var holdCount int
	checkQuery := `SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1 AND legal_hold = TRUE`
	err := s.db.QueryRowContext(ctx, checkQuery, correlationID).Scan(&holdCount)
	if err != nil {
		s.logger.Error(err, "Failed to check legal hold existence", "correlation_id", correlationID)
		s.metrics.LegalHoldFailures.WithLabelValues("db_error").Inc()
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to check legal hold", s.logger)
		return
	}

	if holdCount == 0 {
		s.metrics.LegalHoldFailures.WithLabelValues("no_legal_hold").Inc()
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
			fmt.Sprintf("No legal hold found for correlation_id: %s", correlationID), s.logger)
		return
	}

	// 5. Release legal hold
	releasedAt := time.Now()
	updateQuery := `
		UPDATE audit_events
		SET legal_hold = FALSE,
		    legal_hold_reason = legal_hold_reason || ' [Released: ' || $1 || ']'
		WHERE correlation_id = $2 AND legal_hold = TRUE
	`
	result, err := s.db.ExecContext(ctx, updateQuery, req.ReleaseReason, correlationID)
	if err != nil {
		s.logger.Error(err, "Failed to release legal hold",
			"correlation_id", correlationID,
			"released_by", releasedBy)
		s.metrics.LegalHoldFailures.WithLabelValues("update_failed").Inc()
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to release legal hold", s.logger)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		s.logger.Error(err, "Failed to get rows affected", "correlation_id", correlationID)
	}

	// 6. Log success
	s.logger.Info("Legal hold released",
		"correlation_id", correlationID,
		"events_released", rowsAffected,
		"released_by", releasedBy,
		"release_reason", req.ReleaseReason)

	s.metrics.LegalHoldSuccesses.WithLabelValues("release").Inc()

	// 7. Return response
	response := ReleaseLegalHoldResponse{
		CorrelationID:  correlationID,
		EventsReleased: int(rowsAffected),
		ReleasedBy:     releasedBy,
		ReleasedAt:     releasedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error(err, "Failed to write release legal hold response")
	}
}

// HandleListLegalHolds lists all active legal holds
// GET /api/v1/audit/legal-hold
func (s *Server) HandleListLegalHolds(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1. Query all active legal holds
	query := `
		SELECT
			correlation_id,
			COUNT(*) as event_count,
			legal_hold_placed_by,
			legal_hold_placed_at,
			legal_hold_reason
		FROM audit_events
		WHERE legal_hold = TRUE
		GROUP BY correlation_id, legal_hold_placed_by, legal_hold_placed_at, legal_hold_reason
		ORDER BY legal_hold_placed_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		s.logger.Error(err, "Failed to query legal holds")
		s.metrics.LegalHoldFailures.WithLabelValues("query_failed").Inc()
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to query legal holds", s.logger)
		return
	}
	defer rows.Close()

	// 2. Parse results
	holds := []LegalHold{}
	for rows.Next() {
		var hold LegalHold
		var placedAt sql.NullTime
		err := rows.Scan(&hold.CorrelationID, &hold.EventsAffected, &hold.PlacedBy, &placedAt, &hold.Reason)
		if err != nil {
			s.logger.Error(err, "Failed to scan legal hold row")
			continue
		}
		if placedAt.Valid {
			hold.PlacedAt = placedAt.Time
		}
		holds = append(holds, hold)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error(err, "Error iterating legal hold rows")
		s.metrics.LegalHoldFailures.WithLabelValues("query_failed").Inc()
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to read legal holds", s.logger)
		return
	}

	// 3. Return response
	response := ListLegalHoldsResponse{
		Holds: holds,
		Total: len(holds),
	}

	s.metrics.LegalHoldSuccesses.WithLabelValues("list").Inc()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error(err, "Failed to write list legal holds response")
	}
}

