# Gateway Recovery Plan - DD-GATEWAY-004 Authentication Removal

**Date**: 2025-10-27
**Status**: 🔄 **IN PROGRESS**
**Objective**: Remove OAuth2 authentication from Gateway and restore to working state

---

## 📊 **CURRENT STATE ASSESSMENT**

### **File Status**
| File | Status | Action Needed |
|------|--------|---------------|
| `pkg/gateway/server.go` | ❌ Deleted (was tracked) | Restore from git |
| `pkg/gateway/server/server.go` | ⚠️ New (untracked) | Keep (DD-GATEWAY-004 changes) |
| `pkg/gateway/server/handlers.go` | ⚠️ Corrupted → Minimal | Rebuild from scratch |
| `pkg/gateway/server/responses.go` | ✅ Fixed | Keep |
| `pkg/gateway/server/health.go` | ✅ Good | Keep |
| `pkg/gateway/server/middleware.go` | ✅ Good | Keep |
| `pkg/gateway/metrics/metrics.go` | ✅ Enhanced | Keep (added missing fields) |
| `pkg/gateway/processing/deduplication.go` | ✅ Enhanced | Keep (added `Record()` method) |
| `pkg/gateway/processing/storm_aggregator.go` | ✅ Enhanced | Keep (added `AggregateOrCreate()`) |
| `pkg/gateway/processing/crd_creator.go` | ✅ Enhanced | Keep (added storm methods) |
| `pkg/gateway/middleware/auth.go` | ✅ Deleted | Correct (DD-GATEWAY-004) |
| `pkg/gateway/middleware/authz.go` | ✅ Deleted | Correct (DD-GATEWAY-004) |

### **Key Issues Identified**
1. **API Signature Mismatches**: Minimal `handlers.go` uses incorrect API signatures
2. **Missing Methods**: Several adapter and processing methods don't exist
3. **Package Structure**: Old code was `package gateway`, new is `package server`

---

## 🎯 **RECOVERY STRATEGY**

### **Phase 1: Restore Old Working Code** ✅
**Goal**: Get back to a known-good baseline
**Duration**: 10 minutes

1. ✅ Restore `pkg/gateway/server.go` from git
2. ✅ Check if old code compiles
3. ✅ Identify what needs to be adapted for DD-GATEWAY-004

### **Phase 2: Apply DD-GATEWAY-004 Changes** 🔄
**Goal**: Remove authentication cleanly from working code
**Duration**: 20 minutes

1. Remove `k8sClientset` parameter from `NewServer()`
2. Remove `DisableAuth` from `Config` struct
3. Remove `TokenReviewAuth` and `SubjectAccessReviewAuthz` middleware
4. Remove authentication metrics
5. Update tests to remove authentication setup

### **Phase 3: Package Restructuring** ⏳
**Goal**: Move from `pkg/gateway/` to `pkg/gateway/server/`
**Duration**: 15 minutes

1. Create proper `pkg/gateway/server/` package structure
2. Split `server.go` into logical files:
   - `server.go` - Server struct and constructor
   - `handlers.go` - HTTP handlers
   - `responses.go` - Response helpers
   - `health.go` - Health endpoints
   - `middleware.go` - Middleware setup
3. Update imports across codebase

### **Phase 4: Validation** ⏳
**Goal**: Ensure everything compiles and tests pass
**Duration**: 15 minutes

1. Build all Gateway packages
2. Run unit tests
3. Run integration tests (basic smoke test)
4. Document any remaining issues

---

## 🔧 **DETAILED EXECUTION STEPS**

### **Phase 1: Restore Old Working Code**

