# HAPI E2E Go Bootstrap Migration - Feasibility Analysis

**Date**: February 2, 2026  
**Question**: How difficult to migrate bootstrap to Go (AA pattern) while keeping tests parallel?  
**Answer**: **EASY - 1-2 hours** ✅

---

## 🎯 TL;DR

**Difficulty**: ⭐⭐☆☆☆ (2/5 - Easy)  
**Effort**: 1-2 hours  
**Risk**: Very Low  
**Tests Stay Parallel**: ✅ Yes (pytest -n auto still works)  
**Recommendation**: **DO IT** - Clean pattern, better performance, matches AA

---

## 📊 COMPARISON: Current State vs Target State

| Aspect | Current (Python Bootstrap) | Target (Go Bootstrap) | Change Required |
|--------|---------------------------|----------------------|-----------------|
| **Bootstrap Location** | pytest session fixture | `SynchronizedBeforeSuite` Phase 1 | Move 80 lines to Go |
| **Workflow Count** | 5 workflows | 5 workflows | Same data |
| **Parallel Execution** | ✅ (with worker_id fix) | ✅ (native) | No change |
| **Startup Time** | 15s wait for gw1-gw10 | 0s wait (sequential setup) | **Faster** |
| **Code Duplication** | Python-only | Reuses AA infrastructure | **Less duplication** |
| **Maintenance** | Python fixture logic | Go + Python no-op fixture | **Simpler** |

---

## 🔍 WHAT NEEDS TO MIGRATE

### Current Python Bootstrap (holmesgpt-api/tests/fixtures/workflow_fixtures.py)

**5 workflows**:
1. `oomkill-increase-memory-v1` (OOMKilled, critical, production)
2. `memory-optimize-v1` (OOMKilled, high, staging)
3. `crashloop-config-fix-v1` (CrashLoopBackOff, high, production)
4. `node-drain-reboot-v1` (NodeNotReady, critical, production)
5. `image-pull-backoff-fix-credentials` (ImagePullBackOff, high, production)

### Target Go Pattern (test/integration/aianalysis/test_workflows.go)

**AA has 6 base workflows × 3 environments = 18 total**  
**HAPI E2E has 5 workflows × 1 environment = 5 total** (simpler!)

---

## ✅ WHAT'S ALREADY AVAILABLE (Reusable Infrastructure)

### 1. Go DataStorage Client ✅
- **Location**: `pkg/datastorage/ogen-client/`
- **Status**: Already generated, used by AA
- **Action**: **REUSE AS-IS**

### 2. Workflow Registration Function ✅
- **Location**: `test/integration/aianalysis/test_workflows.go:153-180`
- **Function**: `SeedTestWorkflowsInDataStorage(client, output)`
- **Status**: Production-tested with 18 workflows
- **Action**: **COPY & ADAPT** (change workflow list)

### 3. Authentication Setup ✅
- **Location**: `test/e2e/holmesgpt-api/holmesgpt_api_e2e_suite_test.go:223`
- **Status**: Already generates ServiceAccount token
- **Action**: **REUSE AS-IS** (already working)

---

## 📝 MIGRATION STEPS (1-2 hours)

### Step 1: Create Go Workflow Definitions (30 min)

**File**: `test/e2e/holmesgpt-api/test_workflows.go` (NEW)

