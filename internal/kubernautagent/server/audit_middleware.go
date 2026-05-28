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

package server

import (
	"net/http"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap returns the underlying ResponseWriter so that http.NewResponseController
// can access http.Flusher and http.Hijacker on the real writer. Required for SSE
// streams where the MCP SDK must flush response headers immediately.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// AuditAuthMiddleware wraps an HTTP handler and emits audit events for
// 401 (auth failure) and 403 (auth denied) responses. This satisfies
// FedRAMP AU-12 without modifying the shared auth middleware (H5).
func AuditAuthMiddleware(next http.Handler, store audit.AuditStore, logger logr.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		switch rec.status {
		case http.StatusUnauthorized:
			evt := audit.NewEvent(audit.EventTypeAuthFailure, "")
			evt.EventAction = audit.ActionAuthFailure
			evt.EventOutcome = audit.OutcomeFailure
			evt.Data["source_ip"] = extractIP(r, nil)
			evt.Data["path"] = r.URL.Path
			evt.Data["method"] = r.Method
			audit.StoreBestEffort(r.Context(), store, evt, logger)
		case http.StatusForbidden:
			evt := audit.NewEvent(audit.EventTypeAuthDenied, "")
			evt.EventAction = audit.ActionAuthDenied
			evt.EventOutcome = audit.OutcomeFailure
			evt.Data["source_ip"] = extractIP(r, nil)
			evt.Data["path"] = r.URL.Path
			evt.Data["method"] = r.Method
			audit.StoreBestEffort(r.Context(), store, evt, logger)
		}
	})
}
