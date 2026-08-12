# ADR-068: OpenTelemetry Distributed Tracing Adoption

**Status**: ✅ Accepted
**Date**: 2026-08-11
**Decision Makers**: Architecture Team
**Impact**: Medium
**Related**: GAP-14 / Issue #1519 (GA Readiness Audit #1505), DD-OTEL-001 (detailed design), DD-AUDIT-003 (service audit trace requirements), ADR-034 (unified audit table design), ADR-030 (service configuration management), DD-005 (observability standards)

---

## Context

Issue #1519 (GAP-14) tracked a finding from the GA readiness audit (#1505): Kubernaut has no distributed tracing (OpenTelemetry). The audit's original scoping proposed instrumenting four services (Gateway, Signal Processing, AI Analysis, API Frontend) with `otelhttp` middleware, LLM call spans, and an operator-configurable OTLP collector endpoint, at an estimated 5-8 days of effort. It was deferred post-GA with an explicit note to re-scope (service list, exporter/backend choice, sampling strategy) when picked up, since it was "not required to close #1505" — Kubernaut's structured audit event trail (`correlation_id`-based reconstruction, ADR-034) already provides cross-service causal reconstruction for compliance purposes.

Picking this up required answering a question the original audit left open: **what does OTel add on top of a system that already has `correlation_id`-based causal reconstruction (ADR-034) and W3C-style request-ID log correlation (DD-005)?**

An exploratory spike on Auth Webhook (the first candidate pilot service) surfaced two findings that changed the shape of this decision:

1. **Cross-service trace continuation across an admission-webhook boundary requires object mutation, and Auth Webhook's `RemediationWorkflow` handler is registered as validating-only.** Auth Webhook is a dual-mode webhook server — it already runs a `MutatingWebhookConfiguration` (SOC2 user-attribution on `WorkflowExecution`/`RemediationApprovalRequest`/`RemediationRequest` status updates) alongside a `ValidatingWebhookConfiguration` (`NotificationRequest` DELETE, `RemediationWorkflow` CREATE/DELETE). The `RemediationWorkflow` handler specifically is wired as validating-only, per `DD-WEBHOOK-001`'s documented convention ("complex validation → `ValidatingWebhookConfiguration`, no mutation" vs. "authentication attribution → `MutatingWebhookConfiguration`") — its job is a business-decision gate (register the workflow with Data Storage, deny admission if that fails), which is squarely "complex validation" by that taxonomy. `ADR-058` is the originating decision: it evaluated and explicitly rejected a mutating-webhook design for `RemediationWorkflow` ("Alternative B: MutatingWebhook + JSON patch"), for a reason specific to that resource's `.status` subresource (JSON-patch mutation can't reach `.status` at CREATE-admission time) — a separate concern from the trace-link annotation question here, since a trace-link would land on `.metadata.annotations`, not `.status`, and so isn't blocked by that same subresource constraint. **This is a deliberate, documented design convention, not an accidental anti-pattern.** It is also not a hard technical wall: a `MutatingWebhookConfiguration` handler in K8s can still deny a request (`admission.Denied(...)`) while also returning a patch, and Auth Webhook's underlying Go handler (`admission.Admission{Handler: rwHandler}`) is agnostic to which config type routes to it — re-registering `RemediationWorkflow` under a `MutatingWebhookConfiguration` (identical deny logic, plus a trace-link patch) would be a small, well-scoped change (a webhook-config swap + a JSON-patch addition), not a redesign. It was not pursued because of finding 2 below, not because it was infeasible.
2. **The value of that cross-service link, once achievable, is thin.** Kubernaut already carries `correlation_id` (the `RemediationRequest` name) through every audit event and structured log line (ADR-034, DD-AUDIT-003). A trace-link annotation would add per-hop *timing* visibility on top of that but would not replace or improve causal reconstruction, which is already solved. For a K8s reconcile loop (inherently async, potentially re-entrant, and not a single call graph), an OTel span tree is a poor model of "what happened" compared to the existing audit trail — it answers "how long did each hop take," not "what happened and why," which the audit trail already answers.

These findings, plus a walk-through of the enterprise value proposition (performance/latency breakdown per hop — an APM-style concern — vs. causal/audit reconstruction, already solved), narrowed the scope of what OTel is *for* in Kubernaut: **per-hop timing visibility for troubleshooting and performance analysis, layered on top of (not replacing) the existing `correlation_id`/audit-trail causal model.**

