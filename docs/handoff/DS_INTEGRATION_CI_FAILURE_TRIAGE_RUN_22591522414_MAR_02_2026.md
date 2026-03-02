# DataStorage Integration CI Failure — RCA Triage (Run 22591522414)

**Date**: March 2, 2026  
**Branch**: `feature/v1.0-bugfixes-demos`  
**CI Run**: [GitHub Actions Run #512](https://github.com/jordigilh/kubernaut/actions/runs/22591522414)  
**Failed Job**: [Integration (datastorage) — Job 65451282088](https://github.com/jordigilh/kubernaut/actions/runs/22591522414/job/65451282088)  
**Commit**: `edeb34e8b` (same as handoff PR_223_DS_CI_STABILIZATION_HANDOFF.md)

**Follow-up**: Run [#514](https://github.com/jordigilh/kubernaut/actions/runs/22592694354) (commit `67727131d` — cancel-before-Stop teardown fix) still failed (exit code 2, ~2m 51s). CI now captures integration test stdout/stderr to `integration-<service>.log` and uploads it as an artifact on failure so the next run provides logs for RCA.

---

## 1a. Why This Works on Main and Fails on This Branch

**On `main`**, the DataStorage integration suite **does not use envtest**. The suite only sets up Podman (PostgreSQL + Redis) and per-process DB/Redis clients. There is no `dsTestEnv`, no `envtest.Environment`, and no envtest teardown. So no hang in `envtest.Stop()` and no coordination/coverage issues from envtest.

**On this branch** (`feature/v1.0-bugfixes-demos`), the following were added for **DD-WE-006** (schema-declared dependency validation):

| Change | Commit | Effect |
|--------|--------|--------|
| **New test file** | `be3cafcb5` | `workflow_dependency_validation_test.go` — integration tests that need a real K8s API (Secrets/ConfigMaps validation). |
| **Suite: envtest added** | Same / follow-ups | `suite_test.go` now starts **envtest** in SynchronizedBeforeSuite Phase 2 (every process) and stops it in SynchronizedAfterSuite Phase 1. This is the **only** place envtest was introduced into the DS integration suite. |
| **Partition naming fix** | `edeb34e8b` | `createDynamicPartitions` format aligned with migrations. |
| **envtest.Stop() timeout** | `edeb34e8b` | 5s timeout around `dsTestEnv.Stop()` to avoid infinite hang. |
| **Cancel before Stop** | `67727131d` | Teardown order aligned with Gateway. |

So the failures are **not** from new test specs that fail assertions. They come from **new infrastructure**: envtest was added on this branch to support the new DD-WE-006 tests, and envtest teardown (or Ginkgo/coverage coordination under that load) is failing in CI (exit code 2, no must-gather). Main never runs envtest in DS integration, so main passes.

**Summary**: Same test suite as main for everything that doesn’t need K8s. This branch adds envtest + `workflow_dependency_validation_test.go`. The failing runs are due to that new envtest lifecycle, not new business-logic tests.

---

## 1b. How Gateway (GW) Handles This Situation

Gateway integration **has used envtest from the start** (it’s already on `main`). The GW suite is **unchanged on this branch** (`git diff origin/main -- test/integration/gateway/suite_test.go` is empty). So GW isn’t “newly adding” envtest; it was designed with it and has been stable.

**What GW does:**

| Aspect | Gateway integration | DataStorage (this branch) |
|--------|--------------------|----------------------------|
| **Envtest on main?** | Yes (suite unchanged) | No (envtest added only on this branch) |
| **Envtest layout** | **Two-tier**: (1) One **shared** envtest in Phase 1 (process 1 only) for DataStorage auth / kubeconfig; (2) One **per-process** envtest in Phase 2 for Gateway CRD tests. | **Single-tier**: One per-process envtest only (Phase 2), no shared envtest. |
| **Who stops envtest** | Phase 1 AfterSuite: each process stops its **own** `testEnv` (per-process). Shared envtest is stored in `dsInfra.SharedTestEnv`; some suites (e.g. SignalProcessing) stop it in Phase 2; GW calls `StopDSBootstrap` in Phase 2 (containers only). | Phase 1 AfterSuite: each process stops its **own** `dsTestEnv`. |
| **Teardown order** | **cancel()** then **testEnv.Stop()** (no timeout). | **cancel()** then **dsTestEnv.Stop()** with 5s timeout (after fix). |
| **Envtest config** | Per-process envtest loads **CRDs** (`CRDDirectoryPaths`). | Per-process envtest **core API only** (no CRDs), `ErrorIfCRDPathMissing: false`. |

So GW “handles” it by: (1) having envtest as part of the original design (no recent add), (2) using **cancel() before Stop()**, and (3) using a **shared** envtest in Phase 1 so only process 1 starts/stops that instance; per-process envtest is then the only thing each of the 4 processes stops in Phase 1. DS, by contrast, added envtest only on this branch and runs **four independent** envtest instances (no shared one), all stopped in Phase 1, which may be more prone to resource contention or coordination issues in CI.

---

## 1. Observed Failure (Run #512 / #514)

| Item | Value |
|------|--------|
| **Step** | Run datastorage integration tests |
| **Result** | Process completed with **exit code 2** |
| **Duration** | ~3m 11s |
| **Annotation** | No files found: `must-gather-*.tar.gz` → no must-gather artifact uploaded |

Exit code **2** from Ginkgo typically indicates an **infrastructure/runner failure** (e.g. timeout, interrupt, or parallel-run coordination failure), not a normal test assertion failure (which is usually exit code 1).

---

## 2. Why Must-Gather Logs Are Missing (RCA for “No Artifacts”)

Must-gather for **integration** tests is produced only when **SynchronizedAfterSuite** runs to completion:

1. **Phase 1** runs on **all** parallel processes (per-process cleanup, including `envtest.Stop()` with 5s timeout).
2. **Phase 2** runs **once** on process 1: close DB/Redis, then call `infrastructure.MustGatherContainerLogs("datastorage", [postgres, redis], GinkgoWriter)`, which writes under `/tmp/kubernaut-must-gather/datastorage-integration-YYYYMMDD-HHMMSS/`.
3. The CI step “Collect must-gather logs on failure” runs only when the job has **failed** and tars `/tmp/kubernaut-must-gather` into `must-gather-${{ matrix.service }}-${TIMESTAMP}.tar.gz`.

If the test process **exits with code 2** before or during AfterSuite (e.g. Ginkgo timeout waiting for parallel procs, or coverage combine failure), then:

- One or more processes may never reach Phase 1 completion.
- Phase 2 may **never run** (or only run partially).
- So `/tmp/kubernaut-must-gather` is either **not created** or **empty**.
- The “Upload must-gather logs” step then finds no `must-gather-*.tar.gz` → **“No files were found”** warning.

**Conclusion**: The absence of must-gather artifacts is **consistent with a failure that prevents AfterSuite from completing** (timeout/hang/coverage combine), not with a simple spec failure. RCA therefore has to rely on code path analysis and prior handoff, not on must-gather logs for this run.

---

## 3. Root Cause Hypotheses (Evidence-Based)

From [PR_223_DS_CI_STABILIZATION_HANDOFF.md](./PR_223_DS_CI_STABILIZATION_HANDOFF.md) and the current codebase:

### Hypothesis A (High): envtest.Stop() Still Causing Hang/Timeout

- **Prior fix** (commit `edeb34e8b`): `dsTestEnv.Stop()` wrapped in a goroutine with **5s** timeout in `test/integration/datastorage/suite_test.go` (SynchronizedAfterSuite Phase 1).
- **Remaining risk**: If `Stop()` does not return within 5s, Phase 1 still proceeds, but if **multiple** processes block for the full 5s, total wall time can approach the job timeout; or if the runner kills the process on timeout, exit code can be 2 and Phase 2 never runs.
- **Evidence**: Run #509 showed “Ginkgo timed out waiting for all parallel procs” and “Failed to combine cover profiles”; run #512 has the same job (integration datastorage) failing with exit code 2 and no must-gather.

**Recommendation**: Confirm in logs whether “envtest.Stop() timed out after 5s” appears; if yes, consider shortening the wait or forcing envtest kill (e.g. separate process group + kill after 3s).

### Hypothesis B (Medium): Coverage Combine Failure (Orphaned Coverage Files)

- **Prior symptom** (run #509): `Failed to combine cover profiles` — `coverage_integration_datastorage.out.1` (or similar) missing because a process never reported back (e.g. stuck in `Stop()`).
- If the same situation occurs in #512, Ginkgo exits with a non-zero code (e.g. 2) and no must-gather is written.

**Recommendation**: In CI, add a step to capture Ginkgo stdout/stderr on failure (e.g. `make test-integration-datastorage 2>&1 | tee integration-datastorage.log`) and upload `integration-datastorage.log` as an artifact when the job fails, so “Failed to combine cover profiles” or timeout messages are visible.

### Hypothesis C (Lower): Real Test Failure Plus Exit Code 2

- Possible but less likely: a spec fails and something in the pipeline (e.g. `make` or wrapper) turns it into exit code 2. Usually Ginkgo uses 1 for failures.
- If that were the case, AfterSuite would often still run and must-gather would be present; the fact that it’s missing makes this less likely.

### Hypothesis D (Mitigated): Partition Naming Mismatch

- **Prior fix** (`edeb34e8b`): Partition name format in `createDynamicPartitions` aligned with migrations (`resource_action_traces_%d_%02d`). This was to avoid “would overlap” and warnings; it was not the direct cause of the combine/timeout.
- Unlikely to be the primary cause of #512, but worth confirming there are no new partition-related errors in logs once we have them.

---

## 4. Why DataStorage and Not Other Integration Jobs?

Several factors make the DataStorage integration job more likely to hit this failure than the other integration jobs:

### 4.1 Only DS Wraps `envtest.Stop()` in a Timeout

- **DataStorage** is the only suite that wraps `envtest.Stop()` in a goroutine + 5s timeout (`suite_test.go` ~391–404). That change was added *because* DS was already observed to hang there (run #509 handoff).
- **All other integration suites** (signalprocessing, remediationorchestrator, workflowexecution, aianalysis, gateway, notification, authwebhook, effectivenessmonitor, holmesgptapi) call `testEnv.Stop()` **directly** with no timeout. If their `Stop()` hangs, the process blocks until the job hits its timeout (e.g. 10–20 min) and gets killed — so you’d see a long-running failure, not an exit code 2 after ~3 min.
- So: **DS is the only one that was known to hang in envtest teardown**, and the 5s timeout was a mitigation. The failure in #512 can still be a consequence of that teardown (e.g. timeout fires, process continues, but coverage combine or Ginkgo coordination then fails).

### 4.2 DS Does Not Shut Down K8s Clients Before `envtest.Stop()`

- **Other suites** (e.g. WorkflowExecution, RemediationOrchestrator, SignalProcessing, AIAnalysis): they run a **controller** that uses the envtest API. In AfterSuite Phase 1 they typically **cancel the controller first** (`cancel()`), then call `testEnv.Stop()`. That stops watches and in-flight requests, so the API server can shut down with no active clients.
- **DataStorage**: there is **no controller**. The suite only has a raw `k8sClient` (used for dependency validation: create namespace, Secrets/ConfigMaps). In Phase 1 they call `cancel()` (the context used for that client) and then `dsTestEnv.Stop()` with a timeout. The **k8s client is never closed**; the context cancel may not immediately close all HTTP connections to the apiserver. So when `Stop()` runs, the envtest apiserver may still see **open connections** and hang during graceful shutdown (waiting for them to close). That matches the “envtest.Stop() can hang if …” comment in the DS suite.

So DS is more likely to hit a **hang in envtest.Stop()** because it’s the only integration suite that tears down envtest **without** first shutting down a controller that holds the main client usage; the only mitigation is the 5s timeout, which can then lead to exit code 2 or coverage combine failure if the process or Ginkgo coordination is left in a bad state.

### 4.3 Per-Process envtest and CI Load

- DS runs **one envtest per parallel process** (4 processes ⇒ 4 apiservers + 4 etcds), with **core API only** (no CRDs, no controller). All four are started in SynchronizedBeforeSuite Phase 2 and torn down in SynchronizedAfterSuite Phase 1.
- Other suites also use per-process envtest (and often a shared envtest in Phase 1), but they **do** shut down the controller before `testEnv.Stop()`, which likely puts envtest in a cleaner state.
- On CI, 4 envtest instances stopping at once can increase resource contention (CPU/memory/disk). DS’s teardown is more fragile (open client + no timeout in the underlying `Stop()`), so it’s more likely to surface as the one that fails.

### Summary

| Factor | DataStorage | Other integration jobs |
|--------|-------------|------------------------|
| envtest.Stop() | Wrapped in 5s timeout (because it was hanging) | Direct call, no timeout |
| Before Stop() | Only `cancel()` (context); k8s client not closed | Controller cancelled, then Stop() |
| Main envtest user | Raw k8sClient (create ns/Secrets/ConfigMaps) | Controller (watches, reconciler) |
| Result | Apiserver may see open connections → Stop() can hang → timeout → possible exit 2 / coverage combine failure | Teardown usually cleaner; if it hangs, job just runs to timeout |

So the failure is happening in DS and not in other jobs because **only DS** combines (1) envtest teardown that was already known to hang, (2) no controller shutdown to release client connections before `Stop()`, and (3) a timeout that can turn that hang into an exit code 2 or coordination/coverage failure instead of a long hang.

---

## 5. How Gateway (GW) Integration Handles the Same Teardown

Gateway integration tests use **envtest** in the same way (per-process envtest in Phase 2 of BeforeSuite, torn down in Phase 1 of AfterSuite). Comparison:

| Aspect | Gateway integration | DataStorage integration |
|--------|---------------------|-------------------------|
| **Teardown order** | **1. `cancel()`** then **2. `testEnv.Stop()`** | **1. `dsTestEnv.Stop()`** (with 5s timeout) then **2. `cancel()`** |
| **Timeout around Stop()** | None (direct `testEnv.Stop()`) | 5s goroutine + select |
| **What cancel() stops** | Context used by audit store (background flusher), K8s client operations | Context used by k8s client only (no controller) |
| **Must-gather** | Phase 2 runs MustGatherContainerLogs (DS/Postgres/Redis) then StopDSBootstrap | Phase 2 runs MustGatherContainerLogs (Postgres/Redis) then cleanupContainers |

**Why GW order helps**: Calling **`cancel()` first** stops any work that holds the context (e.g. audit store background writer, in-flight K8s calls). The API server then has fewer or no active clients when `Stop()` runs, so `envtest.Stop()` is less likely to hang. GW does **not** use a timeout; if `Stop()` ever hangs in GW, that job would block until the job timeout. So far GW has not needed a timeout, likely because cancel-first gives a cleaner shutdown.

**DS today**: DS does **Stop() first**, then **cancel()**. So the apiserver is torn down while the context is still live and the k8s client may still have open connections, which matches the observed hang. The 5s timeout was added so the process doesn’t block forever, but the underlying order is still wrong.

**E2E**: GW E2E uses a **Kind cluster**, not envtest. Teardown is `DeleteGatewayCluster()` (Kind delete). The envtest-hang problem does not apply to E2E.

---

## 6. Recommended Next Steps

1. **Align DS teardown order with GW** (high impact): In `test/integration/datastorage/suite_test.go` SynchronizedAfterSuite Phase 1, do **`cancel()` first**, then **`dsTestEnv.Stop()`** (keep the 5s timeout). That matches Gateway and lets the context cancel in-flight K8s usage before the apiserver is stopped.
2. **Re-run the failing job** (e.g. “Re-run failed jobs” for run #512) to see if the failure is flaky or consistent.
3. **Add a fallback artifact on integration failure** so we have something to inspect when must-gather is missing:
   - Run integration tests with output captured, e.g.  
     `make test-integration-datastorage 2>&1 | tee integration-datastorage.log`
   - On failure, upload `integration-datastorage.log` (and optionally any existing `/tmp/kubernaut-must-gather` contents) as artifacts. This gives at least logs and, when present, partial must-gather.
4. **Reproduce locally with CI-like parallelism**:
   ```bash
   TEST_PROCS=4 make test-integration-datastorage
   ```
   If it hangs, run with `TEST_PROCS=1` to see if the issue is parallel-only (e.g. envtest or coverage combine).
5. **Inspect next failed run** (if any) for:
   - “envtest.Stop() timed out after 5s”
   - “Ginkgo timed out waiting for all parallel procs”
   - “Failed to combine cover profiles”
   If any of these appear, treat Hypothesis A or B as confirmed and harden envtest teardown or coverage collection.

---

## 7. Summary Table

| Finding | Detail |
|--------|--------|
| **Failure** | Integration (datastorage) job failed with exit code 2; ~3m 11s. |
| **Must-gather** | No artifact — failure likely prevents SynchronizedAfterSuite Phase 2 from running. |
| **Likely cause** | Timeout or coordination failure (envtest.Stop() and/or coverage combine), consistent with run #509. |
| **Confidence** | Medium (no log artifact; inference from exit code, handoff, and code paths). |
| **Next** | Re-run job; add test-output artifact on failure; run locally with `TEST_PROCS=4`; re-check after next failure. |

---

## 8. References

- [PR #223 — DataStorage CI Stabilization Handoff](./PR_223_DS_CI_STABILIZATION_HANDOFF.md)
- [CI Run #512 — Integration (datastorage) job](https://github.com/jordigilh/kubernaut/actions/runs/22591522414/job/65451282088)
- Integration must-gather: `test/infrastructure/shared_integration_utils.go` — `MustGatherContainerLogs`
- DS integration suite: `test/integration/datastorage/suite_test.go` — SynchronizedAfterSuite, envtest timeout
- CI must-gather collection: `.github/workflows/ci-pipeline.yml` — “Collect must-gather logs on failure” (integration)
