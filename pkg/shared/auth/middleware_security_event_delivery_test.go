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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// Issue #2694 (BR-SECURITY-016, FedRAMP AU-2): the security_event records
// emitted for every authentication/authorization decision must reach their
// own dedicated logging path -- not sampled by the host service's zap
// sampler and not gated by the host service's configured log level -- while
// the authorization decisions themselves and all unrelated logging are
// unchanged.
//
// Controls proven here: FedRAMP AU-2 (auditable events) and AU-3 (record
// content); SOC2 CC7.2 (no silently dropped security decisions).

// syncBuffer is a concurrency-safe zapcore.WriteSyncer so a real zap core
// (JSON encoder) can be inspected in-memory.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) Sync() error { return nil }

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// newZapLoggerTo builds a zapr logger around a real zap JSON core writing to
// b at the given level, optionally wrapped in zap's production sampler
// (100 entries then every 100th per message+level) -- the exact host-service
// configuration that drops bursty security_event records today.
func newZapLoggerTo(b *syncBuffer, level zapcore.Level, sampled bool) logr.Logger {
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, b, level)
	if sampled {
		core = zapcore.NewSampler(core, time.Second, 100, 100)
	}
	return zapr.NewLogger(zap.New(core))
}

// zapSecurityEvents decodes the JSON lines captured in b and returns the
// field maps of the security_event records.
func zapSecurityEvents(b *syncBuffer) []map[string]any {
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		Expect(json.Unmarshal([]byte(line), &entry)).To(Succeed())
		if entry["msg"] == "security_event" {
			out = append(out, entry)
		}
	}
	return out
}

