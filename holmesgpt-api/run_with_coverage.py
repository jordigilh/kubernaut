"""
E2E Coverage Launcher for HolmesGPT API (DD-TEST-007)

This module replaces the `coverage run -m uvicorn` pattern with explicit
Coverage object management. It exists because uvicorn's Server.startup()
calls loop.add_signal_handler(SIGTERM, ...) which OVERWRITES coverage.py's
built-in SIGTERM handler — making `coverage run --sigterm` ineffective.

Solution: create and control the Coverage object ourselves, register a
SIGUSR1 handler (which uvicorn does NOT intercept), and call cov.save()
explicitly when the E2E test harness signals us.

Signal flow:
  1. Go E2E test sends SIGUSR1 to this process via kubectl exec
  2. Our handler fires: cov.stop() → cov.save() → process keeps running
  3. Go test extracts .coverage via kubectl cp (pod still alive)
  4. Go test scales deployment to 0 → normal SIGTERM shutdown

This file is E2E infrastructure only — it is NOT part of the production
application source (src/). It is only used when E2E_COVERAGE=true.
"""

import os
import signal
import sys

import coverage

# ── Configure and start coverage ────────────────────────────────────────────
# Read configuration from environment (set by entrypoint.sh)
_data_file = os.environ.get("COVERAGE_FILE", "/coverdata/.coverage")
_source_dirs = ["src"]

cov = coverage.Coverage(data_file=_data_file, source=_source_dirs)
cov.start()

# Import the app AFTER starting coverage so all module-level code is measured
from src.main import app  # noqa: E402


# ── Signal handler ──────────────────────────────────────────────────────────
def _flush_coverage(signum, frame):
    """
    SIGUSR1 handler: flush coverage data without exiting.

    Keeping the process alive after flush is intentional — the Go E2E test
    needs the pod running to kubectl cp the .coverage file out. The test
    will scale the deployment to 0 afterwards for normal teardown.
    """
    sys.stdout.write("📊 SIGUSR1 received — flushing coverage data...\n")
    sys.stdout.flush()
    try:
        cov.stop()
        cov.save()
        sys.stdout.write(f"📊 Coverage saved to {_data_file}\n")
        sys.stdout.flush()
        # Restart coverage collection (defensive, in case requests arrive
        # between flush and scale-down — unlikely but harmless)
        cov.start()
    except Exception as exc:
        sys.stderr.write(f"⚠️  Coverage flush failed: {exc}\n")
        sys.stderr.flush()


# Register SIGUSR1 — uvicorn only overrides SIGTERM and SIGINT,
# so this handler survives uvicorn.run() startup.
signal.signal(signal.SIGUSR1, _flush_coverage)


# ── Run uvicorn ─────────────────────────────────────────────────────────────
if __name__ == "__main__":
    import uvicorn

    _port = int(os.environ.get("API_PORT", "8080"))

    sys.stdout.write(
        f"📊 E2E Coverage launcher active (SIGUSR1 → flush to {_data_file})\n"
    )
    sys.stdout.flush()

    # --loop asyncio: use standard event loop (uvloop not needed for E2E)
    # --workers 1: single worker for coverage coherence
    uvicorn.run(
        app,
        host="0.0.0.0",
        port=_port,
        loop="asyncio",
        workers=1,
    )

    # If uvicorn exits normally (e.g., via SIGTERM after scale-down),
    # do a final coverage save as a safety net.
    try:
        cov.stop()
        cov.save()
    except Exception:
        pass
