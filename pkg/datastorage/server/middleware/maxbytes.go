/*
Copyright 2026 Jordi Gil.

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

package middleware

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-logr/logr"
)

// MaxBytesReaderMiddleware returns a Chi middleware that caps the request body
// to maxBytes for methods that carry a body (POST, PUT, PATCH, DELETE).
// GET/HEAD/OPTIONS requests pass through without a limit.
//
// #1048 Phase 4 / SC-5: Prevents oversized payloads from exhausting memory.
// Two-layer enforcement:
//   - Fast path: if Content-Length is present and exceeds maxBytes, returns 413
//     immediately (no body read). Covers all standard API clients.
//   - Slow path: wraps r.Body with http.MaxBytesReader for chunked/unknown-size
//     bodies. The handler's json.Decode will fail with MaxBytesError.
func MaxBytesReaderMiddleware(maxBytes int64, logger logr.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				if r.ContentLength > maxBytes {
					WriteMaxBytesExceeded(w, logger)
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// WriteMaxBytesExceeded writes a 413 RFC 7807 response for body-too-large errors.
// Handlers should call this when json.Decode or io.ReadAll encounters a MaxBytesError.
func WriteMaxBytesExceeded(w http.ResponseWriter, logger logr.Logger) {
	problem := map[string]interface{}{
		"type":   "https://kubernaut.ai/problems/request-too-large",
		"title":  "Request Entity Too Large",
		"status": http.StatusRequestEntityTooLarge,
		"detail": "Request body exceeds the maximum allowed size.",
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		logger.Error(err, "Failed to encode 413 RFC 7807 response")
	}
}

// IsMaxBytesError returns true if err is or wraps an *http.MaxBytesError.
func IsMaxBytesError(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}
