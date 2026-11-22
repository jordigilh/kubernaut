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

package gateway_test

// Issue #2664: wiring proof for the enriched shared auth middleware security
// events. These specs drive the expanded security_event contract through the
// production Gateway dispatch path -- Server.RegisterAdapter applies
// s.authMiddleware.Handler as the outermost layer of every signal route --
// rather than calling pkg/shared/auth directly. The Kubernetes TokenReview/
// SAR dependency is the external service replaced by MockAuthenticator/
// MockAuthorizer (AGENTS.md mock strategy: external APIs only).
//
// Controls: FedRAMP AU-2/AU-3/AC-6, SOC2 CC7.2, OWASP ASVS V2/V3/V7.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	gatewaypkg "github.com/jordigilh/kubernaut/pkg/gateway"
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/types"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

type wiringLogEntry struct {
	message string
	fields  map[string]any
}

type wiringLogSink struct {
	mu      sync.Mutex
	entries []wiringLogEntry
}

func (s *wiringLogSink) Init(_ logr.RuntimeInfo) {}

func (s *wiringLogSink) Enabled(_ int) bool { return true }

func (s *wiringLogSink) Info(_ int, msg string, keysAndValues ...interface{}) {
	fields := make(map[string]any, len(keysAndValues)/2)
	for i := 0; i+1 < len(keysAndValues); i += 2 {
		fields[fmt.Sprint(keysAndValues[i])] = keysAndValues[i+1]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, wiringLogEntry{message: msg, fields: fields})
}

func (s *wiringLogSink) Error(_ error, _ string, _ ...interface{}) {}

func (s *wiringLogSink) WithValues(_ ...interface{}) logr.LogSink { return s }

func (s *wiringLogSink) WithName(_ string) logr.LogSink { return s }

func (s *wiringLogSink) securityEvents() []wiringLogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []wiringLogEntry
	for _, e := range s.entries {
		if e.message == "security_event" {
			out = append(out, e)
		}
	}
	return out
}

// probeAdapter is a minimal RoutableAdapter: the specs only need the route to
// be registered so the production auth middleware wraps it. Parse always
// rejects the body, keeping signal processing out of the assertion path.
type probeAdapter struct{}

func (probeAdapter) Name() string { return "security-event-probe" }

func (probeAdapter) Parse(_ context.Context, _ []byte) (*types.NormalizedSignal, error) {
	return nil, fmt.Errorf("probe adapter rejects payloads by design")
}

func (probeAdapter) Validate(_ *types.NormalizedSignal) error { return nil }

func (probeAdapter) GetMetadata() adapters.AdapterMetadata {
	return adapters.AdapterMetadata{Name: "security-event-probe", Version: "1.0"}
}

func (probeAdapter) GetSourceService() string { return "probe" }

func (probeAdapter) GetSourceType() string { return "alert" }

func (probeAdapter) GetRoute() string { return "/api/v1/signals/security-event-probe" }

func (probeAdapter) ReplayValidator(_ time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}

