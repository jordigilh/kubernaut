package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/contextapi/datastorage"
)

// ========================================
// DAY 11 TDD GREEN: HTTP Aggregation Handlers
// BR-INTEGRATION-008, BR-INTEGRATION-009, BR-INTEGRATION-010
// ========================================
//
// **OBJECTIVE**: Minimal implementation to make unit tests pass
//
// **TDD GREEN Phase**: Implement only what tests require
// ========================================

// ========================================
// BR-INTEGRATION-008: Incident-Type Success Rate API
// ========================================

// HandleGetSuccessRateByIncidentType handles GET /api/v1/aggregation/success-rate/incident-type
func (s *Server) HandleGetSuccessRateByIncidentType(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	incidentType := r.URL.Query().Get("incident_type")
	timeRange := r.URL.Query().Get("time_range")
	if timeRange == "" {
		timeRange = "7d"
	}
	minSamples := 5
	if ms := r.URL.Query().Get("min_samples"); ms != "" {
		if parsed, err := strconv.Atoi(ms); err == nil {
			minSamples = parsed
		}
	}

	// Validate required parameters
	if incidentType == "" {
		respondRFC7807Error(w, http.StatusBadRequest, "bad-request", "incident_type parameter is required")
		return
	}

	// Call AggregationService
	result, err := s.aggregationService.GetSuccessRateByIncidentType(r.Context(), incidentType, timeRange, minSamples)
	if err != nil {
		s.logger.Error("failed to get success rate by incident type",
			zap.String("incident_type", incidentType),
			zap.Error(err))

		if isTimeoutError(err) {
			respondRFC7807Error(w, http.StatusServiceUnavailable, "service-unavailable", "Data Storage Service timeout")
			return
		}

		respondRFC7807Error(w, http.StatusInternalServerError, "internal-server-error", "failed to retrieve success rate data")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// ========================================
// BR-INTEGRATION-009: Playbook Success Rate API
// ========================================

// HandleGetSuccessRateByPlaybook handles GET /api/v1/aggregation/success-rate/playbook
func (s *Server) HandleGetSuccessRateByPlaybook(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	playbookID := r.URL.Query().Get("playbook_id")
	playbookVersion := r.URL.Query().Get("playbook_version")
	timeRange := r.URL.Query().Get("time_range")
	if timeRange == "" {
		timeRange = "7d"
	}
	minSamples := 5
	if ms := r.URL.Query().Get("min_samples"); ms != "" {
		if parsed, err := strconv.Atoi(ms); err == nil {
			minSamples = parsed
		}
	}

	// Validate required parameters
	if playbookID == "" {
		respondRFC7807Error(w, http.StatusBadRequest, "bad-request", "playbook_id parameter is required")
		return
	}

	// Call AggregationService
	result, err := s.aggregationService.GetSuccessRateByPlaybook(r.Context(), playbookID, playbookVersion, timeRange, minSamples)
	if err != nil {
		s.logger.Error("failed to get success rate by playbook",
			zap.String("playbook_id", playbookID),
			zap.Error(err))

		if isTimeoutError(err) {
			respondRFC7807Error(w, http.StatusServiceUnavailable, "service-unavailable", "Data Storage Service timeout")
			return
		}

		respondRFC7807Error(w, http.StatusInternalServerError, "internal-server-error", "failed to retrieve playbook success rate data")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// ========================================
// BR-INTEGRATION-010: Multi-Dimensional Success Rate API
// ========================================

// HandleGetSuccessRateMultiDimensional handles GET /api/v1/aggregation/success-rate/multi-dimensional
func (s *Server) HandleGetSuccessRateMultiDimensional(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	incidentType := r.URL.Query().Get("incident_type")
	playbookID := r.URL.Query().Get("playbook_id")
	playbookVersion := r.URL.Query().Get("playbook_version")
	actionType := r.URL.Query().Get("action_type")
	timeRange := r.URL.Query().Get("time_range")
	if timeRange == "" {
		timeRange = "7d"
	}
	minSamples := 5
	if ms := r.URL.Query().Get("min_samples"); ms != "" {
		if parsed, err := strconv.Atoi(ms); err == nil {
			minSamples = parsed
		}
	}

	// Validate at least one dimension
	if incidentType == "" && playbookID == "" && actionType == "" {
		respondRFC7807Error(w, http.StatusBadRequest, "bad-request", "at least one dimension (incident_type, playbook_id, or action_type) must be specified")
		return
	}

	// Build query
	query := &datastorage.MultiDimensionalQuery{
		IncidentType:    incidentType,
		PlaybookID:      playbookID,
		PlaybookVersion: playbookVersion,
		ActionType:      actionType,
		TimeRange:       timeRange,
		MinSamples:      minSamples,
	}

	// Call AggregationService
	result, err := s.aggregationService.GetSuccessRateMultiDimensional(r.Context(), query)
	if err != nil {
		s.logger.Error("failed to get multi-dimensional success rate",
			zap.String("incident_type", incidentType),
			zap.Error(err))

		if isTimeoutError(err) {
			respondRFC7807Error(w, http.StatusServiceUnavailable, "service-unavailable", "Data Storage Service timeout")
			return
		}

		respondRFC7807Error(w, http.StatusInternalServerError, "internal-server-error", "failed to retrieve multi-dimensional success rate data")
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// ========================================
// Helper Functions (TDD GREEN - Minimal)
// ========================================

// respondJSON writes JSON response
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// respondRFC7807Error writes RFC 7807 error response
func respondRFC7807Error(w http.ResponseWriter, statusCode int, problemType string, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	errorResp := map[string]interface{}{
		"type":   fmt.Sprintf("https://kubernaut.io/problems/%s", problemType),
		"title":  http.StatusText(statusCode),
		"status": statusCode,
		"detail": detail,
	}
	json.NewEncoder(w).Encode(errorResp)
}

// isTimeoutError checks if error is timeout
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if err == context.DeadlineExceeded {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "context")
}

