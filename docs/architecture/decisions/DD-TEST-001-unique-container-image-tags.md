# DD-TEST-001: Unique Container Image Tags for Multi-Team Testing

**Status**: APPROVED
**Date**: December 15, 2025
**Author**: Platform Team
**Category**: Testing Infrastructure
**Version**: 1.5
**Scope**: All CRD Controller Services + Shared Infrastructure

---

## 📝 **Changelog**

### Version 1.5 (January 8, 2026)
**Added**:
- **Section 6**: Container Networking Patterns for Integration Tests
- Authoritative guidance on container-to-container communication vs host communication
- Internal vs external port usage patterns (8080 vs 18095)
- Podman DNS resolution on custom networks
- `host.containers.internal` usage patterns (only for HOST services, not containers)
- RCA from AIAnalysis integration test HTTP 500 errors
- Code examples, validation commands, and debugging procedures

**Impact**: Prevents HTTP 500 errors caused by incorrect container networking configuration. Teams can quickly identify and fix container communication issues in integration tests.

**Affected Services**: AIAnalysis (v1.5), all future integration tests using container-to-container communication

**See**: `docs/handoff/AA_INTEGRATION_HTTP500_FIX_JAN08.md` for detailed RCA

### Version 1.4 (January 7, 2026)
**Added**:
- **Section 5**: Consolidated E2E Image Build Functions for standard parallel patterns
- `BuildAndLoadImageToKind()` function in `test/infrastructure/datastorage_bootstrap.go`
- `E2EImageConfig` struct for configurable image building with DD-TEST-001 v1.3 compliance
- Automatic E2E_COVERAGE=true support and disk space optimization (Podman image cleanup)
- Migrated 4 E2E test suites (DataStorage, Gateway, AuthWebhook, Notification) to use consolidated function

**Impact**: Standard parallel E2E patterns now use single source of truth for image building, reducing ~170 lines of duplicated code. Build-before-cluster optimization patterns documented for 6 additional services.

**See**: `docs/handoff/TEST_INFRASTRUCTURE_PHASE3_COMPLETE_JAN07.md` for migration details

### Version 1.3 (December 26, 2025)
**Added**:
- **Section 1.5**: Infrastructure Image Tag Format for shared services (datastorage, postgresql, redis)
- Configurable NodePort support for service-specific Data Storage deployments
- `GenerateInfraImageName()` and `deployDataStorageServiceInNamespaceWithNodePort()` functions
- Format: `localhost/{infrastructure}:{consumer}-{uuid}` (e.g., `localhost/datastorage:holmesgpt-api-1884d074`)

**Impact**: Infrastructure images (datastorage, postgresql, redis) used by E2E tests now have service-specific unique tags to prevent collisions in parallel E2E execution

### Version 1.2 (December 26, 2025)
**Added**:
- Refactored 10 services to use shared `DeployDataStorageTestServices()` function
- Eliminated ~397 lines of duplicate infrastructure code
- Standardized PostgreSQL, Redis, and Data Storage deployment across all E2E tests

**Impact**: All E2E services now use consistent infrastructure setup with automatic migration application

### Version 1.1 (December 18, 2025)
**Added**:
- **Section 4.3**: Infrastructure Image Cleanup (podman-compose + Kind) - MANDATORY for all services
- Standardized AfterSuite cleanup pattern for integration test infrastructure (podman-compose)
- Standardized AfterSuite cleanup pattern for E2E test service images (Kind)
- Label-based image pruning to prevent disk space exhaustion
- Implementation reference from WorkflowExecution service

**Impact**: All 8 services MUST implement AfterSuite cleanup for BOTH integration tests (podman-compose) AND E2E tests (Kind service images)

### Version 1.0 (December 15, 2025)
- Initial release
- Defined unique image tag format: `{service}-{user}-{git-hash}-{timestamp}`
- Specified cleanup requirements for service images
- Created shared build utilities

---

## 🎯 **Executive Summary**

All services MUST use unique container image tags for integration and E2E tests to enable parallel test execution by multiple teams on shared development hosts without image conflicts.

**Mandatory Tag Format**: `{service}-{user}-{git-hash}-{timestamp}`

**Affected Services**: Gateway, Notification, SignalProcessing, RemediationOrchestrator, WorkflowExecution, AIAnalysis, DataStorage, HAPI

---

## 📋 **Context**

### **Problem Statement**

Multiple teams run integration and E2E tests on shared development hosts, leading to container image conflicts when using static tags:

**Current Behavior (PROBLEMATIC)**:
```bash
# Team A builds notification service
make docker-build-notification
# Image: notification:latest

# Team B builds notification service (15 minutes later)
make docker-build-notification
# Image: notification:latest (OVERWRITES Team A's image)

# Team A's integration tests now fail (uses Team B's incompatible image)
```

**Impact**:
- ❌ Test failures due to image overwrites
- ❌ Unable to run tests in parallel
- ❌ "Works on my machine" debugging issues
- ❌ Difficult to inspect failed test images

### **Business Requirements**

- **BR-TEST-001**: Enable parallel test execution by multiple teams on shared hosts
- **BR-TEST-002**: Isolate test environments to prevent cross-team interference
- **BR-TEST-003**: Support debugging of failed tests via image inspection

---

## ✅ **Decision**

### **Mandatory Requirements**

#### **1. Unique Image Tag Format** (REQUIRED)

All services MUST generate unique container image tags using this format:

```
{service}-{user}-{git-hash}-{timestamp}
```

**Components**:
- `{service}`: Service name (lowercase: notification, signalprocessing, etc.)
- `{user}`: System username (output of `whoami`)
- `{git-hash}`: Short git commit hash (7 characters from `git rev-parse --short HEAD`)
- `{timestamp}`: Unix timestamp (seconds since epoch)

**Example Tags**:
```
notification-jordi-abc123f-1734278400
signalprocessing-alice-def456a-1734278401
remediationorchestrator-bob-789abcd-1734278402
```

**Rationale**:
- **Service prefix**: Identifies which service the image belongs to
- **User**: Isolates images by team member
- **Git hash**: Tracks which commit the image was built from
- **Timestamp**: Ensures uniqueness even for same user/commit

#### **1.5 Infrastructure Image Tag Format** (REQUIRED for Shared Infrastructure)