---

## Decision

**Adopt OpenTelemetry distributed tracing for Kubernaut's stateless HTTP services, scoped to in-process and outbound-call spans only, exported via a bring-your-own-collector (BYO-collector) model with an optional zero-infrastructure log-sink alternative.**

### Scope

| Dimension | Decision |
|---|---|
| **Services** | Gateway (GW), Data Storage (DS), Kubernaut Agent (KA) — re-scoped from the original GAP-14 list (Gateway, Signal Processing, AI Analysis, API Frontend) based on spike findings (see Alternatives Considered) |
| **Span types** | Inbound root span per HTTP request (`otelhttp.NewMiddleware`); outbound child span per HTTP call to another service or external dependency (`otelhttp.NewTransport`) |
| **Cross-service continuation** | Standard W3C `traceparent` propagation for synchronous HTTP call chains (e.g. Gateway → Data Storage audit POST) — this is in scope and "free" via `otelhttp` |
| **Cross-service trace-link annotations** | **Out of scope.** No annotation-based span-linking across causally-disconnected boundaries (e.g. Gateway admission → Remediation Orchestrator reconcile). Rely on existing `correlation_id` for that reconstruction (see Alternatives Considered) |
| **Exporters** | OTLP/HTTP to an operator-supplied collector (BYO — Jaeger, Tempo, a vendor APM), and/or a log-sink exporter that writes one structured log line per completed span through the service's existing `logr.Logger` |
| **Default** | Fully disabled (zero-overhead no-op `TracerProvider`) unless an operator sets `telemetry.endpoint` and/or `telemetry.logSink: true` per ADR-030 YAML config |

### Non-Scope (Deferred / Rejected)

- CRD controllers (Signal Processing, AI Analysis, Remediation Orchestrator, Workflow Execution, etc.) — reconcile loops are not a natural fit for the span-tree model (see Context); not instrumented in this pass.
- Auth Webhook — its `RemediationWorkflow` handler is registered validating-only (a documented convention, `DD-WEBHOOK-001`, not an accidental limitation) and so cannot itself mutate objects to carry a trace-link; converting it to mutating is feasible but was deprioritized once the trace-link feature itself was dropped for ROI reasons (see Alternative A.1, C). The pilot moved to Gateway, which already had write access. Auth Webhook is left as-is (no OTel).
- Non-HTTP outbound dependencies (Postgres, Redis for Data Storage) — no `otelsql`/redis-client-hook instrumentation added; out of scope unless requested.
- An operated/managed OTel Collector — Kubernaut ships exporters only; the collector/backend is the operator's responsibility (BYO-collector).

---

## Consequences

### Positive

- **Zero cost when disabled.** The no-op `TracerProvider` means every call site (`otelhttp` middleware/transport wraps) compiles and runs everywhere, at effectively no runtime cost, until an operator opts in.
- **No new operational dependency by default.** The log-sink exporter gives troubleshooting value (spans as structured log lines, correlatable via `trace_id`, captured by existing `must-gather` and CI log collection) without requiring a collector to be deployed.
- **Per-hop latency visibility where it matters most.** Kubernaut Agent's outbound LLM call is the dominant cost of an AI-driven investigation; Data Storage is the hub every other service calls into. Both now have HTTP-standard timing spans.
- **"Free" instrumentation bonus.** The `ogen`-generated OpenAPI clients used for the Data Storage audit/workflow-catalog calls already bake in their own OTel spans (discovered during the spike) — no additional work needed to get a further sub-span per generated client call.
- **Reusable pattern.** The `pkg/shared/telemetry` package and the config/wiring pattern established here (config field → `NewTracerProvider` bootstrap → `otelhttp` middleware/transport wraps) is directly reusable if/when additional services are added later.

### Negative / Accepted Trade-offs

- **No unified cross-service trace for the full remediation journey.** An operator cannot follow one `trace_id` from a Prometheus alert landing on Gateway through to Workflow Execution completing an action — that journey is only reconstructable via `correlation_id` (as it already was before this work). This was an explicit, informed trade-off (see Alternatives Considered), not an oversight.
- **Partial service coverage.** Only 3 of Kubernaut's services have tracing. A future decision would be needed to extend coverage to CRD controllers if a concrete need arises (e.g. per-reconcile-phase timing).
- **BYO-collector still requires operator effort to get value from `Endpoint`.** The log-sink mitigates this for troubleshooting but does not replace a real APM backend for production-grade trace analysis (retention, sampling, trace-search UI).

