# PR #223 — DataStorage CI Stabilization Handoff

**Date**: March 2, 2026
**Branch**: `feature/v1.0-bugfixes-demos`
**PR**: https://github.com/jordigilh/kubernaut/pull/223
**Last CI Run**: #511 (`edeb34e8b`) — in progress at time of handoff

---

## Current State

PR #223 is a large release-readiness branch (102 commits, 289+ files) covering bug fixes, demo scenarios, Helm infrastructure, and DD-WE-006 (schema-declared dependencies). The PR description has the full scope.

**CI status at handoff**: The last 3 runs on this branch:

| Run | Commit | Status | Notes |
|-----|--------|--------|-------|
| #511 | `edeb34e8b` | In progress | Includes all fixes below |
| #510 | `61f43d802` | Cancelled | Superseded by #511 |
| #509 | `00e0b882c` | Failed | DS E2E (5 failures), DS INT (coverage combine) |

The latest push (`edeb34e8b`) contains fixes for all known failures from run #509.

---

## Recent Work (Last Session — What Changed)

### Commits Pushed (chronological, oldest first)

| Commit | Description |
|--------|-------------|
| `13d1b7576` | Reduce burst from 120 to 80 (0.8x pool) to prevent DLQ interference |
| `2a916fb07` | Increase PostgreSQL `max_connections` from 100 to 200 for pool headroom |
| `39cca7549` | Wrap async persistence assertions in `Eventually` (JSONB + SOC2 tests) |
| `7d9be2821` | Clean up pre-existing anti-patterns in SOC2 tests (`ToNot(BeNil)` → `.Set`, `ToNot(BeEmpty)` → `HaveLen`/`BeNumerically`) |
| `da0fe7383` | Allow cascading tampered event IDs in hash chain verification (≥1 instead of exactly 1) |
| `159380b2c` | **Production bug fix**: Wire DLQ retry worker into Server lifecycle (DD-009 V1.0) |
| `00e0b882c` | Disable legacy `/incidents` endpoints not in OpenAPI spec |
| `61f43d802` | Mark connection pool burst test as `Serial` |
| `edeb34e8b` | Fix partition naming mismatch + envtest.Stop() hang in integration tests |

### Production Bug Fixed: DLQ Retry Worker Not Started (#248)

**Issue**: https://github.com/jordigilh/kubernaut/issues/248

The DLQ retry worker (`pkg/datastorage/server/dlq_retry_worker.go`) was fully implemented per DD-009 V1.0 but **never wired into `Server.Start()`**. Events receiving 202 Accepted (DLQ fallback) were written to Redis but never retried to PostgreSQL — they only got drained on graceful shutdown.

**Fix** (commit `159380b2c`):
- Added `dlqRetryWorker *DLQRetryWorker` field to `Server` struct
- Created worker in `NewServer()` with PID-based consumer name
- Started worker in `Server.Start()` before `ListenAndServe()`
- Stopped worker in `Server.Shutdown()` before DLQ drain step
- Changed `ReadMessages` from blocking (`Block: 0`) to non-blocking (`Block: -1`) so the goroutine responds to context cancellation
- Replaced `stopCh` channel with `context.CancelFunc` for clean lifecycle management

**Files changed**:
- `pkg/datastorage/server/server.go` — Server struct, NewServer, Start, Shutdown
- `pkg/datastorage/server/dlq_retry_worker.go` — Start, Stop, retryLoop, processRetryBatch
- `test/unit/datastorage/dlq/retry_worker_test.go` — New lifecycle unit test
- `test/unit/datastorage/dlq/suite_test.go` — New Ginkgo suite bootstrap

**Unit test**: `should start a background goroutine and stop cleanly without hanging` — proves Start/Stop lifecycle completes without deadlock (the RED phase exposed the blocking read bug).

### Issue #238 Updated: Legacy Endpoints

https://github.com/jordigilh/kubernaut/issues/238

Added `/incidents` and `/incidents/{id}` (BR-STORAGE-021) to the post-V1.0 evaluation issue alongside the already-disabled ADR-033 aggregate/success-rate endpoints. These endpoints are not in the OpenAPI spec, not in the ogen client, and not called by any service.

---

## CI Failures Diagnosed and Fixed

### E2E (datastorage) — 5 Failures in Run #509

**Root cause**: The connection pool burst test (`11_connection_pool_exhaustion_test.go`) fires 80 concurrent writes against a pool of 100 `maxOpenConns`. With 4 parallel Ginkgo processes sharing the same Kind cluster data-storage pod, the burst saturates the pool, causing other parallel tests to get 202 (DLQ fallback) instead of 201. The shared helper `createAuditEventOpenAPI` (`helpers_test.go:181`) fails on 202 responses.

**Tests that failed** (all with "DLQ fallback returned 202 Accepted"):
1. `03_query_api_timeline_test.go` — Query API setup write
2. `22_audit_validation_helper_test.go` — Validation helper test
3. `17_metrics_api_test.go` — Prometheus metrics write
4. `02_dlq_fallback_test.go` — DLQ fallback setup
5. `01_happy_path_test.go` — Happy path write

**Fix** (commit `61f43d802`): Added `Serial` decorator to the burst test `Describe` block. This test belongs in a dedicated performance tier and should be migrated there long-term; `Serial` is a temporary measure.

