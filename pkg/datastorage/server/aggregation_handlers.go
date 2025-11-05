/*
Copyright 2025 Jordi Gil Heredia.

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
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// ========================================
// TDD REFACTOR PHASE: ADR-033 Aggregation Handlers
// 📋 Authority: test/unit/datastorage/aggregation_handlers_test.go
// 📋 Tests Define Contract: Unit tests drive implementation
// ========================================
//
// This file implements ADR-033 multi-dimensional success tracking HTTP handlers.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (aggregation_handlers_test.go) - RED phase
// - Minimal implementation to pass tests - GREEN phase
// - Enhanced with real repository integration - REFACTOR phase
// - Contract defined by test expectations
//
// Business Requirements:
// - BR-STORAGE-031-01: Incident-Type Success Rate API
// - BR-STORAGE-031-02: Playbook Success Rate API
//
// ========================================

// timeRangeRegex validates time range format (e.g., "1h", "24h", "7d", "30d")
var timeRangeRegex = regexp.MustCompile(`^(\d+)(h|d)$`)

// HandleGetSuccessRateByIncidentType handles GET /api/v1/success-rate/incident-type
// BR-STORAGE-031-01: Calculate success rate by incident type
//
// Query Parameters:
//   - incident_type (required): The incident type to query (e.g., "HighCPUUsage")
//   - time_range (optional): Time window for analysis (default: "7d")
//     Valid formats: "1h", "24h", "7d", "30d"
//   - min_samples (optional): Minimum sample size for confidence (default: 5)
//     Must be positive integer
//
// Response: 200 OK with IncidentTypeSuccessRateResponse JSON
// Errors: 400 Bad Request (validation), 500 Internal Server Error (repository)
func (h *Handler) HandleGetSuccessRateByIncidentType(w http.ResponseWriter, r *http.Request) {
	// 1. Parse and validate query parameters
	incidentType := r.URL.Query().Get("incident_type")
	if incidentType == "" {
		h.respondWithRFC7807(w, http.StatusBadRequest, validation.RFC7807Problem{
			Type:   "https://api.kubernaut.io/problems/validation-error",
			Title:  "Validation Error",
			Status: http.StatusBadRequest,
			Detail: "incident_type query parameter is required",
		})
		return
	}

	timeRange := r.URL.Query().Get("time_range")
	if timeRange == "" {
		timeRange = "7d" // Default to 7 days
	}

	// Validate time range format
	if _, err := parseTimeRange(timeRange); err != nil {
		h.respondWithRFC7807(w, http.StatusBadRequest, validation.RFC7807Problem{
			Type:   "https://api.kubernaut.io/problems/validation-error",
			Title:  "Validation Error",
			Status: http.StatusBadRequest,
			Detail: fmt.Sprintf("invalid time_range format: %s (expected format: 1h, 24h, 7d, 30d)", timeRange),
		})
		return
	}

	minSamplesStr := r.URL.Query().Get("min_samples")
	minSamples := 5 // Default minimum samples
	if minSamplesStr != "" {
		parsed, err := strconv.Atoi(minSamplesStr)
		if err != nil {
			h.respondWithRFC7807(w, http.StatusBadRequest, validation.RFC7807Problem{
				Type:   "https://api.kubernaut.io/problems/validation-error",
				Title:  "Validation Error",
				Status: http.StatusBadRequest,
				Detail: fmt.Sprintf("invalid min_samples: must be a positive integer, got %s", minSamplesStr),
			})
			return
		}
		if parsed <= 0 {
			h.respondWithRFC7807(w, http.StatusBadRequest, validation.RFC7807Problem{
				Type:   "https://api.kubernaut.io/problems/validation-error",
				Title:  "Validation Error",
				Status: http.StatusBadRequest,
				Detail: fmt.Sprintf("invalid min_samples: must be positive, got %d", parsed),
			})
			return
		}
		minSamples = parsed
	}

	// 2. Call repository to get success rate data
	// TDD REFACTOR: Connect to real repository layer
	duration, _ := parseTimeRange(timeRange) // Already validated above

	var response *models.IncidentTypeSuccessRateResponse
	var err error

	if h.actionTraceRepository != nil {
		// Production: Use real repository
		response, err = h.actionTraceRepository.GetSuccessRateByIncidentType(
			r.Context(),
			incidentType,
			duration,
			minSamples,
		)
		if err != nil {
			h.respondWithRFC7807(w, http.StatusInternalServerError, validation.RFC7807Problem{
				Type:   "https://api.kubernaut.io/problems/internal-error",
				Title:  "Internal Server Error",
				Status: http.StatusInternalServerError,
				Detail: "Failed to retrieve success rate data",
			})
			h.logger.Error("repository error",
				zap.String("incident_type", incidentType),
				zap.Error(err))
			return
		}
	} else {
		// Test mode: Return minimal response (for unit tests without repository)
		response = &models.IncidentTypeSuccessRateResponse{
			IncidentType:         incidentType,
			TimeRange:            timeRange,
			TotalExecutions:      0,
			SuccessfulExecutions: 0,
			FailedExecutions:     0,
			SuccessRate:          0.0,
			Confidence:           "insufficient_data",
			MinSamplesMet:        false,
		}
	}

	// 3. Return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("failed to encode response",
			zap.Error(err))
	}

	// Log for observability
	h.logger.Debug("incident-type success rate query",
		zap.String("incident_type", incidentType),
		zap.String("time_range", timeRange),
		zap.Int("min_samples", minSamples),
		zap.Float64("success_rate", response.SuccessRate),
		zap.String("confidence", response.Confidence))
}

// HandleGetSuccessRateByPlaybook handles GET /api/v1/success-rate/playbook
// BR-STORAGE-031-02: Calculate success rate by playbook
//
// Query Parameters:
//   - playbook_id (required): The playbook identifier to query (e.g., "restart-pod-v1")
//   - playbook_version (optional): Specific playbook version (e.g., "1.2.3")
//   - time_range (optional): Time window for analysis (default: "7d")
//     Valid formats: "1h", "24h", "7d", "30d"
//   - min_samples (optional): Minimum sample size for confidence (default: 5)
//     Must be positive integer
//
// Response: 200 OK with PlaybookSuccessRateResponse JSON
// Errors: 400 Bad Request (validation), 500 Internal Server Error (repository)
func (h *Handler) HandleGetSuccessRateByPlaybook(w http.ResponseWriter, r *http.Request) {
	// 1. Parse and validate query parameters
	playbookID := r.URL.Query().Get("playbook_id")
	if playbookID == "" {
		h.respondWithRFC7807(w, http.StatusBadRequest, validation.RFC7807Problem{
			Type:   "https://api.kubernaut.io/problems/validation-error",
			Title:  "Validation Error",
			Status: http.StatusBadRequest,
			Detail: "playbook_id query parameter is required",
		})
		return
	}

	playbookVersion := r.URL.Query().Get("playbook_version") // Optional

	timeRange := r.URL.Query().Get("time_range")
	if timeRange == "" {
		timeRange = "7d" // Default to 7 days
	}

	// Validate time range format
	if _, err := parseTimeRange(timeRange); err != nil {
		h.respondWithRFC7807(w, http.StatusBadRequest, validation.RFC7807Problem{
			Type:   "https://api.kubernaut.io/problems/validation-error",
			Title:  "Validation Error",
			Status: http.StatusBadRequest,
			Detail: fmt.Sprintf("invalid time_range format: %s (expected format: 1h, 24h, 7d, 30d)", timeRange),
		})
		return
	}

	minSamplesStr := r.URL.Query().Get("min_samples")
	minSamples := 5 // Default minimum samples
	if minSamplesStr != "" {
		parsed, err := strconv.Atoi(minSamplesStr)
		if err != nil {
			h.respondWithRFC7807(w, http.StatusBadRequest, validation.RFC7807Problem{
				Type:   "https://api.kubernaut.io/problems/validation-error",
				Title:  "Validation Error",
				Status: http.StatusBadRequest,
				Detail: fmt.Sprintf("invalid min_samples: must be a positive integer, got %s", minSamplesStr),
			})
			return
		}
		if parsed <= 0 {
			h.respondWithRFC7807(w, http.StatusBadRequest, validation.RFC7807Problem{
				Type:   "https://api.kubernaut.io/problems/validation-error",
				Title:  "Validation Error",
				Status: http.StatusBadRequest,
				Detail: fmt.Sprintf("invalid min_samples: must be positive, got %d", parsed),
			})
			return
		}
		minSamples = parsed
	}

	// 2. Call repository to get success rate data
	// TDD REFACTOR: Connect to real repository layer
	duration, _ := parseTimeRange(timeRange) // Already validated above

	var response *models.PlaybookSuccessRateResponse
	var err error

	if h.actionTraceRepository != nil {
		// Production: Use real repository
		response, err = h.actionTraceRepository.GetSuccessRateByPlaybook(
			r.Context(),
			playbookID,
			playbookVersion,
			duration,
			minSamples,
		)
		if err != nil {
			h.respondWithRFC7807(w, http.StatusInternalServerError, validation.RFC7807Problem{
				Type:   "https://api.kubernaut.io/problems/internal-error",
				Title:  "Internal Server Error",
				Status: http.StatusInternalServerError,
				Detail: "Failed to retrieve success rate data",
			})
			h.logger.Error("repository error",
				zap.String("playbook_id", playbookID),
				zap.String("playbook_version", playbookVersion),
				zap.Error(err))
			return
		}
	} else {
		// Test mode: Return minimal response (for unit tests without repository)
		response = &models.PlaybookSuccessRateResponse{
			PlaybookID:           playbookID,
			PlaybookVersion:      playbookVersion,
			TimeRange:            timeRange,
			TotalExecutions:      0,
			SuccessfulExecutions: 0,
			FailedExecutions:     0,
			SuccessRate:          0.0,
			Confidence:           "insufficient_data",
			MinSamplesMet:        false,
		}
	}

	// 3. Return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("failed to encode response",
			zap.Error(err))
	}

	// Log for observability
	h.logger.Debug("playbook success rate query",
		zap.String("playbook_id", playbookID),
		zap.String("playbook_version", playbookVersion),
		zap.String("time_range", timeRange),
		zap.Int("min_samples", minSamples),
		zap.Float64("success_rate", response.SuccessRate),
		zap.String("confidence", response.Confidence))
}

// parseTimeRange converts time range string to time.Duration
// Valid formats: "1h", "24h", "7d", "30d"
// Returns error for invalid formats
func parseTimeRange(timeRange string) (time.Duration, error) {
	matches := timeRangeRegex.FindStringSubmatch(timeRange)
	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid time range format: %s", timeRange)
	}

	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("invalid time range value: %s", matches[1])
	}

	if value <= 0 {
		return 0, fmt.Errorf("time range value must be positive: %d", value)
	}

	unit := matches[2]
	switch unit {
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid time range unit: %s", unit)
	}
}

// respondWithRFC7807 sends an RFC 7807 Problem Details error response
// This is a helper method to ensure consistent error responses across handlers
func (h *Handler) respondWithRFC7807(w http.ResponseWriter, statusCode int, problem validation.RFC7807Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		h.logger.Error("failed to encode RFC 7807 error response")
	}
}

// ========================================
// FUTURE: Repository Integration
// ========================================
//
// When ActionTraceRepository is added to Handler struct, update these handlers to:
// 1. Call repository.GetSuccessRateByIncidentType() / GetSuccessRateByPlaybook()
// 2. Handle repository errors (return 500 Internal Server Error)
// 3. Return actual data from database instead of minimal response
//
// Example:
// response, err := h.actionTraceRepo.GetSuccessRateByIncidentType(
//     r.Context(),
//     incidentType,
//     duration,
//     minSamples,
// )
// if err != nil {
//     h.respondWithRFC7807(w, http.StatusInternalServerError, validation.RFC7807Problem{
//         Type:   "https://api.kubernaut.io/problems/internal-error",
//         Title:  "Internal Server Error",
//         Status: http.StatusInternalServerError,
//         Detail: "Failed to retrieve success rate data",
//     })
//     return
// }
//
// ========================================
