# PR #2637 Consolidated RCA

**Reviewed source commit:** `93db7975b` (`fix(ci): stabilize FMC repair contract test`)
**Branch:** `work/2593-fmc-presence`
**Documentation follow-up:** subsequent commits are documentation-only; no production or test implementation changes were made after the reviewed source commit.
**CI runs:** `43642008948` (CI Pipeline, completed with held integration/E2E failures), `43642008872` (CodeQL, passed), `43642008918` (Gitleaks secret scan, passed); follow-up documentation run `43647855182` had no final conclusion at last observation.
**Review date:** 2026-11-14
**Scope:** PR #2637 only; the 87-file PR boundary is preserved.
**Operator deployment follow-up:** `kubernaut-operator` PR [#637](https://github.com/jordigilh/kubernaut-operator/pull/637), head `c9152b8`, adds FMC `POD_UID` and namespace-scoped writer-Lease RBAC.

## Decision

The PR has two different classes of evidence:

1. Several failures are deterministic defects in the changed FMC presence/recovery
   paths or their fixtures. These are approved for minimal RED/GREEN/REFACTOR fixes.
2. The remaining failures and review suggestions depend on unrelated GVK, inventory,
   ownership, deployment, security, or architecture changes. They remain held and are
   not used to expand #2593.

This RCA is the consolidated disposition after the bounded source fixes. It is not a
release approval and does not authorize cluster activity, E2E qualification, #2584,
#2592, GVK migration, inventory redesign, or ownership redesign.

## CI evidence and attribution

### Current-head checks

Commit `93db7975b9fe999ee068068f55e9dffc78c9d3b5` is the final reviewed source
head. The later `8d4fc4cc8` and `7ec90d893` commits only update RCA/review
authority documentation; they do not change production or test implementation.
CodeQL run `43642008872` and Gitleaks run `43642008918` passed. CI Pipeline run
`43642008948` completed with all unit-test lanes, service builds, Go lint, license
scan, CEL immutability regression, operator build/package, and unaffected
integration/E2E lanes passing. Its failures are limited to the held integration
and E2E categories documented below. `Test Suite Summary` and `Merge Gate` are
downstream rollups, not independent root causes.

Local validation for `93db7975b` passed:

- `go build ./...`
- `golangci-lint run --timeout=5m` (0 issues)
- `go test -race ./pkg/fleet/scopecache -ginkgo.v` (33/33 specs)
- `go test ./cmd/fleetmetadatacache -ginkgo.v`
- `go test ./pkg/fleet/fmc -ginkgo.v`
- `go test ./pkg/fleet/mcpclient -ginkgo.v`
- `go test ./pkg/workflowexecution/...`

The documentation-only follow-up run `43647855182` was still queued/in progress
when checked; its CodeQL (`43647854978`) and Gitleaks (`43647855127`) checks passed,
but no CI Pipeline pass/fail claim is made for that run.

The local consolidated package run also exposed the unrelated baseline
`UT-AUDIT-2522-001` timing failure. The earlier local `go test ./...` attempt was
blocked before the EAIGW E2E specs by the container image build: `dnf` reported a
malformed package database and `No space left on device`. Neither is source
evidence against the bounded FMC changes.

### Final CI result: run `43642008948`

The current-head remote result does not authorize source expansion. The PR remains
`BEHIND`, `MERGEABLE`, and `REVIEW_REQUIRED`; the required human review is still
outstanding. The prior FMC E2E `POD_UID` startup defect is covered by the generated
manifest fix and static contract test, but both current FMC E2E lanes fail earlier
in infrastructure setup.

### Failed integration lanes

| Lane | Observed failure | Disposition | Confidence |
|---|---|---|---:|
| `fleetmetadatacache` | The lane still reports inherited `kube-mcp-server: unknown flag: --port` and two `fmc scope check failed` cases; 17/20 executed specs passed, with 6 skipped | Hold dependency/fixture environment attribution; the validated structured-observation fix is already in the bounded diff | 95% |
| `gateway` | Fleet signal/scope paths report `scope: explicit group/version is required for "Deployment"` | Introduced by this PR's explicit remote-GVK validation (`pkg/gateway/signal_ingestion.go:491-495`); current fleet fixtures create remote Deployment signals without `apiVersion`. The fail-closed behavior is intentional, while producer/fixture alignment remains held for an approved follow-up | 98% attribution |
| `signalprocessing` | Remote enrichment and ClusterRegistry wiring fail on the same GVK/adjacent fleet path | Hold; do not expand into GVK or #2592 work | 90% attribution, below fix threshold |
| `apifrontend` | Fleet AF severity/remediation ownership assertions fail | Hold; AF/RO ownership attribution is unresolved | <90% |
| `remediationorchestrator` | Remote managed-resource routing fails in fleet scope integration | Hold; AF/RO ownership attribution is unresolved | <90% |

The previously observed `kubernautagent` structured-observation fixture failure is
resolved; its current unit, integration, and E2E lanes passed.

### Failed E2E lanes

| Lane | Observed failure | Disposition | Confidence |
|---|---|---|---:|
| `fleetmetadatacache-eaigw` | Setup fails before specs: the remote Kind kube-apiserver is `Pending`, and in-cluster `10.96.0.1:443` returns `connection refused`; 0/16 specs ran | Hold remote Kind/Podman infrastructure; no FMC source attribution | 99% |
| `fleetmetadatacache-kuadrant` | Setup fails before specs with the same remote kube-apiserver/API VIP failure; the `mcp-gateway` rollout also has an old replica pending termination; 0/16 specs ran | Hold remote Kind/Podman infrastructure; no FMC source attribution | 99% |
| `fleet` | Setup fails before specs because of independent infrastructure/resource/Podman conditions | Hold; no introduced-versus-inherited proof | <90% |
| `apifrontend-fleet` | Setup fails before specs because of independent infrastructure/resource conditions | Hold; no introduced-versus-inherited proof | <90% |

## CodeRabbit disposition

CodeRabbit review `5905956322` reported 16 major, 9 inline minor, 2 nitpick, and 1
outside-diff minor finding. Each is classified below.

### Major findings

| Finding | Disposition | Reason |
|---|---|---|
| `docs/tests/2593/TEST_PLAN.md`: publish the external active plan | **Fix** | `docs/tests/2593/TEST_PLAN.md` is now the repository-versioned active implementation/test authority; the developer-local OpenChamber plan is retained only as historical provenance. |
| `pkg/fleet/fmc/syncer.go`: use `snapshot.Info` for repair collection | **Fix** | The repair reader must use the registration configuration matching the captured snapshot revision. |
| `pkg/fleet/fmc/syncer.go`: empty registry never becomes ready | **Fix** | `FMCGlobalReadiness.ServiceReady` preserves the empty-registry exemption, while the production readiness path now publishes the registered-cluster count so a registered cluster without a lease or repair partition remains unavailable. |
| `pkg/fleet/mcpclient/client.go`: strict-list integration fixture | **Fix** | The fixture is invalid under the already-approved structured observation contract. |
| `pkg/fleet/mcpclient/client.go`: restore ordinary `Event` list behavior | **Fix** | `Client.List` now uses a generic structured-object path for ordinary reader calls; strict `ListObserved` retains the FMC observed-GVK allowlist. `UT-FLEET-MCP-012` covers both paths. |
| `pkg/fleet/mcpclient/observation.go`: infer empty response without `structuredContent` | Hold | The digest-pinned producer behavior is not proven; fail-closed semantics remain required. |
| `pkg/fleet/registry/delete.go`: resolve the current map under lock | **Fix** | This is a localized deletion consistency race with a clear lock-scoped correction. |
| `pkg/fleet/registry/eaigw_registry.go`: fence `Probe` and `Start` publication | Hold | Requires EAIGW refresh/generation architecture and is not an FMC presence fix. |
| `pkg/fleet/registry/eaigw_registry.go`: replace global repair fencing with per-registration revisions | Hold | Cross-registry repair-fencing redesign; outside #2593. |
| FMC E2E manifest: inject `POD_UID` | **Fix** | The production entry point requires it and the generated manifest omits it. |
| `pkg/fleet/scopecache/readiness.go`: remove deregistered partitions | Hold | Readiness lifecycle/replacement semantics need an architectural decision and broader tests. The current fix does not claim stale-partition removal. |
| `pkg/fleet/scopecache/readiness.go`: compare proofs with the active writer fence | Hold | Security/readiness lifecycle redesign; no change below the authorized boundary. |
| `fmc_v2_atomic.lua`: cleanup-safe pagination | **Fix** | Positional cursors skip stale members after cleanup mutation. |
| `pkg/fleet/scopecache/status_store.go`: prove complete empty observations | Hold | Empty-partition proof semantics are unresolved and must not be fabricated with zero TTLs. |
| `pkg/shared/scope/manager.go`: Gateway HPA list/watch RBAC | Hold | Requires effective deployment RBAC validation and is unrelated to FMC presence. |
| `fmc_broker_readiness_test.go`: production observation readiness gate | Hold | Requires production wiring and a broker-readiness behavior change not authorized for this pass. |

### Minor findings

| Finding | Disposition | Reason |
|---|---|---|
| `pkg/fleet/scopecache/client.go`: nil reader in `IsManaged` | **Fix** | A valid public call must not panic on a status-store-only client. |
| `pkg/fleet/fmc/syncer.go`: cleanup contexts start too early | **Fix** | Cleanup deadlines must begin when deferred cleanup starts. |
| `pkg/fleet/fmc/syncer.go`: aggregate actual page counts | **Fix** | Validation must bound pages, not the number of configured resource kinds. |
| `pkg/fleet/fmc/syncer.go`: fence `publishGlobal` against lease loss | Hold | Same readiness/security lifecycle boundary as the active-fence finding. |
| `TEST_PLAN.md`: typed-scope focus regex | **Fix** | The current pattern omits scenarios 020 and 021. |
| `TEST_PLAN.md`: clarify RED/GREEN gate wording | **Fix** | Documentation must require authored RED contracts and recorded expected failures. |
| `TEST_PLAN.md`: reconcile superseded pre-RED review gate | **Fix** | The text must agree with the approved fast-path boundary without adding another review cycle. |
| `pkg/fleet/inventory/types.go`: RBAC identity-name validation | Hold | Inventory validation and #2584 are explicitly outside this PR closure. |
| `pkg/fleet/scopecache/lease_coordinator.go`: reject expired leases in `Verify`/`Renew` | **Fix** | Expiry must force stale-fence loss and a new epoch. |

### Nitpicks and outside-diff finding

| Finding | Disposition | Reason |
|---|---|---|
| `status_store_red_test.go`: replay a successfully committed token | **Fix** | The test must prove consumed-token rejection, not only mismatched-token rejection. |
| `console_policyread_2584_test.go`: recompute changed fixture ID | Hold | Outside the PR boundary and explicitly excluded #2584 work. |

### Follow-up review validation

CodeRabbit reviews `5907153134` (reviewed head `5a67d69e8`) and `5907439439`
(reviewed head `8d4fc4cc8`) were rechecked against the current branch. The latest
review was published before the documentation-only `7ec90d893` follow-up, so its
external-plan finding is already reflected below.

| Finding | Current evidence | Disposition |
|---|---|---|
| Review authority still requires an external-plan hash | `docs/tests/2593/evidence/reviews/README.md` now requires the repository-versioned authority hash for new captures; `TEST_PLAN.md` identifies the OpenChamber plan as historical only. `shasum -a 256 docs/tests/2593/TEST_PLAN.md` matches `aede83812e3991df16daeab295d4a45da95e1274f5559f9dfa15daebbbad23e8`. | **Addressed** in `7ec90d893`; no source change required. |
| Gateway GVK rejection was classified as inherited | The gateway row now records the rejection as introduced by the explicit remote-GVK validation and identifies the current fixture omission of `apiVersion`; fail-closed behavior remains intentional. | **Addressed** in `7ec90d893`; producer/fixture alignment remains a held GVK follow-up. |
| Successful repair test needs deterministic server time | `status_store_red_test.go:233` calls `server.SetTime(...)` before the cleanup lifecycle; CodeRabbit confirmed this finding as addressed in `93db7975b`. | **Addressed**. |
| Registered-cluster deletion followed by failed initial repair can reuse a stale clean partition | `handleClusterEvent` still logs deletion without removing the identity-keyed readiness partition. Fixing this needs identity-based removal and stale-repair protection, not only the registered-count guard added in `0a1f56b20`. | **Hold** for the readiness-lifecycle design decision; no partial fix is authorized in this slice. |
| Fence verification/reacquisition and dependency refresh can restore stale readiness | `ensureFence` clears local ownership before reacquisition, while `fmcDeps.Ready` and the syncer publish a previously read global snapshot. Both findings are valid concurrency/security concerns. | **Hold** for coordinated readiness/fence synchronization design and tests; no partial fix is authorized. |
| Empty observations can never produce clean proof; missing `structuredContent` might be empty | `hasCleanRepairProof` correctly rejects zero TTL proof, and `extractObservationPage` correctly rejects missing `structuredContent`. No digest-pinned producer capture proves a reliable empty response. | **Hold** on the existing fail-closed protocol boundary; do not fabricate empty proof. |
| FMC operator deployment lacks `POD_UID` and writer-Lease permissions | FMC is deployed by `kubernaut-operator`, not by an FMC Helm chart in this repository. The operator source lacked both the `metadata.uid` downward-API variable and namespace-scoped `coordination.k8s.io/leases` `get/create/update` access required by `fleetmetadatacache-writer`; operator PR [#637](https://github.com/jordigilh/kubernaut-operator/pull/637) addresses both. | **Actionable follow-up opened**; operator CI/review is pending. This is outside #2637's 87-file source boundary, and the current Kubernaut E2E CI failed during remote Kind setup before FMC specs. |
| Controller-runtime fake, rendered-manifest test, integration no-mocks fixture, and `Syncer` state extraction | These are valid maintainability/test-harness suggestions, but each requires a production seam or broader fixture/refactor change beyond the bounded FMC A/B closure. | **Hold**; do not expand the 87-file boundary or introduce an untested refactor. |
| Gateway HPA RBAC | The scope client is informer-backed, but the effective Gateway ServiceAccount binding is owned by `kubernaut-operator`, not this repository. | **Hold** for the deployment owner; no permission change is inferred from this checkout. |

No additional implementation change is justified within the approved #2637 boundary.
Held findings remain explicit follow-up work, not silent approval or evidence that the
current E2E/deployment contracts are qualified. The operator deployment gap is tracked
separately in PR #637.

## Approved implementation boundary

The implementation changed only the following validated concerns:

- snapshot-consistent FMC repair configuration;
- empty-registry readiness;
- deletion locking, cleanup context timing, aggregate page counts, and cleanup-safe pagination;
- structured MCP integration fixture data;
- generic ordinary MCP list behavior without weakening strict FMC observation validation;
- FMC E2E `POD_UID` wiring;
- registered-cluster readiness fencing while preserving empty-registry readiness;
- nil-reader safety and lease-expiry handling;
- committed repair-token replay coverage and focused test-plan documentation.

No held finding is a prerequisite for this bounded implementation. Any new failure in
the held categories requires a new attribution decision rather than an opportunistic
scope expansion. The remaining held findings are digest-pinned producer
behavior, registry refresh/per-registration fencing, readiness partition removal and
active-fence lifecycle, empty-partition Valkey proof, effective Gateway RBAC, and
production broker-observation wiring.

## Validation record

The RED phase added Ginkgo/Gomega tests with BR or scenario IDs before the production
changes. GREEN wired the minimal fixes and passed the affected tests. REFACTOR ran the
required build, lint, focused tests, and diff checks. The current evidence separates
source-attributed defects from the inherited GVK/ownership/infrastructure findings and
the local E2E image-build failure.
