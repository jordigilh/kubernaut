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
	"context"
	"fmt"
	"net/url"
	"time"

	"sigs.k8s.io/yaml"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
)

// ========================================
// TDD GREEN PHASE: Ogen Handler for Reconstruction
// 📋 Business Requirement: BR-AUDIT-006
// 📋 Implements: ogen Handler interface for ReconstructRemediationRequest
// ========================================

// ReconstructRemediationRequest implements the ogen Handler interface for RR reconstruction.
// This method is called by the ogen-generated server when the reconstruction endpoint is hit.
//
// BR-AUDIT-006: Reconstruct RemediationRequest CRD from audit trail
// SOC2 Compliance: Enable complete reconstruction of RR state from immutable audit events
//
// Workflow:
//  1. Query audit events for correlation_id
//  2. Parse events to extract structured data
//  3. Merge data into RR fields
//  4. Build complete RR CRD
//  5. Validate and return
func (h *Handler) ReconstructRemediationRequest(
	ctx context.Context,
	params ogenclient.ReconstructRemediationRequestParams,
) (ogenclient.ReconstructRemediationRequestRes, error) {
	correlationID := params.CorrelationID

	h.logger.V(1).Info("Handling RemediationRequest reconstruction via ogen handler",
		"correlation_id", correlationID)

	// Step 1: Query audit events from database
	events, err := reconstruction.QueryAuditEventsForReconstruction(ctx, h.sqlDB, h.logger, correlationID)
	if err != nil {
		h.logger.Error(err, "Failed to query audit events for reconstruction",
			"correlation_id", correlationID)
		typeURL, _ := url.Parse("https://kubernaut.ai/problems/reconstruction/query-failed")
		return &ogenclient.ReconstructRemediationRequestInternalServerError{
			Type:   *typeURL,                      // url.URL (dereference pointer)
			Title:  "Reconstruction Query Failed", // string (not OptString)
			Status: 500,                           // int32
			Detail: ogenclient.NewOptString(fmt.Sprintf("Failed to query audit events: %v", err)),
		}, nil // Return error response as success (ogen pattern)
	}

	if len(events) == 0 {
		h.logger.V(1).Info("No audit events found for correlation ID",
			"correlation_id", correlationID)
		typeURL, _ := url.Parse("https://kubernaut.ai/problems/audit/correlation-not-found")
		return &ogenclient.ReconstructRemediationRequestNotFound{
			Type:   *typeURL,                 // url.URL (dereference pointer)
			Title:  "Audit Events Not Found", // string (not OptString)
			Status: 404,                      // int32
			Detail: ogenclient.NewOptString(fmt.Sprintf("No audit events found for correlation_id: %s", correlationID)),
		}, nil // Return 404 as success (ogen pattern)
	}

	// Step 2: Parse audit events to extract structured data
	parsedData := make([]reconstruction.ParsedAuditData, 0, len(events))
	for _, event := range events {
		parsed, err := reconstruction.ParseAuditEvent(event)
		if err != nil {
			h.logger.Error(err, "Failed to parse audit event",
				"correlation_id", correlationID,
				"event_id", event.EventID)
			// Continue with other events - partial reconstruction is acceptable
			continue
		}
		parsedData = append(parsedData, *parsed)
	}

	if len(parsedData) == 0 {
		h.logger.V(1).Info("No parseable audit events found",
			"correlation_id", correlationID)
		typeURL, _ := url.Parse("https://kubernaut.ai/problems/reconstruction/no-parseable-events")
		return &ogenclient.ReconstructRemediationRequestBadRequest{
			Type:   *typeURL,                // url.URL (dereference pointer)
			Title:  "Reconstruction Failed", // string (not OptString)
			Status: 400,                     // int32
			Detail: ogenclient.NewOptString("No parseable audit events found for reconstruction"),
		}, nil // Return 400 as success (ogen pattern)
	}

	// Step 3: Map parsed data to RR Spec/Status fields
	rrFields, err := reconstruction.MergeAuditData(parsedData)
	if err != nil {
		h.logger.Error(err, "Failed to merge audit data",
			"correlation_id", correlationID)
		typeURL, _ := url.Parse("https://kubernaut.ai/problems/reconstruction/missing-gateway-event")
		return &ogenclient.ReconstructRemediationRequestBadRequest{
			Type:   *typeURL,
			Title:  "Reconstruction Failed",
			Status: 400,
			Detail: ogenclient.NewOptString(err.Error()),
		}, nil // Return 400 as success (ogen pattern)
	}

	// Step 4: Build complete RemediationRequest CRD
	rr, err := reconstruction.BuildRemediationRequest(correlationID, rrFields)
	if err != nil {
		h.logger.Error(err, "Failed to build RemediationRequest",
			"correlation_id", correlationID)
		typeURL, _ := url.Parse("https://kubernaut.ai/problems/reconstruction/build-failed")
		return &ogenclient.ReconstructRemediationRequestInternalServerError{
			Type:   *typeURL,
			Title:  "Build Failed",
			Status: 500,
			Detail: ogenclient.NewOptString(fmt.Sprintf("Failed to build RemediationRequest: %v", err)),
		}, nil // Return 500 as success (ogen pattern)
	}

	// Step 5: Validate reconstructed RR
	validationResult, err := reconstruction.ValidateReconstructedRR(rr)
	if err != nil {
		h.logger.Error(err, "Failed to validate RemediationRequest",
			"correlation_id", correlationID)
		typeURL, _ := url.Parse("https://kubernaut.ai/problems/reconstruction/validation-failed")
		return &ogenclient.ReconstructRemediationRequestInternalServerError{
			Type:   *typeURL,
			Title:  "Validation Failed",
			Status: 500,
			Detail: ogenclient.NewOptString(fmt.Sprintf("Failed to validate RemediationRequest: %v", err)),
		}, nil // Return 500 as success (ogen pattern)
	}

	// If completeness < 50%, return 400 error
	if validationResult.Completeness < 50 {
		h.logger.V(1).Info("Reconstruction incomplete",
			"correlation_id", correlationID,
			"completeness", validationResult.Completeness)
		typeURL, _ := url.Parse("https://kubernaut.ai/problems/reconstruction/incomplete-data")
		return &ogenclient.ReconstructRemediationRequestBadRequest{
			Type:   *typeURL,
			Title:  "Incomplete Reconstruction",
			Status: 400,
			Detail: ogenclient.NewOptString(fmt.Sprintf("Reconstructed RR is only %d%% complete", validationResult.Completeness)),
		}, nil // Return 400 as success (ogen pattern)
	}

	// Step 6: Convert RR to YAML
	yamlBytes, err := yaml.Marshal(rr)
	if err != nil {
		h.logger.Error(err, "Failed to marshal RR to YAML",
			"correlation_id", correlationID)
		typeURL, _ := url.Parse("https://kubernaut.ai/problems/reconstruction/yaml-marshal-failed")
		return &ogenclient.ReconstructRemediationRequestInternalServerError{
			Type:   *typeURL,
			Title:  "YAML Marshaling Failed",
			Status: 500,
			Detail: ogenclient.NewOptString(fmt.Sprintf("Failed to marshal RemediationRequest to YAML: %v", err)),
		}, nil // Return 500 as success (ogen pattern)
	}

	// Step 7: Build ogen response
	reconstructedAt := time.Now().UTC()
	response := &ogenclient.ReconstructionResponse{
		RemediationRequestYaml: string(yamlBytes),
		Validation: ogenclient.ValidationResult{
			IsValid:      validationResult.IsValid,
			Completeness: validationResult.Completeness,
			Errors:       validationResult.Errors,
			Warnings:     validationResult.Warnings,
		},
		ReconstructedAt: ogenclient.NewOptDateTime(reconstructedAt),
		CorrelationID:   ogenclient.NewOptString(correlationID),
	}

	h.logger.V(1).Info("RemediationRequest reconstruction successful (ogen handler)",
		"correlation_id", correlationID,
		"completeness", validationResult.Completeness,
		"warnings", len(validationResult.Warnings),
		"errors", len(validationResult.Errors))

	return response, nil
}
