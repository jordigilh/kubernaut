# Notification E2E DataStorage Image Name Mismatch - Root Cause

**Date**: December 27, 2025
**Status**: ✅ **ROOT CAUSE IDENTIFIED**
**Severity**: 🔴 **CRITICAL** - Blocks E2E test execution

---

## 🎯 **Triage Summary**

**User Question**: "triage if the problem with the datastorage is due to the changes we made with the additional shared functions and the tag name"

**Answer**: ❌ **NO** - The issue is NOT caused by:
- ✅ Shared utility functions (those are for integration tests, not E2E)
- ✅ User's recent env var changes to `shared_integration_utils.go` (Podman-based, not Kind)
- ✅ Composite tag naming strategy (concept is correct)

**Actual Root Cause**: **Image Name Mismatch** between build and deployment

---

## 🔍 **Root Cause Analysis**

### **The Problem**

DataStorage pod fails readiness check (300s timeout) because the image doesn't exist in Kind cluster.

### **Image Name Mismatch**

**What Gets Built**:
```bash
# In test/infrastructure/notification.go:345
buildDataStorageImage(writer)
  ↓
# Builds: localhost/kubernaut-datastorage:e2e-test-datastorage
# Also tags: localhost/kubernaut-datastorage:e2e-test
```

**What Gets Deployed**:
```bash
# In test/infrastructure/notification.go:358
DeployDataStorageTestServices(ctx, namespace, kubeconfigPath,
    GenerateInfraImageName("datastorage", "notification"), writer)
  ↓
# GenerateInfraImageName("datastorage", "notification") returns:
# localhost/datastorage:notification-<uuid>
# Example: localhost/datastorage:notification-1a2b3c4d
```

**The Mismatch**:
```
BUILT:    localhost/kubernaut-datastorage:e2e-test-datastorage
DEPLOYED: localhost/datastorage:notification-1a2b3c4d
                    ^^^^^^^^               ^^^^^^^^^^^
                    DIFFERENT              DIFFERENT
```

---

## 🔧 **Technical Details**

### **Build Function** (Wrong One Used)

```go
// test/infrastructure/datastorage.go:1154
func buildDataStorageImage(writer io.Writer) error {
    buildArgs := []string{
        "build",
        "--no-cache",
        "-t", "localhost/kubernaut-datastorage:e2e-test-datastorage", // HARDCODED
        "-f", "docker/data-storage.Dockerfile",
    }
    // ...
}
```

### **Correct Function** (Should Be Used)

```go
// test/infrastructure/datastorage.go:1790
func buildDataStorageImageWithTag(imageTag string, writer io.Writer) error {
    buildArgs := []string{
        "build",
        "--no-cache",
        "-t", imageTag, // DYNAMIC TAG (matches deployment)
        "-f", "docker/data-storage.Dockerfile",
    }
    // ...
}
```

### **Tag Generation**

```go
// test/infrastructure/datastorage_bootstrap.go:62
func generateInfrastructureImageTag(infrastructure, consumer string) string {
    uuid := fmt.Sprintf("%x", time.Now().UnixNano())[:8]
    return fmt.Sprintf("%s-%s", consumer, uuid)
    //                  ^^^^^^^^  ^^^^
    //                  consumer  uuid
}

// GenerateInfraImageName combines infrastructure name + generated tag
func GenerateInfraImageName(infrastructure, consumer string) string {
    tag := generateInfrastructureImageTag(infrastructure, consumer)
    return fmt.Sprintf("localhost/%s:%s", infrastructure, tag)
    //                              ^^^^^^^^^^^^^^
    //                              infrastructure name
}

// For notification:
// GenerateInfraImageName("datastorage", "notification")
// → localhost/datastorage:notification-1a2b3c4d
```

---

## 💥 **Why Pod Fails**

1. **Image Build**: Creates `localhost/kubernaut-datastorage:e2e-test-datastorage`
2. **Image Load**: Loads above image into Kind cluster
3. **Pod Deployment**: Tries to use `localhost/datastorage:notification-1a2b3c4d`
4. **Image Pull**: Fails - image doesn't exist
5. **Pod Status**: `ImagePullBackOff` or `ErrImagePull`
6. **Readiness**: Never succeeds → 300s timeout → test fails

---

## ✅ **Solution**

### **Option A: Use Correct Build Function** (RECOMMENDED)

Change Notification E2E to use `buildDataStorageImageWithTag()`:

