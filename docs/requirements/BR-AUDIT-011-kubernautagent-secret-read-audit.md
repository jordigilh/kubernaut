# BR-AUDIT-011: KubernautAgent Secret Read Audit (Detective Control)

**Business Requirement ID**: BR-AUDIT-011
**Category**: KubernautAgent — Security & Compliance
**Priority**: **P2 (MEDIUM)** — Closes a MEDIUM-severity GA readiness gap
**Target Version**: **V1.5**
**Status**: ✅ Implemented
**Date**: June 30, 2026
**Related BRs**: BR-AUDIT-005 (Core Audit Business Requirement), BR-SEC-012 (log all secret access attempts), BR-INTERACTIVE-003 (per-request K8s call audit)
**GitHub Issues**: [#1505](https://github.com/jordigilh/kubernaut/issues/1505) (GAP-13)

---

## Business Need

### Problem Statement

KubernautAgent's autonomous investigator holds broad `get`/`list`/`watch` RBAC on `secrets` (core group), granted deliberately alongside pods/configmaps/events/etc. so that a missing permission never silently degrades root-cause-analysis quality (see [`docs/services/stateless/kubernaut-agent/security-configuration.md`](../services/stateless/kubernaut-agent/security-configuration.md)). Every K8s tool call — including a Secret read via `kubectl_get_by_name`, `kubectl_describe`, `kubectl_get_yaml`, `kubectl_get_by_kind_in_namespace`, etc. — already produced a generic `aiagent.llm.tool_call` audit event, but that event does not distinguish "this tool call touched a Secret" from any other resource read; identifying Secret accesses required parsing the raw `tool_arguments` JSON of every tool-call event after the fact.

This was identified as **GAP-13 (MEDIUM severity)** in the GA Readiness Audit (issue #1505): KubernautAgent's Secret access lacked a dedicated, independently queryable audit trail.

### Decision: Audit-Only, Not RBAC Narrowing

Two options were considered:

1. **Narrow RBAC** — restrict KubernautAgent's ClusterRole so it cannot read `secrets` at all, or only a scoped subset.
2. **Detective control (chosen)** — keep the existing broad RBAC (preserving investigation completeness) and add a dedicated audit event for every Secret access, so every read is independently reviewable.

Narrowing RBAC was rejected: KubernautAgent frequently needs to inspect Secrets to diagnose issues (e.g. malformed credentials causing a CrashLoopBackOff, expired TLS certs, misconfigured `envFrom` references) and a missing permission produces a *silent* investigation quality regression rather than a visible error. A detective control — auditing every access — achieves the compliance goal (SOC2 CC7.2 change/access review, FedRAMP AU-12 audit generation) without that regression risk.

### Implementation

1. **`aiagent.secret.accessed`** is a new, dedicated audit event type (`internal/kubernautagent/audit/emitter.go`), emitted for every Get/List that the K8s resource resolver resolves to the **core** `secrets` resource — resolved via the `RESTMapper`'s `GroupVersionResource`, not by string-matching the LLM-supplied `kind` argument, so a differently-cased kind string (`"secret"`, `"Secret"`) or an unrelated CRD that happens to be *named* similarly cannot spoof or evade the hook.
2. **`SecretAccessObserver`** (`pkg/kubernautagent/tools/k8s/resolver.go`) is an optional callback wired into `NewDynamicResolver` via `WithSecretAccessObserver`. It fires on every Get/List against Secrets, success or failure, and is invisible to any of the ~7 tool implementations that route through the resolver — new tools added later automatically get the same detective coverage without additional wiring.
3. **Correlation**: `session.Manager.launchInvestigation` now sets the investigation's correlation ID on the background context (`audit.WithCorrelationID`), so the observer — running deep inside tool execution with no direct access to the investigation's metadata — can still emit a correctly-correlated event via `audit.CorrelationIDFromContext(ctx)`.
4. **Redaction unaffected**: the existing `SecretSanitizer` (`pkg/kubernautagent/tools/sanitization/secret.go`) still redacts Secret `data`/`stringData` values in the tool *result* string returned to the LLM and stored on the generic `aiagent.llm.tool_call` event; the new `aiagent.secret.accessed` event never carries the Secret's own data — only verb/namespace/name/outcome.
5. **Production wiring**: `cmd/kubernautagent/main.go` (`registerK8sTools`, `newSecretAccessObserver`) constructs the observer from the same `audit.AuditStore` already used for every other KubernautAgent audit event, so the event lands in DataStorage identically to `aiagent.llm.tool_call`, `aiagent.session.started`, etc.

---

## Success Criteria

1. Every `dynamicResourceResolver.Get`/`List` call that resolves to the core `Secret` resource invokes the configured `SecretAccessObserver` exactly once, regardless of success/failure — verified by unit tests in `pkg/kubernautagent/tools/k8s/secret_access_observer_test.go`.
2. Accesses to non-Secret kinds (e.g. `ConfigMap`) never invoke the observer.
3. `aiagent.secret.accessed` audit events carry `verb` (`get`/`list`), `namespace`, `secret_name` (omitted for list), and `outcome_detail` (error message on failure) — verified in `internal/kubernautagent/audit/secret_access_audit_test.go`.
4. The investigation's correlation ID (RemediationID) is retrievable from the tool-execution context via `audit.CorrelationIDFromContext`, so every `aiagent.secret.accessed` event is correlated to its parent remediation request per BR-AUDIT-005 (SOC2 CC8.1 reconstruction) — verified in `internal/kubernautagent/session/correlation_id_context_test.go`.
5. No RBAC change: KubernautAgent's ClusterRole grant on `secrets` is unchanged by this BR.

---

## Related Documents

- [KubernautAgent Security Configuration](../services/stateless/kubernaut-agent/security-configuration.md) — RBAC rationale (broad-by-resource, narrow-by-verb)
- [DD-AUDIT-003: Service Audit Trace Requirements](../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md)

---

**Document Version**: 1.0
**Last Updated**: June 30, 2026
**Maintained By**: Kubernaut Architecture Team