```go
package holmesgptapi

import (
	"fmt"
	"io"
	"context"
	"crypto/sha256"
	"time"
	
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// TestWorkflow represents HAPI E2E test workflow
type TestWorkflow struct {
	WorkflowID       string
	Name             string
	Description      string
	SignalType       string
	Severity         string
	Component        string
	Environment      string
	Priority         string
	ContainerImage   string // HAPI E2E tests need container_image field
	RiskTolerance    string
}

// GetHAPIE2ETestWorkflows returns workflows for HAPI E2E tests
// These match holmesgpt-api/tests/fixtures/workflow_fixtures.py
func GetHAPIE2ETestWorkflows() []TestWorkflow {
	return []TestWorkflow{
		{
			WorkflowID:       "oomkill-increase-memory-v1",
			Name:             "OOMKill Remediation - Increase Memory Limits",
			Description:      "Increases memory limits for pods experiencing OOMKilled events",
			SignalType:       "OOMKilled",
			Severity:         "critical",
			Component:        "pod",
			Environment:      "production",
			Priority:         "P0",
			RiskTolerance:    "low",
			ContainerImage:   "ghcr.io/kubernaut/workflows/oomkill-increase-memory:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000001",
		},
		{
			WorkflowID:       "memory-optimize-v1",
			Name:             "OOMKill Remediation - Scale Down Replicas",
			Description:      "Reduces replica count for deployments experiencing OOMKilled",
			SignalType:       "OOMKilled",
			Severity:         "high",
			Component:        "deployment",
			Environment:      "staging",
			Priority:         "P1",
			RiskTolerance:    "medium",
			ContainerImage:   "ghcr.io/kubernaut/workflows/oomkill-scale-down:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000002",
		},
		{
			WorkflowID:       "crashloop-config-fix-v1",
			Name:             "CrashLoopBackOff - Fix Configuration",
			Description:      "Identifies and fixes configuration issues causing CrashLoopBackOff",
			SignalType:       "CrashLoopBackOff",
			Severity:         "high",
			Component:        "pod",
			Environment:      "production",
			Priority:         "P1",
			RiskTolerance:    "low",
			ContainerImage:   "ghcr.io/kubernaut/workflows/crashloop-fix-config:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000003",
		},
		{
			WorkflowID:       "node-drain-reboot-v1",
			Name:             "NodeNotReady - Drain and Reboot",
			Description:      "Safely drains and reboots nodes in NotReady state",
			SignalType:       "NodeNotReady",
			Severity:         "critical",
			Component:        "node",
			Environment:      "production",
			Priority:         "P0",
			RiskTolerance:    "low",
			ContainerImage:   "ghcr.io/kubernaut/workflows/node-drain-reboot:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000004",
		},
		{
			WorkflowID:       "image-pull-backoff-fix-credentials",
			Name:             "ImagePullBackOff - Fix Registry Credentials",
			Description:      "Fixes ImagePullBackOff errors by updating registry credentials",
			SignalType:       "ImagePullBackOff",
			Severity:         "high",
			Component:        "pod",
			Environment:      "production",
			Priority:         "P1",
			RiskTolerance:    "medium",
			ContainerImage:   "ghcr.io/kubernaut/workflows/imagepull-fix-creds:v1.0.0@sha256:0000000000000000000000000000000000000000000000000000000000000005",
		},
	}
}

// SeedTestWorkflowsInDataStorage - COPIED from test/integration/aianalysis/test_workflows.go
// Minor adaptation: add container_image field support
func SeedTestWorkflowsInDataStorage(client *ogenclient.Client, output io.Writer) (map[string]string, error) {
	// ... 80 lines copied from AA with minor field additions ...
	// See test/integration/aianalysis/test_workflows.go:153-180 for reference
}
```

**Effort**: 30 minutes (mostly copy-paste from Python fixtures + AA pattern)

---

### Step 2: Add Bootstrap Call to BeforeSuite (15 min)

**File**: `test/e2e/holmesgpt-api/holmesgpt_api_e2e_suite_test.go`

**Location**: After line 186 (after cluster setup, before pytest)

```go
// Seed test workflows BEFORE pytest starts (DD-TEST-011 v2.0 pattern)
// Pattern: Same as AIAnalysis - bootstrap in Go, pytest finds workflows already exist
By("Seeding test workflows into DataStorage")
workflowUUIDs, err := SeedTestWorkflowsInDataStorage(seedClient, logger)
Expect(err).ToNot(HaveOccurred(), "Test workflows must be seeded successfully")
logger.Info("✅ Test workflows seeded: " + fmt.Sprintf("%d workflows", len(workflowUUIDs)))
```

**Effort**: 15 minutes (3 lines + import)

---

### Step 3: Update Python Fixture to No-Op (10 min)

**File**: `holmesgpt-api/tests/e2e/conftest.py`

**Replace lines 308-345** with:

```python
@pytest.fixture(scope="session")
def test_workflows_bootstrapped(data_storage_stack):
    """
    DD-TEST-011 v2.0: Workflows already seeded by Go suite setup.
    
    Pattern matches AA integration tests:
    - Go: Seeds workflows in SynchronizedBeforeSuite Phase 1
    - Python: Fixture is a no-op, workflows already exist
    
    This prevents pytest-xdist parallel workers from bootstrapping concurrently.
    """
    data_storage_url = data_storage_stack
    print(f"\n✅ DD-TEST-011 v2.0: Workflows already seeded by Go suite setup")
    print(f"   Data Storage URL: {data_storage_url}")
    
    # Return empty results (workflows already seeded by Go)
    return {
        "created": [],
        "existing": [],
        "failed": [],
        "total": 0,
        "seeded_by": "go_before_suite"
    }
```

**Effort**: 10 minutes (replace fixture body)

---

### Step 4: Remove worker_id Fix (5 min) ⚠️ OPTIONAL

Since Go bootstrap runs ONCE before pytest, the `worker_id` fixture is no longer needed.

**File**: `holmesgpt-api/tests/e2e/conftest.py`

**Action**: Remove lines 67-76 (`worker_id` fixture) - no longer needed

**Effort**: 5 minutes (cleanup only, not required)

---

### Step 5: Test & Validate (30 min)

```bash
# Clean start
kind delete cluster --name holmesgpt-api-e2e

# Run E2E (should show Go bootstrap, then pytest with -n auto)
make test-e2e-holmesgpt-api
```

