# E2E Test Infrastructure - Hybrid Pattern Migration Plan

**Date**: January 7, 2026
**Author**: AI Assistant
**Status**: PLANNING
**Authority**: User Decision - Standardize all E2E services on hybrid pattern

---

## Executive Summary

Migrate all E2E test services to use the **hybrid parallel pattern** (build-before-cluster) for optimal performance and standardization.

**Current State**: Two patterns exist
- **Hybrid Pattern** (Optimal): RO, WE, SP, Gateway Hybrid - Build images → Create cluster → Load → Deploy
- **Standard Pattern** (Suboptimal): Gateway, Notification, DataStorage, AuthWebhook - Create cluster → Build images (cluster idles) → Deploy

**Target State**: Single hybrid pattern for all services
- All E2E services use: Build images → Create cluster → Load → Deploy
- No cluster idle time during image builds
- Consistent infrastructure pattern across all services

---

## Migration Targets

### Services to Migrate (4)

| Service | Current File | Lines | Pattern | Complexity |
|---------|-------------|-------|---------|------------|
| Gateway | `gateway_e2e.go` | 1,279 | Standard | High (2 setup functions) |
| Notification | `notification_e2e.go` | 647 | Standard | Medium |
| DataStorage | `datastorage.go` | 717 | Standard | Medium |
| AuthWebhook | `authwebhook_e2e.go` | 708 | Standard | Medium |

**Total**: ~3,351 lines to migrate

### Services Already Using Hybrid (4)

| Service | File | Status |
|---------|------|--------|
| RemediationOrchestrator | `remediationorchestrator_e2e_hybrid.go` | ✅ Reference implementation |
| WorkflowExecution | `workflowexecution_e2e_hybrid.go` | ✅ Reference implementation |
| SignalProcessing | `signalprocessing_e2e_hybrid.go` | ✅ Reference implementation |
| Gateway Hybrid | `gateway_e2e_hybrid.go` | ✅ Reference implementation |

---

## Hybrid Pattern Template

### Standard Hybrid Setup Sequence

```go
// PHASE 0: Generate dynamic image tags
dataStorageImage := GenerateInfraImageName("datastorage", "servicename")
serviceImage := GenerateInfraImageName("servicename", "e2e")

// PHASE 1: Build images IN PARALLEL (before cluster creation)
// - Build service image with coverage (if applicable)
// - Build DataStorage image with dynamic tag
// Wait for both builds to complete

// PHASE 2: Create Kind cluster (now that images are ready)
// - Create cluster with extraMounts for coverage (if applicable)
// - Install CRDs
// - Create namespace

// PHASE 3: Load images to Kind
// - Load service image
// - Load DataStorage image

// PHASE 4: Deploy infrastructure in parallel
// - Deploy PostgreSQL
// - Deploy Redis
// - Apply migrations
// - Deploy DataStorage
// - Deploy service (if applicable)
```

### Key Differences from Standard Pattern

| Aspect | Standard Pattern | Hybrid Pattern |
|--------|-----------------|----------------|
| **Cluster Creation** | Phase 1 (first) | Phase 2 (after builds) |
| **Image Building** | Phase 2 (cluster idles) | Phase 1 (no cluster yet) |
| **Image Loading** | Integrated with build | Phase 3 (explicit) |
| **Total Time** | ~3-4 min (with idle) | ~3-4 min (no idle) |
| **Resource Usage** | Cluster idles during builds | Cluster created when ready |

---

## Migration Strategy

### Option A: Incremental Migration (RECOMMENDED)
Migrate one service at a time, validate, then proceed:

1. **Gateway** (most critical, 2 setup functions)
2. **DataStorage** (foundation service)
3. **Notification** (simpler)
4. **AuthWebhook** (simpler)

**Benefits**:
- ✅ Lower risk - validate each migration
- ✅ Easier debugging - isolate issues
- ✅ Can pause/resume - no all-or-nothing commitment

**Drawbacks**:
- ❌ Takes more time
- ❌ Mixed patterns during migration

### Option B: Parallel Migration
Migrate all 4 services simultaneously, validate all together:

**Benefits**:
- ✅ Faster completion
- ✅ Single validation phase

**Drawbacks**:
- ❌ Higher risk - harder to isolate issues
- ❌ Harder to debug - multiple changes at once
- ❌ All-or-nothing - can't pause mid-way

---

## Detailed Migration Steps

### Per-Service Migration Checklist

For each service:

#### 1. Backup Current Implementation
```bash
# Already have .orig backups - NOT creating new ones (cleanup policy)
# Relying on git for version control
```

