# E2E (remediationorchestrator) RCA – Must-Gather Analysis

**Date**: February 5, 2026  
**Run**: 21728498756 | **Job**: 62677186029 (E2E remediationorchestrator)  
**Artifact**: `must-gather-logs-e2e-remediationorchestrator-21728498756` (downloaded and extracted)

---

## 1. Must-gather availability

- **Artifact**: Present for run 21728498756 (~223 KB).
- **Content**: Kind cluster export (`kind export logs`) for cluster `ro-e2e` — pods, container logs, kubelet, journal, images, inspect.
- **Location (local)**: `_artifacts/run-21728498756/` (extracted from the downloaded tarball).

---

## 2. Root cause (evidence from logs)

### 2.1 DataStorage exited: PostgreSQL connection refused

**File**: `ro-e2e-control-plane/containers/datastorage-56c6d85d6d-k669g_kubernaut-system_datastorage-*.log`

DataStorage starts, tries to connect to PostgreSQL, and exits after 10 retries:

```
2026-02-05T21:15:24.680Z  Failed to connect, retrying...  attempt=1  error="... dial tcp 10.96.142.63:5432: connect: connection refused"
...
2026-02-05T21:15:42.706Z  ERROR  Failed to create server after all retries  attempts=10  error="... dial tcp 10.96.142.63:5432: connect: connection refused"
main.main
	/opt/app-root/src/cmd/datastorage/main.go:256
```

- **Total wait**: 10 retries × 2s = **20 seconds** (see `cmd/datastorage/main.go`: `maxRetries := 10`, `retryDelay := 2 * time.Second`).
- **Result**: DataStorage process exits with `os.Exit(1)`; pod goes to CrashLoopBackOff or Error.

So the **immediate** cause of the failure is: **DataStorage never connects to PostgreSQL within its 20s retry window**.

### 2.2 Why PostgreSQL wasn’t ready in time

- In `test/infrastructure/remediationorchestrator_e2e_hybrid.go`, **Phase 4** deploys in parallel:
  - PostgreSQL, Redis, Migrations, DataStorage (and related RBAC), RemediationOrchestrator, AuthWebhook.
- There is **no** “wait for PostgreSQL to be Ready” before applying the DataStorage deployment.
- So DataStorage is scheduled as soon as the manifest is applied; it starts and immediately tries to connect. If the PostgreSQL pod is still starting (image pull, init, first listen), the connection is refused.
- In CI, image pull and node load can make PostgreSQL take longer than ~20s to listen on 5432, so DataStorage’s 20s budget is too short.

### 2.3 How this turns into E2E exit code 2

- **waitForROServicesReady** (same file, ~699–732) waits for both:
  - DataStorage pod Ready, and  
  - RemediationOrchestrator pod Ready.
- DataStorage never becomes Ready (it crashes and stays CrashLoopBackOff/Error).
- So `waitForROServicesReady` does not succeed; it eventually times out (Gomega `Eventually(..., 2*time.Minute, 5*time.Second)` for DataStorage).
- **SetupROInfrastructureHybridWithCoverage** returns an error → **BeforeSuite** in `suite_test.go` fails at line 137 (`Expect(err).ToNot(HaveOccurred())`) → Ginkgo reports **exit code 2** (suite/setup failure).

RemediationOrchestrator itself started successfully (RO controller log shows normal startup at 21:13:30); the failure is the **DataStorage** pod never Ready due to PostgreSQL not being ready in time.

---

## 3. Root cause summary

| Item | Conclusion |
|------|------------|
| **Root cause** | DataStorage exits after 10×2s retries because PostgreSQL is not accepting connections yet. |
| **Why** | Parallel deployment with no wait for PostgreSQL before DataStorage; DataStorage allows only 20s total for DB connectivity. |
| **Where** | `cmd/datastorage/main.go` (retry count/delay); `test/infrastructure/remediationorchestrator_e2e_hybrid.go` (deploy order and lack of “wait for PostgreSQL” before DataStorage). |
| **CI impact** | In CI, PostgreSQL often takes longer than 20s to become ready → DataStorage fails → waitForROServicesReady times out → BeforeSuite fails → exit 2. |

---

## 4. Recommended fixes

### Option A (recommended): Wait for PostgreSQL before deploying DataStorage (E2E only)

In `test/infrastructure/remediationorchestrator_e2e_hybrid.go`:

- After deploying PostgreSQL (and optionally Redis), add an explicit wait for the PostgreSQL pod to be Ready (e.g. `kubectl wait --for=condition=ready pod -l app=postgresql -n kubernaut-system --timeout=120s` or equivalent).
- Only then deploy DataStorage (and the rest).
- No change to DataStorage binary; only E2E ordering.

**Pros**: Fixes the race at the source; CI and local both see PostgreSQL ready before DataStorage starts.  
**Cons**: Slightly longer E2E setup (adds ~30–60s typical wait).

### Option B: Increase DataStorage retry budget (E2E/CI)

- Make retries configurable (e.g. env `DATASTORAGE_DB_RETRY_ATTEMPTS` and `DATASTORAGE_DB_RETRY_DELAY`).
- In E2E (or CI), set higher attempts and/or delay (e.g. 30 attempts × 2s = 60s, or 10 × 6s = 60s).

**Pros**: Simple; no change to deploy order.  
**Cons**: DataStorage may sit in CrashLoopBackOff longer if PostgreSQL is genuinely slow; masks ordering issue.

### Option C: Both

- Add “wait for PostgreSQL” in RO E2E (Option A).
- Optionally increase DataStorage retries for E2E/CI (Option B) as a safety net.

---

## 5. Evidence paths in must-gather

| Evidence | Path in extracted artifact |
|----------|----------------------------|
| DataStorage crash (PostgreSQL connection refused) | `ro-e2e-control-plane/containers/datastorage-56c6d85d6d-k669g_kubernaut-system_datastorage-*.log` |
| RO controller healthy startup | `ro-e2e-control-plane/pods/kubernaut-system_remediationorchestrator-controller-79ff758595-kklhd_.../controller/0.log` |
| Cluster / node info | `ro-e2e-control-plane/inspect.json`, `ro-e2e-control-plane/kubelet.log` |
| Images and containerd | `ro-e2e-control-plane/images.log`, `ro-e2e-control-plane/containerd.log` |

---

## 6. References

- E2E_RO_CI_TRIAGE_RUN_21728498756_FEB_05_2026.md – Initial triage (exit 2, BeforeSuite).
- E2E_FAILURES_COMPLETE_ANALYSIS_FEB_04_2026.md – Must-gather and RO E2E timeout context.
- `cmd/datastorage/main.go` lines 238–267 – PostgreSQL/Redis retry logic.
- `test/infrastructure/remediationorchestrator_e2e_hybrid.go` – Phase 4 deploy and `waitForROServicesReady`.
