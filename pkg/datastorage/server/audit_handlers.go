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
	"net/http"
	"time"

	"go.uber.org/zap"

	dsmetrics "github.com/jordigilh/kubernaut/pkg/datastorage/metrics"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/validation"
)

// ========================================
// AUDIT WRITE HANDLERS (TDD GREEN Phase)
// 📋 Tests Define Contract: test/integration/datastorage/http_api_test.go
// Authority: IMPLEMENTATION_PLAN_V4.8.md Day 7
// ========================================
//
// This file implements HTTP WRITE API handlers for audit records.
//
// TDD DRIVEN DESIGN:
// - Tests written FIRST (http_api_test.go - 4 scenarios)
// - Handlers implement MINIMAL functionality to pass tests
// - Contract defined by test expectations
//
// Business Requirements:
// - BR-STORAGE-001 to BR-STORAGE-020: Audit write API
// - DD-009: DLQ fallback on database errors
//
// ========================================

// handleCreateNotificationAudit handles POST /api/v1/audit/notifications
// BR-STORAGE-001 to BR-STORAGE-020: Audit write API
// DD-009: DLQ fallback on database errors
//
// Request Body: NotificationAudit JSON
// Success Response: 201 Created with created audit record
// DLQ Fallback Response: 202 Accepted (async processing)
// Error Responses: 400 Bad Request, 409 Conflict, 500 Internal Server Error (RFC 7807)
func (s *Server) handleCreateNotificationAudit(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug("handleCreateNotificationAudit called",
		zap.String("method", r.Method),
		zap.String("path", r.URL.Path),
		zap.String("remote_addr", r.RemoteAddr))

	// Create context with timeout for database operations
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// 1. Parse request body
	s.logger.Debug("Parsing request body...")
	var audit models.NotificationAudit
	if err := json.NewDecoder(r.Body).Decode(&audit); err != nil {
		s.logger.Warn("Invalid JSON in request body",
			zap.Error(err),
			zap.String("remote_addr", r.RemoteAddr))
		writeRFC7807Error(w, validation.NewValidationErrorProblem(
			"notification_audit",
			map[string]string{"body": "invalid JSON: " + err.Error()},
		))
		return
	}
	s.logger.Debug("Request body parsed successfully",
		zap.String("notification_id", audit.NotificationID),
		zap.String("remediation_id", audit.RemediationID))

	// 2. Validate input using validator
	s.logger.Debug("Validating audit record...")
	if err := s.validator.Validate(&audit); err != nil {
		s.logger.Warn("Validation failed",
			zap.Error(err),
			zap.String("notification_id", audit.NotificationID))

		// Validator returns ValidationError with field-specific errors
		// Extract field errors for RFC 7807 response
		var fieldErrors map[string]string
		if valErr, ok := err.(*validation.ValidationError); ok {
			fieldErrors = valErr.FieldErrors
			// GAP-10: Emit validation failure metrics for each field
			for field := range fieldErrors {
				s.metrics.ValidationFailures.WithLabelValues(field, dsmetrics.ValidationReasonRequired).Inc()
			}
		} else {
			// Fallback for unexpected error type
			fieldErrors = map[string]string{"error": err.Error()}
		}

		writeRFC7807Error(w, validation.NewValidationErrorProblem(
			"notification_audit",
			fieldErrors,
		))
		return
	}

	// 3. Attempt database write via repository
	s.logger.Debug("Writing audit record to database...")
	// GAP-10: Measure write duration
	writeStart := time.Now()
	created, err := s.repository.Create(ctx, &audit)
	writeDuration := time.Since(writeStart).Seconds()

	if err != nil {
		s.logger.Error("Database write failed",
			zap.Error(err),
			zap.String("notification_id", audit.NotificationID),
			zap.Float64("write_duration_seconds", writeDuration))
		// Check if it's a known RFC 7807 error type (validation, conflict, not found)
		if rfc7807Err, ok := err.(*validation.RFC7807Problem); ok {
			s.logger.Info("Database write returned RFC 7807 error",
				zap.String("error_type", rfc7807Err.Type),
				zap.Int("status", rfc7807Err.Status),
				zap.String("notification_id", audit.NotificationID))
			writeRFC7807Error(w, rfc7807Err)
			return
		}

		// DD-009: Unknown database error → DLQ fallback
		s.logger.Error("Database write failed, using DLQ fallback",
			zap.Error(err),
			zap.String("notification_id", audit.NotificationID),
			zap.String("remediation_id", audit.RemediationID))

		// Attempt to enqueue to DLQ
		// Create a FRESH context for DLQ write (not tied to original request timeout)
		// DD-009: DLQ fallback must succeed even if DB operation timed out
		dlqCtx, dlqCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer dlqCancel()

		s.logger.Info("Attempting DLQ fallback",
			zap.String("notification_id", audit.NotificationID),
			zap.String("db_error", err.Error()))

		if dlqErr := s.dlqClient.EnqueueNotificationAudit(dlqCtx, &audit, err); dlqErr != nil {
			s.logger.Error("DLQ fallback also failed - data loss risk",
				zap.Error(dlqErr),
				zap.String("notification_id", audit.NotificationID),
				zap.String("original_error", err.Error()))
			writeRFC7807Error(w, validation.NewServiceUnavailableProblem(
				"database and DLQ both unavailable - please retry"))
			return
		}

		s.logger.Info("DLQ fallback succeeded",
			zap.String("notification_id", audit.NotificationID))

		// GAP-10: Emit DLQ fallback metrics
		s.metrics.AuditTracesTotal.WithLabelValues(
			dsmetrics.ServiceNotification,
			dsmetrics.AuditStatusDLQFallback,
		).Inc()

		// DLQ success - return 202 Accepted (async processing)
		s.logger.Info("Audit record queued to DLQ for async processing",
			zap.String("notification_id", audit.NotificationID))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "accepted",
			"message": "audit record queued for processing",
		})
		return
	}

	// 4. Success - return 201 Created with created record
	s.logger.Info("Audit record created successfully",
		zap.Int64("id", created.ID),
		zap.String("notification_id", created.NotificationID),
		zap.String("remediation_id", created.RemediationID))

	// GAP-10: Emit success metrics
	// Audit traces total (success)
	s.metrics.AuditTracesTotal.WithLabelValues(
		dsmetrics.ServiceNotification,
		dsmetrics.AuditStatusSuccess,
	).Inc()

	// Audit lag (time between event and write)
	auditLag := time.Since(audit.SentAt).Seconds()
	s.metrics.AuditLagSeconds.WithLabelValues(dsmetrics.ServiceNotification).Observe(auditLag)

	// Write duration
	s.metrics.WriteDuration.WithLabelValues("notification_audit").Observe(writeDuration)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// writeRFC7807Error writes an RFC 7807 Problem Details error response
// See: https://www.rfc-editor.org/rfc/rfc7807.html
func writeRFC7807Error(w http.ResponseWriter, problem *validation.RFC7807Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)
	json.NewEncoder(w).Encode(problem)
}
