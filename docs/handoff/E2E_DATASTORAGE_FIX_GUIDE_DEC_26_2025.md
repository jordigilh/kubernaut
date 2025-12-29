# E2E DataStorage Image Tag Fix Guide - December 26, 2025

**Status**: 📋 IMPLEMENTATION GUIDE  
**Applies To**: RemediationOrchestrator, Gateway, SignalProcessing

---

## ✅ **Reference Implementation: WorkflowExecution (CORRECT)**

WorkflowExecution now follows the correct pattern:

```go
// Phase 0: Generate tag BEFORE building
dataStorageImageName := GenerateInfraImageName("datastorage", "workflowexecution")

// Phase 1: Build with dynamic tag
buildDataStorageImageWithTag(dataStorageImageName, writer)

// Phase 3: Load with SAME dynamic tag
loadDataStorageImageWithTag(clusterName, dataStorageImageName, writer)

// Phase 4: Deploy with SAME dynamic tag  
deployDataStorageServiceInNamespace(..., dataStorageImageName, ...)
```

**Result**: Fresh DataStorage build with latest code, unique tag per service ✅

---

## 🔧 **Changes Required for Each Service**

### 1. RemediationOrchestrator

**File**: `test/infrastructure/remediationorchestrator_e2e_hybrid.go`

#### Change 1: Add Phase 0 (BEFORE Phase 1)

**Location**: Line ~54 (before Phase 1 comment)

**ADD**:
```go
// ═══════════════════════════════════════════════════════════════════════
// PHASE 0: Generate dynamic image tags (BEFORE building)
// ═══════════════════════════════════════════════════════════════════════
// Generate DataStorage image tag ONCE (non-idempotent, timestamp-based)
// This ensures each service builds its OWN DataStorage with LATEST code
// Per DD-TEST-001: Dynamic tags for parallel E2E isolation
dataStorageImageName := GenerateInfraImageName("datastorage", "remediationorchestrator")
fmt.Fprintf(writer, "📛 DataStorage dynamic tag: %s\n", dataStorageImageName)
fmt.Fprintln(writer, "   (Ensures fresh build with latest DataStorage code)")
```

#### Change 2: Modify Phase 1 Build

**Location**: Line ~75-78

**CHANGE FROM**:
```go
// Build DataStorage in parallel
go func() {
    err := buildDataStorageImage(writer)
    buildResults <- buildResult{name: "DataStorage", err: err}
}()
```

**CHANGE TO**:
```go
// Build DataStorage with dynamic tag in parallel
go func() {
    err := buildDataStorageImageWithTag(dataStorageImageName, writer)
    buildResults <- buildResult{name: "DataStorage", err: err}
}()
```

#### Change 3: Modify Phase 3 Load

**Location**: Line ~156-159

**CHANGE FROM**:
```go
// Load DataStorage image (with static tag first)
go func() {
    err := loadDataStorageImage(clusterName, writer)
    buildResults <- buildResult{name: "DataStorage", err: err}
}()
```

**CHANGE TO**:
```go
// Load DataStorage image with dynamic tag
go func() {
    err := loadDataStorageImageWithTag(clusterName, dataStorageImageName, writer)
    buildResults <- buildResult{name: "DataStorage", err: err}
}()
```

#### Change 4: REMOVE Phase 3.5 Entirely

**Location**: Lines ~182-189

**DELETE THIS ENTIRE SECTION**:
```go
// ═══════════════════════════════════════════════════════════════════════
// PHASE 3.5: Tag DataStorage image with dynamic name (prevents collisions)
// ═══════════════════════════════════════════════════════════════════════
fmt.Fprintln(writer, "\n🏷️  Tagging DataStorage image with dynamic name...")
// ... (all Phase 3.5 code)
if err := tagDataStorageImageInKind(clusterName, dataStorageImageName, writer); err != nil {
    return fmt.Errorf("failed to tag DataStorage image: %w", err)
}
fmt.Fprintf(writer, "  ✅ DataStorage tagged: %s\n", dataStorageImageName)
```

#### Change 5: REMOVE duplicate tag generation in Phase 3

**Location**: Line ~143-146

