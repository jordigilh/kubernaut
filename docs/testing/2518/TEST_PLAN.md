# Test Plan: Fleet MCP Enrichment Resilience

> IEEE 829-2008 hybrid test plan and validation record for GitHub issue #2518.
> The design/test contract was reviewed before implementation; this document
> now records implementation, wiring, and environment-gate evidence.

**Test Plan Identifier**: TP-2518-v1.0  
**Feature**: Trusted read-only MCP recovery, gateway-neutral binding refresh, and target-scoped evidence propagation for KA Fleet enrichment  
**Version**: 1.1  
**Created**: 2026-10-16  
**Last updated**: 2026-10-16  
**Author**: Kubernaut development team  
**Status**: Implementation complete — scoped validation recorded; environment and external release gates outstanding  
**Branch**: `issue-2518-triage`  

## 1. Introduction

### 1.1 Purpose

Define business-level UT, production-path IT, and separately gated external E2E
evidence for bounded MCP recovery and Fleet enrichment resilience. The plan
protects target scope, prevents mutation replay, preserves typed evidence and
audit reconstruction, and proves both EAIGW and Kuadrant adapter conformance
without placing gateway-specific behavior in the retry core.

### 1.2 Objectives and pass criteria

1. Prove all 15 defined business UT scenarios, with at least 90% scenario
   coverage and 100% coverage of critical safety, scope, integrity, status,
   audit, and malformed-input scenarios.
2. Prove all 12 normalized wiring rows through production KA entry points.
3. Prove trusted read-only replay only; every mutation and unknown tool is sent
   at most once, including the executor-factory path.
4. Prove typed, target-scoped evidence survives success, partial, failure, and
   cancellation terminal paths and never falls through to `WorkflowNotFound`.
5. Keep the real Agent Router/Envoy/extproc journey as a separate release and
   environment gate, not a silently waived local merge test.

## 2. Authority and control traceability

### 2.1 Governing documents

- Issue #2518 and canonical OpenChamber plan
- `DD-FLEET-009-gateway-neutral-mcp-read-recovery.md`
- `DD-FLEET-005-cluster-transparent-tool-exposure.md`
- `DD-KA-2511-generic-recurrent-rca-continuation.md`
- `BR-KA-016`, `BR-HAPI-261`, `BR-AUDIT-005`, `BR-KA-OBSERVABILITY-001`
- `BR-INTEGRATION-054`, `BR-INTEGRATION-065`, `BR-INTEGRATION-1489`
- `DD-AUDIT-003` and `ADR-034`

### 2.2 Control objectives

| Behavior under test | FedRAMP | SOC 2 | OWASP ASVS |
|---|---|---|---|
| Validated target/tool input and fail-closed unknown contracts | SI-10, AC-4, AC-6 | CC7.2 | V2.2.1–V2.3.1 |
| No mutation replay; target isolation and binding fencing | AC-4, AC-6 | CC7.2 | V2.2.2–V2.2.3 |
| Correlated, complete, redacted audit evidence | AU-2, AU-3 | CC7.2 | V16.2.x |
| Bounded retry, reset, exhaustion and metric observability | SI-10, AU-3 | CC7.2 | V16.2.x |
| TLS/authentication and retention | SC-8, AU-9, AU-11 (regression boundary only) | CC7.2 | V6/V13 (regression boundary only) |

## 3. Risks and non-negotiable constraints

| ID | Risk | Required proof |
|---|---|---|
| R1 | Ambiguous mutation is duplicated after reset | `UT-FLEET-2518-005`, `UT-FLEET-2518-015`, `IT-FLEET-2518-007` |
| R2 | Stale session or binding crosses target/generation | `UT-FLEET-2518-002`–`004`, `IT-KA-2317-001`, `IT-KA-FLEET-010`–`012`, `IT-FLEET-FMC-001` |
| R3 | Reconnect waiter cancellation cancels another caller or leaks a session | `UT-FLEET-2518-002`, `IT-KA-2315-001`, `IT-KA-2317-001` |
| R4 | Empty hash or unavailable history crosses the DataStorage boundary | `UT-KA-2518-008`–`009`, `IT-KA-1802-001`, `IT-KA-433-ENR-006`, `IT-KA-2496-AUDIT-006` |
| R5 | Old-target evidence replaces current-target evidence | `UT-KA-2518-010`, `IT-KA-2518-009` |
| R6 | Typed status is lost on failed/cancelled terminal paths | `UT-KA-2518-012`, `IT-AA-KA-2233-001`, `IT-KA-823-007` |
| R7 | Audit fields, event ID, or secrets are lost/leaked in generated APIs | `UT-KA-2518-013`, `IT-AUD-001`, `IT-AUD-002`, `IT-AUD-004`, `IT-KA-1818-002` |
| R8 | High-cardinality metrics or unobservable retry failures | `UT-KA-2518-014`, `IT-KA-FLEET-2518-012`, `IT-KA-OBS-006` |

