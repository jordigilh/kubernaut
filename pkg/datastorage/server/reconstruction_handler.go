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

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
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

// handleReconstructRemediationRequestWrapper is a Chi router wrapper that delegates to the ogen handler
// This bridges between Chi's http.HandlerFunc and our ogen Handler implementation
func (s *Server) handleReconstructRemediationRequestWrapper(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	correlationID := chi.URLParam(r, "correlation_id")

	// Call the ogen handler method
	result, err := s.handler.ReconstructRemediationRequest(ctx, ogenclient.ReconstructRemediationRequestParams{
		CorrelationID: correlationID,
	})

	if err != nil {
		// Unexpected error (ogen handler should return errors as response types, not err)
		s.logger.Error(err, "Unexpected error from reconstruction handler",
			"correlation_id", correlationID)
		response.WriteRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.ai/problems/reconstruction/unexpected-error",
			"Unexpected Error",
			fmt.Sprintf("Unexpected error: %v", err),
			s.logger,
		)
		return
	}

	// Handle response types
	switch resp := result.(type) {
	case *ogenclient.ReconstructionResponse:
		// Success - write JSON response
		responseData := ReconstructionResponse{
			RemediationRequestYAML: resp.RemediationRequestYaml,
			Validation: ValidationResult{
				IsValid:      resp.Validation.IsValid,
				Completeness: resp.Validation.Completeness,
				Errors:       resp.Validation.Errors,
				Warnings:     resp.Validation.Warnings,
			},
			ReconstructedAt: resp.ReconstructedAt.Value.Format(time.RFC3339),
			CorrelationID:   resp.CorrelationID.Value,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(responseData); err != nil {
			s.logger.Error(err, "Failed to encode response")
		}

	case *ogenclient.ReconstructRemediationRequestBadRequest:
		// 400 error
		response.WriteRFC7807Error(w, int(resp.Status),
			resp.Type.String(),
			resp.Title,
			resp.Detail.Value,
			s.logger,
		)

	case *ogenclient.ReconstructRemediationRequestNotFound:
		// 404 error
		response.WriteRFC7807Error(w, int(resp.Status),
			resp.Type.String(),
			resp.Title,
			resp.Detail.Value,
			s.logger,
		)

	case *ogenclient.ReconstructRemediationRequestInternalServerError:
		// 500 error
		response.WriteRFC7807Error(w, int(resp.Status),
			resp.Type.String(),
			resp.Title,
			resp.Detail.Value,
			s.logger,
		)

	default:
		s.logger.Error(fmt.Errorf("unknown response type: %T", resp), "Unknown response type from handler")
		response.WriteRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.ai/problems/reconstruction/unknown-response",
			"Unknown Response Type",
			fmt.Sprintf("Unexpected response type: %T", resp),
			s.logger,
		)
	}
}
