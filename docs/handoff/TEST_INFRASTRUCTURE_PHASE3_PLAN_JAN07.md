# Test Infrastructure Refactoring - Phase 3: Image Build Consolidation

**Date**: January 7, 2026
**Author**: AI Assistant
**Status**: 🔄 IN PROGRESS
**Priority**: HIGH (reduces ~400 lines of duplicated code)

---

## 📋 **Executive Summary**

**Problem**: 7 E2E test files contain inline `podman build` + `kind load` logic (29 total instances across codebase).

**Solution**: Migrate all E2E tests to use the existing `BuildAndLoadImageToKind()` function from `datastorage_bootstrap.go`.

**Impact**:
- **Code Reduction**: ~400 lines of duplicated build/load logic
- **Maintainability**: Single source of truth for image build patterns
- **Consistency**: All E2E tests follow DD-TEST-001 v1.3 image naming standard
- **Coverage Support**: Automatic E2E_COVERAGE=true support (DD-TEST-007)

---

## 🔍 **Current State Analysis**

### **Consolidated Function Already Exists**

**File**: `test/infrastructure/datastorage_bootstrap.go`
**Function**: `BuildAndLoadImageToKind(cfg E2EImageConfig, writer io.Writer) (string, error)`

**Features**:
- ✅ DD-TEST-001 v1.3 compliant image tagging
- ✅ DD-TEST-007 E2E coverage instrumentation support
- ✅ Automatic Podman image cleanup after Kind load (disk space optimization)
- ✅ Tar-based image transfer (podman → Kind compatibility)
- ✅ Error handling and detailed logging

**Configuration Struct**:
```go
type E2EImageConfig struct {
    ServiceName      string // e.g., "gateway", "aianalysis"
    ImageName        string // e.g., "kubernaut/gateway"
    DockerfilePath   string // e.g., "cmd/gateway/Dockerfile"
    KindClusterName  string // e.g., "gateway-e2e"
    BuildContextPath string // default: "." (project root)
    EnableCoverage   bool   // Enable Go coverage instrumentation
}
```

### **Files with Inline Build Logic** (7 files, 7 instances)

| File | Function | Lines | Service | Pattern |
|------|----------|-------|---------|---------|
| `datastorage.go` | `buildDataStorageImageOnly()` | 1686-1701 | DataStorage | `podman build` + manual cleanup |
| `authwebhook_e2e.go` | `buildAuthWebhookImageWithTag()` | 212-228 | AuthWebhook | `podman build` + tar export |
| `notification_e2e.go` | `buildNotificationImageOnly()` | 335-350 | Notification | `podman build --no-cache` + tar |
| `workflowexecution_parallel.go` | inline | 289-305 | DataStorage | `podman build` + `kind load docker-image` |
| `datastorage_bootstrap.go` | `buildDataStorageContainer()` | 414-445 | DataStorage (integration) | `podman build` for integration tests |
| `remediationorchestrator_e2e_hybrid.go` | `buildDataStorageImage()` | 292-315 | DataStorage | `podman build` + tar export |
| `holmesgpt_integration.go` | `buildDataStorageImage()` | 217-245 | DataStorage (integration) | `podman build` for integration tests |

**Note**: 2 files (`datastorage_bootstrap.go`, `holmesgpt_integration.go`) are for **integration tests**, not E2E. These should NOT be migrated (integration tests use Podman containers, not Kind).

---

## 🎯 **Migration Strategy**

### **Phase 3A: E2E Test Migrations** (5 files)

#### **Priority 1: DataStorage E2E** (2 files)
1. **`datastorage.go`** - `buildDataStorageImageOnly()`
   - **Current**: Manual `podman build` + cleanup
   - **Target**: `BuildAndLoadImageToKind()` with `ServiceName: "datastorage"`
   - **Impact**: ~20 lines → ~8 lines