Constraints: trusted read-only replay only; mutation `ReplayNever`; gateway-
neutral core; adapter-owned Kuadrant/EAIGW binding leases; target-scoped
evidence; typed terminal status; bounded metrics; generated artifacts; and
separate external Agent Router/Envoy E2E gating.

## 4. Scope

### 4.1 Included

- `pkg/fleet/mcpclient` classifier, recovery policy, provider generation/CAS,
  binding resolver/lease, BridgeTool and WriterClient behavior.
- Production `registerFleetTools`/`gatewayOverlayResolver` wiring.
- Enricher, DataStorage history guards, context-tool evidence scope, target
  aware merges, AgentSession/AIAnalysis terminal mapping.
- Audit catalog/emitter/store/OpenAPI/ogen/DataStorage round trip.
- Dependency-injected observer and bounded metrics bootstrap.
- SDK-backed reset fixture on the supported Streamable HTTP transport.
- EAIGW and Kuadrant adapter conformance fixtures, including Kuadrant lease
  coverage from `select_tools` through `ListTools` and `CallTool`.

### 4.2 Excluded or separately gated

- SDK fork or universal resume guarantee.
- Automatic replay of mutations, new service/table, new business retry count,
  or semantic history heuristics.
- External Agent Router/Envoy/extproc change or upgrade.
- Existing repository E2E lanes are required evidence, not a missing capability:
  `make test-e2e-fleetmetadatacache-kuadrant` and
  `make test-e2e-fleetmetadatacache-eaigw` run the FMC gateway-specific lanes,
  while `make test-e2e-fleet` runs the broader EAIGW full-pipeline lane. These
  targets provision their own Kind environments; they do not consume the
  `helios08` hub kubeconfig.
- The approximately 120-second Agent Router/Envoy/extproc E2E remains
  `E2E-FLEET-2518-001` only when it means the separate external Agent Router
  path not exercised by the repository FMC/full-pipeline lanes. It is a
  release/environment gate, not a silently waived local test.
- SC-8, AU-9, and AU-11 changes; these remain regression/prerequisite checks.

## 5. Test approach and environment

- UTs use Ginkgo/Gomega and real business logic; external dependencies may be
  isolated only at their boundary.
- ITs use Ginkgo/Gomega, production constructors and dispatch paths. The reset
  fixture uses `httptest` and the repository's real SDK transport; no sleeping
  for the external 120-second path and no direct unit-only wiring claims.
- E2E uses the existing Fleet gateway infrastructure and the repository `make`
  targets above. The FMC lanes exercise both Kuadrant and EAIGW with real
  Keycloak, kube-mcp-server, RFC8693 exchange, Valkey/FMC, dynamic registration,
  and cross-cluster isolation; the full Fleet lane exercises the EAIGW pipeline
  through the Kubernaut services and KA investigation path. Live EAIGW and
  Kuadrant evidence is recorded in the canonical plan. The current live result
  does not prove mutation safety, cancellation release, CAS fencing, or the
  unimplemented production overlay; those remain local merge gates.
- All tests assert observable business outcomes, not merely calls or line
  execution. Native `testing.T` is not used for business tests.

### 5.1 Retry contract under test

`MaxAttempts=3` including the initial call, `MaxElapsed=10s`, initial backoff
100ms, maximum backoff 2s, injectable deterministic jitter ±20%, and capped
`Retry-After` 5s. The effective deadline is the minimum of parent, caller/tool,
and policy deadlines and includes call, reconnect, lease refresh, close, and
sleep. Cancellation/deadline always wins. Only the explicit trusted read list
(`resources_get`, `resources_list`, plus separately registered context reads)
gets `ReplayRead`; all other and unknown tools get `ReplayNever`.

## 6. Business scenario matrix

### 6.1 Unit scenarios (15; target 15/15)

