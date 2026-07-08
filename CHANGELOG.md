# Changelog

All notable changes to Kubernaut will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.5.3] - 2026-07-08

### Fixed

- **Remove execution.bundle pre-flight registry check (#1642)** — DataStorage no longer performs a registry existence check (OCI `HEAD`) on `execution.bundle` at workflow registration time. The check ran from the DataStorage pod's own network/credential context, which cannot validate self-signed or credential-required private registries reachable only by the actual workflow execution environment, unconditionally blocking valid registrations against such registries. Kubernetes now fails fast at Job/PipelineRun image-pull time instead (`ImagePullBackOff`/`ErrImagePull`), consistent with the runtime-observability pattern already used for other execution-time failures.

## [1.5.2] - 2026-06-24

### Added

- **OpenAI-compatible LLM adapter (#1487, BR-INTEGRATION-1254)** — In-house `model.LLM` adapter for OpenAI-compatible endpoints (LlamaStack, vLLM, Ollama, Azure OpenAI) using `net/http` with `http.Client` injection for mTLS transport chains. Supports streaming (SSE), tool call accumulation, finish reason mapping, and generation config forwarding. Factory-wired via `openai_compatible` provider in `NewModelFromConfig`.

### Changed

- **4-level severity model (#1484, ADR-066)** — Replaced `medium` with `warning` and `low` with `info` across all CRD enums, OpenAPI specs, rego policies, LLM prompt templates, mock-LLM handlers, golden transcripts, and test fixtures. Regenerated ogen clients and CRD manifests.

### Fixed

- **Cluster-scoped namespace strip (#1480, #1477)** — Dynamic scope resolution via RESTMapper for `ka_investigate_mcp`, self-healing namespace strip for cluster-scoped resources in AF, and scope-aware namespace resolution in Effectiveness Monitor target resource fetch.

### Security

- **CVE remediation in db-migrate (#1485)** — Pinned `x/crypto` and `x/net` in db-migrate image to resolve known CVEs.

### Dependencies

- Bump Go module dependencies (15 updates) via Dependabot (#1479)
- Bump `actions/checkout` from 6 to 7 (#1478)

## [1.5.1] - 2026-06-20

### Added

- **Kubernaut Console** — Web UI for interactive investigation, real-time agent chat, remediation monitoring, and workflow management ([kubernaut-console](https://github.com/jordigilh/kubernaut-console))

### Fixed

- **Stale session reactivation (#1472)** — `kubernaut_reconnect` now validates RR existence and phase before attempting reconnection. Prevents misleading "reconnecting" UX after pod restarts when the investigation no longer exists or has completed.
- **Per-phase LLM routing (#1470)** — Investigation phases can now route to different LLM providers via `phaseModels` config with hot-reload support.
- **Status/subscribe RR context (#1468)** — RR spec context now included in status/subscribe SSE metadata for richer UI state.
- **EA hot-loop prevention (#1466)** — GenerationChangedPredicate prevents unnecessary reconciliation in Effectiveness Monitor.
- **Audit trail truncation (#1464)** — Raised MaxToolResultBytes to 16KB and aggregates audit trail into lifecycle phases.
- **Session ID forwarding (#1452)** — AF-provided session ID properly threaded through to KA for direct lookup.

## [1.5.0] - 2026-06-10

### Added

#### MCP Interactive Mode (#703)

- **Interactive investigation sessions** — Kubernaut Agent exposes a Model Context Protocol (MCP) endpoint (`POST /api/v1/mcp`) enabling human-in-the-loop investigation of remediation requests. Operators can guide, question, and direct the agent in real-time.
- **Three MCP tools**: `kubernaut_investigate` (start/takeover/message/complete/cancel/status), `kubernaut_enrich` (K8s resource enrichment with impersonation), `kubernaut_select_workflow` (workflow catalog lookup and selection).
- **Lease-based session management** — Single-driver exclusivity via Kubernetes coordination/v1 Leases with automatic TTL expiry and inactivity timeout.
- **Dynamic takeover** — Operators can take over autonomous or stale interactive sessions, reconstructing conversation context from DataStorage audit events.
- **User impersonation** — All K8s API calls during interactive sessions execute under the operator's identity, enforcing their RBAC permissions.
- **Session notifications** — MCP log-level push notifications for inactivity warnings and session events via `InMemoryNotificationBus`.
- **Per-user rate limiting** — Token-bucket rate limiter keyed by authenticated username with configurable requests-per-second.
- **Prometheus metrics** — 4 interactive metrics: `aiagent_mcp_interactive_sessions_active`, `aiagent_mcp_interactive_command_duration_seconds`, `aiagent_mcp_interactive_takeover_total`, `aiagent_mcp_interactive_lease_contention_total`.
- **Observer status endpoint** — Read-only `action=status` for checking investigation mode without Lease acquisition.
- **Helm integration** — Feature-gated via `kubernautAgent.interactive.enabled`; auto-provisions Lease Role/RoleBinding and impersonate ClusterRole.

#### Session Cancellation and SSE Streaming (#823)

- **Session cancellation infrastructure** — `StatusCancelled` terminal state, `CancelInvestigation()` with context propagation, `Subscribe()` with lazy event channel.
- **SOC2 CC8.1 audit trail** — 22 `aiagent.*` event types emitted fire-and-forget for session lifecycle, investigation cancellation, and alignment events.
- **SSE streaming** — Token-level streaming via `StreamChat` on `llm.Client` interface, delivered through `LazySink` + `io.Pipe` SSE pipeline. Autonomous mode unchanged (no sink = no streaming).
- **Object-level session authorization** — Session ownership tracking with `created_by` identity; cross-user access returns 404.
- **Operational hardening** — Per-IP rate limiter, `Manager.Shutdown()` graceful cancellation, panic recovery in investigation goroutines, error sanitization at handler boundaries.

#### Configuration 3-Domain Restructure

- **Runtime/AI/Integrations** top-level domains with camelCase YAML tags across all config fields.
- **Interactive top-level config** — `InteractiveConfig` struct with `sessionTTL`, `inactivityTimeout`, `maxConcurrentSessions`, `rateLimitPerUser`, `maxAnalyzingTimeout`.
- **RO config hot-reload** — `FileWatcher`-based live reload of Remediation Orchestrator configuration (#835).

#### Alignment Check (Shadow Agent)

- **Shadow agent evaluator** — Parallel evaluation of investigation quality via alignment check with configurable `mode` (shadow/enforce) and `verdictTimeout`.
- **Canary force-escalation** — `canary.forceEscalation` flag for testing alignment enforcement paths.
- **Full-context grounding review for shadow agent** (#1096) — Added a second evaluation layer that reviews the entire RCA conversation through the shadow LLM in a single call, triggered at the RCA-to-workflow-discovery boundary. Detects distributed prompt injection that per-step isolation cannot catch (boiling frog attacks). Runs in parallel with workflow discovery for zero added latency. New `EvaluateGrounding` method on Evaluator, `StartGroundingReview`/`NotifyRCAComplete` integration points, `GroundingReview` config section, 2 new Prometheus metrics (`kubernaut_alignment_grounding_total`, `kubernaut_alignment_grounding_duration_seconds`), and 2 new audit event types (`aiagent.alignment.grounding.request`, `aiagent.alignment.grounding.response`). Gated behind `ai.alignmentCheck.groundingReview.enabled: true`. 29 unit tests covering observer integration, evaluator, config, metrics, and concurrency.
- **Shadow agent alignment verdict schema** (#1076) — New `alignment_verdict` field on KA `IncidentResponse` (OpenAPI) and AA `AIAnalysisStatus` (CRD). Carries shadow agent verdict (`result`, `circuit_breaker_activated`, `summary`, `findings`) for ALL investigations. Enables structured reporting of shadow agent findings alongside primary LLM results.
- **Circuit breaker for shadow agent enforcement** (#1076) — When shadow agent detects suspicious LLM content in enforce mode, the primary investigation is cancelled via `context.WithCancelCause(ErrCircuitBreaker)`. Shadow evaluations continue on parent context. New `alignmentCircuitBreakerTotal` Prometheus counter.
- **LLMProxy bypass fix** (C-1) — `PinDecorator` on investigator ensures `LLMProxy` is re-applied around pinned `SwappableClient` snapshots, preventing unmonitored LLM traffic.
- **RO alignment verdict notifications** (#1076) — Manual review notifications now render shadow agent findings prominently before (relegated) primary LLM RCA. Circuit breaker verdicts show `SUSPICIOUS (Circuit Breaker Activated)`. `alignment_check_failed` SubReason escalates to `NotificationPriorityCritical`.
- **`ReviewContext` CRD fields** — `alignmentVerdict` and `circuitBreakerActivated` fields on NotificationRequest ReviewContext for routing rule support.

#### Session Lifecycle (#1078)

- **Session panic recovery** — Investigation goroutines now recover from panics, log stack traces, and transition to `StatusFailed`.
- **Two-tier session TTL eviction** — Terminal sessions (`Completed`/`Failed`) evicted after `ttl`; non-terminal (`Pending`/`Running`) after configurable `maxSessionAge` (default `2×ttl`).
- **AA investigation timeout** — Wall-clock cap (`DefaultMaxInvestigationDuration = 25min`) prevents unbounded investigation sessions. Transitions to `PhaseFailed` with `Reason=TransientError`.

#### Logging Migration (#885, #1274)

- **slog-to-logr migration** — Kubernaut Agent migrated from `log/slog` to `logr.Logger` for consistency with controller-runtime services. All internal packages, tests, and wiring updated.
- **AF slog-to-logr migration** (#1274) — API Frontend migrated from `slog.Default()` to injected `logr.Logger` across all production code (TTL reconciler, session service, A2A launcher, FileWatcher, ka_stream tool). Ensures all AF log output flows through the unified `zapr`-backed audit pipeline (FedRAMP AU-3).

#### AF Session Controller Hardening (#1272, #1273)

- **Graceful degradation** (#1272) — Session health flag (`atomic.Bool`) gated on informer cache sync; `/readyz` returns 503 until cache syncs. `af_session_ttl_actions_total` Prometheus counter for TTL cancel/delete actions (FedRAMP SI-4, SI-4(2)).
- **Pre-flight diagnostics** (#1273) — CRD discovery and RBAC SSAR checks (all 6 verbs) logged at startup with structured JSON. `session controller manager started` log entry confirms controller is operational (FedRAMP SI-10).
- **FedRAMP GA hardening** — Removed over-provisioned `patch` verb from RBAC (AC-6), enforced RetentionTTL ≥ 30d floor (AU-11), restricted ports to non-privileged range ≥1024 (SC-4), added audit emission for config hot-reload events (CM-3), seeded `apifrontend` in DB audit retention policies.

#### Observability

- **pprof runtime profiling** — `/debug/pprof/*` endpoints registered on the shared health server (DataStorage, KA, Gateway). Enabled by default following `kube-apiserver --profiling` pattern; gated via `disableProfiling` config field for hardened environments. Zero overhead when not actively queried.
- **Workflow validation duration metric** — New `datastorage_workflow_validation_duration_seconds` Prometheus histogram with `phase` and `result` labels for per-phase observability.

#### Data Storage (PROD-L1)

- **Helm autoscaler & PDB** — Horizontal Pod Autoscaler hooks and PodDisruptionBudget manifests for Data Storage workloads.
- **Hash chain verification** — `POST /api/v1/audit/verify-chain` for tamper-evidence validation of chained audit hashes.
- **Signed audit export** — `GET /api/v1/audit/export` for cryptographically packaged audit exports consumed by downstream compliance tooling.
- **Batch FK parent lookups** — Batched lookups for parent event dates tighten composite FK integrity validation under load (batch write pathway).
- **Workflow status transition validation** — Declarative state machine enforcement on workflow transitions before commits.

#### Documentation & Standards

- **ADR-060** — Architecture decision record documenting the parallel validation patterns and error priority contract.
- **RFC 7807 type constraint** (UX-1) — OpenAPI `RFC7807Problem.type` field now uses a `pattern` constraint (`^https://kubernaut\.ai/problems/.+`) with documented common types across all Data Storage API specs.
- **Concurrency guidelines** (DX-5) — Added concurrency patterns section to project guidelines documenting `errgroup`, typed-result-slot, and timeout budget patterns.
- **DD-WE-006 v2.2** (DOC-4) — Added changelog entry noting dependency validation parallelization per Issue #1070.
- **CONTRIBUTING.md Go version** (DX-4) — Updated prerequisite Go version from 1.25.3+ to 1.25.6+ to match `go.mod`.
- **DD-008 v1.1** — Updated DLQ drain design doc: `terminationGracePeriodSeconds` 30s→90s, added last-reviewed date.
- **Migration playbook** — Added type migration playbook to `migrations/README.md` documenting the column-type change workflow.
- **Migration 011** — `011_timestamp_timezone_alignment.sql` aligns `legal_hold_placed_at` from `TIMESTAMP` to `TIMESTAMP WITH TIME ZONE`.

### Changed

- **VERSION** bumped to 1.5.0 with propagation to Chart.yaml, Dockerfiles, and airgap templates via `make sync-version`.
- **golangci-lint CI gate** — Lint step is now blocking (removed `continue-on-error`). New issues must be clean before merge.
- **Verdict label rename** (#1077) — `VerdictClean` constant changed from `"clean"` to `"aligned"` for OpenAPI/API consistency. **Breaking**: Prometheus `result` label changes from `result="clean"` to `result="aligned"` — update dashboard queries.
- **Parallelized workflow validation** (#1070) — External validation checks (action-type taxonomy, OCI bundle existence, K8s dependency validation) now run concurrently during workflow registration, reducing registration latency from sum-of-three to max-of-three backend calls. Error priority contract preserved via typed-result-slot pattern (ADR-060).
- **Concurrency cap on dependency validation** (#1070) — `ValidateDependencies` now limits concurrent K8s API calls to 10 via `errgroup.SetLimit`, preventing API server overload from schemas with many dependencies.
- **Validation timeout budget** (#1070) — `validateExternalChecks` enforces a 10-second timeout to prevent degraded backends from consuming the full server WriteTimeout.
- **Data Storage pod termination grace** — `terminationGracePeriodSeconds` increased **30 → 90s** so shutdown can complete DLQ/admission/retry windows before kubelet SIGKILL under load.
- **Data Storage CORS default** — Moved from permissive wildcard to **deny-by-default** (empty allowed origin list emits no wildcard `Access-Control-Allow-Origin`; explicit opt-in origins required).
- **Audit retention default** — Data Storage retention cron now ships **enabled by default** (previously opted-in/out per environment)—override via Helm/`ConfigMap` for shorter dev cycles.

### Security

- **MCP impersonation hardening** (#895, #896) — Hardened impersonation flow to eliminate double-auth and prevent header injection in interactive sessions.

- **Empty username rejection** — `Takeover` rejects sessions with empty driver identity (SEC-01).
- **Max concurrent sessions** — Atomic counter enforcement per agent instance (SEC-03).
- **Session TTL and inactivity timeout** — Checked on every `GetDriver` call (SEC-04).
- **Per-user rate limiting middleware** — 401 for unauthenticated, 429 with `Retry-After` for rate-exceeded (SEC-02).
- **Impersonate-* header stripping** — Client-supplied impersonation headers stripped before processing.
- **Explicit QPS/Burst/Timeout** on MCP controller-runtime client (SEC-07).
- **Constant-time session ownership comparison** — Mitigates timing attacks on session authorization.
- **Data Storage security auditing** — Structured security audit logs for **`401`**/**`403`** authz failures (**FedRAMP AU-2** alignment).
- **Data Storage request containment** — `MaxBytesReaderMiddleware` rejects oversized payloads before OpenAPI/auth work (**SC‑5**) alongside hardened CORS defaults.
- **Problem Details on panic paths** — API panic middleware now terminates with RFC **7807** JSON instead of bubbling raw panics to clients.

### Helm

- **Interactive mode ConfigMap** — `interactive:` block emitted when `kubernautAgent.interactive.enabled=true`.
- **coordination/v1 Leases RBAC** — Namespace-scoped Role/RoleBinding (least privilege).
- **Impersonate verb** — Cluster-wide impersonate for users/groups/serviceaccounts (gated by `interactive.enabled`).
- **values.schema.json** — Strict validation for interactive configuration fields.

### Fixed

- **ParentEventDate propagation** — Guaranteed consistent parent-event linkage for composite foreign keys when batch-writing nested audit payloads.
- **DLQ handler hygiene** — Sanitized DLQ error surfaces so Postgres/SQL engine details cannot leak upstream through retry responses/logs.
- **CORS regression** — Empty allow-lists correctly reject cross-origin callers (no accidental allow-all serialization).
- **Data races** — Fixed 2 races in `event_store_test.go` (buffered channels replacing bare variables) and 1 race in `session/manager.go` (shallow field reference used outside lock).
- **errcheck lint violations** — 12 fixes across wiring, timeout, security integration tests and `vertexanthropic/client.go`.
- **Duplicate declarations** — Removed duplicate `EventTypeTokenDelta` and `chatOrStream` from rebase.
- **nil context guard** — `GetUserFromContext` returns "" safely on nil context.
- **100 Go Mistakes audit** — Systematic remediation of 28 issues across Critical/High/Medium/Low tiers:
  - Critical: `rows.Err()` checks after SQL iteration, `errors.Is` for sentinel comparisons.
  - High: HTTP client/server timeouts (LLM 5min, DS 30s, health 10s), `sync.Map` eviction for `mcpToSession`, `strings.Clone` for substring memory leaks, `errors.Is` for Redis sentinel.
  - Medium: `errors.As` for wrapped type assertions, epsilon comparison for floats, deterministic map iteration via `slices.Sort`, variable shadowing fix, pointer-based range loops for large K8s structs.
  - Low: Modern `0o` octal literals, rune-aware UTF-8 truncation, godoc on exported sentinels.
  - Tooling: `ineffassign` linter enabled in `.golangci.yml`.
- **CI stability** — UUID-based namespace naming to prevent parallel test collisions, E2E RR fixture provisioning for HARM-004 tests, fullpipeline timeout alignment.
- **Double logging** — Removed redundant `logger.Error` calls before `fmt.Errorf(... %w)` returns in `reconstruct.go` and `llm_builder.go`.
- **RR existence check** (HARM-004) — `RRExistenceChecker` interface prevents Lease creation for non-existent RemediationRequests.
- **Shadow agent false positive on standard K8s/OCP metadata** (#1094) — Refined shadow agent evaluation prompt to reduce false positives on standard Kubernetes and OpenShift metadata. Narrowed classification rule #4 to target imperative agent-manipulation intent rather than incidental keyword matches. Added explicit CLEAN whitelist for well-known annotation namespaces (`kubernetes.io/*`, `kubectl.kubernetes.io/*`, `openshift.io/*`, etc.), container spec commands, probe exec commands, K8s event lifecycle messages, RBAC verbs, and registry URLs. Added 3 new CLEAN few-shot examples (OCP Secret metadata, container commands/probes, K8s events) and 6 new test payloads (5 CLEAN + 1 adversarial SUSPICIOUS).
- **Inconclusive RR flood prevention** (#1091) — `Inconclusive` outcomes (EA confirms alert still firing, `alertScore=0`) now trigger exponential backoff and 3-strikes blocking. `completeVerificationIfNeeded` increments `ConsecutiveFailureCount` and sets `NextAllowedExecution`. `CheckConsecutiveFailures` counts `Completed+Inconclusive` as a functional failure instead of a chain-breaker. Prevents 30+ RR flood for persistent alerts. **BR-ORCH-042.6 updated**.
- **Request body size limit** — `HandleCreateWorkflow` now caps request body at 2 MiB via `http.MaxBytesReader` to prevent memory exhaustion from oversized payloads.
- **Deployment manifest probe paths** — Fixed `deploy/data-storage/deployment.yaml` liveness and readiness probes from `/health` to `/healthz` and `/readyz` to match the health server implementation.
- **OpenAPI domain mismatch** (UX-2) — Fixed `kubernaut.io` → `kubernaut.ai` in RFC 7807 problem type URIs across all OpenAPI specs (5 files). Domain now matches the URIs emitted by Go code.
- **Copyright year** (COMPAT-3) — Updated copyright headers from 2025 to 2026 in test files modified by this PR.

## [1.4.0] - 2026-05-12

### Added

#### Prompt Injection Defense — Shadow Agent (#601, #1076, #1096)

- **Shadow agent alignment check** (#601) — Fail-closed shadow agent evaluates every LLM tool output for prompt injection using random boundary markers, head+tail truncation, and data exfiltration detection. 82 unit tests, E2E tests with poisoned ConfigMap injection.
- **Full-context grounding review** (#1096) — Second evaluation layer reviews the entire RCA conversation through the shadow LLM at the RCA-to-workflow boundary. Detects distributed injection (boiling frog attacks) that per-step isolation cannot catch. Runs in parallel with workflow discovery for zero added latency. Fail-closed design with `hasDuplicateGroundedKey` pre-scan for duplicate JSON key attacks. 2 new Prometheus metrics (`kubernaut_alignment_grounding_total`, `kubernaut_alignment_grounding_duration_seconds`) and 2 new audit events.
- **Alignment verdict schema** (#1076) — New `alignment_verdict` field on KA `IncidentResponse` (OpenAPI) and AA `AIAnalysisStatus` (CRD) carrying shadow agent verdict (`result`, `circuit_breaker_activated`, `summary`, `findings`). `alignmentVerdict` and `circuitBreakerActivated` fields added to NotificationRequest `ReviewContext` for routing rule support.
- **Circuit breaker enforcement** (#1076) — When shadow agent detects suspicious content in enforce mode, the primary investigation is cancelled via `context.WithCancelCause(ErrCircuitBreaker)`. New `alignmentCircuitBreakerTotal` Prometheus counter.
- **RO alignment verdict notifications** (#1076) — Manual review notifications render shadow agent findings prominently. `alignment_check_failed` SubReason escalates to `NotificationPriorityCritical`.
- **Shadow agent LLM token audit events** (#1059) — Per-step audit trail with shadow LLM request/response payloads and token counts for cost tracking.
- **RCA completion audit event** (#847) — `aiagent.rca.complete` audit event with causal chain and due diligence propagation from Phase 1 to final result. Prometheus `match[]` filter added.
- **Mock-LLM tool call scenarios** (#657) — Per-scenario `ForceText` override, `ToolCallOverride` for custom tool call bypass, `injection_configmap_read` scenario, and `CreatePoisonedConfigMap` E2E fixture for security testing.

#### Notification Channels (#60, #593)

- **PagerDuty delivery channel** (#60) — Events API v2 delivery adapter with circuit breaker, hot-reload routing integration, and configurable URL override for E2E testability.
- **Microsoft Teams delivery channel** (#593) — Adaptive Card delivery adapter with circuit breaker and hot-reload routing integration.
- **Generic circuit breaker for delivery channels** (#60, #593) — Unified circuit breaker pattern replaces per-channel implementations.

#### Orchestrator Enhancements

- **RAR operator workflow overrides** (#594) — Operators can override AIA-selected workflows via RAR status. Validated by authwebhook (verifies override RW exists and is Active), merged by RO (`ResolveWorkflow` merge logic — RAR override takes precedence over AIA).
- **RO Phase Handler Registry** (#666) — Refactored the 2.5k-line monolithic RO reconciler into 7 modular phase handlers (Pending, Processing, Executing, Verifying, Blocked, Analyzing, AwaitingApproval) dispatched via a `PhaseHandlerRegistry`. Removed ~705 lines of dead legacy code. ADR-062 documents the architectural decision.
- **Dry-run mode** (#712, #736) — When `dryRun` is enabled in RO config, the pipeline stops after AI analysis — no WFE, RAR, or EA CRDs are created. RemediationRequest completes with outcome `DryRun`. `NextAllowedExecution` set for Gateway dedup suppression.
- **Execution-time dedup classification** (#190) — RO dedup handler classifies execution-time resource collisions as `Deduplicated`. Outcome aligned with CRD enum and OpenAPI spec.
- **DuplicateInProgress outcome inheritance** (#614) — Generalized inherited transitions so DuplicateInProgress outcomes inherit from the original WFE instead of re-running the pipeline.
- **CRD TTL enforcement** (#265) — 24h retention TTL on terminal RemediationRequests with `RetentionConfig` and Helm-configurable `retention.period`.

#### Kubernaut Agent

- **Parallel tool execution** (#970) — LLM loop executes multiple tool calls concurrently when the LLM returns batched tool requests.
- **Tool call batching directive** (#971) — Investigation prompt instructs the LLM to batch independent tool calls for reduced round-trips.
- **apiVersion validation gate** (#1044) — Detects ambiguous CRD Kinds (multiple API groups for same Kind), triggers human review on gate exhaustion. Prevents incorrect `kubectl` operations against wrong API group.
- **Signal annotations forwarding** (#462) — `RR.spec.signalAnnotations` forwarded through KA handler, prompt builder, and investigation template. Anti-confirmation-bias guardrail added to investigation prompt.
- **DetectedLabels wiring** (#1052) — DetectedLabels from enrichment wired to DS catalog queries. Unified into single canonical type in `pkg/shared/types`.
- **SA token refresh and audit auth handling** (#1055, #1056) — Custom token path constructor with 401 cache invalidation for SA token refresh. Audit 401/403 reclassified as retryable auth errors. `TokenSource` extracted for shared token cache across all callers.
- **OAuth2 client credentials transport** (#417) — Support for enterprise LLM gateways requiring OAuth2 token acquisition with configurable custom authentication headers.
- **CRD-aware engine registration** (#868) — Engine registration validates CRD availability; enters degraded status when required CRDs are missing.
- **Session hardening** (#1078) — Panic recovery in investigation goroutines, two-tier TTL eviction (terminal after `ttl`, non-terminal after `maxSessionAge`), and 25-minute wall-clock investigation timeout.
- **LLMProxy bypass fix** (C-1) — `PinDecorator` ensures `LLMProxy` is re-applied around pinned `SwappableClient` snapshots, preventing unmonitored LLM traffic.
- **Authenticated audit actor** (#998) — Propagates the authenticated user identity into all audit events for SOC2 attribution.
- **LLM and DS circuit breaker** (OPS-2) — Circuit breaker wrapping LLM and DataStorage HTTP clients for graceful degradation under downstream failures.

#### Gateway

- **Security hardening** (#673) — 14-finding security audit remediation: 256KB body limits via `MaxBytesReader`, generic error responses (RFC 7807), `X-Auth-Request-User` header stripping, RBAC least-privilege, per-handler K8s API timeout (15s), trusted proxy RealIP middleware (fail-closed), CORS restrictive default, image tag pinning.
- **Dynamic owner resolution** (#1029, #1032) — Dynamic API resource registry with existence validation. Batch-independent alert processing with FedRAMP readiness remediation.
- **Prometheus reserved label denylist** (#1045, #1067) — `namespace` and Prometheus-reserved labels excluded from dynamic kind resolution to prevent misrouting.

#### Orchestrator Resilience

- **RO config hot-reload** (#835) — FileWatcher-based hot-reload for RO ConfigMap fields. Thread-safe config access via `ReloadCallback` eliminates pod restarts for runtime tuning.
- **Cache sync readiness gating** (#852, #853) — Controller readiness probes now gate on informer cache sync completion. HTTP retry transport with exponential backoff for transient DataStorage failures.

#### Infrastructure

- **Inter-service TLS with security profiles** (#748) — TLS wired between all 10 services (Gateway, DataStorage, KA, RO, WE, EM, NT, SP, AuthWebhook, Operator). Configurable `tlsProfile` field selects built-in cipher/protocol profiles (Modern, Intermediate, Old). ADR-TLS-001 documents the design.
- **SBOM and license scan** (COMP-1) — `go-licenses` SBOM and license compliance scan added to CI pipeline.
- **NetworkPolicy templates** (#285) — 12 NetworkPolicy templates for all Kubernaut services with default-deny posture, configurable CIDRs, and per-service toggle.
- **FileWatcher routing hot-reload** (#244) — Notification routing ConfigMap informer replaced with FileWatcher. `SLACK_WEBHOOK_URL` environment variable dependency removed.
- **Defense-in-depth parameter filtering** (#243) — WE controller filters workflow parameters against the workflow schema with consolidated DataStorage calls.
- **Unified monitoring config** (#463) — Prometheus and AlertManager configuration unified into a single `monitoring` block for EM and KA.
- **CRD-to-OpenAPI enum drift detection** (#838) — Automated detection of enum value mismatches between CRD Go types and OpenAPI specs.
- **Workflow validation duration metric** — New `datastorage_workflow_validation_duration_seconds` Prometheus histogram with `phase` and `result` labels.
- **ADR-060** — Architecture decision record documenting parallel validation patterns and error priority contract.
- **`-race` detector enforcement** (#1073) — All E2E test targets now run with Go's race detector enabled.

### Changed

- **KA config camelCase migration** (#908) — All KA YAML config fields migrated from `snake_case` to `camelCase` per ADR-030. **Breaking**: existing KA ConfigMaps must be updated.
- **KA config restructured into 3 domains** (#908) — Config reorganized into `runtime`, `ai`, and `integrations` top-level domains (`server` nested under `runtime`; `tools` nested under `integrations`). **Breaking**: config field paths have changed.
- **KA config split** (#916) — KA config split into static ConfigMap and hot-reloadable ConfigMap. Runtime changes to AI/tool settings take effect without pod restart.
- **Verdict label rename** (#1077) — `VerdictClean` changed from `"clean"` to `"aligned"` for API consistency. **Breaking**: Prometheus `result` label changes from `result="clean"` to `result="aligned"`.
- **Standardized log levels** (#875) — Log level configuration standardized across all services with consistent YAML key naming.
- **logr logging standard** (#935) — KA migrated to `go-logr/logr` per DD-005 v2.0.
- **Parallelized workflow validation** (#1070) — External validation checks run concurrently during workflow registration. Concurrency capped at 10 via `errgroup.SetLimit` with 10-second timeout budget.
- **gobreaker v1 to v2 migration** (#1087) — `github.com/sony/gobreaker` upgraded from v1.0.0 to v2.4.0. Generic type parameters eliminate unsafe `interface{}` type assertions. `ManagerConfig` API encapsulates gobreaker so consumers never import it directly.
- **Generic delivery timeout** (#60, #593) — `SlackTimeout` renamed to `DeliveryTimeout` for channel-agnostic configuration.

### Security

- **MCP impersonation hardening** (#895, #896) — Hardened impersonation flow to eliminate double-auth and prevent header injection in interactive sessions.

### Fixed

- **Shadow agent false positives on K8s/OCP metadata** (#1094) — Narrowed classification rule #4 to target imperative agent-manipulation intent. Added CLEAN whitelist for well-known K8s/OCP annotation namespaces, container commands, probe commands, event messages, RBAC verbs, and registry URLs.
- **Shadow agent evaluates raw tool output** (#1101) — Moved `SubmitToolStep` from post-summarizer to post-sanitizer so the shadow agent evaluates raw external data, not LLM-generated directive language from the summarizer. Eliminates false positives from summarized analysis content.
- **Inconclusive RR flood prevention** (#1091) — `Inconclusive` outcomes now trigger exponential backoff and 3-strikes blocking, preventing 30+ RR flood for persistent alerts.
- **CompletedAt on PhaseSkipped** (#612) — Skip handlers (`ResourceBusy`, `RecentlyRemediated`) now set `CompletedAt` when transitioning to `PhaseSkipped`.
- **Enrichment NotFound exemption** (#1039) — `NotFound` errors exempt from `HardFail` for deleted resources. Deleted-resource warning surfaced and propagated to workflow result.
- **apiVersion propagation** (#1040) — `apiVersion` propagated through the full remediation target pipeline (schema → API → CRD).
- **SignalToPrompt label override** (#1061) — Signal prompt now prefers `target_resource` labels over enrichment labels.
- **Multi-group/multi-version kind resolution** (#1062, #1064) — `K8sAdapter` tries all API groups for ambiguous kinds. `RESTMappings` fallback for multi-version resolution.
- **Prometheus reserved label `namespace`** (#1067) — `namespace` added to reserved label denylist to prevent gateway misrouting.
- **Audit data quality** (#1033) — Outcome vocabulary normalized; `workflow_name` added to audit events.
- **Shadow agent Vertex AI provider** (#922) — Shadow agent uses `buildLLMClientFromConfig` for Vertex AI compatibility.
- **Markdown fences in shadow responses** (#925) — Markdown code fences stripped from shadow agent evaluator responses before JSON parsing.
- **Target-workflow alignment gate** (#934) — Phase 3 validates workflow Component scope against RCA remediation target kind.
- **tool_result always set** (#929) — `LLMToolCallPayload` always includes `tool_result` field, preventing OpenAPI validation failures on empty tool output.
- **Request body size limit** — `HandleCreateWorkflow` caps request body at 2 MiB via `MaxBytesReader`.
- **Flaky UT-GAP2-001 test** (#1098) — Eliminated race condition in wrapper gap test by providing clean response for signal step and using monitor mode.
- **Stale HAPI references in CRD docs** (#1103) — Renamed 32 stale `HAPI` → `Kubernaut Agent`/`KA` references in Go source comments across 5 `api/` type files. Regenerated `docs/generated/crds.md`.
- **OpenAPI domain mismatch** — Fixed `kubernaut.io` → `kubernaut.ai` in RFC 7807 problem type URIs across all OpenAPI specs.
- **Deployment manifest probe paths** — Fixed liveness/readiness probes from `/health` to `/healthz` and `/readyz`.

### Removed

- **Conversation API** (#867) — Conversational mode for Kubernaut Agent (#592) removed from v1.4; deferred to v1.5 (`development/v1.5` branch).

### Upgrade Notes

- **Breaking: KA config camelCase** (#908, ADR-030) — All KA YAML config fields migrated from `snake_case` to `camelCase`. Update your KA ConfigMap before upgrading.
- **Breaking: KA config restructured** (#908) — Config reorganized into `runtime`, `ai`, and `integrations` top-level domains (e.g., `llm_provider` → `ai.llmProvider`, `server` is now under `runtime`, `tools` under `integrations`).
- **Breaking: KA config split** (#916) — KA now reads from two ConfigMaps: a static one (mounted at startup) and a hot-reloadable one (watched at runtime). Update Helm values accordingly.
- **Breaking: Prometheus verdict label** (#1077) — Shadow agent Prometheus metric `result` label changed from `"clean"` to `"aligned"`. Update dashboard queries and alerting rules.
- **Database migrations required** — Run v1.4 migrations before upgrading controllers. The Helm pre-upgrade hook handles this automatically.
- **NetworkPolicy** (#285) — NetworkPolicies are now deployed for all services by default with a default-deny posture. Verify your cluster's CNI supports NetworkPolicy enforcement. Disable per-service with `networkPolicies.<service>.enabled: false`.

[1.4.0]: https://github.com/jordigilh/kubernaut/compare/v1.3.2...v1.4.0
## [1.2.0] - 2026-04-06

### Added

#### CRD Schema Hardening (#453–#459, #483)

- **Typed enums for all CRD status and reason fields** — Replaced raw strings with typed Go enums across all 9 CRDs: `SkipReason`, `BlockReason`, `FailurePhase` (RemediationRequest), `AnalysisType`, `Reason`, `PolicyDecision` (AIAnalysis), `Environment`, `Priority` (SignalProcessing), `ExecutionStatus` as `ConditionStatus` (WorkflowExecution), `Criticality`, `SLARequirement` (shared BusinessClassification), `CatalogStatus` (RemediationWorkflow, ActionType). Provides compile-time safety and OpenAPI validation.
- **Duration fields migrated to `metav1.Duration`** (#455) — WorkflowExecution duration strings replaced with structured `metav1.Duration` types.
- **Wide printer columns for RemediationRequest** (#387) — `kubectl get remediationrequests` now shows phase, outcome, severity, target, and age in wide format.
- **OAS catalog status alignment** (#483) — DataStorage OpenAPI enum values aligned with PascalCase CRD convention.

#### Per-Workflow ServiceAccount (DD-WE-005 v2.0, #481)

- **End-to-end SA propagation** — `serviceAccountName` field added to RemediationWorkflow, ActionType, AIAnalysis, RemediationRequest, and WorkflowExecution CRDs. Propagated through HAPI validation, RO controller, and WE executors.
- **Executor SA injection** — Ansible and Job executors use per-workflow SA instead of the hardcoded default, enabling least-privilege RBAC per remediation workflow.
- **DataStorage SA persistence** — Workflow catalog stores and returns `serviceAccountName` for catalog consistency.

#### WE Ansible TokenRequest Injection (#501)

- **TokenRequest API integration** — Ansible executor injects short-lived SA tokens via the Kubernetes TokenRequest API with configurable TTL validation, replacing long-lived secrets.
- **CRD schema migration** — `serviceAccount` string field migrated to structured `spec.serviceAccountName` with backward compatibility.

#### DS Resilience and Startup Reconciliation (#548)

- **Deterministic UUIDv5 workflow IDs** — Workflow IDs derived from content hash (UUIDv5), ensuring idempotent re-registration after PVC wipe.
- **Authwebhook startup reconciler** — New startup reconciler in authwebhook re-registers all RemediationWorkflows with DataStorage on controller startup, recovering from PVC data loss without manual intervention.

#### Hash-Capture Degradation Notifications (#546)

- **Degradation condition types** — New `PreRemediationHashCaptured` and `PostRemediationHashCaptured` condition types in VerificationContext surface hash-capture failures.
- **RO and EM reconciler integration** — Both controllers set degradation conditions when hash capture fails, propagating status to parent resources.
- **Completion notification enrichment** — Completion notifications now include hash-capture degradation status for operator visibility.

#### Effectiveness Monitor ADR-EM-001 Gaps (#573)

- **G1: Failed phase** — EM transitions EA to `Failed` for unrecoverable conditions (missing correlation, invalid spec).
- **G2: Scheduled event timing** — `effectiveness.assessment.scheduled` audit event emitted on all three entry transitions (WaitingForPropagation, Stabilizing, Assessing).
- **G3: Config knobs** — `prometheusLookback`, `maxConcurrentReconciles`, and `scrapeInterval` configurable via YAML.
- **G4: Assessment path differentiation** — Reconciler branches assessment depth based on WFE started/completed status. Partial-scope grace period (30s) handles async event propagation.

#### Feature Enrichments (#318, #366, #396, #435)

- **EA verification in completion notifications** (#318) — Completion notifications include effectiveness assessment results (health score, alert resolution, spec drift).
- **ResourceQuota detection** (#366) — Signal Processing detects namespace ResourceQuota/LimitRange constraints and surfaces them to the LLM via `ResourceQuotaConstrained` in DetectedLabels.
- **ConfigMap composite hashing** (#396) — RO and EM spec hash computation includes mounted ConfigMap content for drift detection across configuration changes.
- **LLM token usage in audit traces** (#435) — Token counts from HolmesGPT responses wired into audit events for cost tracking and usage analysis.

#### Notification Metrics (DD-METRICS-001)

- **1-layer metrics architecture** — Collapsed 3-layer notification metrics (interface → recorder → metrics) into a single `*Metrics` struct with direct Prometheus counter/histogram methods.

### Fixed

- **Duplicate scheduled audit event** — Removed duplicate `effectiveness.assessment.scheduled` emission from `emitAssessingTransitionEvents`.
- **HasWorkflowCompleted event type** — Corrected from `workflowexecution.execution.completed` to `workflowexecution.workflow.completed` to match the WE controller's actual event type.
- **Notification CircuitBreakerState type assertion** — Fixed `UpdateCircuitBreakerState` type assertion that would panic on non-string values.
- **RR CRD namespace and kind columns** (#622) — Added namespace and kind to `kubectl get rr -owide` output.
- **Notification field ordering and content** (#621, #626, #627) — Added RR name, reordered notification fields, included cluster name in timeout messages.
- **HAPI Phase 1 structured output** (#624) — Enabled `PHASE1_SECTIONS` for structured LLM output and refactored Pattern 2B to parse directly into Python dict, eliminating markdown round-trip.
- **EM validityWindow** (#625) — Increased EM `validityWindow` from 120s to 300s to prevent premature assessment expiry.
- **WE controller RBAC** (#637) — Added missing `serviceaccounts/token` create and `serviceaccounts` get permissions to the WorkflowExecution controller ClusterRole.
- **RR phase-specific Ready reasons** (#636) — Replaced generic Ready condition reasons with 12 phase-specific reasons (e.g., `Processing`, `Analyzing`, `AwaitingApproval`) for meaningful `kubectl get rr` REASON column.
- **RR kubectl column layout** (#635) — Overhauled `kubectl get rr` output with composite TARGET, WORKFLOW, CONFIDENCE, and ALERT columns. Added `FormatResourceDisplay`, `FormatWorkflowDisplay`, and `FormatConfidence` display helpers.
- **EM CPU metric query** (#639) — Corrected CPU metric query from raw `sum(container_cpu_usage_seconds_total)` to `sum(rate(...[5m]))`, preventing always-zero metric scores for counter-type metrics.
- **Graduated notification wording** (#639) — Replaced unconditional "anomaly persists" message with graduated wording based on `MetricsScore` (full improvement / partial improvement / minimal improvement / no improvement).
- **NotificationRequest PascalCase enums** (#640) — Migrated `NotificationType` and `NotificationPriority` enum values from lowercase to PascalCase for consistency with other CRD enums. Updated OpenAPI specs, ogen-client, routing attributes, and all test fixtures.
- **RR WorkflowDisplayName resolution** (#643) — Resolved workflow UUID to human-readable CRD name via `RemediationWorkflow` lookup. Fixed scheme registration, RBAC, and cache-blocking issue by using `apiReader` for direct API reads.
- **RR printer column layout** (#644) — Reordered `kubectl get rr -owide` columns to follow the pipeline flow, renamed "Target" to "RCA Target", added "Signal NS" column, removed redundant "Reason" column.

### Changed

- **CRD: Storm detection fields removed** (#448, DD-GATEWAY-015) — `stormCorrelationId`, `stormWindowId`, and related fields removed from SignalProcessing and RemediationRequest CRDs. All storm detection code removed from gateway.
- **EM client constructors simplified** — Prometheus and AlertManager HTTP clients now accept `(url, timeout)` instead of a pre-configured `*http.Client`.
- **Database migrations** — `002_add_service_account_name.sql` adds SA column; `003_capitalize_catalog_status.sql` normalizes enum casing.

### Upgrade Notes

- **Breaking: Storm detection removed** — `stormCorrelationId` and `stormWindowId` fields no longer exist in SignalProcessing and RemediationRequest CRDs. Any custom tooling reading these fields must be updated.
- **Breaking: CRD typed enums** — String fields replaced with typed enums across all CRDs. Existing resources with free-form string values that don't match the enum will fail validation on update.
- **Breaking: `metav1.Duration` fields** — WorkflowExecution duration fields now require Go duration format (e.g., `30s`, `5m`) instead of free-form strings.
- **Breaking: NotificationRequest PascalCase enums** (#640) — `NotificationType` values changed from lowercase (e.g., `escalation`, `manual-review`) to PascalCase (e.g., `Escalation`, `ManualReview`). `NotificationPriority` values changed similarly (e.g., `critical` → `Critical`). Update any routing configurations or tooling that matches on these values.
- **Database migration required** — Run migrations 002 and 003 before upgrading controllers. The Helm pre-upgrade hook handles this automatically.

[1.2.0]: https://github.com/jordigilh/kubernaut/compare/v1.1.0...v1.2.0

## [1.1.0] - 2026-03-28

### Added

#### AI Analysis (HAPI)

- **Three-phase RCA architecture** (#529) -- Rewrote the HolmesGPT integration with a three-phase protocol: Investigation, Enrichment, and Workflow Selection. Each phase runs as a separate LLM conversation with dedicated tools and prompts, replacing the single-shot approach from v1.0.
- **Anti-confirmation-bias guardrails** (#462) -- Investigation prompt now includes explicit instructions to consider alternative root causes and avoid anchoring on the first hypothesis.
- **Enrichment audit events** (#533) -- Phase 2 enrichment events (`aiagent.enrichment.*`) provide SOC2 chain-of-custody for the label detection and workflow discovery steps.
- **Tool rename and conditional injection** (#524) -- Renamed `get_resource_context` to scoped tools (`get_namespaced_resource_context`, `get_cluster_resource_context`) with conditional injection based on the target resource type.
- **Target resource mismatch detection** (#496) -- Detects when the LLM-identified remediation target differs from the signal source and resolves to the Kubernetes root owner.

#### Workflow Execution

- **AWX credential injection** (#500) -- Injects the WE controller ServiceAccount token into AWX/AAP jobs, giving Ansible playbooks authenticated access to the Kubernetes API without hardcoded credentials.
- **Runtime engine resolution** (#518) -- The `executionEngine` field moved from WFE spec to status; the WE controller now resolves the engine at runtime from the DataStorage workflow catalog.
- **Ansible playbook RBAC** (#551) -- Added missing RBAC rules for Ansible-executed playbooks to the `kubernaut-workflow-runner` ClusterRole.
- **Custom K8s credential type** (#552) -- Fixed `kubeconfig-file` injection in AWX/AAP execution environments with Jinja2-templated credential types and structured Go data models.

#### Effectiveness Monitor

- **TLS CA support** (#452) -- External HTTP clients (Prometheus, AlertManager) now accept custom TLS CA bundles for environments using internal certificate authorities.
- **Buffered audit ingestion** (ADR-038) -- Audit events are buffered and batch-flushed to DataStorage, reducing per-event HTTP overhead.

#### Notification

- **Workflow name enrichment** (#553) -- Notification bodies now display human-readable workflow names instead of raw UUIDs. The Notification controller resolves names from the DataStorage workflow catalog via a new `WorkflowNameResolver` interface.

#### Helm Chart

- **OCP support** (#348, #452) -- OpenShift Container Platform deployment with RHEL10 base images, service-serving CA injection, cluster-monitoring-view RBAC, and `values-ocp.yaml` overlay.
- **Pre-upgrade CRD hook** (#521) -- Helm pre-upgrade hook job applies CRD schema updates during `helm upgrade`, ensuring new fields are available before controllers restart.
- **Externalized Rego policies** (#404) -- AI Analysis approval policies and Signal Processing classification policies are loaded from user-provided ConfigMaps.
- **Externalized notification routing** (#405) -- Notification routing configuration loaded from user-provided ConfigMap.
- **Helm smoke tests** (#342) -- 14 IEEE 829-compliant smoke tests validate ConfigMap split, Prometheus integration, and SDK tiers.
- **OCI chart distribution** (#403) -- Chart published to `oci://quay.io/kubernaut-ai/kubernaut` with embedded defaults for demo-ready deployment.
- **Istio RBAC rules** (#373) -- ClusterRole rules for HolmesGPT, WorkflowExecution, Remediation Orchestrator, and Effectiveness Monitor in Istio-enabled clusters.
- **Vertex AI support** -- Helm chart supports Google Vertex AI as an LLM provider with project/region environment variables and GCP credential mounting.

#### Remediation Orchestrator

- **Forward hash chain detection** (#525) -- Detects escalation regression by comparing pre-remediation spec hashes across consecutive remediations.
- **NoActionRequiredDelayHours** (#353) -- Configurable cooldown period before re-processing signals that previously resulted in no action.
- **RBAC for cert-manager CRDs** (#545) -- RO and EM ClusterRoles now include read access for cert-manager Certificate resources used as remediation targets.

#### DataStorage

- **Query metrics and DLQ observability** -- Prometheus metrics for query latency, dead-letter queue depth, and audit event ingestion rates.
- **RFC 7807 error details** (#446) -- `DSClientAdapter` surfaces structured error details from DataStorage API responses.

#### Signal Processing

- **Consolidated Rego policies** (#414, #415) -- Five separate Rego policy files consolidated into a single unified policy file for simpler configuration and reasoning.

#### Infrastructure

- **Automated CRD API reference** -- Auto-generated CRD documentation covering 9 CRDs and 80 types using `crd-ref-docs`.
- **Pre-commit anti-pattern detection** -- CRD docs drift detection and test anti-pattern checks in pre-commit hooks.

### Fixed

#### AI Analysis (HAPI)

- **Scope enforcement** (#513) -- Deny-by-default when scopeChecker is nil; fail-closed on `IsManaged` errors.
- **Remediation history wiring** (#540) -- `query_remediation_history` correctly wired into EnrichmentService with spec hash context.
- **Root owner resolution** (#535) -- `list_available_actions` now uses the resolved root owner kind (e.g., Deployment) instead of the symptom resource kind (e.g., Pod).
- **Structured output retry** (#372) -- Returns failed `ValidationResult` on LLM format failures instead of silently proceeding.
- **LLM credential validation** (#487) -- Fail-fast at startup when LLM credentials are missing instead of failing on first request.
- **AffectedResource → RemediationTarget rename** (#542) -- LLM schema, prompts, mock LLM, and all Go types updated consistently.

#### Workflow Execution

- **AWX credential merge** (#365) -- Template credentials correctly merged with ephemeral credentials on AWX job launch.
- **Pre-execution Job cleanup** (#374, #375) -- Completed Jobs from previous runs are cleaned up before new execution, with ownership checks to prevent cross-WFE Job deletion (#383).
- **Explicit engine dispatch** -- Removed silent Tekton fallback; engine dispatch is now explicit in `reconcilePending`.

#### Effectiveness Monitor

- **DataStorage JSON deserialization** (#575) -- EM was decoding the DS audit events response as a bare array, but the API returns a paginated envelope `{"data": [...]}`. Fixed with typed structs matching the OpenAPI contract.
- **Audit event type mismatch** (#579) -- `HasWorkflowStarted` queried for `workflowexecution.workflow.started`, but the WE controller emits `workflowexecution.execution.started`. Also removed the dead `RecordWorkflowStarted` method that emitted the wrong event type.
- **OCP AlertManager RBAC** (#576) -- Replaced `nonResourceURLs` with resource-level RBAC (`monitoring.coreos.com/alertmanagers/api`) for OCP's `kube-rbac-proxy`.
- **OCP AlertManager ClusterRole** (#471) -- Added `kubernaut-alertmanager-view` ClusterRole for EM on OCP.

#### Remediation Orchestrator

- **ManualReviewRequired completion** (#550) -- `ManualReviewRequired` remediations now transition to `PhaseCompleted` with `Outcome=ManualReviewRequired` instead of `PhaseFailed`.
- **Resilient hash capture** (#545) -- Pre-remediation hash capture uses 3-tuple return with graceful fallback when target resource is temporarily unavailable.

#### Helm Chart

- **PostgreSQL data directory** (#464) -- Pinned `PGDATA` to image-agnostic `/var/lib/kubernaut-pg/data`, preventing silent data loss when switching between upstream and OCP PostgreSQL images.
- **PostgreSQL secret rotation** (#557) -- Removed auto-generation of PostgreSQL passwords; secrets must be pre-created. Consolidated `postgresql-secret` and `datastorage-db-secret` into a single secret. Added `fail` validation with canary lookup guard.
- **CRD upgrade hook RBAC race** (#558) -- Added RBAC readiness check (`kubectl auth can-i`) before applying CRDs in the pre-upgrade hook job.
- **Notification routing** (#571) -- Replaced specific but incorrect routes with a catch-all `slack-and-console` receiver that correctly routes all notification types.
- **Restricted PodSecurity** (#449) -- `wait-for-postgres` init container complies with restricted PodSecurity standards.
- **LLM credentials mandatory** (#429) -- `llm-credentials` Secret is now mandatory; chart fails early if missing.
- **Self-healing caBundle** (#377) -- AuthWebhook caBundle is injected via init-container instead of relying on stale cert-manager annotations.

#### DataStorage

- **Wildcard label matching** (#464) -- `buildContextFilterSQL` array branch now includes `OR labels->'priority' ? '*'` for consistent wildcard matching.
- **Authwebhook catalog consistency** (#418, #512) -- Finalizer controller ensures DS catalog consistency on RemediationWorkflow deletion; recovery from orphaned DS workflows.

#### Signal Processing

- **Stale cache classification** -- Signal Processing refetches via APIReader to prevent stale informer cache from causing misclassification.

#### Gateway

- **Deleted pod webhook handling** (#451) -- Gateway no longer drops the entire webhook payload when one alert references a deleted pod.

### Changed

- **CRD: AffectedResource → RemediationTarget** (#542) -- Breaking rename across all CRDs, Go types, Rego policies, LLM schema, and documentation.
- **CRD: executionEngine moved to WFE status** (#518) -- Runtime-resolved field removed from spec; WE controller sets it during reconciliation.
- **Helm: Simplified values.yaml** (#403–#412) -- Consolidated `postgresql`/`externalPostgresql` and `valkey`/`externalValkey` into single groups; removed `eventExporter`, `nodePort`, and security context noise.
- **Helm: RHEL10 base images** (#348) -- PostgreSQL and Valkey migrated to RHEL10 direct pulls; Redis replaced with Valkey 8.
- **Helm: OCI registry** (#357) -- Chart URI moved from `ghcr.io` to `oci://quay.io/kubernaut-ai`.
- **Metrics port standardization** (#283) -- DataStorage and Notification metrics port aligned to `:9090`.
- **Rego: approval policies** (#542) -- `affected_resource` renamed to `remediation_target` in all approval and classification policies.
- **Rego: OPA v1.0 compliance** -- Replaced inline if/else with split rules for OPA v1.0 compatibility.
- **DB migrations squashed** -- All migrations consolidated into single `001_v1_schema.sql` for clean installations.

### Upgrade Notes

- **Breaking: CRD rename** — `AffectedResource` renamed to `RemediationTarget` across all CRDs. Existing `AIAnalysis` and `RemediationRequest` resources referencing the old field name must be recreated. See [blast radius documentation](docs/architecture/decisions/DD-CRD-004-affected-resource-rename.md) for migration guide.
- **Breaking: PostgreSQL secrets** — Auto-generation of PostgreSQL passwords has been removed. Secrets must be pre-created before `helm install`. See [README](charts/kubernaut/README.md) for required secret format. The separate `datastorage-db-secret` has been consolidated into `postgresql-secret`.
- **Breaking: Rego policies** — Approval policies now use `remediation_target` instead of `affected_resource`. Update any custom Rego policies.
- **PostgreSQL data directory** — Moved to `/var/lib/kubernaut-pg/data`. Existing installations from v1.0.0 must either copy data from the old path or repopulate the catalog after upgrade.
- **Helm values restructured** — `externalPostgresql`/`externalValkey` sections removed; use `postgresql.enabled=false` with `postgresql.auth.existingSecret` instead. `eventExporter` removed entirely.
- **OCI chart location** — Chart moved from `ghcr.io` to `oci://quay.io/kubernaut-ai/kubernaut`.

[1.1.0]: https://github.com/jordigilh/kubernaut/compare/v1.0.0...v1.1.0

## [1.0.0] - 2026-03-02

### Added

- **Signal Processing** -- Ingests Prometheus AlertManager webhooks and Kubernetes Events with fingerprint-based deduplication, Rego policy evaluation, and custom label injection
- **AI Analysis** -- HolmesGPT-powered root cause analysis with live `kubectl` access, remediation history context, and structured workflow recommendations
- **Workflow Execution** -- Multi-engine remediation via Kubernetes Jobs, Tekton Pipelines, and Ansible (AWX/AAP) with OCI-packaged workflow images
- **Remediation Orchestrator** -- Full pipeline coordination from signal to outcome, including approval gates, GitOps-aware delays, and escalation policies
- **Effectiveness Monitor** -- Post-remediation health checks using alert resolution, spec hash drift detection, and effectiveness scoring fed back into future analyses
- **DataStorage** -- Centralized workflow catalog with hybrid weighted-label scoring, remediation history tracking, and audit trail (SOC2-ready)
- **Gateway** -- Alert ingestion with distributed locking, storm detection, and suppression for NoActionRequired outcomes
- **Notification Service** -- Slack and console notification channels for remediation lifecycle events
- **Human Approval Gates** -- RemediationApprovalRequest CRD with configurable auto-approve and timeout policies
- **Helm Chart** -- Single-command deployment via `helm install` with CRD management and configurable RBAC
- **22 Demo Scenarios** -- End-to-end validated scenarios covering CrashLoopBackOff, OOMKill, HPA scaling, node failures, GitOps drift, certificate expiry, network policies, and more (see [kubernaut-demo-scenarios](https://github.com/jordigilh/kubernaut-demo-scenarios))

[1.0.0]: https://github.com/jordigilh/kubernaut/compare/v0.0.0...v1.0.0