var _ = Describe("Gateway auth security_event wiring [Issue #2664]", Ordered, func() {
	const (
		testUser  = "system:serviceaccount:kubernaut-system:probe"
		testToken = "wiring-secret-token"
	)

	var (
		logs          *wiringLogSink
		authenticator *auth.MockAuthenticator
		authorizer    *auth.MockAuthorizer
		server        *gatewaypkg.Server
	)

	BeforeAll(func() {
		Expect(os.Setenv("KUBERNAUT_CONTROLLER_NAMESPACE", "kubernaut-system")).To(Succeed())

		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

		cfg := &config.ServerConfig{
			Server: config.ServerSettings{
				ListenAddr:   "127.0.0.1:0",
				ReadTimeout:  30 * time.Second,
				WriteTimeout: 30 * time.Second,
				IdleTimeout:  120 * time.Second,
			},
			Processing: config.ProcessingSettings{
				Deduplication: config.DeduplicationSettings{CooldownPeriod: 300 * time.Second},
				Retry: config.RetrySettings{
					MaxAttempts:    3,
					InitialBackoff: 100 * time.Millisecond,
					MaxBackoff:     1 * time.Second,
				},
			},
		}
		cfg.Server.TLS.CertDir = gatewayTestCertDir()

		logs = &wiringLogSink{}
		authorizer = &auth.MockAuthorizer{AllowedUsers: map[string]bool{testUser: true}}
		authenticator = &auth.MockAuthenticator{ValidUsers: map[string]string{testToken: testUser}}

		var err error
		server, err = gatewaypkg.NewServerForTesting(gatewaypkg.ServerTestDeps{
			Config:          cfg,
			Logger:          logr.New(logs),
			MetricsInstance: metrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			CtrlClient:      k8sClient,
			Authenticator:   authenticator,
			Authorizer:      authorizer,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(server.RegisterAdapter(probeAdapter{})).To(Succeed())
	})

	post := func(authHeader string) int {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, probeAdapter{}.GetRoute(), strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		server.Handler().ServeHTTP(recorder, req)
		return recorder.Code
	}

	postWithRequestID := func(authHeader string) (int, string) {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, probeAdapter{}.GetRoute(), strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		server.Handler().ServeHTTP(recorder, req)
		return recorder.Code, recorder.Header().Get("X-Request-ID")
	}

	It("IT-AUTH-2664-050: an unauthenticated request through the production route emits an enriched authentication failure event", func() {
		Expect(post("")).To(Equal(http.StatusUnauthorized))

		events := logs.securityEvents()
		Expect(events).To(HaveLen(1))
		event := events[0]
		Expect(event.fields["reason"]).To(Equal("missing_auth_header"))
		Expect(event.fields["event_type"]).To(Equal("authentication"))
		Expect(event.fields["event_action"]).To(Equal("authenticate"))
		Expect(event.fields["event_outcome"]).To(Equal("failure"))
		Expect(event.fields["actor_type"]).To(Equal("external"))
		Expect(event.fields["path"]).To(Equal(probeAdapter{}.GetRoute()))
		Expect(event.fields["resource_name"]).To(Equal("gateway-service"))

		parsed, err := uuid.Parse(event.fields["event_id"].(string))
		Expect(err).NotTo(HaveOccurred())
		Expect(parsed.Version()).To(Equal(uuid.Version(4)))
	})

	It("IT-AUTH-2664-051: a denied request through the production route attributes the authenticated actor", func() {
		authorizer.AllowedUsers[testUser] = false
		DeferCleanup(func() { authorizer.AllowedUsers[testUser] = true })

		Expect(post("Bearer " + testToken)).To(Equal(http.StatusForbidden))

		var last wiringLogEntry
		events := logs.securityEvents()
		Expect(events).NotTo(BeEmpty())
		last = events[len(events)-1]
		Expect(last.fields["reason"]).To(Equal("authorization_denied"))
		Expect(last.fields["event_type"]).To(Equal("authorization"))
		Expect(last.fields["event_action"]).To(Equal("deny"))
		Expect(last.fields["actor_type"]).To(Equal("user"))
		Expect(last.fields["actor_id"]).To(Equal(testUser))
	})

	It("IT-AUTH-2664-052: an authorized request through the production route emits the grant event before dispatch", func() {
		// The probe adapter rejects the payload afterwards (400); the grant
		// event proves the authorization decision itself is observable.
		Expect(post("Bearer " + testToken)).To(Equal(http.StatusBadRequest))

		var last wiringLogEntry
		events := logs.securityEvents()
		Expect(events).NotTo(BeEmpty())
		last = events[len(events)-1]
		Expect(last.fields["reason"]).To(Equal("authorization_granted"))
		Expect(last.fields["event_action"]).To(Equal("grant"))
		Expect(last.fields["event_outcome"]).To(Equal("success"))
		Expect(last.fields["actor_id"]).To(Equal(testUser))
		Expect(last.fields["status_code"]).To(Equal(200),
			"status_code records the authorization decision, not the probe's 400")
	})

	It("IT-AUTH-2664-053: a TokenReview outage through the production route emits authentication_service_unavailable", func() {
		authenticator.ErrorToReturn = errors.New("tokenreviews.authentication.k8s.io unreachable")
		DeferCleanup(func() { authenticator.ErrorToReturn = nil })

		Expect(post("Bearer " + testToken)).To(Equal(http.StatusInternalServerError))

		var last wiringLogEntry
		events := logs.securityEvents()
		Expect(events).NotTo(BeEmpty())
		last = events[len(events)-1]
		Expect(last.fields["reason"]).To(Equal("authentication_service_unavailable"))
		Expect(last.fields["event_type"]).To(Equal("authentication"))
		Expect(last.fields["event_action"]).To(Equal("authenticate"))
		Expect(last.fields["event_outcome"]).To(Equal("failure"))
		Expect(last.fields["actor_type"]).To(Equal("external"))
	})

	It("IT-AUTH-2664-054: a SAR outage through the production route emits authorization_service_unavailable", func() {
		authorizer.ErrorToReturn = errors.New("subjectaccessreviews.authorization.k8s.io unreachable")
		DeferCleanup(func() { authorizer.ErrorToReturn = nil })

		Expect(post("Bearer " + testToken)).To(Equal(http.StatusInternalServerError))

		var last wiringLogEntry
		events := logs.securityEvents()
		Expect(events).NotTo(BeEmpty())
		last = events[len(events)-1]
		Expect(last.fields["reason"]).To(Equal("authorization_service_unavailable"))
		Expect(last.fields["event_type"]).To(Equal("authorization"))
		Expect(last.fields["event_action"]).To(Equal("authorize"))
		Expect(last.fields["event_outcome"]).To(Equal("failure"))
		Expect(last.fields["actor_type"]).To(Equal("user"))
		Expect(last.fields["actor_id"]).To(Equal(testUser))
	})

	It("IT-AUTH-2664-055: the security-event correlation_id matches the request ID returned to the client", func() {
		status, requestID := postWithRequestID("")
		Expect(status).To(Equal(http.StatusUnauthorized))
		Expect(requestID).NotTo(BeEmpty(), "Gateway must return X-Request-ID")

		last := logs.securityEvents()
		Expect(last).NotTo(BeEmpty())
		Expect(last[len(last)-1].fields["correlation_id"]).To(Equal(requestID),
			"clients and access logs must be able to find security events by the request ID they were given")
	})
})
