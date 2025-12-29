# 🚨 CRITICAL: Notification Integration Test Infrastructure Missing

**Date**: December 18, 2025
**Service**: Notification
**Issue**: Integration tests expect real Data Storage but no podman-compose infrastructure exists
**Impact**: 6/113 audit-related integration tests fail (infrastructure dependency)

---

## 📋 **Issue Summary**

### **Problem**
- ❌ **No `podman-compose.notification.test.yml` file exists**
- ❌ **Integration tests expect REAL Data Storage service** (not mocked)
- ❌ **Tests fail with "Data Storage not available" errors**
- ❌ **6 audit-related integration tests blocked**

### **Root Cause**
**Incorrect Assumption**: Integration tests were assumed to use only envtest (in-memory Kubernetes)
**Reality**: Integration tests require:
1. **PostgreSQL** (for Data Storage)
2. **Redis** (for Data Storage caching)
3. **Data Storage service** (for audit event persistence)

### **Evidence**
```go
// test/integration/notification/audit_integration_test.go:72-86
// Check if Data Storage is available (REQUIRED per DD-AUDIT-003)
// Per TESTING_GUIDELINES.md: Integration tests MUST fail when required infrastructure is unavailable
// NO Skip() allowed - audit infrastructure is MANDATORY for compliance
resp, err := httpClient.Get(dataStorageURL + "/health")
if err != nil || resp.StatusCode != http.StatusOK {
    Fail(fmt.Sprintf(
        "❌ REQUIRED: Data Storage not available at %s\n"+
        "  Per DD-AUDIT-003: Audit infrastructure is MANDATORY\n"+
        "  Per TESTING_GUIDELINES.md: Integration tests MUST use real services (no Skip() allowed)\n\n"+
        "  To run these tests, start infrastructure:\n"+
        "    cd test/integration/notification\n"+
        "    # TODO: Create podman-compose.notification.test.yml with DataStorage/PostgreSQL/Redis\n"+
        "    podman-compose -f podman-compose.notification.test.yml up -d\n\n"+
        "  Or use shared infrastructure from another service (e.g., port 18090)\n"+
        "  Verify with: curl %s/health",
        dataStorageURL, dataStorageURL))
}
```

---

## 🔍 **Comparison with Other Services**

### **WorkflowExecution Integration Infrastructure** (✅ Reference)
```yaml
# test/integration/workflowexecution/podman-compose.test.yml
services:
  postgres:    # Port: 15443 (WE baseline +10)
  redis:       # Port: 16389 (WE baseline +10)
  datastorage: # Port: 18100 (WE baseline +10)
```

### **Notification Integration Infrastructure** (❌ MISSING)
```bash
$ ls test/integration/notification/
# No podman-compose.notification.test.yml found!
# No config/ directory found!
```

---

## 🎯 **Required Implementation**

### **Files to Create**

#### 1. **`test/integration/notification/podman-compose.notification.test.yml`**
**Port Scheme**:
- PostgreSQL: `15453` (DS baseline 15433 + 20)
- Redis: `16399` (DS baseline 16379 + 20)
- Data Storage: `18110` (DS baseline 18090 + 20)

**Services**:
- PostgreSQL 16-alpine
- Redis 7-alpine
- Data Storage (built from source)

#### 2. **`test/integration/notification/config/config.yaml`**
Data Storage configuration for Notification integration tests.

#### 3. **`test/integration/notification/config/db-secrets.yaml`**
PostgreSQL credentials (test environment only).

#### 4. **`test/integration/notification/config/redis-secrets.yaml`**
Redis credentials (empty password for test environment).

---

## 📊 **Test Failure Impact**

### **6 Integration Tests Blocked** (all audit-related)
1. ✅ `audit_integration_test.go` - Audit event persistence (BR-NOT-041)
2. ✅ `controller_audit_emission_test.go` - Controller audit emission (BR-NOT-041)
3. ❓ 4 other audit-related tests (TBD based on test run)

### **Expected Outcome After Fix**
- **Before**: 106/113 passing (6 infrastructure failures, 1 pre-existing bug)
- **After**: 112/113 passing (1 pre-existing bug remains)

---

## 🔧 **DD-TEST-001 v1.1 Correction**

### **Current (INCORRECT)**
```markdown
| **Notification** | ⚠️ N/A (envtest) | ✅ Kind | ✅ **COMPLETE** | Notification Team | Dec 18, 2025 |
```

### **Corrected**
```markdown
| **Notification** | ⏳ **TODO** (missing podman-compose) | ✅ Kind | 🟡 **PARTIAL** | Notification Team | Dec 18, 2025 |
```

**Explanation**:
- ✅ **E2E Cleanup**: COMPLETE (Kind image cleanup implemented)
- ⏳ **Integration Cleanup**: TODO (infrastructure doesn't exist yet, can't add cleanup)
- 🟡 **Status**: PARTIAL (E2E complete, Integration TODO)

---

## ✅ **Next Steps**

### **Priority 1: Create Infrastructure**
1. Create `podman-compose.notification.test.yml` (based on WE template)
2. Create `config/` directory with config.yaml, db-secrets.yaml, redis-secrets.yaml
3. Update `audit_integration_test.go` to use correct port (18110)

### **Priority 2: Add DD-TEST-001 v1.1 Cleanup**
After infrastructure exists:
1. Add `SynchronizedAfterSuite` cleanup in `suite_test.go`
2. Remove containers: `postgres`, `redis`, `datastorage`
3. Prune dangling images from podman-compose builds

### **Priority 3: Validate**
1. Start infrastructure: `podman-compose -f podman-compose.notification.test.yml up -d`
2. Run audit tests: `go test -v ./test/integration/notification/audit_*.go`
3. Verify 112/113 passing (only 1 pre-existing bug remains)

---

## 📝 **Confidence Assessment**

**Analysis Confidence**: 95%
- Evidence: 6 test files explicitly check for Data Storage availability
- Evidence: Other services (WE, DS, AI) use podman-compose for integration tests
- Evidence: Comments in `audit_integration_test.go` reference missing podman-compose.yml

**Implementation Confidence**: 90%
- Template: WorkflowExecution provides complete reference implementation
- Port Scheme: Clear pattern across services (DS baseline + offset)
- Risk: Minor adjustments may be needed for Notification-specific config

**Timeline**: ~2-3 hours
- File creation: 30 min
- Testing: 1-2 hours
- Documentation: 30 min

---

## 🔗 **Related Documents**
- DD-TEST-001 v1.1: `docs/handoff/NOTICE_DD_TEST_001_V1_1_INFRASTRUCTURE_IMAGE_CLEANUP_DEC_18_2025.md`
- WorkflowExecution Reference: `test/integration/workflowexecution/podman-compose.test.yml`
- Audit Integration Tests: `test/integration/notification/audit_integration_test.go`

