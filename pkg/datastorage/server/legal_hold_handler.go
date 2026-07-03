package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	dsmiddleware "github.com/jordigilh/kubernaut/pkg/datastorage/server/middleware"
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

// parseAndAuthenticatePlaceLegalHold implements steps 1-3 of
// HandlePlaceLegalHold: decode the request body, validate that
// correlation_id/reason are present, and require the X-Auth-Request-User
// header (SOC2 compliance: DD-AUTH-014/DD-AUTH-005). On any failure it
// writes the RFC 7807 error response itself and returns ok=false. Extracted
// from HandlePlaceLegalHold (Wave 6 6f GREEN: funlen remediation) — pure
// code motion, no behavior change.
func (s *Server) parseAndAuthenticatePlaceLegalHold(w http.ResponseWriter, r *http.Request) (PlaceLegalHoldRequest, string, bool) {
	// 1. Parse request body
	var req PlaceLegalHoldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if dsmiddleware.IsMaxBytesError(err) {
			dsmiddleware.WriteMaxBytesExceeded(w, s.logger)
			return req, "", false
		}
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid-request", "Invalid Request",
			"request body is not valid JSON", s.logger)
		return req, "", false
	}

	// 2. Validate correlation_id
	if req.CorrelationID == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-correlation-id", "Missing Correlation ID",
			"correlation_id is required", s.logger)
		return req, "", false
	}

	if req.Reason == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-reason", "Missing Reason",
			"reason is required", s.logger)
		return req, "", false
	}

	// 3. Extract X-Auth-Request-User header (placed_by) - REQUIRED for SOC2 compliance
	// DD-AUTH-014: OAuth-proxy injects this header after validating JWT token + SAR
	// DD-AUTH-005: All services authenticate via oauth-proxy, which sets this header
	placedBy := r.Header.Get("X-Auth-Request-User")
	if placedBy == "" {
		response.WriteRFC7807Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized",
			"X-Auth-Request-User header is required for legal hold operations (missing authentication)", s.logger)
		return req, "", false
	}

	return req, placedBy, true
}

// HandlePlaceLegalHold places a legal hold on all events with a correlation_id
// POST /api/v1/audit/legal-hold
func (s *Server) HandlePlaceLegalHold(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1-3. Parse request body, validate required fields, and authenticate
	req, placedBy, ok := s.parseAndAuthenticatePlaceLegalHold(w, r)
	if !ok {
		return
	}

	// 4-5. Check correlation_id exists, then place the legal hold
	placedAt, rowsAffected, ok := s.placeLegalHold(ctx, w, req, placedBy)
	if !ok {
		return
	}

	// 6. Log success
	s.logger.Info("Legal hold placed",
		"correlation_id", req.CorrelationID,
		"events_affected", rowsAffected,
		"placed_by", placedBy,
		"reason", req.Reason)

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

// placeLegalHold implements steps 4-5 of HandlePlaceLegalHold: verify the
// correlation_id has at least one event, then place the legal hold on all
// of its events. On any failure it writes the RFC 7807 error response
// itself and returns ok=false. Extracted from HandlePlaceLegalHold
// (Wave 6 6f GREEN: funlen remediation) — pure code motion, no behavior
// change.
func (s *Server) placeLegalHold(ctx context.Context, w http.ResponseWriter, req PlaceLegalHoldRequest, placedBy string) (time.Time, int64, bool) {
	// 4. Check if correlation_id exists
	var eventCount int
	checkQuery := `SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1`
	if err := s.db.QueryRowContext(ctx, checkQuery, req.CorrelationID).Scan(&eventCount); err != nil {
		s.logger.Error(err, "Failed to check correlation_id existence",
			"correlation_id", req.CorrelationID)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to check correlation_id", s.logger)
		return time.Time{}, 0, false
	}

	if eventCount == 0 {
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
			fmt.Sprintf("No events found for correlation_id: %s", req.CorrelationID), s.logger)
		return time.Time{}, 0, false
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
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to place legal hold", s.logger)
		return time.Time{}, 0, false
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		s.logger.Error(err, "Failed to get rows affected", "correlation_id", req.CorrelationID)
	}

	return placedAt, rowsAffected, true
}

