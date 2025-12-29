# WorkflowExecution E2E DataStorage Timeout Investigation

**Date**: December 18, 2025
**Service**: WorkflowExecution
**Status**: 🚧 **IN PROGRESS** - DataStorage deployment timeout during E2E setup
**Priority**: P1 - Blocking E2E tests

---

## 📋 **Executive Summary**

WorkflowExecution E2E tests are failing during infrastructure setup because DataStorage deployment never becomes ready. The issue surfaced when we updated E2E setup to apply all database migrations (not just audit migrations) to fix workflow catalog registration.

**Root Cause**: Unknown - DataStorage pod is not reaching ready state
**Impact**: All WE E2E tests blocked (0/9 specs run)
**Blocker**: E2E validation of recent WE enhancements

---

## 🔍 **Problem Timeline**

### **Initial Issue: Workflow Registration Failed**
1. E2E tests were applying only audit migrations
2. Workflow catalog table was missing required columns
3. Workflow registration failed with HTTP 500

### **First Fix Attempt: Apply Workflow Catalog Migrations**
1. Added `WorkflowCatalogTables` list
2. Applied workflow catalog migrations separately
3. Still failed - table had old schema (only migration 015)

### **Second Fix Attempt: Apply ALL Migrations**
1. Changed to `ApplyAllMigrations()` to get complete schema
2. Includes migrations 015-022 for workflow catalog
3. **NEW PROBLEM**: DataStorage deployment never becomes ready
4. Test times out after 15 minutes (suite timeout)

---

## 🧪 **Test Execution Details**

### **Test Command**
```bash
make test-e2e-workflowexecution
```

### **Failure Point**
```
PHASE 3: Deploying Data Storage + migrations...
  deployment.apps/datastorage created
  ⏳ Waiting for Data Storage to be ready...
  [TIMEDOUT] A suite timeout occurred (15 minutes)
```

### **What Works**
- ✅ Kind cluster creation
- ✅ Tekton installation
- ✅ PostgreSQL deployment (becomes ready)
- ✅ Redis deployment (becomes ready)
- ✅ DataStorage image build

### **What Fails**
- ❌ DataStorage deployment readiness check (times out after 15 minutes)

---

## 🔧 **Changes Made**

### **Files Modified**

1. **test/infrastructure/migrations.go**
   - Added: `var WorkflowCatalogTables = []string{"remediation_workflow_catalog"}`

2. **test/infrastructure/workflowexecution_parallel.go**
   - Changed from: Apply audit migrations → Apply workflow catalog migrations
   - Changed to: Apply ALL migrations (auto-discovered)
   - Rationale: Workflow catalog requires migrations 015-022, not just 015

**Code Change:**
```go
// OLD (broken - only applies migration 015):
Tables: WorkflowCatalogTables,

// NEW (triggers timeout):
ApplyAllMigrations(context.Background(), WorkflowExecutionNamespace, kubeconfigPath, output)
```

---

## 🤔 **Possible Causes**

### **Hypothesis 1: Migration Failure**
- One of the migrations might be failing silently
- DataStorage expects schema but gets different schema
- DataStorage crashes on startup due to schema mismatch

**Next Step**: Check DataStorage pod logs in the Kind cluster

### **Hypothesis 2: Missing Migration in E2E**
- Some migrations might not be safe to run multiple times
- Some migrations might require specific order or dependencies
- Auto-discovery might apply migrations in wrong order

**Next Step**: Compare migration order with DataStorage service

### **Hypothesis 3: Resource Contention**
- Applying ALL migrations takes longer
- DataStorage needs migrations complete before starting
- Kind cluster running out of resources

**Next Step**: Check if migrations are still running when DataStorage starts

### **Hypothesis 4: Configuration Mismatch**
- DataStorage config expects certain schema state
- ALL migrations changes schema in unexpected way
- DataStorage readiness probe fails due to schema issues

**Next Step**: Review DataStorage E2E setup vs WE E2E setup

---

## 📊 **Investigation Checklist**

### **Immediate Investigation Needed**
- [ ] Check DataStorage pod logs during timeout
- [ ] Verify migrations actually applied (check PostgreSQL schema)
- [ ] Compare with DataStorage's own E2E setup
- [ ] Check if migrations complete before DataStorage starts
- [ ] Review DataStorage health check / readiness probe

### **Questions to Answer**
1. Does DataStorage pod crash-loop or just not become ready?
2. Are migrations being applied before or after DataStorage starts?
3. What does DataStorage E2E setup do for migrations?
4. Are there any migrations that shouldn't be auto-applied?
5. Does the migration order matter?

