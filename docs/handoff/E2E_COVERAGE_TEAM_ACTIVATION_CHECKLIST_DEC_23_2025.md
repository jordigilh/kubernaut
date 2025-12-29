# E2E Coverage Activation - Team Checklist

**Date**: December 23, 2025
**Context**: DD-TEST-008 Reusable E2E Coverage Infrastructure
**Audience**: Service teams (DataStorage, Gateway, WorkflowExecution, SignalProcessing, etc.)

---

## 🎯 Quick Answer

**If your service already has E2E tests**, you need to do **3 things** to activate coverage:

1. ✅ **Check Dockerfile** (5 min) - Ensure it supports `GOFLAGS=-cover`
2. ✅ **Check Infrastructure** (5 min) - Ensure Kind cluster has `/coverdata` mount
3. ✅ **Add Makefile Target** (30 seconds) - Add **1 line** to use reusable infrastructure

**Total Time**: ~10-15 minutes per service

---

## 📋 Detailed Checklist

### ✅ **Step 1: Verify Dockerfile Supports Coverage** (5 min)

**Check**: Does your service's Dockerfile have conditional coverage support?

**Location**: `docker/{service}.Dockerfile`

**Required Pattern**:
```dockerfile
# Must accept GOFLAGS as build arg
ARG GOFLAGS=""

# Must use conditional build based on GOFLAGS
RUN if [ "${GOFLAGS}" = "-cover" ]; then \
        # Coverage build: Simple go build with GOFLAGS
        CGO_ENABLED=0 GOOS=linux GOFLAGS="${GOFLAGS}" go build \
            -o {service}-controller ./cmd/{service}; \
    else \
        # Production build: Optimized with symbol stripping
        CGO_ENABLED=0 GOOS=linux go build \
            -ldflags="-s -w" \
            -o {service}-controller ./cmd/{service}; \
    fi
```

**Critical Rules**:
- ⚠️ **Coverage build MUST use simple `go build`** (no `-a`, `-installsuffix`, `-extldflags`)
- ⚠️ **Production build keeps all optimizations** (`-ldflags="-s -w"`)
- ⚠️ **Binary name must be consistent** between coverage and production builds

**Status Check**:
```bash
# Search your Dockerfile for GOFLAGS
grep -A 10 "ARG GOFLAGS" docker/{service}.Dockerfile
```

**Action If Missing**:
- Update Dockerfile following the pattern above
- Test both coverage and production builds:
  ```bash
  # Coverage build
  podman build --build-arg GOFLAGS=-cover -t test:coverage -f docker/{service}.Dockerfile .

  # Production build
  podman build -t test:production -f docker/{service}.Dockerfile .
  ```

---

### ✅ **Step 2: Verify Kind Cluster Has Coverage Support** (5 min)

**Check**: Does your Kind cluster config have `/coverdata` extraMounts?

**Location**: `test/infrastructure/kind-{service}-config.yaml`

**Required Pattern**:
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
  - hostPath: /tmp/{service}-e2e-coverage  # Or similar
    containerPath: /coverdata              # MUST be /coverdata
    readOnly: false
- role: worker
  extraMounts:
  - hostPath: /tmp/{service}-e2e-coverage
    containerPath: /coverdata
    readOnly: false
```

**Critical Rules**:
- ⚠️ **`containerPath` MUST be `/coverdata`** (hardcoded in DD-TEST-007)
- ⚠️ **Must be on ALL nodes** (control-plane + workers)
- ⚠️ **`readOnly: false`** (coverage needs write access)

**Status Check**:
```bash
# Search your Kind config for coverdata
grep -A 3 "extraMounts" test/infrastructure/kind-{service}-config.yaml | grep coverdata
```

**Action If Missing**:
- Add extraMounts to your Kind cluster config
- Ensure the hostPath directory is created before cluster creation
- Apply to both control-plane and worker nodes

---

### ✅ **Step 3: Verify E2E Deployment Supports Coverage** (5 min)

**Check**: Does your E2E deployment add `GOCOVERDIR` when `E2E_COVERAGE=true`?

**Location**: `test/infrastructure/{service}.go` (or `test/e2e/{service}/manifests/`)

**Required Pattern**:
```go
// In your deployment creation code
envVars := []corev1.EnvVar{
    {Name: "CONFIG_PATH", Value: "/etc/{service}/config.yaml"},
}

// DD-TEST-007: Add GOCOVERDIR if E2E_COVERAGE=true
if os.Getenv("E2E_COVERAGE") == "true" {
    envVars = append(envVars, corev1.EnvVar{
        Name:  "GOCOVERDIR",
        Value: "/coverdata",  // MUST match Kind extraMounts containerPath
    })
}

// ... and add volume mount:
volumeMounts := []corev1.VolumeMount{
    {Name: "config", MountPath: "/etc/{service}"},
}

if os.Getenv("E2E_COVERAGE") == "true" {
    volumeMounts = append(volumeMounts, corev1.VolumeMount{
        Name:      "coverage",
        MountPath: "/coverdata",
    })
}