### Integration (datastorage) — Coverage Combine Failure in Run #509

**All 144 specs passed.** The failure was infrastructure:

```
Ginkgo timed out waiting for all parallel procs to report back
Failed to combine cover profiles
open .../coverage_integration_datastorage.out.1: no such file or directory
```

**Root cause** (two issues):

1. **envtest.Stop() hang**: `SynchronizedAfterSuite` calls `dsTestEnv.Stop()` on all 4 parallel processes. If `kube-apiserver` or `etcd` is already dead, `Stop()` hangs indefinitely, blocking the Ginkgo process from reporting back. The CI logs confirmed `SynchronizedAfterSuite` never ran (no cleanup log lines), and 4 orphan `kube-apiserver` + 4 orphan `etcd` processes were force-killed.

   **Fix** (commit `edeb34e8b`): Wrapped `dsTestEnv.Stop()` in a goroutine with a 5-second timeout via `select`. If Stop() hangs, cleanup proceeds.

2. **Partition naming mismatch**: `createDynamicPartitions` in `suite_test.go` used format `resource_action_traces_y2026m03`, but SQL migrations use `resource_action_traces_2026_03`. The existence check found nothing (wrong name), then CREATE failed with "would overlap" (same date range, different name). This logged warnings but didn't cause test failures directly.

   **Fix** (commit `edeb34e8b`): Changed format string from `_y%dm%02d` to `_%d_%02d` to match migrations.

---

## Key Files Modified in This Session

| File | What Changed |
|------|-------------|
| `pkg/datastorage/server/server.go` | DLQ worker wiring + disabled `/incidents` routes |
| `pkg/datastorage/server/dlq_retry_worker.go` | Context-based cancellation, non-blocking reads |
| `test/unit/datastorage/dlq/retry_worker_test.go` | Lifecycle unit test |
| `test/unit/datastorage/dlq/suite_test.go` | New Ginkgo suite |
| `test/e2e/datastorage/11_connection_pool_exhaustion_test.go` | `Serial` decorator |
| `test/e2e/datastorage/05_soc2_compliance_test.go` | Anti-pattern cleanup + cascading tamper fix |
| `test/e2e/datastorage/09_event_type_jsonb_comprehensive_test.go` | `Eventually` wrappers (reverted timeout increase) |
| `test/integration/datastorage/suite_test.go` | Partition naming fix + envtest.Stop() timeout |

---

## Open Issues Relevant to This PR

| Issue | Title | Status | Notes |
|-------|-------|--------|-------|
| #248 | DLQ retry worker not started | **Fixed** in `159380b2c` | Close after CI passes |
| #238 | Evaluate ADR-033 endpoints | Open | Post-V1.0 decision; `/incidents` added |
| #246 | EA healthScore incorrect | Open | Known demo issue (P10 gitops-drift) |
| #247 | Add actionType to Rego input | Open | Follow-up |
| #244 | Notification ConfigMap→FileWatcher | Open | Follow-up |
| #242 | Gateway exponential backoff | Open | Follow-up |
| #235 | Automatic partition creation | Open | Current static partitions extend through Dec 2028 |

---

## What to Watch in CI Run #511

1. **DS E2E**: The 5 "202 Accepted" failures should be gone now that the burst test is `Serial`. If any persist, investigate whether the DLQ retry worker is functioning correctly in the Kind cluster (check pod logs for `DLQ retry worker started`).

2. **DS Integration**: The coverage combine failure should be resolved by the envtest.Stop() timeout. If it recurs, add `--output-interceptor-mode=none` to the Ginkgo command in the Makefile to get subprocess output for debugging.

3. **DS Integration partitions**: The overlap warnings should disappear now that naming is aligned. If new overlap warnings appear, check if `030_extend_partitions_2028.sql` migration created partitions that conflict.

4. **All other services**: Were passing in run #509. No changes affect them.

---

## How to Run Tests Locally

```bash
# Unit tests (fast, no infrastructure)
make test-unit-datastorage

# Integration tests (requires Podman for PostgreSQL + Redis)
make test-integration-datastorage

# E2E tests (requires Kind cluster, ~5 min)
# Local: 12 parallel procs. CI: 4 parallel procs.
make test-e2e-datastorage
```

**Local vs CI difference**: `TEST_PROCS` defaults to `nproc` (12 locally, 4 in CI). The burst test's 80 concurrent writes against 100 `maxOpenConns` leaves less headroom in CI (80/100 with 4 processes vs 80/100 with 12 processes). The `Serial` fix addresses this.

---

## Architecture Decisions Referenced

| Decision | Relevance |
|----------|-----------|
| DD-009 | DLQ retry worker design — now correctly wired in Server lifecycle |
| DD-007 | Kubernetes-aware graceful shutdown — DLQ worker stops in Shutdown() |
| DD-008 | DLQ drain on shutdown — happens after worker.Stop() |
| ADR-034 | Unified audit events API — the `/incidents` endpoints predate this |
| ADR-033 | Success-rate endpoints — disabled, tracked in #238 |
| DD-AUTH-014 | Authenticated HTTP client for all API calls in E2E tests |
| DD-TEST-001 | Dynamic infrastructure for tests (envtest, Podman containers) |

---

## Conversation Reference

Full session transcript: [DS CI Stabilization](05b3c855-86f0-4578-ab4a-2d8613aa04a3)