#### 2. Restructure Setup Function

**Before (Standard Pattern)**:
```go
func SetupServiceInfrastructure(...) error {
    // Phase 1: Create cluster + CRDs + namespace
    createKindCluster(...)
    installCRDs(...)
    createNamespace(...)

    // Phase 2: Build images in parallel (cluster IDLES)
    go buildServiceImage(...)
    go buildDataStorageImage(...)

    // Phase 3: Deploy services
    deployPostgreSQL(...)
    deployRedis(...)
    deployDataStorage(...)
}
```

**After (Hybrid Pattern)**:
```go
func SetupServiceInfrastructure(...) error {
    // Phase 0: Generate image tags
    dataStorageImage := GenerateInfraImageName("datastorage", "service")

    // Phase 1: Build images in parallel (NO CLUSTER YET)
    buildResults := make(chan result, 2)
    go buildServiceImage(...)
    go buildDataStorageImageOnly(...) // Just build, don't load yet
    // Wait for builds...

    // Phase 2: Create cluster (images ready)
    createKindCluster(...)
    installCRDs(...)
    createNamespace(...)

    // Phase 3: Load images
    loadServiceImage(...)
    loadDataStorageImage(...)

    // Phase 4: Deploy services
    deployPostgreSQL(...)
    deployRedis(...)
    applyMigrations(...)
    deployDataStorage(...)
}
```

#### 3. Update Image Build/Load Logic

**Current**: `BuildAndLoadImageToKind()` (build + load in one step)

**Hybrid**: Two separate steps
- Phase 1: `buildDataStorageImageWithTag()` - build only, save to tar
- Phase 3: `loadDataStorageImageToKind()` - load tar to cluster

**Note**: This means we **cannot use `BuildAndLoadImageToKind()`** in hybrid pattern!

**Alternative**: Modify `BuildAndLoadImageToKind()` to support deferred loading:
```go
type E2EImageConfig struct {
    // ... existing fields ...
    DeferLoad bool // If true, only build+save tar, return tar path for later loading
}
```

#### 4. Validate E2E Tests

```bash
# Per service
cd test/e2e/<service>
ginkgo -v

# Expected: All tests pass, no regressions
```

#### 5. Update Documentation

- Update DD-TEST-001 to document single hybrid pattern
- Update service-specific docs if any
- Remove references to "standard" vs "hybrid" patterns

---

## Risk Assessment

### Technical Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| **Test regressions** | HIGH | Validate each service after migration |
| **Image loading failures** | MEDIUM | Explicit load step makes failures clearer |
| **Coverage collection breaks** | MEDIUM | Test both coverage and non-coverage modes |
| **Timing issues** | LOW | Hybrid pattern is more reliable (no idle timeouts) |

### Business Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| **CI/CD pipeline disruption** | HIGH | Migrate incrementally, validate each step |
| **Developer workflow impact** | MEDIUM | Clear documentation, single pattern easier |
| **Debugging complexity** | LOW | Single pattern easier to understand |

---

## Implementation Plan

### Phase 1: Gateway Migration (CRITICAL)

**Complexity**: HIGH (2 setup functions + coverage support)

**Files to Modify**:
- `test/infrastructure/gateway_e2e.go`
  - `SetupGatewayInfrastructureParallel()` → Hybrid pattern
  - `SetupGatewayInfrastructureParallelWithCoverage()` → Hybrid pattern

**Validation**:
```bash
cd test/e2e/gateway
ginkgo -v
# Expected: 36/37 passing (Test 24 pre-existing failure)
```

**Decision Point**: Proceed to Phase 2 only if Gateway migration is successful

---

### Phase 2: DataStorage Migration

**Complexity**: MEDIUM (foundation service)

**Files to Modify**:
- `test/infrastructure/datastorage.go`
  - `SetupDataStorageInfrastructureParallel()` → Hybrid pattern

**Validation**:
```bash
cd test/e2e/datastorage
ginkgo -v
# Expected: 84/84 passing
```

**Decision Point**: Proceed to Phase 3 only if DataStorage migration is successful

---

### Phase 3: Notification Migration

**Complexity**: MEDIUM (simpler service)

**Files to Modify**:
- `test/infrastructure/notification_e2e.go`
  - `SetupNotificationAuditInfrastructure()` → Hybrid pattern

**Validation**:
```bash
cd test/e2e/notification
ginkgo -v
# Expected: 21/21 passing
```

**Decision Point**: Proceed to Phase 4 only if Notification migration is successful

---

