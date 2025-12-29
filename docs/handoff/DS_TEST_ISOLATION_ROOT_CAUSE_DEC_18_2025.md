# DataStorage Test Isolation - Root Cause Analysis

**Date**: December 18, 2025, 10:45
**Issue**: Integration tests fail when run together via `make test-datastorage-all`
**Root Cause**: Serial tests using public schema + incomplete cleanup = data contamination

---

## 🎯 **Root Cause Identified**

### **The Architecture**

Integration tests use TWO different isolation strategies:

```
Parallel Tests (process-specific schemas):
├─ Process 1: test_process_1 schema
├─ Process 2: test_process_2 schema
├─ Process 3: test_process_3 schema
└─ Process 4: test_process_4 schema
    └─ ✅ Complete isolation via separate schemas

Serial Tests (public schema):
├─ ALL tests use: public schema
└─ ⚠️ Must clean up ALL test data, not just current testID
```

---

## 🔍 **Evidence from Code**

### **Serial Tests Explicitly Use Public Schema**

**`workflow_repository_integration_test.go` (line 70)**:
```go
BeforeEach(func() {
    usePublicSchema()  // Switches to public schema

    // Generate unique test ID for isolation
    testID = generateTestID()

    // Clean up test data matching CURRENT testID only
    _, err := db.ExecContext(ctx,
        "DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1",
        fmt.Sprintf("wf-repo-%s%%", testID))  // ⚠️ Only cleans current testID!
})
```

**Why Public Schema?** (from `http_api_test.go` line 55-56):
```go
// Serial tests MUST use public schema (HTTP API writes to public schema)
usePublicSchema()
```

---

## 🚨 **The Problem**

### **Scenario: Running `make test-datastorage-all`**

```
Run #1 (make test-integration-datastorage):
├─ Parallel tests: Use test_process_N schemas ✅
├─ Serial tests: Use public schema
│   ├─ testID = "1766072510123"
│   ├─ Creates workflows: wf-repo-1766072510123-*
│   └─ AfterEach: DELETE WHERE workflow_name LIKE 'wf-repo-1766072510123%'
└─ Container persists (Makefile didn't clean it up)

Run #2 (make test-datastorage-all includes integration again):
├─ Reuses same PostgreSQL container
├─ Serial tests: Use public schema
│   ├─ testID = "1766072550456" (NEW testID)
│   ├─ BeforeEach: DELETE WHERE workflow_name LIKE 'wf-repo-1766072550456%'
│   │   └─ ⚠️ Doesn't delete wf-repo-1766072510123-* from Run #1!
│   ├─ Creates workflows: wf-repo-1766072550456-*
│   └─ Test expects 3 workflows, but finds 6!
│       (3 from Run #1 + 3 from Run #2) ❌
```

---

## 📊 **Data Flow Diagram**

```
PostgreSQL Container (public schema):
┌─────────────────────────────────────────┐
│  After Run #1:                          │
│  ├─ wf-repo-1766072510123-workflow1     │
│  ├─ wf-repo-1766072510123-workflow2     │
│  └─ wf-repo-1766072510123-workflow3     │
└─────────────────────────────────────────┘
         ↓ (Container persists)
         ↓
┌─────────────────────────────────────────┐
│  Run #2 BeforeEach (testID=1766072550456): │
│  DELETE WHERE name LIKE 'wf-repo-1766072550456%' │
│  └─ Deletes: 0 rows (doesn't match Run #1 data!) │
└─────────────────────────────────────────┘
         ↓
         ↓
┌─────────────────────────────────────────┐
│  Run #2 Creates Data:                   │
│  ├─ wf-repo-1766072510123-workflow1 ← OLD │
│  ├─ wf-repo-1766072510123-workflow2 ← OLD │
│  ├─ wf-repo-1766072510123-workflow3 ← OLD │
│  ├─ wf-repo-1766072550456-workflow1 ← NEW │
│  ├─ wf-repo-1766072550456-workflow2 ← NEW │
│  └─ wf-repo-1766072550456-workflow3 ← NEW │
└─────────────────────────────────────────┘
         ↓
Test expects: 3 workflows
Test finds:   6 workflows ❌ FAIL!
```

---

## ✅ **Why Tests Pass Individually**

### **When Running Only `make test-integration-datastorage`**