// HandleReleaseLegalHold releases a legal hold on all events with a correlation_id
// DELETE /api/v1/audit/legal-hold/{correlation_id}
func (s *Server) HandleReleaseLegalHold(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1. Extract correlation_id from URL path
	correlationID, req, releasedBy, ok := s.parseAndAuthenticateReleaseLegalHold(w, r)
	if !ok {
		return
	}

	// 4. Check if correlation_id has any events with legal hold
	var holdCount int
	checkQuery := `SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1 AND legal_hold = TRUE`
	err := s.db.QueryRowContext(ctx, checkQuery, correlationID).Scan(&holdCount)
	if err != nil {
		s.logger.Error(err, "Failed to check legal hold existence", "correlation_id", correlationID)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to check legal hold", s.logger)
		return
	}

	if holdCount == 0 {
		response.WriteRFC7807Error(w, http.StatusNotFound, "not-found", "Not Found",
			fmt.Sprintf("No legal hold found for correlation_id: %s", correlationID), s.logger)
		return
	}

	// 5. Release legal hold within a transaction that authorizes the SOC2 trigger bypass
	releasedAt := time.Now()
	rowsAffected, ok := s.releaseLegalHoldTx(ctx, w, correlationID, req.ReleaseReason, releasedBy)
	if !ok {
		return
	}

	// 6. Log success
	s.logger.Info("Legal hold released",
		"correlation_id", correlationID,
		"events_released", rowsAffected,
		"released_by", releasedBy,
		"release_reason", req.ReleaseReason)

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

// parseAndAuthenticateReleaseLegalHold implements steps 1-3 of
// HandleReleaseLegalHold: extract the correlation_id path param, decode the
// release-reason request body, and require the X-Auth-Request-User header
// (SOC2 compliance: DD-AUTH-014/DD-AUTH-005). On any failure it writes the
// RFC 7807 error response itself and returns ok=false. Extracted from
// HandleReleaseLegalHold (Wave 6 6f GREEN: funlen remediation) — pure code
// motion, no behavior change.
func (s *Server) parseAndAuthenticateReleaseLegalHold(w http.ResponseWriter, r *http.Request) (string, ReleaseLegalHoldRequest, string, bool) {
	// 1. Extract correlation_id from URL path
	correlationID := chi.URLParam(r, "correlation_id")
	if correlationID == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-correlation-id", "Missing Correlation ID",
			"correlation_id is required in URL path", s.logger)
		return "", ReleaseLegalHoldRequest{}, "", false
	}

	// 2. Parse request body (release_reason)
	var req ReleaseLegalHoldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if dsmiddleware.IsMaxBytesError(err) {
			dsmiddleware.WriteMaxBytesExceeded(w, s.logger)
			return "", ReleaseLegalHoldRequest{}, "", false
		}
		response.WriteRFC7807Error(w, http.StatusBadRequest, "invalid-request", "Invalid Request",
			"request body is not valid JSON", s.logger)
		return "", ReleaseLegalHoldRequest{}, "", false
	}

	if req.ReleaseReason == "" {
		response.WriteRFC7807Error(w, http.StatusBadRequest, "missing-release-reason", "Missing Release Reason",
			"release_reason is required", s.logger)
		return "", ReleaseLegalHoldRequest{}, "", false
	}

	// 3. Extract X-Auth-Request-User header (released_by) - REQUIRED for SOC2 compliance
	// DD-AUTH-014: OAuth-proxy injects this header after validating JWT token + SAR
	// DD-AUTH-005: All services authenticate via oauth-proxy, which sets this header
	releasedBy := r.Header.Get("X-Auth-Request-User")
	if releasedBy == "" {
		response.WriteRFC7807Error(w, http.StatusUnauthorized, "unauthorized", "Unauthorized",
			"X-Auth-Request-User header is required for legal hold operations (missing authentication)", s.logger)
		return "", ReleaseLegalHoldRequest{}, "", false
	}

	return correlationID, req, releasedBy, true
}

