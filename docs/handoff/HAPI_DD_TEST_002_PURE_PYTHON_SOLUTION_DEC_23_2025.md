# HAPI DD-TEST-002 Compliance: Pure Python Sequential Startup Solution

**Date**: December 23, 2025
**Status**: ✅ **RECOMMENDED SOLUTION** (replaces Option D)
**Approach**: Python implementation of DD-TEST-002 sequential startup pattern
**Effort**: 1 day (lower complexity than cross-language Go approach)

---

## 🎯 **Executive Summary**

Instead of documenting HAPI as a DD-TEST-002 exception OR using cross-language Go infrastructure, we can implement the **DD-TEST-002 sequential startup pattern directly in Python**.

### **Why This is Better**

| Criterion | Option D (Exception) | Option A (Go CLI) | **Option E (Pure Python)** |
|-----------|---------------------|-------------------|----------------------------|
| **DD-TEST-002 Compliance** | ⚠️ Intent only | ✅ 100% | ✅ **100%** |
| **Cross-Language Complexity** | ✅ None | 🔴 High | ✅ **None** |
| **Developer Experience** | ✅ Excellent | ❌ Poor | ✅ **Excellent** |
| **Maintenance Burden** | ✅ Low | 🔴 High | ✅ **Low** |
| **Implementation Effort** | ✅ 1 hour | 🔴 2-3 days | ⚠️ **1 day** |
| **Reliability** | ✅ Proven | ⚠️ Unknown | ✅ **Proven pattern** |
| **Self-Contained** | ✅ Yes | ❌ No | ✅ **Yes** |

**Result**: Best of all worlds - DD-TEST-002 compliant, Python-native, self-contained.

---

## 🔧 **Implementation: Pure Python Sequential Startup**

### **Architecture**

```
holmesgpt-api/tests/integration/
├── infrastructure.py          # NEW: Sequential startup module (Python)
├── conftest.py                # UPDATED: Use infrastructure.py
├── config/
│   ├── postgres.env           # Container environment variables
│   ├── redis.env
│   ├── datastorage.env
│   └── embedding.env
├── docker-compose.workflow-catalog.yml  # DEPRECATED (kept for reference)
├── setup_workflow_catalog_integration.sh  # DEPRECATED
└── teardown_workflow_catalog_integration.sh  # DEPRECATED
```

**Key Insight**: DD-TEST-002's `podman run` pattern can be replicated using Python's `subprocess` module, just like Go uses `exec.Command`.

---

## 📝 **Implementation Code**

### **File 1: `infrastructure.py`** (NEW - 250 lines)

