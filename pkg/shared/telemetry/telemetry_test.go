/*
Copyright 2025 Jordi Gil.

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

package telemetry_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/shared/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

func TestTelemetry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Telemetry Suite (GAP-14 / Issue #1519)")
}

// UT-1519-001: ServiceName is required.
var _ = Describe("NewTracerProvider validation", func() {
	It("rejects an empty ServiceName", func() {
		_, err := telemetry.NewTracerProvider(context.Background(), telemetry.Config{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ServiceName"))
	})

	It("rejects LogSink=true without a Logger", func() {
		_, err := telemetry.NewTracerProvider(context.Background(), telemetry.Config{
			ServiceName: "spike",
			LogSink:     true,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Logger"))
	})
})

// UT-1519-004: TLS.Enabled with a nonexistent CAFile surfaces a clear error
// instead of silently falling back to plaintext or an opaque SDK failure.
var _ = Describe("NewTracerProvider with TLS misconfigured", func() {
	It("returns an error naming the unreadable CA file", func() {
		_, err := telemetry.NewTracerProvider(context.Background(), telemetry.Config{
			ServiceName: "gateway",
			Endpoint:    "collector.example.com:4318",
			TLS: internalconfig.TelemetryTLSConfig{
				Enabled: true,
				CAFile:  "/nonexistent/ca.pem",
			},
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("/nonexistent/ca.pem"))
	})
})

// UT-1519-002: neither Endpoint nor LogSink configured => tracing stays
// disabled (BYO-collector default), no error, no panic.
var _ = Describe("NewTracerProvider with nothing configured", func() {
	It("returns a no-op shutdown without registering an SDK provider", func() {
		shutdown, err := telemetry.NewTracerProvider(context.Background(), telemetry.Config{
			ServiceName: "authwebhook",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(shutdown).NotTo(BeNil())
		Expect(shutdown(context.Background())).To(Succeed())
	})
})

// UT-1519-003: LogSink emits a compact structured log line per span through
// the caller's logr.Logger -- no collector required.
var _ = Describe("NewTracerProvider with LogSink enabled", func() {
	It("logs a line containing the span name, trace_id, and duration for each completed span", func() {
		var lines []string
		testLogger := funcr.New(func(prefix, args string) {
			lines = append(lines, prefix+" "+args)
		}, funcr.Options{})

		shutdown, err := telemetry.NewTracerProvider(context.Background(), telemetry.Config{
			ServiceName: "authwebhook",
			LogSink:     true,
			Logger:      testLogger,
		})
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = shutdown(context.Background()) }()

		tracer := otel.Tracer("spike-test")
		_, span := tracer.Start(context.Background(), "rw_reconciler.Reconcile")
		span.End()

		Expect(lines).To(HaveLen(1))
		Expect(lines[0]).To(ContainSubstring("rw_reconciler.Reconcile"))
		Expect(lines[0]).To(ContainSubstring("trace_id"))
		Expect(lines[0]).To(ContainSubstring("duration_ms"))
	})

	It("includes the span's error description when the span status is Error", func() {
		var lines []string
		testLogger := funcr.New(func(prefix, args string) {
			lines = append(lines, prefix+" "+args)
		}, funcr.Options{})

		shutdown, err := telemetry.NewTracerProvider(context.Background(), telemetry.Config{
			ServiceName: "authwebhook",
			LogSink:     true,
			Logger:      testLogger,
		})
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = shutdown(context.Background()) }()

		tracer := otel.Tracer("spike-test")
		_, span := tracer.Start(context.Background(), "rw_reconciler.Reconcile")
		span.SetStatus(codes.Error, "failed to get RemediationWorkflow")
		span.End()

		Expect(lines).To(HaveLen(1))
		Expect(lines[0]).To(ContainSubstring("failed to get RemediationWorkflow"))
	})
})
