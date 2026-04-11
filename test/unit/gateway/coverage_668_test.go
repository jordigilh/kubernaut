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

package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	gwconfig "github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"
	"github.com/jordigilh/kubernaut/pkg/gateway/processing"
)

// spyLogSink implements logr.LogSink for assertions on adapter logging (BR-GATEWAY-004).
type spyLogSink struct {
	errorCount int
	names      []string
}

func (s *spyLogSink) Init(_ logr.RuntimeInfo) {}

func (s *spyLogSink) Enabled(_ int) bool { return true }

func (s *spyLogSink) Info(_ int, _ string, _ ...interface{}) {}

func (s *spyLogSink) Error(_ error, _ string, _ ...interface{}) { s.errorCount++ }

func (s *spyLogSink) WithValues(...interface{}) logr.LogSink { return s }

func (s *spyLogSink) WithName(name string) logr.LogSink {
	s.names = append(s.names, name)
	return s
}

type failingOwnerResolver struct{}

func (failingOwnerResolver) ResolveTopLevelOwner(context.Context, string, string, string) (string, string, error) {
	return "", "", errors.New("intentional owner resolution failure for BR-GATEWAY-004 coverage")
}

var _ = Describe("BR-GATEWAY-004: Kubernetes Event adapter SetLogger", func() {
	It("BR-GATEWAY-004: wires the configured logger into fingerprint resolution so owner errors are logged", func() {
		spy := &spyLogSink{}
		logger := logr.New(spy)
		adapter := adapters.NewKubernetesEventAdapter(failingOwnerResolver{})
		adapter.SetLogger(logger)

		Expect(spy.names).To(ContainElement("kubernetes-event-adapter"),
			"SetLogger must apply the adapter name for observability routing")

		payload := []byte(`{
			"type": "Warning",
			"reason": "OOMKilled",
			"involvedObject": {"kind": "Pod", "namespace": "ns", "name": "pod-1"}
		}`)
		_, err := adapter.Parse(context.Background(), payload)
		Expect(err).To(HaveOccurred())
		Expect(spy.errorCount).To(BeNumerically(">=", 1),
			"ResolveFingerprint must emit Error on owner resolution failure using the logger from SetLogger")
	})
})

var _ = Describe("BR-GATEWAY-074: Kubernetes Event adapter ReplayValidator", func() {
	It("BR-GATEWAY-074: rejects POST requests when body lastTimestamp is older than tolerance (replay/stale signal)", func() {
		adapter := adapters.NewKubernetesEventAdapter()
		tolerance := 30 * time.Second
		staleTime := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339Nano)
		body := fmt.Sprintf(`{"lastTimestamp":%q,"reason":"x","involvedObject":{"kind":"Pod","name":"p"}}`, staleTime)

		var nextCalled bool
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true })

		wrapped := adapter.ReplayValidator(tolerance)(next)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/signals/kubernetes-event", strings.NewReader(body))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		Expect(nextCalled).To(BeFalse(), "stale events must not reach the handler")
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	It("BR-GATEWAY-075: invokes the next handler when event timestamps are within the tolerance window", func() {
		adapter := adapters.NewKubernetesEventAdapter()
		tolerance := 5 * time.Minute
		fresh := time.Now().Add(-10 * time.Second).UTC().Format(time.RFC3339Nano)
		body := fmt.Sprintf(`{"lastTimestamp":%q}`, fresh)

		var nextCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			b, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(body), "middleware must rewind the body for downstream parsing")
			w.WriteHeader(http.StatusNoContent)
		})

		wrapped := adapter.ReplayValidator(tolerance)(next)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/signals/kubernetes-event", strings.NewReader(body))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		Expect(nextCalled).To(BeTrue())
		Expect(rec.Code).To(Equal(http.StatusNoContent))
	})
})

