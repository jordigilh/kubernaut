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

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// deadlineWriter is a test double that records SetWriteDeadline calls.
// It embeds httptest.ResponseRecorder for standard ResponseWriter behavior
// and exposes the deadline that http.ResponseController propagates through
// the Unwrap() chain.
type deadlineWriter struct {
	*httptest.ResponseRecorder
	deadline    time.Time
	deadlineSet bool
}

func (dw *deadlineWriter) SetWriteDeadline(t time.Time) error {
	dw.deadline = t
	dw.deadlineSet = true
	return nil
}

// UT-AF-SSE-001: SetWriteDeadline propagates through statusRecorder [SC-8]
//
// SC-8 (Transmission Integrity): SSE streams carrying incident investigation
// events must not be silently killed by a fixed WriteTimeout. The middleware
// chain must allow downstream handlers to clear the write deadline for
// long-lived streaming connections.
func TestStatusRecorderUnwrap_SetWriteDeadlinePropagates(t *testing.T) {
	inner := &deadlineWriter{ResponseRecorder: httptest.NewRecorder()}
	wrapped := &statusRecorder{ResponseWriter: inner, status: http.StatusOK}

	rc := http.NewResponseController(wrapped)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		t.Fatalf("SC-8: SetWriteDeadline must propagate through statusRecorder, got error: %v", err)
	}
	if !inner.deadlineSet {
		t.Fatal("SC-8: SetWriteDeadline did not reach the underlying writer through statusRecorder")
	}
	if !inner.deadline.IsZero() {
		t.Fatalf("SC-8: expected zero deadline (clear), got %v", inner.deadline)
	}
}

// UT-AF-SSE-002: ResponseController sets non-zero deadline through statusRecorder [AU-12]
//
// AU-12 (Audit Generation): Investigation events constitute the audit
// trail. The middleware wrapper must not prevent downstream handlers from
// controlling connection timeouts for audit event delivery.
func TestStatusRecorderUnwrap_AuditEventDeliveryPath(t *testing.T) {
	inner := &deadlineWriter{ResponseRecorder: httptest.NewRecorder()}
	wrapped := &statusRecorder{ResponseWriter: inner, status: http.StatusOK}

	rc := http.NewResponseController(wrapped)
	target := time.Now().Add(10 * time.Minute)
	if err := rc.SetWriteDeadline(target); err != nil {
		t.Fatalf("AU-12: SetWriteDeadline must propagate through statusRecorder for audit delivery, got error: %v", err)
	}
	if !inner.deadlineSet {
		t.Fatal("AU-12: SetWriteDeadline did not reach the underlying writer")
	}
	if !inner.deadline.Equal(target) {
		t.Fatalf("AU-12: expected deadline %v, got %v", target, inner.deadline)
	}
}