| ID | Business outcome | Controls |
|---|---|---|
| `UT-FLEET-2518-001` | SDK/wrapped reset, no-progress, timeout, 429 and 5xx actions retry; 4xx, MCP `IsError`, malformed response and cancellation do not | SI-10, CC7.2 |
| `UT-FLEET-2518-002` | Deadline composition, bounded retry-after/jitter, independent waiter cancellation, CAS and close fencing are safe | SI-10, CC7.2 |
| `UT-FLEET-2518-003` | Provider re-resolves a current session/binding and returns one logical read result | AC-4, AU-3 |
| `UT-FLEET-2518-004` | EAIGW and Kuadrant refresh bindings; Kuadrant selection/list/call is isolated by lease | AC-4, SI-10 |
| `UT-FLEET-2518-005` | Only trusted reads replay; mutation, approval, patch, delete, execution and unknown tools never duplicate | AC-6, ASVS V2.2.2–V2.2.3 |
| `UT-FLEET-2518-006` | Exhaustion produces bounded typed evidence with source/tool/cluster/attempt/correlation | AU-3, CC7.2 |
| `UT-FLEET-2518-007` | #2316/#2340 discovery and reader recovery regressions remain intact | AC-4, SI-10 |
| `UT-KA-2518-008` | Empty/whitespace hash causes zero DataStorage calls in adapter, enricher and context paths | SI-10, ASVS V2.2.1 |
| `UT-KA-2518-009` | Hash unavailable, valid empty, transport, and schema failures remain distinct | AU-3, CC7.2 |
| `UT-KA-2518-010` | Current-target history survives optional-source failure; old-target history is never substituted | AC-4, CC7.2 |
| `UT-KA-2518-011` | History escalation, RCA incomplete, transport unavailable and schema failure remain distinct | AU-3, CC7.2 |
| `UT-KA-2518-012` | Evidence status survives context/enricher, success/failure/cancel/partial paths and maps safely | AU-3, SI-10 |
| `UT-KA-2518-013` | Audit payload is typed, correlated, redacted, and has one canonical event ID | AU-2/AU-3, CC7.2 |
| `UT-KA-2518-014` | Injected metrics observer works with bounded labels and no secret/high-cardinality leakage | AU-3, SI-10, ASVS V16.2.x |
| `UT-FLEET-2518-015` | Failed mutations can be reconciled by a separate read without resending the mutation | AC-6, AC-4 |

### 6.2 Integration wiring scenarios (12 normalized rows)

The manifest uses the IDs that exist in the repository. Four new issue-specific
production-path tests are retained verbatim; the other rows reference the
existing Fleet/KA integration IDs that already exercise the surrounding
production entry points. The finer-grained retry, lease, classification, and
mapping contracts remain covered by the issue-specific UTs and focused package
suites; no legacy test is presented as proof of a behavior it does not assert.

| Manifest row | Production path and observable outcome | Production IT evidence |
|---|---|---|
| Read recovery | `registerFleetTools` → overlay → provider BridgeTool recovers a reset `resources_get` once logically | `IT-KA-FLEET-2518-001` |
| Provider bridge lifecycle | `registerFleetTools` retains a provider-backed resolver across initial and mid-lifetime reconnects | `IT-KA-1553-001`, `IT-KA-2315-001`, `IT-KA-2317-001` |
| Binding lease/generation | EAIGW/Kuadrant discovery and target-scoped reader paths preserve the selected cluster | `IT-KA-FLEET-010`, `IT-KA-FLEET-011`, `IT-KA-FLEET-012`, `IT-FLEET-FMC-001` |
| Read allow-list | Generic pre-scoped tools route to the intended wire tool and executor mutations use the non-replay path | `IT-FLEET-001`, `IT-FLEET-004`, `IT-FLEET-2518-007` |
| Target overlay reads | Investigator/context enrichment uses the target overlay and does not fall back to the hub | `IT-KA-FLEET-032`, `IT-KA-FLEET-033`, `IT-KA-FLEET-036`, `IT-KA-FLEET-2417`, `IT-KA-MCP-LIST-001` |
| List reset/schema validation | Production list parsing handles structured/list responses and rejects unsupported or malformed shapes | `IT-FLEET-PARSE-001`, `IT-FLEET-PARSE-003`, `IT-FLEET-PARSE-004`, `IT-KA-MCP-LIST-001` |
| Mutation non-replay | Executor factory → WriterClient sends one mutation wire call; reconciliation is a separate read | `IT-FLEET-2518-007` |
| Hash/source guard | Enrichment/history production paths preserve cluster scope, empty-history semantics, and failure audit outcomes | `IT-KA-1802-001`, `IT-KA-433-ENR-003`, `IT-KA-433-ENR-004`, `IT-KA-433-ENR-006`, `IT-KA-2496-AUDIT-006` |
| Target merge | Recurrent RCA retains ordered current-target history through later optional-source failure | `IT-KA-2518-009` |
| Evidence terminal mapping | AgentSession dispatch reaches completed/failed terminal paths without losing the result contract | `IT-AA-KA-2233-001`, `IT-KA-823-007` plus focused AgentSession/AIAnalysis handler suites |
| Audit/generated artifacts | Audit lifecycle and enrichment events round-trip through the production store/query boundary | `IT-AUD-001`, `IT-AUD-002`, `IT-AUD-004`, `IT-KA-433-ENR-003`, `IT-KA-433-ENR-004`, `IT-KA-1818-002` |
| Metrics/bootstrap | Metrics are constructed before Fleet tools and the production BridgeTool uses the injected bounded observer | `IT-KA-FLEET-2518-012`, `IT-KA-OBS-006` |