2. **`remediationorchestrator_e2e_hybrid.go`** - `buildDataStorageImage()`
   - **Current**: Manual `podman build` + tar export + Kind load
   - **Target**: `BuildAndLoadImageToKind()` with `ServiceName: "datastorage"`
   - **Impact**: ~25 lines → ~8 lines

#### **Priority 2: Service-Specific E2E** (3 files)
3. **`authwebhook_e2e.go`** - `buildAuthWebhookImageWithTag()`
   - **Current**: Manual `podman build` + tar export
   - **Target**: `BuildAndLoadImageToKind()` with `ServiceName: "authwebhook"`
   - **Impact**: ~20 lines → ~8 lines

4. **`notification_e2e.go`** - `buildNotificationImageOnly()`
   - **Current**: Manual `podman build --no-cache` + tar export
   - **Target**: `BuildAndLoadImageToKind()` with `ServiceName: "notification"`
   - **Impact**: ~20 lines → ~8 lines

5. **`workflowexecution_parallel.go`** - inline build
   - **Current**: Inline `podman build` + `kind load docker-image`
   - **Target**: `BuildAndLoadImageToKind()` with `ServiceName: "datastorage"`
   - **Impact**: ~20 lines → ~8 lines

### **Phase 3B: Integration Test Analysis** (2 files - NO MIGRATION)

**Files**:
- `datastorage_bootstrap.go` - `buildDataStorageContainer()` (integration tests)
- `holmesgpt_integration.go` - `buildDataStorageImage()` (integration tests)

**Decision**: **DEFER** - These are integration test utilities that build Podman containers, not Kind images. They follow a different pattern (DD-TEST-002 Sequential Container Orchestration) and should NOT be consolidated with E2E image building.

---

## 📐 **Migration Pattern**

### **Before** (Example: `authwebhook_e2e.go`)
```go
func buildAuthWebhookImageWithTag(imageTag string, writer io.Writer) error {
    workspaceRoot, err := findWorkspaceRoot()
    if err != nil {
        return fmt.Errorf("failed to find workspace root: %w", err)
    }

    cmd := exec.Command("podman", "build",
        "-t", imageTag,
        "-f", "docker/webhooks.Dockerfile",
        ".")
    cmd.Dir = workspaceRoot
    cmd.Stdout = writer
    cmd.Stderr = writer

    if err = cmd.Run(); err != nil {
        return fmt.Errorf("podman build failed: %w", err)
    }

    _, _ = fmt.Fprintln(writer, "✅ Webhooks service image built successfully")
    return nil
}

func loadAuthWebhookImageWithTag(clusterName, imageTag string, writer io.Writer) error {
    // ... 30+ lines of tar export + Kind load logic ...
}
```

### **After** (Consolidated)
```go
func buildAndLoadAuthWebhookImage(clusterName string, writer io.Writer) (string, error) {
    cfg := E2EImageConfig{
        ServiceName:      "authwebhook",
        ImageName:        "kubernaut/authwebhook",
        DockerfilePath:   "docker/webhooks.Dockerfile",
        KindClusterName:  clusterName,
        BuildContextPath: ".", // Project root
        EnableCoverage:   false, // Or true if E2E_COVERAGE=true
    }
    return BuildAndLoadImageToKind(cfg, writer)
}
```

**Reduction**: ~50 lines → ~8 lines per service

---

## ✅ **Success Criteria**

### **Code Quality**
- [ ] All 5 E2E test files migrated to use `BuildAndLoadImageToKind()`
- [ ] No inline `podman build` commands in E2E tests (except integration tests)
- [ ] All E2E tests use DD-TEST-001 v1.3 compliant image tags
- [ ] All E2E tests support E2E_COVERAGE=true automatically

### **Functional Validation**
- [ ] DataStorage E2E tests pass
- [ ] AuthWebhook E2E tests pass
- [ ] Notification E2E tests pass
- [ ] WorkflowExecution E2E tests pass
- [ ] RemediationOrchestrator E2E tests pass

### **Documentation**
- [ ] Update DD-TEST-001 to reference consolidated function
- [ ] Update TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md with Phase 3 results