var _ = Describe("BR-GATEWAY-074: Prometheus adapter ReplayValidator", func() {
	var (
		adapter *adapters.PrometheusAdapter
	)

	BeforeEach(func() {
		adapter = adapters.NewPrometheusAdapter(nil, nil)
	})

	It("BR-GATEWAY-074: accepts AlertManager JSON using startsAt when X-Timestamp is absent", func() {
		startsAt := time.Now().UTC().Format(time.RFC3339Nano)
		body := fmt.Sprintf(`{"alerts":[{"startsAt":%q,"labels":{"alertname":"A","severity":"warning","pod":"p"}}]}`, startsAt)

		var nextCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			b, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).To(Equal(body))
			w.WriteHeader(http.StatusAccepted)
		})

		wrapped := adapter.ReplayValidator(5 * time.Minute)(next)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/signals/prometheus", strings.NewReader(body))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		Expect(nextCalled).To(BeTrue())
		Expect(rec.Code).To(Equal(http.StatusAccepted))
	})

	It("BR-GATEWAY-074: rejects AlertManager payloads whose startsAt is beyond clock-skew tolerance", func() {
		future := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339Nano)
		body := fmt.Sprintf(`{"alerts":[{"startsAt":%q}]}`, future)

		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			Fail("next handler must not run when startsAt is in the future")
		})

		wrapped := adapter.ReplayValidator(5 * time.Minute)(next)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/signals/prometheus", strings.NewReader(body))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	It("BR-GATEWAY-075: applies strict X-Timestamp validation when the header is present", func() {
		oldTS := strconv.FormatInt(time.Now().Add(-2*time.Hour).Unix(), 10)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/signals/prometheus", strings.NewReader(`{}`))
		req.Header.Set("X-Timestamp", oldTS)

		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			Fail("next handler must not run when X-Timestamp is outside the tolerance window")
		})

		wrapped := adapter.ReplayValidator(30 * time.Second)(next)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	It("BR-GATEWAY-074: forwards the request when X-Timestamp is within the tolerance window", func() {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/signals/prometheus", nil)
		req.Header.Set("X-Timestamp", ts)

		var nextCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		wrapped := adapter.ReplayValidator(5 * time.Minute)(next)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		Expect(nextCalled).To(BeTrue())
		Expect(rec.Code).To(Equal(http.StatusOK))
	})
})

// UT-GW-668-004: Gateway Metrics (BR-GATEWAY-008)
var _ = Describe("UT-GW-668-004: Gateway Metrics (BR-GATEWAY-008)", func() {
	It("BR-GATEWAY-008: should create metrics with custom registry", func() {
		reg := prometheus.NewRegistry()
		m := metrics.NewMetricsWithRegistry(reg)
		Expect(m).NotTo(BeNil())
		Expect(m.Registry()).NotTo(BeNil())
	})
})

