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
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// BR-SECURITY-1900 (AU-3/CC7.2): the security_event log emitted on an
// authentication failure must distinguish an audience-bound TokenReview
// mismatch from other "invalid token" failures (e.g. unauthenticated,
// empty identity) so SOC2/FedRAMP audit review can tell a cross-service
// token-replay attempt apart from a routine expired/garbage token,
// without changing the ErrTokenInvalid contract callers already match on.
var _ = Describe("Middleware security_event reason detail [BR-SECURITY-1900]", func() {
	var (
		logBuf   *bytes.Buffer
		recorder *httptest.ResponseRecorder
	)

	newMiddleware := func(authErr error) *auth.Middleware {
		logBuf = &bytes.Buffer{}
		testLogger := funcr.New(func(prefix, args string) {
			fmt.Fprintf(logBuf, "%s %s\n", prefix, args)
		}, funcr.Options{Verbosity: 1})

		authenticator := &auth.MockAuthenticator{ErrorToReturn: authErr}
		authorizer := &auth.MockAuthorizer{}
		return auth.NewMiddleware(authenticator, authorizer, auth.MiddlewareConfig{
			Namespace: "kubernaut-system", Resource: "services", ResourceName: "test-service", Verb: "create",
		}, testLogger)
	}

	doRequest := func(mw *auth.Middleware) {
		recorder = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/test", nil)
		req.Header.Set("Authorization", "Bearer some-token")
		mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(recorder, req)
	}

	It("IT-KA-1900-006: logs reason=invalid_token_audience on a TokenReview audience mismatch", func() {
		mw := newMiddleware(fmt.Errorf("%w: token audience mismatch: expected one of [kubernaut-agent], server returned [some-other-service]", auth.ErrTokenInvalid))
		doRequest(mw)

		Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
		Expect(logBuf.String()).To(ContainSubstring(`"reason"="invalid_token_audience"`))
	})

	It("IT-KA-1900-007: still logs the generic reason=invalid_token for non-audience token failures (regression)", func() {
		mw := newMiddleware(fmt.Errorf("%w: token rejected by API server", auth.ErrTokenInvalid))
		doRequest(mw)

		Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
		Expect(logBuf.String()).To(ContainSubstring(`"reason"="invalid_token"`))
		Expect(logBuf.String()).NotTo(ContainSubstring(`"reason"="invalid_token_audience"`))
	})
})