**DELETE**:
```go
// Generate DataStorage image name ONCE (non-idempotent, likely timestamp-based)
// This prevents mismatches between loading and deployment phases
dataStorageImageName := GenerateInfraImageName("datastorage", "remediationorchestrator")
fmt.Fprintf(writer, "  📛 DataStorage dynamic tag: %s\n", dataStorageImageName)
```

(Tag is now generated in Phase 0)

#### Change 6: REMOVE tagDataStorageImageInKind function

**Location**: Lines ~294-324

**DELETE THIS ENTIRE FUNCTION** (now obsolete):
```go
// tagDataStorageImageInKind tags the loaded DataStorage image...
func tagDataStorageImageInKind(clusterName, dynamicTag string, writer io.Writer) error {
    // ... (entire function)
}
```

---

### 2. Gateway

**File**: `test/infrastructure/gateway_e2e_hybrid.go`

#### Change 1: Add Phase 0 (BEFORE Phase 1)

**Location**: Before Phase 1 comment

**ADD**:
```go
// ═══════════════════════════════════════════════════════════════════════
// PHASE 0: Generate dynamic image tags (BEFORE building)
// ═══════════════════════════════════════════════════════════════════════
dataStorageImageName := GenerateInfraImageName("datastorage", "gateway")
fmt.Fprintf(writer, "📛 DataStorage dynamic tag: %s\n", dataStorageImageName)
fmt.Fprintln(writer, "   (Ensures fresh build with latest DataStorage code)")
```

#### Change 2: Modify Phase 1 Build

**CHANGE FROM**:
```go
err := buildDataStorageImage(writer)
```

**CHANGE TO**:
```go
err := buildDataStorageImageWithTag(dataStorageImageName, writer)
```

#### Change 3: Modify Phase 3 Load

**ADD BEFORE Phase 3**:
```go
// Load DataStorage with dynamic tag
go func() {
    err := loadDataStorageImageWithTag(clusterName, dataStorageImageName, writer)
    loadResults <- buildResult{name: "DataStorage", err: err}
}()
```

#### Change 4: Modify Phase 4 Deploy

**Location**: Line ~191

**CHANGE FROM**:
```go
err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, GenerateInfraImageName("datastorage", "gateway"), writer)
```

**CHANGE TO**:
```go
// CRITICAL: Use the tag generated in Phase 0 (UUID-based, non-idempotent)
// This ensures we deploy the SAME fresh-built image with latest DataStorage code
err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, dataStorageImageName, writer)
```

---

### 3. SignalProcessing

**File**: `test/infrastructure/signalprocessing_e2e_hybrid.go`

**Same changes as Gateway**:
1. Add Phase 0 with tag generation
2. Modify Phase 1 to use `buildDataStorageImageWithTag(dataStorageImageName, writer)`
3. Modify Phase 3 to use `loadDataStorageImageWithTag(clusterName, dataStorageImageName, writer)`
4. Modify Phase 4 deploy to use `dataStorageImageName` (not regenerate)

---

## 🎯 **Validation After Changes**

### For Each Service, Verify:

1. ✅ **Phase 0 exists** and generates tag BEFORE Phase 1
2. ✅ **Phase 1** uses `buildDataStorageImageWithTag(dataStorageImageName, ...)`
3. ✅ **Phase 3** uses `loadDataStorageImageWithTag(clusterName, dataStorageImageName, ...)`
4. ✅ **Phase 4** uses `dataStorageImageName` from Phase 0
5. ✅ **NO Phase 3.5** re-tagging operations
6. ✅ **NO** `tagDataStorageImageInKind()` function
7. ✅ **NO** `GenerateInfraImageName()` called at deploy time

### Expected Result:

Each service builds and uses its OWN fresh DataStorage image:
- WorkflowExecution: `localhost/datastorage:workflowexecution-abc123`
- RemediationOrchestrator: `localhost/datastorage:remediationorchestrator-def456`
- Gateway: `localhost/datastorage:gateway-ghi789`
- SignalProcessing: `localhost/datastorage:signalprocessing-jkl012`

All different tags, all fresh builds with latest code! ✅

---

**Status**: 📋 READY FOR IMPLEMENTATION  
**Next Step**: Apply changes to RO, Gateway, SP systematically

