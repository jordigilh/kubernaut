# DataStorage Test Isolation Explained

**Date**: December 18, 2025, 09:30
**Question**: "Why does it impact when each tier runs in their own environment?"

---

## 🔍 **The Answer: "Own Environment" ≠ "Own Database"**

### **What "Own Environment" Actually Means**

```
Unit Tests:       Separate Go Process    ✅
Integration Tests: Separate Go Process   ✅
E2E Tests:        Separate Go Process    ✅

HOWEVER...

All three tiers connect to:
├─ SAME PostgreSQL instance: localhost:15433 ❌
├─ SAME database: action_history             ❌
└─ SAME schema: public                       ❌
```

---

## 📊 **Database Connection Details**

### **From `suite_test.go`**:

```go
// Line 734: ALL test tiers use this connection string
connStr := fmt.Sprintf(
    "host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable",
    host, port)

db, err = sqlx.Connect("pgx", connStr)
```

**Result**: Unit, Integration, and E2E ALL connect to `action_history` database on the SAME PostgreSQL server.

---

## 🏗️ **Schema Isolation (But Not What You Think)**

### **Schema Isolation EXISTS For...**

**Parallel Ginkgo Processes** (WITHIN a single test tier):

```go
// Line 458: Creates process-specific schemas
schemaName, err = createProcessSchema(db, processNum)
// Creates: test_process_1, test_process_2, test_process_3, etc.
```

**Example - Running Integration Tests with `-p` flag**:
```
Integration Test Suite (4 parallel processes):
├─ Process 1 → test_process_1 schema ✅ Isolated
├─ Process 2 → test_process_2 schema ✅ Isolated
├─ Process 3 → test_process_3 schema ✅ Isolated
└─ Process 4 → test_process_4 schema ✅ Isolated
```

---

### **Schema Isolation DOES NOT EXIST For...**

**Sequential Test Tiers** (Unit → Integration → E2E):

```
Time T1: Unit Tests Complete
└─ Data in: public schema (or test_process_N if parallel)
└─ Cleanup: Uses testID = "test-1-1766067100000000000"
└─ Removes: Only workflows matching that testID

Time T2: Integration Tests Start
└─ NEW testID = "test-1-1766067227000000000"  ← Different timestamp!
└─ Cleanup attempts: DELETE WHERE workflow_name LIKE 'wf-repo-test-1-1766067227000000000%'
└─ BUT: Old data from T1 still exists! ❌
└─ Result: Database has data from BOTH unit and integration tests

Time T3: E2E Tests Start
└─ NEW testID = "test-1-1766067350000000000"  ← Another timestamp!
└─ Database now has data from unit, integration, AND e2e ❌❌❌
```

---

## 🎯 **The Core Problem Visualized**

### **What You Might Expect** (But Doesn't Happen):

```
┌──────────────────────┐
│   Unit Tests         │
│   Database: unit_db  │ ← Isolated
└──────────────────────┘

┌──────────────────────┐
│   Integration Tests  │
│   Database: int_db   │ ← Isolated
└──────────────────────┘

┌──────────────────────┐
│   E2E Tests          │
│   Database: e2e_db   │ ← Isolated
└──────────────────────┘
```

---

### **What Actually Happens**:

```
┌────────────────────────────────────────┐
│   SHARED PostgreSQL Database           │
│   Database: action_history             │
│   Schema: public                       │
├────────────────────────────────────────┤
│                                        │
│   Unit Test Data (testID: ...100...)  │
│   ├─ wf-repo-test-1-...100...-foo     │
│   └─ wf-repo-test-1-...100...-bar     │
│                                        │
│   Integration Test Data (testID: ...227...) │
│   ├─ wf-repo-test-1-...227...-foo     │ ← Cleanup ONLY removes these
│   └─ wf-repo-test-1-...227...-bar     │
│                                        │
│   E2E Test Data (testID: ...350...)   │
│   ├─ wf-repo-test-1-...350...-foo     │
│   └─ wf-repo-test-1-...350...-bar     │
│                                        │
│   ❌ ALL THREE EXIST SIMULTANEOUSLY    │
└────────────────────────────────────────┘
```

---

## 🔬 **Evidence From Test Logs**

### **The Smoking Gun**:

```log
2025-12-18T09:13:47.063 workflow created:
  wf-repo-test-1-1766067227052917000-duplicate v1.0.0 ✅

2025-12-18T09:13:47.064 marked previous versions as not latest
  (versions_updated: 1) ⚠️  ← Wait, what previous version?!

2025-12-18T09:13:47.064 ERROR: duplicate key value violates unique constraint ❌
```