// ... and add hostPath volume:
volumes := []corev1.Volume{
    {Name: "config", VolumeSource: corev1.VolumeSource{...}},
}

if os.Getenv("E2E_COVERAGE") == "true" {
    volumes = append(volumes, corev1.Volume{
        Name: "coverage",
        VolumeSource: corev1.VolumeSource{
            HostPath: &corev1.HostPathVolumeSource{
                Path: "/coverdata",  // MUST match Kind extraMounts
                Type: &[]corev1.HostPathType{corev1.HostPathDirectoryOrCreate}[0],
            },
        },
    })
}
```

**Critical Rules**:
- ⚠️ **`GOCOVERDIR=/coverdata`** (must match Kind mount)
- ⚠️ **Only add when `E2E_COVERAGE=true`** (production builds don't need it)
- ⚠️ **HostPath must match Kind config** (`/coverdata`)

**Status Check**:
```bash
# Search for GOCOVERDIR in your E2E infrastructure
grep -r "GOCOVERDIR" test/infrastructure/{service}.go test/e2e/{service}/
```

**Action If Missing**:
- Add conditional `GOCOVERDIR` environment variable
- Add conditional volume mount
- Add conditional hostPath volume
- Follow the pattern in `test/infrastructure/datastorage.go` (lines 860-975)

---

### ✅ **Step 4: Add Reusable Coverage Target** (30 seconds)

**Check**: Is the reusable infrastructure included in the main Makefile?

**Location**: `Makefile` (top of file)

**Required**:
```makefile
# Include reusable E2E coverage infrastructure (DD-TEST-008)
include Makefile.e2e-coverage.mk
```

**Status Check**:
```bash
# Check if included
grep "Makefile.e2e-coverage.mk" Makefile
```

**Action If Missing**:
- Add the `include` statement at the top of the main Makefile (after `export GOFLAGS`)

---

**Then add your service's coverage target**:

**Location**: `Makefile` (near your existing `test-e2e-{service}` target)

**Required** (just **1 line**):
```makefile
# E2E Coverage Target (DD-TEST-008)
$(eval $(call define-e2e-coverage-target,{service},{module_path},{procs}))
```

**Examples**:
```makefile
# For Notification Service
$(eval $(call define-e2e-coverage-target,notification,notification,4))

# For DataStorage Service
$(eval $(call define-e2e-coverage-target,datastorage,datastorage,4))

# For Gateway Service
$(eval $(call define-e2e-coverage-target,gateway,gateway,4))
```

**Parameters**:
1. `{service}` - Service name (lowercase, matches directory names)
2. `{module_path}` - Go module path (usually same as service name)
3. `{procs}` - Number of parallel Ginkgo processes (usually `4`)

---

## 🎯 Testing Your Setup

After completing the checklist, test your coverage setup:

```bash
# Run E2E tests with coverage
make test-e2e-{service}-coverage

# Expected output:
# 1. Build with coverage instrumentation (GOFLAGS=-cover)
# 2. Run E2E tests
# 3. Generate coverage reports:
#    - test/e2e/{service}/e2e-coverage.txt
#    - test/e2e/{service}/e2e-coverage.html
#    - test/e2e/{service}/e2e-coverage-func.txt
# 4. Show coverage summary
```

**Success Criteria**:
- ✅ Tests run successfully
- ✅ Coverage reports generated
- ✅ HTML report opens in browser
- ✅ Coverage data shows actual percentages (not 0%)

---

## 🚨 Common Issues & Solutions

### Issue 1: "Image build failed"

**Symptom**: Coverage build fails with compile errors

**Possible Causes**:
- Dockerfile has invalid conditional logic
- Build flags incompatible with coverage (`-a`, `-installsuffix`)

**Solution**:
```bash
# Test coverage build manually
podman build --build-arg GOFLAGS=-cover -t test:cov -f docker/{service}.Dockerfile .

# Check build output for errors
```

### Issue 2: "No coverage data found"

**Symptom**: Tests pass but no coverage data in `/coverdata`

**Possible Causes**:
- `GOCOVERDIR` not set in deployment
- Volume mount missing or incorrect
- Controller crashed before flushing coverage

**Solution**:
```bash
# Check if GOCOVERDIR is set in pod
kubectl exec -n {namespace} {pod} -- env | grep GOCOVERDIR

# Check if /coverdata is writable
kubectl exec -n {namespace} {pod} -- ls -la /coverdata

# Check controller logs for coverage messages
kubectl logs -n {namespace} {pod} | grep -i cover
```

### Issue 3: "Coverage reports empty (0%)"

**Symptom**: Reports generated but show 0% coverage

**Possible Causes**:
- Binary not built with `-cover` flag
- Controller didn't shut down gracefully (coverage not flushed)
- Wrong binary path in coverage data

**Solution**:
```bash
# Verify binary has coverage instrumentation
kubectl exec -n {namespace} {pod} -- /path/to/binary -h 2>&1 | grep -i cover

# Check if coverage files exist
ls -la test/e2e/{service}/coverdata/

