# SME Request: Python coverage.py + Uvicorn Signal Handling in Containers

**Date**: 2026-02-07
**Priority**: High
**Requesting Team**: Kubernaut Platform
**Branch**: `fix/e2e-coverage-extraction-aw-hapi`
**CI Run**: https://github.com/jordigilh/kubernaut/actions/runs/21771649017

---

## 1. Problem Statement

We cannot get `coverage.py` to flush its `.coverage` SQLite database when running
under `uvicorn` inside a Kubernetes pod (Kind cluster, Podman runtime).

The Python process receives `SIGTERM` (confirmed: `kill -TERM $PID` exits with
code 0), but the process **remains alive** for >20 seconds without writing the
`.coverage` file. The process state shows `STAT=Sl` (sleeping, multi-threaded)
throughout the entire polling window.

**Goal**: Reliably extract Python code coverage from a running `uvicorn`
FastAPI application in a container E2E test environment.

---

## 2. Environment Details

| Component              | Version / Detail                                       |
|------------------------|-------------------------------------------------------|
| Python                 | 3.12 (Red Hat UBI9 `python-312` image)                |
| Uvicorn                | 0.30.6 (pinned in `requirements-e2e.txt`)             |
| coverage.py            | >=7.0 (latest at pip install time)                    |
| FastAPI                | (version from requirements, not pinned exact)         |
| Container Image        | Multi-stage: `ubi9/python-312:latest`                 |
| Container Runtime      | Podman (GitHub Actions)                               |
| Orchestration          | Kind (Kubernetes in Docker/Podman)                    |
| Test Framework         | Ginkgo/Gomega (Go) driving E2E tests against the pod  |
| Pod User               | UID 1001 (non-root)                                   |
| PID 1                  | `/bin/bash ./entrypoint.sh` (NOT Python)              |
| Python PID             | Typically PID 20 (child of bash)                      |

---

## 3. Architecture

```
┌──────────────────────────────────────────────────┐
│  Kind Node (Podman container)                     │
│                                                   │
│  ┌─────────────────────────────────────────────┐  │
│  │  Pod: holmesgpt-api-xxxx                    │  │
│  │                                             │  │
│  │  PID 1: bash entrypoint.sh                  │  │
│  │    └─ PID 20: python3.12 -m coverage run \  │  │
│  │         --rcfile=/tmp/.coveragerc \          │  │
│  │         -m uvicorn src.main:app \            │  │
│  │         --host 0.0.0.0 --port 8080 \         │  │
│  │         --workers 1 --loop asyncio           │  │
│  │                                             │  │
│  │  /coverdata/  (hostPath, 0777)              │  │
│  │    Expected: .coverage SQLite DB            │  │
│  └─────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────┘

Coverage Collection (from Go test):
  1. Find running pod
  2. kubectl exec: find Python PID, send signal
  3. Poll /coverdata/.coverage for up to 20s
  4. kubectl cp .coverage file out of pod
```

---

## 4. What We Want to Happen

1. E2E tests finish
2. Go test code sends a signal to the Python process inside the pod
3. `coverage.py` saves its accumulated coverage data to `/coverdata/.coverage`
4. Go test code extracts the file via `kubectl cp`
5. Coverage report is generated

---

## 5. Current Entrypoint Script (E2E Coverage Block)

```bash
# Inside: if [ "$E2E_COVERAGE" = "true" ]; then
# (after verifying coverage module exists)

cat > /tmp/.coveragerc <<RCEOF
[run]
sigterm = true
source = src
data_file = /coverdata/.coverage
RCEOF

# Python runs in background, bash stays PID 1
python3.12 -m coverage run --rcfile=/tmp/.coveragerc \
    -m uvicorn src.main:app \
    --host 0.0.0.0 --port 8080 \
    --workers 1 --loop asyncio &
PID=$!

# Forward SIGTERM from bash to Python
trap 'echo "Received SIGTERM, forwarding to Python (PID $PID)..."; kill -TERM $PID 2>/dev/null' SIGTERM SIGINT

# Double-wait pattern: first wait is interrupted by signal,
# second wait blocks until Python actually exits
set +e
wait $PID 2>/dev/null
wait $PID 2>/dev/null
EXIT_CODE=$?
set -e

# Verify coverage file (diagnostic)
COV_FILE="${COVERAGE_FILE:-/coverdata/.coverage}"
if [ -f "$COV_FILE" ]; then
    echo "Coverage file written: $COV_FILE"
else
    echo "Coverage file NOT found at $COV_FILE after Python exit"
fi
exit $EXIT_CODE
```

---

## 6. Current Go-side Signal Delivery (test/infrastructure/coverage.go)

```go
// Step 3: Send SIGTERM directly to the Python/coverage process (NOT PID 1)
killCmd := exec.Command("kubectl", "--kubeconfig", opts.KubeconfigPath,
    "exec", "-n", opts.Namespace, podName, "--",
    "sh", "-c", `