---

## 🎯 **Recommended Next Steps**

### **Option A: Check DataStorage Pod**
**Action**: Investigate why DataStorage pod doesn't become ready
```bash
# During next test run, in another terminal:
kubectl logs -n kubernaut-system deployment/datastorage --kubeconfig=~/.kube/workflowexecution-e2e-config
kubectl describe pod -n kubernaut-system -l app=datastorage --kubeconfig=~/.kube/workflowexecution-e2e-config
```

**Pros**: Gets us concrete error messages
**Cons**: Requires catching the cluster before timeout cleanup

### **Option B: Compare with DataStorage E2E Setup**
**Action**: See how DataStorage service itself handles migrations in E2E tests
```bash
grep -r "ApplyAllMigrations\|ApplyMigrations" test/e2e/datastorage/
grep -r "migrations" test/integration/datastorage/
```

**Pros**: Learn from working setup
**Cons**: May not be applicable to WE setup

### **Option C: Revert to Table-Filtered Migrations**
**Action**: Go back to applying only specific migrations needed for WE
**Approach**: Apply audit migrations + workflow catalog migrations (015-022 only)

**Pros**: Faster, more targeted
**Cons**: Need to identify ALL required migrations manually

### **Option D: Debug Migration Application**
**Action**: Add detailed logging to migration application
**Approach**: See what migrations are applied, when, and if they succeed

**Pros**: Understand exactly what's happening
**Cons**: Requires code changes and re-running

---

## 📁 **Relevant Files**

### **E2E Setup**
- `test/infrastructure/workflowexecution_parallel.go` - Parallel E2E infrastructure
- `test/infrastructure/migrations.go` - Migration application logic
- `test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go` - Test suite

### **Migrations**
- `migrations/015_create_workflow_catalog_table.sql` - Creates base table
- `migrations/017-022_*.sql` - Workflow catalog schema updates

### **DataStorage**
- Check DataStorage E2E/integration test setup for comparison

---

## 🚨 **Impact**

### **Blocked Work**
- ✅ V2.2 audit pattern validation (unit/integration tests pass)
- ❌ E2E workflow execution testing (blocked)
- ❌ E2E failure scenario validation (blocked)
- ❌ E2E audit trail persistence validation (blocked)

### **Test Status**
- **Unit Tests**: ✅ All passing
- **Integration Tests**: ✅ 40/42 passing (2 moved to E2E)
- **E2E Tests**: ❌ 0/9 running (setup timeout)

---

## 💡 **Key Insights**

1. **Migration Complexity**: Workflow catalog requires multiple migrations (015-022)
2. **Table-Filtered Approach Incomplete**: Only applies migrations that list the table
3. **ApplyAllMigrations Side Effect**: Causes DataStorage deployment timeout
4. **Need Investigation**: Why DataStorage doesn't become ready with all migrations

---

## 📞 **Collaboration Points**

### **DataStorage Team**
- How do you handle migrations in your E2E tests?
- Do you apply all migrations or filter by table?
- Are there any migrations that shouldn't run in E2E?
- What's the expected schema state for DataStorage startup?

### **WorkflowExecution Team**
- Should we apply all migrations or just catalog-specific ones?
- Can we manually list migrations 015-022 instead of auto-discover?
- What's the minimum schema needed for workflow registration?

---

## ⏱️ **Time Spent**

- Migration fix development: ~30 minutes
- Test execution (3 runs × 15 min timeout): ~45 minutes
- Investigation and documentation: ~15 minutes
- **Total**: ~1.5 hours

---

## 🔄 **Next Session Handoff**

**When Resuming**:
1. Choose investigation option (A, B, C, or D above)
2. If Option A: Prepare to capture logs during test run
3. If Option B: Review DataStorage E2E setup
4. If Option C: Identify minimal migration set for workflow catalog
5. If Option D: Add migration logging and re-test

**Current State**:
- Code changes committed (ApplyAllMigrations approach)
- E2E tests fail with DataStorage timeout
- Need investigation to determine root cause

---

**Document Status**: 🚧 Active Investigation
**Last Updated**: December 18, 2025
**Next Review**: After DataStorage timeout investigation
**Related Documents**:
- WE_E2E_WORKFLOW_BUNDLE_SETUP_DEC_17_2025.md
- WE_INTEGRATION_TEST_STATUS_DEC_18_2025.md
- WE_AUDIT_V2_2_MIGRATION_COMPLETE_DEC_17_2025.md


