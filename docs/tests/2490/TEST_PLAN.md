# Test Plan: Issue #2490 — Complete Chained Pre/Post Spec-Hash History

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2490-v1.1
**Feature**: Preserve the complete target-scoped remediation chain across linked pre/post spec hashes
**Version**: 1.1
**Created**: 2026-10-07
**Author**: Kubernaut Team
**Status**: Implementation and blocker-remediation validation complete; live validation pending
**Branch**: `fix/2490-remediation-history`

---

## 1. Purpose and Business Outcome

Issue [#2490](https://github.com/jordigilh/kubernaut/issues/2490) fixes the
remediation-history path truncating a causal chain after the latest remediation
whose Effectiveness Monitor post-hash matches the current resource hash. The
DataStorage query must expose every earlier linked remediation so the
Remediation Orchestrator and Kubernaut Agent can distinguish a first attempt
from recurrence and assess durability without inventing a retry threshold.

The approved implementation is **Option A: a PostgreSQL recursive CTE** inside
the existing `QueryROEventsBySpecHash` contract. The CTE preserves the #616
dual-hash anchor, follows only causal EM post-hash edges backward in time,
scopes every hop by target and optional cluster, deduplicates event identities,
and retains the existing time and result bounds. The repository also applies a
256-hop depth guard, a 10-second query-local deadline, and a one-row overflow
sentinel; resource exhaustion fails closed instead of returning partial history.
Tier 2 performs the broad lookback needed for bridge traversal and partitions
the response into disjoint Tier 1 and Tier 2 windows.

## 2. Objectives and Success Metrics

1. A three-remediation forward chain queried by its current post-hash returns
   all three rows in deterministic oldest-to-newest order.
2. A Tier 1 remediation can bridge discovery of an older Tier 2 remediation,
   while no row appears in both response windows.
3. Same-hash siblings, future rows, malformed links, cycles, other targets,
   and other clusters cannot contaminate the causal chain.
4. Effectiveness hash events remain time-unbounded relative to the RO query
   window (F1 regression protection).
5. Prompt output describes recurrence and durability qualitatively; one
   completed remediation is not labeled recurrence and no arbitrary retry
   threshold or Job policy is introduced.
6. Existing #616, #1802, F1, RO, KA, and prompt regression suites remain green.
7. A depth, timeout, or result-overflow guard never returns partial history as a
   successful response; the HTTP contract is RFC 7807 `503 Service Unavailable`.
8. Audit-derived recurrence fields are sanitized before prompt interpolation.

| Metric | Target | Measurement |
|---|---:|---|
| P0 test pass rate | 100% | Focused unit/integration suites |
| Wiring manifest coverage | 100% | Production callers exercised by integration tests |
| Regression count | 0 | Existing #616/#1802/F1 and prompt suites |
| Complete-chain behavior | 3/3 linked rows | `IT-DS-2490-001` |
| Tier partition behavior | Disjoint bridge windows | `IT-DS-2490-002` |
| Resource safety | Overflow fails closed with no partial response | `IT-DS-2490-RESOURCE-001`, `UT-DS-2490-RESOURCE-002` |

## 3. Authority, Requirements, and Controls

### 3.1 Governing documents

- [BR-ORCH-042.5](../../requirements/BR-ORCH-042-consecutive-failure-blocking.md): ineffective remediation chain detection.
- **BR-KA-016**: remediation history context for LLM prompt enrichment.
- **BR-INS-001 / BR-INS-002**: effectiveness assessment and correlation consumed by the history context.
- [DD-KA-016](../../architecture/decisions/DD-KA-016-remediation-history-context.md): two-tier history architecture, amended to v1.6 for #2490.
- Issue [#2490](https://github.com/jordigilh/kubernaut/issues/2490).
- Issue #616: dual pre/post-hash anchor matching.
- Issue #1802: target-resource and optional cluster isolation.
- DataStorage due-diligence F1: time-unbounded EM correlation across tier boundaries.

### 3.2 Control-objective mapping

| Requirement / objective | Evidence | Control mapping |
|---|---|---|
| Complete remediation reconstruction by correlation and causal chain | `IT-DS-2490-001`, `IT-DS-F1-001` | FedRAMP AU-3, AU-9; SOC 2 CC7.2 |
| Target/cluster information-flow isolation on every recursive hop | `IT-DS-2490-003`, existing #1802 tests | FedRAMP AC-4, AC-6; OWASP ASVS V5.1, V5.5.2 |
| Malformed/temporal-cycle/branching input terminates safely and remains bounded | `IT-DS-2490-004`, `IT-DS-2490-RESOURCE-001` | FedRAMP SI-10, SC-5; OWASP API4:2023, ASVS V5.5.2, V7.1.1 |
| Complete history is not silently truncated when a safety budget is exceeded | `IT-DS-2490-RESOURCE-001`, `UT-DS-2490-RESOURCE-001`, `UT-DS-2490-RESOURCE-002`, `UT-DS-2490-RESOURCE-003` | FedRAMP AU-3, SC-5; SOC 2 CC7.2; OWASP API4:2023 |
| Effectiveness evidence remains attributable and queryable | Existing audit schema tests and `IT-DS-F1-001` | FedRAMP AU-2, AU-3; SOC 2 CC7.2 |
| Prompt does not turn history into an arbitrary retry policy | `UT-KA-2490-001`, `UT-KA-2490-002`, prompt builder tests | BR-INS-001/002; SOC 2 CC8.1 change evidence |
| Audit-derived history fields cannot inject workflow-selection instructions | `UT-KA-2490-SEC-001` | DD-KA-005; FedRAMP SI-10, AC-4; OWASP LLM01:2025 |

## 4. Risks and Mitigations

| ID | Risk | Impact | Affected tests | Mitigation |
|---|---|---|---|---|
| R1 | Recursive SQL follows a same-pre-hash sibling instead of a causal post-hash edge | False recurrence and incorrect RO/LLM context | `IT-DS-2490-003` | Recursive term joins only on candidate EM post-hash = current RO pre-hash and requires an older RO timestamp |
| R2 | A malformed link, temporal cycle, or high-fan-out branch causes unbounded work | Database saturation or incomplete history | `IT-DS-2490-004`, `IT-DS-2490-RESOURCE-001` | Strictly earlier timestamps, recursive `UNION`, final event-identity deduplication, 256-hop depth cap, 10s query deadline, max+1 overflow sentinel, fail-closed 503 |
| R3 | Tier 2 query stops at the Tier 1 boundary and misses an older link | Incomplete historical context | `IT-DS-2490-002` | Query full Tier 2 lookback through now, then filter the response window |
| R4 | EM assessment timestamp falls outside the RO tier | False negative post-hash anchor | `IT-DS-F1-001` | Keep EM correlation lookup time-unbounded while the RO rows remain window-bounded |
| R5 | Cross-target or cross-cluster identical hashes contaminate history | Incorrect ineffective-chain blocking or prompt bias | `IT-DS-2490-003`, existing #1802 tests | Apply target and optional cluster predicates to anchor and recursive candidates |
| R6 | Prompt guidance implies successful assessment proves durable resolution | Unsafe workflow selection | `UT-KA-2490-001`, `UT-KA-2490-002` | State qualitative assessment-window semantics and require RCA/linked-history reasoning |
| R7 | Audit-derived action/signal text injects instructions into the workflow-selection prompt | Incorrect or unsafe workflow selection | `UT-KA-2490-SEC-001` | Apply the existing `sanitizeField` injection-pattern filter before interpolation |

## 5. Scope and Design Decisions

### 5.1 In scope

- `pkg/datastorage/repository/remediation_history_repository.go` recursive
  CTE and bounded causal traversal.
- `pkg/datastorage/server/remediation_history_handler.go` full-lookback Tier 2
  bridge query and disjoint response partitioning.
- `internal/kubernautagent/prompt/history.go` and prompt templates for
  qualitative recurrence/durability guidance.
- DataStorage integration coverage using real PostgreSQL.
- Unit coverage for pure prompt and window-partition behavior.
- Documentation amendment to DD-KA-016.
- OpenAPI and generated-client coverage for the fail-closed `503` response.

### 5.2 Out of scope

- Changing Effectiveness Monitor scoring or audit event schemas.
- Adding recurrence/escalation thresholds to the prompt or workflow Job.
- Moving recurrence decisions into workflow implementations.
- Replacing the existing `QueryROEventsBySpecHash` contract or adding a new
  database round-trip per hash hop.
- Demo-repository ownership of the production fix; image publication/PR
  ownership is release follow-up after code validation.

### 5.3 Approved architecture

| Decision | Choice | Rationale |
|---|---|---|
| Traversal | PostgreSQL recursive CTE | One DB round trip, preserves existing repository contract and bounds |
| Anchor | RO pre-hash OR correlated EM post-hash | Preserves #616 behavior |
| Recursive edge | Candidate EM post-hash equals current node RO pre-hash | Represents the forward remediation transition being walked backward |
| Temporal direction | Candidate RO timestamp strictly earlier than current node | Prevents future/sibling links and guarantees backward traversal |
| Deduplication | Recursive `UNION` plus final event-identity deduplication | Removes duplicate recursive states and returned event identities; strict earlier timestamps make traversal temporally acyclic |
| Resource guard | Depth 256, query-local 10s deadline, max+1 result sentinel | Prevents unbounded work and rejects incomplete history rather than returning a partial prefix |
| Prompt safety | Existing `sanitizeField` injection-pattern filter | Keeps audit-derived recurrence values from becoming workflow-selection instructions |
| Tier 2 | Full lookback query, then response filtering | Allows a recent bridge to reach older history while keeping windows disjoint |

## 6. TDD Phases and Wiring Manifest

### 6.1 RED

- Add the DataStorage integration scenarios below before refining the
  recursive predicate.
- Add failing safety tests for depth/result overflow and the fail-closed HTTP
  response, plus a prompt-injection sanitization test.
- Add/retain prompt unit scenarios for durability and recurrence semantics.
- Record the initial failing result; infrastructure failures are reported
  separately from assertion failures.

### 6.2 GREEN

- Refine the existing CTE to carry the current node's pre-hash and timestamp.
- Restrict recursion to earlier candidates whose correlated EM post-hash equals
  that pre-hash; preserve target/cluster/window predicates and result cap.
- Add the 256-hop/10-second/max+1 guards and return resource-limit failures
  through RFC 7807 `503` instead of partial `200` history.
- Keep Tier 2's full-lookback traversal and explicit response filtering.
- Wire qualitative prompt guidance and sanitized audit-derived fields through the
  existing prompt builder/templates.

### 6.3 REFACTOR

- Update DD-KA-016 to v1.6 and align stale comments with the final traversal.
- Make event identity and recursive-column intent explicit without introducing
  a new component.
- Run build, lint, targeted tests, and the mandatory post-refactor validation.

### 6.4 Wiring manifest

| Component | Production entry point | Wiring location | Proof |
|---|---|---|---|
| Complete causal RO chain | DS repository query used by Tier 1/Tier 2 | `pkg/datastorage/repository/remediation_history_repository.go:QueryROEventsBySpecHash` | `IT-DS-2490-001`, `IT-DS-2490-003`, `IT-DS-2490-004` |
| Complete-chain resource guard | DS repository and HTTP error mapping | `pkg/datastorage/repository/remediation_history_repository.go` and `pkg/datastorage/server/remediation_history_handler.go` | `IT-DS-2490-RESOURCE-001`, `UT-DS-2490-RESOURCE-001`, `UT-DS-2490-RESOURCE-002` |
| Fail-closed API contract | OpenAPI source, embedded copies, and generated client | `api/openapi/data-storage-v1.yaml`, `pkg/datastorage/server/middleware/openapi_spec_data.yaml`, `pkg/datastorage/ogen-client/` | `go generate ./pkg/datastorage/server/middleware/... ./pkg/audit/... ./pkg/datastorage/ogen-client/...` |
| Tier 2 bridge partition | History HTTP handler and RO/KA adapters | `pkg/datastorage/server/remediation_history_handler.go:queryTier2History` | `IT-DS-2490-002` |
| Qualitative history interpretation | KA workflow-selection prompt rendering | `internal/kubernautagent/prompt/builder.go:RenderWorkflowSelection` | `UT-KA-2490-001`, `UT-KA-2490-002`, `UT-KA-2490-003` |

## 7. BR Coverage Matrix

| Requirement | Business outcome | Tier | Test ID | Status |
|---|---|---|---|---|
| BR-ORCH-042.5 / #2490 | Current post-hash exposes the complete linked chain | Integration | `IT-DS-2490-001` | PASS |
| BR-KA-016 / #2490 | Tier 2 bridge discovers older linked history without overlap | Integration | `IT-DS-2490-002` | PASS |
| #616 + #1802 + #2490 | Only causal, prior, target/cluster-scoped rows are returned | Integration | `IT-DS-2490-003` | PASS |
| #2490 safety | Strict temporal traversal excludes a temporal cycle and malformed links | Integration | `IT-DS-2490-004` | PASS |
| #2490 safety | Large branching overflow fails closed instead of returning a partial prefix | Integration | `IT-DS-2490-RESOURCE-001` | PASS |
| BR-ORCH-042.5 / #1802 | Recursive candidates remain isolated by requested cluster | Integration | `IT-DS-2490-005` | PASS |
| F1 / BR-KA-016 | EM post-hash remains discoverable outside RO tier window | Integration | `IT-DS-F1-001` | PASS (regression) |
| BR-KA-016 / BR-INS-001/002 | Two linked completed entries produce qualitative recurrence/durability context | Unit | `UT-KA-2490-001` | PASS |
| BR-KA-016 / BR-INS-001/002 | One completed entry is not labeled recurrence | Unit | `UT-KA-2490-002` | PASS |
| BR-KA-016 / #2490 | Tier 2 bridge partition is disjoint at the handler boundary | Unit | `UT-DS-2490-004` | PASS |
| BR-KA-016 / #2490 | Repository resource limits produce a typed error and HTTP 503, never partial history | Unit | `UT-DS-2490-RESOURCE-001`, `UT-DS-2490-RESOURCE-002` | PASS |
| DD-KA-005 / #2490 | Audit-derived recurrence fields are sanitized before prompt interpolation | Unit | `UT-KA-2490-SEC-001` | PASS |

## 8. Test Scenarios

### Tier 1: Unit

| ID | Business outcome | File | Phase |
|---|---|---|---|
| `UT-KA-2490-001` | A resolved/`Remediated` assessment is framed as assessment-window evidence, not durable root-cause proof; linked repetition is qualitative recurrence evidence | `internal/kubernautagent/prompt/history_test.go` | PASS |
| `UT-KA-2490-002` | A single completed remediation produces durability guidance but no recurrence label or numeric retry policy | `internal/kubernautagent/prompt/history_test.go` | PASS |
| `UT-KA-2490-003` | Workflow-selection rendering carries the same qualitative recurrence/durability guidance into the production prompt entry point | `internal/kubernautagent/prompt/builder_test.go` | PASS |
| `UT-DS-2490-004` | Handler queries Tier 2 through `now` for bridge traversal and partitions recent rows out of the historical response | `pkg/datastorage/remediation_history_handler_test.go` | PASS |
| `UT-DS-2490-RESOURCE-001` | Repository rejects a result set beyond the complete-history cap instead of returning rows | `pkg/datastorage/remediation_history_repository_test.go` | PASS |
| `UT-DS-2490-RESOURCE-003` | Repository rejects a row at the traversal-depth boundary instead of returning a depth-limited prefix | `pkg/datastorage/remediation_history_repository_test.go` | PASS |
| `UT-DS-2490-RESOURCE-002` | Handler maps a repository resource-limit error to RFC 7807 `503 Service Unavailable` | `pkg/datastorage/remediation_history_handler_test.go` | PASS |
| `UT-KA-2490-SEC-001` | Malicious audit-derived action/signal values are sanitized before recurrence guidance is rendered | `internal/kubernautagent/prompt/history_test.go` | PASS |

### Tier 2: Integration

| ID | Business outcome | File | Phase |
|---|---|---|---|
| `IT-DS-2490-001` | Three forward changes `H0→H1→H2→H3` return R1, R2, R3 for `currentSpecHash=H3`, oldest first | `test/integration/datastorage/remediation_history_query_fix_integration_test.go` | PASS |
| `IT-DS-2490-002` | A recent Tier 1 bridge discovers an older Tier 2 row; response chains are disjoint | same as above | PASS |
| `IT-DS-2490-003` | Same-pre-hash sibling, future link, malformed link, other target, and other cluster are excluded | same as above | PASS |
| `IT-DS-2490-004` | A temporal A↔B fixture proves recursive edges only move to strictly earlier RO timestamps and terminates safely | same as above | PASS |
| `IT-DS-2490-005` | A same-target chain from another cluster, including mismatched EM cluster edges, is excluded during recursive traversal | same as above | PASS |
| `IT-DS-2490-RESOURCE-001` | A large branching chain exceeds the complete-result budget and returns a resource-limit error instead of a partial prefix | same as above | PASS |
| `IT-DS-F1-001` | EM post-hash correlation outside the RO query window still matches | `test/integration/datastorage/ds_due_diligence_integration_test.go` | PASS (regression) |

### Tier 3: E2E

The existing DataStorage and KA E2E suites remain the production-stack
regression gate. A new E2E scenario is not required to prove SQL recursion
because the real PostgreSQL integration tests exercise the repository and the
existing E2E tests exercise the deployed endpoint. A deployable image/tag and
live recurrence validation remain release deliverables after implementation.

### Tier skip rationale

- **New E2E scenario**: deferred because the behavior is fully proven at the
  real-PostgreSQL repository boundary and existing deployed endpoint tests
  cover service wiring. The release validation must still run the existing E2E
  history journey against the published image.

## 9. Pass/Fail and Environment

**PASS** requires all P0 scenarios to pass, existing #616/#1802/F1 tests to
remain green, the wiring manifest to be complete, resource exhaustion to fail
closed with RFC 7807 `503` (never partial `200` history), and no prompt text to
add an arbitrary retry threshold or Job-level recurrence policy.

**FAIL** includes any incomplete chain, overlapping windows, cross-target leak,
unbounded/cyclic query behavior, silent result truncation, a missing resource
guard, F1 regression, unsanitized audit-derived prompt text, or prompt guidance
that treats one successful assessment as durable resolution.

Required validation:

```bash
gofmt -w pkg/datastorage/repository/remediation_history_repository.go \
  pkg/datastorage/server/remediation_history_handler.go \
  internal/kubernautagent/prompt/history.go \
  internal/kubernautagent/prompt/history_test.go \
  test/integration/datastorage/remediation_history_query_fix_integration_test.go
go test ./pkg/datastorage/... ./internal/kubernautagent/prompt/...
go test ./test/integration/datastorage -run '^$'
go build ./...
golangci-lint run --timeout=5m
make test
```

Real PostgreSQL is required for `test/integration/datastorage`; the KA
envtest/E2E prerequisite is currently unavailable when
`/usr/local/kubebuilder/bin/etcd` is absent and must be reported, not masked.

## 10. Deliverables and Traceability

| Deliverable | Location | Status |
|---|---|---|
| Test plan | `docs/tests/2490/TEST_PLAN.md` | Complete for implementation; live validation pending |
| Recursive traversal | `pkg/datastorage/repository/remediation_history_repository.go` | Implemented and validated |
| Tier bridge partition | `pkg/datastorage/server/remediation_history_handler.go` | Implemented and validated |
| Prompt guidance | `internal/kubernautagent/prompt/` | Implemented and validated |
| DD amendment | `docs/architecture/decisions/DD-KA-016-remediation-history-context.md` | Complete |
| Upstream PR/image digest/ownership | PR [#2491](https://github.com/jordigilh/kubernaut/pull/2491) and image evidence below | Published; live validation pending |

## 11. Release Image and Ownership Evidence

The deployable Kubernaut Agent production image is published to the upstream
Kubernaut Quay organization for live `operator-oomkill-informer` validation:

- **Image**: `quay.io/kubernaut-ai/kubernautagent:pr-2491-edf54ee70`
- **Immutable OCI index digest**: `sha256:318c5563454ae23fd79edacbc9f3537b9812dc967414a3ca64df1a1ea6a569dd`
- **Platforms**: `linux/amd64`, `linux/arm64/v8`
- **amd64 child manifest**: `sha256:bb42137252114aa2b4e8bd172d423e56727043899bf71ff614f62e91c1f9ae12`
- **arm64 child manifest**: `sha256:3c10b68035cc9146d62f05b7e30f6b5a183c4a73604f1bbf3f8d92a7b211beef`
- **Source**: PR #2491, commit `edf54ee70`
- **Runtime**: production scratch image; consume the index by digest rather than
  relying on a mutable tag.

**Ownership boundary:** upstream Kubernaut maintainers own the production fix,
PR, and `quay.io/kubernaut-ai` image publication. The
`operator-oomkill-informer` demo/validation owner consumes the digest for live
scenario validation; that validation does not transfer production code or image
ownership to the demo repository.

Both child images were cross-compiled with `CGO_ENABLED=0` from commit
`edf54ee70` and assembled with the production runtime-only Dockerfile; both are
scratch production runtimes and were verified in the published OCI index.

Implementation confidence: **96%**. The approved architecture is implemented
and verified with real PostgreSQL chain, bridge, strict-temporal-cycle,
large-branching-overflow, target/cluster, #616, and F1 scenarios plus
DataStorage/RO/KA package tests. Remaining release risks are the
repository-wide pre-existing lint findings, the parallel full-suite timeout in
unrelated ApiFrontend tests, and unavailable KA envtest tooling.