### 6.3 External E2E gate

`E2E-FLEET-2518-001` runs only when the provisioned Agent Router/Envoy/extproc
lane exists. It verifies the real reset journey, one logical LLM result, three
ordered recurrent RRs, no duplicate mutation, valid hash/status classification,
and correlation-reconstructable evidence. The result must record transport,
router/extproc versions and configuration. It is not silently waived or counted
as a local merge pass.

### 6.4 CI validation evidence

The PR-validation workflow `.github/workflows/ci-pipeline.yml` includes
`fleetmetadatacache-kuadrant`, `fleetmetadatacache-eaigw`, and `fleet` in its
E2E matrix. Each matrix entry executes `make test-e2e-${{ matrix.service }}`;
the aggregate `e2e-tests` job is required by the repository's `Merge Gate`.
This proves the lanes are CI merge-gate inputs, not merely developer-local
commands. The latest merged PR #2513 (head `3ed5e433`, merge commit
`ebfcfe18`) passed CI run `38919125785`: `E2E (fleet)`,
`E2E (fleetmetadatacache-kuadrant)`, `E2E (fleetmetadatacache-eaigw)`,
`E2E (fullpipeline)`, `Test Suite Summary`, and `Merge Gate` all completed
successfully. This is recorded evidence that the repository's gateway and
Fleet E2E lanes are operational PR-validation gates. For issue #2518, record
the corresponding run/attempt and individual FMC/Fleet job results when the
implementation branch is validated.

## 7. Wiring manifest and checkpoint W

| Component | Production entry point | Normalized production IT evidence |
|---|---|---|
| Read recovery | `BridgeTool.Execute`, `gatewayOverlayResolver.Overlay` | `IT-KA-FLEET-2518-001` |
| Provider bridge lifecycle | `registerFleetTools` / overlay construction | `IT-KA-1553-001`, `IT-KA-2315-001`, `IT-KA-2317-001` |
| Gateway-neutral binding lease | discovery resolver and EAIGW/Kuadrant adapters | `IT-KA-FLEET-010`, `IT-KA-FLEET-011`, `IT-KA-FLEET-012`, `IT-FLEET-FMC-001` |
| Read allow-list | BridgeTool and genericNameTool | `IT-FLEET-001`, `IT-FLEET-004`, `IT-FLEET-2518-007` |
| Target overlay reads | overlay reader and history fetch | `IT-KA-FLEET-032`, `IT-KA-FLEET-033`, `IT-KA-FLEET-036`, `IT-KA-FLEET-2417`, `IT-KA-MCP-LIST-001` |
| List reset/schema validation | production overlay reader | `IT-FLEET-PARSE-001`, `IT-FLEET-PARSE-003`, `IT-FLEET-PARSE-004`, `IT-KA-MCP-LIST-001` |
| Mutation non-replay | WriterClient and `client_factory.go:138-180` | `IT-FLEET-2518-007` |
| Hash/source guard | DSAdapter, Enricher and context history | `IT-KA-1802-001`, `IT-KA-433-ENR-003`, `IT-KA-433-ENR-004`, `IT-KA-433-ENR-006`, `IT-KA-2496-AUDIT-006` |
| Target merge | RCA re-enrichment and continuation | `IT-KA-2518-009` |
| Evidence terminal mapping | session manager/status writer/mapping/response processor | `IT-AA-KA-2233-001`, `IT-KA-823-007` plus focused AgentSession/AIAnalysis handler suites |
| Audit/generated artifacts | catalog/emitter/store/OpenAPI/ogen/DataStorage | `IT-AUD-001`, `IT-AUD-002`, `IT-AUD-004`, `IT-KA-433-ENR-003`, `IT-KA-433-ENR-004`, `IT-KA-1818-002` |
| Metrics/bootstrap | `NewMetrics` before fleet construction | `IT-KA-FLEET-2518-012`, `IT-KA-OBS-006` |