**Expected output**:
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🌱 Seeding Test Workflows in DataStorage
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📋 Registering 5 test workflows...
  ✅ oomkill-increase-memory-v1 (production) → 123e4567-e89b-12d3-a456-426614174000
  ✅ memory-optimize-v1 (staging) → 123e4567-e89b-12d3-a456-426614174001
  ✅ crashloop-config-fix-v1 (production) → 123e4567-e89b-12d3-a456-426614174002
  ✅ node-drain-reboot-v1 (production) → 123e4567-e89b-12d3-a456-426614174003
  ✅ image-pull-backoff-fix-credentials (production) → 123e4567-e89b-12d3-a456-426614174004
✅ All test workflows registered (5 UUIDs captured)

Running pytest in containerized Python environment...
✅ DD-TEST-011 v2.0: Workflows already seeded by Go suite setup

[pytest runs with -n auto, 11 workers, NO bootstrap delays]

Ran 18 of 18 Specs
SUCCESS! -- 18 Passed | 0 Failed
```

**Effort**: 30 minutes (run + validate)

---

## ⚖️ TRADE-OFFS

| Aspect | Current Fix | Go Migration |
|--------|------------|--------------|
| **Implementation Time** | ✅ Done (5 min) | 1-2 hours |
| **Code Complexity** | ⚠️ Python worker_id logic | ✅ Simpler (standard pattern) |
| **Startup Performance** | ⚠️ 15s wait for workers | ✅ 0s wait (sequential setup) |
| **Consistency** | ⚠️ HAPI-specific pattern | ✅ Matches AA pattern |
| **Maintenance** | ⚠️ Two bootstrap implementations | ✅ Reuses AA infrastructure |
| **Parallel Tests** | ✅ Works | ✅ Works (better) |
| **Risk** | ✅ Already validated | ⚠️ Needs testing (low risk) |

---

## 🎯 RECOMMENDATION

### **MIGRATE TO GO BOOTSTRAP NOW** ✅

**Why**:
1. **Easy** - 1-2 hours, mostly copy-paste
2. **Better** - 15s faster startup, cleaner code
3. **Consistent** - Matches AA pattern (one way to do things)
4. **Proven** - AA pattern validated with 18 workflows
5. **Simpler** - No worker_id detection logic needed

**When NOT to migrate**:
- If you need to ship TODAY (current fix works)
- If Python-only team maintains E2E tests (but Go infrastructure already exists)

---

## 📋 CHECKLIST

### Before Starting
- [ ] Current tests passing with worker_id fix
- [ ] Confirmed 5 workflows in Python fixtures
- [ ] AA pattern works in integration tests

### During Migration
- [ ] Create `test/e2e/holmesgpt-api/test_workflows.go`
- [ ] Copy 5 workflow definitions from Python
- [ ] Copy `SeedTestWorkflowsInDataStorage` from AA
- [ ] Add `container_image` field support (AA doesn't have this)
- [ ] Update `SynchronizedBeforeSuite` to call seeding
- [ ] Update Python fixture to no-op
- [ ] Remove `worker_id` fixture (optional cleanup)

### After Migration
- [ ] Clean cluster: `kind delete cluster --name holmesgpt-api-e2e`
- [ ] Run E2E: `make test-e2e-holmesgpt-api`
- [ ] Verify Go bootstrap logs (5 workflows seeded)
- [ ] Verify pytest logs (no worker bootstrap delays)
- [ ] Verify 18/18 tests pass
- [ ] Check must-gather (no TokenReview rate limit errors)
- [ ] Update testing documentation

---

## 🔗 REFERENCES

**Current Pattern**:
- `holmesgpt-api/tests/e2e/conftest.py:308` - Python bootstrap
- `holmesgpt-api/tests/fixtures/workflow_fixtures.py:124` - Workflow definitions

**Target Pattern (AA)**:
- `test/integration/aianalysis/suite_test.go:402` - Go bootstrap call
- `test/integration/aianalysis/test_workflows.go:153` - Seeding function
- `holmesgpt-api/tests/integration/conftest.py:287` - Python no-op fixture

**Documentation**:
- DD-TEST-011 v2.0 - File-Based Configuration
- DD-AUTH-014 - ServiceAccount Authentication
- BR-TEST-008 - Performance Optimization

---

## ✅ CONCLUSION

**Migration Difficulty**: ⭐⭐☆☆☆ (2/5 - Easy)

**Total Effort**: **1-2 hours**
- 30 min: Create Go workflow definitions
- 15 min: Add bootstrap call to BeforeSuite
- 10 min: Update Python fixture to no-op
- 5 min: Optional cleanup (remove worker_id)
- 30 min: Test & validate

**Result**: Cleaner code, faster tests, consistent pattern across all services.

**Recommendation**: **DO IT** - The migration is straightforward and brings significant benefits.
