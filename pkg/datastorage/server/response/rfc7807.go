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

package response

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
)

// ========================================
// RFC 7807 PROBLEM DETAILS
// ========================================
// Authority: BR-STORAGE-024 (RFC 7807 error responses)
// Purpose: Standardized, machine-readable error responses
// ========================================

// RFC7807Problem represents an RFC 7807 Problem Details response
// https://datatracker.ietf.org/doc/html/rfc7807
type RFC7807Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

// WriteRFC7807Error writes an RFC 7807 Problem Details error response
// BR-STORAGE-024: Standardized error responses
//
// Parameters:
//   - w: HTTP response writer
//   - status: HTTP status code (400, 404, 500, etc.)
//   - errorType: Machine-readable error type (e.g., "invalid-limit", "not-found")
//   - title: Human-readable error title
//   - detail: Detailed error message
//   - logger: Optional logger for encoding failures
func WriteRFC7807Error(w http.ResponseWriter, status int, errorType, title, detail string, logger logr.Logger) {
	problem := RFC7807Problem{
		// DD-004: Use kubernaut.ai/problems/* for RFC 7807 error type URIs
		// V1.0 Domain: kubernaut.ai (standardized across all services)
		Type:   fmt.Sprintf("https://kubernaut.ai/problems/%s", errorType),
		Title:  title,
		Status: status,
		Detail: detail,
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	// Encode RFC 7807 error response
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		// Note: We've already written the status code, so we can't change the response
		// Log the encoding failure for observability
		if !logger.GetSink().Enabled(0) {
			logger = logr.Discard()
		}
		logger.Error(err, "Failed to encode RFC 7807 error response",
			"error_type", errorType,
			"status", status,
		)
	}
}

// WriteRFC7807ErrorWithRequestID writes an RFC 7807 error with request ID in logs
// Convenience wrapper that includes request ID context
func WriteRFC7807ErrorWithRequestID(w http.ResponseWriter, status int, errorType, title, detail, requestID string, logger logr.Logger) {
	if !logger.GetSink().Enabled(0) {
		logger = logr.Discard()
	}

	logger.V(1).Info("Returning RFC 7807 error",
		"request_id", requestID,
		"status", status,
		"error_type", errorType,
	)

	WriteRFC7807Error(w, status, errorType, title, detail, logger)
}