```python
"""
HAPI Integration Test Infrastructure - DD-TEST-002 Compliant Sequential Startup

This module implements the DD-TEST-002 sequential container orchestration pattern
in pure Python, eliminating the need for podman-compose for multi-service dependencies.

Reference: docs/architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md
"""

import subprocess
import time
import os
import sys
from typing import Optional, Dict, List
from pathlib import Path


class ContainerOrchestrator:
    """
    Sequential container startup following DD-TEST-002 pattern.

    Implements the same pattern as Go services but in pure Python:
    1. Cleanup existing containers
    2. Create network
    3. Start PostgreSQL → wait for ready
    4. Start Redis → wait for ready
    5. Start DataStorage → wait for HTTP health
    6. Start Embedding Service → wait for HTTP health
    """

    # DD-TEST-001 Port allocations for HAPI integration tests
    POSTGRES_PORT = 15435
    REDIS_PORT = 16381
    DATASTORAGE_HTTP_PORT = 18094
    DATASTORAGE_METRICS_PORT = 19095
    EMBEDDING_PORT = 18001

    # Container names
    NETWORK_NAME = "hapi_test_network"
    POSTGRES_CONTAINER = "hapi_postgres_integration"
    REDIS_CONTAINER = "hapi_redis_integration"
    DATASTORAGE_CONTAINER = "hapi_datastorage_integration"
    EMBEDDING_CONTAINER = "hapi_embedding_integration"

    # Database credentials (aligned with Go services per DD-TEST-002)
    DB_USER = "slm_user"
    DB_PASSWORD = "test_password"
    DB_NAME = "action_history"

    def __init__(self, verbose: bool = True):
        """Initialize orchestrator with optional verbose logging."""
        self.verbose = verbose
        self.project_root = Path(__file__).parent.parent.parent.parent

    def _log(self, message: str):
        """Print log message if verbose mode enabled."""
        if self.verbose:
            print(f"🔧 {message}", flush=True)

    def _run_command(
        self,
        cmd: List[str],
        check: bool = True,
        capture_output: bool = False
    ) -> subprocess.CompletedProcess:
        """
        Run shell command with error handling.

        Args:
            cmd: Command and arguments as list
            check: Raise exception on non-zero exit
            capture_output: Capture stdout/stderr

        Returns:
            CompletedProcess with result

        Raises:
            RuntimeError: If command fails and check=True
        """
        if self.verbose:
            self._log(f"Running: {' '.join(cmd)}")

        result = subprocess.run(
            cmd,
            check=False,
            capture_output=capture_output,
            text=True
        )

        if check and result.returncode != 0:
            error_msg = f"Command failed: {' '.join(cmd)}"
            if capture_output:
                error_msg += f"\nstdout: {result.stdout}\nstderr: {result.stderr}"
            raise RuntimeError(error_msg)

        return result

    def cleanup_containers(self):
        """
        Stop and remove existing test containers.

        Per DD-TEST-002 §3.1: Clean slate before starting infrastructure
        """
        self._log("Cleaning up existing containers...")

        containers = [
            self.POSTGRES_CONTAINER,
            self.REDIS_CONTAINER,
            self.DATASTORAGE_CONTAINER,
            self.EMBEDDING_CONTAINER,
        ]

        for container in containers:
            # Stop (ignore errors if not running)
            self._run_command(
                ["podman", "stop", container],
                check=False
            )
            # Remove (ignore errors if doesn't exist)
            self._run_command(
                ["podman", "rm", "-f", container],
                check=False
            )

        self._log("✅ Cleanup complete")

    def create_network(self):
        """
        Create test network (idempotent).

        Per DD-TEST-002 §3.2: Isolated network for test containers
        """
        self._log(f"Creating network: {self.NETWORK_NAME}...")

        # Create network (ignore error if already exists)
        self._run_command(
            ["podman", "network", "create", self.NETWORK_NAME],
            check=False
        )

        self._log("✅ Network ready")

    def start_postgres(self):
        """
        Start PostgreSQL container.

        Per DD-TEST-002 §3.3: Start database FIRST (critical dependency)
        """
        self._log("Starting PostgreSQL...")

        self._run_command([
            "podman", "run", "-d",
            "--name", self.POSTGRES_CONTAINER,
            "--network", self.NETWORK_NAME,
            "-p", f"{self.POSTGRES_PORT}:5432",
            "-e", f"POSTGRES_USER={self.DB_USER}",
            "-e", f"POSTGRES_PASSWORD={self.DB_PASSWORD}",
            "-e", f"POSTGRES_DB={self.DB_NAME}",
            "postgres:16-alpine"
        ])

        self._log("✅ PostgreSQL container started")

    def wait_for_postgres(self, timeout: int = 30):
        """
        Wait for PostgreSQL to be ready.

        Per DD-TEST-002 §3.4: Explicit wait is CRITICAL to avoid race conditions

        Args:
            timeout: Maximum seconds to wait

        Raises:
            TimeoutError: If PostgreSQL not ready within timeout
        """
        self._log("⏳ Waiting for PostgreSQL to be ready...")

        for i in range(timeout):
            result = self._run_command(
                ["podman", "exec", self.POSTGRES_CONTAINER,
                 "pg_isready", "-U", self.DB_USER],
                check=False,
                capture_output=True
            )

            if result.returncode == 0:
                self._log(f"✅ PostgreSQL ready (took {i+1}s)")
                return

            time.sleep(1)

        raise TimeoutError(f"PostgreSQL not ready after {timeout}s")

    def start_redis(self):
        """Start Redis container."""
        self._log("Starting Redis...")

        self._run_command([
            "podman", "run", "-d",
            "--name", self.REDIS_CONTAINER,
            "--network", self.NETWORK_NAME,
            "-p", f"{self.REDIS_PORT}:6379",
            "redis:7-alpine"
        ])

        self._log("✅ Redis container started")

    def wait_for_redis(self, timeout: int = 10):
        """Wait for Redis to be ready."""
        self._log("⏳ Waiting for Redis to be ready...")

        for i in range(timeout):
            result = self._run_command(
                ["podman", "exec", self.REDIS_CONTAINER,
                 "redis-cli", "ping"],
                check=False,
                capture_output=True
            )

            if result.returncode == 0 and "PONG" in result.stdout:
                self._log(f"✅ Redis ready (took {i+1}s)")
                return

            time.sleep(1)

        raise TimeoutError(f"Redis not ready after {timeout}s")

    def start_datastorage(self):
        """
        Start DataStorage container.

        Per DD-TEST-002 §3.8: Start DataStorage AFTER dependencies are ready
        """
        self._log("Starting DataStorage...")

        # Build DataStorage image if not exists
        self._build_datastorage_image()

        self._run_command([
            "podman", "run", "-d",
            "--name", self.DATASTORAGE_CONTAINER,
            "--network", self.NETWORK_NAME,
            "-p", f"{self.DATASTORAGE_HTTP_PORT}:8080",
            "-p", f"{self.DATASTORAGE_METRICS_PORT}:9090",
            "-e", f"DB_HOST={self.POSTGRES_CONTAINER}",
            "-e", "DB_PORT=5432",
            "-e", f"DB_NAME={self.DB_NAME}",
            "-e", f"DB_USER={self.DB_USER}",
            "-e", f"DB_PASSWORD={self.DB_PASSWORD}",
            "-e", f"REDIS_HOST={self.REDIS_CONTAINER}",
            "-e", "REDIS_PORT=6379",
            "-e", "LOG_LEVEL=INFO",
            "localhost/kubernaut-datastorage:integration-test"
        ])

        self._log("✅ DataStorage container started")

    def _build_datastorage_image(self):
        """Build DataStorage Docker image if needed."""
        # Check if image exists
        result = self._run_command(
            ["podman", "images", "-q", "localhost/kubernaut-datastorage:integration-test"],
            check=False,
            capture_output=True
        )

        if result.stdout.strip():
            self._log("DataStorage image exists, skipping build")
            return

        self._log("Building DataStorage image...")

        datastorage_dir = self.project_root / "services" / "datastorage"

        self._run_command(
            ["podman", "build",
             "-t", "localhost/kubernaut-datastorage:integration-test",
             "-f", "Dockerfile",
             "."],
            check=True
        )

        self._log("✅ DataStorage image built")

    def wait_for_datastorage(self, timeout: int = 30):
        """
        Wait for DataStorage HTTP health check.

        Per DD-TEST-002 §3.9: Verify HTTP endpoint is responding
        """
        self._log("⏳ Waiting for DataStorage health check...")

        import urllib.request
        import urllib.error

        health_url = f"http://localhost:{self.DATASTORAGE_HTTP_PORT}/health"

        for i in range(timeout):
            try:
                with urllib.request.urlopen(health_url, timeout=1) as response:
                    if response.status == 200:
                        self._log(f"✅ DataStorage healthy (took {i+1}s)")
                        return
            except (urllib.error.URLError, TimeoutError):
                pass

            time.sleep(1)

        raise TimeoutError(f"DataStorage not healthy after {timeout}s")

    def start_embedding_service(self):
        """Start Embedding Service container (HAPI-specific)."""
        self._log("Starting Embedding Service...")

        # Build Embedding Service image if not exists
        self._build_embedding_image()

        self._run_command([
            "podman", "run", "-d",
            "--name", self.EMBEDDING_CONTAINER,
            "--network", self.NETWORK_NAME,
            "-p", f"{self.EMBEDDING_PORT}:8086",
            "-e", "LOG_LEVEL=INFO",
            "localhost/kubernaut-embedding-service:integration-test"
        ])

        self._log("✅ Embedding Service container started")

    def _build_embedding_image(self):
        """Build Embedding Service Docker image if needed."""
        result = self._run_command(
            ["podman", "images", "-q", "localhost/kubernaut-embedding-service:integration-test"],
            check=False,
            capture_output=True
        )

        if result.stdout.strip():
            self._log("Embedding Service image exists, skipping build")
            return

        self._log("Building Embedding Service image...")

        embedding_dir = self.project_root / "embedding-service"

        self._run_command(
            ["podman", "build",
             "-t", "localhost/kubernaut-embedding-service:integration-test",
             "-f", "Dockerfile",
             "."],
            check=True
        )

        self._log("✅ Embedding Service image built")

    def wait_for_embedding(self, timeout: int = 30):
        """Wait for Embedding Service HTTP health check."""
        self._log("⏳ Waiting for Embedding Service health check...")

        import urllib.request
        import urllib.error

        health_url = f"http://localhost:{self.EMBEDDING_PORT}/health"

        for i in range(timeout):
            try:
                with urllib.request.urlopen(health_url, timeout=1) as response:
                    if response.status == 200:
                        self._log(f"✅ Embedding Service healthy (took {i+1}s)")
                        return
            except (urllib.error.URLError, TimeoutError):
                pass

            time.sleep(1)

        raise TimeoutError(f"Embedding Service not healthy after {timeout}s")

    def start_all(self):
        """
        Start all infrastructure in DD-TEST-002 sequential order.

        Implements the exact pattern from DD-TEST-002:
        1. Cleanup → 2. Network → 3. PostgreSQL (wait) →
        4. Redis (wait) → 5. DataStorage (wait) → 6. Embedding (wait)
        """
        self._log("🚀 Starting HAPI integration test infrastructure (DD-TEST-002 sequential pattern)")

        try:
            # Phase 1: Cleanup
            self.cleanup_containers()

            # Phase 2: Network setup
            self.create_network()

            # Phase 3: PostgreSQL (CRITICAL - must be first)
            self.start_postgres()
            self.wait_for_postgres()

            # Phase 4: Redis
            self.start_redis()
            self.wait_for_redis()

            # Phase 5: DataStorage (depends on PostgreSQL + Redis)
            self.start_datastorage()
            self.wait_for_datastorage()

            # Phase 6: Embedding Service (HAPI-specific)
            self.start_embedding_service()
            self.wait_for_embedding()

            self._log("✅ All infrastructure started successfully!")
            self._log("")
            self._log("📊 Service Endpoints:")
            self._log(f"  - PostgreSQL:         localhost:{self.POSTGRES_PORT}")
            self._log(f"  - Redis:              localhost:{self.REDIS_PORT}")
            self._log(f"  - DataStorage HTTP:   http://localhost:{self.DATASTORAGE_HTTP_PORT}")
            self._log(f"  - DataStorage Metrics: http://localhost:{self.DATASTORAGE_METRICS_PORT}")
            self._log(f"  - Embedding Service:  http://localhost:{self.EMBEDDING_PORT}")

        except Exception as e:
            self._log(f"❌ Infrastructure startup failed: {e}")
            self._log("🧹 Cleaning up containers...")
            self.cleanup_containers()
            raise

    def stop_all(self):
        """Stop and remove all infrastructure."""
        self._log("🧹 Stopping HAPI integration test infrastructure...")
        self.cleanup_containers()
        self._log("✅ Infrastructure stopped")


# Convenience functions for pytest integration
def start_infrastructure(verbose: bool = True) -> ContainerOrchestrator:
    """
    Start HAPI integration test infrastructure.

    Usage:
        orchestrator = start_infrastructure()

    Returns:
        ContainerOrchestrator instance for cleanup
    """
    orchestrator = ContainerOrchestrator(verbose=verbose)
    orchestrator.start_all()
    return orchestrator


def stop_infrastructure(orchestrator: Optional[ContainerOrchestrator] = None):
    """
    Stop HAPI integration test infrastructure.

    Args:
        orchestrator: Orchestrator instance (or create new if None)
    """
    if orchestrator is None:
        orchestrator = ContainerOrchestrator(verbose=False)
    orchestrator.stop_all()


# CLI for manual testing
if __name__ == "__main__":
    import sys

    if len(sys.argv) < 2:
        print("Usage: python infrastructure.py [start|stop]")
        sys.exit(1)

    action = sys.argv[1]

    if action == "start":
        start_infrastructure()
    elif action == "stop":
        stop_infrastructure()
    else:
        print(f"Unknown action: {action}")
        sys.exit(1)
```

