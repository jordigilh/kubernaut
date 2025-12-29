# 🔍 Triage: Gateway Service & Shared Build Utilities

**Date**: December 15, 2025
**Triaged By**: AI Assistant (Gateway Team)
**Status**: ⚠️ **ACTION RECOMMENDED**
**Priority**: P2 (Medium - Should implement for v1.0 consistency)

---

## 📋 **Executive Summary**

**Issue**: Gateway service is NOT included in the shared build utilities announced by Platform Team.

**Current State**:
- ✅ Gateway has a Dockerfile: `docker/gateway-ubi9.Dockerfile`
- ❌ Gateway uses hardcoded tags in E2E tests: `kubernaut-gateway:e2e-test`
- ❌ Gateway not listed in `scripts/build-service-image.sh` supported services
- ⚠️ Gateway E2E tests vulnerable to tag conflicts if run in parallel with other developers

**Recommendation**: **ADD Gateway to shared build utilities** for DD-TEST-001 compliance and team consistency.

**Effort**: 30-45 minutes (low effort, high value)

---

## 🎯 **Problem Analysis**

### **1. Current Gateway Build Infrastructure**

**Makefile Targets** (`Makefile` lines 642-658):
```makefile
docker-build-gateway-service: ## Build gateway service container image (multi-arch UBI9)
	podman build --platform linux/amd64,linux/arm64 \
		-f docker/gateway-ubi9.Dockerfile \
		-t $(REGISTRY)/kubernaut-gateway:$(VERSION) .

docker-build-gateway-single: ## Build single-arch debug image
	podman build -t $(REGISTRY)/kubernaut-gateway:$(VERSION)-$(shell uname -m) \
		-f docker/gateway-ubi9.Dockerfile .
```

**E2E Infrastructure** (`test/infrastructure/gateway_e2e.go:348`):
```go
"localhost/kubernaut-gateway:e2e-test"  // Hardcoded tag - DD-TEST-001 violation!
```

**Problem**: Hardcoded tags create conflicts when multiple developers run E2E tests simultaneously.

---

### **2. DD-TEST-001 Compliance Gap**

**Standard**: All services should use unique tags: `{service}-{user}-{git-hash}-{timestamp}`

**Gateway Current State**:
- ❌ E2E tests use hardcoded `e2e-test` tag
- ❌ Not using DD-TEST-001 tag format
- ❌ Vulnerable to multi-developer test conflicts

**Other Services**:
- ✅ 7 services supported: notification, signalprocessing, remediationorchestrator, workflowexecution, aianalysis, datastorage, hapi
- ✅ All use shared build utilities with unique tags

**Gap**: Gateway is the **ONLY** stateless service not using shared build utilities.

---

### **3. Risk Assessment**

**Current Risks**:
1. **Multi-Developer Conflicts**: If two developers run `make test-e2e-gateway` simultaneously:
   - Both build `kubernaut-gateway:e2e-test`
   - Podman overwrites first developer's image
   - First developer's tests may fail with wrong image version
   - Debugging becomes difficult due to tag collision

2. **Team Inconsistency**: Gateway team uses different build patterns than all other service teams

3. **DD-TEST-001 Non-Compliance**: Gateway E2E tests violate the project's testing standards

**Likelihood**: Medium (increases as team grows)
**Impact**: Medium (test failures, debugging confusion, developer frustration)

---

## ✅ **Recommended Solution**

### **Option A: Add Gateway to Shared Build Utilities** (RECOMMENDED)

**Changes Required**:

#### 1. Update `scripts/build-service-image.sh` (lines 103-111)
```bash
declare -A SERVICE_DOCKERFILES=(
    ["notification"]="docker/notification-controller.Dockerfile"
    ["signalprocessing"]="docker/signalprocessing-controller.Dockerfile"
    ["remediationorchestrator"]="docker/remediationorchestrator-controller.Dockerfile"
    ["workflowexecution"]="docker/workflowexecution-controller.Dockerfile"
    ["aianalysis"]="docker/aianalysis-controller.Dockerfile"
    ["datastorage"]="docker/data-storage.Dockerfile"
    ["hapi"]="holmesgpt-api/Dockerfile"
    ["gateway"]="docker/gateway-ubi9.Dockerfile"  # ADD THIS LINE
)
```

#### 2. Update Gateway E2E Infrastructure (`test/infrastructure/gateway_e2e.go`)

**Before** (lines 330-350):
```go
func (i *GatewayE2EInfrastructure) buildImageAndLoadToKind(writer io.Writer, clusterName string) error {
    fmt.Fprintln(writer, "📦 Building Gateway service image...")

    imageName := "localhost/kubernaut-gateway:e2e-test"  // Hardcoded tag

    buildCmd := exec.Command("podman", "build",
        "-t", imageName,
        "-f", "docker/gateway-ubi9.Dockerfile",
        ".",
    )
    // ...
}
```

