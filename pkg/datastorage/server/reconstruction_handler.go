/*
Copyright 2025 Jordi Gil.

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
	"time"

	"github.com/go-chi/chi/v5"
	"sigs.k8s.io/yaml"

	"github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
)

// ========================================
// TDD GREEN PHASE: Reconstruction Handler
// 📋 Tests Define Contract: test/unit/datastorage/reconstruction_handler_test.go
// 📋 Business Requirement: BR-AUDIT-006
// ========================================
//
// This file implements the REST API handler for RemediationRequest reconstruction.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (reconstruction_handler_test.go - 8 scenarios)
// - Handler implements MINIMAL functionality to pass tests
// - Contract defined by test expectations
//
// Business Requirements:
// - BR-AUDIT-006: RemediationRequest Reconstruction from Audit Traces
//
// OpenAPI Compliance:
// - Endpoint: POST /api/v1/audit/remediation-requests/{correlation_id}/reconstruct
// - Response: 200 OK with ReconstructionResponse
// - Errors: 400 Bad Request, 404 Not Found, 500 Internal Server Error (RFC 7807)
//
// ========================================

// ReconstructionResponse matches the OpenAPI schema
// Implements BR-AUDIT-006 reconstruction API response
type ReconstructionResponse struct {
	RemediationRequestYAML string            `json:"remediation_request_yaml"`
	Validation             ValidationResult  `json:"validation"`
	ReconstructedAt        string            `json:"reconstructed_at"`
	CorrelationID          string            `json:"correlation_id"`
}

// ValidationResult matches the OpenAPI schema
// Provides completeness metrics and validation errors/warnings
type ValidationResult struct {
	IsValid      bool     `json:"is_valid"`
	Completeness int      `json:"completeness"`
	Errors       []string `json:"errors"`
	Warnings     []string `json:"warnings"`
}

// handleReconstructRemediationRequest handles POST /api/v1/audit/remediation-requests/{correlation_id}/reconstruct
// BR-AUDIT-006: Reconstruct RemediationRequest CRD from audit trail
//
// Request: POST with correlation_id path parameter
// Success Response: 200 OK with ReconstructionResponse (RR YAML + validation results)
// Error Responses: 400 Bad Request, 404 Not Found, 500 Internal Server Error (RFC 7807)
func (s *Server) handleReconstructRemediationRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := chi.URLParam(r, "correlation_id")

	s.logger.V(1).Info("Handling RemediationRequest reconstruction request",
		"correlation_id", correlationID,
		"method", r.Method,
		"path", r.URL.Path)

	// Step 1: Query audit events from database
	events, err := reconstruction.QueryAuditEventsForReconstruction(ctx, s.db, s.logger, correlationID)
	if err != nil {
		s.logger.Error(err, "Failed to query audit events for reconstruction",
			"correlation_id", correlationID)
		response.WriteRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.ai/problems/reconstruction/query-failed",
			"Reconstruction Query Failed",
			fmt.Sprintf("Failed to query audit events: %v", err),
			s.logger,
		)
		return
	}

	if len(events) == 0 {
		s.logger.V(1).Info("No audit events found for correlation ID",
			"correlation_id", correlationID)
		response.WriteRFC7807Error(w, http.StatusNotFound,
			"https://kubernaut.ai/problems/audit/correlation-not-found",
			"Audit Events Not Found",
			fmt.Sprintf("No audit events found for correlation_id: %s", correlationID),
			s.logger,
		)
		return
	}

	// Step 2: Parse audit events to extract structured data
	parsedData := make([]reconstruction.ParsedAuditData, 0, len(events))
	for _, event := range events {
		parsed, err := reconstruction.ParseAuditEvent(event)
		if err != nil {
			s.logger.Error(err, "Failed to parse audit event",
				"correlation_id", correlationID,
				"event_id", event.EventID)
			// Continue with other events - partial reconstruction is acceptable
			continue
		}
		parsedData = append(parsedData, *parsed)
	}

	if len(parsedData) == 0 {
		s.logger.V(1).Info("No parseable audit events found",
			"correlation_id", correlationID)
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.ai/problems/reconstruction/no-parseable-events",
			"Reconstruction Failed",
			"No parseable audit events found for reconstruction",
			s.logger,
		)
		return
	}

	// Step 3: Map parsed data to RR Spec/Status fields
	rrFields, err := reconstruction.MergeAuditData(parsedData)
	if err != nil {
		s.logger.Error(err, "Failed to merge audit data",
			"correlation_id", correlationID)
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.ai/problems/reconstruction/missing-gateway-event",
			"Reconstruction Failed",
			err.Error(),
			s.logger,
		)
		return
	}

	// Step 4: Build complete RemediationRequest CRD
	rr, err := reconstruction.BuildRemediationRequest(correlationID, rrFields)
	if err != nil {
		s.logger.Error(err, "Failed to build RemediationRequest",
			"correlation_id", correlationID)
		response.WriteRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.ai/problems/reconstruction/build-failed",
			"Build Failed",
			fmt.Sprintf("Failed to build RemediationRequest: %v", err),
			s.logger,
		)
		return
	}

	// Step 5: Validate reconstructed RR
	validationResult, err := reconstruction.ValidateReconstructedRR(rr)
	if err != nil {
		s.logger.Error(err, "Failed to validate RemediationRequest",
			"correlation_id", correlationID)
		response.WriteRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.ai/problems/reconstruction/validation-failed",
			"Validation Failed",
			fmt.Sprintf("Failed to validate RemediationRequest: %v", err),
			s.logger,
		)
		return
	}

	// If completeness < 50%, return 400 error
	if validationResult.Completeness < 50 {
		s.logger.V(1).Info("Reconstruction incomplete",
			"correlation_id", correlationID,
			"completeness", validationResult.Completeness)
		response.WriteRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.ai/problems/reconstruction/incomplete-data",
			"Incomplete Reconstruction",
			fmt.Sprintf("Reconstructed RR is only %d%% complete", validationResult.Completeness),
			s.logger,
		)
		return
	}

	// Step 6: Convert RR to YAML
	yamlBytes, err := yaml.Marshal(rr)
	if err != nil {
		s.logger.Error(err, "Failed to marshal RR to YAML",
			"correlation_id", correlationID)
		response.WriteRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.ai/problems/reconstruction/yaml-marshal-failed",
			"YAML Marshaling Failed",
			fmt.Sprintf("Failed to marshal RemediationRequest to YAML: %v", err),
			s.logger,
		)
		return
	}

	// Step 7: Build response
	reconstructionResponse := ReconstructionResponse{
		RemediationRequestYAML: string(yamlBytes),
		Validation: ValidationResult{
			IsValid:      validationResult.IsValid,
			Completeness: validationResult.Completeness,
			Errors:       validationResult.Errors,
			Warnings:     validationResult.Warnings,
		},
		ReconstructedAt: time.Now().UTC().Format(time.RFC3339),
		CorrelationID:   correlationID,
	}

	// Step 8: Write JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(reconstructionResponse); err != nil {
		s.logger.Error(err, "Failed to encode reconstruction response",
			"correlation_id", correlationID)
		// Response already started, can't change status code
		return
	}

	s.logger.V(1).Info("RemediationRequest reconstruction successful",
		"correlation_id", correlationID,
		"completeness", validationResult.Completeness,
		"warnings", len(validationResult.Warnings),
		"errors", len(validationResult.Errors))
}