#### Step 1.1: Restore `pkg/gateway/server.go`
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
git restore pkg/gateway/server.go
```

#### Step 1.2: Backup new files
```bash
mkdir -p /tmp/gateway-recovery-backup
cp -r pkg/gateway/server/ /tmp/gateway-recovery-backup/
cp pkg/gateway/metrics/metrics.go /tmp/gateway-recovery-backup/
cp pkg/gateway/processing/*.go /tmp/gateway-recovery-backup/
```

#### Step 1.3: Test old code compilation
```bash
go build ./pkg/gateway/...
```

**Expected**: Should compile with authentication still present

---

### **Phase 2: Apply DD-GATEWAY-004 Changes**

#### Step 2.1: Remove authentication from `server.go`

**Changes to make**:
1. Remove `k8sClientset kubernetes.Interface` from `Server` struct
2. Remove `DisableAuth bool` from `Config` struct
3. Remove `k8sClientset` parameter from `NewServer()`
4. Remove `ValidateAuthConfig()` call
5. Remove conditional auth middleware from handler setup

#### Step 2.2: Remove authentication metrics

**File**: `pkg/gateway/metrics/metrics.go`

**Remove**:
- `TokenReviewRequests`
- `TokenReviewTimeouts`
- `SubjectAccessReviewRequests`
- `SubjectAccessReviewTimeouts`
- `K8sAPILatency`

**Keep** (already added):
- All other metrics we added today
- Custom registry support

#### Step 2.3: Update tests

**Files to update**:
- `test/integration/gateway/helpers.go` - Remove `k8sClientset` from `StartTestGateway()`
- Delete `test/integration/gateway/security_integration_test.go`

---

### **Phase 3: Package Restructuring**

#### Step 3.1: Create `pkg/gateway/server/` package

**Approach**: Keep both structures temporarily
1. Old `pkg/gateway/server.go` stays as-is
2. New `pkg/gateway/server/` package coexists
3. Gradually migrate imports

#### Step 3.2: Split server.go into logical files

**Target structure**:
```
pkg/gateway/server/
├── server.go          # Server struct, NewServer(), Start()
├── handlers.go        # handlePrometheusWebhook(), etc.
├── responses.go       # respondJSON(), respondError(), etc.
├── health.go          # handleHealth(), handleReadiness(), handleLiveness()
├── middleware.go      # setupMiddleware()
└── types.go           # Response types
```

---

### **Phase 4: Validation**

#### Step 4.1: Build validation
```bash
go build ./pkg/gateway/...
go build ./cmd/prometheus-alerts-slm/...
```

#### Step 4.2: Unit test validation
```bash
go test ./pkg/gateway/... -v
```

#### Step 4.3: Integration test smoke test
```bash
cd test/integration/gateway
./run-tests-kind.sh
```

---

## 📈 **SUCCESS CRITERIA**

### **Phase 1 Success**
- ✅ Old `pkg/gateway/server.go` restored from git
- ✅ Old code compiles without errors
- ✅ Backup of new code created

### **Phase 2 Success**
- ✅ Authentication removed from server
- ✅ No `k8sClientset` references
- ✅ No authentication metrics
- ✅ Code compiles without authentication

### **Phase 3 Success**
- ✅ `pkg/gateway/server/` package structure created
- ✅ All files split logically
- ✅ Imports updated across codebase
- ✅ Code compiles with new structure

### **Phase 4 Success**
- ✅ All Gateway packages build successfully
- ✅ Unit tests pass (>90%)
- ✅ Integration tests pass (>80%)
- ✅ No authentication code remains

---

## 🚨 **RISK MITIGATION**

### **Risk 1: Old code doesn't compile**
**Mitigation**: We have backups of all new code in `/tmp/gateway-recovery-backup/`

### **Risk 2: Tests fail after authentication removal**
**Mitigation**: Update tests incrementally, keep DD-GATEWAY-004 documentation as reference

### **Risk 3: Package restructuring breaks imports**
**Mitigation**: Use `go build` after each file move, fix imports immediately

---

## 📝 **EXECUTION LOG**

### **2025-10-27 19:40 - Phase 1 Started**
- Created recovery plan document
- Assessed current file status
- Identified restoration strategy

### **Next Steps**
1. Execute Phase 1: Restore old working code
2. Apply DD-GATEWAY-004 changes cleanly
3. Restructure package if needed
4. Validate and test

---

## 🔗 **REFERENCES**

- [DD-GATEWAY-004](docs/decisions/DD-GATEWAY-004-authentication-strategy.md) - Authentication removal decision
- [Gateway Security Deployment Guide](docs/deployment/gateway-security.md) - Network-level security
- Old working code: `git show HEAD:pkg/gateway/server.go`

---

**Status**: 🔄 **Phase 1 - Ready to Execute**


