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

package telemetry

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// logExporter is a sdktrace.SpanExporter that writes one compact structured
// log line per completed span through a logr.Logger, instead of requiring a
// collector. Designed for CI/E2E troubleshooting and must-gather log
// correlation (GAP-14 / Issue #1519): grep a service's pod logs for a
// trace_id and get the same causal chain a trace backend would show, with
// no infrastructure dependency.
type logExporter struct {
	logger logr.Logger
}

func newLogExporter(logger logr.Logger) *logExporter {
	return &logExporter{logger: logger.WithName("otel-span")}
}

// ExportSpans implements sdktrace.SpanExporter.
func (e *logExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, s := range spans {
		sc := s.SpanContext()
		kvs := []interface{}{
			"trace_id", sc.TraceID().String(),
			"span_id", sc.SpanID().String(),
			"duration_ms", s.EndTime().Sub(s.StartTime()).Milliseconds(),
		}
		if parent := s.Parent(); parent.IsValid() {
			kvs = append(kvs, "parent_span_id", parent.SpanID().String())
		}
		for _, link := range s.Links() {
			kvs = append(kvs, "link_trace_id", link.SpanContext.TraceID().String())
		}
		for _, attr := range s.Attributes() {
			kvs = append(kvs, string(attr.Key), attr.Value.Emit())
		}

		if s.Status().Code == otelcodes.Error {
			e.logger.Error(errors.New(s.Status().Description), s.Name(), kvs...)
		} else {
			e.logger.Info(s.Name(), kvs...)
		}
	}
	return nil
}

// Shutdown implements sdktrace.SpanExporter. Nothing to flush: every span
// is written synchronously in ExportSpans.
func (e *logExporter) Shutdown(_ context.Context) error {
	return nil
}
