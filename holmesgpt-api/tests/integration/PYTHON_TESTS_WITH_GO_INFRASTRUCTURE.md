# Python Integration Tests with Go Infrastructure

**Date**: December 29, 2025
**Status**: ✅ **CORRECT APPROACH DOCUMENTED**

---

## 🎯 **Correct Architecture**

### HAPI is a Python Service → Tests Should Be in Python

**Key Principle**: Use the **shared Go infrastructure library** to bootstrap services, but keep **test logic in Python**.

```
┌─────────────────────────────────────────────────────────────┐
│                  HAPI Integration Tests                       │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────────────┐     ┌─────────────────────────┐  │
│  │  Go Infrastructure   │────▶│  Python Test Logic      │  │
│  │  (Bootstrap)         │     │  (Actual Tests)         │  │
│  └──────────────────────┘     └─────────────────────────┘  │
│           │                              │                   │
│           ▼                              ▼                   │
│  ┌──────────────────────┐     ┌─────────────────────────┐  │
│  │  HAPI Service        │     │  39 Python Tests        │  │
│  │  Data Storage        │     │  test_audit_flow.py     │  │
│  │  PostgreSQL          │     │  test_workflow_*.py     │  │
│  │  Redis               │     │  test_metrics.py        │  │
│  └──────────────────────┘     └─────────────────────────┘  │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

---

## ✅ **What We Have (Correct)**

### Go Infrastructure (test/infrastructure/holmesgpt_integration.go)

**Purpose**: Start all required services programmatically
**Status**: ✅ **COMPLETE and CORRECT**

```go
func StartHolmesGPTAPIIntegrationInfrastructure(writer io.Writer) error {
    // Starts in sequence:
    // 1. PostgreSQL (port 15439)
    // 2. Redis (port 16387)
    // 3. Data Storage (port 18098)
    // 4. HAPI service (port 18120)
    return nil
}
```

**Benefits**:
- ✅ Reuses 720 lines of shared utilities
- ✅ Proper health checks
- ✅ Composite image tags (DD-INTEGRATION-001 v2.0)
- ✅ Sequential startup (DD-TEST-002)
- ✅ Consistent with other 7 services

---

## 🔧 **How Python Tests Should Work**

### Option 1: Python Tests Discover Go-Started Services (RECOMMENDED)

**Pattern**: Python tests read service ports from environment/config

```python
# holmesgpt-api/tests/integration/conftest.py (UPDATED)

import os
import pytest

# Read ports from environment (set by Go infrastructure or CI)
HAPI_PORT = int(os.environ.get("HAPI_INTEGRATION_PORT", "18120"))
DATA_STORAGE_PORT = int(os.environ.get("DS_INTEGRATION_PORT", "18098"))
POSTGRES_PORT = int(os.environ.get("PG_INTEGRATION_PORT", "15439"))
REDIS_PORT = int(os.environ.get("REDIS_INTEGRATION_PORT", "16387"))

@pytest.fixture(scope="session")
def hapi_url():
    """HAPI service URL (started by Go infrastructure)"""
    return f"http://localhost:{HAPI_PORT}"

@pytest.fixture(scope="session")
def data_storage_url():
    """Data Storage URL (started by Go infrastructure)"""
    return f"http://localhost:{DATA_STORAGE_PORT}"

# NO MORE: subprocess.run(["docker-compose", "up"])
# NO MORE: Starting services in conftest.py
# Services are already running via Go infrastructure!
```

**Python Test Example**:
```python
# holmesgpt-api/tests/integration/test_audit_flow_integration.py

def test_incident_analysis_emits_audit_events(hapi_url, data_storage_url):
    """Test uses services started by Go infrastructure"""
    # Make request to HAPI
    response = requests.post(f"{hapi_url}/api/v1/incident/analyze", json=data)

    # Verify audit events in Data Storage
    audit_events = requests.get(f"{data_storage_url}/api/v1/audit/events", params={
        "correlation_id": remediation_id
    })

    assert len(audit_events.json()["data"]) >= 2
```

---

### Option 2: Wrapper Script (ALTERNATIVE)

**Pattern**: Shell script coordinates Go bootstrap + Python tests

```bash
#!/bin/bash
# holmesgpt-api/tests/integration/run_python_tests.sh

set -e

echo "🚀 Starting services via Go infrastructure..."
cd ../../..  # Project root
ginkgo run --label-filter="hapi-infrastructure" ./test/infrastructure/

echo "🧪 Running Python integration tests..."
cd holmesgpt-api
python -m pytest tests/integration/ -v

