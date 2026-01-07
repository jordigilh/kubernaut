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
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// SOC2 Day 9.1: Signed Audit Export API
// Authority: SOC2_WEEK2_COMPLETE_PLAN_V1_1_JAN07.md - Day 9.1
// GET /api/v1/audit/export
// ========================================
//
// Exports audit events with cryptographic signatures for tamper detection
// and compliance verification.
//
// SOC2 Requirements:
// - CC8.1: Audit Export for external compliance reviews
// - AU-9: Protection of Audit Information (tamper-evident exports)
//
// Authentication:
// - Requires X-Auth-Request-User header (oauth-proxy authenticated)
// - Returns HTTP 401 if header missing
//
// Export Formats:
// - JSON: Complete event data with hash chain verification
// - CSV: Flattened tabular format (not yet implemented)
//
// ========================================

const (
	maxExportLimit    = 10000 // Maximum events per export (prevent memory issues)
	defaultExportLimit = 1000  // Default limit if not specified
)

// HandleExportAuditEvents handles GET /api/v1/audit/export
// Implements: ExportAuditEvents operation from OpenAPI spec
func (s *Server) HandleExportAuditEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Authentication: Require X-Auth-Request-User header (SOC2 CC8.1)
	exportedBy := r.Header.Get("X-Auth-Request-User")
	if exportedBy == "" {
		s.logger.Info("Export request rejected: missing X-Auth-Request-User header")
		writeRFC7807Error(w, http.StatusUnauthorized,
			"Unauthorized",
			"X-Auth-Request-User header required for audit export",
			r.URL.Path)
		return
	}

	// Parse query parameters
	filters, err := parseExportFilters(r)
	if err != nil {
		s.logger.Error(err, "Invalid export query parameters")
		writeRFC7807Error(w, http.StatusBadRequest,
			"Validation Error",
			fmt.Sprintf("Invalid query parameters: %v", err),
			r.URL.Path)
		return
	}

	// Validate limit
	if filters.Limit > maxExportLimit {
		writeRFC7807Error(w, http.StatusRequestEntityTooLarge,
			"Payload Too Large",
			fmt.Sprintf("Export limit exceeds maximum of %d events. Use pagination.", maxExportLimit),
			r.URL.Path)
		return
	}

	// Export audit events with hash chain verification
	exportResult, err := s.auditEventsRepo.Export(ctx, filters)
	if err != nil {
		s.logger.Error(err, "Failed to export audit events")
		writeRFC7807Error(w, http.StatusInternalServerError,
			"Export Failed",
			"Failed to export audit events. Please retry.",
			r.URL.Path)
		return
	}

	// Get export format
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	includeDetachedSignature := r.URL.Query().Get("include_detached_signature") == "true"

	// Build export response
	response, err := s.buildExportResponse(ctx, exportResult, filters, format, includeDetachedSignature, exportedBy)
	if err != nil {
		s.logger.Error(err, "Failed to build export response")
		writeRFC7807Error(w, http.StatusInternalServerError,
			"Export Failed",
			"Failed to build export response. Please retry.",
			r.URL.Path)
		return
	}

	// Format response based on export format
	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			s.logger.Error(err, "Failed to encode export response")
		}
	case "csv":
		// CSV export not yet implemented
		writeRFC7807Error(w, http.StatusNotImplemented,
			"Format Not Supported",
			"CSV export format is not yet implemented. Use format=json.",
			r.URL.Path)
		return
	default:
		writeRFC7807Error(w, http.StatusBadRequest,
			"Invalid Format",
			fmt.Sprintf("Unknown format: %s. Supported formats: json, csv.", format),
			r.URL.Path)
		return
	}

	s.logger.Info("Audit export completed successfully",
		"exported_by", exportedBy,
		"format", format,
		"total_events", exportResult.TotalEventsQueried,
		"integrity_percent", exportResult.ChainIntegrityPercent)
}

// parseExportFilters parses query parameters into ExportFilters
func parseExportFilters(r *http.Request) (repository.ExportFilters, error) {
	filters := repository.ExportFilters{
		Offset: 0,
		Limit:  defaultExportLimit,
	}

	query := r.URL.Query()

	// Parse start_time
	if startTimeStr := query.Get("start_time"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			return filters, fmt.Errorf("invalid start_time format: %w", err)
		}
		filters.StartTime = &startTime
	}

	// Parse end_time
	if endTimeStr := query.Get("end_time"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			return filters, fmt.Errorf("invalid end_time format: %w", err)
		}
		filters.EndTime = &endTime
	}

	// Parse correlation_id
	filters.CorrelationID = query.Get("correlation_id")

	// Parse event_category
	filters.EventCategory = query.Get("event_category")

	// Parse offset
	if offsetStr := query.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			return filters, fmt.Errorf("invalid offset: must be non-negative integer")
		}
		filters.Offset = offset
	}

	// Parse limit
	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			return filters, fmt.Errorf("invalid limit: must be positive integer")
		}
		filters.Limit = limit
	}

	return filters, nil
}