---

### **File 2: `conftest.py`** (UPDATED - 50 lines)

```python
"""
HAPI Integration Test Fixtures - DD-TEST-002 Compliant

This module provides pytest fixtures for HAPI integration tests using
the DD-TEST-002 sequential container orchestration pattern.

Reference: DD-TEST-002 §3 - Sequential Startup Pattern
"""

import pytest
import os
from typing import Generator

# Import our DD-TEST-002 compliant infrastructure
from tests.integration.infrastructure import (
    ContainerOrchestrator,
    start_infrastructure,
    stop_infrastructure
)


# Session-scoped fixture: Start infrastructure once for all tests
_orchestrator: ContainerOrchestrator | None = None


def is_integration_infra_available() -> bool:
    """
    Check if infrastructure is already running (manual start).

    Allows developers to start infrastructure manually and run tests
    without automatic startup/teardown.

    Returns:
        True if DataStorage health check responds
    """
    import urllib.request
    import urllib.error

    try:
        with urllib.request.urlopen("http://localhost:18094/health", timeout=1) as response:
            return response.status == 200
    except (urllib.error.URLError, TimeoutError, ConnectionRefusedError):
        return False


@pytest.fixture(scope="session", autouse=True)
def integration_infrastructure() -> Generator[ContainerOrchestrator, None, None]:
    """
    Auto-start integration test infrastructure for entire test session.

    Implements DD-TEST-002 sequential startup pattern:
    - Cleanup existing containers
    - Start PostgreSQL → Redis → DataStorage → Embedding (sequentially)
    - Wait for health checks at each step

    Per DD-TEST-002: This eliminates race conditions from podman-compose.

    Yields:
        ContainerOrchestrator instance for test access
    """
    global _orchestrator

    # Skip if manually started (developer convenience)
    if is_integration_infra_available():
        print("\n✅ Infrastructure already running, skipping auto-start")
        yield None
        return

    # Start infrastructure using DD-TEST-002 sequential pattern
    print("\n🚀 Starting integration test infrastructure (DD-TEST-002 sequential pattern)...")
    _orchestrator = start_infrastructure(verbose=True)

    yield _orchestrator

    # Cleanup after all tests complete
    print("\n🧹 Cleaning up integration test infrastructure...")
    stop_infrastructure(_orchestrator)
    _orchestrator = None


@pytest.fixture
def datastorage_url() -> str:
    """Provide DataStorage HTTP URL for tests."""
    return f"http://localhost:{ContainerOrchestrator.DATASTORAGE_HTTP_PORT}"


@pytest.fixture
def embedding_url() -> str:
    """Provide Embedding Service URL for tests."""
    return f"http://localhost:{ContainerOrchestrator.EMBEDDING_PORT}"


@pytest.fixture
def postgres_connection_string() -> str:
    """Provide PostgreSQL connection string for tests."""
    return (
        f"postgresql://{ContainerOrchestrator.DB_USER}:"
        f"{ContainerOrchestrator.DB_PASSWORD}@localhost:"
        f"{ContainerOrchestrator.POSTGRES_PORT}/{ContainerOrchestrator.DB_NAME}"
    )
```

