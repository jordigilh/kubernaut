# PR Review Prompt Template

Use this prompt when asking the AI to review a pull request from all project personas.

---

## The Prompt

> Review this PR. Audit every file changed from **all 8 personas** below. For each finding, assign a severity (P1 must-fix, P2 should-fix, P3 nit/hardening) and cite the specific file and line range. At the end, provide a summary verdict: **approve**, **approve-with-nits**, or **request-changes**.
>
> ### 1. QE (Quality Engineering) -- "Can we ship this with confidence?"
>
> 1.1. **Coverage and traceability**: Does every new or modified public function/method have at least one test? Can every test be traced back to a requirement or acceptance criterion (via test ID, issue number, or descriptive name)? Flag any public symbol with zero test coverage.
>
> 1.2. **Negative and boundary testing**: For every happy-path test, is there a corresponding error-path test? For every validated input, is there a test that exercises rejection? Flag any function with only sunny-day tests.
>
> 1.3. **Test anti-patterns**: Flag tests that: sleep for fixed durations instead of polling/channels, assert on implementation details (struct field names, log strings) instead of behavior, share mutable state across specs without isolation, use `time.Now()` without a clock interface, or have assertions that can never fail (tautologies).
>
> 1.4. **Regression confidence**: For any bug fixed in this PR, is there a test that reproduces the original failure? For any race condition addressed, is there a concurrent stress test with `-race`? Flag any fix without a corresponding regression test.
>
> ### 2. Prod Security -- "Can an attacker exploit this?"
>
> 2.1. **Input validation**: For every string parameter accepted from outside the package (IDs, names, messages, headers), is it validated or sanitized before use? Flag any input used directly as a K8s name, log field, SQL parameter, or file path without validation.
>
> 2.2. **Secret handling**: Are secrets excluded from logs, metrics labels, error messages, and truncated tool responses? Is credential rotation supported without restarts? Flag any code path that could leak a secret.
>
> 2.3. **RBAC least-privilege**: Are RBAC verbs scoped to the minimum required? No wildcard verbs? Impersonation justified with compensating controls and documented in an ADR? Flag any over-privileged role binding.
>
> 2.4. **Dependency supply chain**: Are CI actions and tool versions pinned to immutable refs (commit SHA, not mutable tags)? Is `govulncheck` clean? Flag any unpinned dependency or known CVE.
>
> ### 3. Product -- "Does this deliver what was promised?"
>
> 3.1. **Acceptance criteria coverage**: Does every user story or acceptance criterion in the linked issue have a corresponding test or code path? Flag any AC that is not addressed.
>
> 3.2. **Backward compatibility**: Are there breaking changes to existing API contracts, CRD schemas, SSE event formats, or CLI flags? If so, is there a migration path documented? Flag any silent breaking change.
>
> 3.3. **Deprecation hygiene**: Are removed or replaced features marked with deprecation notices before removal? Is there zombie code (unreachable functions, unused exports) left behind? Flag any dead code.
>
> 3.4. **User-facing error quality**: Are error messages returned to API consumers actionable and free of internal IDs, stack traces, or implementation details? Flag any error response that leaks internals.
>
> ### 4. UX (API Consumer Experience) -- "Is this intuitive to consume?"
>
> 4.1. **API contract clarity**: Are REST endpoints, SSE event types, request/response schemas, and error codes consistent with existing conventions and documented (OpenAPI, GoDoc, or ARCHITECTURE.md)? Flag any undocumented public endpoint or event type.
>
> 4.2. **Naming consistency**: Do field names, event types, metric labels, and CRD fields follow the project's established naming conventions (snake_case for JSON, camelCase for Go, kebab-case for K8s)? Flag any naming inconsistency.
>
> 4.3. **Error response ergonomics**: Do error responses include a machine-readable error code, a human-readable message, and enough context for the caller to retry or escalate? Flag any bare HTTP status with no body or any unstructured error string.
>
> 4.4. **Progressive disclosure**: For streaming endpoints (SSE), does the event sequence tell a coherent story (start, progress, tool calls, completion)? Are heartbeats, reconnection hints (`retry:`), and terminal signals (`done`) present? Flag any streaming contract gap.
>
> ### 5. Architecture -- "Does this fit the system?"
>
> 5.1. **Dependency direction**: Does any `internal/` package import from `cmd/`? Are there circular dependencies? Flag any import that violates the dependency direction defined in ARCHITECTURE.md.
>
> 5.2. **Abstraction leakage**: Are concrete types crossing package boundaries where interfaces are expected? Flag any function that accepts or returns a concrete struct from another package when an interface would suffice.
>
> 5.3. **CRD schema evolution**: Are new CRD fields optional with defaults? Is any required field removed or renamed? Flag any CRD change that would break existing resources on upgrade.
>
> 5.4. **ADR compliance**: Does the implementation match the decisions documented in `docs/adr/`? Flag any deviation from an ADR that is not itself documented as a new ADR or amendment.
>
> 5.5. **API surface hygiene**: Are test helpers, internal constants, or debug functions exported from production packages? Flag any exported symbol only referenced from `_test.go` files.
>
> ### 6. SRE/Ops -- "Can we run this in production?"
>
> 6.1. **Observability wiring**: Is every metric/gauge/counter that is defined also incremented from the correct production code path? Flag any metric that exists in the registry but is never called, or any significant state change that doesn't emit a metric.
>
> 6.2. **Error-path observability**: For every error return, does the log line include enough structured context (resource name, namespace, phase, user) for an SRE to diagnose without reading source code? Flag any `fmt.Errorf` that wraps without adding context, or any error swallowed with `_ =`.
>
> 6.3. **Resource bounds**: For every map, slice, channel, or cache that grows with usage, is there a bounded lifecycle (eviction, pruning, max size)? Flag any unbounded collection keyed by user-controlled or session-scoped data.
>
> 6.4. **Graceful degradation**: Are external dependencies (KA, JWKS, K8s API) protected by circuit breakers, timeouts, and fallbacks? Flag any `http.Get` or API call without a timeout or context deadline.
>
> 6.5. **Readiness/liveness**: Do health check endpoints (`/readyz`, `/healthz`) reflect actual component readiness (not just "process is alive")? Flag any readiness check that always returns 200.
>
> ### 7. Release / Change Management -- "Can we roll this out and roll it back safely?"
>
> 7.1. **Rollback safety**: Are there irreversible data migrations or CRD schema changes that would prevent rollback to the previous version? Flag any destructive migration without a rollback path.
>
> 7.2. **Feature flags**: Is new behavior gated behind feature flags or environment variables where appropriate, to allow progressive rollout? Flag any high-risk behavior that is unconditionally enabled.
>
> 7.3. **Changelog**: Are user-visible changes (new endpoints, changed behavior, new CRD fields) documented in a changelog or release notes? Flag any user-facing change without documentation.
>
> 7.4. **Deployment ordering**: Are there cross-service deployment dependencies that require coordinated rollout (e.g., CRD must be applied before controller starts)? Flag any ordering constraint that is not documented.
>
> ### 8. FedRAMP Compliance -- "Does this satisfy NIST 800-53?"
>
> 8.1. **AU-2/AU-12 (Audit)**: Does every security-relevant event (authentication, session lifecycle, access denial, privilege escalation) emit a structured audit event via `audit.Emitter`? Flag any security action without an audit trail.
>
> 8.2. **AC-3/AC-6 (Access Control)**: Does RBAC follow least-privilege? Is impersonation documented with compensating controls in an ADR? Flag any over-scoped permission.
>
> 8.3. **IA-5 (Authenticator Management)**: Are JWKS keys rotated and cached with a maximum age? Is token validation enforced? Is any fail-open behavior justified in an ADR? Flag any authentication bypass.
>
> 8.4. **SC-4/SC-28 (Information Protection)**: Is PII classified per ADR-017? Is PII excluded from logs, metrics labels, and error responses? Flag any PII leakage path.
>
> 8.5. **SI-10 (Input Validation)**: Are all external inputs sanitized per their respective specs (RFC 1123 for K8s names, JWT claims validation, SSE field escaping)? Flag any unsanitized external input.

---

## Severity Guide

| Severity | Criteria | Action |
|---|---|---|
| **P1** (must-fix) | Security vulnerability, data loss risk, spec violation, missing audit trail, broken contract | Block merge until resolved |
| **P2** (should-fix) | Missing test coverage, observability gap, degraded UX, hardening opportunity | Fix before next release; may merge with tracked follow-up |
| **P3** (nit/hardening) | Style, naming, documentation, defense-in-depth | Fix at author's discretion |

## Verdict Criteria

| Verdict | When to use |
|---|---|
| **Approve** | Zero P1s, zero P2s, at most minor P3s |
| **Approve with nits** | Zero P1s, P2s have tracked follow-up issues, P3s noted |
| **Request changes** | Any P1, or P2s without a mitigation plan |