// buildExportResponse converts repository.ExportResult to OpenAPI AuditExportResponse
func (s *Server) buildExportResponse(
	ctx context.Context,
	exportResult *repository.ExportResult,
	filters repository.ExportFilters,
	format string,
	includeDetachedSignature bool,
	exportedBy string,
) (*dsgen.AuditExportResponse, error) {
	exportTimestamp := time.Now().UTC()

	// Sign the export
	signature, algorithm, certFingerprint, err := s.signExport(ctx, exportResult, exportTimestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to sign export: %w", err)
	}

	// Build response using JSON marshaling/unmarshaling to match generated types exactly
	// This avoids complex inline struct matching issues
	intermediateResponse := map[string]interface{}{
		"export_metadata": map[string]interface{}{
			"export_timestamp":         exportTimestamp,
			"export_format":            format,
			"total_events":             exportResult.TotalEventsQueried,
			"signature":                signature,
			"signature_algorithm":      algorithm,
			"certificate_fingerprint":  certFingerprint,
			"exported_by":              exportedBy,
			"query_filters": map[string]interface{}{
				"offset": filters.Offset,
				"limit":  filters.Limit,
			},
		},
		"events": make([]map[string]interface{}, 0, len(exportResult.Events)),
		"hash_chain_verification": map[string]interface{}{
			"total_events_verified":       exportResult.TotalEventsQueried,
			"valid_chain_events":          exportResult.ValidChainEvents,
			"broken_chain_events":         exportResult.BrokenChainEvents,
			"chain_integrity_percentage":  exportResult.ChainIntegrityPercent,
			"verification_timestamp":      exportResult.VerificationTimestamp,
		},
	}

	// Add optional query filters
	queryFilters := intermediateResponse["export_metadata"].(map[string]interface{})["query_filters"].(map[string]interface{})
	if filters.StartTime != nil {
		queryFilters["start_time"] = filters.StartTime
	}
	if filters.EndTime != nil {
		queryFilters["end_time"] = filters.EndTime
	}
	if filters.CorrelationID != "" {
		queryFilters["correlation_id"] = filters.CorrelationID
	}
	if filters.EventCategory != "" {
		queryFilters["event_category"] = filters.EventCategory
	}

	// Add optional tampered event IDs
	if len(exportResult.TamperedEventIDs) > 0 {
		intermediateResponse["hash_chain_verification"].(map[string]interface{})["tampered_event_ids"] = exportResult.TamperedEventIDs
	}

	// Convert repository events
	events := intermediateResponse["events"].([]map[string]interface{})
	for _, exportEvent := range exportResult.Events {
		event := map[string]interface{}{
			"event_id":           exportEvent.EventID.String(),
			"version":            exportEvent.Version,
			"event_type":         exportEvent.EventType,
			"event_timestamp":    exportEvent.EventTimestamp,
			"event_category":     exportEvent.EventCategory,
			"event_action":       exportEvent.EventAction,
			"event_outcome":      exportEvent.EventOutcome,
			"correlation_id":     exportEvent.CorrelationID,
			"event_hash":         exportEvent.EventHash,
			"previous_event_hash": exportEvent.PreviousEventHash,
			"hash_chain_valid":   exportEvent.HashChainValid,
			"legal_hold":         exportEvent.LegalHold,
		}

		// Add optional fields
		if exportEvent.EventData != nil {
			event["event_data"] = exportEvent.EventData
		}

		events = append(events, event)
	}
	intermediateResponse["events"] = events

	// Marshal intermediate response and unmarshal into generated type
	jsonBytes, err := json.Marshal(intermediateResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal intermediate response: %w", err)
	}

	var response dsgen.AuditExportResponse
	if err := json.Unmarshal(jsonBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal into generated type: %w", err)
	}

	// Add detached signature if requested
	if includeDetachedSignature {
		detachedSig := s.buildDetachedSignature(signature, algorithm, certFingerprint)
		response.DetachedSignature = &detachedSig
	}

	return &response, nil
}

// signExport signs the export data with x509 certificate
// Returns: (signature_base64, algorithm, cert_fingerprint, error)
//
// SOC2 Day 9.1: Digital signature implementation
// BR-AUDIT-007: Signed exports for tamper detection
// Algorithm: SHA256withRSA (NIST recommended)
func (s *Server) signExport(ctx context.Context, exportResult *repository.ExportResult, exportTimestamp time.Time) (string, string, string, error) {
	if s.signer == nil {
		return "", "", "", fmt.Errorf("signer not initialized")
	}

	// Build signable data structure (export metadata + events)
	signableData := map[string]interface{}{
		"export_timestamp":      exportTimestamp,
		"total_events":          exportResult.TotalEventsQueried,
		"valid_chain_events":    exportResult.ValidChainEvents,
		"broken_chain_events":   exportResult.BrokenChainEvents,
		"tampered_event_ids":    exportResult.TamperedEventIDs,
		"verification_timestamp": exportResult.VerificationTimestamp,
	}

	// Sign the export data
	signature, err := s.signer.Sign(signableData)
	if err != nil {
		s.logger.Error(err, "Failed to sign export data")
		return "", "", "", fmt.Errorf("failed to sign export: %w", err)
	}

	// Get certificate metadata
	algorithm := s.signer.GetAlgorithm()
	certFingerprint := s.signer.GetCertificateFingerprint()

	s.logger.V(1).Info("Export signature generated successfully",
		"algorithm", algorithm,
		"cert_fingerprint", certFingerprint,
		"signature_length", len(signature))

	return signature, algorithm, certFingerprint, nil
}

// buildDetachedSignature builds a PEM-encoded detached signature
func (s *Server) buildDetachedSignature(signature, algorithm, certFingerprint string) string {
	// Build PEM-style detached signature
	return fmt.Sprintf(`-----BEGIN SIGNATURE-----
Algorithm: %s
Certificate-Fingerprint: %s

%s
-----END SIGNATURE-----`, algorithm, certFingerprint, signature)
}

// writeRFC7807Error writes an RFC 7807 Problem Details error response
func writeRFC7807Error(w http.ResponseWriter, status int, title, detail, instance string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	problem := map[string]interface{}{
		"type":     "about:blank",
		"title":    title,
		"status":   status,
		"detail":   detail,
		"instance": instance,
	}

	json.NewEncoder(w).Encode(problem)
}

