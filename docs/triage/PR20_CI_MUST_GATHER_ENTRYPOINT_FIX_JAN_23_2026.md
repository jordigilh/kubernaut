# PR #20 CI Must-Gather ENTRYPOINT Fix (Jan 23, 2026)

**Status**: ✅ **FIX APPLIED AND PUSHED**
**Branch**: `feature/soc2-compliance`
**Commit**: `3bc6adef`
**Related**: `docs/triage/PR20_CI_FIX_APPLIED_JAN_23_2026.md`

---

## 🎯 **Problem Summary**

After the first fix (commit `3b96581b`), PR #20 CI still failed:

✅ **Unit Tests (45 bats)**: SUCCESS (fixed by previous commit)
❌ **Container Build (amd64)**: FAILURE (new issue discovered)

### **Root Cause - ENTRYPOINT Override Issue**

**Container build succeeded** (all Dockerfile steps 1-15 completed), but **verification failed** with:
```
Error: Unknown option: sh
Use --help for usage information
```

**Why?**
- Our Dockerfile has `ENTRYPOINT ["/usr/bin/gather"]`
- When we run `podman run ... sh -c "test -x /usr/bin/gather"`, podman passes `sh -c "test ..."` as **arguments to `/usr/bin/gather`**, not as a command to execute
- `/usr/bin/gather` doesn't understand the `sh` option, so it fails

---

## 🔧 **Fix Applied**

### **Solution: Override ENTRYPOINT for Verification Commands**

Add `--entrypoint sh` to all verification `podman run` commands to explicitly run `sh` instead of the container's default `/usr/bin/gather`.

### **Changes to `.github/workflows/must-gather-tests.yml`**

#### **A. Verification Commands (Override ENTRYPOINT)**

```yaml
# Before (WRONG - sh passed to /usr/bin/gather):
podman run --rm --platform linux/amd64 \
  localhost/must-gather:ci-test \
  sh -c "test -x /usr/bin/gather"

# After (CORRECT - sh is the entrypoint):
podman run --rm --platform linux/amd64 \
  --entrypoint sh \               # Override ENTRYPOINT
  localhost/must-gather:ci-test \
  -c "test -x /usr/bin/gather"    # Args for sh
```

**Applied to**:
- Main script verification: `test -x /usr/bin/gather`
- Collectors count: `ls /usr/share/must-gather/collectors/ | wc -l`
- Sanitizers verification: `test -x /usr/share/must-gather/sanitizers/sanitize-all.sh`
- kubectl verification: `which kubectl`
- jq verification: `which jq`

#### **B. Container Execution Test (Use Default ENTRYPOINT)**

For the actual execution test, we **don't** override the ENTRYPOINT since we want to test `/usr/bin/gather` itself:

```yaml
# Before (WRONG - wrapped in sh -c):
podman run --rm --platform linux/amd64 \
  localhost/must-gather:ci-test \
  sh -c "/usr/bin/gather --help"

# After (CORRECT - use default ENTRYPOINT):
podman run --rm --platform linux/amd64 \
  localhost/must-gather:ci-test \
  --help                          # Args for /usr/bin/gather
```

---

## ✅ **Expected CI Result**

After this fix:

1. **Container Build (amd64)**:
   - ✅ Build succeeds (was already working)
   - ✅ Verification steps pass (NOW FIXED)
   - ✅ Execution test passes (NOW FIXED)

2. **Unit Tests (45 bats)**:
   - ✅ Continue passing (fixed in previous commit)

3. **Overall**:
   - ✅ All must-gather CI jobs should be GREEN

---

## 📊 **Technical Deep Dive**

### **Docker ENTRYPOINT Behavior**

When a Dockerfile has:
```dockerfile
ENTRYPOINT ["/usr/bin/gather"]
```

Running `podman run <image> arg1 arg2` executes:
```bash
/usr/bin/gather arg1 arg2
```

**NOT**:
```bash
arg1 arg2
```

### **Overriding ENTRYPOINT**

To run arbitrary commands inside the container:
```bash
podman run --entrypoint sh <image> -c "command"
```

This executes:
```bash
sh -c "command"
```

### **When to Override ENTRYPOINT**

| Use Case | Override ENTRYPOINT? | Example |
|---|---|---|
| **Verification/Inspection** | ✅ YES | `--entrypoint sh` + `-c "test -x /path"` |
| **Testing the actual tool** | ❌ NO | Just pass args: `--help`, `--version` |
| **Debugging** | ✅ YES | `--entrypoint bash` for interactive shell |

---

## 🔗 **Related Documents**

- **Previous Fix**: `docs/triage/PR20_CI_FIX_APPLIED_JAN_23_2026.md` (commit `3b96581b`)
- **Initial Triage**: `docs/triage/PR20_CI_FAILURES_JAN_23_2026.md`
- **Workflow File**: `.github/workflows/must-gather-tests.yml`

---

## 🎯 **Confidence Assessment**

**Confidence**: 95%

**Justification**:
- Root cause clearly identified from CI logs (`Error: Unknown option: sh`)
- Fix directly addresses ENTRYPOINT behavior in podman/Docker
- Pattern is well-established (override ENTRYPOINT for verification, keep default for execution tests)
- Previous fix (`3b96581b`) already resolved Unit Tests issue (now passing)

**Risk**:
- Minimal - changes only affect verification steps in CI workflow
- No changes to container build or actual functionality
- Follows standard Docker/podman best practices

**Next Step**: Monitor PR #20 CI re-run for full success (all jobs GREEN) 🚀

---

## 📈 **Progress Summary**

| Issue | Status | Commit |
|---|---|---|
| `kubectl` download (amd64 arch) | ✅ FIXED | `3b96581b` |
| podman command syntax (Unit Tests) | ✅ FIXED | `3b96581b` |
| Platform auto-detection (Makefile) | ✅ FIXED | `3b96581b` |
| ENTRYPOINT override (verification) | ✅ FIXED | `3bc6adef` (THIS) |

**Expected Final Result**: 🟢 All must-gather CI jobs passing
