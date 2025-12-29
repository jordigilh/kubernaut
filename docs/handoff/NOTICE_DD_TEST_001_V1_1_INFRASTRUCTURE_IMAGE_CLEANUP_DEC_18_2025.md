# 🚨 NOTICE: DD-TEST-001 v1.1 - Test Image Cleanup Required

**Date**: December 18, 2025
**Priority**: **P1 - REQUIRED**
**Deadline**: December 22, 2025 (End of Week)
**Affected Services**: All 8 services (integration + E2E tests)
**Document**: [DD-TEST-001 v1.1](../architecture/decisions/DD-TEST-001-unique-container-image-tags.md)

---

## 📋 **Executive Summary**

**DD-TEST-001 v1.1** adds **MANDATORY image cleanup** for all services in BOTH test tiers:
1. **Integration Tests**: podman-compose infrastructure cleanup
2. **E2E Tests**: Kind service image cleanup

**What Changed**:
- ✅ **NEW Section 4.3**: Infrastructure Image Cleanup (podman-compose + Kind)
- ✅ **Integration BeforeSuite**: Clean stale containers from failed previous runs
- ✅ **Integration AfterSuite**: Stop containers + prune infrastructure images
- ✅ **E2E AfterSuite**: Remove service images built for Kind

**Why Required**:
- 🚨 **Disk Space**: Images accumulate rapidly
  - Integration: ~500MB-1GB per run (postgres, redis, datastorage)
  - E2E: ~200-500MB per run (service images per unique tag)
  - Combined: ~7-15GB saved per 10 test runs
- 🚨 **Multi-Team**: Parallel testing creates multiple image stacks
- 🚨 **Stability**: Prevents "disk full" and "address already in use" errors

**Impact**: ~7 seconds cleanup overhead per test run vs 7-15GB disk space saved per day

---

## 🎯 **Action Required**

### **All Service Teams MUST**:

1. ✅ **Update Integration Test Suite** (`test/integration/{service}/suite_test.go`)
   - Add BeforeSuite cleanup (stale containers)
   - Add AfterSuite cleanup (containers + infrastructure image pruning)

2. ✅ **Update E2E Test Suite** (`test/e2e/{service}/suite_test.go` or similar)
   - Add AfterSuite cleanup (service image removal)
   - Remove images built per unique tag format

3. ✅ **Verify Cleanup Works**
   - Run integration tests → confirm containers stopped + images pruned
   - Run E2E tests → confirm service images removed
   - Check: `podman images` should show minimal/no test images

4. ✅ **Update Service Documentation** (if applicable)
   - Document cleanup behavior in service README
   - Update testing guide with new requirements

5. ✅ **Acknowledge Receipt**
   - Reply to this notice with service name and completion status
   - Add entry to "Team Acknowledgments" section below

---

## 📊 **Affected Services (8 Total)**

| Service | Integration Tests | E2E Tests | Status | Owner | Due Date |
|---------|------------------|-----------|--------|-------|----------|
| **WorkflowExecution** | ✅ podman-compose | ✅ Kind | ✅ **COMPLETE** (Int Ref) | WE Team | Dec 18, 2025 |
| **DataStorage** | ✅ podman-compose | ✅ Kind | ✅ **COMPLETE** | DS Team | Dec 18, 2025 |
| **AIAnalysis** | ✅ podman-compose | ✅ Kind | ✅ **COMPLETE** | AI Team | Dec 18, 2025 |
| **Gateway** | ✅ podman-compose | ✅ Kind | ✅ **COMPLETE** | Gateway Team | Dec 18, 2025 |
| **Notification** | ✅ podman-compose | ✅ Kind | ✅ **COMPLETE** | Notification Team | Dec 18, 2025 |
| **SignalProcessing** | ✅ podman-compose | ✅ Kind | ✅ **COMPLETE** | SP Team | Dec 18, 2025 |
| **RemediationOrchestrator** | ✅ podman-compose | ✅ Kind | ✅ **COMPLETE** | RO Team | Dec 18, 2025 |
| **HAPI** | ✅ pytest hooks | N/A Go-managed | ✅ **COMPLETE** (Python) | HAPI Team | Dec 18, 2025 |

**Cleanup Required**:
- **Integration**: podman-compose infrastructure (postgres, redis, datastorage, etc.)
- **E2E**: Service images built per unique tag format for Kind cluster

---

## 📝 **Implementation Guide**

### **Step 1: Import Required Package**

Add to your `suite_test.go` imports:

```go
import (
	"os/exec"
	// ... other imports
)
```

