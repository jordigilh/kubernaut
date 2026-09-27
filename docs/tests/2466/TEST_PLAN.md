# Test Plan: Minimize LLM-Facing Workflow Discovery Data

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2466-v1.0
**Feature**: Expose only workflow-selection information to the LLM and keep parameter schemas out of audit storage
**Created**: 2026-09-26
**Author**: Kubernaut Team
**Status**: Implemented and verified. Focused projection/audit UTs, live etcd-Catalog IT, post-fix focused E2E, build, and branch-delta lint pass. The E2E also proves raw remediation-ID audit minimization. The latest all-services `make test` rerun failed only in two timing-sensitive SignalProcessing shutdown tests under 12-way parallel load; those tests pass when isolated.
**Issue**: [#2466](https://github.com/jordigilh/kubernaut/issues/2466)

## 1. Purpose and boundaries

KA discovers `RemediationWorkflow` and `ActionType` data from Kubernetes CRDs in etcd. The full CRD-backed Catalog record must remain available to KA for validation, target injection, and execution handoff. Only the tool response shown to the LLM is projected:

- Step 2 returns candidate identity/version and the structured description, not execution metadata.
- Step 3 returns the structured description and operational parameter schema, excluding KA-managed target parameters.
- The full Step 3 response remains available to the LLM, but the persisted `aiagent.llm.tool_call` record stores an omission marker. The in-memory `aiagent.llm.request` audit event's message-history field is also projected to that marker before storage mapping; the current Data Storage request schema does not persist message history. Tool identity and arguments continue to correlate the tool-call event.

This is an output/audit projection, not a catalog-source or execution-snapshot change.

## 2. Authoritative requirements

- [BR-WORKFLOW-004](../../requirements/BR-WORKFLOW-004-workflow-schema-format.md): structured descriptions and parameter descriptions support LLM decision-making; dependencies are operator-provisioned.
- [DD-KA-017](../../architecture/decisions/DD-KA-017-three-step-workflow-discovery-integration.md): `get_workflow` is a read-only parameter-schema lookup.
- [DD-WORKFLOW-003](../../architecture/decisions/DD-WORKFLOW-003-parameterized-actions.md): KA-managed target parameters are not supplied by the LLM and are stripped from its schema.
- [DD-KA-006](../../architecture/decisions/DD-KA-006-remediation-target-in-rca.md): KA injects `TARGET_RESOURCE_NAME`, `TARGET_RESOURCE_KIND`, `TARGET_RESOURCE_NAMESPACE`, and, when known, `TARGET_RESOURCE_API_VERSION` from the authoritative RCA target.
- [DD-WE-006](../../architecture/decisions/DD-WE-006-schema-declared-dependencies.md): the LLM must not handle credentials; dependency configuration is an operator/runtime concern.
- [DD-AUDIT-009](../../architecture/decisions/DD-AUDIT-009-workflow-discovery-event-specific-payloads.md), BR-AUDIT-005, and #2459: parameter schemas and execution bundles are excluded from persisted audit payloads.
- Controls: FedRAMP AU-2/AU-3; SOC 2 CC7.2; OWASP ASVS v5.0.0 V16.2.1, V16.2.4, and V16.2.5.

## 3. Risks and mitigations

| Risk | Mitigation / proving test |
|---|---|
| KA-managed target parameters or their names leak into the LLM projection. | UT asserts all four canonical fields are absent and any `dependsOn` references to them are removed. |
| Execution bundles, engine configuration, infrastructure dependencies, service-account details, or raw CRD metadata reach the LLM. | UT and live-Catalog IT inspect complete serialized tool responses for excluded fields and the seeded metadata sentinel while proving operational parameter descriptions remain intact. The Data Storage IT/E2E separately prove the sentinel is absent from persisted audit payloads. |
| The projection accidentally removes useful LLM parameter guidance. | UT and IT assert operational parameter name/type/required/description and validation metadata remain available. |
| Audit minimization also truncates the response actually sent to the LLM. | Investigator UT proves the model message keeps the full permitted response while the tool-call audit result and in-memory request-event history field contain an omission marker only. |
| The full CRD is mutated or lost, breaking validation/execution handoff. | Projection tests verify the source model/raw parameter document remains unchanged; existing validation and execution tests remain regression gates. |

## 4. Test scenarios and TDD sequence

| ID | Tier | Business outcome | Verification |
|---|---|---|---|
| UT-KA-2466-001 | Unit | Step 2 candidate entries preserve identity, version, and structured description while excluding schema image, execution bundle/engine, and service account. | Execute `list_workflows` against a Catalog fake and inspect the complete JSON result. |
| UT-KA-2466-002 | Unit | Step 3 exposes only description plus valid operational parameter definitions. | Assert four KA-managed parameter definitions and their references are absent; allowed operational schema fields remain; no CRD metadata/content/execution/dependency fields or seeded metadata sentinel are returned; source model stays unchanged. |
| UT-KA-2466-003 | Unit | Audit minimization does not alter the tool result delivered back to the LLM or leak it through audit-event history data. | Exercise `Investigator.processToolCalls` and the subsequent LLM request audit event; assert the LLM tool message contains the full allowed Step 3 result, while the tool-call result and in-memory request-event history field contain only the omission marker. |
| UT-KA-2466-004 | Unit | Malformed parameter schemas never fall back to an unfiltered tool response. | Supply malformed JSON, missing schema/parameters, unnamed parameters, and invalid dependency shapes; assert `get_workflow` returns an observable error and no response content. |
| UT-KA-2466-005 | Unit | Audit minimization does not alter other tools' existing audit contract. | Exercise a non-workflow tool through `Investigator.processToolCalls`; assert both the LLM message and tool-call audit result/preview remain unchanged. |
| IT-KA-2466-001 | Integration | The real etcd-backed Catalog and registered `get_workflow` tool return the minimized selection projection. | Query a seeded workflow CRD through the live informer Catalog and inspect allowed operational parameters and excluded target/execution fields. |
| E2E-KA-2459-001 | E2E | Deployed three-step discovery still selects a workflow while raw remediation-ID Data Storage results contain no schema or seeded sentinel. | Extend the existing scenario to inspect the persisted generic `get_workflow` tool-call payload for the omission marker; continue asserting Step 1/2 typed audit reconstruction. |

### TDD phases

1. **RED**: Add UT assertions for the Step 2/Step 3 projection and for the separation between the LLM message and persisted tool-call result. Add the live-Catalog IT assertion and strengthen the existing E2E raw-record assertion.
2. **GREEN**: Build an explicit allowlisted tool-response projection without mutating Catalog records. At the existing `processToolCalls` audit emission point, replace only the audited `get_workflow` result/preview with a fixed omission marker; append the original result to the LLM message history unchanged. Apply the same audit projection when conversation history is placed in the in-memory `aiagent.llm.request` event and final/cancelled investigation audit snapshots.
3. **REFACTOR**: Keep the field projection and audit omission behavior named and localized; update the canonical-parameter documentation to match DD-KA-006's four managed values.

## 5. Wiring manifest

| Component | Production entry point | Wiring location | Integration proof |
|---|---|---|---|
| Step 2/3 LLM projection | Registered KA discovery tools | `cmd/kubernautagent/toolregistry.go` → `internal/kubernautagent/tools/custom/tools.go` → etcd-backed `workflowcatalog.Catalog` | IT-KA-2466-001; E2E-KA-2459-001 |
| Step 3 audit minimization | Investigator tool-call dispatch and audit-history serialization | `internal/kubernautagent/investigator/investigator_loop.go:processToolCalls`; `internal/kubernautagent/investigator/investigator_audit.go:messagesToAuditFormat` | UT-KA-2466-003; E2E-KA-2459-001 |

## 6. Completion criteria

- All new UTs and the live-Catalog IT pass; the focused E2E passes with the production buffered Data Storage audit path.
- The LLM still receives workflow descriptions and operational parameter schemas, but never receives KA-managed target parameters or execution/dependency internals.
- Persisted Step 3 tool-call records contain no parameter schema/result, execution metadata, or seeded sentinel. The in-memory LLM-request audit event history and persisted cancellation snapshots use the omission marker; the current Data Storage LLM-request schema does not persist message history. Other tools' audit-result behavior is unchanged.
- Catalog-backed records remain unchanged for KA's internal validation and downstream execution handoff.
- Affected Ginkgo suites, build, and lint pass; the earlier E2E failure status in the #2459 plan/DD is reconciled.

## 7. Execution record

| Gate | Status | Evidence |
|---|---|---|
| Projection and investigator UTs | PASS | `go test ./internal/kubernautagent/tools/custom ./internal/kubernautagent/investigator -count=1`; includes the LLM-vs-audit history assertion. |
| All-services unit target | PARTIAL | `make test` returned non-zero for two unrelated timing-sensitive assertions in `pkg/signalprocessing/controller_shutdown_test.go` under the 12-process aggregate run (`should allow in-progress operation to complete before exit`, `should enforce shutdown timeout`). `go test ./pkg/signalprocessing -ginkgo.focus='Controller Shutdown' -count=1` passes in isolation. |
| Live etcd-Catalog IT | PASS | `IT-KA-2466-001` queried seeded workflow CRDs through the live informer-backed Catalog and verified the LLM projection. |
| Focused E2E | PASS | `E2E-KA-2459-001` ran against deployed KA and real Data Storage; it asserted the omission marker, raw sentinel exclusion, and the three-step selection/reconstruction behavior. |
| Build | PASS | `go build ./...`. |
| Full test-package compilation | PASS | `go test ./... -run=^$ -timeout=30s`. |
| Branch-delta lint | PASS | `bin/golangci-lint run --new-from-rev=origin/main --timeout=5m` reports zero issues. |

**Verification note:** Local Podman capacity initially blocked image builds. After authorized cleanup of dangling images and the failed Buildah containers, the focused E2E reused already-built local images through `KUBERNAUT_CI_ARTIFACT_TAG`; the active Engram container, volumes, and tagged source images were preserved.