# Manually inspect coverage data
go tool covdata percent -i=test/e2e/{service}/coverdata/
```

### Issue 4: "Image not present locally" (Kind/Podman)

**Symptom**: `kind load docker-image` fails with "image not present"

**Possible Causes**:
- Podman localhost prefix mismatch
- Kind experimental provider not configured

**Solution**:
- Ensure `KIND_EXPERIMENTAL_PROVIDER=podman` is set
- Use image archive approach (already implemented in DD-TEST-008)
- Check `test/infrastructure/datastorage_bootstrap.go` for reference implementation

---

## 📊 Current Service Status

| Service | Dockerfile | Kind Config | E2E Deploy | Makefile | Status |
|---------|-----------|-------------|------------|----------|--------|
| **Notification** | ✅ | ✅ | ✅ | ✅ | 🚧 Testing (pod readiness issue) |
| **DataStorage** | ✅ | ✅ | ✅ | ❓ | ⏳ Needs Makefile target |
| **Gateway** | ❓ | ❓ | ❓ | ❓ | ⏳ Needs assessment |
| **WorkflowExecution** | ❓ | ❓ | ❓ | ❓ | ⏳ Needs assessment |
| **SignalProcessing** | ❓ | ❓ | ❓ | ❓ | ⏳ Needs assessment |
| **RemediationOrchestrator** | ❓ | ❓ | ❓ | ❓ | ⏳ Needs assessment |
| **AIAnalysis** | ❓ | ❓ | ❓ | ❓ | ⏳ Needs assessment |
| **Toolset** | ❓ | ❓ | ❓ | ❓ | ⏳ Needs assessment |

**Legend**:
- ✅ Implemented
- ❓ Unknown (needs assessment)
- ⏳ Pending implementation
- 🚧 In progress

---

## 🎯 Migration Priority

### Phase 1: Services with Existing E2E Tests (High Priority)
1. **DataStorage** - Has E2E tests, has custom 45-line coverage target → migrate to 1-line
2. **WorkflowExecution** - Has E2E tests, has custom coverage target → migrate
3. **SignalProcessing** - Has E2E tests, has custom coverage target → migrate
4. **Gateway** - Has E2E tests → add coverage support

### Phase 2: Services Without E2E Coverage (Medium Priority)
5. **Notification** - In progress (pod readiness issue blocking)
6. **RemediationOrchestrator** - Needs E2E tests first
7. **AIAnalysis** - Needs E2E tests first
8. **Toolset** - Needs E2E tests first

---

## 📚 Reference Documents

### Authoritative Standards
- **[DD-TEST-007](DD-TEST-007-e2e-coverage-capture-standard.md)**: E2E Coverage Capture Standard (technical foundation)
- **[DD-TEST-008](DD-TEST-008-reusable-e2e-coverage-infrastructure.md)**: Reusable E2E Coverage Infrastructure (implementation guide)

### Implementation Examples
- **Notification Service**: `test/infrastructure/notification.go` (reference for E2E deployment patterns)
- **DataStorage Service**: `test/infrastructure/datastorage.go` (lines 860-975 for GOCOVERDIR conditional logic)
- **Makefile**: Line ~952 for Notification coverage target example

### Handoff Documents
- **[REUSABLE_E2E_COVERAGE_INFRASTRUCTURE_DEC_23_2025.md](REUSABLE_E2E_COVERAGE_INFRASTRUCTURE_DEC_23_2025.md)**: Overview and rationale
- **[NT_E2E_COVERAGE_IMPLEMENTATION_PROGRESS_DEC_23_2025.md](NT_E2E_COVERAGE_IMPLEMENTATION_PROGRESS_DEC_23_2025.md)**: Notification implementation progress (includes bug fixes)

---

## 🆘 Getting Help

### For Questions
- Check DD-TEST-007 for technical details
- Check DD-TEST-008 for usage examples
- Review Notification service implementation as reference

### For Issues
- Create handoff document describing the issue
- Include service name, error messages, and what you've tried
- Reference this checklist to show which steps you've completed

### For Collaboration
- Notification team has solved most infrastructure issues (image loading, Kind/Podman integration)
- DataStorage team contributed build flag guidance and permissions patterns
- Share your learnings in handoff documents for other teams

---

## ✅ Summary

**To activate E2E coverage for your service**:

1. ✅ **Verify Dockerfile** supports `GOFLAGS=-cover` (5 min)
2. ✅ **Verify Kind config** has `/coverdata` extraMounts (5 min)
3. ✅ **Verify E2E deployment** adds `GOCOVERDIR` conditionally (5 min)
4. ✅ **Add 1 line** to Makefile using reusable infrastructure (30 sec)
5. ✅ **Test** by running `make test-e2e-{service}-coverage`

**Total effort**: ~15 minutes per service (if infrastructure already exists)

**Benefit**: Comprehensive E2E coverage reports with **97% less code** (1 line vs 45+ lines)

---

**Document Owner**: Platform Team
**Last Updated**: December 23, 2025
**Next Review**: After all services have E2E coverage enabled



