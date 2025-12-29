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
	"net/http"

	"github.com/go-logr/logr"
)

// ========================================
// JSON RESPONSE HELPERS
// ========================================
// Purpose: Consistent JSON encoding with error handling
// ========================================

// WriteJSON writes a JSON response with the given status code
// Handles encoding errors gracefully with logging
//
// Parameters:
//   - w: HTTP response writer
//   - status: HTTP status code (200, 201, etc.)
//   - data: Data to encode as JSON
//   - logger: Logger for encoding failures
//   - logContext: Additional context for logging (key-value pairs)
func WriteJSON(w http.ResponseWriter, status int, data interface{}, logger logr.Logger, logContext ...interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// Encode JSON response
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Note: We've already written the status code, so we can't change the response
		// Log the encoding failure for observability
		if !logger.GetSink().Enabled(0) {
			logger = logr.Discard()
		}

		contextArgs := append([]interface{}{"status", status}, logContext...)
		logger.Error(err, "Failed to encode JSON response", contextArgs...)
	}
}

// WriteJSONWithRequestID writes a JSON response with request ID header
// Convenience wrapper that sets X-Request-ID header
func WriteJSONWithRequestID(w http.ResponseWriter, status int, data interface{}, requestID string, logger logr.Logger, logContext ...interface{}) {
	// Set request ID header for tracing
	w.Header().Set("X-Request-ID", requestID)

	// Add request ID to log context
	contextArgs := append([]interface{}{"request_id", requestID}, logContext...)
	WriteJSON(w, status, data, logger, contextArgs...)
}