**NEW (v1.3)**: Shared infrastructure images (datastorage, postgresql, redis) used by multiple services' E2E tests MUST use a different tagging format to prevent collisions:

```
localhost/{infrastructure}:{consumer}-{uuid}
```

**Components**:
- `localhost/`: Registry prefix (required for Kind local images with ImagePullPolicy: Never)
- `{infrastructure}`: Infrastructure service name (datastorage, postgresql, redis)
- `{consumer}`: Service being tested (holmesgpt-api, gateway, signalprocessing, etc.)
- `{uuid}`: 8-character hex UUID from `time.Now().UnixNano()`

**Example Tags**:
```
localhost/datastorage:holmesgpt-api-1884d074
localhost/datastorage:gateway-a5f3c2e9
localhost/datastorage:signalprocessing-7b8d9f12
localhost/datastorage:workflowexecution-3c4e5a67
```

**Rationale**:
- **Infrastructure isolation**: Each service's E2E test gets its own infrastructure image
- **Parallel execution**: Multiple services can test concurrently without image collisions
- **Consumer tracking**: Tag indicates which service is using this infrastructure instance
- **UUID uniqueness**: Prevents collisions even when same service runs tests multiple times

**Implementation**:
```go
// test/infrastructure/datastorage_bootstrap.go

// GenerateInfraImageName generates DD-TEST-001 v1.3 compliant image name
// Returns: "localhost/{infrastructure}:{consumer}-{uuid}"
// Example: "localhost/datastorage:holmesgpt-api-1884d074"
func GenerateInfraImageName(infrastructure, consumer string) string {
    uuid := fmt.Sprintf("%x", time.Now().UnixNano())[:8]
    tag := fmt.Sprintf("%s-%s", consumer, uuid)
    return fmt.Sprintf("localhost/%s:%s", infrastructure, tag)
}

// Usage in HAPI E2E setup:
dataStorageImage := GenerateInfraImageName("datastorage", "holmesgpt-api")
// Result: "localhost/datastorage:holmesgpt-api-1884d074"

// Deploy with service-specific NodePort
err := deployDataStorageServiceInNamespaceWithNodePort(
    ctx, namespace, kubeconfigPath, dataStorageImage, 30098, writer)
```

**Kubernetes Deployment MUST use**:
```yaml
spec:
  containers:
  - name: datastorage
    image: localhost/datastorage:holmesgpt-api-1884d074  # Full image name with localhost/
    imagePullPolicy: Never  # CRITICAL: Use local image, no registry pull
```

**Configurable NodePort** (v1.3):
Different services may require different NodePorts for their E2E infrastructure:

```go
// HAPI E2E: NodePort 30098
deployDataStorageServiceInNamespaceWithNodePort(ctx, ns, kube, image, 30098, w)

// AIAnalysis E2E: NodePort 30081 (default)
deployDataStorageServiceInNamespace(ctx, ns, kube, image, w)

// Gateway E2E: NodePort 30081 (default)
deployDataStorageServiceInNamespace(ctx, ns, kube, image, w)
```

**Why Different from Service Images**:
- **Service images**: Test artifacts built per developer/commit → `{service}-{user}-{git-hash}-{timestamp}`
- **Infrastructure images**: Shared dependencies built per service → `localhost/{infrastructure}:{consumer}-{uuid}`
- **Infrastructure is reused**: Same datastorage codebase, different tags per consuming service
- **Service is unique**: Each service has its own codebase and build

**Affected Infrastructure**:
- ✅ **datastorage**: Used by HAPI, Gateway, SignalProcessing, WorkflowExecution, Notification, RemediationOrchestrator E2E
- ⚠️ **postgresql**: Currently uses static tags (future enhancement)
- ⚠️ **redis**: Currently uses static tags (future enhancement)

#### **2. Shared Build Utilities** (REQUIRED)

All services MUST use the shared build utilities to ensure consistency:

##### **Shared Makefile Include**: `.makefiles/image-build.mk`

Include this file in your service Makefile or use from root Makefile:

```makefile
# Include shared image build utilities (DD-TEST-001 compliant)
include .makefiles/image-build.mk

# Build notification service with unique tag
.PHONY: docker-build-notification
docker-build-notification:
	$(call build_service_image,notification,docker/notification-controller.Dockerfile)

# Build and load into Kind cluster
.PHONY: docker-build-notification-kind
docker-build-notification-kind: docker-build-notification
	$(call load_image_to_kind,notification,notification-test)

# Run integration tests with automatic cleanup
.PHONY: test-integration-notification
test-integration-notification: docker-build-notification
	$(call run_integration_tests_with_cleanup,notification,./test/integration/notification/...)

# Cleanup notification image
.PHONY: docker-clean-notification
docker-clean-notification:
	$(call cleanup_service_image,notification)
```

##### **Generic Build Script**: `scripts/build-service-image.sh`

Alternative to Makefile targets, use the generic build script:

```bash
# Build with auto-generated unique tag
./scripts/build-service-image.sh notification

# Build and load into Kind cluster for testing
./scripts/build-service-image.sh signalprocessing --kind

# Build with custom tag
./scripts/build-service-image.sh datastorage --tag v1.0.0

# Build for integration tests with automatic cleanup
./scripts/build-service-image.sh aianalysis --kind --cleanup

# Multi-arch build for production
./scripts/build-service-image.sh workflowexecution --multi-arch --push
```

**Override Support**:
```bash
# Use default unique tag
make test-integration-notification

# Override with custom tag
IMAGE_TAG=my-custom-tag-123 make test-integration-notification

# CI/CD usage
IMAGE_TAG=ci-${GITHUB_SHA}-${GITHUB_RUN_NUMBER} make test-integration-notification
```

**Shared Utilities Benefits**:
- ✅ Single source of truth for tag generation
- ✅ Consistent behavior across all services
- ✅ No code duplication
- ✅ Easy maintenance and updates
- ✅ Built-in cleanup automation
- ✅ Support for both Docker and Podman

#### **3. Test Infrastructure Updates** (REQUIRED)

##### **Kind Cluster Image Loading**

Update `test/integration/testenv/kind_cluster.go` and `test/e2e/testenv/kind_cluster.go`:

```go
package testenv

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// LoadServiceImageToKind loads a service's Docker image into the Kind cluster
func LoadServiceImageToKind(clusterName, serviceName string) error {
	imageTag := GetImageTag(serviceName)
	imageName := fmt.Sprintf("%s:%s", serviceName, imageTag)

	log.Printf("Loading image to Kind: %s", imageName)
	cmd := exec.Command("kind", "load", "docker-image", imageName, "--name", clusterName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to load image %s: %w\n%s", imageName, err, output)
	}

	return nil
}

// GetImageTag returns the image tag from environment or generates unique tag
func GetImageTag(serviceName string) string {
	// Check if IMAGE_TAG env var is set (from Makefile or CI)
	if tag := os.Getenv("IMAGE_TAG"); tag != "" {
		return tag
	}

	// Fallback: Generate unique tag
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}

	gitHash := getGitHash()
	timestamp := time.Now().Unix()

	return fmt.Sprintf("%s-%s-%s-%d", serviceName, user, gitHash, timestamp)
}

func getGitHash() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return string(output[:7])
}
```

##### **Kubernetes Manifest Updates**

All deployment manifests in `test/integration/manifests/` and `test/e2e/manifests/` MUST use dynamic image tags:

**Option A: Environment Variable Substitution** (Recommended)
```yaml
# test/integration/manifests/notification-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: notification
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: notification
  template:
    metadata:
      labels:
        app: notification
    spec:
      containers:
      - name: notification
        image: notification:${IMAGE_TAG}  # Substituted before apply
        imagePullPolicy: Never  # REQUIRED for Kind local images
        env:
        - name: DATA_STORAGE_URL
          value: "http://datastorage:8080"
```

**Test Setup Code**:
```go
func applyManifestWithImageTag(manifestPath, imageTag string) error {
	// Read manifest
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	// Substitute IMAGE_TAG
	manifestContent := string(manifestBytes)
	manifestContent = strings.ReplaceAll(manifestContent, "${IMAGE_TAG}", imageTag)

	// Apply to cluster
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifestContent)
	return cmd.Run()
}
```

**Option B: Dynamic Manifest Generation** (Alternative)
```go
func generateDeploymentManifest(serviceName, imageTag string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: "kubernaut-system",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": serviceName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": serviceName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            serviceName,
							Image:           fmt.Sprintf("%s:%s", serviceName, imageTag),
							ImagePullPolicy: corev1.PullNever, // REQUIRED for Kind
						},
					},
				},
			},
		},
	}
}
```

#### **4. Automatic Cleanup** (REQUIRED)

##### **Immediate Cleanup (Post-Test)**

Test targets MUST clean up images after test completion:

```makefile
.PHONY: test-integration
test-integration: docker-build
	IMAGE_TAG=$(IMAGE_TAG) go test ./test/integration/... -v || \
		(docker rmi $(SERVICE_NAME):$(IMAGE_TAG) 2>/dev/null; exit 1)
	docker rmi $(SERVICE_NAME):$(IMAGE_TAG) || true
```

##### **Periodic Cleanup (Automated)**

Create `scripts/cleanup-test-images.sh` in repository root:

```bash
#!/bin/bash
# cleanup-test-images.sh
# Removes test images older than 24 hours

set -e

SERVICES=("gateway" "notification" "signalprocessing" "remediationorchestrator" "workflowexecution" "aianalysis" "datastorage" "hapi")

echo "=== Cleaning up old test images ==="

for service in "${SERVICES[@]}"; do
    echo "Checking $service images..."

    # Find images older than 24 hours
    old_images=$(docker images --format "{{.Repository}}:{{.Tag}} {{.CreatedAt}}" | \
        grep "^$service:" | \
        awk '
            {
                # Extract timestamp
                if ($3 ~ /days?/ || ($3 ~ /hours?/ && $2 >= 24)) {
                    print $1
                }
            }
        ')

    if [ -n "$old_images" ]; then
        echo "$old_images" | xargs docker rmi 2>/dev/null || true
        echo "Cleaned up old $service images"
    else
        echo "No old $service images to clean"
    fi
done

# Prune dangling images
docker image prune -f --filter "until=24h"

echo "=== Cleanup complete ==="
```

**Cron Installation** (Optional but Recommended):
```bash
# Add to crontab (runs daily at 2 AM)
0 2 * * * /path/to/kubernaut/scripts/cleanup-test-images.sh >> /var/log/test-image-cleanup.log 2>&1
```

##### **Infrastructure Image Cleanup (podman-compose + Kind)** (REQUIRED v1.1)

**NEW REQUIREMENT**: All services MUST clean up test images in `AfterSuite` for BOTH test tiers:
1. **Integration Tests**: podman-compose infrastructure (postgres, redis, datastorage, etc.)
2. **E2E Tests**: Kind service images (built and loaded into Kind cluster)

**Why Required**:
- Infrastructure images (postgres, redis, service builds) accumulate rapidly
- Service images built for Kind (per tag format) consume 200-500MB each
- Multi-team parallel testing creates multiple infrastructure stacks and service images
- Disk space exhaustion blocks test execution (common with 10+ test runs)
- Manual cleanup is error-prone and forgotten

**Implementation Pattern** (Add to `test/integration/{service}/suite_test.go`):

```go
var _ = AfterSuite(func() {
	By("Tearing down the test environment")

	cancel()

	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	By("Stopping infrastructure (podman-compose)")
	// Stop podman-compose services (postgres, redis, datastorage, etc.)
	// Determine absolute path for parallel test safety
	testDir, pathErr := filepath.Abs(filepath.Join(".", "..", "..", ".."))
	if pathErr != nil {
		GinkgoWriter.Printf("⚠️  Failed to determine project root: %v\n", pathErr)
	} else {
		cmd := exec.Command("podman-compose", "-f", "podman-compose.test.yml", "down")
		cmd.Dir = filepath.Join(testDir, "test", "integration", "{service}")
		output, cmdErr := cmd.CombinedOutput()
		if cmdErr != nil {
			GinkgoWriter.Printf("⚠️  Failed to stop containers: %v\n%s\n", cmdErr, output)
		} else {
			GinkgoWriter.Println("✅ Infrastructure stopped")
		}
	}

	By("Cleaning up infrastructure images to prevent disk space issues")
	// Prune ONLY infrastructure images for this service's tests
	// Uses podman-compose project label for targeted cleanup
	pruneCmd := exec.Command("podman", "image", "prune", "-f",
		"--filter", "label=io.podman.compose.project={service}")
	pruneOutput, pruneErr := pruneCmd.CombinedOutput()
	if pruneErr != nil {
		GinkgoWriter.Printf("⚠️  Failed to prune images: %v\n%s\n", pruneErr, pruneOutput)
	} else {
		GinkgoWriter.Println("✅ Infrastructure images pruned")
	}

	GinkgoWriter.Println("✅ Cleanup complete")
})
```

