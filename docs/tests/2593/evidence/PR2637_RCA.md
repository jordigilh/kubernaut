# PR #2637 Consolidated RCA

**Reviewed commit:** `e10aad684` (`fix(ci): stabilize FMC presence checks`)
**Branch:** `work/2593-fmc-presence`
**CI run:** `43616596962`
**Review date:** 2026-11-14
**Scope:** PR #2637 only; the 87-file PR boundary is preserved.

## Decision

The PR has two different classes of evidence:

1. Several failures are deterministic defects in the changed FMC presence/recovery
   paths or their fixtures. These are approved for minimal RED/GREEN/REFACTOR fixes.
2. The remaining failures and review suggestions depend on unrelated GVK, inventory,
   ownership, deployment, security, or architecture changes. They remain held and are
   not used to expand #2593.

This RCA is the consolidated disposition required before implementation. It is not a
release approval and does not authorize cluster activity, E2E qualification, #2584,
#2592, GVK migration, inventory redesign, or ownership redesign.

## CI evidence and attribution

### Passed checks

The following PR checks passed in run `43616596962`: Go analysis and lint, CodeQL,
gitleaks, license scan, all unit-test lanes, all build lanes, CEL regression checks,
and must-gather validation. `Test Suite Summary` and `Merge Gate` are downstream
rollups, not independent root causes.

### Failed integration lanes

| Lane | Observed failure | Disposition | Confidence |
|---|---|---|---:|
| `fleetmetadatacache` | FMC integration path reports inherited `kube-mcp-server: unknown flag: --port`; strict fixture failures are separately covered below | Hold infrastructure failure; fix the validated fixture only | 95% |
| `kubernautagent` | `observation invalid_item`: fixture rows lack the requested Pod GVK/metadata | Fix the fixture with structured Pod items | 97% |
| `gateway` | Fleet signal/scope paths report `scope: explicit group/version is required for "Deployment"` | Hold; GVK contract is outside #2593 | 92% attribution, below fix threshold |
| `signalprocessing` | Remote enrichment and ClusterRegistry wiring fail on the same GVK/adjacent fleet path | Hold; do not expand into GVK or #2592 work | 90% attribution, below fix threshold |
| `apifrontend` | Fleet AF severity/remediation ownership assertions fail | Hold; AF/RO ownership attribution is unresolved | <90% |
| `remediationorchestrator` | Remote managed-resource routing fails in fleet scope integration | Hold; AF/RO ownership attribution is unresolved | <90% |

### Failed E2E lanes

| Lane | Observed failure | Disposition | Confidence |
|---|---|---|---:|
| `fleetmetadatacache-eaigw` | FMC deployment cannot become ready; `cmd/fleetmetadatacache/main.go` logs `POD_UID is required` | Add the downward-API `POD_UID` to the generated FMC pod manifest | 98% |
| `fleetmetadatacache-kuadrant` | Same FMC rollout timeout and missing `POD_UID` startup condition | Covered by the same manifest fix; no E2E run is authorized here | 98% |
| `fleet` | Setup fails before specs because of independent infrastructure/resource/Podman conditions | Hold; no introduced-versus-inherited proof | <90% |
| `apifrontend-fleet` | Setup fails before specs because of independent infrastructure/resource conditions | Hold; no introduced-versus-inherited proof | <90% |

## CodeRabbit disposition

CodeRabbit review `5905956322` reported 16 major, 9 inline minor, 2 nitpick, and 1
outside-diff minor finding. Each is classified below.

### Major findings

| Finding | Disposition | Reason |
|---|---|---|
| `docs/tests/2593/TEST_PLAN.md`: publish the external active plan | Hold | The approved fast-path boundary deliberately retains the external plan authority; publishing or replacing it changes the review/approval workflow. |
| `pkg/fleet/fmc/syncer.go`: use `snapshot.Info` for repair collection | **Fix** | The repair reader must use the registration configuration matching the captured snapshot revision. |
| `pkg/fleet/fmc/syncer.go`: empty registry never becomes ready | **Fix** | Empty-registry readiness is an existing contract and belongs in `FMCGlobalReadiness.ServiceReady`. |
| `pkg/fleet/mcpclient/client.go`: strict-list integration fixture | **Fix** | The fixture is invalid under the already-approved structured observation contract. |
| `pkg/fleet/mcpclient/client.go`: restore ordinary `Event` list behavior | Hold | Requires an executor-facing list-path redesign outside the bounded FMC observation change. |
| `pkg/fleet/mcpclient/observation.go`: infer empty response without `structuredContent` | Hold | The digest-pinned producer behavior is not proven; fail-closed semantics remain required. |
| `pkg/fleet/registry/delete.go`: resolve the current map under lock | **Fix** | This is a localized deletion consistency race with a clear lock-scoped correction. |
| `pkg/fleet/registry/eaigw_registry.go`: fence `Probe` and `Start` publication | Hold | Requires EAIGW refresh/generation architecture and is not an FMC presence fix. |
| `pkg/fleet/registry/eaigw_registry.go`: replace global repair fencing with per-registration revisions | Hold | Cross-registry repair-fencing redesign; outside #2593. |
| FMC E2E manifest: inject `POD_UID` | **Fix** | The production entry point requires it and the generated manifest omits it. |
| `pkg/fleet/scopecache/readiness.go`: remove deregistered partitions | Hold | Readiness lifecycle/replacement semantics need an architectural decision and broader tests. |
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

## Approved implementation boundary

The implementation may change only the following validated concerns:

- snapshot-consistent FMC repair configuration;
- empty-registry readiness;
- deletion locking, cleanup context timing, aggregate page counts, and cleanup-safe pagination;
- structured MCP integration fixture data;
- FMC E2E `POD_UID` wiring;
- nil-reader safety and lease-expiry handling;
- committed repair-token replay coverage and focused test-plan documentation.

No held finding is a prerequisite for this bounded implementation. Any new failure in
the held categories requires a new attribution decision rather than an opportunistic
scope expansion.

## Validation record

The RED phase must add Ginkgo/Gomega tests with BR or scenario IDs before production
changes. GREEN must wire the minimal fixes and pass the affected tests. REFACTOR must
run the required build, lint, focused tests, and diff checks. The final report must
separate source-attributed failures from inherited CI/infrastructure failures.