```go
// test/infrastructure/notification.go:343-358
func DeployNotificationAuditInfrastructure(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
    // Generate consistent image name
    dataStorageImage := GenerateInfraImageName("datastorage", "notification")

    // 1. Build with correct tag
    fmt.Fprintf(writer, "🔨 Building Data Storage image with tag: %s\n", dataStorageImage)
    if err := buildDataStorageImageWithTag(dataStorageImage, writer); err != nil {
        return fmt.Errorf("failed to build Data Storage image: %w", err)
    }

    // 2. Load image into Kind
    clusterName := "notification-e2e"
    fmt.Fprintf(writer, "📦 Loading Data Storage image into Kind cluster...\n")
    if err := loadDataStorageImageWithTag(clusterName, dataStorageImage, writer); err != nil {
        return fmt.Errorf("failed to load Data Storage image: %w", err)
    }

    // 3. Deploy with same image name
    fmt.Fprintf(writer, "📦 Deploying Data Storage infrastructure...\n")
    if err := DeployDataStorageTestServices(ctx, namespace, kubeconfigPath, dataStorageImage, writer); err != nil {
        return fmt.Errorf("failed to deploy Data Storage infrastructure: %w", err)
    }

    // ...
}
```

### **Option B: Use Hardcoded Image Name** (NOT RECOMMENDED)

Change deployment to use hardcoded image:

```go
// Don't use this - defeats composite tag strategy
if err := DeployDataStorageTestServices(ctx, namespace, kubeconfigPath,
    "localhost/kubernaut-datastorage:e2e-test-datastorage", writer); err != nil {
```

**Why Not Recommended**: Breaks DD-TEST-001 composite tag isolation strategy

---

## 🔄 **Affected E2E Tests**

This is a **systemic issue** affecting ALL E2E tests:

| Service | Build Function | Deploy Function | Status |
|---------|---------------|-----------------|---------|
| Gateway | `buildDataStorageImage()` | `GenerateInfraImageName()` | ❌ Same issue |
| SignalProcessing | `buildDataStorageImage()` | `GenerateInfraImageName()` | ❌ Same issue |
| WorkflowExecution | `buildDataStorageImage()` | `GenerateInfraImageName()` | ❌ Same issue |
| Notification | `buildDataStorageImage()` | `GenerateInfraImageName()` | ❌ Same issue |

**All E2E tests** need the same fix!

---

## 📊 **User's Changes - Not the Cause**

### **What User Changed** (`shared_integration_utils.go`):
- Added PostgreSQL env vars (`POSTGRES_SSLMODE`, etc.)
- Added Redis env vars (`REDIS_DB`, etc.)
- Added service config env vars (`USE_ENV_CONFIG`, etc.)

### **Why Not the Cause**:
1. **Different Context**: User's changes are for **integration tests** (Podman-based)
2. **E2E Uses Kind**: E2E tests deploy to Kubernetes (Kind cluster), not Podman
3. **No Shared Function Used**: E2E DataStorage deployment doesn't use `shared_integration_utils.go`
4. **Image Issue Pre-Existed**: Image mismatch existed before user's changes

---

## 🎯 **Validation Steps**

To confirm the fix works:

1. **Apply Fix**: Change to `buildDataStorageImageWithTag()`
2. **Run Test**: `make test-e2e-notification`
3. **Check Logs**: Pod should pull correct image
4. **Verify**: Pod becomes ready within timeout
5. **Success**: E2E tests execute

Expected output:
```
🔨 Building Data Storage image with tag: localhost/datastorage:notification-1a2b3c4d
✅ DataStorage image built: localhost/datastorage:notification-1a2b3c4d
📦 Loading Data Storage image: localhost/datastorage:notification-1a2b3c4d
✅ DataStorage image loaded: localhost/datastorage:notification-1a2b3c4d
🚀 Deploying Data Storage Service...
✅ Data Storage Service deployed
⏳ Waiting for services to be ready...
✅ PostgreSQL pod ready
✅ Redis pod ready
✅ Data Storage Service pod ready  <-- THIS SHOULD NOW WORK
```

---

## 📚 **Related Documents**

- `DD-INTEGRATION-001-local-image-builds.md` - Composite tag strategy
- `DD-TEST-001` - Service-specific image tags for parallel isolation
- `DATASTORAGE_AUDIT_BUFFER_FLUSH_TIMING_ISSUE.md` - Different DataStorage issue (not related)

---

## 🎉 **Key Takeaways**

1. ✅ **User's Changes**: Not the cause (different test type)
2. ✅ **Composite Tags**: Strategy is correct, implementation needs fix
3. ✅ **Root Cause**: Using wrong build function (hardcoded vs. dynamic tags)
4. ✅ **Solution**: Use `buildDataStorageImageWithTag()` + `loadDataStorageImageWithTag()`
5. ✅ **Scope**: All E2E tests need same fix (systematic issue)

---

**Status**: ✅ **ROOT CAUSE IDENTIFIED**
**Solution**: ✅ **CLEAR AND ACTIONABLE**
**Priority**: 🔴 **HIGH** - Blocks all E2E tests
**Effort**: 🟢 **LOW** - Simple function swap