var _ = Describe("Auth middleware dedicated security-event logger [Issue #2694]", func() {
	const (
		testUser  = "system:serviceaccount:kubernaut-system:caller"
		testToken = "secret-bearer-token-value"
	)

	newConfig := func(securityLogger logr.Logger) auth.MiddlewareConfig {
		return auth.MiddlewareConfig{
			Namespace:           "kubernaut-system",
			Resource:            "services",
			ResourceName:        "data-storage-service",
			Verb:                "create",
			SecurityEventLogger: securityLogger,
		}
	}

	doRequest := func(mw *auth.Middleware, authHeader string) int {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/audit/events", nil)
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(recorder, req)
		return recorder.Code
	}

	newValidPair := func() (*auth.MockAuthenticator, *auth.MockAuthorizer) {
		return &auth.MockAuthenticator{ValidUsers: map[string]string{testToken: testUser}},
			&auth.MockAuthorizer{AllowedUsers: map[string]bool{testUser: true}}
	}

	It("UT-AUTH-2694-001: security events go to the dedicated logger while unrelated logging stays on the host logger", func() {
		hostLogs, dedicatedLogs := &captureLog{}, &captureLog{}
		authenticator, authorizer := newValidPair()
		mw := auth.NewMiddleware(authenticator, authorizer, newConfig(logr.New(dedicatedLogs)), logr.New(hostLogs))

		Expect(doRequest(mw, "")).To(Equal(http.StatusUnauthorized))

		Expect(dedicatedLogs.securityEvents()).To(HaveLen(1))
		Expect(dedicatedLogs.securityEvents()[0].record().Reason).To(Equal("missing_auth_header"))
		Expect(hostLogs.securityEvents()).To(BeEmpty(),
			"security_event records must leave the host logger's sampled/level-gated path entirely")
	})

	It("UT-AUTH-2694-002: a zero-value SecurityEventLogger falls back to the host logger (backward compatibility)", func() {
		hostLogs := &captureLog{}
		authenticator, authorizer := newValidPair()
		mw := auth.NewMiddleware(authenticator, authorizer, auth.MiddlewareConfig{
			Namespace:    "kubernaut-system",
			Resource:     "services",
			ResourceName: "data-storage-service",
			Verb:         "create",
		}, logr.New(hostLogs))

		Expect(doRequest(mw, "")).To(Equal(http.StatusUnauthorized))
		Expect(hostLogs.securityEvents()).To(HaveLen(1),
			"direct NewMiddleware callers without a dedicated logger keep today's behavior")
	})

	It("UT-AUTH-2694-003: a burst of authorized requests delivers one grant record per SAR decision under a sampling host logger", func() {
		const total = 250
		hostBuf, dedicatedBuf := &syncBuffer{}, &syncBuffer{}
		hostLogger := newZapLoggerTo(hostBuf, zapcore.InfoLevel, true)
		dedicatedLogger := newZapLoggerTo(dedicatedBuf, zapcore.InfoLevel, false)
		authenticator, authorizer := newValidPair()
		mw := auth.NewMiddleware(authenticator, authorizer, newConfig(dedicatedLogger), hostLogger)

		ids := make(map[string]bool, total)
		for i := 0; i < total; i++ {
			ExpectWithOffset(1, doRequest(mw, "Bearer "+testToken)).To(Equal(http.StatusOK),
				"the dedicated logging path must not change authorization decisions")
		}

		events := zapSecurityEvents(dedicatedBuf)
		Expect(events).To(HaveLen(total),
			"AU-2: one grant record per SAR decision -- none may be sampled away")
		for _, e := range events {
			record, err := auth.ParseSecurityEventRecord(e)
			Expect(err).NotTo(HaveOccurred())
			Expect(record.Reason).To(Equal("authorization_granted"))
			Expect(ids[record.EventID]).To(BeFalse(), "each grant record must carry a unique event_id")
			ids[record.EventID] = true
		}
		Expect(ids).To(HaveLen(total))
	})

	It("UT-AUTH-2694-004: a WARN-gated host logger still delivers every security event kind via the dedicated logger", func() {
		hostBuf, dedicatedBuf := &syncBuffer{}, &syncBuffer{}
		hostLogger := newZapLoggerTo(hostBuf, zapcore.WarnLevel, false)
		dedicatedLogger := newZapLoggerTo(dedicatedBuf, zapcore.InfoLevel, false)
		authenticator, authorizer := newValidPair()
		mw := auth.NewMiddleware(authenticator, authorizer, newConfig(dedicatedLogger), hostLogger)

		// Grant.
		Expect(doRequest(mw, "Bearer "+testToken)).To(Equal(http.StatusOK))
		// Denial.
		authorizer.AllowedUsers[testUser] = false
		Expect(doRequest(mw, "Bearer "+testToken)).To(Equal(http.StatusForbidden))
		authorizer.AllowedUsers[testUser] = true
		// Authentication failure.
		Expect(doRequest(mw, "")).To(Equal(http.StatusUnauthorized))

		events := zapSecurityEvents(dedicatedBuf)
		Expect(events).To(HaveLen(3),
			"a service configured at WARN must still emit grant, deny and authentication-failure records")
		reasons := []string{}
		for _, e := range events {
			record, err := auth.ParseSecurityEventRecord(e)
			Expect(err).NotTo(HaveOccurred())
			reasons = append(reasons, record.Reason)
		}
		Expect(reasons).To(Equal([]string{"authorization_granted", "authorization_denied", "missing_auth_header"}))

		// Control: the same events through the WARN-gated host logger with no
		// dedicated logger configured are suppressed entirely.
		controlBuf := &syncBuffer{}
		controlHost := newZapLoggerTo(controlBuf, zapcore.WarnLevel, false)
		authenticator2, authorizer2 := newValidPair()
		control := auth.NewMiddleware(authenticator2, authorizer2, auth.MiddlewareConfig{
			Namespace:    "kubernaut-system",
			Resource:     "services",
			ResourceName: "data-storage-service",
			Verb:         "create",
		}, controlHost)
		Expect(doRequest(control, "Bearer "+testToken)).To(Equal(http.StatusOK))
		Expect(zapSecurityEvents(controlBuf)).To(BeEmpty(),
			"control proves the WARN gate is real: without the dedicated logger the grant record is dropped")
	})

	It("UT-AUTH-2694-005: the dedicated logger preserves the exact security_event field contract including error_details on outages", func() {
		dedicatedLogs := &captureLog{}
		authenticator := &auth.MockAuthenticator{
			ErrorToReturn: fmt.Errorf("tokenreviews.authentication.k8s.io unreachable"),
		}
		authorizer := &auth.MockAuthorizer{}
		mw := auth.NewMiddleware(authenticator, authorizer, newConfig(logr.New(dedicatedLogs)), logr.New(&captureLog{}))

		Expect(doRequest(mw, "Bearer "+testToken)).To(Equal(http.StatusInternalServerError))

		events := dedicatedLogs.securityEvents()
		Expect(events).To(HaveLen(1))
		fields := events[0].fields
		for _, key := range []string{
			"event_id", "event_type", "event_action", "event_outcome", "reason",
			"correlation_id", "actor_type", "actor_id", "user", "status_code",
			"method", "path", "remote_addr", "resource", "resource_name", "verb",
			"error_details",
		} {
			Expect(fields).To(HaveKey(key), "ADR-034/DD-ERROR-001 field %q must survive the dedicated logger path", key)
		}
		record := events[0].record()
		Expect(record.Reason).To(Equal("authentication_service_unavailable"))
		Expect(record.ErrorDetails).NotTo(BeNil())
		Expect(record.ErrorDetails.Code).To(Equal("ERR_AUTH_SERVICE_UNAVAILABLE"))
		Expect(record.ErrorDetails.Component).To(Equal("auth-middleware"))
	})
})