### **Step 2: Add BeforeSuite Cleanup**

Add at the **start** of your `BeforeSuite` function:

```go
var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("Cleaning up stale containers from previous runs")
	// Stop any existing containers from failed previous runs
	testDir, err := filepath.Abs(filepath.Join(".", "..", "..", ".."))
	if err != nil {
		GinkgoWriter.Printf("⚠️  Failed to determine project root: %v\n", err)
	} else {
		cleanupCmd := exec.Command("podman-compose", "-f", "podman-compose.test.yml", "down")
		cleanupCmd.Dir = filepath.Join(testDir, "test", "integration", "{YOUR_SERVICE}")
		_, cleanupErr := cleanupCmd.CombinedOutput()
		if cleanupErr != nil {
			GinkgoWriter.Printf("⚠️  Cleanup of stale containers failed (may not exist): %v\n", cleanupErr)
		} else {
			GinkgoWriter.Println("✅ Stale containers cleaned up")
		}
	}

	// ... continue with existing BeforeSuite logic
	By("Registering CRD schemes")
	// ... rest of setup
})
```

**Replace `{YOUR_SERVICE}` with**:
- `datastorage` for DataStorage
- `aianalysis` for AIAnalysis
- `gateway` for Gateway
- `notification` for Notification
- `signalprocessing` for SignalProcessing
- `remediationorchestrator` for RemediationOrchestrator
- `hapi` for HAPI

### **Step 3: Add AfterSuite Cleanup**

Add **after** EnvTest teardown in your `AfterSuite`:

```go
var _ = AfterSuite(func() {
	By("Tearing down the test environment")

	cancel()

	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	By("Stopping infrastructure (podman-compose)")
	// Stop podman-compose services
	testDir, pathErr := filepath.Abs(filepath.Join(".", "..", "..", ".."))
	if pathErr != nil {
		GinkgoWriter.Printf("⚠️  Failed to determine project root: %v\n", pathErr)
	} else {
		cmd := exec.Command("podman-compose", "-f", "podman-compose.test.yml", "down")
		cmd.Dir = filepath.Join(testDir, "test", "integration", "{YOUR_SERVICE}")
		output, cmdErr := cmd.CombinedOutput()
		if cmdErr != nil {
			GinkgoWriter.Printf("⚠️  Failed to stop containers: %v\n%s\n", cmdErr, output)
		} else {
			GinkgoWriter.Println("✅ Infrastructure stopped")
		}
	}

	By("Cleaning up infrastructure images to prevent disk space issues")
	// Prune ONLY infrastructure images for this service
	pruneCmd := exec.Command("podman", "image", "prune", "-f",
		"--filter", "label=io.podman.compose.project={YOUR_SERVICE}")
	pruneOutput, pruneErr := pruneCmd.CombinedOutput()
	if pruneErr != nil {
		GinkgoWriter.Printf("⚠️  Failed to prune images: %v\n%s\n", pruneErr, pruneOutput)
	} else {
		GinkgoWriter.Println("✅ Infrastructure images pruned")
	}

	GinkgoWriter.Println("✅ Cleanup complete")
})
```

**Replace `{YOUR_SERVICE}` with your service name (same as Step 2).**

### **Step 4: Add E2E Test Cleanup**

Add to your E2E test suite (`test/e2e/{YOUR_SERVICE}/suite_test.go` or similar):

```go
var _ = AfterSuite(func() {
	By("Tearing down Kind cluster")
	// ... existing Kind cluster teardown ...

	By("Cleaning up service images built for Kind")
	// Remove service image built for this test run
	imageTag := os.Getenv("IMAGE_TAG")  // Set by build/test infrastructure
	if imageTag != "" {
		serviceName := "{YOUR_SERVICE}"  // e.g., "notification", "gateway"
		imageName := fmt.Sprintf("%s:%s", serviceName, imageTag)

		pruneCmd := exec.Command("podman", "rmi", imageName)
		pruneOutput, pruneErr := pruneCmd.CombinedOutput()
		if pruneErr != nil {
			GinkgoWriter.Printf("⚠️  Failed to remove service image: %v\n%s\n", pruneErr, pruneOutput)
		} else {
			GinkgoWriter.Printf("✅ Service image removed: %s\n", imageName)
		}
	}

	By("Pruning dangling images from Kind builds")
	// Prune any dangling images left from failed builds
	pruneCmd := exec.Command("podman", "image", "prune", "-f")
	_, _ = pruneCmd.CombinedOutput()

	GinkgoWriter.Println("✅ E2E cleanup complete")
})
```