Checkpoint W is satisfied by the production callers above and the passing
legacy/new IT evidence. Direct UTs do not replace wiring proof; they prove the
bounded contracts that the production ITs dispatch. The external FMC/Fleet E2E
lanes remain separate release gates and are not silently counted as local ITs.

## 8. Suspension, dependencies, and execution order

Suspend RED/GREEN if the formal decision is unresolved, a P1 design finding
appears, generated API contracts cannot be regenerated, or the supported SDK
transport cannot produce a deterministic reset. Suspend only the separate
external Agent Router/Envoy/extproc E2E when that environment is unavailable;
record it as blocked, never passed. The repository's FMC and Fleet E2E lanes
remain required CI evidence.

1. The design decision and test-plan review completed; `DD-FLEET-009` is
   adopted and indexed.
2. RED/GREEN/REFACTOR implementation and focused proving tests are complete.
3. Validate focused suites, generated artifacts, build, lint, and required
   gateway/Fleet lanes; record infrastructure and external release blockers
   separately rather than treating them as product passes.

Known baseline dependency: investigator integration setup requires
`/usr/local/kubebuilder/bin/etcd`; its absence blocks that environment suite,
not the product baseline. Existing focused suites and build pass on this branch.

## 9. Deliverables

- This plan and its BR/control matrix.
- `DD-FLEET-009-gateway-neutral-mcp-read-recovery.md` and index entry.
- Ginkgo/Gomega UT and IT suites with the IDs above.
- Generated CRD/OpenAPI/ogen artifacts where typed status/audit fields change.
- Local merge evidence and a separately recorded external E2E result.

## 10. Current validation evidence

| Validation | Result | Evidence / limitation |
|---|---|---|
| Aggregate unit target | **PASS** | `make test`; all 25 Ginkgo unit suites passed. |
| Focused unit and production suites | **PASS** | `go test ./pkg/fleet/mcpclient ./internal/kubernautagent/enrichment ./internal/kubernautagent/investigator ./internal/kubernautagent/session ./internal/kubernautagent/agentsession ./internal/kubernautagent/audit ./internal/kubernautagent/metrics ./pkg/workflowexecution/executor ./cmd/kubernautagent ./test/integration/kubernautagent/fleet`; the cmd suite ran 157/157 and investigator 474/474. |
| Integration compile check | **PASS** | `go test -run '^$' ./test/integration/kubernautagent/investigator`. |
| Build/lint/generation/diff gates | **PASS** | `go build ./...`, `make lint` (0 issues), `make manifests`, `make generate`, and `git diff --check` all passed on the final source state. |
| Full KA integration target | **BLOCKED** | `make test-integration-kubernautagent` ran 18 suites; `enrichment`, `investigator`, `mcp`, and `tools/custom` could not start envtest because `/usr/local/kubebuilder/bin/etcd` is absent. Other suites passed; no product assertion was attributed to the missing binary. |
| FMC Kuadrant E2E | **BLOCKED** | `make test-e2e-fleetmetadatacache-kuadrant` failed before specs while building the UBI image: Podman reported `No space left on device` during the DNF transaction; 0/16 specs ran. |
| FMC EAIGW E2E | **PASS** | `make test-e2e-fleetmetadatacache-eaigw`: 16/16 specs passed, including discovery, RFC8693, target isolation, dynamic registration, TLS, least privilege, and Valkey recovery; Kind cleanup completed. |
| Broader Fleet E2E | **NOT COMPLETED** | `make test-e2e-fleet` was interrupted by the execution harness before a result was returned; no pass is claimed. No Kind cluster remained afterward. |
| External Agent Router/Envoy/extproc E2E | **SEPARATE GATE** | Not run in this checkout; remains `E2E-FLEET-2518-001` and requires the provisioned external environment. |

The final local build, lint, generation, and diff checks are recorded in the
implementation/PR validation notes after they are rerun on the final commit
state. Environment-blocked rows remain open release gates; they are not
silently converted into passes.

## 11. Approval and release-gate status

The implementation and deterministic local wiring gates are ready for review.
The missing envtest `etcd` binary, Podman storage exhaustion for the Kuadrant
lane, the incomplete broader Fleet run, and the separate Agent Router/Envoy
journey remain explicitly tracked environment/release gates.
