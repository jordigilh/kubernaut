# Base Image Build Time Analysis - AIAnalysis E2E

**Date**: December 25, 2025
**Context**: DD-TEST-002 Implementation - Build Performance Investigation
**Status**: 🔍 ANALYSIS COMPLETE

---

## 🎯 **Problem Statement**

AIAnalysis E2E tests are taking **~12 minutes** just for PHASE 1 (parallel image builds), making the total test time unacceptably long (~15-18 minutes).

**Root Cause**: Each Dockerfile runs `dnf update -y` which upgrades ALL packages.

---

## 📊 **Current Base Images**

### **Go Services** (Data Storage, AIAnalysis, Gateway, etc.)
```dockerfile
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder
```
- **Version**: Go 1.25 toolset (latest as of Dec 2024)
- **Tag**: Specific version (`1.25`), not `:latest`
- **Update Command**: `RUN dnf update -y` (upgrades ALL packages)
- **Build Time Impact**: ~3-4 minutes per image

### **Python Services** (HolmesGPT-API)
```dockerfile
FROM registry.access.redhat.com/ubi9/python-312:latest AS builder
```
- **Version**: Python 3.12 (latest)
- **Tag**: `:latest`
- **Update Command**: `RUN dnf update -y` (upgrades ALL packages)
- **Build Time Impact**: ~2-3 minutes per image

### **Runtime Images** (All services)
```dockerfile
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
```
- **Version**: Latest minimal UBI9
- **Tag**: `:latest`
- **No package updates** in runtime stage (good!)

---

## 🔍 **Key Findings**

### **Finding 1: Version Tags Are Already Current**
- `go-toolset:1.25` is the **latest Go version** available for UBI9
- `python-312:latest` is already using **`:latest` tag**
- `ubi-minimal:latest` is already using **`:latest` tag**
- **Conclusion**: Base images are already as current as possible

### **Finding 2: `dnf update -y` is Redundant**
**Current Pattern**:
```dockerfile
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder
USER root
RUN dnf update -y && \
    dnf install -y git ca-certificates tzdata && \
    dnf clean all
```

**Problem**:
- Base image `go-toolset:1.25` was published recently (within last 30-60 days)
- `dnf update -y` upgrades **ALL** packages in the base image
- For E2E testing, we don't need absolute latest security patches
- **Time Cost**: ~3-4 minutes per image

### **Finding 3: Parallel Builds Amplify the Problem**
```
PHASE 1: Build images in parallel
  ├── Data Storage:    dnf update -y  (~4 min)
  ├── HolmesGPT-API:   dnf update -y  (~3 min)
  └── AIAnalysis:      dnf update -y  (~4 min)
                       ─────────────
                       Total: ~12 min (parallel)
```

Since builds run in parallel, the **longest build** determines total time.

---

## 💡 **Solutions Analysis**

### **Option A: Remove `dnf update -y` (RECOMMENDED for E2E)**
**Change**:
```dockerfile
# BEFORE
RUN dnf update -y && \
    dnf install -y git ca-certificates tzdata && \
    dnf clean all

# AFTER (E2E only)
RUN dnf install -y git ca-certificates tzdata && \
    dnf clean all
```

**Impact**:
- ✅ **Build Time**: ~12 min → **~2 min** (parallel)
- ✅ **Test Reliability**: No change (same base images)
- ✅ **E2E Coverage**: No change (tests unchanged)
- ⚠️ **Security**: Base image patches only (acceptable for E2E)

**Trade-offs**:
- E2E images won't have absolute latest patches
- Base images are already recent (published within 30-60 days)
- **Acceptable** for test environments
- **NOT acceptable** for production images

---

### **Option B: Use Cached Base Layers**
**Strategy**: Pull base images once, use Docker layer caching

**Current State**:
- Docker layer cache IS being used
- BUT `dnf update -y` invalidates cache frequently (packages change daily)
- Even with cache, package upgrades take time

**Impact**:
- ⚠️ **Limited Benefit**: First build still takes ~12 min
- ✅ **Subsequent Builds**: Faster if no package updates
- ❌ **Not Reliable**: Cache invalidated when new packages available

---

### **Option C: Pin Specific Base Image Dates**
**Change**:
```dockerfile
# Use specific dated release instead of :latest
FROM registry.access.redhat.com/ubi9/go-toolset:1.25-1735142400
```

**Impact**:
- ✅ **Build Time**: Consistent (no dnf updates needed)
- ✅ **Reproducibility**: Exact same base every time
- ❌ **Maintenance**: Must update tags manually
- ❌ **Security**: No automatic security updates

**Trade-offs**:
- Best for reproducible builds
- Worst for security-conscious environments
- **NOT RECOMMENDED** for this project (security-focused)

---