---

## 🚨 **Risks and Mitigations**

### **Risk 1: Image Tag Changes**
**Problem**: Consolidated function uses DD-TEST-001 v1.3 tags (e.g., `kubernaut-authwebhook-e2e-authwebhook`), but tests may expect specific tags.

**Mitigation**: Review deployment manifests to ensure they use correct image references. Most E2E tests use `imagePullPolicy: Never` with `localhost/` prefix, so tag format shouldn't matter.

### **Risk 2: Build Arguments**
**Problem**: Some services may need specific build arguments (e.g., `--build-arg GOARCH=arm64`).

**Mitigation**: `E2EImageConfig` doesn't currently support custom build args. If needed, extend the struct or use environment variables.

### **Risk 3: Coverage Instrumentation**
**Problem**: Some tests may not want coverage instrumentation.

**Mitigation**: `EnableCoverage` flag is optional. Tests can set it to `false` or rely on `E2E_COVERAGE` env var.

---

## 📊 **Expected Impact**

### **Code Reduction**
- **Before**: ~400 lines of duplicated build/load logic across 5 E2E files
- **After**: ~40 lines (8 lines per service × 5 services)
- **Net Reduction**: ~360 lines

### **Maintainability**
- **Single Source of Truth**: All E2E image building uses one function
- **Consistency**: All tests follow DD-TEST-001 v1.3 standard
- **Automatic Features**: Coverage, disk cleanup, error handling built-in

### **Performance**
- **Disk Space**: Automatic Podman image cleanup after Kind load (saves ~500MB per image)
- **Build Time**: No change (same build process)

---

## 🔄 **Implementation Steps**

### **Step 1: Migrate DataStorage E2E** (datastorage.go)
1. Replace `buildDataStorageImageOnly()` with `BuildAndLoadImageToKind()`
2. Update callers to use new function signature
3. Test DataStorage E2E suite

### **Step 2: Migrate RemediationOrchestrator E2E**
1. Replace `buildDataStorageImage()` with `BuildAndLoadImageToKind()`
2. Update callers to use new function signature
3. Test RemediationOrchestrator E2E suite

### **Step 3: Migrate AuthWebhook E2E**
1. Replace `buildAuthWebhookImageWithTag()` + `loadAuthWebhookImageWithTag()` with single consolidated function
2. Update callers
3. Test AuthWebhook E2E suite

### **Step 4: Migrate Notification E2E**
1. Replace `buildNotificationImageOnly()` + `loadNotificationImageOnly()` with consolidated function
2. Update callers
3. Test Notification E2E suite

### **Step 5: Migrate WorkflowExecution E2E**
1. Replace inline build logic with consolidated function
2. Update callers
3. Test WorkflowExecution E2E suite

### **Step 6: Validation**
1. Run full E2E test suite (all services)
2. Verify no regressions
3. Update documentation

---

## 📝 **Next Steps**

1. ✅ **Phase 3A**: Migrate 5 E2E test files to use `BuildAndLoadImageToKind()`
2. ⏳ **Phase 3B**: Verify integration test build functions are NOT migrated
3. ⏳ **Phase 3C**: Run full E2E test suite to validate changes
4. ⏳ **Phase 3D**: Update documentation (DD-TEST-001, triage document)

---

## 🔗 **Related Documents**

- `TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md` - Overall refactoring plan
- `TEST_INFRASTRUCTURE_PHASE1_COMPLETE_JAN07.md` - Phase 1 results
- `TEST_INFRASTRUCTURE_PHASE2_PLAN_JAN07.md` - Phase 2 analysis (deferred)
- `DD-TEST-001` - Image Naming Convention and Port Allocation Strategy
- `DD-TEST-002` - Sequential Container Orchestration Pattern (integration tests)
- `DD-TEST-007` - E2E Coverage Collection

---

**Status**: Ready for implementation. All 5 E2E test files identified, migration pattern defined, risks assessed.