// UT-GW-668-005: Gateway Config (BR-GATEWAY-010)
var _ = Describe("UT-GW-668-005: Gateway Config (BR-GATEWAY-010)", func() {
	It("BR-GATEWAY-010: should load config from YAML file", func() {
		tmpDir, err := os.MkdirTemp("", "gw-config-*")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(tmpDir)

		cfgYAML := `
server:
  listenAddr: ":8080"
  readTimeout: 10s
  writeTimeout: 30s
datastorage:
  url: "http://ds:8080"
`
		cfgPath := filepath.Join(tmpDir, "config.yaml")
		Expect(os.WriteFile(cfgPath, []byte(cfgYAML), 0644)).To(Succeed())

		cfg, err := gwconfig.LoadFromFile(cfgPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Server.ListenAddr).To(Equal(":8080"))
	})

	It("BR-GATEWAY-010: should return error for nonexistent config file (Issue #674 Bug 4 fix)", func() {
		_, err := gwconfig.LoadFromFile("/nonexistent/config.yaml")
		Expect(err).To(HaveOccurred())
	})

	It("BR-GATEWAY-010: LoadFromEnv should be callable without panic", func() {
		cfg := gwconfig.DefaultServerConfig()
		Expect(func() { cfg.LoadFromEnv() }).NotTo(Panic())
	})

	It("BR-GATEWAY-010: ServerConfig.Validate should reject empty listenAddr", func() {
		cfg := gwconfig.DefaultServerConfig()
		cfg.Server.ListenAddr = ""
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("listenAddr"))
	})

	It("BR-GATEWAY-010: RetrySettings.Validate should reject maxAttempts < 1", func() {
		r := gwconfig.RetrySettings{MaxAttempts: 0, InitialBackoff: 100 * time.Millisecond, MaxBackoff: 5 * time.Second}
		Expect(r.Validate()).To(HaveOccurred())
	})

	It("BR-GATEWAY-010: RetrySettings.Validate should reject maxAttempts > 10", func() {
		r := gwconfig.RetrySettings{MaxAttempts: 15, InitialBackoff: 100 * time.Millisecond, MaxBackoff: 5 * time.Second}
		Expect(r.Validate()).To(HaveOccurred())
	})

	It("BR-GATEWAY-010: RetrySettings.Validate should reject negative initialBackoff", func() {
		r := gwconfig.RetrySettings{MaxAttempts: 3, InitialBackoff: -1, MaxBackoff: 5 * time.Second}
		Expect(r.Validate()).To(HaveOccurred())
	})

	It("BR-GATEWAY-010: RetrySettings.Validate should reject initialBackoff > 5s", func() {
		r := gwconfig.RetrySettings{MaxAttempts: 3, InitialBackoff: 10 * time.Second, MaxBackoff: 30 * time.Second}
		Expect(r.Validate()).To(HaveOccurred())
	})

	It("BR-GATEWAY-010: RetrySettings.Validate should reject maxBackoff < initialBackoff", func() {
		r := gwconfig.RetrySettings{MaxAttempts: 3, InitialBackoff: 2 * time.Second, MaxBackoff: 1 * time.Second}
		Expect(r.Validate()).To(HaveOccurred())
	})

	It("BR-GATEWAY-010: RetrySettings.Validate should reject maxBackoff > 30s", func() {
		r := gwconfig.RetrySettings{MaxAttempts: 3, InitialBackoff: 100 * time.Millisecond, MaxBackoff: 60 * time.Second}
		Expect(r.Validate()).To(HaveOccurred())
	})

	It("BR-GATEWAY-010: RetrySettings.Validate should accept valid settings", func() {
		r := gwconfig.DefaultRetrySettings()
		Expect(r.Validate()).NotTo(HaveOccurred())
	})
})

// UT-GW-668-006: Gateway Processing Errors (BR-GATEWAY-012)
var _ = Describe("UT-GW-668-006: Gateway Processing Errors (BR-GATEWAY-012)", func() {
	It("BR-GATEWAY-012: NewDeduplicationError should produce actionable error message", func() {
		startTime := time.Now().Add(-100 * time.Millisecond)
		underlying := errors.New("k8s api timeout")

		dedupErr := processing.NewDeduplicationError(
			"fp-abc123", "default", "new", 2, startTime, underlying,
		)
		Expect(dedupErr.Error()).To(ContainSubstring("check_deduplication failed"))
		Expect(dedupErr.Error()).To(ContainSubstring("dedupe_status=new"))
		Expect(dedupErr.Error()).To(ContainSubstring("fp-abc123"))
		Expect(dedupErr.Unwrap()).To(Equal(underlying))
	})

	It("BR-GATEWAY-012: NewCRDCreationError should include CRD and signal context", func() {
		startTime := time.Now().Add(-50 * time.Millisecond)
		underlying := errors.New("conflict")

		crdErr := processing.NewCRDCreationError(
			"fp-xyz", "prod-ns", "rr-pod-crash-xyz", "prometheus-alert", "HighMemory",
			3, startTime, underlying,
		)
		Expect(crdErr.Error()).To(ContainSubstring("create_remediation_request failed"))
		Expect(crdErr.Error()).To(ContainSubstring("crd_name=rr-pod-crash-xyz"))
		Expect(crdErr.Error()).To(ContainSubstring("signal_type=prometheus-alert"))
		Expect(crdErr.Unwrap()).To(Equal(underlying))
	})

	It("BR-GATEWAY-012: MockClock.Set should allow deterministic time in tests", func() {
		clock := processing.NewMockClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		target := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
		clock.Set(target)
		Expect(clock.Now()).To(Equal(target))
	})
})