// releaseLegalHoldTx implements step 5 of HandleReleaseLegalHold: release the
// legal hold within a transaction that authorizes the SOC2 audit-trigger
// bypass (`SET LOCAL kubernaut.legal_hold_release`), then commits. On any
// failure it writes the RFC 7807 error response itself, rolls back via the
// deferred rollback, and returns ok=false. Extracted from
// HandleReleaseLegalHold (Wave 6 6f GREEN: funlen remediation) — pure code
// motion, no behavior change.
func (s *Server) releaseLegalHoldTx(ctx context.Context, w http.ResponseWriter, correlationID, releaseReason, releasedBy string) (int64, bool) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.Error(err, "Failed to begin transaction for legal hold release",
			"correlation_id", correlationID)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to release legal hold", s.logger)
		return 0, false
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, "SET LOCAL kubernaut.legal_hold_release = 'authorized'"); err != nil {
		s.logger.Error(err, "Failed to set legal hold release authorization",
			"correlation_id", correlationID)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to release legal hold", s.logger)
		return 0, false
	}

	updateQuery := `
		UPDATE audit_events
		SET legal_hold = FALSE,
		    legal_hold_reason = legal_hold_reason || ' [Released: ' || $1 || ']'
		WHERE correlation_id = $2 AND legal_hold = TRUE
	`
	result, err := tx.ExecContext(ctx, updateQuery, releaseReason, correlationID)
	if err != nil {
		s.logger.Error(err, "Failed to release legal hold",
			"correlation_id", correlationID,
			"released_by", releasedBy)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to release legal hold", s.logger)
		return 0, false
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		s.logger.Error(err, "Failed to get rows affected", "correlation_id", correlationID)
	}

	if err = tx.Commit(); err != nil {
		s.logger.Error(err, "Failed to commit legal hold release",
			"correlation_id", correlationID)
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to release legal hold", s.logger)
		return 0, false
	}

	return rowsAffected, true
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
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to query legal holds", s.logger)
		return
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Error(err, "Failed to close database rows")
		}
	}()

	// 2. Parse results
	holds, ok := scanLegalHoldRows(w, s, rows)
	if !ok {
		return
	}

	// 3. Return response
	response := ListLegalHoldsResponse{
		Holds: holds,
		Total: len(holds),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error(err, "Failed to write list legal holds response")
	}
}

// scanLegalHoldRows implements step 2 of HandleListLegalHolds: scan each row
// into a LegalHold, coalescing nullable columns to their zero value. On any
// scan/iteration failure it writes the RFC 7807 error response itself and
// returns ok=false. Extracted from HandleListLegalHolds (Wave 6 6f GREEN:
// funlen remediation) — pure code motion, no behavior change.
func scanLegalHoldRows(w http.ResponseWriter, s *Server, rows *sql.Rows) ([]LegalHold, bool) {
	holds := []LegalHold{}
	for rows.Next() {
		var hold LegalHold
		var placedAt sql.NullTime
		var placedBy sql.NullString
		var reason sql.NullString
		if err := rows.Scan(&hold.CorrelationID, &hold.EventsAffected, &placedBy, &placedAt, &reason); err != nil {
			s.logger.Error(err, "Failed to scan legal hold row")
			response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
				"Failed to scan legal hold data", s.logger)
			return nil, false
		}
		if placedAt.Valid {
			hold.PlacedAt = placedAt.Time
		}
		if placedBy.Valid {
			hold.PlacedBy = placedBy.String
		}
		if reason.Valid {
			hold.Reason = reason.String
		}
		holds = append(holds, hold)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error(err, "Error iterating legal hold rows")
		response.WriteRFC7807Error(w, http.StatusInternalServerError, "database-error", "Database Error",
			"Failed to read legal holds", s.logger)
		return nil, false
	}

	return holds, true
}
