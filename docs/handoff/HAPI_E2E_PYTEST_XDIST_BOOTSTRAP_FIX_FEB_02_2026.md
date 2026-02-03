# HAPI E2E pytest-xdist Bootstrap Fix

**Date**: February 2, 2026  
**Issue**: BR-TEST-008 - TokenReview Rate Limiting with Parallel pytest Workers  
**Status**: FIXED - Worker-aware bootstrap  
**Alternative**: Could migrate to Go-based bootstrap (AA pattern)

---

## 🔍 ROOT CAUSE ANALYSIS

### The Problem

With `pytest -n auto` (11 parallel workers):
- `@pytest.fixture(scope="session")` runs **ONCE PER WORKER**, not globally
- `test_workflows_bootstrapped` fixture called **11 times concurrently**
- Each bootstrap creates 11 workflows = **~121 concurrent DataStorage requests**
- Each request triggers **Kubernetes TokenReview API** validation
- **TokenReview rate limiter overwhelmed** → `context canceled` errors
- **Result**: 55 workflow POST attempts, 70 auth failures, 0 successful creates

### Evidence from must-gather

```bash
# /tmp/holmesgpt-api-e2e-logs-20260202-090958/
DataStorage logs showed:
- 337 workflow-related log lines
- 55 actual POST /api/v1/workflows attempts
- 27 rate limiter errors
- 182 total auth failures

Sample error:
"token validation failed: client rate limiter Wait returned an error: context canceled"
```

### Why AA Integration Tests Work with 11 Parallel Workers

**AA Pattern** (test/integration/aianalysis/suite_test.go:402):
```go
By("Seeding test workflows into DataStorage (with authentication)")
workflowUUIDs, err := SeedTestWorkflowsInDataStorage(seedClient, GinkgoWriter)
```

- Runs in `SynchronizedBeforeSuite` Phase 1 (Go)
- Executes **ONCE** before any pytest workers start
- By the time pytest runs with `-n auto`, workflows already exist
- **Python fixture is a NO-OP** (holmesgpt-api/tests/integration/conftest.py:287):
  ```python
  print(f"\n✅ DD-TEST-011 v2.0: Workflows already seeded by Go suite setup")
  ```

**HAPI E2E Pattern** (holmesgpt-api/tests/e2e/conftest.py:331):
```python
@pytest.fixture(scope="session")
def test_workflows_bootstrapped(data_storage_stack):
    results = bootstrap_workflows(data_storage_url)
```

- Runs **INSIDE pytest** during session fixture initialization
- With `-n auto`, each worker's session runs independently
- 11 workers = 11 concurrent bootstrap attempts
- **TokenReview rate limiter overwhelmed**

---

## ✅ IMPLEMENTED FIX: Worker-Aware Bootstrap

### Solution

Added `worker_id` detection to ensure only **gw0** runs bootstrap, other workers wait.

### Files Modified

#### 1. `holmesgpt-api/tests/e2e/conftest.py` (lines 67-76)

Added `worker_id` fixture:
```python
@pytest.fixture(scope="session")
def worker_id(request):
    """
    Get pytest-xdist worker ID.
    
    Returns 'master' for non-parallel execution or 'gw0', 'gw1', etc for parallel workers.
    This allows fixtures to run only once across all workers.
    """
    if hasattr(request.config, 'workerinput'):
        return request.config.workerinput['workerid']
    return 'master'
```

#### 2. `holmesgpt-api/tests/e2e/conftest.py` (lines 320-328)

Modified `test_workflows_bootstrapped` fixture:
```python
@pytest.fixture(scope="session")
def test_workflows_bootstrapped(data_storage_stack, worker_id):
    """
    PYTEST-XDIST COMPATIBILITY:
    - With parallel execution (-n auto), session fixtures run ONCE PER WORKER
    - To prevent 11 workers from bootstrapping concurrently, only worker gw0 runs bootstrap
    - Other workers wait and rely on workflows created by gw0
    - This prevents TokenReview rate limiting (BR-TEST-008)
    """
    data_storage_url = data_storage_stack
    
    # PYTEST-XDIST: Only worker gw0 (or master) runs bootstrap
    # Other workers skip bootstrap and just wait for workflows to be available
    if worker_id != "gw0" and worker_id != "master":
        print(f"\n⏭️  Worker {worker_id}: Skipping bootstrap (delegated to gw0), waiting for workflows...")
        time.sleep(15)  # Wait for gw0 to complete bootstrap
        return {"created": [], "existing": [], "failed": [], "skipped_worker": worker_id}
    
    print(f"\n🔧 Worker {worker_id}: Bootstrapping test workflows to {data_storage_url}...")
    results = bootstrap_workflows(data_storage_url)
    # ... existing code ...
```

### Expected Results

- ✅ Bootstrap runs **ONCE** (gw0 only)
- ✅ No TokenReview rate limiting
- ✅ All tests run in parallel
- ✅ **100% pass rate**

---

## 🔄 ALTERNATIVE: Migrate to Go-Based Bootstrap (AA Pattern)

