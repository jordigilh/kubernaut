# ADR-021: Severity Triage Pipeline Design

**Status:** Accepted
**Date:** 2026-05-10
**Context:** Issue #92 — Three-Tier Severity Triage for Manual Signals

---

## Context

When a manual signal arrives without a user-supplied severity (via `af_create_rr` or `kubernaut_submit_signal`), the API Frontend must determine severity before creating the RemediationRequest CRD. The kubernaut pipeline requires severity for classification, prioritization, and routing.

The key design tension is between:
- **Availability** — always producing a severity (even a guess) so the pipeline can proceed
- **Correctness** — never producing an unjustified severity that could misroute remediations

## Decision

### 1. LLM is mandatory — panic on nil

`NewTriager` panics if the `LLMTriager` dependency is nil. The pipeline requires an LLM fallback as the last resort to guarantee a justified result. This follows the same startup fail-fast pattern as `RegisterTools` panicking on nil `RBACRoles`.

**Rationale:** A triage pipeline without LLM can only derive severity from Prometheus. If Prometheus is unreachable (network, downtime, misconfiguration), every triage call would fail. Making LLM optional would require a hardcoded default, which violates the correctness constraint.

### 2. No silent defaults — Tier 3 errors propagate

If the LLM call fails at Tier 3 (the last resort), the error is propagated to the caller. The pipeline never returns a hardcoded "medium" or any other default severity.

**Rationale:** A silent default masks pipeline failures. An SRE cannot distinguish "triage determined medium" from "triage failed and we guessed medium." Every severity value must be traceable to a source (firing alert, pending rule, rule evaluation, or LLM classification).

### 3. PromQL expressions are server-controlled only

All PromQL expressions used in Tier 2 instant queries originate from Prometheus `/api/v1/rules` responses. User input is never interpolated into query strings.

**Rationale:** Prevents PromQL injection. The `ExtractLabelMatchers` function reads ASTs — it does not construct queries from user data.

### 4. Lazy cache eviction (no background goroutines)

The rules cache uses TTL-based lazy eviction checked on `Get()`. There is no background refresh goroutine.

**Rationale:** Avoids goroutine lifecycle complexity (`#62: Starting goroutine without knowing when to stop` from 100 Go Mistakes). A 30-second TTL with lazy check is sufficient — the cache is only read during triage calls.

## Consequences

- The AF binary **cannot start** without a configured LLM provider (intentional fail-fast)
- Triage failures surface as errors to the tool caller, which must handle them (e.g., retry, surface to user)
- Prometheus downtime degrades triage to LLM-only (Tier 3) but does not cause failure
- LLM downtime causes triage failure — this is the intended behavior, not a deficiency

## Alternatives Considered

| Alternative | Rejected Because |
|-------------|-----------------|
| Default to "medium" when LLM is nil or fails | Masks failures; produces unjustified severity; SRE cannot distinguish default from derived |
| Make LLM optional, fall back to "unknown" | The `unknown` severity is not in the canonical set and causes downstream misrouting |
| Filter `tools/list` by RBAC instead of runtime enforcement | RBAC state can change between listing and calling; runtime enforcement is more accurate (see ADR-020) |
| Background cache refresh goroutine | Adds goroutine lifecycle complexity with minimal benefit over lazy TTL |

---

*Design document: `docs/design/SEVERITY_TRIAGE.md`*
*Test plan: `docs/tests/92/test_plan.md`*