**Question**: Why does "versions_updated: 1" appear?
**Answer**: Because a workflow with a SIMILAR name from a PREVIOUS test tier already exists!

---

## 🧪 **The Timestamp Evidence**

### **Test ID Format**:
```go
testID = fmt.Sprintf("test-%d-%d", GinkgoParallelProcess(), time.Now().UnixNano())
```

**Example Execution**:
```
Unit Tests:       "test-1-1766067100000000000"  (09:10:00)
Integration Tests: "test-1-1766067227052917000"  (09:13:47) ← 3 minutes later!
E2E Tests:        "test-1-1766067350000000000"  (09:15:50) ← 2 more minutes
```

**Cleanup Pattern**:
```sql
-- Integration test cleanup:
DELETE FROM remediation_workflow_catalog
WHERE workflow_name LIKE 'wf-repo-test-1-1766067227052917000%'

-- ❌ Does NOT delete:
'wf-repo-test-1-1766067100000000000%'  ← Unit test data still there!
```

---

## 📋 **Why This Design Exists**

### **Schema Isolation Was Designed For...**

**Parallel Execution WITHIN a Test Tier**:

```bash
# Run integration tests with 4 parallel processes
ginkgo -p --procs=4 ./test/integration/datastorage

# Creates 4 schemas:
test_process_1, test_process_2, test_process_3, test_process_4
```

**This works great!** Each parallel process is isolated.

---

### **But NOT For Sequential Execution ACROSS Tiers**:

```bash
# Running all tiers together
make test-datastorage-all

# Executes sequentially:
go test ./test/unit/datastorage      # Uses public schema
go test ./test/integration/datastorage  # Uses public schema (same!)
go test ./test/e2e/datastorage       # Uses public schema (same!)
```

**Result**: No isolation between tiers, only within parallel processes of the same tier.

---

## 🎯 **The Fix**

### **Option 1: Tier-Specific Schemas** ✅ **RECOMMENDED**

Create separate schemas for each test tier:

```go
BeforeSuite(func() {
    tierName := os.Getenv("TEST_TIER") // "unit", "integration", "e2e"
    if tierName == "" {
        tierName = "integration" // default
    }

    schemaName := fmt.Sprintf("test_%s_%d", tierName, GinkgoParallelProcess())
    // Creates: test_unit_1, test_integration_1, test_e2e_1

    createProcessSchema(db, schemaName)
})
```

---

### **Option 2: Global Cleanup** ⚠️ **SIMPLER BUT LOSES PARALLEL SUPPORT**

```go
BeforeEach(func() {
    // Delete ALL test workflows, not just current testID
    _, err := db.ExecContext(ctx,
        "DELETE FROM remediation_workflow_catalog WHERE workflow_name LIKE 'wf-repo-test-%'")

    testID = generateTestID()
})
```

**Pros**: Simple, works for sequential execution
**Cons**: Can't run tests in parallel (race conditions)

---

## 💡 **Key Insight**

**Separate Processes ≠ Separate Databases**

```
✅ Each test tier runs in a separate Go process
✅ Each parallel Ginkgo process gets a separate schema
❌ But all tiers share the SAME PostgreSQL database
❌ And sequential tiers use the SAME schema (public)
❌ And cleanup is testID-based (timestamp-specific)

Result: Test data accumulates across tiers
```

---

## 🚀 **Bottom Line**

**"Own environment"** means:
- ✅ Own Go process
- ✅ Own test execution context
- ✅ Own schema (if running in parallel)
- ❌ NOT own database instance
- ❌ NOT isolated from other test tiers

**The database is shared infrastructure**, just like PostgreSQL container is shared across all test tiers.

---

## 📊 **Summary Table**

| Isolation Level | Supported? | Evidence |
|----------------|------------|----------|
| **Parallel Processes (within tier)** | ✅ YES | `test_process_N` schemas |
| **Sequential Tiers (unit→int→e2e)** | ❌ NO | Same `action_history` database |
| **Separate Go Processes** | ✅ YES | Separate test executables |
| **Separate Database Instances** | ❌ NO | All use `localhost:15433` |
| **Separate Schemas Per Tier** | ❌ NO | All use `public` (or process schema) |

---

**Created**: December 18, 2025, 09:30
**Status**: ✅ **EXPLAINED**
**Recommendation**: Add tier-specific schema isolation (P2 enhancement)


