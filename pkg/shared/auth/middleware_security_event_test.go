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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// Issue #2664 (extends BR-AUDIT-005 and BR-SECURITY-016): every security
// decision the shared auth middleware makes -- authentication failure,
// authorization denial, authorization grant, and the two fail-closed
// service-unavailable paths -- must be observable as one structured
// security_event carrying the ADR-034 classification fields
// (event_id, event_type, event_action, event_outcome, actor_type, actor_id).
//
// Controls proven here: FedRAMP AU-2 (auditable events), AU-3 (record
// content/actor attribution), AC-6 (least-privilege grant/deny decisions
// observable); SOC2 CC7.2 (monitoring of security decisions); OWASP ASVS
// V2 (authentication), V3 (session/identity attribution) and V7 (no
// credential material in logs). OWASP ASVS V7.1.1 is the explicit
// credential-non-disclosure scenario (UT-AUTH-2664-009).

// capturedEvent is one structured log record emitted by the middleware.
type capturedEvent struct {
	message string
	fields  map[string]string
}

// captureLog is a logr.LogSink that records structured log entries so tests
// can assert on the exact ADR-034 field set rather than substring-matching
// a formatted log line.
type captureLog struct {
	mu      sync.Mutex
	entries []capturedEvent
}

func (c *captureLog) Init(_ logr.RuntimeInfo) {}

func (c *captureLog) Enabled(_ int) bool { return true }

func (c *captureLog) Info(_ int, msg string, keysAndValues ...interface{}) {
	fields := make(map[string]string, len(keysAndValues)/2)
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		fields[fmt.Sprint(keysAndValues[i])] = fmt.Sprint(keysAndValues[i+1])
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, capturedEvent{message: msg, fields: fields})
}

func (c *captureLog) Error(_ error, _ string, _ ...interface{}) {}

func (c *captureLog) WithValues(_ ...interface{}) logr.LogSink { return c }

func (c *captureLog) WithName(_ string) logr.LogSink { return c }

func (c *captureLog) events() []capturedEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]capturedEvent, len(c.entries))
	copy(out, c.entries)
	return out
}

// securityEvents filters the captured log down to security_event records.
func (c *captureLog) securityEvents() []capturedEvent {
	var out []capturedEvent
	for _, e := range c.events() {
		if e.message == "security_event" {
			out = append(out, e)
		}
	}
	return out
}

func (c *captureLog) lastSecurityEvent() capturedEvent {
	events := c.securityEvents()
	ExpectWithOffset(1, events).NotTo(BeEmpty(), "expected at least one security_event log entry")
	return events[len(events)-1]
}