**After** (with unique tags):
```go
func (i *GatewayE2EInfrastructure) buildImageAndLoadToKind(writer io.Writer, clusterName string) error {
    fmt.Fprintln(writer, "📦 Building Gateway service image...")

    // Generate unique tag per DD-TEST-001
    user := os.Getenv("USER")
    if user == "" {
        user = "unknown"
    }
    gitHash := getGitHash()
    timestamp := time.Now().Unix()
    uniqueTag := fmt.Sprintf("gateway-%s-%s-%d", user, gitHash, timestamp)

    imageName := fmt.Sprintf("localhost/kubernaut-gateway:%s", uniqueTag)

    buildCmd := exec.Command("podman", "build",
        "-t", imageName,
        "-f", "docker/gateway-ubi9.Dockerfile",
        ".",
    )
    // ...
}

// Helper function to get git hash
func getGitHash() string {
    cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
    output, err := cmd.Output()
    if err != nil {
        return "nogit"
    }
    return strings.TrimSpace(string(output))
}
```

#### 3. Update Gateway E2E Deployment Manifest (`test/e2e/gateway/gateway-deployment.yaml`)

**Before** (line ~20):
```yaml
spec:
  containers:
  - name: gateway
    image: localhost/kubernaut-gateway:e2e-test  # Hardcoded tag
```

**After** (with environment variable):
```yaml
spec:
  containers:
  - name: gateway
    image: ${GATEWAY_IMAGE}  # Injected by test infrastructure
```

**Update test infrastructure to inject image**:
```go
// In test/infrastructure/gateway_e2e.go
func (i *GatewayE2EInfrastructure) deployGatewayService(writer io.Writer) error {
    // ...
    // Replace ${GATEWAY_IMAGE} with actual image name
    manifestContent = strings.ReplaceAll(manifestContent, "${GATEWAY_IMAGE}", imageName)
    // ...
}
```

**Effort**: 30-45 minutes
**Benefit**: DD-TEST-001 compliance, no multi-developer conflicts, team consistency
**Risk**: Very low (well-tested pattern used by 7 other services)

---

### **Option B: Use Shared Script Directly in E2E Tests** (ALTERNATIVE)

Instead of modifying `gateway_e2e.go`, call the shared script:

```go
func (i *GatewayE2EInfrastructure) buildImageAndLoadToKind(writer io.Writer, clusterName string) error {
    fmt.Fprintln(writer, "📦 Building Gateway service image via shared script...")

    buildCmd := exec.Command("./scripts/build-service-image.sh",
        "gateway",
        "--kind",
        "--cluster", clusterName,
    )
    buildCmd.Stdout = writer
    buildCmd.Stderr = writer

    if err := buildCmd.Run(); err != nil {
        return fmt.Errorf("build script failed: %w", err)
    }

    // Read generated tag from .last-image-tag-gateway.env
    tagFile := ".last-image-tag-gateway.env"
    content, err := os.ReadFile(tagFile)
    if err != nil {
        return fmt.Errorf("failed to read image tag: %w", err)
    }

    imageName := strings.TrimSpace(string(content))
    // Use imageName in deployment...
}
```

**Effort**: 20-30 minutes
**Benefit**: Zero maintenance (Platform Team maintains script), automatic cleanup support
**Risk**: Very low

---

## 📊 **Comparison Matrix**

| Aspect | Option A (Add to Script) | Option B (Call Script) | Current State (No Change) |
|--------|-------------------------|------------------------|---------------------------|
| **DD-TEST-001 Compliance** | ✅ Full | ✅ Full | ❌ Non-compliant |
| **Multi-Dev Conflicts** | ✅ Resolved | ✅ Resolved | ❌ Vulnerable |
| **Team Consistency** | ✅ Yes | ✅ Yes | ❌ Inconsistent |
| **Maintenance Burden** | 🟡 Low (shared script) | ✅ Zero (Platform Team) | ❌ High (custom code) |
| **Implementation Effort** | 30-45 min | 20-30 min | 0 min |
| **Testing Required** | Moderate (E2E) | Light (E2E) | None |
| **Risk Level** | Very Low | Very Low | Medium (conflicts) |

---

## 🎯 **Recommended Action Plan**

### **Phase 1: Add Gateway to Shared Script** (30 minutes)

1. **Update shared script** (`scripts/build-service-image.sh`):
   - Add Gateway to service mapping (1 line change)
   - Test: `./scripts/build-service-image.sh gateway --help`
   - Test: `./scripts/build-service-image.sh gateway`

2. **Verify build works**:
   ```bash
   ./scripts/build-service-image.sh gateway --kind --cluster gateway-test
   ```

