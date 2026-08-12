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

// Package telemetry provides a shared OpenTelemetry TracerProvider setup for
// Kubernaut services.
//
// GAP-14 / Issue #1519: this package intentionally supports two independent,
// composable sinks:
//
//   - Endpoint: OTLP/HTTP export to a real collector/backend (bring-your-own
//     -- Jaeger, Tempo, a vendor). Batched for throughput. Plain HTTP by
//     default (TLS.Enabled=false); set TLS.Enabled to use HTTPS, with an
//     optional CAFile for a self-signed/private collector certificate.
//   - LogSink: a compact structured log line per span through the service's
//     existing logr.Logger. No collector needed -- lands in the same log
//     stream already captured by must-gather and CI log collection. Uses a
//     synchronous exporter (not batched) so a span survives a hard crash
//     (panic/OOM-kill) the same way a normal log line would.
//
// Both are opt-in and off by default (BYO-collector: absence of either is a
// valid, zero-overhead production configuration).
package telemetry

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// Config controls TracerProvider construction for one service process.
type Config struct {
	// ServiceName identifies this service in emitted spans (e.g. "authwebhook").
	// Required.
	ServiceName string

	// Endpoint is the OTLP/HTTP collector endpoint (host:port, no scheme).
	// Empty disables OTLP export.
	Endpoint string

	// TLS configures the OTLP/HTTP connection to Endpoint. Ignored when
	// Endpoint is empty. Zero value (Enabled=false) is a plain HTTP
	// connection -- the existing default, unchanged. Reuses
	// internal/config.TelemetryTLSConfig directly (rather than redefining an
	// identical shape here) so every caller passes serverCfg.Telemetry.TLS
	// straight through with no field-by-field remapping.
	TLS internalconfig.TelemetryTLSConfig

	// LogSink, when true, emits a compact structured log line per completed
	// span through Logger. Requires Logger to be set.
	LogSink bool

	// Logger receives span-completion log lines when LogSink is true.
	Logger logr.Logger
}

// buildTLSConfig delegates to pkg/shared/tls.BuildClientTLSConfig -- the
// same CA-loading, optional-mTLS, and process-wide SecurityProfile logic
// used by every other outbound TLS client in Kubernaut (Issue #493/#748) --
// rather than re-implementing it here. Returns nil if TLS is disabled.
func buildTLSConfig(t internalconfig.TelemetryTLSConfig) (*tls.Config, error) {
	if !t.Enabled {
		return nil, nil
	}

	var opts []sharedtls.TLSTransportOption
	if t.CertFile != "" && t.KeyFile != "" {
		opts = append(opts, sharedtls.WithClientCert(t.CertFile, t.KeyFile))
	}
	return sharedtls.BuildClientTLSConfig(t.CAFile, opts...)
}

// Shutdown flushes buffered spans and stops the TracerProvider. Callers
// MUST defer this during graceful shutdown.
type Shutdown func(context.Context) error

var noopShutdown Shutdown = func(context.Context) error { return nil }

// NewTracerProvider builds and registers a global TracerProvider for cfg,
// along with the W3C traceparent/baggage propagator. Returns a Shutdown
// func for graceful drain on process exit.
//
// If neither cfg.Endpoint nor cfg.LogSink is set, tracing stays disabled
// (OTel's default no-op provider remains active) so that instrumentation
// call sites (otelhttp middleware, tracer.Start, etc.) compile and run
// everywhere but cost effectively nothing until an operator opts in.
func NewTracerProvider(ctx context.Context, cfg Config) (Shutdown, error) {
	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("telemetry: ServiceName is required")
	}
	if cfg.LogSink && cfg.Logger.GetSink() == nil {
		return nil, fmt.Errorf("telemetry: LogSink requires a Logger")
	}
	if cfg.Endpoint == "" && !cfg.LogSink {
		return noopShutdown, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}

	if cfg.Endpoint != "" {
		httpOpts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.Endpoint)}
		if cfg.TLS.Enabled {
			tlsConfig, err := buildTLSConfig(cfg.TLS)
			if err != nil {
				return nil, fmt.Errorf("telemetry: build TLS config: %w", err)
			}
			httpOpts = append(httpOpts, otlptracehttp.WithTLSClientConfig(tlsConfig))
		} else {
			// Default: plain HTTP, matching most in-cluster collector
			// deployments (no TLS termination in front of the collector).
			httpOpts = append(httpOpts, otlptracehttp.WithInsecure())
		}

		exporter, err := otlptracehttp.New(ctx, httpOpts...)
		if err != nil {
			return nil, fmt.Errorf("telemetry: build OTLP exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}

	if cfg.LogSink {
		// WithSyncer (not WithBatcher): export on every span End() so a
		// span survives a hard crash immediately after an error, the same
		// way a normal log.Error() call already would.
		opts = append(opts, sdktrace.WithSyncer(newLogExporter(cfg.Logger)))
	}

	tp := sdktrace.NewTracerProvider(opts...)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
