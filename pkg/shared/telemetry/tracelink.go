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

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TraceLinkAnnotation is the object annotation used to hand a trace context
// from an admission-time span to a later, causally-disconnected reconcile
// (GAP-14 / Issue #1519, design decision: span Links, not same-trace
// continuation -- see triage notes for issue #1519).
//
// A K8s admission webhook call and the reconcile it later triggers are NOT
// the same request: the webhook call completes synchronously against
// kube-apiserver, and the reconcile fires later from a watch event, with no
// in-band way to carry a traceparent between them. This annotation is the
// hand-off mechanism, mirroring how correlation_id is already propagated
// via CRD fields today (ADR-034).
const TraceLinkAnnotation = "kubernaut.ai/otel-trace-link"

// WriteTraceLinkAnnotation stamps the SpanContext active on ctx onto obj's
// annotations as a W3C traceparent string, so a later reconcile can
// re-attach it as a trace.Link via ExtractTraceLink. Returns false (no-op)
// if ctx carries no valid span, e.g. tracing is disabled -- callers should
// not treat this as an error.
func WriteTraceLinkAnnotation(ctx context.Context, obj metav1.Object) bool {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return false
	}

	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	traceparent := carrier.Get("traceparent")
	if traceparent == "" {
		return false
	}

	anns := obj.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	anns[TraceLinkAnnotation] = traceparent
	obj.SetAnnotations(anns)
	return true
}

// ExtractTraceLink reads a traceparent previously written by
// WriteTraceLinkAnnotation from obj's annotations and returns it as a
// trace.Link ready to pass to tracer.Start via trace.WithLinks. Returns
// ok=false if no valid annotation is present (tracing was disabled at
// admission time, the object predates this feature, or the object doesn't
// carry the annotation for any other reason) -- callers should proceed with
// a linkless root span rather than treat this as an error.
func ExtractTraceLink(obj metav1.Object) (link trace.Link, ok bool) {
	val := obj.GetAnnotations()[TraceLinkAnnotation]
	if val == "" {
		return trace.Link{}, false
	}

	carrier := propagation.MapCarrier{"traceparent": val}
	ctx := propagation.TraceContext{}.Extract(context.Background(), carrier)
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return trace.Link{}, false
	}

	return trace.Link{SpanContext: sc}, true
}