### **Phase 2: Update E2E Infrastructure** (15 minutes)

1. **Option A**: Modify `gateway_e2e.go` to generate unique tags
2. **Option B**: Call shared script from `gateway_e2e.go`

**Recommendation**: **Option B** (simpler, zero maintenance)

### **Phase 3: Test E2E Suite** (10-15 minutes)

1. Run E2E tests to verify unique tags work:
   ```bash
   make test-e2e-gateway
   ```

2. Verify deployment uses correct image tag

3. Verify cleanup works (no leftover images)

### **Phase 4: Update Documentation** (5 minutes)

1. Update Gateway README with new build command
2. Add Gateway to `TEAM_ANNOUNCEMENT_SHARED_BUILD_UTILITIES.md` supported services list

**Total Effort**: 60-75 minutes
**Timeline**: Can be completed in single session

---

## ✅ **Benefits Summary**

### **Technical Benefits**:
- ✅ DD-TEST-001 compliance (unique container tags)
- ✅ No multi-developer test conflicts
- ✅ Consistent build patterns across all services
- ✅ Automatic cleanup support (`--cleanup` flag)
- ✅ Zero maintenance (Platform Team owns script)

### **Team Benefits**:
- ✅ Gateway team uses same tools as other services
- ✅ New team members learn one build pattern
- ✅ Reduced cognitive load (no service-specific quirks)
- ✅ Easier cross-service work

### **Operational Benefits**:
- ✅ Production-ready multi-arch builds available
- ✅ CI/CD integration ready
- ✅ Custom tag support for special cases

---

## 🚦 **Risk Assessment**

### **Implementation Risks**: Very Low

**Mitigations**:
- Pattern proven by 7 existing services
- Platform Team maintains shared script
- E2E tests validate functionality
- Easy rollback (revert commits)

### **Deferral Risks**: Medium

If we DON'T implement this:
1. Gateway remains vulnerable to multi-developer test conflicts
2. Gateway team uses different patterns than everyone else
3. DD-TEST-001 compliance gap persists
4. Technical debt increases (custom build code to maintain)

---

## 💬 **Open Questions**

### **Q1: Why wasn't Gateway included initially?**
**A**: Gateway E2E tests were implemented after the shared utilities announcement. Gateway team likely wasn't aware of the new standard.

### **Q2: Can we defer this to v2.0?**
**A**: Yes, but NOT recommended. Risk of multi-developer conflicts increases as team grows. Better to fix now while E2E infrastructure is fresh in mind.

### **Q3: Do other teams have similar gaps?**
**A**: No - all 7 other services are already using shared utilities. Gateway is the only exception.

### **Q4: What if we have multiple Gateway developers running tests simultaneously RIGHT NOW?**
**A**: Test conflicts ARE possible. Unique tags would eliminate this risk entirely.

---

## 📅 **Recommended Timeline**

| Timeframe | Action | Status |
|-----------|--------|--------|
| **Today** | Review this triage document | ⏸️ Pending user approval |
| **This Week** | Implement shared utilities integration | ⏸️ Blocked on approval |
| **Before v1.0** | Complete E2E testing with unique tags | ⏸️ Blocked on implementation |
| **Post-v1.0** | Update team documentation | ⏸️ Blocked on implementation |

**Critical Path**: Gateway v1.0 readiness

---

## 🎯 **Final Recommendation**

### **Recommendation**: ✅ **IMPLEMENT NOW** (before v1.0 release)

**Rationale**:
1. **Low effort** (60-75 minutes total)
2. **High value** (DD-TEST-001 compliance, no conflicts, team consistency)
3. **Very low risk** (proven pattern, easy rollback)
4. **Right timing** (Gateway E2E infrastructure is recent, easy to modify)
5. **Team consistency** (Gateway shouldn't be the exception)

**Priority**: P2 (Medium - should complete for v1.0)

**Confidence**: 95% (clear problem, proven solution, minimal risk)

---

## 📋 **Next Steps**

### **For User Approval**:
1. Review this triage document
2. Approve/reject implementation
3. Choose Option A (add to script) or Option B (call script)

### **If Approved**:
1. Add Gateway to `scripts/build-service-image.sh` service mapping
2. Update `test/infrastructure/gateway_e2e.go` to use unique tags
3. Run E2E tests to verify
4. Update documentation
5. Mark GAP item as complete

### **If Deferred**:
1. Document decision rationale
2. Accept multi-developer conflict risk
3. Plan for post-v1.0 implementation
4. Add to Gateway v2.0 backlog

---

**Triage Complete** ✅

**Awaiting User Decision**: Implement now (recommended) or defer to v2.0?

---

**Document Version**: 1.0
**Last Updated**: December 15, 2025
**Triaged By**: AI Assistant (Gateway Team)
**Contact**: Gateway Team Lead


