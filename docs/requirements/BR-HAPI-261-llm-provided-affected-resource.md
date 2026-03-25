# BR-HAPI-261: LLM-Provided Affected Resource with Owner Resolution

**Business Requirement ID**: BR-HAPI-261
**Category**: HolmesGPT API Service
**Priority**: P1
**Target Version**: V1
**Status**: Approved
**Date**: 2026-03-25
**Related Issue**: [#529](https://github.com/jordigilh/kubernaut/issues/529)

---

## Business Need

### Problem Statement

Under the current architecture (DD-HAPI-006 v1.5), the LLM never provides `affectedResource`. Instead, the resource context tool resolves the root owner and HAPI injects it. This means the LLM's RCA output has no explicit declaration of which resource it identified as the root cause. The target identity depends entirely on which resource the LLM happened to call the resource context tool for.

Additionally, the LLM may name a child resource (e.g., Pod) as the affected resource when the remediation should target the root owner (e.g., Deployment). Applying a remediation to an ephemeral Pod is ineffective.

### Business Objective

Require the LLM to explicitly declare `affectedResource` in its RCA output. HAPI validates the format, resolves the K8s owner chain to the root managing resource, and auto-corrects the target. This gives:

1. Explicit LLM accountability for identifying the root cause target
2. K8s-verified owner resolution (Pod -> Deployment) as defense-in-depth
3. Clear separation: LLM identifies the resource, HAPI verifies and resolves it

---

## Acceptance Criteria

1. The RCA prompt instructs the LLM to provide `affectedResource` with `kind` and `name` (and `namespace` for namespace-scoped resources)
2. Phase 1 self-correction loop validates `affectedResource` format; missing or malformed triggers a retry with feedback
3. HAPI resolves the K8s owner chain for the validated `affectedResource` to find the root owner
4. If the resolved root owner differs from the LLM-provided resource (e.g., Pod -> Deployment), HAPI auto-corrects to the root owner
5. `_inject_target_resource` uses the resolved root owner (not the LLM-provided resource) for `TARGET_RESOURCE_*` injection
6. `affectedResource` in the HAPI response reflects the resolved root owner
7. If owner chain resolution fails after retries, HAPI fails hard with `rca_incomplete`

---

## Design References

- **DD-HAPI-006 v1.6**: Affected Resource in RCA (updated for LLM-provided + HAPI-resolved)
- **ADR-055 v1.5**: Context Enrichment (EnrichmentService)
- **Issue #529**: RCA Flow Redesign

---

## Supersedes

This BR partially supersedes DD-HAPI-006 v1.5's "HAPI owns target resource identity -- the LLM never provides affectedResource" principle. The new approach is hybrid: the LLM provides, HAPI verifies and resolves.

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-25 | Initial requirement: LLM-provided affectedResource with HAPI owner chain resolution |