**Service Names**:
- `datastorage`, `aianalysis`, `gateway`, `notification`, `signalprocessing`, `remediationorchestrator`, `workflowexecution`, `hapi`

### **Step 5: Verify Implementation**

**Integration Test Verification**:
```bash
# Before running tests - check for existing containers
cd test/integration/{YOUR_SERVICE}
podman-compose -f podman-compose.test.yml ps

# Run integration tests
cd ../../..
make test-integration-{YOUR_SERVICE}

# Verify cleanup - should show NO containers
cd test/integration/{YOUR_SERVICE}
podman-compose -f podman-compose.test.yml ps
# Expected: Empty output (no containers)

# Verify images pruned
podman images | grep "io.podman.compose.project={YOUR_SERVICE}"
# Expected: Empty or minimal output (base images may remain if shared)
```

**E2E Test Verification**:
```bash
# Check for service images before E2E run
podman images | grep "^{YOUR_SERVICE}:"

# Run E2E tests
make test-e2e-{YOUR_SERVICE}

# Verify service image cleaned up
podman images | grep "^{YOUR_SERVICE}:"
# Expected: Empty output (service images removed)

# Check dangling images
podman images --filter "dangling=true"
# Expected: Minimal or empty (dangling images pruned)
```

---

## 🔍 **Reference Implementation**

**WorkflowExecution** (COMPLETE) - Use as reference:

📁 **Integration Tests**: `test/integration/workflowexecution/suite_test.go`
- **Lines 177-192**: BeforeSuite cleanup (stale containers)
- **Lines 303-327**: AfterSuite cleanup (containers + infrastructure images)
- **Line 21**: Import addition (`"os/exec"`)

📁 **E2E Tests**: `test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go`
- **Lines 226-270**: AfterSuite cleanup (service images + dangling image pruning)
- **Runs regardless of test pass/fail**: Prevents image accumulation

**Key Patterns**:
- ✅ Absolute path resolution for parallel test safety
- ✅ Error handling doesn't block test execution
- ✅ Label-based filtering prevents cross-service conflicts (integration)
- ✅ IMAGE_TAG environment variable for service image cleanup (E2E)
- ✅ Informative GinkgoWriter output for debugging
- ✅ Best-effort pruning (non-blocking on errors)

---

## 📊 **Benefits**

### **Disk Space Management**
**Per Test Run**:
- Integration: Prevents ~500MB-1GB accumulation (infrastructure)
- E2E: Prevents ~200-500MB accumulation (service images)
- **Combined**: ~700MB-1.5GB per complete test run

**Daily (10 runs)**:
- Integration: ~5-10GB saved
- E2E: ~2-5GB saved
- **Combined**: ~7-15GB saved per day

**Weekly (50 runs)**:
- Integration: ~25-50GB saved
- E2E: ~10-25GB saved
- **Combined**: ~35-75GB saved per week

**Impact**: Eliminates "disk full" test failures

### **Multi-Team Stability**
- ✅ Parallel test runs don't interfere
- ✅ Clean slate for each test execution
- ✅ No "port already in use" errors from stale containers

### **Developer Experience**
- ✅ No manual cleanup required
- ✅ Faster debugging (clean state every run)
- ✅ Consistent test results

### **CI/CD**
- ✅ Automatic cleanup in pipelines
- ✅ No build failures from disk space issues
- ✅ Predictable resource usage

---

## ⏱️ **Performance Impact**

**Overhead per test run**:
- BeforeSuite cleanup: ~0.5s (if stale containers exist)
- Test execution: No change
- AfterSuite cleanup: ~7s (5s down + 2s prune)

**Total**: ~7.5s per test run

**Trade-off Analysis**:
- ✅ **Worth it**: 7s overhead vs 5-10GB disk space saved
- ✅ **Scalable**: Overhead doesn't increase with test count
- ✅ **Necessary**: Prevents infrastructure failures

---

## 🚨 **Common Issues & Solutions**

### **Issue 1: "chdir: no such file or directory"**

**Symptom**: AfterSuite cleanup fails with path error

**Solution**: Verify path resolution logic:
```go
testDir, pathErr := filepath.Abs(filepath.Join(".", "..", "..", ".."))
// This resolves to project root from test/integration/{service}/
```

**Debugging**:
```go
GinkgoWriter.Printf("Project root: %s\n", testDir)
GinkgoWriter.Printf("Compose dir: %s\n", filepath.Join(testDir, "test", "integration", "{service}"))
```

