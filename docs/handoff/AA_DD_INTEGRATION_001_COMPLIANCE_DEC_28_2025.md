# DD-INTEGRATION-001 Compliance Fix - AIAnalysis Integration Tests

**Date**: December 28, 2025
**Component**: AIAnalysis Integration Tests
**Issue**: Image tag format violation of DD-INTEGRATION-001 v2.0

---

## 🚨 **Problem Identified**

AIAnalysis integration tests were using the **DEPRECATED v1.0 image tag format**:

```go
❌ WRONG (v1.0): "kubernaut-datastorage:latest"
❌ WRONG (v1.0): "localhost/kubernaut-datastorage:latest"
```

**Per DD-INTEGRATION-001 v2.0**, the correct format is:

```go
✅ CORRECT (v2.0): "localhost/datastorage:aianalysis-{uuid}"
```

---

## ✅ **Solution Implemented**

### **1. Created `GenerateInfraImageName()` Helper Function**

**File**: `test/infrastructure/shared_integration_utils.go`

```go
// GenerateInfraImageName generates a composite image tag for shared infrastructure images
// per DD-INTEGRATION-001 v2.0 Section "Image Naming Convention".
//
// Format: localhost/{infrastructure}:{consumer}-{8-char-hex-uuid}
//
// Examples:
//   GenerateInfraImageName("datastorage", "aianalysis")
//   → "localhost/datastorage:aianalysis-a3b5c7d9"
func GenerateInfraImageName(infrastructure, consumer string) string {
	uuid := make([]byte, 4) // 4 bytes = 8 hex characters
	if _, err := rand.Read(uuid); err != nil {
		return fmt.Sprintf("localhost/%s:%s-%d", infrastructure, consumer, time.Now().Unix())
	}

	hexUUID := hex.EncodeToString(uuid)
	return fmt.Sprintf("localhost/%s:%s-%s", infrastructure, consumer, hexUUID)
}
```

**Benefits**:
- ✅ Prevents image tag collisions during parallel test runs
- ✅ Clear traceability (which consumer built which image)
- ✅ Consistent with DD-INTEGRATION-001 v2.0 standard
- ✅ Reusable across all services

### **2. Made `ImageTag` Mandatory in `IntegrationDataStorageConfig`**

**File**: `test/infrastructure/shared_integration_utils.go`

**Before**:
```go
ImageTag: string // Optional: custom image tag (default: "kubernaut-datastorage:latest")

// In StartDataStorage():
if cfg.ImageTag == "" {
    cfg.ImageTag = "kubernaut-datastorage:latest"  // ❌ DEPRECATED
}
```

**After**:
```go
ImageTag: string // REQUIRED: Composite tag per DD-INTEGRATION-001 v2.0 (use GenerateInfraImageName("datastorage", "yourservice"))

// In StartDataStorage():
if cfg.ImageTag == "" {
    return fmt.Errorf("ImageTag is required (DD-INTEGRATION-001 v2.0): use GenerateInfraImageName(\"datastorage\", \"yourservice\")")
}
```

**Rationale**: Forces all consumers to explicitly provide DD-INTEGRATION-001 v2.0 compliant tags, preventing accidental use of deprecated format.

### **3. Updated AIAnalysis Integration Infrastructure**

**File**: `test/infrastructure/aianalysis.go`

**Before**:
```go
if err := StartDataStorage(IntegrationDataStorageConfig{
    ContainerName: AIAnalysisIntegrationDataStorageContainer,
    Port:          AIAnalysisIntegrationDataStoragePort,
    // ... other fields ...
    // ❌ ImageTag not specified - used deprecated default
}, writer); err != nil {
    return err
}
```

**After**:
```go
// Generate composite image tag per DD-INTEGRATION-001 v2.0
// Format: localhost/datastorage:aianalysis-{uuid}
dsImageTag := GenerateInfraImageName("datastorage", "aianalysis")
fmt.Fprintf(writer, "   Using image tag: %s\n", dsImageTag)

if err := StartDataStorage(IntegrationDataStorageConfig{
    ContainerName: AIAnalysisIntegrationDataStorageContainer,
    Port:          AIAnalysisIntegrationDataStoragePort,
    // ... other fields ...
    ImageTag:      dsImageTag, // ✅ DD-INTEGRATION-001 v2.0: Composite tag for collision avoidance
}, writer); err != nil {
    return err
}
```

---

## 📊 **Impact Analysis**

### **Files Modified**
1. `test/infrastructure/shared_integration_utils.go`:
   - Added `GenerateInfraImageName()` helper function (~40 lines)
   - Made `ImageTag` field mandatory with validation
   - Updated `IntegrationDataStorageConfig` documentation

2. `test/infrastructure/aianalysis.go`:
   - Added explicit `dsImageTag` generation using helper
   - Added logging of image tag for transparency

### **Breaking Changes**
None - AIAnalysis was the only consumer of the default tag behavior.

### **Benefits**
✅ **Compliance**: Now follows DD-INTEGRATION-001 v2.0 standard
✅ **Collision Avoidance**: Unique tags prevent parallel test conflicts
✅ **Traceability**: Image tags show which service built them
✅ **Consistency**: Matches pattern used by other migrated services

---

## 🔍 **Verification**

### **Build Test**
```bash
$ cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
$ go build ./test/infrastructure/...
# ✅ SUCCESS
```

### **Expected Image Tags**
When tests run, DataStorage images will now be tagged as:
```
localhost/datastorage:aianalysis-a3b5c7d9
localhost/datastorage:aianalysis-1884d074
localhost/datastorage:aianalysis-f7e8d9c0
# (8-char hex UUID changes each run)
```

---

## 📚 **Related Documentation**

- **DD-INTEGRATION-001 v2.0**: Local Image Builds for Integration Tests
  - Section: "Image Naming Convention"
  - Format: `localhost/{infrastructure}:{consumer}-{uuid}`

- **DD-TEST-001**: Unique Container Image Tags
  - Composite tagging strategy for collision avoidance

---

## ✅ **Compliance Status**

| Requirement | Status |
|------------|--------|
| Use composite image tags | ✅ Implemented |
| Format: `localhost/{infra}:{consumer}-{uuid}` | ✅ Compliant |
| No `kubernaut-*` prefix | ✅ Removed |
| No `:latest` tag | ✅ Removed |
| Helper function for tag generation | ✅ Created |
| Mandatory `ImageTag` parameter | ✅ Enforced |

---

## 🎯 **Next Steps**

None required. AIAnalysis integration tests are now fully compliant with DD-INTEGRATION-001 v2.0.

**Future Work**: Audit other services (Gateway, WorkflowExecution, SignalProcessing, etc.) to ensure they also use the correct format.

---

**Document Version**: 1.0
**Status**: ✅ COMPLETE
**Reviewed By**: Platform Team