---

## 📊 **Comparison: Python vs Go Implementation**

| Aspect | Go CLI Approach (Option A) | Pure Python (Option E) |
|--------|---------------------------|------------------------|
| **Language** | Go (cross-language) | Python (native) |
| **Lines of Code** | ~250 Go + ~50 Python | ~300 Python |
| **Dependencies** | Go compiler, Go knowledge | Python stdlib only |
| **Maintenance** | Go team + Python team | Python team only |
| **Developer Experience** | Requires Go setup | Native Python workflow |
| **Debugging** | Cross-language boundary issues | Single-language debugging |
| **CI/CD** | Requires Go build step | Python-only CI |
| **Self-Contained** | No (lives in `test/infrastructure/`) | Yes (lives in `holmesgpt-api/`) |

**Winner**: Pure Python approach is simpler and more maintainable.

---

## ✅ **Migration Plan**

### **Phase 1: Create Infrastructure Module** (4 hours)

1. **Create `infrastructure.py`** (2 hours)
   - [ ] Copy code above to `holmesgpt-api/tests/integration/infrastructure.py`
   - [ ] Adjust paths for project structure
   - [ ] Add type hints and docstrings

2. **Update `conftest.py`** (1 hour)
   - [ ] Import new infrastructure module
   - [ ] Replace shell script calls with `start_infrastructure()`
   - [ ] Add session-scoped fixture

