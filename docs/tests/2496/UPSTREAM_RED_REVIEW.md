# Issue #2496 — Upstream Handoff / Persistence RED Review

**Status**: RED review complete — REQUIRES CHANGES; not approved for GREEN<br>
**Scope**: Isolated spike only; no production implementation approval<br>
**Reviewed evidence**: Corrected checkpoint 3, reported 2026-10-09 12:39:06 UTC; independent source review and test reruns completed afterward<br>
**Worker**: GPT-5.6 Luna, `ses_edf594445ffeG2pVwf6kIrXS5g`; stopped before GREEN<br>
**Related plan**: [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) · [TEST_PLAN.md](TEST_PLAN.md)

**Incorporated issue**: [#2498](https://github.com/jordigilh/kubernaut/issues/2498) — the phase-isolation findings below are part of the combined #2496 implementation scope. The current contract makes RCA context-tool calls optional and limits Phase 3 to workflow-discovery/result-submission tooling.

**Subsequent compatibility scope**: The user permits a v1.6 break from v1.5 deployments for this enhancement. Legacy AA recovery/backfill is not required. This does not change the historical RED findings below: current-version producer wiring, captured/frozen evidence, conditional repeat validation and real status/audit persistence still need their own proof.

This supersedes the checkpoint-2 review previously recorded in this file. The replacement worker fixed some fixtures and exact-value assertions and provisioned runtime prerequisites. Independent review still rejects the complete RED gate. A failing test is valid RED evidence only when its failure and intended passing behavior match the business contract; an unconditional failure or a passing old-field round-trip cannot stand in for missing new-boundary proof.

## Boundary readiness

| Boundary | Ready for prototype GREEN? | Independently reviewed checkpoint-3 evidence |
|---|---|---|
| Prior attempts → upstream RCA | **No** | `UP-005` uses the actual OpenAI-compatible HTTP client and autonomous investigator, but still uses substring assertions and an exact four-request assumption. The provider fixture submits RCA immediately; it collects no observation through an RCA tool. `current-hash` is supplied as a history response field, not a proven captured observation. No opposite-target/origin case is written. Source confirms prefetch exists but `runRCA` receives no explicit enrichment input. |
| Frozen RCA/current-prior packet → selection | **No** | No immutability or source-ownership test exists. Absence of `spike-secret-parameter` proves nothing because that value is never supplied. Narrative/spec-hash presence does not establish a frozen, sanitized packet, authoritative captured observations or preserved unknowns. |
| Repeat selection → bounded correction | **No** | Prior `restart-pod` identity and `.94` selection confidence are now supplied. However, `UP-004` requires correction to `scale-up`, then requires that different workflow's rationale to contain `prior`. Different workflows do not require repeat justification, and justified repeats must remain eligible. The catalog fetcher injects discovered IDs through a business stub; no assertion identifies missing repeat justification as the actual validation error. |
| Runtime phase isolation | **Valid narrow RED finding; incomplete gate** | `UP-003b` reproduces dispatch of a registered off-phase test tool (`calls=1`, expected `0`). This proves dispatcher behavior, not a live Kubernetes read. Advertisement-only `UP-003` passes. Authorized RCA/catalog/fleet-overlay paths and observable rejection still need proving tests before a complete guard gate can be approved. |
| AgentSession → AA success/review mapping | **Useful narrow RED; boundary not ready** | Both mapping UTs now assert the three exact justification values rather than mere keys, and they fail because the structured object is dropped. They inspect snake-case `repeat_justification` on serialized CRD status, whereas the proposed plan specifies camel-case `repeatJustification` there. Align the provisional wire contract first; selected confidence, exact review reason/phase and partial preservation need complete coverage. These remain in-memory UTs. |
| Typed audit → authenticated DS → PostgreSQL | **No; schema harness provisioned** | The worker applied production Goose migrations and the partition helper. The test then calls unconditional `Fail` without constructing a DS server/client or attempting authenticated ingestion/query. Its message cites unavailable permission to provision Redis; this is a harness-scope restriction, not an architectural incompatibility or a proved Redis/service failure. No typed audit payload, authenticated HTTP request or persisted new value was exercised. |
| AA status → admitted envtest API → read-back | **No; admission harness verified** | Independent envtest rerun passes on repository-supported Kubernetes `1.37.0` assets. The test directly constructs status and stores explanation-like text in existing `Rationale`, then reads it back. It neither runs the production mapper nor persists a structured justification/evidence packet. Directly setting a review flag is not proof that the low-confidence production path produces it. |

## Independent validation

The parent read all three submitted test files, the worker plan and production type/dispatcher/RCA definitions. It independently reran these commands against the isolated detached worktree:

```sh
go test ./internal/kubernautagent/investigator ./pkg/aianalysis \
  -count=1 -timeout=3m -ginkgo.focus='Issue 2496' -ginkgo.no-color
```

- Investigator: **5 selected, 1 passed, 4 failed**. Failure count alone is not RED approval: `UP-004` has the wrong acceptance condition, and handoff/correction cases still use business catalog stubs.
- AA mapping: **2 selected, 0 passed, 2 failed**. Exact-value preservation failures are useful characterization, subject to fixing CRD/wire naming and completing review-path assertions.

```sh
KUBEBUILDER_ASSETS="$(bin/setup-envtest use 1.37.0 -p path)" \
go test ./test/integration/issue2496 -count=1 -timeout=5m \
  -ginkgo.focus='IT-AA-2496-UP-003' -ginkgo.no-color -v
```

- Admission harness: **1 selected, 1 passed, 0 failed**, about 6 seconds. This verifies only existing direct status persistence. Assets resolve to the compatible envtest cache; no default/live Kubernetes context was used.
- Database migration success is supported by the worker's runtime attempt; the parent reviewed the harness source but did **not** rerun its unconditional-failure DS test. Authenticated DS persistence remains **unexecuted**, not a contract failure.
- Unselected specs in focused Ginkgo runs are framework filtering, not newly authored pending/`Skip()` cases.

## Blocking test findings (worktree paths)

| Finding | Evidence location |
|---|---|
| Different-workflow rationale is incorrectly treated as mandatory repeat rationale; no true repeat-error assertion | `internal/kubernautagent/investigator/upstream_handoff_2496_test.go:172-180` |
| No RCA observation collection; exact request count; absent secret never seeded | `internal/kubernautagent/investigator/upstream_handoff_2496_test.go:183-235` |
| Proposed CRD camel-case field is checked as a model/audit snake-case field | `pkg/aianalysis/upstream_handoff_2496_persistence_test.go:48,83`; [implementation contract](IMPLEMENTATION_PLAN.md#42-justification-and-agent-owned-suitability) |
| Schema setup ends in unconditional failure, with no DS HTTP path | `test/integration/issue2496/runtime_red_test.go:35-55` |
| Directly authored old-field status bypasses the production mapper and structured contract | `test/integration/issue2496/runtime_red_test.go:84-104` |
| Plan still labels runtime rows unwritten and mapping UTs as IT; contradicts runtime RED permission | `docs/tests/2496/UPSTREAM_SPIKE_PLAN.md:32-44` |
| Cleanup errors are ignored rather than asserted/logged | `test/integration/issue2496/runtime_red_test.go:38,43,64` |

No field contracts were approved by these prototype fixtures. In particular, a prior ineffective outcome must not force an invented historical failure mechanism, and the three-part structured object remains a proposed contract rather than an accepted production schema.

## Review checklist (RED scope)

| Dimension | Result |
|---|---|
| Plan alignment | **Requires changes** — the full upstream/persistence proof is not written/exercised, and the worktree ledger is stale. |
| Business/control mapping | BR/scenario references are present, but references alone do not establish the scoped controls or reconstruction journey. |
| TDD / test quality | No production edits before RED; Ginkgo/Gomega used. **Complete RED gate fails** because some acceptance conditions are wrong or bypass the required boundary. |
| Code quality | Test harness ignores cleanup errors; complete wire contracts and business-stub replacement are outstanding. |
| Integration / wiring | Actual provider HTTP dispatch and old-field API admission exercised; required typed handoff, authenticated DS and production-mapper persistence remain unproved. CHECKPOINT W not reached. |
| Build/lint/regression/release | **Not assessed** — focused suites compile and exhibit the results above; no whole-repository build/lint, full regression, E2E or reasoning-quality evaluation is claimed. GREEN/REFACTOR are not started. |

## Required repairs before another RED checkpoint

1. Supply actual same-workflow identity and selected confidence through trusted scoped history; deliberately distinguish prior selection confidence from current RCA and current selection confidence.
2. Capture the real autonomous RCA provider request **before** selection, then prove exact current/prior handoff values and source binding in the selection request. Preserve unknown/SpecDrift semantics and minimize sensitive data.
3. Exercise real catalog membership and repeat validation/correction. Accept a justified same-`workflow_id` repeat after both successful/remediated and ineffective completed assessments, as well as a corrected different workflow; do not demand repeat rationale from the latter. First/different/no-workflow and unknown/SpecDrift remain valid; missing repeat justification must be the actual rejection reason, not an unrelated membership or setup failure.
4. Reject forced off-phase tool dispatch before its external side effect; preserve allowed phase tools and make rejection observable. No Kubernetes MCP privileges are added to discovery.
5. Assert exact nested explanation values, selected confidence and human-review state in mapping UTs, then prove actual admitted status read-back separately.
6. Reuse the now-working envtest assets and production migration setup. Resolve the spike-owned Redis harness restriction, drive real authenticated DS ingestion/query and verify the actual typed payload persisted to PostgreSQL. Drive admitted status updates from production mapper output with exact structured values. No ad hoc table, unconditional-failure placeholder or old-field serialization-only replacement.
7. Record genuine absent-contract failures separately from infrastructure failures; identify unexecuted boundaries honestly. Do not mark GREEN until retained UT and wiring IT pass.

## Supervision and scope

- Two earlier checkpoints were rejected; their original tests/logs remain preserved in the isolated worktree. The fresh third checkpoint also requires changes; this review does not authorize another automatic correction loop or GREEN.
- The user explicitly approved including the phase-dispatch guard after the finding was escalated.
- The fresh Luna worker returned its checkpoint and stopped. No prototype production source edits were made. The main checkout contains only uncommitted planning/review documents under `docs/tests/2496/`; its unrelated branch remains untouched.
- No runtime reviewing agent is part of this spike; that future improvement is [#2497](https://github.com/jordigilh/kubernaut/issues/2497).
- No commit, push, live-cluster use, shared-infrastructure cleanup or production implementation is approved.

**Confidence: 95% in this RED review.** The rejection follows direct source inspection and independent investigator, mapping and admission runs. Database migrations were worker-verified, not independently rerun, and authenticated DS/production-mapper persistence and model reasoning quality remain unproved. Harness readiness improved, but **upstream handoff and persistence are not ready for prototype GREEN or implementation approval**.

## User-directed continuation after this verdict

The user subsequently authorized a **separate foundational history slice**, following the recommendation to supply previous workflow identity/name, selected confidence and existing assessment/chronology before attempting the full handoff. It must prove stored scoped history through the actual DS/KA path into the **first RCA request**, starting with a new valid RED checkpoint. This is explicit user intervention, not another automatic correction loop or retroactive approval of the rejected tests. A slice-level prototype gate may be evaluated independently; all full-handoff/selection/status/audit findings above remain open. Production implementation, commit and push remain unapproved.