PYTHON_PID=$(ps -eo pid,comm,args | grep '[p]ython3.*coverage run' | awk '{print $1}' | head -1)
if [ -n "$PYTHON_PID" ]; then
    echo "Found Python/coverage process: PID $PYTHON_PID"
    kill -TERM "$PYTHON_PID"
    echo "SIGTERM sent to PID $PYTHON_PID (exit=$?)"
else
    echo "No Python/coverage process found, sending SIGTERM to PID 1 as fallback"
    kill -TERM 1
    echo "SIGTERM sent to PID 1 (exit=$?)"
fi
`)

// Step 4: Poll for .coverage for up to 20 seconds (1s intervals)
```

---

## 7. Observed Behavior (CI Logs)

```
📋 Checking /coverdata/ inside pod before SIGTERM...
total 360
drwxrwxrwx 3 root root   4096 Feb  7 01:33 .
drwxr-xr-x 1 root root   4096 Feb  7 01:32 ..
-rw-r--r-- 1 root root 352906 Feb  7 01:31 covmeta.4490dfe72bcc10fd30e5d9a507a0dee5
drwxr-xr-x 2 root root   4096 Feb  7 01:33 holmesgpt-api
---
[run]
sigterm = true
source = src
data_file = /coverdata/.coverage

📤 Sending SIGTERM to Python process inside pod...
   Found Python/coverage process: PID 20
SIGTERM sent to PID 20 (exit=0)
⏳ Waiting for coverage data flush (up to 20s)...
   ⚠️  .coverage not detected after 20s
   Process state:
USER         PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
default        1  0.0  0.0   7272  3608 ?        Ss   01:32   0:00 /bin/bash ./entrypoint.sh
default       20 52.4  2.4 1385788 404184 ?      Sl   01:32   0:27 python3.12 -m coverage run --rcfile=/tmp/.coveragerc -m uvicorn src.main:app --host 0.0.0.0 --port 8080 --workers 1 --loop asyncio
```

**Key observations**:
- `kill -TERM $PID` reports `exit=0` (signal was delivered successfully)
- Python process (PID 20) **remains running** after 20 seconds
- Process state is `Sl` (sleeping, multi-threaded) -- not zombie, not exiting
- No `.coverage` file is ever written
- `/coverdata/` directory is writable (0777, confirmed by write-test in entrypoint)
- `.coveragerc` is correct (`sigterm = true`, `data_file = /coverdata/.coverage`)

---

## 8. Approaches Already Tried (Do NOT Recommend These)

### Approach 1: `exec` Python as PID 1
**What**: Replaced bash with `exec python3.12 -m coverage run ...` so Python was PID 1.
**Result**: Failed. Uvicorn registers its own SIGTERM handler that overrides coverage.py's handler. When Kubernetes sends SIGTERM, uvicorn shuts down but coverage.py's atexit handler never runs.

### Approach 2: hostPath volume for coverage data
**What**: Relied on hostPath volume mounts (pod → Kind node → host) to extract `.coverage` after pod termination.
**Result**: Failed. The hostPath mount chain through Kind + Podman was unreliable. Coverage directory on Kind node was always empty for HAPI.

### Approach 3: bash PID 1 + signal forwarding (no double-wait)
**What**: Bash stays PID 1, Python runs in background. Bash trap forwards SIGTERM to Python. Single `wait $PID`.
**Result**: Failed. Bash exits before Python finishes flushing `.coverage`. Container dies, file never written.

### Approach 4: bash PID 1 + double-wait
**What**: Same as #3 but added a second `wait $PID` call after the trap runs, to block until Python actually exits.
**Result**: Failed. SIGTERM sent to PID 1 (bash) via `kubectl exec kill -TERM 1` was not processed.

### Approach 5: Send SIGTERM directly to Python PID (found via `ps`)
**What**: Instead of signaling PID 1, used `ps -eo pid,comm,args | grep 'python3.*coverage run'` to find the Python PID and sent `kill -TERM $PID` directly.
**Result**: Failed. `kill` reports success (exit=0), but Python process remains running. `.coverage` never written. This is the current state.

### Approach 6: Disable uvloop (`--loop asyncio`)
**What**: Added `--loop asyncio` to the uvicorn command to force the standard asyncio event loop instead of uvloop. Hypothesis was that uvloop's `loop.add_signal_handler()` was eating the SIGTERM.
**Result**: Failed. Same behavior -- Python still alive after 20s, no `.coverage` written. This rules out uvloop as the specific cause.

### Approach 7: Complex signal sequence (SIGTERM then SIGINT via Python os.kill)
**What**: Used `kubectl exec` to run `python3 -c "import os; os.kill($PID, 15)"` for SIGTERM, waited, then sent SIGINT as fallback. Added `/proc/$PID/status` diagnostics.
**Result**: Failed and reverted. Same core issue -- uvicorn's signal handler prevents coverage.py from flushing. Added unnecessary complexity.

### Approach 8: `fsGroup` in pod SecurityContext
**What**: Set `fsGroup: 1001` on the HAPI pod's security context to ensure coverage files had correct group ownership on the hostPath volume.
**Result**: Ineffective. Removed. The permission issue was solved by `chmod 777` on the coverdata directory.