**BeforeSuite Cleanup** (Also add for failed previous runs):

```go
var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("Cleaning up stale containers from previous runs")
	// Stop any existing containers from failed previous runs
	// Prevents port conflicts and "address already in use" errors
	testDir, err := filepath.Abs(filepath.Join(".", "..", "..", ".."))
	if err != nil {
		GinkgoWriter.Printf("⚠️  Failed to determine project root: %v\n", err)
	} else {
		cleanupCmd := exec.Command("podman-compose", "-f", "podman-compose.test.yml", "down")
		cleanupCmd.Dir = filepath.Join(testDir, "test", "integration", "{service}")
		_, cleanupErr := cleanupCmd.CombinedOutput()
		if cleanupErr != nil {
			GinkgoWriter.Printf("⚠️  Cleanup of stale containers failed (may not exist): %v\n", cleanupErr)
		} else {
			GinkgoWriter.Println("✅ Stale containers cleaned up")
		}
	}

	// ... continue with normal BeforeSuite setup
})
```

**Label-Based Filtering Explanation**:

The `--filter label=io.podman.compose.project={service}` ensures:
- ✅ Only images from THIS service's podman-compose stack are removed
- ✅ Parallel test runs for other services are unaffected
- ✅ Base images (postgres:16-alpine, redis:7-alpine) are preserved if shared
- ✅ Service-specific built images (e.g., `{service}_datastorage:latest`) are removed

**Label Format by Container Engine**:
- **Docker Compose**: `com.docker.compose.project={service}`
- **Podman Compose**: `io.podman.compose.project={service}`

**E2E Test Cleanup Pattern** (Kind Service Images):

```go
var _ = AfterSuite(func() {
	By("Tearing down Kind cluster")
	// ... existing Kind cluster teardown ...

	By("Cleaning up service images built for Kind")
	// Remove service image built for this test run
	// Uses unique tag format from DD-TEST-001: {service}-{user}-{git-hash}-{timestamp}
	imageTag := os.Getenv("IMAGE_TAG")  // Set by test infrastructure
	if imageTag != "" {
		serviceName := "{service}"  // e.g., "notification", "gateway", etc.
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

**Why Both Patterns Required**:
- **Integration tests**: Clean infrastructure (postgres, redis, datastorage) - ~500MB-1GB per run
- **E2E tests**: Clean service images (built per unique tag) - ~200-500MB per run
- **Combined impact**: 10 test runs = ~7-15GB disk space saved

**Services MUST Implement**:

All services MUST add cleanup for BOTH test tiers:

1. ✅ **WorkflowExecution** - IMPLEMENTED (Integration reference)
2. ⏳ **DataStorage** - Integration: postgres/redis, E2E: datastorage service image
3. ⏳ **AIAnalysis** - Integration: LLM/redis, E2E: aianalysis service image
4. ⏳ **Gateway** - Integration: service mocks, E2E: gateway service image
5. ⏳ **Notification** - Integration: datastorage/redis, E2E: notification service image
6. ⏳ **SignalProcessing** - Integration: datastorage/redis, E2E: signalprocessing service image
7. ⏳ **RemediationOrchestrator** - Integration: datastorage, E2E: remediationorchestrator service image
8. ⏳ **HAPI** - Integration: datastorage/redis, E2E: hapi service image

**Verification Commands**:

```bash
# Before cleanup (after test run) - should show containers/images
cd test/integration/{service}
podman-compose -f podman-compose.test.yml ps
podman images | grep -E "{service}_|postgres|redis"

# Run tests with cleanup
cd ../../..
make test-integration-{service}