3. **Test manually** (1 hour)
   ```bash
   cd holmesgpt-api/tests/integration
   python infrastructure.py start
   # Verify all services are running
   python infrastructure.py stop
   ```

---

### **Phase 2: Validate with Integration Tests** (2 hours)

1. **Run integration tests** (1 hour)
   ```bash
   cd holmesgpt-api
   pytest tests/integration/ -v
   ```

2. **Fix any issues** (1 hour)
   - Connection string format
   - Health check endpoints
   - Timeout values

---

### **Phase 3: Cleanup and Documentation** (2 hours)

1. **Deprecate old files** (30 min)
   - [ ] Move `docker-compose.workflow-catalog.yml` to `deprecated/`
   - [ ] Move shell scripts to `deprecated/`
   - [ ] Add `DEPRECATED.md` with migration notes

2. **Update documentation** (1 hour)
   - [ ] Update `holmesgpt-api/tests/integration/README.md`
   - [ ] Add DD-TEST-002 compliance note
   - [ ] Document manual start/stop workflow

3. **CI/CD validation** (30 min)
   - [ ] Run tests in GitHub Actions
   - [ ] Verify ~2-3 minute startup time
   - [ ] Confirm 100% pass rate maintained

---

## 📝 **Total Effort Estimate**

| Phase | Time | Complexity |
|-------|------|------------|
| **Phase 1**: Create infrastructure module | 4 hours | Medium |
| **Phase 2**: Validate with tests | 2 hours | Low |
| **Phase 3**: Cleanup and docs | 2 hours | Low |
| **Total** | **1 day** | Medium |