var _ = Describe("Auth middleware security_event ADR-034 fields [Issue #2664]", func() {
	const (
		testUser  = "system:serviceaccount:kubernaut-system:caller"
		testToken = "secret-bearer-token-value"
	)

	var (
		logs       *captureLog
		authorizer *auth.MockAuthorizer
		nextCalled bool
	)

	newMiddleware := func(authenticator auth.Authenticator) *auth.Middleware {
		logs = &captureLog{}
		authorizer = &auth.MockAuthorizer{AllowedUsers: map[string]bool{testUser: true}}
		return auth.NewMiddleware(authenticator, authorizer, auth.MiddlewareConfig{
			Namespace:    "kubernaut-system",
			Resource:     "services",
			ResourceName: "data-storage-service",
			Verb:         "create",
		}, logr.New(logs))
	}

	validAuthenticator := func() *auth.MockAuthenticator {
		return &auth.MockAuthenticator{
			ValidUsers: map[string]string{testToken: testUser},
		}
	}

	doRequest := func(mw *auth.Middleware, authHeader string) int {
		nextCalled = false
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/events", nil)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(recorder, req)
		return recorder.Code
	}

	// assertADR034Fields proves the ADR-034 mandatory classification fields
	// exist and are non-empty on an emitted event.
	assertADR034Fields := func(event capturedEvent) {
		GinkgoHelper()
		for _, key := range []string{"event_id", "event_type", "event_action", "event_outcome", "actor_type", "actor_id"} {
			Expect(event.fields).To(HaveKey(key), "ADR-034 field %q must be present (FedRAMP AU-3)", key)
		}
		Expect(event.fields["event_outcome"]).To(BeElementOf("success", "failure", "pending"),
			"event_outcome must match the ADR-034 enum")

		parsed, err := uuid.Parse(event.fields["event_id"])
		Expect(err).NotTo(HaveOccurred(), "event_id must be a UUID (ADR-034)")
		Expect(parsed.Version()).To(Equal(uuid.Version(4)), "event_id must be a UUIDv4")
	}

	Context("authentication failures", func() {
		It("UT-AUTH-2664-001: a missing Authorization header emits an authentication/failure event without an authenticated actor", func() {
			mw := newMiddleware(&auth.MockAuthenticator{})
			Expect(doRequest(mw, "")).To(Equal(http.StatusUnauthorized))
			Expect(nextCalled).To(BeFalse())

			event := logs.lastSecurityEvent()
			Expect(event.fields["reason"]).To(Equal("missing_auth_header"))
			Expect(event.fields["event_type"]).To(Equal("authentication"))
			Expect(event.fields["event_action"]).To(Equal("authenticate"))
			Expect(event.fields["event_outcome"]).To(Equal("failure"))
			Expect(event.fields["actor_type"]).To(Equal("external"))
			Expect(event.fields["actor_id"]).To(BeEmpty())
			Expect(event.fields["status_code"]).To(Equal("401"))
			assertADR034Fields(event)
		})

		It("UT-AUTH-2664-002: an invalid token preserves its reason and attributes no user identity", func() {
			mw := newMiddleware(&auth.MockAuthenticator{
				ErrorToReturn: fmt.Errorf("%w: token rejected by API server", auth.ErrTokenInvalid),
			})
			Expect(doRequest(mw, "Bearer "+testToken)).To(Equal(http.StatusUnauthorized))

			event := logs.lastSecurityEvent()
			Expect(event.fields["reason"]).To(Equal("invalid_token"))
			Expect(event.fields["event_type"]).To(Equal("authentication"))
			Expect(event.fields["event_action"]).To(Equal("authenticate"))
			Expect(event.fields["actor_type"]).To(Equal("external"))
			assertADR034Fields(event)
		})
	})

	Context("authorization denials", func() {
		It("UT-AUTH-2664-003: a SAR denial emits authorization/deny with the authenticated user as actor", func() {
			mw := newMiddleware(validAuthenticator())
			authorizer.AllowedUsers[testUser] = false

			Expect(doRequest(mw, "Bearer "+testToken)).To(Equal(http.StatusForbidden))
			Expect(nextCalled).To(BeFalse())

			event := logs.lastSecurityEvent()
			Expect(event.fields["reason"]).To(Equal("authorization_denied"))
			Expect(event.fields["event_type"]).To(Equal("authorization"))
			Expect(event.fields["event_action"]).To(Equal("deny"))
			Expect(event.fields["event_outcome"]).To(Equal("failure"))
			Expect(event.fields["actor_type"]).To(Equal("user"))
			Expect(event.fields["actor_id"]).To(Equal(testUser))
			Expect(event.fields["user"]).To(Equal(testUser))
			assertADR034Fields(event)
		})

		It("UT-AUTH-2664-008: a denial never emits a grant event", func() {
			mw := newMiddleware(validAuthenticator())
			authorizer.AllowedUsers[testUser] = false

			Expect(doRequest(mw, "Bearer "+testToken)).To(Equal(http.StatusForbidden))

			events := logs.securityEvents()
			Expect(events).To(HaveLen(1), "a denial must produce exactly one security decision event")
			Expect(events[0].fields["event_action"]).To(Equal("deny"))
		})
	})

	Context("fail-closed service-unavailable paths", func() {
		It("UT-AUTH-2664-004: an authenticator outage emits authentication_service_unavailable, not action=deny", func() {
			mw := newMiddleware(&auth.MockAuthenticator{
				ErrorToReturn: errors.New("tokenreviews.authentication.k8s.io is unreachable"),
			})
			Expect(doRequest(mw, "Bearer "+testToken)).To(Equal(http.StatusInternalServerError))
			Expect(nextCalled).To(BeFalse())

			event := logs.lastSecurityEvent()
			Expect(event.fields["reason"]).To(Equal("authentication_service_unavailable"))
			Expect(event.fields["event_type"]).To(Equal("authentication"))
			Expect(event.fields["event_action"]).To(Equal("authenticate"),
				"a 500 outage is not a denial decision, so it must not carry action=deny")
			Expect(event.fields["event_action"]).NotTo(Equal("deny"))
			Expect(event.fields["event_outcome"]).To(Equal("failure"))
			Expect(event.fields["actor_type"]).To(Equal("external"))
			Expect(event.fields["status_code"]).To(Equal("500"))
			assertADR034Fields(event)
		})

		It("UT-AUTH-2664-005: an authorizer outage emits authorization_service_unavailable and keeps the known actor", func() {
			mw := newMiddleware(validAuthenticator())
			authorizer.ErrorToReturn = errors.New("subjectaccessreviews.authorization.k8s.io is unreachable")

			Expect(doRequest(mw, "Bearer "+testToken)).To(Equal(http.StatusInternalServerError))
			Expect(nextCalled).To(BeFalse())

			event := logs.lastSecurityEvent()
			Expect(event.fields["reason"]).To(Equal("authorization_service_unavailable"))
			Expect(event.fields["event_type"]).To(Equal("authorization"))
			Expect(event.fields["event_action"]).To(Equal("authorize"))
			Expect(event.fields["event_action"]).NotTo(Equal("deny"))
			Expect(event.fields["event_outcome"]).To(Equal("failure"))
			Expect(event.fields["actor_type"]).To(Equal("user"))
			Expect(event.fields["actor_id"]).To(Equal(testUser))
			assertADR034Fields(event)
		})
	})

	Context("authorization grants", func() {
		It("UT-AUTH-2664-006: a successful authorization emits exactly one authorization/grant/success event", func() {
			mw := newMiddleware(validAuthenticator())
			Expect(doRequest(mw, "Bearer "+testToken)).To(Equal(http.StatusOK))
			Expect(nextCalled).To(BeTrue())

			events := logs.securityEvents()
			Expect(events).To(HaveLen(1))

			event := events[0]
			Expect(event.fields["reason"]).To(Equal("authorization_granted"))
			Expect(event.fields["event_type"]).To(Equal("authorization"))
			Expect(event.fields["event_action"]).To(Equal("grant"))
			Expect(event.fields["event_outcome"]).To(Equal("success"))
			Expect(event.fields["actor_type"]).To(Equal("user"))
			Expect(event.fields["actor_id"]).To(Equal(testUser))
			Expect(event.fields["resource"]).To(Equal("services"))
			Expect(event.fields["resource_name"]).To(Equal("data-storage-service"))
			Expect(event.fields["verb"]).To(Equal("create"))
			assertADR034Fields(event)
		})
	})

	Context("audit record content", func() {
		It("UT-AUTH-2664-007: independent decisions get distinct event_ids", func() {
			mw := newMiddleware(validAuthenticator())

			Expect(doRequest(mw, "")).To(Equal(http.StatusUnauthorized))
			Expect(doRequest(mw, "Bearer "+testToken)).To(Equal(http.StatusOK))

			events := logs.securityEvents()
			Expect(events).To(HaveLen(2))
			Expect(events[0].fields["event_id"]).NotTo(Equal(events[1].fields["event_id"]),
				"each security decision is a distinct audit record (AU-3)")
		})

		It("UT-AUTH-2664-009: no emitted security event discloses the bearer credential", func() {
			mw := newMiddleware(validAuthenticator())
			authorizer.AllowedUsers[testUser] = false

			Expect(doRequest(mw, "Bearer "+testToken)).To(Equal(http.StatusForbidden))

			var rendered strings.Builder
			for _, event := range logs.events() {
				fmt.Fprintf(&rendered, "%s %v\n", event.message, event.fields)
			}
			Expect(rendered.String()).NotTo(ContainSubstring(testToken),
				"OWASP ASVS V7: credentials must never appear in security logs")
		})

		It("UT-AUTH-2664-010: service-unavailable reasons reach the outer failure-reason capture", func() {
			mw := newMiddleware(&auth.MockAuthenticator{ErrorToReturn: errors.New("tokenreview outage")})

			ctx, getReason := auth.WithFailureReasonCapture(context.Background())
			req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/events", nil)
			req.Header.Set("Authorization", "Bearer "+testToken)
			mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})).ServeHTTP(httptest.NewRecorder(), req.WithContext(ctx))

			Expect(getReason()).To(Equal("authentication_service_unavailable"),
				"outer audit wrappers must be able to distinguish an outage from a credential failure (AU-3)")
		})

		It("UT-AUTH-2664-011: the complete reason vocabulary maps to one consistent classification", func() {
			cases := []struct {
				name          string
				header        string
				authenticator *auth.MockAuthenticator
				deny          bool
				authorizerErr error
				wantReason    string
				wantType      string
				wantAction    string
			}{
				{"missing header", "", &auth.MockAuthenticator{}, false, nil,
					"missing_auth_header", "authentication", "authenticate"},
				{"invalid scheme", "Basic dXNlcjpwYXNz", &auth.MockAuthenticator{}, false, nil,
					"invalid_auth_format", "authentication", "authenticate"},
				{"empty bearer", "Bearer ", &auth.MockAuthenticator{}, false, nil,
					"empty_bearer_token", "authentication", "authenticate"},
				{"invalid token", "Bearer " + testToken,
					&auth.MockAuthenticator{ErrorToReturn: fmt.Errorf("%w: rejected", auth.ErrTokenInvalid)},
					false, nil, "invalid_token", "authentication", "authenticate"},
				{"audience mismatch", "Bearer " + testToken,
					&auth.MockAuthenticator{ErrorToReturn: fmt.Errorf("%w: token audience mismatch", auth.ErrTokenInvalid)},
					false, nil, "invalid_token_audience", "authentication", "authenticate"},
				{"authorization denial", "Bearer " + testToken, validAuthenticator(), true, nil,
					"authorization_denied", "authorization", "deny"},
				{"authenticator outage", "Bearer " + testToken,
					&auth.MockAuthenticator{ErrorToReturn: errors.New("tokenreview unavailable")},
					false, nil, "authentication_service_unavailable", "authentication", "authenticate"},
				{"authorizer outage", "Bearer " + testToken, validAuthenticator(), false,
					errors.New("subjectaccessreview unavailable"),
					"authorization_service_unavailable", "authorization", "authorize"},
			}

			for _, tc := range cases {
				By(tc.name)
				mw := newMiddleware(tc.authenticator)
				authorizer.AllowedUsers[testUser] = !tc.deny
				authorizer.ErrorToReturn = tc.authorizerErr

				doRequest(mw, tc.header)

				event := logs.lastSecurityEvent()
				Expect(event.fields["reason"]).To(Equal(tc.wantReason))
				Expect(event.fields["event_type"]).To(Equal(tc.wantType))
				Expect(event.fields["event_action"]).To(Equal(tc.wantAction),
					"reason %q must classify consistently", tc.wantReason)
			}
		})
	})
})
