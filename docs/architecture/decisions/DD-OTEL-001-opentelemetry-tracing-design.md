# DD-OTEL-001: OpenTelemetry Tracing Design

**Date**: 2026-08-11
**Status**: ✅ **APPROVED**
**Builds On**: ADR-068 (OpenTelemetry Distributed Tracing Adoption)
**Confidence**: 90%
**Decision Makers**: Architecture Team
**Affected Services**: Gateway (GW), Data Storage (DS), Kubernaut Agent (KA)
**Tracking**: GAP-14 / Issue #1519

---

## 🎯 **DECISION**

**Gateway, Data Storage, and Kubernaut Agent SHALL instrument inbound HTTP requests and outbound HTTP calls with OpenTelemetry (`otelhttp`), bootstrapped from a shared `pkg/shared/telemetry` package, configured per ADR-030, and exported to either an OTLP/HTTP collector, a structured-log sink, both, or neither (fully disabled, zero-overhead default).**

### Scope

- **Services**: Gateway (`cmd/gateway`), Data Storage (`cmd/datastorage`), Kubernaut Agent (`cmd/kubernautagent`)
- **Span types**: inbound root span per HTTP request; outbound child span per HTTP call to another Kubernaut service, LLM provider, or Prometheus
- **Exporters**: OTLP/HTTP (BYO-collector) and/or a custom log-based `sdktrace.SpanExporter`

### Non-Scope

- CRD controllers (reconcile-loop instrumentation) — not a natural fit for the span-tree model; deferred (see ADR-068 Context)
- Auth Webhook — `RemediationWorkflow` handler is validating-only by documented convention (`DD-WEBHOOK-001`), not a hard technical limitation (see §5, Alternative B); parked because the trace-link feature itself was dropped for ROI reasons, not because converting it was infeasible
- Cross-service trace-link annotations (span `Links` via K8s object annotations) — implemented for Gateway, then **removed** after cost/value analysis (§5)
- Non-HTTP outbound instrumentation (Postgres, Redis) — Data Storage's DB/cache calls are not wrapped; would require `otelsql` / redis-client hooks, out of scope unless requested
- Operating/managing an OTel Collector — Kubernaut exports; the collector/backend is the operator's

---

## 📊 **Context & Problem**

### Business Requirements

1. **Per-hop latency visibility**: identify which hop in a request chain (Gateway → Data Storage; Kubernaut Agent → LLM provider) is slow, without re-deriving this from timestamps scattered across separate log lines.
2. **Zero infrastructure dependency by default**: many deployments (CI, air-gapped, early adopters) have no OTel collector. Tracing must degrade to "off" with zero overhead, not become a hard dependency.
3. **CI/CD and `must-gather` troubleshooting**: traces should be usable for debugging test failures and production incidents without deploying a trace backend.
4. **No duplication of the existing causal-reconstruction mechanism**: Kubernaut already reconstructs cross-service causality via `correlation_id` (the `RemediationRequest` name), carried through every audit event (ADR-034) and structured log line (DD-005). OTel must add value on top of this, not duplicate it.

### Problem Statement