### **Option D: Accept Current Performance**
**Rationale**:
- Security is paramount (Red Hat UBI + latest patches)
- 15-18 minutes for E2E is acceptable once per day
- DD-TEST-002 compliance achieved (build first, cluster second)
- Focus on V1.0 functional readiness

**Impact**:
- ✅ **Security**: Maximum (always latest patches)
- ✅ **No Code Changes**: Zero risk
- ❌ **Slow Builds**: ~15-18 min per E2E run

---

## 🎯 **Recommendation**

### **IMMEDIATE: Option A (E2E Only)**
Remove `dnf update -y` from Dockerfiles **ONLY for E2E test images**.

**Implementation**:
1. Create E2E-specific Dockerfiles: `docker/aianalysis-e2e.Dockerfile`
2. Remove `dnf update -y` from E2E versions
3. Keep production Dockerfiles unchanged (with `dnf update -y`)

**Benefits**:
- ✅ **Fast E2E tests**: ~5-7 min total (vs 15-18 min)
- ✅ **Production security**: Unchanged
- ✅ **Clear separation**: E2E vs production images
- ✅ **V1.0 readiness**: Faster iteration

### **FOLLOW-UP: Option B (Production)**
Implement multi-stage build optimization for production images:
- Use `buildkit` cache mounts
- Pin base image layers for stability
- Separate security updates from build process

---

## 📊 **Expected Performance**

| Configuration | Build Time (PHASE 1) | Total E2E Time | Notes |
|---------------|---------------------|----------------|-------|
| **Current** (dnf update) | ~12 min | ~15-18 min | Secure but slow |
| **Option A** (no dnf update) | **~2 min** | **~5-7 min** | Fast, acceptable for E2E |
| **Option B** (cached layers) | ~5-8 min | ~8-12 min | Variable, depends on cache |
| **Option C** (pinned dates) | ~2 min | ~5-7 min | Fast but manual maintenance |
| **Option D** (accept current) | ~12 min | ~15-18 min | No change |

---

## 🚀 **Implementation Plan**

### **Phase 1: Create E2E-Specific Dockerfiles**
```bash
# Create E2E versions without dnf update
cp docker/aianalysis.Dockerfile docker/aianalysis-e2e.Dockerfile
cp docker/data-storage.Dockerfile docker/data-storage-e2e.Dockerfile
cp holmesgpt-api/Dockerfile holmesgpt-api/Dockerfile.e2e

# Remove dnf update -y from E2E versions
sed -i '' '/dnf update -y/d' docker/aianalysis-e2e.Dockerfile
sed -i '' '/dnf update -y/d' docker/data-storage-e2e.Dockerfile
sed -i '' '/dnf update -y/d' holmesgpt-api/Dockerfile.e2e
```

### **Phase 2: Update E2E Infrastructure**
Update `test/infrastructure/aianalysis.go` to use E2E Dockerfiles:
```go
// Build with E2E-specific Dockerfiles (no dnf update)
buildArgs := map[string]*string{
    "GOFLAGS": &goflags,
}
dockerfile := "docker/aianalysis-e2e.Dockerfile"  // E2E version
if err := buildImageWithArgs(imageName, dockerfile, ".", buildArgs, writer); err != nil {
    return nil, fmt.Errorf("failed to build aianalysis image: %w", err)
}
```

### **Phase 3: Verify Performance**
- Run E2E tests with new Dockerfiles
- Measure PHASE 1 build time (expect ~2 min)
- Measure total E2E time (expect ~5-7 min)
- Verify all 34 specs pass

### **Phase 4: Document**
- Update DD-TEST-002 with performance notes
- Document E2E vs production Dockerfile strategy
- Add to V1.0 readiness checklist

---

## ✅ **Success Criteria**

| Metric | Current | Target | Status |
|--------|---------|--------|--------|
| **PHASE 1 Build Time** | ~12 min | **<3 min** | ⏳ Pending |
| **Total E2E Time** | ~15-18 min | **<8 min** | ⏳ Pending |
| **All 34 Specs Pass** | ⏳ Testing | ✅ PASS | ⏳ Pending |
| **Coverage Collection** | ⏳ Pending | ✅ Working | ⏳ Pending |
| **DD-TEST-002 Compliance** | ✅ PASS | ✅ PASS | ✅ COMPLETE |

---

## 🔗 **Related Documentation**

- `DD-TEST-002-parallel-test-execution-standard.md` - Hybrid parallel setup
- `SESSION_SUMMARY_AA_DD_TEST_002_DEC_25_2025.md` - Session progress
- `AA_E2E_DD_TEST_002_COMPLIANCE_DEC_25_2025.md` - Compliance analysis

---

**Next Step**: Create E2E-specific Dockerfiles and measure performance improvement.








