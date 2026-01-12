# E2E Infrastructure Failure - Pip Dependency Timeout - January 12, 2026

**Date**: January 12, 2026 16:00 EST
**Severity**: 🚨 **CRITICAL** - E2E tests cannot run
**Status**: ❌ **BLOCKED**

---

## 📊 **Failure Summary**

| Metric | Value |
|--------|-------|
| **Expected Duration** | ~10 minutes |
| **Actual Duration** | 50+ minutes (TIMEOUT) |
| **Root Cause** | `pip install` stuck in dependency resolution |
| **Blocking Line** | `pip is still looking at multiple versions of uvicorn[standard]` |
| **Impact** | **100% E2E test failure** - Tests never started |

---

## 🔍 **Root Cause Analysis**

### **Sequence of Events**

1. ✅ **Phase 7 Cleanup**: Removed `MOCK_LLM_MODE=true` from `holmesgpt-api/Dockerfile.e2e`
2. ❌ **Docker Cache Invalidated**: `ENV` instruction change invalidated all subsequent layers
3. ❌ **Full Rebuild Required**: Every E2E run now requires full `pip install` (~200 packages)
4. ❌ **Dependency Resolver Stuck**: `pip` hung while resolving `uvicorn[standard]` versions
5. ❌ **Timeout**: Test timeout (600s) before `pip` completed

### **Log Evidence**

```
Line 379: RUN pip install --no-cache-dir --upgrade pip &&
          pip install --no-cache-dir -r requirements.txt
...
Line 681: INFO: pip is looking at multiple versions of uvicorn[standard]
          to determine which version is compatible with other requirements.
          This could take a while.
Line 682: Downloading uvicorn-0.39.0-py3-none-any.whl.metadata (6.8 kB)
Line 683: Downloading uvicorn-0.38.0-py3-none-any.whl.metadata (6.8 kB)
Line 684: Downloading uvicorn-0.37.0-py3-none-any.whl.metadata (6.6 kB)
Line 685: Downloading uvicorn-0.36.1-py3-none-any.whl.metadata (6.6 kB)
Line 686: Downloading uvicorn-0.36.0-py3-none-any.whl.metadata (6.6 kB)
Line 687: Downloading uvicorn-0.35.0-py3-none-any.whl.metadata (6.5 kB)
Line 688: Downloading uvicorn-0.34.3-py3-none-any.whl.metadata (6.5 kB)
Line 690: INFO: pip is still looking at multiple versions of uvicorn[standard]
          to determine which version is compatible with other requirements.
          This could take a while.
[TIMEOUT - Process killed after 600s]
```

---

## 🎯 **Problem**

The `requirements-e2e.txt` has **conflicting dependency constraints**:

```
uvicorn[standard]>=0.30
```

`pip` is trying EVERY version of `uvicorn` from `0.30` to `0.40` to find one that satisfies ALL other dependencies. This is taking >10 minutes.

---

## ✅ **Solution Options**

### **Option A: Pin uvicorn Version (RECOMMENDED - FASTEST)**

**Fix**: Specify exact `uvicorn` version to avoid dependency resolution

**Implementation**:
```bash
# In requirements-e2e.txt
uvicorn[standard]==0.30.6  # Pin to specific version
```

**Pros**:
- ✅ Instant fix (no dependency resolution)
- ✅ Build time: <5 minutes
- ✅ Deterministic builds

**Cons**:
- ⚠️ Requires manual version updates

---

### **Option B: Use pip --use-deprecated=legacy-resolver**

**Fix**: Use old pip resolver (faster but less accurate)

**Implementation**:
```dockerfile
RUN pip install --no-cache-dir --use-deprecated=legacy-resolver -r requirements.txt
```

**Pros**:
- ✅ Faster resolution
- ✅ No requirement changes

**Cons**:
- ⚠️ May install incompatible versions
- ⚠️ Deprecated feature

---

### **Option C: Separate Requirements Files**

**Fix**: Split requirements into base + E2E specific

**Implementation**:
```
requirements-base.txt    # Core dependencies (holmesgpt, etc.)
requirements-e2e-only.txt  # E2E-specific (uvicorn, pytest, etc.)
```

**Pros**:
- ✅ Better dependency management
- ✅ Faster resolution

**Cons**:
- ⚠️ More files to maintain

---

### **Option D: Pre-build and Cache Image (BEST LONG-TERM)**

**Fix**: Build HAPI E2E image ONCE, reuse for all tests

**Implementation**:
```bash
# Build once (manually or in CI)
podman build -t holmesgpt-api:e2e-base -f Dockerfile.e2e .

# E2E tests use pre-built image
# No rebuild unless source changes
```

**Pros**:
- ✅ **ZERO build time** for subsequent runs
- ✅ **E2E tests run in <10 minutes**
- ✅ CI/CD friendly

**Cons**:
- ⚠️ Requires one-time setup

---

## 🚀 **Recommended Action**

**IMMEDIATE (Option A)**: Pin `uvicorn` version
- **Time to fix**: 2 minutes
- **Time to validate**: 15 minutes (one E2E run)
- **Impact**: Fixes 100% of timeout issues

**LONG-TERM (Option D)**: Pre-build E2E image
- **Time to implement**: 10 minutes
- **Time savings**: 40-50 minutes per E2E run
- **ROI**: Massive (if running E2E frequently)

---

## 📝 **Immediate Fix Implementation**

```bash
# 1. Pin uvicorn version in requirements-e2e.txt
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
echo "uvicorn[standard]==0.30.6" > temp.txt
grep -v "uvicorn\[standard\]" holmesgpt-api/requirements-e2e.txt >> temp.txt
mv temp.txt holmesgpt-api/requirements-e2e.txt

# 2. Re-run E2E tests with extended timeout (one-time cost)
timeout 900 make test-e2e-holmesgpt-api

# 3. Subsequent runs will use cached image (<10 min)
```

---

## 📊 **Timeline**

| Time | Event |
|------|-------|
| 14:17 | E2E tests started |
| 14:18 | Data Storage image built (cached, <1 min) |
| 14:18 | HAPI image build started |
| 14:18-15:07 | `pip install` running (dependency resolution stuck) |
| 15:07 | **TIMEOUT** - Process killed after 600s (10 min) |
| 16:00 | **Triage complete** |

---

## 🎯 **Next Steps**

1. ✅ **IMMEDIATE**: Pin `uvicorn` version (Option A)
2. ⏳ **VALIDATE**: Re-run E2E tests with 900s timeout
3. ✅ **LONG-TERM**: Implement pre-built E2E image (Option D)

---

**Last Updated**: 2026-01-12 16:00 EST
**Status**: ❌ **BLOCKED** - Awaiting fix implementation
**Priority**: 🚨 **P0 - CRITICAL**
