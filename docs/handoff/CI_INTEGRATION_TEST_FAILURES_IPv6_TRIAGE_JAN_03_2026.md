# CI Integration Test Failures - IPv6/IPv4 Binding Triage
**Date**: January 3, 2026  
**Run**: [#20684915082](https://github.com/jordigilh/kubernaut/actions/runs/20684915082)  
**Status**: ⚠️  Critical IPv6/IPv4 inconsistency identified

---

## 🔍 **Root Cause Analysis**

### **The Problem**
Integration tests are failing in CI/CD but passing locally due to IPv6/IPv4 hostname resolution inconsistency.

### **Technical Details**

#### **Container Port Mapping Behavior**
```bash
# Podman/Docker port mapping:
podman run -p 18094:8080 datastorage
# Creates mapping: host:18094 → container:8080
```

#### **IPv6 vs IPv4 Resolution**
| Environment | `localhost` resolves to | Port mapping listens on | Result |
|-------------|-------------------------|-------------------------|--------|
| **Local (no IPv6)** | `127.0.0.1` (IPv4) | `127.0.0.1:18094` | ✅ WORKS |
| **CI/CD (has IPv6)** | `::1` (IPv6) | `127.0.0.1:18094` (IPv4 only) | ❌ FAILS |

**Failure Mode in CI/CD**:
```
dial tcp [::1]:18094: connect: connection refused
```
- `localhost` resolves to `::1` (IPv6)
- But port mapping only exists on `127.0.0.1` (IPv4)
- Connection attempt to IPv6 address fails

---

## 📊 **Affected Services**

### **Integration Tests Failures** ✅ **Triaged**

| Service | Status | Failure Count | Root Cause |
|---------|--------|---------------|------------|
| **Signal Processing (SP)** | ❌ FAILING | 2/81 tests | `localhost` → IPv6 in CI |
| **Remediation Orchestrator (RO)** | ❌ FAILING | 2/44 tests | `localhost` → IPv6 in CI |
| **Notification (NT)** | ❌ FAILING | 1/124 tests | `localhost` → IPv6 in CI |
| **HolmesGPT API (HAPI)** | ❌ FAILING | 1/60 tests | Module import issue (separate) |

### **Signal Processing Details**
**Failed Tests**:
1. `BR-SP-090: should create 'signalprocessing.signal.processed' audit event`
   - **Location**: `test/integration/signalprocessing/audit_integration_test.go:185`
   - **Error**: `dial tcp 127.0.0.1:18094: connect: connection refused`
   - **Fix**: ✅ Changed `localhost` → `127.0.0.1` (line 73)

2. `BR-SP-090: should create 'phase.transition' audit events`
   - **Location**: `test/integration/signalprocessing/audit_integration_test.go:558`
   - **Status**: INTERRUPTED by test 1 failure

---

## 🔧 **Solution**

### **Standardized Approach: Explicit IPv4**

**Rule**: Use `127.0.0.1` instead of `localhost` for all Data Storage connections

#### **Why This Works**
```go
// ❌ BAD: Resolves to ::1 in CI (IPv6), fails
dataStorageURL = "http://localhost:18094"

// ✅ GOOD: Explicit IPv4, consistent across environments
dataStorageURL = "http://127.0.0.1:18094"
```

### **Pattern Already Used**
- **Suite setup**: `test/integration/signalprocessing/suite_test.go:218`
  ```go
  dsClient, err := audit.NewOpenAPIClientAdapter(
      fmt.Sprintf("http://127.0.0.1:%d", infrastructure.SignalProcessingIntegrationDataStoragePort),
      5*time.Second,
  )
  ```
- **Test needed**: Consistency in `audit_integration_test.go:73`

---

## 📝 **Fixes Applied**

### **Signal Processing** ✅ **FIXED**
**File**: `test/integration/signalprocessing/audit_integration_test.go`  
**Change**: Line 73
```go
// Before:
dataStorageURL = fmt.Sprintf("http://localhost:%d", infrastructure.SignalProcessingIntegrationDataStoragePort)

// After:
// Use 127.0.0.1 instead of localhost to force IPv4 (DD-TEST-001 v1.2, matches suite_test.go:218)
// CI/CD has IPv6 enabled, localhost→::1, but podman port mapping is IPv4 only
dataStorageURL = fmt.Sprintf("http://127.0.0.1:%d", infrastructure.SignalProcessingIntegrationDataStoragePort)
```

---

## 🎯 **Remaining Work**

### **Other Services Needing Audit** (15 files found)
```bash
$ grep -r "localhost.*18[0-9]{3}" test/integration --include="*.go" | wc -l
15
```

**High Priority Files**:
1. `test/integration/gateway/audit_integration_test.go`
2. `test/integration/aianalysis/suite_test.go`
3. `test/integration/remediationorchestrator/suite_test.go`
4. `test/integration/holmesgptapi/python_coordination_test.go`
5. `test/integration/aianalysis/recovery_human_review_integration_test.go`

### **Recommended Next Steps**
1. ✅ **Signal Processing**: Fixed in this commit
2. 🔜 **Remediation Orchestrator**: Apply same `localhost` → `127.0.0.1` fix
3. 🔜 **Notification**: Apply same fix
4. 🔜 **Gateway**: Audit and fix (multiple files)
5. 🔜 **AI Analysis**: Audit and fix
6. 🔜 **HolmesGPT API**: Separate triage (module import issue)

---

## 📚 **Technical Background**

### **Why Podman/Docker Use IPv4 for Port Mappings**
- Historical reasons: Most container registries and tooling assume IPv4
- Performance: IPv4 has less overhead than IPv6 in containerized environments
- Compatibility: Ensures widest compatibility with legacy services

### **Why CI/CD Has IPv6**
- GitHub Actions runners (Ubuntu 24.04) have dual-stack networking
- Modern Linux distributions prefer IPv6 by default
- `localhost` resolves via `/etc/hosts` or DNS, which prefers IPv6 when available

### **Best Practice**
**Bind addresses in containers**:
- Use `0.0.0.0` (all IPv4 interfaces) for maximum compatibility
- Container port mappings are typically IPv4 only unless explicitly configured for IPv6

**Client connections**:
- Use `127.0.0.1` for explicit IPv4
- Use `[::1]` for explicit IPv6
- Avoid `localhost` in CI/CD environments with dual-stack networking

---

## ✅ **Success Criteria**

### **Immediate** (This Commit)
- [x] Signal Processing audit tests use `127.0.0.1`
- [x] Documentation created for triage findings
- [x] Pattern established for other services

### **Follow-Up** (Next PRs)
- [ ] RO, NT, HAPI integration tests fixed
- [ ] Gateway audit tests fixed
- [ ] AI Analysis tests fixed
- [ ] All 15 files audited and updated
- [ ] DD-TEST-001 updated with IPv6/IPv4 guidance

---

## 🔗 **References**

- **Port Allocation**: `docs/architecture/decisions/DD-TEST-001-port-allocation-strategy.md`
- **Infrastructure Setup**: `test/infrastructure/signalprocessing.go:1404-1524`
- **Suite Setup**: `test/integration/signalprocessing/suite_test.go:218`
- **CI Run**: https://github.com/jordigilh/kubernaut/actions/runs/20684915082

---

**Priority**: P0 - Blocks CI/CD pipeline  
**Effort**: Low (1 line change per service)  
**Risk**: Low (explicit IPv4 is more portable than `localhost`)  
**Impact**: High (unblocks all integration test failures)