```
Run #1:
├─ Fresh PostgreSQL container
├─ No old data exists
├─ BeforeEach cleanup finds 0 rows
├─ Creates 3 workflows
└─ Test finds exactly 3 workflows ✅
```

**Key**: No leftover data from previous runs!

---

## 🔧 **The Fix**

### **Option 1: Global Cleanup for Serial Tests** ✅ **RECOMMENDED**

**Update Serial test BeforeEach**:
```go
BeforeEach(func() {
    usePublicSchema()

    // GLOBAL cleanup: Remove ALL test workflows from public schema
    _, err := db.ExecContext(ctx,
        "DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE 'wf-repo-test-%'")
    Expect(err).ToNot(HaveOccurred())

    testID = generateTestID()
})
```

**Why**: Serial tests run one at a time, so global cleanup is safe

---

### **Option 2: Container Force-Remove Before Tests**

**Update `test/integration/datastorage/suite_test.go`**:
```go
func startPostgreSQL() {
    // ALWAYS remove old container to start fresh
    exec.Command("podman", "rm", "-f", postgresContainer).Run()

    // Then start fresh container
    cmd := exec.Command("podman", "run", "-d",
        "--name", postgresContainer,
        "-p", "15433:5432",
        ...)
    cmd.Run()
}
```

**Why**: Ensures fresh database on every test run

---

### **Option 3: Extend testID Cleanup to Match Prefix**

**Update Serial test cleanup**:
```go
BeforeEach(func() {
    usePublicSchema()

    // Clean up ALL test workflows (not just current testID)
    _, err := db.ExecContext(ctx,
        "DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE 'wf-repo-%'")
    Expect(err).ToNot(HaveOccurred())

    testID = generateTestID()
})
```

**Why**: Removes all test data, regardless of testID

---

## 📋 **Affected Test Files**

All Serial tests that use `usePublicSchema()`:

1. ✅ `workflow_repository_integration_test.go` - Repository CRUD tests
2. ✅ `workflow_label_scoring_integration_test.go` - Label scoring tests
3. ✅ `audit_events_repository_integration_test.go` - Audit events tests
4. ✅ `audit_events_query_api_test.go` - HTTP API tests
5. ✅ `http_api_test.go` - HTTP API tests
6. ✅ `audit_events_write_api_test.go` - Write API tests
7. ✅ `audit_events_batch_write_api_test.go` - Batch write tests
8. ✅ `audit_events_schema_test.go` - Schema tests
9. ✅ `aggregation_api_adr033_test.go` - ADR-033 tests

---

## 🎯 **Recommended Approach**

**Implement Option 1 (Global Cleanup) + Option 2 (Force Container Remove)**

### **Step 1: Update Serial Test BeforeEach**

For all Serial tests using `usePublicSchema()`, change from:
```go
_, err := db.ExecContext(ctx,
    "DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE $1",
    fmt.Sprintf("wf-repo-%s%%", testID))
```

To:
```go
// Serial tests: Global cleanup since they run sequentially
_, err := db.ExecContext(ctx,
    "DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE 'wf-repo-%'")
```

### **Step 2: Force Container Cleanup**

Update `test/integration/datastorage/suite_test.go` `startPostgreSQL()`:
```go
func startPostgreSQL() {
    // Force remove any existing container to ensure fresh state
    exec.Command("podman", "rm", "-f", postgresContainer).Run()
    time.Sleep(1 * time.Second)  // Allow cleanup to complete

    // Start fresh container
    cmd := exec.Command("podman", "run", "-d", ...)
    ...
}
```

---

## 📊 **Summary**

| Aspect | Current State | Issue | Fix |
|--------|---------------|-------|-----|
| **Parallel Tests** | Use test_process_N schemas | ✅ Isolated | No change needed |
| **Serial Tests** | Use public schema | ❌ testID-only cleanup | Global cleanup |
| **Container** | Persists between runs | ⚠️ Old data accumulates | Force remove |
| **Makefile** | Wrong container name | ✅ FIXED | Already done |

---

## ✅ **Expected Outcome**

After implementing these fixes:
- ✅ Tests pass individually
- ✅ Tests pass when run together (`make test-datastorage-all`)
- ✅ No data contamination across test runs
- ✅ Clean slate for every test execution

---

**Created**: December 18, 2025, 10:45
**Priority**: P1 (Blocks V1.0 confidence)
**Status**: Root cause identified, fix ready to implement
**Estimated Fix Time**: 30 minutes


