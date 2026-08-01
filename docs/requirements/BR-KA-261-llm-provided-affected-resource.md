# BR-KA-261: LLM-Provided Affected Resource with Owner Resolution

**Business Requirement ID**: BR-KA-261
**Category**: Kubernaut Agent (KA) Service
**Priority**: P1
**Target Version**: V1
**Status**: Approved
**Date**: 2026-03-25
**Related Issue**: [#529](https://github.com/jordigilh/kubernaut/issues/529)

---

## Business Need

### Problem Statement

Under the pre-Go-rewrite architecture (DD-HAPI-006 v1.5, historical), the LLM never provided `affectedResource`. Instead, the resource context tool resolved the root owner and the service injected it. This means the LLM's RCA output had no explicit declaration of which resource it identified as the root cause. The target identity depended entirely on which resource the LLM happened to call the resource context tool for.

> **Current state (Go KA, see [DD-KA-006](../architecture/decisions/DD-KA-006-remediation-target-in-rca.md))**: `InjectRemediationTarget` reconciles whatever target the LLM proposes against the K8s-verified owner chain resolved during enrichment — the LLM's proposal is never blindly trusted, but it also is not required to go through an explicit declare-then-validate round-trip. This BR's acceptance criteria describe the design intent that motivated the current reconciliation logic; treat the Go implementation notes in DD-KA-006 as authoritative if the two diverge.

Additionally, the LLM may name a child resource (e.g., Pod) as the affected resource when the remediation should target the root owner (e.g., Deployment). Applying a remediation to an ephemeral Pod is ineffective.

### Business Objective

Require the LLM to explicitly declare `affectedResource` in its RCA output. KA validates the format, resolves the K8s owner chain to the root managing resource, and auto-corrects the target. This gives:

1. Explicit LLM accountability for identifying the root cause target
2. K8s-verified owner resolution (Pod -> Deployment) as defense-in-depth
3. Clear separation: LLM identifies the resource, KA verifies and resolves it

---

## Acceptance Criteria

1. The RCA prompt instructs the LLM to provide `affectedResource` with `kind` and `name` (and `namespace` for namespace-scoped resources)
2. Phase 1 self-correction loop validates `affectedResource` format; missing or malformed triggers a retry with feedback
3. KA resolves the K8s owner chain for the validated `affectedResource` to find the root owner
4. If the resolved root owner differs from the LLM-provided resource (e.g., Pod -> Deployment), KA auto-corrects to the root owner
5. `_inject_target_resource` uses the resolved root owner (not the LLM-provided resource) for `TARGET_RESOURCE_*` injection
6. `affectedResource` in the KA response reflects the resolved root owner
7. If owner chain resolution fails after retries, KA fails hard with `rca_incomplete`

---

## Design References

- **DD-KA-006 v2.0**: Remediation Target in RCA (LLM-proposed + KA-reconciled)
- **ADR-055 v1.5**: Context Enrichment (EnrichmentService)
- **Issue #529**: RCA Flow Redesign

---

## Supersedes

This BR partially supersedes DD-HAPI-006 v1.5's (historical) "KA owns target resource identity -- the LLM never provides affectedResource" principle. The current approach is hybrid: the LLM proposes, KA verifies and resolves — see [DD-KA-006](../architecture/decisions/DD-KA-006-remediation-target-in-rca.md) for the current Go implementation.

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-25 | Initial requirement: LLM-provided affectedResource with KA owner chain resolution |
