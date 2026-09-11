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
//     -- Jaeger, Tempo, a vendor). Batched for throughput over mandatory TLS,
//     with an optional CAFile for a self-signed/private collector certificate.
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
	"time"

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

	// TLS configures trust and optional client authentication for the OTLP/HTTP
	// connection to Endpoint. Reuses
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
// rather than re-implementing it here.
func buildTLSConfig(t internalconfig.TelemetryTLSConfig) (*tls.Config, error) {
	if (t.CertFile == "") != (t.KeyFile == "") {
		return nil, fmt.Errorf("telemetry TLS certFile and keyFile must be set together")
	}
	var opts []sharedtls.TLSTransportOption
	if t.CertFile != "" && t.KeyFile != "" {
		opts = append(opts, sharedtls.WithClientCert(t.CertFile, t.KeyFile))
	}
	return sharedtls.BuildClientTLSConfig(t.CAFile, opts...)
}

// buildOTLPBatcherOption builds the sdktrace.WithBatcher option for
// cfg.Endpoint over TLS. Extracted out of NewTracerProvider to keep that
// function's branching flat.
func buildOTLPBatcherOption(ctx context.Context, cfg Config) (sdktrace.TracerProviderOption, error) {
	httpOpts := make([]otlptracehttp.Option, 1, 2)
	httpOpts[0] = otlptracehttp.WithEndpoint(cfg.Endpoint)
	tlsConfig, err := buildTLSConfig(cfg.TLS)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build TLS config: %w", err)
	}
	httpOpts = append(httpOpts, otlptracehttp.WithTLSClientConfig(tlsConfig))

	exporter, err := otlptracehttp.New(ctx, httpOpts...)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build OTLP exporter: %w", err)
	}
	return sdktrace.WithBatcher(exporter), nil
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
	if err := (internalconfig.TelemetryConfig{
		Endpoint: cfg.Endpoint,
		LogSink:  cfg.LogSink,
		TLS:      cfg.TLS,
	}).Validate(); err != nil {
		return nil, err
	}
	logSinkEnabled := cfg.LogSink || cfg.Endpoint == "stdout"
	if logSinkEnabled && cfg.Logger.GetSink() == nil {
		return nil, fmt.Errorf("telemetry: stdout or LogSink requires a Logger")
	}
	if cfg.Endpoint == "" && !logSinkEnabled {
		return noopShutdown, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}

	if cfg.Endpoint != "" && cfg.Endpoint != "stdout" {
		batcherOpt, err := buildOTLPBatcherOption(ctx, cfg)
		if err != nil {
			return nil, err
		}
		opts = append(opts, batcherOpt)
	}

	if logSinkEnabled {
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

// Bootstrap wraps NewTracerProvider with the identical log-and-defer
// boilerplate every service's main() needs (see cmd/gateway/main.go,
// cmd/datastorage/main.go, cmd/kubernautagent/main.go): log success/failure,
// and hand back a ready-to-defer shutdown func bounded by a 5s timeout. On
// failure (ok=false), the error is already logged via cfg.Logger and the
// returned shutdown is nil -- callers should exit non-zero without
// registering a defer for it.
func Bootstrap(ctx context.Context, cfg Config) (shutdown func(), ok bool) {
	tracerShutdown, err := NewTracerProvider(ctx, cfg)
	if err != nil {
		cfg.Logger.Error(err, "failed to initialize OpenTelemetry tracer provider")
		return nil, false
	}
	cfg.Logger.Info("OpenTelemetry tracing configured",
		"otlp_endpoint", cfg.Endpoint,
		"log_sink_enabled", cfg.LogSink)
	// Deliberately context.Background(), not ctx (the one passed to
	// NewTracerProvider above): this closure runs during graceful shutdown,
	// potentially after ctx itself has already been canceled -- draining
	// buffered spans needs its own independent, un-canceled deadline.
	//nolint:contextcheck
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracerShutdown(shutdownCtx); err != nil {
			cfg.Logger.Error(err, "failed to shut down tracer provider")
		}
	}, true
}