### Phase 4: AuthWebhook Migration

**Complexity**: MEDIUM (simpler service)

**Files to Modify**:
- `test/infrastructure/authwebhook_e2e.go`
  - `SetupAuthWebhookInfrastructureParallel()` → Hybrid pattern

**Validation**:
```bash
cd test/e2e/authwebhook
ginkgo -v
# Expected: Tests pass (note: pre-existing AuthWebhook pod deployment issue)
```

---

## Critical Issue: `BuildAndLoadImageToKind()` Incompatibility

### Problem

The consolidated `BuildAndLoadImageToKind()` function (Phase 3 achievement) **cannot be used in hybrid pattern** because it loads images immediately after building, but in hybrid pattern we need to:
1. Build images (Phase 1 - before cluster exists)
2. Load images (Phase 3 - after cluster created)

### Solution Options

#### Option 1: Revert to Separate Build/Load Functions (BREAKS PHASE 3)
- Use `buildDataStorageImageWithTag()` and `loadDataStorageImageToKind()`
- **Impact**: Loses Phase 3 consolidation benefits
- **Risk**: HIGH - undoes recent refactoring work

#### Option 2: Extend `BuildAndLoadImageToKind()` with Deferred Loading (RECOMMENDED)
```go
type E2EImageConfig struct {
    // ... existing fields ...
    DeferLoad        bool   // If true, only build+save tar, don't load to Kind yet
    TarOutputPath    string // Where to save tar for deferred loading
}

// Returns: (imageName, tarPath, error) - tarPath only populated if DeferLoad=true
func BuildAndLoadImageToKind(cfg E2EImageConfig, writer io.Writer) (string, string, error)

// New companion function for deferred loading
func LoadImageTarToKind(clusterName, tarPath string, writer io.Writer) error
```

**Benefits**:
- ✅ Maintains Phase 3 consolidation
- ✅ Single function for image building
- ✅ Supports both patterns (standard + hybrid)

**Drawbacks**:
- ❌ More complex API
- ❌ Requires additional testing

#### Option 3: Accept Two Patterns with Different Helpers
- Keep `BuildAndLoadImageToKind()` for standard pattern
- Keep separate `buildXxx/loadXxx` for hybrid pattern
- **Impact**: Two parallel approaches
- **Risk**: MEDIUM - more code to maintain

---

## Recommendation

### Immediate Action: VALIDATE ASSUMPTION

Before proceeding with migration, **validate that hybrid pattern is actually better**:

1. **Measure Standard Pattern**:
   ```bash
   time ginkgo -v test/e2e/gateway/01_signal_ingestion_test.go
   # Record: Total time, cluster creation time, image build time
   ```

2. **Measure Hybrid Pattern**:
   ```bash
   time ginkgo -v test/e2e/remediationorchestrator/
   # Record: Total time, build time, cluster creation time
   ```

3. **Compare**:
   - Is hybrid actually faster?
   - Is idle time actually a problem?
   - Are there measurable benefits?

### If Validation Confirms Benefits:

**Proceed with Option 2 (Extend `BuildAndLoadImageToKind()`)**
- This maintains Phase 3 consolidation
- Supports both patterns during migration
- Can be simplified later if all services use hybrid

### If Validation Shows No Benefit:

**DEFER MIGRATION**
- Keep both patterns as-is
- Document when to use each pattern
- Focus on other higher-value work

---

## Success Criteria

- ✅ All 4 services migrated to hybrid pattern
- ✅ All E2E tests passing (no regressions)
- ✅ Single pattern documented in DD-TEST-001
- ✅ Coverage collection working for all services
- ✅ No increase in test execution time
- ✅ Maintained or improved Phase 3 consolidation

---

## Rollback Plan

If migration causes issues:

1. **Git revert** to previous working state
2. **Restore service-specific** from git history if needed
3. **Document issues** in handoff document
4. **Re-evaluate** pattern choice

---

## Questions for User

1. **Validation First**: Should we measure both patterns to confirm hybrid is actually better?
2. **Incremental vs Parallel**: Option A (one-by-one) or Option B (all at once)?
3. **BuildAndLoadImageToKind**: Option 2 (extend with deferred loading) or Option 3 (accept two patterns)?
4. **Scope**: All 4 services or start with Gateway only?

---

## Next Steps

**AWAITING USER DECISION** on:
1. Should we validate assumption first (measure both patterns)?
2. Which migration strategy (incremental vs parallel)?
3. How to handle `BuildAndLoadImageToKind()` incompatibility?

**DO NOT PROCEED** until user confirms approach.