The GA readiness audit (#1505 → GAP-14 → Issue #1519) flagged the absence of distributed tracing. An exploratory spike (see §5) on the first candidate pilot service, Auth Webhook, surfaced that:

- Auth Webhook's `RemediationWorkflow` admission handler is a **validating** webhook — it can allow/deny a request but cannot mutate the object being admitted, so it cannot stamp a trace-context annotation for a later reconcile to pick up.
- Even where write access exists (Gateway, which creates `RemediationRequest` objects directly), the value of a cross-service span *link* on top of the existing `correlation_id` mechanism is marginal: it adds timing, not causal understanding, and K8s reconcile loops are asynchronous/re-entrant in a way that doesn't map cleanly onto a single span tree.

This narrowed OTel's role in Kubernaut to: **HTTP-hop timing, within a service and to its immediate synchronous HTTP dependencies**, layered on top of (not replacing) `correlation_id`.

---

## 🔍 **Alternatives Considered**

### Alternative A: Instrument the original GAP-14 service list (Gateway, Signal Processing, AI Analysis, API Frontend) — ❌ Rejected

The original audit scoped tracing to the services on the signal → AI-analysis path, implicitly following the *request*, not just individual HTTP hops. Signal Processing and AI Analysis are CRD controllers, not HTTP services in the request/response sense — reconcile loops don't have a natural "inbound request span" the way a `chi.Router` handler does. Re-scoped to the three stateless HTTP services where the `otelhttp` middleware/transport pattern applies directly.

### Alternative B: Auth Webhook as the pilot service, with a mutating-webhook rewrite to enable trace-link annotations — ❌ Not pursued (feasible, but low value)

**Clarification (corrects an earlier framing of this as a hard blocker)**: Auth Webhook already runs both a `MutatingWebhookConfiguration` (SOC2 attribution on `WorkflowExecution`/`RemediationApprovalRequest`/`RemediationRequest` status updates — `deploy/authwebhook/06-mutating-webhook.yaml`) and a `ValidatingWebhookConfiguration` (`RemediationWorkflow` CREATE/DELETE, `NotificationRequest` DELETE — `07-validating-webhook.yaml` and the Helm-templated equivalent). `RemediationWorkflow`'s handler is validating-only by deliberate, documented convention (`DD-WEBHOOK-001`: "complex validation → Validating, no mutation" vs. "authentication attribution → Mutating"), not by accident — its job (register the workflow with Data Storage, deny on failure) is exactly the "complex validation" case that convention describes. `ADR-058` is the originating decision and already evaluated a mutating-webhook design for this resource ("Alternative B") — rejected there because `RemediationWorkflow`'s `.status` subresource can't be JSON-patch-mutated at CREATE-admission time. That's a distinct concern from a trace-link annotation, which would target `.metadata.annotations` and isn't subject to the same subresource constraint.

This is also not a hard technical wall: K8s allows a `MutatingWebhookConfiguration` handler to both deny (`admission.Denied(...)`) and patch, and Auth Webhook's Go handler (`admission.Admission{Handler: rwHandler}` in `cmd/authwebhook/main.go`) doesn't care which config type routes to it. Converting `RemediationWorkflow`'s registration from Validating to Mutating (same deny logic + a trace-link patch) would have been a small, well-scoped change — a webhook-config swap plus a JSON-patch addition — not new webhook-server infrastructure or a validating/mutating architecture rewrite.

**Not pursued** because doing so would blur `DD-WEBHOOK-001`'s clean separation of concerns for one resource, in service of a feature independently found to have thin ROI (Alternative C below) — not because it was infeasible. Pivoted the pilot to Gateway instead, which already has write access to the object it creates, sidestepping the question entirely.

### Alternative C: Cross-service span-link annotations on `RemediationRequest`, via Gateway — ❌ Rejected (implemented, then removed)

**Approach**: `WriteTraceLinkAnnotation(ctx, obj)` (in `pkg/shared/telemetry/tracelink.go`) stamps the active `SpanContext` as a W3C `traceparent` string onto `obj`'s annotations (`kubernaut.ai/otel-trace-link`) at `RemediationRequest` creation time in `pkg/gateway/processing/crd_creator.go`. A later reconciler would call `ExtractTraceLink(obj)` to re-attach it as a `trace.Link` on a new root span.

**Why implemented first**: Gateway, unlike Auth Webhook, has full write access to the object it creates — no validating-webhook blocker.

**Why removed**:
- **Duplicated an existing, authoritative mechanism.** `correlation_id` (the `RemediationRequest` name) already flows through every audit event (ADR-034) and structured log line, and is the mechanism DD-AUDIT-003 designates for cross-service causal reconstruction, including for compliance purposes (AU-2/AU-3). A trace-link annotation would only ever cover *one additional hop* (Gateway → whichever reconciler reads the annotation), not the full remediation journey — a narrow, partial improvement over a mechanism that already covers the whole journey.
- **Timing-only value, and niche.** The only genuinely new information a span link adds is "how long between admission and reconcile pickup" — a real but narrow signal, not worth the annotation plumbing, an extra `pkg/shared/telemetry` surface (`tracelink.go`), and its test suite, for one hop.
- **User cost/value framing** (verbatim from the deciding conversation): *"we already capture our own audits, so otel does not help here, the logs are already correlated with the rr_id. So the only thing is performance timing upon requests... I can live with that, but it's a niche scope."* This framed span-links as the niche part of an already-niche feature, i.e. not worth carrying forward.

**Disposition**: `pkg/gateway/processing/crd_creator.go`'s call site and `pkg/gateway/processing/crd_creator_tracing_test.go` were removed. `pkg/shared/telemetry/tracelink.go` (`WriteTraceLinkAnnotation`/`ExtractTraceLink`) and its test file were **retained** in the shared package, unused by any service, in case a future concrete need justifies revisiting this — the mechanism is validated and tested, just not wired into production for now.

### Alternative D: In-process + outbound-only spans, `correlation_id` remains the sole cross-service causal-reconstruction mechanism — ✅ Selected

Every span produced is either (a) a root span for one inbound HTTP request, or (b) a child span for one outbound HTTP call made while handling that request. No span ever crosses an async/reconcile boundary. This keeps OTel's job narrow and well-defined: per-hop timing, nothing else.

### Alternative E: Full OTLP-only export, no log-sink — ❌ Rejected

Considered restricting exporters to OTLP/HTTP only (simpler package, one less code path). Rejected because it would make tracing useless without an operated collector, defeating the CI/CD and `must-gather` troubleshooting use case that motivated re-prioritizing this work. The log-sink adds one file (`log_exporter.go`, ~55 lines) for a capability with no infrastructure cost.

---

## 🎯 **Architecture**

### Shared Package: `pkg/shared/telemetry`

```
pkg/shared/telemetry/
├── telemetry.go        # Config, NewTracerProvider, Shutdown
├── log_exporter.go     # logExporter: sdktrace.SpanExporter -> logr.Logger
├── tracelink.go         # WriteTraceLinkAnnotation / ExtractTraceLink (retained, unused -- see Alternative C)
└── *_test.go
```

**`Config` / `NewTracerProvider` / `Bootstrap`** (`telemetry.go`):

```go
type Config struct {
    ServiceName string                          // required, e.g. "gateway", "datastorage", "kubernaut-agent"
    Endpoint    string                          // OTLP/HTTP collector host:port; empty disables OTLP export
    TLS         internalconfig.TelemetryTLSConfig // OTLP/HTTP connection security; ignored when Endpoint == ""
    LogSink     bool                            // emit one log line per span via Logger
    Logger      logr.Logger                     // required if LogSink is true
}

type Shutdown func(context.Context) error

func NewTracerProvider(ctx context.Context, cfg Config) (Shutdown, error)

// Bootstrap wraps NewTracerProvider with the log-success/failure +
// bounded-shutdown-timeout boilerplate every service's main() needs.
// Extracted (after this DD was first written) out of three near-identical
// call sites in cmd/{gateway,datastorage,kubernautagent}/main.go to keep
// each under the funlen lint budget -- all three services now bootstrap via
// this, not NewTracerProvider directly.
func Bootstrap(ctx context.Context, cfg Config) (shutdown func(), ok bool)
```

- `Endpoint == "" && !LogSink` -> returns a no-op `Shutdown`, OTel's default no-op `TracerProvider` stays globally registered. This is the default, zero-overhead state.
- `Endpoint != ""` -> adds an `otlptracehttp` exporter via `sdktrace.WithBatcher` (throughput-optimized), over TLS if `TLS.Enabled`.
- `LogSink == true` -> adds the custom `logExporter` via `sdktrace.WithSyncer` (synchronous — a span survives a hard crash/OOM-kill immediately after export, matching how a normal `logger.Error()` call already behaves; batching would risk losing the very spans most useful for post-mortem).
- Both can be enabled simultaneously; they are independent, composable sinks.
- Always registers the W3C `TraceContext` + `Baggage` propagator globally (`otel.SetTextMapPropagator`), regardless of exporter state, so `otelhttp` middleware/transport call sites behave identically whether tracing is active or not.

**Log-sink exporter** (`log_exporter.go`): implements `sdktrace.SpanExporter.ExportSpans` by writing one `logger.Info`/`logger.Error` call per completed span, with `trace_id`, `span_id`, `duration_ms`, `parent_span_id` (if any), any `Links`, and all span attributes as key-value pairs. Errors (`Status().Code == otelcodes.Error`) log via `logger.Error`; everything else via `logger.Info`. This makes a trace greppable directly from `kubectl logs` or a `must-gather` bundle by `trace_id`, without a collector.

### Config Schema (ADR-030)

`internal/config/telemetry.go` defines the shared, embeddable config struct:

```go
type TelemetryConfig struct {
    Endpoint string              `yaml:"endpoint,omitempty"` // OTLP/HTTP collector; empty disables OTLP export
    LogSink  bool                `yaml:"logSink,omitempty"`  // per-span structured log line
    TLS      TelemetryTLSConfig  `yaml:"tls,omitempty"`      // OTLP/HTTP connection security; ignored when Endpoint == ""
}

// Mirrors pkg/datastorage/config.RedisTLSConfig's shape for consistency.
type TelemetryTLSConfig struct {
    Enabled  bool   `yaml:"enabled,omitempty"`  // false (default) = plain HTTP
    CAFile   string `yaml:"caFile,omitempty"`   // self-signed/private collector cert; empty trusts system CA pool
    CertFile string `yaml:"certFile,omitempty"` // optional mTLS client cert
    KeyFile  string `yaml:"keyFile,omitempty"`  // optional mTLS client key
}

func DefaultTelemetryConfig() TelemetryConfig {
    return TelemetryConfig{Endpoint: "", LogSink: false}
}
```

**TLS default is plain HTTP** (`TLS.Enabled == false`), matching the common case of an in-cluster OTel Collector `Service` with no TLS termination. Set `TLS.Enabled: true` for a collector that terminates TLS (vendor SaaS endpoint, or an in-cluster collector behind a TLS-terminating route/ingress); set `TLS.CAFile` when that collector presents a self-signed or privately-issued certificate. `CertFile`/`KeyFile` are for the uncommon case of a collector requiring mTLS client authentication.

**No per-telemetry cipher-suite or TLS-version field, by design.** `TelemetryTLSConfig` has no `cipherSuites`/`minVersion`-style field to configure — when `TLS.Enabled`, the exporter's `*tls.Config` is built via `pkg/shared/tls.BuildClientTLSConfig`, which unconditionally applies the process-wide `SecurityProfile` (`ApplyProfile`: minimum TLS version, cipher suites, curve preferences) that every other Kubernaut TLS client (Redis, Postgres, inter-service mTLS) already uses. This is deliberate: cipher/version policy is a fleet-wide security posture set once (Issue #493/#748), not a per-connection knob — allowing OTel's exporter to pick weaker ciphers than the rest of the fleet would be a needless inconsistency. IT-1519-007 proves this profile is enforced on the actual negotiated connection, not just present in the in-memory config.

Each service embeds this in its own YAML-backed config struct and merges it into `telemetry.Config` at startup (service name is the only field each `main.go` supplies itself).

```yaml
# example: any of the three services' YAML config
telemetry:
  endpoint: "otel-collector.observability.svc:4318"  # optional, BYO-collector
  logSink: true                                       # optional, CI/must-gather friendly
  tls:
    enabled: false        # true for a TLS-terminating collector
    caFile: ""             # self-signed collector cert, if any
    certFile: ""           # optional mTLS client cert
    keyFile: ""            # optional mTLS client key
```

### Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | Test ID |
|---|---|---|---|
| Gateway/DS/KA `TracerProvider` bootstrap (TLS branch) | `main()` (all 3 services, via `telemetry.Bootstrap`) | `pkg/shared/telemetry/telemetry.go` (`NewTracerProvider`, wrapped by `Bootstrap` — shared by all 3 `cmd/*/main.go`) | `pkg/shared/telemetry/tls_internal_test.go` (UT-1519-005..012: `buildTLSConfig` branching logic) + `test/integration/shared/telemetry/otel_tls_wiring_integration_test.go` (**IT-1519-001/002/006/006b/007/008**: real TLS-encrypted span delivery to a trusted collector; fail-closed rejection of an untrusted one; a real mTLS handshake when the collector requires a client cert, and fail-closed when none is presented; the process-wide `SecurityProfile`'s minimum TLS version enforced on the actual negotiated connection, not just the in-memory config; and the exported span payload never containing the inbound request's bearer token — all through the same `otelhttp.NewMiddleware` call production servers use) |
| Gateway `TracerProvider` bootstrap (no-TLS path) | `run()` | `cmd/gateway/main.go` (`telemetry.Bootstrap`) | Manual/build verification (bootstrap-only, no branching logic to unit test) |
| Gateway inbound root span | `setupRoutes()` (`chi.Router` construction) | `pkg/gateway/server.go` (`otelhttp.NewMiddleware("gateway.http", ...)`) | `pkg/gateway/otel_middleware_test.go` (UT: direct middleware construction) + `test/integration/gateway/otel_tls_wiring_integration_test.go` (**IT-1519-003/009/010**: real webhook POST through the actual `gateway.Server.Handler()` — real auth, scope check, CRD creation, DS audit — exports a span over TLS (003); the same real dispatch still succeeds and delivers zero spans when the collector's cert is untrusted (009); and still completes promptly when the collector never responds at all (010)) |
| Gateway outbound span (Data Storage audit client) | `buildAuditStore` | `pkg/gateway/server_constructors.go` (`otelhttp.NewTransport(...)` wrapping `dsTransport`) | `pkg/gateway/ds_transport_tracing_test.go` |
| Data Storage `TracerProvider` bootstrap (no-TLS path) | `run()` (via `loadRunConfig()`) | `cmd/datastorage/main.go` (`telemetry.Bootstrap`) | Manual/build verification |
| Data Storage inbound root span | `Handler()` (`chi.Router` construction) | `pkg/datastorage/server/server_routes.go` (`otelhttp.NewMiddleware("datastorage.http", ...)`) | `pkg/datastorage/server/otel_middleware_test.go` (UT-1519-010) + `test/integration/datastorage/otel_tls_wiring_integration_test.go` (**IT-1519-004**: real authenticated GET through `server.NewServer`'s actual `Handler()` reading real seeded Postgres data, exports a span over TLS) |
| Kubernaut Agent `TracerProvider` bootstrap | `main()` | `cmd/kubernautagent/main.go` (`telemetry.Bootstrap`) | Manual/build verification |
| Kubernaut Agent inbound root span | `/api/v1` route group | `cmd/kubernautagent/routes.go` (`otelhttp.NewMiddleware("kubernautagent.http", ...)`) | `cmd/kubernautagent/otel_wiring_test.go` (UT-1519-012) + `test/integration/kubernautagent/server/otel_tls_wiring_integration_test.go` (**IT-1519-005**: real JWT-authenticated POST through a router mirroring main.go's real `/api/v1` route group — real `otelhttp` middleware, real rate limiter, real `auth.Middleware`/`JWTAuthenticator` verified against a real mock JWKS server, real `kaserver.NewHandler` — exports a span over TLS) |
| Kubernaut Agent outbound span (Data Storage client) | `buildDSBaseTransport` | `cmd/kubernautagent/datastorage.go` | Covered transitively by existing DS client tests + `otel_wiring_test.go` pattern |
| Kubernaut Agent outbound span (audit flush to DS) | `buildAuditStore` | `cmd/kubernautagent/datastorage.go` | Covered transitively by existing audit store tests |
| Kubernaut Agent outbound span (Prometheus tool) | `registerPrometheusTools` | `cmd/kubernautagent/toolregistry.go` | Covered transitively by existing Prometheus tool tests |
| Kubernaut Agent outbound span (LLM provider) | `buildTransportChain` | `cmd/kubernautagent/llm_builder.go` (`otelhttp.NewTransport` + `otelCloseAwareTransport` shim) | `cmd/kubernautagent/otel_wiring_test.go` (UT-1519-011), `cmd/kubernautagent/llm_builder_tls_test.go` |

**CHECKPOINT W**: every row above has a production caller in `cmd/` (no orphaned `pkg/` wiring code); the three `otelhttp.NewMiddleware` rows and the LLM transport row each have a dedicated UT proving the span/context wiring through an `httptest` request; the remaining outbound rows are `otelhttp.NewTransport` one-line wraps around already-tested client-construction functions, so no new dedicated span-assertion test was written for them individually — the wrap itself is a pure library call with no branching logic to fail. The shared `TracerProvider` bootstrap's TLS branch is the one row that gained real branching logic after this table was first written (`Config.TLS`/`TelemetryTLSConfig`); it is the only bootstrap row with both a UT (`buildTLSConfig` config-construction logic) and an IT (real TLS handshake + fail-closed rejection through the actual `otelhttp` middleware) — the no-TLS bootstrap calls remain build-verification-only per the original rationale, since `Endpoint == ""`/`WithInsecure()` has no branching logic to fail.

**Wiring-depth note (added after initial IT pass)**: the first IT pass (`otel_tls_wiring_integration_test.go` in `test/integration/shared/telemetry`) proved `NewTracerProvider`'s TLS behavior against a hand-constructed `otelhttp.NewMiddleware` call — real network encryption, but not proof that any specific service's actual router is wired correctly, since the middleware call was rebuilt inline rather than invoked through a service's `Handler()`. All three services now have that stronger per-service proof:
- **Gateway** (IT-1519-003): constructs the real `gateway.Server` via the same test helpers (`createGatewayConfig`, `createGatewayServer`) every other Gateway IT test uses, registers the real Prometheus adapter, and drives a real authenticated webhook POST through `gatewayServer.Handler()` — real auth, real scope filtering, real envtest CRD creation, real Data Storage audit emission.
- **Data Storage** (IT-1519-004): constructs the real `server.NewServer`/`Handler()` via the same pattern `context_propagation_test.go`/`batch_size_limit_test.go` use, and drives a real authenticated GET reading real seeded Postgres data via `repository.NewAuditEventsRepository`.
- **Kubernaut Agent** (IT-1519-005): assembles a router mirroring `main.go`'s `/api/v1` route group line-for-line (real `otelhttp.NewMiddleware`, real `HTTPMetricsMiddleware`, real `RateLimiter`, real `auth.Middleware`/`CompositeAuthenticator`/`JWTAuthenticator`), verified against a real mock JWKS server (same convention as the package's existing `jwt_middleware_test.go` — only the K8s SAR/TokenReview leg is test-doubled, consistent with mocking the Kubernetes API as an allowed external dependency), and drives a real JWT-signed POST through the real `kaserver.NewHandler`.

All three are the strongest wiring evidence in the manifest: each proves span export through the actual (or line-for-line mirrored, for KA) production dispatch path, not a reconstructed middleware call in isolation.

**Control-objective depth note (added after a compliance-focused review)**: proving "a span crosses TLS" is necessary but not sufficient evidence that the wiring satisfies the specific control objectives an auditor would check. Five gaps were identified and closed with additional IT coverage, all at the `test/integration/shared/telemetry` bootstrap level except the two that specifically required proof through a real service's dispatch path:
- **mTLS is real, not just constructed** (IT-1519-006/006b): UT-1519-009/010 already proved `buildTLSConfig` builds a `*tls.Config` with `Certificates` set when `CertFile`/`KeyFile` are configured — but a config field with no runtime effect would pass that UT trivially. IT-1519-006 proves a real mTLS handshake completes against a collector that mandates a client certificate; IT-1519-006b proves the same collector rejects a client presenting none, proving the requirement (and therefore the client cert) is doing real work.
- **The `SecurityProfile` floor holds on the wire, not just in memory** (IT-1519-007): UT-1519-011/012 prove `ApplyProfile` sets `MinVersion`/`CipherSuites` on the constructed `*tls.Config`. IT-1519-007 proves that construction actually constrains the negotiated connection: a `ModernProfile()` (TLS 1.3-only) client still negotiates TLS 1.3 against a collector willing to go as low as TLS 1.0, so the profile is enforced by the client, not merely offered.
- **Span payloads never leak the inbound bearer token** (IT-1519-008): `otelhttp`'s default inbound instrumentation does not capture headers, and no code in this repo adds a custom attribute extractor that would (confirmed by grep for `SetAttributes`/`otelhttp.With*` across the codebase) — true today, but previously unverified by any test. IT-1519-008 decodes the actual OTLP protobuf body delivered to the collector and asserts a real `Authorization: Bearer <token>` sent on the inbound request never appears in it, turning an implicit property into a regression-guarded one. This is an AU-3-adjacent "telemetry content must not itself become a secret-disclosure channel" control, distinct from SC-8 (transport only).
- **Fail-closed holds through the real dispatch path, not just a reconstructed middleware call** (IT-1519-009): IT-1519-002 proved `NewTracerProvider` fails closed against an untrusted collector, but only through the hand-wired `otelhttp.NewMiddleware` call, not through any service's actual `Handler()`. IT-1519-009 re-runs the same fail-closed scenario through Gateway's real production `Handler()` — a full webhook request (auth, CRD creation, audit) still succeeds, and the untrusted collector still receives zero spans.
- **An unresponsive collector cannot degrade the request path** (IT-1519-010): the async `BatchSpanProcessor` should decouple export from the response by construction, but that property was previously asserted nowhere. IT-1519-010 points Gateway's real `Handler()` at a collector that accepts TCP connections and never responds, and asserts a real webhook request still completes well within a bounded latency budget — proving enabling tracing cannot itself become an availability risk.

### Per-Service Differences

| Service | Inbound span | Outbound spans | Rationale |
|---|---|---|---|
| **Gateway** | Yes (`gateway.http`) | Data Storage audit client (`otelhttp.NewTransport`) | Entry point for all inbound signals; DS write is its only outbound HTTP dependency in the audit path |
| **Data Storage** | Yes (`datastorage.http`) | **None** | Every other service's outbound span terminates here as a child; DS's own dependencies (Postgres, Redis) are non-HTTP, so `otelhttp.NewTransport` does not apply without `otelsql`/redis hooks (out of scope) |
| **Kubernaut Agent** | Yes (`kubernautagent.http`) | Data Storage, Prometheus, LLM provider, audit flush (all `otelhttp.NewTransport`) | All of KA's outbound dependencies are HTTP; LLM call latency is the dominant cost of an investigation, hence unconditional wrapping even on the "no custom transport config" path |

### `otelCloseAwareTransport` Shim (Kubernaut Agent LLM Client)

`otelhttp.Transport` does not implement the optional `CloseIdleConnections()` method that KA's LLM hot-reload path (`llmRuntimeReloadCallback`) relies on to drain idle connections when the LLM runtime config changes. `cmd/kubernautagent/llm_builder.go` wraps it:

```go
type otelCloseAwareTransport struct {
    http.RoundTripper // otelhttp.NewTransport(base) -- handles RoundTrip
    base http.RoundTripper // pre-otel transport -- handles CloseIdleConnections
}

func (t *otelCloseAwareTransport) CloseIdleConnections() {
    if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
        closer.CloseIdleConnections()
    }
}
```

This preserves the hot-reload `Closer` contract without reaching into `otelhttp`'s internals.

### "Free" Instrumentation: `ogen`-Generated Clients

Discovered during the spike: Data Storage's `ogen-go/ogen`-generated OpenAPI client (used by both Gateway's audit client and Kubernaut Agent's DS/audit clients) bakes in its own OTel span per generated method call (e.g. `CreateAuditEventsBatch`), independent of and in addition to the `otelhttp.NewTransport` wrap around its base transport. A single outbound call therefore produces **two** nested spans (the `otelhttp` transport span, and the `ogen` client-method span) plus the receiving service's inbound root span — three spans for one logical hop. This is accounted for in `pkg/gateway/ds_transport_tracing_test.go`'s span-count assertions and required no additional wiring.

---

## 🔒 **Zero-Overhead Guarantee**

| State | Behavior |
|---|---|
| `endpoint == ""` and `logSink == false` (default) | OTel's global no-op `TracerProvider`/`Tracer` stays active. `otelhttp` middleware/transport call `tracer.Start()`, which returns a no-op span immediately — negligible allocation, no export, no goroutines, no network calls. |
| `logSink == true` only | One `logr.Logger` call per completed span, synchronously, in-process. No network dependency. |
| `endpoint != ""` | Adds a batched OTLP/HTTP exporter (background goroutine, periodic flush) — the only state with a genuine, expected runtime cost, and it is entirely opt-in. |

---

## 🧪 **Validation**

### Test Coverage

| Test | What it validates |
|---|---|
| `pkg/shared/telemetry/telemetry_test.go` | `NewTracerProvider` requires `ServiceName`; no-op behavior when both sinks disabled; `logExporter` produces correct `Info`/`Error` log lines |
| `pkg/shared/telemetry/tls_internal_test.go` | `buildTLSConfig`'s branching logic: disabled/enabled, empty vs. custom `CAFile`, unreadable `CAFile`, mTLS cert/key pairs, `SecurityProfile` (cipher suites, min TLS version) propagation |
| `test/integration/shared/telemetry/otel_tls_wiring_integration_test.go` (**IT-1519-001, IT-1519-002**) | Real network proof, not just config-construction: a span produced by the actual `otelhttp.NewMiddleware` call production servers use is delivered over a real TLS connection to a collector trusted via `CAFile` (IT-1519-001), and is **never** delivered when the collector's certificate isn't signed by that CA — fail-closed, not merely a flag with no runtime effect (IT-1519-002) |
| `test/integration/shared/telemetry/otel_tls_wiring_integration_test.go` (**IT-1519-006, IT-1519-006b**) | A real mTLS handshake completes and delivers a span when the collector requires a client certificate and one is configured via `CertFile`/`KeyFile` (006); the same collector delivers zero spans when no client certificate is configured (006b) — proves the mTLS requirement is actually enforced, not decorative config |
| `test/integration/shared/telemetry/otel_tls_wiring_integration_test.go` (**IT-1519-007**) | The process-wide `SecurityProfile`'s minimum TLS version is enforced on the actual negotiated connection: a `ModernProfile()` client negotiates TLS 1.3 against a collector willing to go as low as TLS 1.0 — proving the client enforces its own floor, not just the in-memory `*tls.Config` (already covered by UT-1519-011/012) |
| `test/integration/shared/telemetry/otel_tls_wiring_integration_test.go` (**IT-1519-008**) | Data-minimization control: the actual OTLP protobuf body delivered to the collector never contains the inbound request's `Authorization: Bearer <token>` header/value — decodes the real export request to confirm span data is present (not a vacuous pass) before asserting the token's absence |
| `pkg/shared/telemetry/tracelink_test.go` | `WriteTraceLinkAnnotation`/`ExtractTraceLink` round-trip (retained, unused in production — see Alternative C) |
| `pkg/gateway/otel_middleware_test.go` | Gateway's `otelhttp.NewMiddleware` wiring creates a root span visible to the inner handler |
| `test/integration/gateway/otel_tls_wiring_integration_test.go` (**IT-1519-003**) | Real production-dispatch proof: an authenticated webhook POST through the actual `gateway.Server.Handler()` (real auth, scope check, envtest CRD creation, DS audit) exports a span over a TLS-verified connection — the only test in the manifest exercising an unmodified service `Handler()`, not a reconstructed middleware call |
| `test/integration/gateway/otel_tls_wiring_integration_test.go` (**IT-1519-009**) | Fail-closed re-proven through the real dispatch path (not the hand-wired middleware call IT-1519-002 uses): a real webhook request through `gatewayServer.Handler()` still succeeds and the untrusted collector still receives zero spans |
| `test/integration/gateway/otel_tls_wiring_integration_test.go` (**IT-1519-010**) | Resilience/availability control: a real webhook request through `gatewayServer.Handler()` completes within a bounded latency budget even when the configured OTLP collector accepts connections but never responds — proves span export can never block or materially delay the business request path |
| `pkg/gateway/ds_transport_tracing_test.go` | Gateway's outbound DS audit transport creates a child span; accounts for the `ogen` client's own span (3 spans total, correct parent/child chain) |
| `pkg/datastorage/server/otel_middleware_test.go` (UT-1519-010) | Data Storage's inbound `otelhttp.NewMiddleware` wiring |
| `test/integration/datastorage/otel_tls_wiring_integration_test.go` (**IT-1519-004**) | Real production-dispatch proof: an authenticated GET through the actual `server.NewServer(...).Handler()` (real DD-AUTH-014 auth, real Postgres read via `repository.NewAuditEventsRepository`) exports a span over a TLS-verified connection |
| `cmd/kubernautagent/otel_wiring_test.go` (UT-1519-011, UT-1519-012) | KA's outbound LLM transport wrap carries an active span as parent; KA's inbound `/api/v1` middleware creates a root span |
| `cmd/kubernautagent/llm_builder_tls_test.go` | `buildTransportChain` always returns a non-nil, `otelhttp`-wrapped transport, even with no custom TLS/OAuth2/headers/circuit-breaker config |
| `test/integration/kubernautagent/server/otel_tls_wiring_integration_test.go` (**IT-1519-005**) | Real production-dispatch proof: a JWT-authenticated POST through a router mirroring `main.go`'s real `/api/v1` route group (real `otelhttp`, rate limiter, `auth.Middleware`/`JWTAuthenticator`, `kaserver.NewHandler`) exports a span over a TLS-verified connection |

### Build/Test Validation Performed

- `go build ./...` across all three services — clean.
- `go vet ./...` — clean.
- Full affected-package test suites (`pkg/shared/telemetry/...`, `pkg/gateway/...`, `pkg/datastorage/...`, `cmd/kubernautagent/...`) — all passing.

---

## 🔐 **FedRAMP Controls**

Each row is a control objective this implementation makes a specific, testable claim about — not a general "OTel is observability, therefore compliant" assertion. Distinct from `correlation_id`/DD-AUDIT-003's AU-2/AU-3 audit trail (line 74): these controls apply specifically to the *tracing side channel* Kubernaut added, not the pre-existing audit mechanism.

| Control | Claim | Test ID |
|---|---|---|
| SC-8 (Transmission Confidentiality and Integrity) | OTLP export to the collector is TLS-encrypted, and fails closed (delivers nothing) when the collector's certificate isn't signed by the configured CA — proven at the shared bootstrap level and, independently, through each service's real, unmodified production `Handler()` | IT-1519-001, IT-1519-002, IT-1519-003, IT-1519-004, IT-1519-005, IT-1519-009 |
| SC-8(1) / SC-13 (Cryptographic Protection, incl. mutual authentication) | Optional mTLS to the collector (`CertFile`/`KeyFile`) completes a real handshake and is actually enforced by the collector (not accepted as decorative config); the process-wide `SecurityProfile`'s minimum TLS version constrains the real negotiated connection, not just the in-memory `*tls.Config` | IT-1519-006, IT-1519-006b, IT-1519-007 |
| AU-3-adjacent (telemetry content must not become a secret-disclosure channel) | The exported span payload never contains the inbound request's `Authorization` header or bearer token value — decoded from the real OTLP protobuf body, not inferred from config | IT-1519-008 |
| *(no single control number — general availability hygiene)* | An unreachable/unresponsive OTLP collector cannot block or materially delay the business request path — enabling optional tracing must never itself introduce an availability risk | IT-1519-010 |

**Explicitly out of scope for this table**: audit-content controls (AU-2/AU-3) for the *audit trail itself* are Data Storage's/DD-AUDIT-003's responsibility via `correlation_id`, not this tracing side channel's (see Alternative D, line 74-80) — OTel spans are not submitted as evidence of "what happened," only "how long it took and where."

---

## 📁 **Implementation Files**

| File | Purpose |
|---|---|
| `pkg/shared/telemetry/telemetry.go` | `Config`, `NewTracerProvider`, `Shutdown`, `Bootstrap` — shared bootstrap for all services |
| `pkg/shared/telemetry/log_exporter.go` | `logExporter` — `sdktrace.SpanExporter` writing spans as structured log lines |
| `pkg/shared/telemetry/tracelink.go` | `WriteTraceLinkAnnotation`/`ExtractTraceLink` — retained, unused (Alternative C) |
| `internal/config/telemetry.go` | `TelemetryConfig`, `TelemetryTLSConfig`, `DefaultTelemetryConfig` — ADR-030 YAML config shared by all services |
| `pkg/shared/tls/tls.go` (`BuildClientTLSConfig`) | Reused (not duplicated) for the OTLP exporter's TLS `*tls.Config` — same CA-loading + `SecurityProfile` (cipher suites, min TLS version) application as every other Kubernaut TLS client |
| `test/integration/shared/telemetry/otel_tls_wiring_integration_test.go` | IT proof that the TLS branch of `NewTracerProvider` actually encrypts and validates, not just parses config: trusted-CA delivery and fail-closed rejection (IT-1519-001/002); real mTLS handshake and its fail-closed negative (IT-1519-006/006b); `SecurityProfile` minimum-version enforcement on the wire (IT-1519-007); span-payload data minimization (IT-1519-008) |
| `test/integration/gateway/otel_tls_wiring_integration_test.go` | IT proof that Gateway's real `Handler()` (not a reconstructed middleware call) exports a span over TLS for a real, fully-processed webhook request (IT-1519-003); the same fail-closed and resilience controls re-proven through that real dispatch path (IT-1519-009/010) |
| `cmd/gateway/main.go`, `pkg/gateway/config/config.go`, `pkg/gateway/server.go`, `pkg/gateway/server_constructors.go` | Gateway wiring (bootstrap, config, inbound middleware in `server.go`'s `setupRoutes()`, outbound DS transport in `server_constructors.go`'s `buildAuditStore`) |
| `cmd/datastorage/main.go`, `pkg/datastorage/config/config.go`, `pkg/datastorage/server/server_routes.go` | Data Storage wiring (bootstrap, config, inbound middleware only) |
| `test/integration/datastorage/otel_tls_wiring_integration_test.go` | IT proof that Data Storage's real `Handler()` exports a span over TLS for a real authenticated request reading real seeded data (IT-1519-004) |
| `cmd/kubernautagent/main.go`, `cmd/kubernautagent/routes.go`, `cmd/kubernautagent/datastorage.go`, `cmd/kubernautagent/toolregistry.go`, `cmd/kubernautagent/llm_builder.go`, `internal/kubernautagent/config/config_types.go` | Kubernaut Agent wiring (bootstrap in `main.go`, config in `config_types.go`, inbound middleware in `routes.go`, outbound DS/audit transports in `datastorage.go`, outbound Prometheus transport in `toolregistry.go`, outbound LLM transport + `otelCloseAwareTransport` in `llm_builder.go`) |
| `test/integration/kubernautagent/server/otel_tls_wiring_integration_test.go` | IT proof that a router mirroring KA's real `/api/v1` route group exports a span over TLS for a real JWT-authenticated request (IT-1519-005) |

---

## 📚 **Related Documents**

| Document | Relationship |
|---|---|
| [ADR-068](./ADR-068-opentelemetry-distributed-tracing-adoption.md) | Architecture-level decision this DD implements |
| [DD-AUDIT-003](./DD-AUDIT-003-service-audit-trace-requirements.md) | Defines `correlation_id`-based cross-service causal reconstruction — the mechanism OTel is designed to complement, not duplicate |
| [ADR-034](./ADR-034-unified-audit-table-design.md) | Unified audit table / event-sourcing design underlying `correlation_id` reconstruction |
| [ADR-030](./ADR-030-service-configuration-management.md) | YAML config convention followed by `TelemetryConfig` |
| [ADR-058](./ADR-058-webhook-driven-workflow-registration.md) | Originating decision for `RemediationWorkflow`'s validating-only wiring; already rejected a mutating-webhook alternative for reasons unrelated to tracing |
| [DD-005](./DD-005-OBSERVABILITY-STANDARDS.md) | Metrics/logging/request-ID standards; OTel tracing is a complementary fourth pillar |
| Issue #1519 (GAP-14) | Tracking issue, original audit finding and proposed (later re-scoped) implementation plan |

---

## ✅ **Acceptance Criteria**

1. Tracing is fully disabled (no-op, zero measurable overhead) unless an operator sets `telemetry.endpoint` and/or `telemetry.logSink` in a service's YAML config.
2. Gateway, Data Storage, and Kubernaut Agent each produce a root span per inbound HTTP request when tracing is enabled (verified by respective middleware tests).
3. Outbound HTTP calls from Gateway (to Data Storage) and Kubernaut Agent (to Data Storage, Prometheus, and the LLM provider) produce child spans correctly parented to the active span (verified by transport tests).
4. The log-sink exporter produces one structured, greppable log line per completed span, with `trace_id` present, requiring no collector infrastructure.
5. No cross-service span-link annotation mechanism is wired into production `RemediationRequest` creation or reconciliation (Alternative C removed); `correlation_id` remains the sole cross-service causal-reconstruction mechanism.
6. `go build ./...` and `go vet ./...` pass cleanly across all three services; all listed test files pass.

---

## 📊 **Confidence Assessment**

**Overall Confidence**: 90%

- **Design soundness**: 95% — pattern proven across three services with different outbound-dependency shapes (Gateway: one HTTP dep; Data Storage: zero, non-HTTP only; Kubernaut Agent: four HTTP deps), and validated by a real spike before committing to the final scope.
- **Zero-overhead claim**: 90% — backed by OTel's documented no-op `TracerProvider` behavior and this design's explicit avoidance of any allocation/branching when both sinks are disabled; not independently benchmarked under production load.
- **Scope-narrowing decision (dropping trace-link annotations)**: 90% — well-justified by the `correlation_id` precedent (ADR-034, DD-AUDIT-003) and explicit user cost/value sign-off; residual risk is that a future concrete use case (e.g. an APM vendor integration expecting a single unified trace) could reopen this, at which point the retained-but-unused `tracelink.go` mechanism is available to reactivate.
- **Coverage gap (CRD controllers not instrumented)**: acknowledged, not a confidence deduction — explicitly out of scope by design (ADR-068), not a defect.