**Risk**: Low - Pattern is proven by Go services, just translating to Python

---

## ✅ **Success Criteria**

### **Immediate (Post-Migration)**

- [ ] Integration tests use `infrastructure.py` (no docker-compose)
- [ ] 100% test pass rate maintained
- [ ] Startup time ~2-3 minutes (same as current)
- [ ] No race condition failures in CI/CD
- [ ] DD-TEST-002 compliant (sequential startup)

### **Developer Experience**

- [ ] Manual start/stop workflow documented
  ```bash
  # Start infrastructure manually
  python tests/integration/infrastructure.py start

  # Run specific tests
  pytest tests/integration/test_workflow_catalog.py -v

  # Stop infrastructure
  python tests/integration/infrastructure.py stop
  ```

- [ ] Automatic start/stop in pytest (session-scoped fixture)
  ```bash
  # Infrastructure starts automatically if not running
  pytest tests/integration/ -v
  ```

---

## 🎯 **Benefits Over Other Options**

### **vs. Option A (Go CLI)**

✅ **No cross-language complexity** - Pure Python
✅ **No Go dependency** - Python developers stay in Python
✅ **Self-contained** - All code in `holmesgpt-api/`
✅ **Simpler debugging** - Single-language stack traces

### **vs. Option C (Hybrid)**

✅ **No docker-compose** - 100% DD-TEST-002 compliant
✅ **Consistent pattern** - Same approach for all services
✅ **No dual orchestration** - Single mechanism (Python subprocess)

### **vs. Option D (Exception)**

✅ **DD-TEST-002 compliant** - No exception needed
✅ **Consistent with Go services** - Same pattern, different language
✅ **Future-proof** - No need for post-v1.0 review

---

## 📊 **Updated Recommendation**

### **NEW Recommendation: Option E (Pure Python)**

| Aspect | Assessment |
|--------|------------|
| **DD-TEST-002 Compliance** | ✅ 100% (sequential startup pattern) |
| **Developer Experience** | ✅ Excellent (Python-native) |
| **Maintenance Burden** | ✅ Low (Python team only) |
| **Implementation Effort** | ⚠️ 1 day (medium) |
| **V1.0 Risk** | ⚠️ Medium (1 day implementation) |
| **Consistency** | ✅ High (DD-TEST-002 pattern, Python implementation) |
| **Self-Contained** | ✅ Yes (no external dependencies) |

---

## 🤝 **Decision Required**

**HAPI Team Decision**:

Should we:
1. ✅ **Implement Option E** (Pure Python DD-TEST-002 compliance) - 1 day
2. ⚠️ **Keep Option D** (Document as exception) - 1 hour, but permanent exception

**Recommendation**: **Option E** if we can afford 1 day before v1.0, otherwise **Option D** and migrate post-v1.0.

**Trade-off**: 1 day implementation vs. permanent exception documentation

---

## 📚 **References**

- **DD-TEST-002**: Integration test container orchestration pattern
- **DD-TEST-001**: Port allocation strategy (HAPI ports already allocated)
- **Go Implementation**: `test/infrastructure/datastorage_bootstrap.go` (reference pattern)
- **Python subprocess**: Standard library for running shell commands
- **pytest fixtures**: Session-scoped fixtures for infrastructure lifecycle

---

**Created**: December 23, 2025
**Author**: HAPI Team
**Status**: ✅ **READY FOR IMPLEMENTATION**

**Next Step**: HAPI team decision - Option E (1 day) vs. Option D (1 hour + post-v1.0 migration)

