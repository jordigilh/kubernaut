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

package auth_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// BR-SECURITY-1900 (AU-3/CC7.2, extending BR-AUDIT-005): an outer
// audit-persistence wrapper (e.g. KubernautAgent's AuditAuthMiddleware) only
// observes the HTTP status code Handler produced, not *why* authentication
// or authorization failed -- today's persisted audit-table event for a 401
// is indistinguishable from an audience-bound TokenReview mismatch. This
// capture mechanism lets such a wrapper recover the exact same reason
// classification logSecurityEvent already computes, without Handler's
// signature changing or every caller adopting an audit dependency directly.
var _ = Describe("WithFailureReasonCapture [BR-SECURITY-1900]", func() {
	newMiddleware := func(authErr error, denied bool) *auth.Middleware {
		authenticator := &auth.MockAuthenticator{ErrorToReturn: authErr}
		authorizer := &auth.MockAuthorizer{}
		if authErr == nil {
			// authorizeRequest is only reached with a successfully authenticated
			// token, so a valid-token scenario needs ValidUsers populated.
			authenticator.ValidUsers = map[string]string{"some-token": "test-user"}
		}
		if denied {
			authorizer.AllowedUsers = map[string]bool{"test-user": false}
		} else {
			authorizer.AllowedUsers = map[string]bool{"test-user": true}
		}
		return auth.NewMiddleware(authenticator, authorizer, auth.MiddlewareConfig{
			Namespace: "kubernaut-system", Resource: "services", ResourceName: "test-service", Verb: "create",
		}, logr.Discard())
	}

	doRequestWithCapture := func(mw *auth.Middleware, authHeader string) (int, string) {
		ctx, getReason := auth.WithFailureReasonCapture(context.Background())
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/test", nil).WithContext(ctx)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(recorder, req)
		return recorder.Code, getReason()
	}

	It("IT-KA-1900-008: captures reason=invalid_token_audience for a TokenReview audience mismatch", func() {
		mw := newMiddleware(fmt.Errorf("%w: token audience mismatch: expected one of [kubernaut-agent], server returned [some-other-service]", auth.ErrTokenInvalid), false)
		status, reason := doRequestWithCapture(mw, "Bearer some-token")

		Expect(status).To(Equal(http.StatusUnauthorized))
		Expect(reason).To(Equal("invalid_token_audience"))
	})

	It("IT-KA-1900-009: captures reason=missing_auth_header when no Authorization header is present", func() {
		mw := newMiddleware(nil, false)
		status, reason := doRequestWithCapture(mw, "")

		Expect(status).To(Equal(http.StatusUnauthorized))
		Expect(reason).To(Equal("missing_auth_header"))
	})

	It("IT-KA-1900-010: captures reason=authorization_denied for a valid token with insufficient RBAC", func() {
		mw := newMiddleware(nil, true)
		status, reason := doRequestWithCapture(mw, "Bearer some-token")

		Expect(status).To(Equal(http.StatusForbidden))
		Expect(reason).To(Equal("authorization_denied"))
	})

	It("IT-KA-1900-011: returns an empty reason when authentication and authorization both succeed", func() {
		authenticator := &auth.MockAuthenticator{ValidUsers: map[string]string{"some-token": "test-user"}}
		authorizer := &auth.MockAuthorizer{AllowedUsers: map[string]bool{"test-user": true}}
		mw := auth.NewMiddleware(authenticator, authorizer, auth.MiddlewareConfig{
			Namespace: "kubernaut-system", Resource: "services", ResourceName: "test-service", Verb: "create",
		}, logr.Discard())
		status, reason := doRequestWithCapture(mw, "Bearer some-token")

		Expect(status).To(Equal(http.StatusOK))
		Expect(reason).To(BeEmpty())
	})
})