### **Issue 2: Images Not Pruned**

**Symptom**: Images remain after tests

**Cause**: Incorrect label filter or images in use

**Solution**:
```bash
# Check actual label
podman inspect {image_id} | grep -A5 Labels

# Verify label matches your service name
# Expected: "io.podman.compose.project": "{service}"

# If different, update filter in AfterSuite
```

### **Issue 3: Containers Still Running**

**Symptom**: `podman-compose ps` shows containers after tests

**Cause**: AfterSuite not executing or errors in cleanup

**Solution**:
```bash
# Check test output for AfterSuite errors
make test-integration-{service} 2>&1 | grep -A10 "AfterSuite"

# Manual cleanup if needed
cd test/integration/{service}
podman-compose -f podman-compose.test.yml down
```

---

## 📚 **Related Documents**

- **DD-TEST-001 v1.1**: [Unique Container Image Tags](../architecture/decisions/DD-TEST-001-unique-container-image-tags.md)
- **DD-TEST-001 v1.0**: Original specification (December 15, 2025)
- **Reference Implementation**: `test/integration/workflowexecution/suite_test.go`

---

## ✅ **Team Acknowledgments**

Please update this section when your service implementation is complete:

### **Completed**

| Service | Team | Date | Notes |
|---------|------|------|-------|
| **WorkflowExecution** | WE Team | Dec 18, 2025 | ✅ Reference implementation (Integration + E2E cleanup) |
| **RemediationOrchestrator** | RO Team | Dec 18, 2025 | ✅ Integration + E2E cleanup implemented |
| **DataStorage** | DS Team | Dec 18, 2025 | Integration + E2E cleanup implemented |
| **Notification** | Notification Team | Dec 18, 2025 | ✅ Integration + E2E cleanup implemented (podman-compose infrastructure created) |
| **AIAnalysis** | AI Team | Dec 18, 2025 | ✅ Integration + E2E cleanup implemented per DD-TEST-001 v1.1 |
| **SignalProcessing** | SP Team | Dec 18, 2025 | ✅ Integration + E2E cleanup implemented per DD-TEST-001 v1.1 |
| **Gateway** | Gateway Team | Dec 18, 2025 | ✅ Integration + E2E cleanup implemented per DD-TEST-001 v1.1 |
| **HAPI** | HAPI Team | Dec 18, 2025 | ✅ Integration cleanup with pytest hooks (E2E uses Go-managed infrastructure) |

### **In Progress**

| Service | Team | Started | ETA | Blockers |
|---------|------|---------|-----|----------|
| | | | | |

### **Not Started**

| Service | Team | Planned Start | Notes |
|---------|------|---------------|-------|
| | | | |

---

## 📞 **Support**

### **Questions?**
- **Platform Team**: Contact for DD-TEST-001 clarifications
- **WE Team**: Contact for implementation guidance (reference implementation)

### **Implementation Help**
- **Pairing Sessions**: Available for complex integrations
- **Code Review**: Available for verification before merge

### **Troubleshooting**
- **Slack**: #platform-testing channel
- **GitHub**: Open issue with label `infrastructure-cleanup`

---

## 🎯 **Success Criteria**

Your service implementation is complete when:

**Integration Tests**:
- ✅ BeforeSuite cleans stale containers (verified by test output)
- ✅ AfterSuite stops containers (verified by `podman-compose ps`)
- ✅ AfterSuite prunes infrastructure images (verified by `podman images | grep {service}`)
- ✅ Integration tests pass with cleanup
- ✅ No containers remain after test completion

**E2E Tests**:
- ✅ AfterSuite removes service image (verified by test output)
- ✅ AfterSuite prunes dangling images
- ✅ E2E tests pass with cleanup
- ✅ No service images remain after test completion (check `podman images | grep "^{service}:"`)

**Documentation & Acknowledgment**:
- ✅ Team acknowledges completion in this document
- ✅ Service README updated (if applicable)

---

## ⏰ **Timeline**

- **Dec 18, 2025**: Notice issued, DD-TEST-001 v1.1 published
- **Dec 18-22, 2025**: Service teams implement cleanup (Week 1)
- **Dec 22, 2025**: **Deadline** - All services MUST have cleanup implemented
- **Dec 23, 2025**: Verification and compliance check
- **Dec 26, 2025**: Follow-up for non-compliant services

---

**Document Status**: ✅ Active
**Priority**: P1 - REQUIRED
**Compliance**: MANDATORY for all services using podman-compose
**Next Review**: After all services complete implementation

