# Implementation Plan: Target-scoped RCA type discovery

**Plan**: IP-2609-v1 | **Status**: Implementation complete; local and focused remote verification passed; full bootstrap/gateway wiring evidence pending
**Branch**: `triage/v16-rr-cancellation` | **Issue**: [#2609](https://github.com/jordigilh/kubernaut/issues/2609)
**Requirements**: BR-KA-212, BR-WORKFLOW-004, BR-AUDIT-005, BR-INTEGRATION-054/065
**Test plan**: [TEST_PLAN.md](TEST_PLAN.md) | **Related, out of scope**: [#2588](https://github.com/jordigilh/kubernaut/issues/2588) lock design, [#1039](https://github.com/jordigilh/kubernaut/issues/1039) missing-resource incident.

## Outcome and approved constraints

An RCA remediation target is eligible for workflow selection only when the target cluster supplies sufficient trustworthy evidence for its exact type. For ambiguous kinds, the LLM must call target-scoped `api_resources_list`, reason from investigation evidence, and supply the exact `apiVersion`; the harness independently verifies that exact GVK is registered. The bounded correction retry may call target-scoped discovery before resubmitting, but the harness verification is a separate step. Escalate if clarification or verification remains unresolved. Signal target and RCA target remain distinct. Named-object `NotFound` is not type absence and can describe either an incident or a stale alert; the LLM must determine the GVK from available evidence. Never fabricate a historical version for a missing object or silently replace a model choice with a preferred version.

The LLM may also use target-scoped `resources_list` where investigation evidence needs it; this is not a substitute for independent type-registration verification. It must obey existing resource, namespace and authorization limits, and its failure must not be reinterpreted as type absence.

Preferred discovery returns the server-preferred version **per group/resource**, not arbitrary ordering or all served versions. It is evidence for the LLM, not permission for the harness to fill or rewrite a missing/conflicting model value. A proposed non-preferred version absent from this output is inconclusive, not invalid; the independent exact-GVK verifier may use target-scoped `resources_list` without requiring the named object to exist. Do not infer a missing object's historical GVK. Do not change cluster selection, access controls, or add a dependency without approval. User has approved no backward-compatibility/mixed-version requirement for this change where applicable; do not use that to bypass security or alter unrelated contracts.

## Preflight evidence and blockers

- `cmd/kubernautagent/bootstrap.go` constructs the phase resolver from the local mapper; `health.go` initializes local client discovery. `investigator_gates.go` skips on discovery errors, auto-replaces unambiguous versions, accepts a nonempty retry as validated, and clears review; all are incompatible with the intended safety contract.
- `fleet_overlay.go` carries per-investigation target-scoped tools in context and fails closed on unavailable fleet overlays; this does not yet establish that `api_resources_list` is advertised, bridged and phase-available locally or remotely. **Never fall back to hub discovery for a remote target.** Determine production dispatch, schema, serialization, context and registry/suppression wiring before choosing changes.
- Upstream [PR 1395](https://github.com/containers/kubernetes-mcp-server/pull/1395) is merged. Local uncommitted `go.mod`/`go.sum` use v0.1.2; `test/infrastructure/fleet_e2e.go` pins its image, but this alone does not establish deployed/demo image rollout or tool visibility. Preserve existing user changes, including two RED tests; they were not run in this planning task.
- v0.1.2 `pkg/toolsets/core/resources.go` returns structured `{resources, warning?}`, where partial discovery warning begins exactly `some API groups could not be queried and are omitted from this list: `; tool errors begin `failed to list API resources: `. It silently removes configured denied GVKs, without a filtering indicator. The v0.1.2 handler filters Kind using a case-insensitive exact match; a resource name is not a Kind. Verify returned Kind and group exactly rather than relying on descriptions or untrusted output. A missing warning is not proof the list is unfiltered or complete. Determine how trustworthy completeness/denial provenance can be obtained **without bypassing access controls**; if unavailable, fail closed and escalate rather than infer absence or uniqueness.
- Workflow component arrays declare supported full GVKs, for example `apps/v1/Deployment`. Use the verified GVK in the component selector; do not introduce a separate API-version parameter requirement. `InjectTargetResourceParameters` already injects `TARGET_RESOURCE_API_VERSION` when nonempty; trace existing consumers before proposing any removal. Let workflows determine compatible handling rather than assume version conversion.
- Gate retry is currently submit-only (`submitOnlyRCATools`); the approved #2609 correction retry must instead expose only target-scoped discovery plus `submit_result`, with bounded tool execution before resubmission. Preserve unrelated human-review reasons, authoritative target identity and audit outcome; revalidate any changed retry target and do not let selection/finalization revive unsafe workflow fields.
- `RunWorkflowDiscoveryFromRCA` calls `autoResolveRCATargetAPIVersion` **before** `prescopeFleetOverlay`, using `inv.scopeResolver`; changing only the autonomous gate leaves a remote hub-mapper bypass. `mergeRCAContinuationTarget` can carry the validated marker when kind/name/version match even if an explicitly supplied namespace differs: bind validation provenance to the complete cluster/target identity, not just those fields. Verify the actual scope of the marker and invalidate/revalidate when identity changes.
- Interactive `kubernaut_select_workflow` is a separate selection/assembly path after discovery; it is not proven safe by testing `RunWorkflowDiscoveryFromRCA` alone. Trace persisted session snapshots, selected parameters, human selection and downstream approval/handoff so none can turn inconclusive or stale validation into an executable target. Preserve existing approval policy; any new override authority needs user approval.

## Decision gate — resolved before implementation

1. **Discovery trust / non-preferred validation**: a returned exact GVK can establish positive registration evidence; omitted entries cannot prove absence or uniqueness. The user approved human review when the LLM cannot clarify or the selected GVK cannot be verified. Preferred-version-only output cannot verify an absent non-preferred version; target-scoped `resources_list` may positively verify the exact served GVK and does not require the named object to exist. Do not add a dependency or bypass denied-resource policy.
2. **Production route**: extend the existing context-scoped local/fleet client pattern. Local and remote `api_resources_list`/`resources_list` calls are target-scoped; a remote overlay failure or missing tool never falls back to the hub. No new architecture or dependency is required.
3. **Object-present semantics**: the LLM must provide the valid exact GVK from available evidence. A missing object does not permit historical-version inference or preferred-version substitution. The workflow selector uses the verified full GVK.
4. **Retry semantics**: the correction retry exposes only target-scoped discovery tools and `submit_result`, with bounded execution; the submitted value is independently verified after the retry. Any tool error, partial warning, silent filtering uncertainty, malformed value, or target identity change is review/inconclusive.

Planning confidence: **95%**. The user has resolved the ambiguity sequence, exact-GVK requirement, inconclusive-discovery escalation, local/fleet approach, selector contract, and bounded retry behavior. Remaining implementation risk is limited to adapting existing tests and proving both local and remote transport wiring.

## TDD and delivery sequence (estimates for implementer)

1. **Discovery/spike, 2–4 hours**: inspect real local registry, gateway bridge, result formats and schema/catalog registration; verify image digest/config and denial visibility without cluster mutation. Decision gate is resolved; write DD only if implementation reveals a genuinely new architecture.
2. **RED, 4–8 hours**: implement test-plan P0 unit and production-entry integration tests first. Preserve/adapt `UT-KA-2588-001/002` without weakening their safety assertions; record actual failing output. Test existing conflicting expectations as in §15 of test plan.
3. **GREEN, 1–2 days**: minimally expose/route target-scoped discovery via the approved per-investigation mechanism, allow the bounded discovery correction retry, ensure independent exact-GVK validation, and propagate verified target/review through autonomous and interactive selection. No verified target => no workflow. Run all UT and wiring IT; CHECKPOINT W before calling GREEN complete.
4. **REFACTOR, 2–4 hours**: consolidate result parsing, exact kind/GVK matching, provenance and error handling; remove duplicate validation while preserving per-context isolation. No new types/components during refactor. Run `go build ./...`, compile-only `go test ./... -run=^$ -timeout=30s`, affected suites and lint.
5. **Verification, 0.5–2 days**: run applicable E2E in an authorized disposable environment only, never mutate the user's clusters. Capture scenario numerator/denominator and control evidence; record environment blockers rather than claim a pass. Run required build/lint/affected regression checks.

## Wiring manifest and CHECKPOINT W

The rows below are the stronger full-wiring manifest. Focused investigator-entry integration specs pass, but they do not replace production bootstrap, gateway, persistence and handoff proof for these rows.

| Path | Production entry / location | Observable exit | Required IT |
|---|---|---|---|
| Local tool discovery | KA bootstrap and investigator registry/phase dispatch | LLM tool result and independent verified RCA target | IT-KA-2609-001 |
| Fleet-scoped discovery | `prescopeFleetOverlay` → existing gateway bridge → investigator | selected spoke only, no hub fallback | IT-KA-2609-002 |
| Autonomous workflow | investigator RCA/gate → workflow selection and `InjectTargetResourceParameters` | registered compatible selected workflow + exact target parameters | IT-KA-2609-003 |
| Interactive discovery | existing interactive workflow discovery entry → same target-scoped gate | verified result or review, no unsafe selection | IT-KA-2609-004 |
| Failure and audit | discovery/gate → existing response/audit store | review/inconclusive and correlation-linked failure event | IT-KA-2609-005 |

All rows require production callers, real integration dispatch and passing IT. IT-004 also covers the separate interactive selection/session and approval handoff, not merely discovery. Exact entry points and both local/remote tool transports must be proven, not assumed. The current focused IT suite is not sufficient evidence for this stronger manifest. Verify authorized RBAC uses correct group/resource/namespace and does not expand permissions. Discovery itself uses non-resource API discovery URLs (`api_resources_list` declares unbounded RBAC metadata), not a namespaced resource permission; separately verify object lookup permissions where used. Exercise the configured gateway authentication and target identity, including rejected/expired credentials, without exposing tokens or assuming a particular token exchange mechanism.

## Risks, controls, rollback

Wrong-cluster/group execution is critical: fail closed, exact-match evidence, isolate contexts, test AC-6 and ASVS 4.0.3 V4.1.5. Partial or silently filtered discovery can mislead; only proven completeness permits an absence/uniqueness conclusion. Type registration is not named-object existence. TOCTOU can invalidate discovery before execution: recheck at the approved boundary or fail safely on changed evidence; never claim a discovery snapshot guarantees later state. Do not log secrets or raw sensitive resource contents. Audit existing/new events for SOC2 CC7.2 reconstruction and FedRAMP AU-2/AU-3 fields, correlation, cluster, evidence provenance, outcome and error details; AC-6 least privilege and SI-10 input validation require IT and E2E proof. CC8.1 concerns change management, not event completeness.

Rollback is an authorized revert of this issue's own committed changes or a deployment rollback, never discard unrelated work. Since old behavior is unsafe, rollback must disable automatic selection for affected targets or require manual review; do not reinstate silent hub fallback. Coordinate image/schema rollback with the owner; no mixed-version support is promised.

## Coverage and security evidence at PR #2612 head

The changed-production statement mapping against
`da031cc8232c1df5d9337d8f09ee63370fe14b97..639f69376e15cf34aa58ef9964ddc036fffc48d0`
keeps the full **1,155-statement** denominator: executable changed Go code,
excluding `_test.go`, `test/`, and `docs/`. The current profile is **958/1,155
(82.94%)**, up from **897/1,155 (77.66%)**. The narrower target-only subset is
not used as the gate. The current rerun covered every package containing changed
production Go code, including the new `pkg/kubernautagent/tools/k8s` specs.

The new tests provide lower-tier evidence for:

- exact target-scoped GVK discovery and namespace/cluster scope;
- malformed input, invalid GVK, partial warning, and resolver-error fail-closed behavior;
- bounded metadata-only output and selector propagation;
- exact-kind filtering and no inferred absence from incomplete discovery.

These tests support the existing BR-KA-212, BR-WORKFLOW-004, BR-AUDIT-005,
BR-INTEGRATION-054/065, FedRAMP AC-4/AC-6/SI-10, OWASP ASVS V4.1.5, AU-2/AU-3,
and SOC 2 CC7.2 mappings already recorded in the test plan. They do **not** add
evidence for real gateway credentials, rejected/expired sessions, full bootstrap
or gateway deployment wiring, audit persistence, or the two planned E2E journeys.
SOC 2 CC8.1 remains a change-management/process mapping only.
