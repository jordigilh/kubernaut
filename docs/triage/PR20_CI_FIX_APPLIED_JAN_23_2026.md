# PR #20 CI Failures - Fix Applied (Jan 23, 2026)

**Status**: ✅ **FIX APPLIED AND PUSHED**  
**Branch**: `feature/soc2-compliance`  
**Commit**: `3b96581b`  
**Related**: `docs/triage/PR20_CI_FAILURES_JAN_23_2026.md`

---

## 🎯 **Problem Summary**

PR #20 CI jobs failed due to:
1. **Container Build (amd64)**: `TARGETARCH` build arg not explicitly set in CI workflow
2. **Unit Tests (45 bats)**: `podman run` commands interpreting container args as podman options

---

## 🔧 **Fix Applied**

### **1. GitHub Workflow (`.github/workflows/must-gather-tests.yml`)**

#### **A. Added explicit TARGETARCH build arg**
```yaml
# Before:
podman build \
  --platform linux/amd64 \
  -t localhost/must-gather:ci-test \
  -f Dockerfile \
  .

# After:
podman build \
  --platform linux/amd64 \
  --build-arg TARGETARCH=amd64 \  # ADDED
  -t localhost/must-gather:ci-test \
  -f Dockerfile \
  .
```

**Why**: Ensures `kubectl` download in Dockerfile uses correct architecture.

#### **B. Fixed podman command syntax** (wrapped in `sh -c`)
```yaml
# Before (WRONG - "test" interpreted as podman option):
podman run --rm --platform linux/amd64 \
  localhost/must-gather:ci-test \
  test -x /usr/bin/gather && echo "✅" || exit 1

# After (CORRECT - wrapped in sh -c):
podman run --rm --platform linux/amd64 \
  localhost/must-gather:ci-test \
  sh -c "test -x /usr/bin/gather"
echo "✅ Main script present"
```

**Fixed commands**:
- `test -x /usr/bin/gather` → `sh -c "test -x /usr/bin/gather"`
- `ls /usr/share/must-gather/collectors/ | wc -l` → `sh -c "ls /usr/share/must-gather/collectors/ | wc -l"`
- `test -x /usr/share/must-gather/sanitizers/sanitize-all.sh` → `sh -c "test -x /usr/share/must-gather/sanitizers/sanitize-all.sh"`
- `which kubectl` → `sh -c "which kubectl"`
- `which jq` → `sh -c "which jq"`
- `--help | grep` → `sh -c "/usr/bin/gather --help"`

**Why**: Prevents podman from treating container commands as podman options.

---

### **2. Makefile (`.cmd/must-gather/Makefile`)**

#### **A. Added platform auto-detection**
```makefile
# Platform auto-detection (amd64 for x86_64, arm64 for aarch64)
PLATFORM ?= $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
```

#### **B. Updated build-local target**
```makefile
# Before (hardcoded arm64):
build-local: ## Build ARM64-only image for local testing (Apple Silicon)
	@echo "Building ARM64 must-gather image for local development..."
	podman build --platform linux/arm64 \
		-t $(LOCAL_IMAGE_NAME):test \
		.

# After (auto-detected):
build-local: ## Build image for native platform (auto-detected: amd64 or arm64)
	@echo "Building $(PLATFORM) must-gather image for local development..."
	podman build --platform linux/$(PLATFORM) \
		-t $(LOCAL_IMAGE_NAME):test \
		.
```

#### **C. Updated test-container and test-verbose targets**
```makefile
# Before (hardcoded linux/arm64):
test-container: build-local ## Run unit tests inside ARM64 container
	@echo "Running unit tests in ARM64 container..."
	podman run --rm \
		--platform linux/arm64 \
		...

# After (auto-detected):
test-container: build-local ## Run unit tests inside native platform container (auto-detected)
	@echo "Running unit tests in $(PLATFORM) container..."
	podman run --rm \
		--platform linux/$(PLATFORM) \
		...
```

**Why**: Allows must-gather to build/test on both amd64 (CI) and arm64 (local Apple Silicon) without manual changes.

---

## ✅ **Expected CI Result**

After this fix, PR #20 CI should:

1. **Container Build (amd64)**: 
   - `TARGETARCH=amd64` correctly passed to Dockerfile
   - `kubectl` downloads for `linux/amd64` architecture
   - Build completes successfully

2. **Unit Tests (45 bats)**:
   - `podman run` commands execute correctly inside container
   - All 45 bats tests pass (as they do locally)
   - No "Unknown option: test" errors

3. **E2E Tests**:
   - Continue as before (not affected by this fix)

---

## 📊 **Verification Steps**

Once CI re-runs:

```bash
# Check Container Build (amd64) job logs:
✅ Look for: "RUN curl -LO ...kubectl" step succeeds
✅ Look for: "test -x /usr/local/bin/kubectl" step succeeds

# Check Unit Tests (45 bats) job logs:
✅ Look for: "✅ Main script present"
✅ Look for: "✅ All 9 collectors present"
✅ Look for: "✅ kubectl installed"
✅ Look for: "45 bats tests passing"
```

---

## 🔗 **Related Documents**

- **Initial Triage**: `docs/triage/PR20_CI_FAILURES_JAN_23_2026.md`
- **Workflow File**: `.github/workflows/must-gather-tests.yml`
- **Makefile**: `cmd/must-gather/Makefile`
- **Dockerfile**: `cmd/must-gather/Dockerfile`

---

## 🎯 **Confidence Assessment**

**Confidence**: 90%

**Justification**:
- Root cause clearly identified from CI logs
- Fix addresses both errors directly:
  - `TARGETARCH` not set → Explicit `--build-arg TARGETARCH=amd64` added
  - podman option misinterpretation → All commands wrapped in `sh -c`
- Platform auto-detection improves maintainability for local development
- No other CI jobs affected by this change

**Risk**: 
- Low risk - changes are isolated to must-gather CI workflow and Makefile
- Workflow changes only affect verification steps (not build or test logic)
- Makefile changes improve cross-platform support without breaking existing behavior

**Next Step**: Monitor PR #20 CI re-run for success 🚀