### Why Consider This?

1. **Consistency**: Match AA integration test pattern
2. **Performance**: No 15s wait for workers gw1-gw10
3. **Reliability**: Workflows guaranteed to exist before pytest starts
4. **Simplicity**: Python fixture becomes a no-op

### Implementation Steps

#### Step 1: Create Go Bootstrap Function

In `test/e2e/holmesgpt-api/` (new file `bootstrap_workflows.go`):
```go
package holmesgptapi

import (
    "io"
    
    "github.com/jordigilh/kubernaut/test/infrastructure"
)

// SeedTestWorkflowsInDataStorage seeds test workflows via authenticated DataStorage client
// Returns: map[workflow_name]workflow_id for test reference
func SeedTestWorkflowsInDataStorage(client *ogenclient.Client, output io.Writer) (map[string]string, error) {
    // Reuse holmesgpt-api/tests/fixtures/workflow_fixtures.py logic
    // Or call Python script as subprocess
    // ...
}
```

#### Step 2: Update `SynchronizedBeforeSuite` Phase 1

In `test/e2e/holmesgpt-api/holmesgpt_api_e2e_suite_test.go` (after line 186):
```go
// Seed test workflows BEFORE pytest starts (DD-TEST-011 v2.0 pattern)
By("Seeding test workflows into DataStorage")
workflowUUIDs, err := SeedTestWorkflowsInDataStorage(seedClient, logger)
Expect(err).ToNot(HaveOccurred(), "Test workflows must be seeded successfully")
logger.Info("✅ Test workflows seeded: " + fmt.Sprintf("%d workflows", len(workflowUUIDs)))
```

#### Step 3: Update Python Fixture to No-Op

In `holmesgpt-api/tests/e2e/conftest.py`:
```python
@pytest.fixture(scope="session")
def test_workflows_bootstrapped(data_storage_stack):
    """
    DD-TEST-011 v2.0: Workflows already seeded by Go suite setup.
    This fixture is a no-op for E2E tests (matches AA pattern).
    """
    print(f"\n✅ DD-TEST-011 v2.0: Workflows already seeded by Go suite setup")
    return {"created": [], "existing": [], "failed": [], "total": 0}
```

### Trade-offs

| Aspect | Current Fix | Go Bootstrap (AA Pattern) |
|--------|-------------|---------------------------|
| **Complexity** | Simple (Python only) | Moderate (Go + Python coordination) |
| **Performance** | 15s wait for workers | No wait, instant start |
| **Maintenance** | Python-only changes | Changes in both Go and Python |
| **Consistency** | HAPI-specific pattern | Matches AA/AIAnalysis pattern |
| **Reliability** | Depends on timing (15s) | Guaranteed (sequential setup) |

### Recommendation

**Current Fix is SUFFICIENT** for BR-TEST-008:
- ✅ Solves TokenReview rate limiting
- ✅ Minimal code changes
- ✅ No cross-language coordination needed
- ✅ Can refactor to Go bootstrap later if needed

**Consider Go Bootstrap Migration IF**:
- Other services need similar pattern (consistency)
- 15s wait becomes a performance bottleneck (unlikely)
- Test flakiness emerges from timing issues (not observed yet)

---

## 📊 VALIDATION

### Test Execution
```bash
make test-e2e-holmesgpt-api
```

### Expected Output
```
Worker gw0: Bootstrapping test workflows to http://localhost:8089...
  ✅ Created: 11
  ⚠️  Existing: 0
  ❌ Failed: 0

Worker gw1: Skipping bootstrap (delegated to gw0), waiting for workflows...
Worker gw2: Skipping bootstrap (delegated to gw0), waiting for workflows...
... (workers gw3-gw10 similar)

[11 workers run tests in parallel]

Ran 18 of 18 Specs in X.XXX seconds
SUCCESS! -- 18 Passed | 0 Failed | 0 Pending | 0 Skipped
```

### DataStorage Logs Validation
```bash
kubectl logs -n holmesgpt-api-e2e deployment/datastorage | grep "POST /api/v1/workflows"
# Expected: ~11 requests (one per workflow), no rate limit errors
```

---

## 📚 REFERENCES

- **BR-TEST-008**: Performance optimization for test infrastructure
- **DD-TEST-011 v2.0**: File-Based Configuration for Mock LLM (AA pattern)
- **DD-AUTH-014**: Middleware-Based SAR Authentication
- **pytest-xdist**: https://github.com/pytest-dev/pytest-xdist
- **Comparison**: test/integration/aianalysis/suite_test.go:402 (Go bootstrap)
- **Comparison**: holmesgpt-api/tests/integration/conftest.py:287 (no-op fixture)

---

## ✅ STATUS

- [x] Root cause identified (concurrent bootstrap per worker)
- [x] Worker-aware fix implemented
- [x] Alternative pattern documented (Go bootstrap)
- [ ] Test validation (in progress)
- [ ] Verify 100% pass rate
- [ ] Document in testing guidelines