---

## 9. Root Cause Hypothesis

`uvicorn` registers its own signal handlers via the asyncio event loop:

```python
# Uvicorn's Server.startup() calls:
loop.add_signal_handler(signal.SIGTERM, self.handle_exit, ...)
loop.add_signal_handler(signal.SIGINT, self.handle_exit, ...)
```

This **replaces** coverage.py's SIGTERM handler (installed when `sigterm = true` in `.coveragerc`). When SIGTERM arrives:
1. Uvicorn's handler sets `self.should_exit = True`
2. Uvicorn begins graceful shutdown (closing connections, waiting for in-flight requests)
3. But the shutdown does not complete within 20 seconds (possibly waiting on something)
4. coverage.py's handler **never runs** because uvicorn replaced it
5. coverage.py's `atexit` handler **never runs** because the process doesn't exit

This happens with **both** uvloop and asyncio event loops because the `loop.add_signal_handler()` call is in uvicorn's core `Server` class, not in the event loop implementation.

---

## 10. Questions for the SME

1. **Is there a way to chain signal handlers in Python's asyncio?** Can we ensure
   coverage.py's SIGTERM handler runs *after* (or alongside) uvicorn's handler?

2. **Would `SIGUSR1` work?** Uvicorn only registers handlers for SIGTERM and SIGINT.
   If we send `SIGUSR1` to the Python process with a custom handler that calls
   `coverage.Coverage.current().save()` and then `os._exit(0)`, would this bypass
   uvicorn's signal interception entirely?

3. **Is there a coverage.py API we can use?** For example:
   - Can we call `coverage.Coverage.current().save()` from within the FastAPI
     application code (e.g., a shutdown endpoint)?
   - Is `coverage.process_startup()` relevant here?

4. **Would a `/save-coverage` HTTP endpoint be more reliable?** Instead of using
   signals at all, we could add a FastAPI endpoint that calls
   `coverage.Coverage.current().save()`. The Go E2E test would `curl` this
   endpoint before tearing down the pod.

5. **Is there a uvicorn configuration to preserve pre-existing signal handlers?**
   Something like a callback or hook that runs before uvicorn installs its own
   handlers, or a way to tell uvicorn to chain rather than replace?

6. **Could `coverage run --sigterm` + `uvicorn --timeout-graceful-shutdown=N` help?**
   If we set a very short graceful shutdown timeout, would uvicorn exit faster,
   allowing coverage.py's atexit handler to fire?

7. **Are there known workarounds in the coverage.py or uvicorn communities?**
   This seems like it would be a common problem for any Python service wanting
   coverage in containers.

8. **Would wrapping uvicorn differently help?** E.g., using `coverage run` with
   a Python script that starts uvicorn programmatically (via `uvicorn.run()`)
   instead of `python -m coverage run -m uvicorn ...`? This might give us more
   control over signal handler registration order.

---

## 11. Constraints

- The Python application code (`src/`) should **not** contain test-specific logic
  (the user has explicitly requested this)
- The solution must work with non-root user (UID 1001)
- The solution must work in a Kubernetes pod (Kind cluster, Podman runtime)
- coverage.py data file is at `/coverdata/.coverage` (hostPath mount, writable)
- The Go E2E test framework controls the signal delivery and file extraction
- We have full control over `entrypoint.sh` and the `Dockerfile.e2e`
- Adding Python packages to `requirements-e2e.txt` is acceptable
- The coverage collection is E2E-only (guarded by `E2E_COVERAGE=true` env var)

---

## 12. Relevant Files

| File                                              | Purpose                                        |
|---------------------------------------------------|-------------------------------------------------|
| `holmesgpt-api/entrypoint.sh`                     | Container entrypoint (E2E coverage block)       |
| `holmesgpt-api/Dockerfile.e2e`                    | E2E Docker image build                          |
| `holmesgpt-api/requirements-e2e.txt`              | Python dependencies for E2E                     |
| `test/infrastructure/coverage.go`                 | Go-side coverage extraction logic               |
| `test/infrastructure/holmesgpt_api.go`            | HAPI Kind cluster setup                         |

---

## 13. What a Good Solution Looks Like

- The `.coverage` file is reliably written within 5-10 seconds of receiving the signal
- Python process exits cleanly after writing coverage
- No test-specific code in the production application source
- Works consistently in CI (GitHub Actions, Podman, Kind)
- Minimal changes to entrypoint.sh and/or Go test infrastructure

---

## 14. Note on AuthWebhook (Go Service) -- SOLVED

The AuthWebhook service (Go, not Python) had a similar E2E coverage reporting
issue that has been **resolved**. The fix involved:
- `os.Chmod(coverdataPath, 0777)` after `os.MkdirAll`
- `ensureCoverdataWritableInKindNode` (podman exec chmod)
- Go's `GOCOVERDIR` flushes on SIGTERM natively (no signal handler issues)

AW now reports ~33% E2E coverage. The HAPI Python issue is isolated.
