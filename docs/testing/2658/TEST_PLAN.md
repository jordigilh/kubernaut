# Test Plan — Issue #2658: Group-Blind Resource Identity (Severity Triage + Execution Plane)

- **Issue**: [#2658](https://github.com/jordigilh/kubernaut/issues/2658)
- **Plan**: `PLAN-2658.md` (v12, clean bill after 13 independent review rounds)
- **BRs**: BR-SEVERITY-001 (AC-8), BR-ORCH-042 + ADR-052, DD-RO-002 / DD-WE-001, BR-AUDIT-005 / BR-GATEWAY-058-A
- **Framework**: Ginkgo/Gomega BDD (no `testing.T` business tests)
- **Coverage accounting** (requirements-based, per AGENTS.md): critical scenarios 100%; in-scope ≥90%

## Scenario Matrix — Critical (100% required)

| ID | Scenario | Tier | Component |
|---|---|---|---|
| UT-SCOPE-2658-001 | Normalizer/ParseAPIVersion matrix: `core/v1`→("",v1); `core/v2`,`core/v1beta1`,`v2`,`v1beta1`→invalid; `unresolved`→ScopeUnknown; `apps/v1`/`apps`→apps; bare `core` on api_version rejected | UT | pkg/shared/scope |
| UT-AF-2658-001 | Cross-group sibling exists → ambiguous `group_unscopable`, CandidateAPIGroups surfaced | UT | pkg/apifrontend/severity |
| UT-AF-2658-002 | No sibling (name/namespace mismatch) → unique instance, severity accepted | UT | pkg/apifrontend/severity |
| UT-AF-2658-003 | Single-group Kind fast path unchanged | UT | pkg/apifrontend/severity |
| UT-AF-2658-004 | Rule tiers gated; rule-name candidate non-empty | UT | pkg/apifrontend/severity |
| UT-AF-2658-005 | confirmed_api_group round trip incl. 2×2 reason×confirmation matrix, core-sibling case, normalization | UT | pkg/apifrontend/severity + tools |
| UT-AF-2658-006 | DD-AF-012 contract intact for cluster_scoped_only | UT | pkg/apifrontend/severity |
| UT-AF-2658-007 | Checker error / cannot-enumerate (irregular plural) → fail closed; fleet → adjudicate + audit | UT | pkg/apifrontend/severity |
| UT-AF-2658-008 | prompt.txt distinct action per reason | UT | pkg/apifrontend/agent |
| UT-AF-2658-009 | severity_triage.ambiguous persisted with api_group/ambiguity_reason (schema + store adapter) | UT | pkg/apifrontend/audit |
| UT-RO-2658-001 | Busy gate: cross-group pair does not block; same-group still blocks | UT | pkg/remediationorchestrator/routing |
| UT-RO-2658-002/003 | Cooldown gate; distributed lock key group-aware | UT | routing + approval handler |
| UT-RO-2658-004 | PipelineRun names diverge; labels valid incl. >63-char hash truncation | UT | pkg/workflowexecution/executor |
| UT-RO-2658-005 | identity formats: resolved/core/unresolved; unknown ≠ core gate keys | UT | pkg/shared/scope builders |
| UT-RO-2658-006 | WFE validator accepts/rejects canonical identities | UT | internal/controller/workflowexecution |
| UT-RO-2658-007 | parseTargetResource identity-aware | UT | routing |
| UT-RO-2658-009 | ResourceIdentifier.String() verbatim Kind, canonical identity | UT | api/remediation |
| UT-DS-2658-001 | Required targetApiVersion: absent/empty→400; core/v1→core form; unresolved form; no sibling leak; depth≥2; patterns; empty-ns 200 | UT | pkg/datastorage |
| UT-DS-2658-002 | DS join key == RO emission key (all forms) | UT | datastorage + RO audit |
| UT-WE-2658-001..003 | Validator; WFE-side cooldown regression; sanitizer/param | UT | workflowexecution |
| UT-GW-2658-501 | correlation_id: alertname + canonical identity (group+version); core form; never version-only | UT | pkg/gateway |
| IT-AF-2658-101..107 | Wiring manifest rows (AF) | IT | tools + backend_deps |
| IT-RO-2658-201..208 | Wiring manifest rows (RO) | IT | creator/routing/audit |
| IT-WE-2658-205..207 | Wiring manifest rows (WE; 206 regression) | IT | WFE controller/executor |
| IT-DS-2658-301..303 | Wiring manifest rows (DS) | IT | openapi/ogen/handler/repo |
| IT-KA-2658-401/402 | Wiring manifest rows (KA enrichment + resource_context) | IT | kubernautagent |
| IT-GW-2658-501 | correlation_id (canonical identity) wiring | IT | pkg/gateway |

**Critical count: 23 UT + 17 IT scenario rows = 40/40 required (100%).**

## Scenario Matrix — Non-critical (≥90% of defined)

reason-driven messages; audit detail keys; CEL immutability unaffected; fleet ClusterID×group composition; legacy Job fallback; KA evidence attribution; TARGET_RESOURCE param value; notification `Kind/Name` (group extension deferred to #2671); AF history tool arg. **10 defined; ≥9 required.**

## Wiring Manifest

See PLAN-2658.md §3.2 (authoritative, 22 rows). CHECKPOINT W executed per row during GREEN.

## Notes

- No pending/skipped tests (AGENTS.md Rule 2).
- RRs constructed directly in RO ITs (#2653 merge-order prerequisite documented).
- Control objectives: SI-10, AU-3/AU-12, CC7.2, AC-4/DD-RO-002/DD-WE-001 (mapping in PLAN-2658.md §3.5).