# After cleanup - should be empty
cd test/integration/{service}
podman-compose -f podman-compose.test.yml ps
podman images | grep "io.podman.compose.project={service}"
```

**Performance Impact**:
- **Container Stop**: ~5 seconds (podman-compose down)
- **Image Prune**: ~2 seconds (filtered prune)
- **Total Overhead**: ~7 seconds per test run
- **Trade-off**: Acceptable overhead to prevent disk space exhaustion

**Disk Space Savings**:
- **Per Test Run**: ~500MB-1GB (depends on service infrastructure)
- **Daily (10 runs)**: ~5-10GB prevented accumulation
- **Weekly (50 runs)**: ~25-50GB prevented accumulation

---

## 🚀 **Implementation Plan**

### **Phase 1: Infrastructure Setup** (Week 1)

#### **1.1: Create Shared Build Utilities** (Day 1) ✅ COMPLETE
- [x] Create `.makefiles/image-build.mk` with tag generation logic
- [x] Create `scripts/build-service-image.sh` generic build script
- [x] Make script executable
- [x] Update DD-TEST-001 documentation
- [x] Test utilities with notification service

**Deliverables**:
- `.makefiles/image-build.mk` - Shared Makefile functions
- `scripts/build-service-image.sh` - Generic build script
- All services can use same utilities (no duplication)

#### **1.2: Update Test Environment** (Day 1-2)
- [ ] Update `test/integration/testenv/kind_cluster.go`
- [ ] Update `test/e2e/testenv/kind_cluster.go`
- [ ] Add `GetImageTag()` helper function
- [ ] Test image loading with unique tags

#### **1.3: Update Kubernetes Manifests** (Day 2)
- [ ] Update all `test/integration/manifests/*.yaml` files
- [ ] Update all `test/e2e/manifests/*.yaml` files
- [ ] Add `${IMAGE_TAG}` substitution points
- [ ] Set `imagePullPolicy: Never` for Kind compatibility

#### **1.4: Create Cleanup Automation** (Day 2)
- [ ] Create `scripts/cleanup-test-images.sh`
- [ ] Make executable: `chmod +x scripts/cleanup-test-images.sh`
- [ ] Test cleanup script with old images
- [ ] Document cron installation (optional)

### **Phase 2: Service-by-Service Migration** (Week 1-2)

Each service team MUST complete:

#### **Service Checklist**:
- [ ] Update service-specific `Makefile` (if exists)
- [ ] Verify `docker-build` target uses `IMAGE_TAG`
- [ ] Run integration tests with unique tag: `make test-integration`
- [ ] Run E2E tests with unique tag: `make test-e2e`
- [ ] Verify cleanup: Check `docker images` after tests
- [ ] Update service documentation in `docs/services/`

**Services to Migrate**:
1. ✅ Gateway (`cmd/gateway/`)
2. ✅ Notification (`cmd/notification/`)
3. ✅ SignalProcessing (`cmd/signalprocessing/`)
4. ✅ RemediationOrchestrator (`cmd/remediationorchestrator/`)
5. ✅ WorkflowExecution (`cmd/workflowexecution/`)
6. ✅ AIAnalysis (`cmd/aianalysis/`)
7. ✅ DataStorage (`cmd/datastorage/`)
8. ✅ HAPI (`cmd/hapi/`)

### **Phase 3: CI/CD Integration** (Week 2)

#### **3.1: Update GitHub Actions** (if applicable)
```yaml
# .github/workflows/integration-tests.yml
name: Integration Tests

on: [push, pull_request]

env:
  IMAGE_TAG: ci-${{ github.sha }}-${{ github.run_number }}

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Build Docker Image
        run: make docker-build TAG=${{ env.IMAGE_TAG }}

      - name: Run Integration Tests
        run: make test-integration TAG=${{ env.IMAGE_TAG }}

      - name: Cleanup
        if: always()
        run: docker rmi $(SERVICE_NAME):${{ env.IMAGE_TAG }} || true
```

#### **3.2: Update Documentation**
- [ ] Update `docs/development/testing.md`
- [ ] Update `README.md` with new test commands
- [ ] Add troubleshooting guide for image issues
- [ ] Document cleanup automation

### **Phase 4: Validation** (Week 2)

#### **Multi-Team Parallel Test**:
```bash
# Terminal 1 (Team A - Notification)
cd /path/to/kubernaut
make test-integration-notification

# Terminal 2 (Team B - SignalProcessing)
cd /path/to/kubernaut
make test-integration-signalprocessing

# Terminal 3 (Team C - RemediationOrchestrator)
cd /path/to/kubernaut
make test-integration-remediationorchestrator

# Verify: All three should run concurrently without conflicts
docker images | grep -E "notification|signalprocessing|remediationorchestrator"
```

**Success Criteria**:
- ✅ All three test runs complete successfully
- ✅ Three unique image tags exist simultaneously
- ✅ No "image not found" or "image mismatch" errors
- ✅ Images are cleaned up after tests

---

## 📊 **Consequences**

### **Positive**

1. ✅ **Test Isolation**: Teams can run tests in parallel without interference
2. ✅ **Debugging Support**: Failed test images remain available for inspection
3. ✅ **CI/CD Compatibility**: Natural integration with GitHub Actions/GitLab CI
4. ✅ **Traceability**: Image tags track user, commit, and build time
5. ✅ **Reduced Conflicts**: Eliminates "works on my machine" issues

### **Negative**

1. ⚠️ **Disk Space**: Unique tags consume more disk space
   - **Mitigation**: Automatic cleanup after tests + periodic cleanup script

2. ⚠️ **Complexity**: Additional Makefile and test infrastructure logic
   - **Mitigation**: Centralized in shared test utilities

3. ⚠️ **Migration Effort**: All services need updates
   - **Mitigation**: Phased migration plan, service-by-service

### **Risks**

#### **Risk 1: Disk Space Exhaustion** (MEDIUM)
**Impact**: Host runs out of disk space from accumulated test images

**Mitigation**:
- Immediate cleanup after each test run
- Periodic cleanup script (daily cron job)
- Manual cleanup: `docker image prune -f --filter "until=24h"`
- Monitoring: Alert if disk usage > 80%

#### **Risk 2: Cleanup Failure** (LOW)
**Impact**: Images not cleaned up, disk space accumulates

**Mitigation**:
- `|| true` in cleanup commands (non-blocking)
- Separate periodic cleanup script as backup
- Manual intervention instructions in documentation

#### **Risk 3: Tag Collision** (VERY LOW)
**Impact**: Two teams generate identical tags

**Probability**: < 0.01% (user + git-hash + timestamp provides strong uniqueness)

**Mitigation**:
- Timestamp provides 1-second granularity
- Git hash provides commit-level uniqueness
- User provides team-level isolation
- If needed: Add UUID component (`uuidgen | cut -d'-' -f1`)

---

## 🔧 **Technical Details**

### **Tag Format Specification**

```
Format:     {service}-{user}-{git-hash}-{timestamp}
Regex:      ^[a-z]+-([\w]+)-([0-9a-f]{7})-(\d{10})$
Max Length: 253 characters (Docker limit)

Examples:
- notification-jordi-abc123f-1734278400
- signalprocessing-alice-def456a-1734278401
- remediationorchestrator-bob-789abcd-1734278402

Components:
- service:   [a-z]+ (lowercase service name)
- user:      [\w]+ (alphanumeric + underscore)
- git-hash:  [0-9a-f]{7} (7-char hex from git rev-parse --short HEAD)
- timestamp: \d{10} (10-digit Unix timestamp in seconds)
```

### **Docker Image Naming Conventions**

**Repository**: Service name (lowercase)
**Tag**: Unique tag from format above

```bash
# Good:
notification:notification-jordi-abc123f-1734278400
signalprocessing:signalprocessing-alice-def456a-1734278401

# Bad (DO NOT USE):
notification:latest
notification:dev
notification:test
```

### **Kind Cluster Image Loading**

**CRITICAL**: Images MUST use `imagePullPolicy: Never` in Kind:

```yaml
spec:
  containers:
  - name: notification
    image: notification:notification-jordi-abc123f-1734278400
    imagePullPolicy: Never  # REQUIRED: Use local image, don't pull from registry
```

**Why**: Kind cluster has local Docker registry; `Never` ensures it uses the loaded image instead of attempting external pull.

### **Environment Variable Substitution**

**Supported in Test Code**:
```go
imageTag := os.Getenv("IMAGE_TAG")  // Set by Makefile
```

**Supported in Shell**:
```bash
export IMAGE_TAG=my-custom-tag
make test-integration  # Uses IMAGE_TAG from environment
```

**Supported in Kubernetes Manifests**:
```yaml
image: notification:${IMAGE_TAG}  # Substituted by test code before kubectl apply
```

---

## 📋 **Compliance Requirements**

### **Mandatory (MUST)**

1. ✅ All services MUST generate unique image tags using specified format
2. ✅ All `docker-build` targets MUST support `TAG` environment variable override
3. ✅ All test targets MUST clean up images after test completion
4. ✅ All Kubernetes manifests MUST use `imagePullPolicy: Never` for Kind
5. ✅ All test infrastructure MUST read `IMAGE_TAG` from environment

### **Recommended (SHOULD)**

1. ✅ Services SHOULD install periodic cleanup cron job
2. ✅ Services SHOULD document image tag format in service README
3. ✅ CI/CD pipelines SHOULD use `ci-{sha}-{run}` format
4. ✅ Teams SHOULD verify cleanup after manual test runs

### **Optional (MAY)**

1. ⚠️ Services MAY add UUID to tag format for absolute uniqueness
2. ⚠️ Services MAY customize tag format if business need exists
3. ⚠️ Services MAY implement custom cleanup logic beyond standard script

---

## 📦 **Section 5: Consolidated E2E Image Build Functions** (v1.4, January 7, 2026)

### **Overview**

All E2E tests that use **standard parallel patterns** (build+load together after cluster creation) MUST use the consolidated `BuildAndLoadImageToKind()` function. This eliminates code duplication and ensures DD-TEST-001 v1.3 compliance.

### **Consolidated Function**

**File**: `test/infrastructure/datastorage_bootstrap.go`

```go
// E2EImageConfig configures image building and loading for E2E tests
type E2EImageConfig struct {
    ServiceName      string // Service name (e.g., "gateway", "datastorage")
    ImageName        string // Base image name (e.g., "kubernaut/datastorage")
    DockerfilePath   string // Relative to project root (e.g., "docker/data-storage.Dockerfile")
    KindClusterName  string // Kind cluster name to load image into
    BuildContextPath string // Build context path, default: "." (project root)
    EnableCoverage   bool   // Enable Go coverage instrumentation (--build-arg GOFLAGS=-cover)
}

// BuildAndLoadImageToKind builds a service image and loads it into Kind cluster
// Features:
//   - DD-TEST-001 v1.3 compliant image tagging
//   - DD-TEST-007 E2E coverage instrumentation support
//   - Automatic Podman image cleanup after Kind load (disk space optimization)
func BuildAndLoadImageToKind(cfg E2EImageConfig, writer io.Writer) (string, error)
```

### **Usage Pattern**

```go
// Standard Parallel E2E Setup (build+load after cluster creation)
go func() {
    cfg := E2EImageConfig{
        ServiceName:      "datastorage",
        ImageName:        "kubernaut/datastorage",
        DockerfilePath:   "docker/data-storage.Dockerfile",
        KindClusterName:  clusterName,
        BuildContextPath: ".",
        EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
    }
    _, err := BuildAndLoadImageToKind(cfg, writer)
    results <- result{name: "DS image", err: err}
}()
```

### **Migrated Services**

- DataStorage: `SetupDataStorageInfrastructureParallel()`
- Gateway: `SetupGatewayInfrastructureParallel()` (2 occurrences)
- AuthWebhook: `SetupAuthWebhookInfrastructureParallel()`
- Notification: `SetupNotificationInfrastructure()`

**Code Reduction**: ~170 lines eliminated

### **Build-Before-Cluster Optimization Pattern**

Some services build images BEFORE creating the Kind cluster (optimization). These use separate `buildDataStorageImageWithTag()` and `loadDataStorageImageWithTag()` functions and should NOT be migrated.

**Services**: Gateway (coverage), SignalProcessing, WorkflowExecution, RemediationOrchestrator

**See**: `docs/handoff/TEST_INFRASTRUCTURE_PHASE3_COMPLETE_JAN07.md` for details

---

## 🌐 **Section 6: Container Networking Patterns for Integration Tests** (v1.5, January 8, 2026)

### **Overview**

Integration tests using containerized infrastructure (PostgreSQL, Redis, DataStorage, HAPI) require correct container networking configuration to enable communication between services. Incorrect networking configuration causes HTTP 500 errors and connection failures.

**Discovered**: January 8, 2026 during AIAnalysis integration test debugging  
**Root Cause**: HAPI unable to reach DataStorage due to incorrect networking configuration  
**Impact**: All integration tests using container-to-container communication

---

### 📋 **Problem Statement**

**Symptom**: HTTP 500 errors from HolmesGPT-API (HAPI) during integration tests
```
HolmesGPT-API error (HTTP 500): HolmesGPT-API returned HTTP 500: 
decode response: unexpected status code: 500
```

**Root Cause**: HAPI configured with incorrect DataStorage URL
```go
// ❌ INCORRECT CONFIGURATION
Env: map[string]string{
    "DATA_STORAGE_URL": "http://host.containers.internal:18095", // WRONG!
}
```

**Why This Failed**:
1. `host.containers.internal` resolves to the HOST machine, not other containers
2. DataStorage runs in a container on the same podman network, not on host
3. DataStorage exposes port **18095 on HOST**, but listens on **8080 internally**
4. Port mapping: `18095:8080` (host:container)
5. Result: Connection refused → HTTP 500

---

### 🎯 **Container Networking Decision Matrix**

| Communication Type | Source | Target | URL Pattern | Example |
|-------------------|--------|--------|-------------|---------|
| **Container → Container** | HAPI container | DataStorage container | `http://{container_name}:{internal_port}` | `http://aianalysis_datastorage_test:8080` ✅ |
| **Container → Host** | Any container | Host service | `http://host.containers.internal:{host_port}` | `http://host.containers.internal:18095` ✅ |
| **Host → Container** | Test code | Container service | `http://localhost:{host_port}` | `http://localhost:18095` ✅ |
| **Container → Container (WRONG)** | HAPI container | DataStorage container | `http://host.containers.internal:{port}` | `http://host.containers.internal:18095` ❌ |

---

### ✅ **Container-to-Container Communication (CORRECT PATTERN)**

When both containers are on the **same podman network**, use container names for DNS resolution:

#### **Correct Configuration**
```go
// test/integration/aianalysis/suite_test.go
hapiConfig := infrastructure.GenericContainerConfig{
    Name:    "aianalysis_hapi_test",
    Network: "aianalysis_test_network", // Same network as DataStorage
    Ports:   map[int]int{8080: 18120},  // container:host
    Env: map[string]string{
        "DATA_STORAGE_URL": "http://aianalysis_datastorage_test:8080", // ✅ CORRECT
        "MOCK_LLM_MODE":    "true",
        "PORT":             "8080",
    },
}
```

#### **Why This Works**
1. **Container name resolution**: `aianalysis_datastorage_test` resolves via podman DNS
2. **Correct port**: `8080` is DataStorage's **internal** listening port
3. **Same network**: Both containers on `aianalysis_test_network`
4. **Direct communication**: No host traversal, faster and more reliable

#### **Container Network Topology**
```
┌─────────────────────────────────────────────────┐
│ Host Machine (macOS/Linux)                      │
│                                                 │
│  Port 18095 → DataStorage Container (port 8080)│
│  Port 18120 → HAPI Container (port 8080)       │
│                                                 │
│  ┌─────────────────────────────────────────┐  │
│  │ aianalysis_test_network (podman)        │  │
│  │                                         │  │
│  │  ┌───────────────────────────────┐     │  │
│  │  │ aianalysis_datastorage_test   │     │  │
│  │  │ - Internal port: 8080         │     │  │
│  │  │ - Host port: 18095            │     │  │
│  │  └───────────────────────────────┘     │  │
│  │                ↑                        │  │
│  │                │ ✅ Container DNS      │  │
│  │                │ name:8080             │  │
│  │  ┌─────────────┴─────────────────┐     │  │
│  │  │ aianalysis_hapi_test          │     │  │
│  │  │ - Internal port: 8080         │     │  │
│  │  │ - Host port: 18120            │     │  │
│  │  │ - ENV: DATA_STORAGE_URL=      │     │  │
│  │  │   http://aianalysis_          │     │  │
│  │  │   datastorage_test:8080       │     │  │
│  │  └───────────────────────────────┘     │  │
│  └─────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

---

### 🔧 **Port Mapping: Internal vs External**

Understanding port mappings is critical for correct configuration:

#### **DataStorage Bootstrap Port Mapping**
```go
// test/infrastructure/datastorage_bootstrap.go (line ~440)
"-p", fmt.Sprintf("%d:8080", cfg.DataStoragePort)
// Result: "-p 18095:8080" (host:container)
```

| Port Type | Port Number | Used By | Access Pattern |
|-----------|-------------|---------|----------------|
| **Internal** | `8080` | Container services | `http://{container_name}:8080` |
| **External (Host)** | `18095` | Test code on host | `http://localhost:18095` |

**Rule**: Container-to-container communication ALWAYS uses **internal ports**, never external ports.

---

### 🚫 **Anti-Patterns (DO NOT USE)**

#### **❌ Anti-Pattern 1: Using host.containers.internal for Container-to-Container**
```go
// ❌ WRONG: Tries to reach host, not other containers
Env: map[string]string{
    "DATA_STORAGE_URL": "http://host.containers.internal:18095",
}
```

#### **❌ Anti-Pattern 2: Using External Port for Container Communication**
```go
// ❌ WRONG: 18095 is HOST port, not container port
Env: map[string]string{
    "DATA_STORAGE_URL": "http://aianalysis_datastorage_test:18095",
}
```

#### **❌ Anti-Pattern 3: Using localhost for Container-to-Container**
```go
// ❌ WRONG: localhost inside container refers to the container itself
Env: map[string]string{
    "DATA_STORAGE_URL": "http://localhost:18095",
}
```

---

### 🔍 **Validation Commands**

#### **Step 1: Verify Container DNS Resolution**
```bash
# From inside HAPI container, resolve DataStorage container name
podman exec aianalysis_hapi_test nslookup aianalysis_datastorage_test

# Expected output:
# Server:    10.88.0.1
# Address:   10.88.0.1:53
# Name:      aianalysis_datastorage_test
# Address:   10.88.0.X  ← Container IP on aianalysis_test_network
```

#### **Step 2: Test Container-to-Container Connectivity**
```bash
# From inside HAPI container, test DataStorage health endpoint
podman exec aianalysis_hapi_test curl -v http://aianalysis_datastorage_test:8080/health

# Expected: HTTP 200 with {"status":"healthy",...}
```

#### **Step 3: Verify Port Mappings**
```bash
# Check container port mappings
podman ps --filter "name=aianalysis" --format "{{.Names}}: {{.Ports}}"

# Expected output:
# aianalysis_datastorage_test: 0.0.0.0:18095->8080/tcp
# aianalysis_hapi_test: 0.0.0.0:18120->8080/tcp
```

#### **Step 4: Test Host-to-Container Connectivity**
```bash
# From host machine, test DataStorage via external port
curl http://localhost:18095/health

# Expected: HTTP 200 with {"status":"healthy",...}
```

---

### 📚 **Reference Implementation**

#### **AIAnalysis Integration Test Suite**
**File**: `test/integration/aianalysis/suite_test.go` (lines 157-179)

**Before (Incorrect)**:
```go
Env: map[string]string{
    "DATA_STORAGE_URL": "http://host.containers.internal:18095", // ❌
},
```

**After (Correct)**:
```go
Env: map[string]string{
    "DATA_STORAGE_URL": "http://aianalysis_datastorage_test:8080", // ✅
},
```

**Commit**: `fe0f76adf` - "fix(aa-integration): Use container-to-container URL for DataStorage"

---

### 🎓 **When to Use Each Pattern**

#### **Use Container Names (Container-to-Container)**
- ✅ Service A in container needs to reach Service B in container
- ✅ Both services on same podman network
- ✅ Integration tests with multiple containerized services
- ✅ Example: HAPI → DataStorage, Service → PostgreSQL

**Format**: `http://{container_name}:{internal_port}`

#### **Use host.containers.internal (Container-to-Host)**
- ✅ Container needs to reach a service running on HOST machine
- ✅ Service is NOT containerized
- ✅ Example: Container → IDE debugger, Container → Host database

**Format**: `http://host.containers.internal:{host_port}`

#### **Use localhost (Host-to-Container)**
- ✅ Test code running on host needs to reach containerized service
- ✅ Using external (mapped) port
- ✅ Example: Go test code → DataStorage HTTP API

**Format**: `http://localhost:{external_port}`

---

### 🧪 **Testing Checklist**

When configuring container networking in integration tests:

- [ ] **Identify communication type**: Container→Container, Container→Host, or Host→Container
- [ ] **Use correct URL pattern** from decision matrix above
- [ ] **Use internal ports** for container-to-container (e.g., 8080, not 18095)
- [ ] **Use external ports** for host-to-container (e.g., 18095, not 8080)
- [ ] **Verify both containers on same network** (check Network config)
- [ ] **Test DNS resolution** with `nslookup` inside container
- [ ] **Test connectivity** with `curl` inside container
- [ ] **Document in suite_test.go** why specific URL pattern was chosen

---

### 🐛 **Debugging Container Networking Issues**

#### **Symptom: HTTP 500 errors or "connection refused"**

**Step 1: Check container network**
```bash
podman inspect {container_name} | grep -A 5 "Networks"
```

**Step 2: Test DNS resolution**
```bash
podman exec {container_name} nslookup {target_container_name}
```

**Step 3: Test connectivity with verbose output**
```bash
podman exec {container_name} curl -v http://{target_container}:{port}/health
```

**Step 4: Check port mappings**
```bash
podman ps --filter "name={service}" --format "{{.Names}}: {{.Ports}}"
```

**Step 5: Verify environment variables**
```bash
podman exec {container_name} env | grep URL
```

---

### 📊 **Impact and Affected Services**

#### **Version 1.5 Scope**
- ✅ **AIAnalysis**: Fixed container networking in integration tests (January 8, 2026)
- ⏳ **Future Services**: All services using containerized dependencies in integration tests

#### **Expected Benefits**
- ✅ Eliminates HTTP 500 errors from incorrect container networking
- ✅ Faster container-to-container communication (no host traversal)
- ✅ Clear debugging procedures for networking issues
- ✅ Authoritative reference for future integration test development

#### **Performance Impact**
- Container-to-container: ~1-2ms latency (direct network communication)
- Container-to-host-to-container: ~5-10ms latency (unnecessary host traversal)
- **Improvement**: ~5-8ms faster per request with correct pattern

---

### 🔗 **Related Documentation**

- **RCA Document**: `docs/handoff/AA_INTEGRATION_HTTP500_FIX_JAN08.md` (detailed root cause analysis)
- **DataStorage Bootstrap**: `test/infrastructure/datastorage_bootstrap.go` (reference implementation)
- **DD-TEST-002**: Integration Test Container Orchestration (deprecated, see DD-INTEGRATION-001)
- **Podman Networking**: https://docs.podman.io/en/latest/markdown/podman-network.1.html

---

## 🔗 **Related Documents**

### **Architecture Decisions**
- ADR-034: Unified Audit Table Design (audit event tracing)
- ADR-XXX: API Group Migration Strategy (service integration patterns)

### **Design Decisions**
- DD-AUDIT-003: Service Audit Trace Requirements (cross-service patterns)
- DD-AUDIT-002: Audit Architecture Refactoring (shared library usage)
- DD-TEST-007: E2E Coverage Collection (coverage instrumentation)

### **Testing Documentation**
- `docs/development/testing.md`: Testing strategy and patterns
- `docs/services/crd-controllers/*/README.md`: Service-specific testing
- `.github/workflows/*.yml`: CI/CD pipeline configurations
- `docs/handoff/TEST_INFRASTRUCTURE_PHASE3_COMPLETE_JAN07.md`: Phase 3 migration results

### **Business Requirements**
- BR-TEST-001: Parallel test execution capability
- BR-TEST-002: Test environment isolation
- BR-TEST-003: Failed test debugging support

---

## 📊 **Metrics and Monitoring**

### **Success Metrics**

1. **Test Conflict Rate**: Should be 0% after migration
   - **Before**: ~15% of test runs fail due to image conflicts
   - **Target**: 0% test runs fail due to image conflicts

2. **Parallel Test Capacity**: Number of concurrent test runs
   - **Before**: 1 test run at a time
   - **Target**: Unlimited (resource-bound only)

3. **Disk Space Usage**: Aggregate test image storage
   - **Monitor**: Daily disk usage on shared hosts
   - **Alert**: If usage > 80% of available space
   - **Action**: Manual cleanup + review cleanup automation

4. **Cleanup Effectiveness**: Percentage of images cleaned up
   - **Target**: 100% of test images removed within 24 hours
   - **Monitor**: Weekly audit of old test images

### **Monitoring Commands**

```bash
# Check current test images
docker images | grep -E "gateway|notification|signalprocessing|remediationorchestrator|workflowexecution|aianalysis|datastorage|hapi"

# Check disk usage
df -h /var/lib/docker

# Count test images by service
for service in gateway notification signalprocessing remediationorchestrator workflowexecution aianalysis datastorage hapi; do
    count=$(docker images | grep "^$service" | wc -l)
    echo "$service: $count images"
done

# Find old test images (>24 hours)
docker images --format "{{.Repository}}:{{.Tag}} {{.CreatedAt}}" | \
    awk '$3 ~ /days?/ || ($3 ~ /hours?/ && $2 >= 24) {print}'
```

---

## ✅ **Approval and Status**

**Decision Status**: ✅ APPROVED

**Approved By**: Platform Team
**Approval Date**: December 15, 2025

**Implementation Status**: 🚧 IN PROGRESS

**Target Completion**: December 22, 2025 (Week 2)

**Migration Status by Service**:
- [ ] Gateway: Pending
- [ ] Notification: Pending
- [ ] SignalProcessing: Pending
- [ ] RemediationOrchestrator: Pending
- [ ] WorkflowExecution: Pending
- [ ] AIAnalysis: Pending
- [ ] DataStorage: Pending
- [ ] HAPI: Pending

---

## 📞 **Support and Questions**

**Questions**: Contact Platform Team or open issue in GitHub

**Migration Support**: Platform team available for pairing on service migration

**Troubleshooting**: See `docs/development/testing.md` troubleshooting section

---

**Document Version**: 1.5
**Last Updated**: January 8, 2026
**Previous Versions**:
- 1.4 (January 7, 2026) - Consolidated E2E image build functions
- 1.3 (December 26, 2025) - Infrastructure image tag format
- 1.2 (December 26, 2025) - Infrastructure refactoring
- 1.1 (December 18, 2025) - Cleanup patterns
- 1.0 (December 15, 2025) - Initial release
**Next Review**: April 8, 2026 (3 months post-v1.5)

