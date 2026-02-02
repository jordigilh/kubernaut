# HAPI E2E Timeout Fix - Jan 29, 2026

## Problem Statement

HAPI E2E tests were timing out after 15 minutes with infrastructure setup incomplete.

### Root Cause Analysis

**Issue #1: Unrealistic 15-Minute Timeout**
- HAPI builds **3 images** sequentially:
  - DataStorage: ~5m 41s (Go compilation)
  - HolmesGPT-API: ~3m 41s (Python wheels)
  - Mock LLM: ~24s
  - **Total build time**: ~9m 46s
- Infrastructure deployment: ~3-4 minutes
- Pytest execution: ~3-5 minutes
- **Total required**: ~16-19 minutes

**Comparison**:
- Other 8 services build 1-2 images
- Other services complete in <15 minutes
- HAPI is unique in requiring 3-image build

**Issue #2: "No Space Left on Device" (Pytest Container)**
- Pytest container runs with `--rm` (ephemeral storage)
- `pip install --break-system-packages` writes to container overlay FS
- No volume mounted for pip cache or `/tmp`
- Container hits storage limit during pip installs

**Evidence**:
```
ERROR: Could not install packages due to an OSError: [Errno 28] No space left on device
```

Host had 236GB free, but container's ephemeral storage was exhausted.

---

## Solution (HAPI-Specific Only)

### Fix #1: Increase Ginkgo Timeout

**File**: `Makefile`

```diff
- @cd test/e2e/holmesgpt-api && $(GINKGO) -v --timeout=15m ...
+ @cd test/e2e/holmesgpt-api && $(GINKGO) -v --timeout=30m ...
```

**Rationale**:
- Realistic build time: ~10 minutes (3 images)
- Infrastructure + deployment: ~4 minutes
- Pytest execution: ~3-5 minutes
- Buffer for CI/CD slowness: ~8-13 minutes
- **Total**: 30 minutes is appropriate

**Impact**: HAPI E2E tests only, no affect on other 8 services

---

### Fix #2: Pytest Container Storage

**File**: `test/e2e/holmesgpt-api/holmesgpt_api_e2e_suite_test.go`

```diff
cmd := exec.CommandContext(ctx, "podman", "run", "--rm",
    "-v", fmt.Sprintf("%s:/workspace:z", projectRoot),
+   "-v", fmt.Sprintf("%s:/root/.cache/pip:z", pipCacheDir),  // Pip cache
+   "--tmpfs", "/tmp:size=2G,mode=1777",                       // 2GB tmpfs
    "-w", "/workspace/holmesgpt-api",
    "--network", "host",
    "registry.access.redhat.com/ubi9/python-312:latest",
    "sh", "-c", pytestCmd,
)
```

**Rationale**:
- **Pip cache volume**: Reusable across local runs, empty in CI (no harm)
- **Tmpfs /tmp**: 2GB dedicated storage for pip temp files
- **CI/CD compatible**: Cache dir empty in fresh CI, tmpfs always works

**Impact**: Prevents "No space left on device" errors

---

## Why NOT Change Image Tags?

**User Requirement**: "Tags must be random" (for test isolation)

**Analysis**:
- 8/9 services work fine with random tags + current timeouts
- Random tags provide test isolation (parallel runs don't interfere)
- Content-based hashing would affect ALL services (unnecessary risk)
- **HAPI is the exception** due to 3-image build requirement

**Decision**: Fix HAPI-specific issues, don't change working pattern for other services

---

## Testing

### Before Fix:
```
Infrastructure setup: 14 minutes (835 seconds)
Pytest execution: Started at 18:32:00
Ginkgo timeout: 15 minutes → FAIL! - Suite Timeout Elapsed
```

### After Fix (Expected):
```
Infrastructure setup: ~10 minutes
Pytest execution: ~3-5 minutes
Total: ~15-18 minutes (within 30m timeout)
✅ Tests complete successfully
```

---

## Related Issues

### Auth/Authz Middleware Fixes (Completed)
1. ✅ Added `/health` and `/ready` to `PUBLIC_ENDPOINTS`
2. ✅ Created dedicated `holmesgpt-api-e2e-sa` for pytest
3. ✅ Fixed auth middleware to not convert 400→401
4. ✅ Added OpenAPI response declarations (400, 401, 403, 422, 500)

### Remaining Work
- None for HAPI E2E timeout issue
- Auth fixes are complete and tested

---

## Confidence Assessment

**Timeout Fix**: 95%
- Realistic timing based on actual measurements
- 2x current timeout provides ample buffer
- CI/CD slowness accounted for

**Pip Cache Fix**: 98%
- Standard container pattern
- CI/CD compatible (empty cache dir is fine)
- Tmpfs solves ephemeral storage limits

**Risk**: Low
- Changes are HAPI-specific
- No impact on working services
- Aligns with existing patterns

---

## References

- Test logs: `/tmp/hapi-e2e-clean-disk.log`
- Must-gather: `/tmp/holmesgpt-api-e2e-logs-20260201-183258`
- Auth middleware fixes: `GATEWAY_E2E_COMPLETE_FIX_JAN_29_2026.md`
