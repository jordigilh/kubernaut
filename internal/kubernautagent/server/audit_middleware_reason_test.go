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

package server_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kaserver "github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	sharedauth "github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// recordingAuditStore captures every event passed to StoreAudit for assertion.
type recordingAuditStore struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (s *recordingAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *recordingAuditStore) last() *audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.events) == 0 {
		return nil
	}
	return s.events[len(s.events)-1]
}

// BR-SECURITY-1900 (AU-3, extends BR-AUDIT-005): AuditAuthMiddleware today
// persists a generic "auth_failure"/"auth_denied" audit-table event for any
// 401/403, indistinguishable from an audience-bound TokenReview mismatch
// (cross-service token replay attempt). Wiring it through
// sharedauth.WithFailureReasonCapture closes that gap without changing the
// wrapper's HTTP-status-driven dispatch.
var _ = Describe("AuditAuthMiddleware — reason enrichment [BR-SECURITY-1900]", func() {

	It("UT-KA-1900-014: persists Data[\"reason\"]=invalid_token_audience for a 401 produced by the shared auth middleware's audience check", func() {
		store := &recordingAuditStore{}

		authenticator := &sharedauth.MockAuthenticator{
			ErrorToReturn: fmt.Errorf("%w: token audience mismatch: expected one of [kubernaut-agent], server returned [some-other-service]", sharedauth.ErrTokenInvalid),
		}
		authorizer := &sharedauth.MockAuthorizer{}
		mw := sharedauth.NewMiddleware(authenticator, authorizer, sharedauth.MiddlewareConfig{
			Namespace: "kubernaut-system", Resource: "services", ResourceName: "kubernaut-agent", Verb: "create",
		}, logr.Discard())

		handler := kaserver.AuditAuthMiddleware(mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})), store, logr.Discard())

		req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", nil)
		req.Header.Set("Authorization", "Bearer some-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
		evt := store.last()
		Expect(evt).NotTo(BeNil())
		Expect(evt.Data["reason"]).To(Equal("invalid_token_audience"))
	})

	It("UT-KA-1900-015: persists no Data[\"reason\"] key for a 401 with no auth middleware in the chain (defense-in-depth catch-all preserved)", func() {
		store := &recordingAuditStore{}

		handler := kaserver.AuditAuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}), store, logr.Discard())

		req := httptest.NewRequest(http.MethodGet, "/api/v1/mcp", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
		evt := store.last()
		Expect(evt).NotTo(BeNil())
		_, hasReason := evt.Data["reason"]
		Expect(hasReason).To(BeFalse())
	})
})