echo "🧹 Cleaning up services..."
# Teardown via Go or docker-compose down
```

---

## ❌ **What Was Wrong (Fixed)**

### Deprecated Python Infrastructure (conftest.py)

**Problem**: Python `conftest.py` was:
- ❌ Using `subprocess.run()` to call docker-compose
- ❌ Generating wrong image names
- ❌ Duplicating 720 lines of shared Go utilities
- ❌ Inconsistent with other services

**Solution**: Remove deprecated infrastructure, use Go-started services instead.

---

## 📝 **Migration from Deprecated conftest.py**

### Step 1: Update conftest.py

**Before** (DEPRECATED):
```python
@pytest.fixture(scope="session")
def integration_infrastructure():
    """Start services via docker-compose"""
    subprocess.run(["docker-compose", "-f", "podman-compose.test.yml", "up", "-d"])
    # Wait for services...
    yield {"data_storage_url": "http://localhost:18090"}
    subprocess.run(["docker-compose", "down"])
```

**After** (CORRECT):
```python
@pytest.fixture(scope="session")
def integration_infrastructure():
    """Services already started by Go infrastructure"""
    # NO subprocess calls! Just return URLs.
    return {
        "hapi_url": f"http://localhost:{HAPI_PORT}",
        "data_storage_url": f"http://localhost:{DATA_STORAGE_PORT}",
    }
```

### Step 2: Update Python Tests

**No changes needed!** Tests already use fixtures, so they automatically work with Go-started services.

---

## 🚀 **Running Python Tests**

### Makefile Target (RECOMMENDED) ✅

```bash
# One-command execution: infrastructure + tests + cleanup
cd /path/to/kubernaut
make test-integration-holmesgpt
```

**What it does:**
1. ✅ Starts Go infrastructure (PostgreSQL, Redis, Data Storage, HAPI)
2. ✅ Waits for services to be ready
3. ✅ Runs Python integration tests
4. ✅ Cleans up infrastructure automatically

**Duration**: ~2 minutes

### Manual Approach (Two Terminals)

```bash
# Terminal 1: Start Go infrastructure
cd /path/to/kubernaut/test/integration/holmesgptapi
./setup-infrastructure.sh
# Infrastructure will remain running until you press Ctrl+C

# Terminal 2: Run Python tests
cd /path/to/kubernaut/holmesgpt-api
export HAPI_INTEGRATION_PORT=18120
export DS_INTEGRATION_PORT=18098
export HAPI_URL="http://localhost:18120"
export DATA_STORAGE_URL="http://localhost:18098"
python -m pytest tests/integration/ -v

# Terminal 1: Stop infrastructure with Ctrl+C
```

---

## ✅ **Benefits of This Approach**

| Aspect | Benefit |
|--------|---------|
| **Infrastructure** | ✅ Reuses shared Go utilities (720 lines) |
| **Consistency** | ✅ Same bootstrap as other 7 services |
| **Test Logic** | ✅ Stays in Python (native for HAPI) |
| **Type Safety** | ✅ Go infrastructure uses typed clients |
| **Maintenance** | ✅ Single infrastructure codebase |
| **Migration Effort** | ✅ Minimal (just update conftest.py) |

---

## 📊 **Status: 39 Python Tests**

All 39 Python integration tests remain in Python:

| Test File | Tests | Status | Action Needed |
|-----------|-------|--------|---------------|
| `test_hapi_audit_flow_integration.py` | 6 | ✅ KEEP | Update conftest.py |
| `test_workflow_catalog_data_storage.py` | 10 | ✅ KEEP | Update conftest.py |
| `test_data_storage_label_integration.py` | 15 | ✅ KEEP | Update conftest.py |
| `test_hapi_metrics_integration.py` | 8 | ✅ KEEP | Update conftest.py |

**Total**: 39 Python tests, 0 Go tests

**Migration Effort**: ~30 minutes (update conftest.py only)

---

## 🎯 **Next Steps**

1. ✅ **Update conftest.py** to remove subprocess calls
2. ✅ **Set environment variables** for service ports
3. ✅ **Run Python tests** against Go-started services
4. ✅ **Verify** all 39 tests pass
5. ✅ **Document** in Makefile or CI/CD

---

## 📚 **References**

- **Go Infrastructure**: `test/infrastructure/holmesgpt_integration.go`
- **Python Tests**: `holmesgpt-api/tests/integration/*.py`
- **Port Allocation**: DD-TEST-001 v1.8
- **Integration Pattern**: DD-INTEGRATION-001 v2.0

---

**Document Status**: ✅ AUTHORITATIVE
**Authority**: User clarification (Dec 29, 2025)
**Key Insight**: "HAPI is the only Python service. The only thing I expect is the shared Go library to be used to bootstrap the integration and e2e tests for HAPI, but the test logic should be in Python."

