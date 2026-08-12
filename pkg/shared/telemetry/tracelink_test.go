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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/telemetry"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UT-1519-005/006: the admission->reconcile trace-link handoff (GAP-14 /
// Issue #1519 design decision: span Links via a stored annotation, since
// admission and reconcile are causally-disconnected invocations).
var _ = Describe("Trace-link annotation round trip", func() {
	It("writes a traceparent annotation when the context carries a valid span", func() {
		tp := sdktrace.NewTracerProvider() // no exporter needed -- SpanContext validity only
		otel.SetTracerProvider(tp)
		defer func() { _ = tp.Shutdown(context.Background()) }()

		ctx, span := otel.Tracer("test").Start(context.Background(), "validate-remediationworkflow")
		defer span.End()

		obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rw-1"}}
		ok := telemetry.WriteTraceLinkAnnotation(ctx, obj)

		Expect(ok).To(BeTrue())
		Expect(obj.Annotations).To(HaveKey(telemetry.TraceLinkAnnotation))
		Expect(obj.Annotations[telemetry.TraceLinkAnnotation]).To(MatchRegexp(`^00-[0-9a-f]{32}-[0-9a-f]{16}-0[01]$`))
	})

	It("is a no-op when the context carries no valid span (tracing disabled)", func() {
		obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rw-2"}}
		ok := telemetry.WriteTraceLinkAnnotation(context.Background(), obj)

		Expect(ok).To(BeFalse())
		Expect(obj.Annotations).To(BeEmpty())
	})

	It("round-trips: a Link extracted from the annotation carries the original admission span's TraceID", func() {
		tp := sdktrace.NewTracerProvider()
		otel.SetTracerProvider(tp)
		defer func() { _ = tp.Shutdown(context.Background()) }()

		ctx, admissionSpan := otel.Tracer("test").Start(context.Background(), "validate-remediationworkflow")
		obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rw-3"}}
		Expect(telemetry.WriteTraceLinkAnnotation(ctx, obj)).To(BeTrue())
		admissionTraceID := admissionSpan.SpanContext().TraceID()
		admissionSpan.End()

		link, ok := telemetry.ExtractTraceLink(obj)

		Expect(ok).To(BeTrue())
		Expect(link.SpanContext.TraceID()).To(Equal(admissionTraceID))
	})

	It("returns ok=false when the object has no trace-link annotation", func() {
		obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "rw-4"}}
		_, ok := telemetry.ExtractTraceLink(obj)
		Expect(ok).To(BeFalse())
	})
})