---

## Alternatives Considered

### A. Implement the original GAP-14 scope as-is (Gateway, Signal Processing, AI Analysis, API Frontend + trace-link annotations) — ❌ Rejected

The original audit's proposed scope included cross-service trace-link propagation implicitly (a single trace following a signal from Gateway through the AI analysis controllers). The Auth Webhook spike showed this requires mutating admission webhooks or annotation write access at object-creation time, and — even where achievable (e.g. Gateway, which creates `RemediationRequest` and therefore *can* write annotations) — the ROI is thin given `correlation_id` already solves causal reconstruction (ADR-034). Continuing this scope would have spent effort on a feature with marginal value over the existing audit trail.

### A.1 Convert Auth Webhook's `RemediationWorkflow` validating webhook into a mutating one, to unblock trace-link annotations — ❌ Not pursued (feasible, but low value)

Technically viable (see finding 1 above) and cheaper than initially assessed — no new webhook-server infrastructure needed, just a K8s config-type swap plus a patch addition to the existing handler. Not pursued because it would blur `DD-WEBHOOK-001`'s clean "validation vs. attribution" separation of concerns for a single resource, in service of a feature (cross-service span links) independently found to have thin ROI over the existing `correlation_id` mechanism (Alternative C/D below). Revisit only if a concrete future need for `RemediationWorkflow`-specific mutation arises independent of tracing.

### B. No OpenTelemetry at all; rely solely on `correlation_id` + structured logs — ❌ Rejected

Considered directly after evaluating the enterprise value proposition. Rejected because per-hop *timing* is a genuinely different concern than causal reconstruction: `correlation_id` answers "what happened, in what order, across services," but does not answer "which hop was slow." Standard `otelhttp` spans answer the latter for free once wired, and OTLP export is the industry-standard on-ramp to any APM/backend an enterprise customer already operates (Jaeger, Tempo, Datadog, Dynatrace, etc.) — a capability `correlation_id` alone cannot provide.

### C. Cross-service span-link annotations via K8s object annotations, dropped after cost/value analysis — ❌ Rejected (implemented then removed)

Initially implemented for Gateway (`WriteTraceLinkAnnotation` stamping a `traceparent` onto `RemediationRequest` annotations at creation, for later extraction by Remediation Orchestrator's reconcile). Removed after a cost/value discussion: the annotation mechanism only linked Gateway's admission-time span to Remediation Orchestrator's reconcile-time span (a single hop of an already-multi-hop system), duplicated a correlation mechanism (`correlation_id`) that already exists and is authoritative for compliance/audit purposes, and added surface area (a K8s-object side-channel for trace context) for a niche, narrow benefit. See DD-OTEL-001 for the full alternatives analysis.

### D. In-process + outbound-only tracing, no cross-service span links, `correlation_id` remains the causal-reconstruction mechanism — ✅ Selected

Scoped OTel to what it does best and `correlation_id`/audit events do not: per-hop timing within and immediately downstream of a single service call. Kept the two mechanisms complementary rather than overlapping.

---

## Related Decisions

- **DD-OTEL-001**: OpenTelemetry Tracing Design (detailed design — config schema, per-service wiring manifest, log-sink exporter, alternatives analysis for the trace-link annotation removal)
- **DD-AUDIT-003**: Service Audit Trace Requirements — defines `correlation_id`-based cross-service causal reconstruction, the mechanism OTel deliberately does not duplicate
- **ADR-034**: Unified Audit Table Design — the audit event trail this decision treats as authoritative for causal/compliance reconstruction
- **ADR-030**: Service Configuration Management — YAML config convention followed by `TelemetryConfig`
- **ADR-058**: Webhook-Driven Workflow Registration — originating decision for `RemediationWorkflow`'s validating-only wiring (rejected a mutating-webhook alternative, for reasons unrelated to tracing)
- **DD-005**: Observability Standards — Prometheus metrics, structured logging, and `X-Request-ID` propagation; OTel tracing is a complementary, opt-in fourth pillar, not a replacement for any of the three
- **Issue #1519 (GAP-14)**: Original tracking issue and audit finding
- **Issue #95 / DD-GATEWAY-002**: A related but distinct initiative — ingesting OTLP traces as a Gateway signal *source* (not covered by this ADR, which is about Kubernaut *emitting* traces for its own observability)
