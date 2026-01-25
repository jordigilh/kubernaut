# Mock LLM DD-TEST-004 Compliance Update

**Date**: 2026-01-11
**Related**: DD-TEST-004 Unique Resource Naming Strategy
**Migration Plan**: MOCK_LLM_MIGRATION_PLAN.md v1.6.0

---

## Summary

Updated Mock LLM infrastructure to comply with DD-TEST-004 unique resource naming strategy using `GenerateInfraImageName()` for image tags.

---

## Changes Made

### **1. Infrastructure Code** (`test/infrastructure/mock_llm.go`)

**Before** (Hardcoded tag):
```go
const MockLLMImage = "localhost/mock-llm:latest"
```

**After** (DD-TEST-004 compliant):
```go
// MockLLMConfig now includes ImageTag field
type MockLLMConfig struct {
    ServiceName   string
    Port          int
    ContainerName string
    ImageTag      string // DD-TEST-004: Unique tag per service
}

// HAPI configuration
func GetMockLLMConfigForHAPI() MockLLMConfig {
    return MockLLMConfig{
        ServiceName:   "hapi",
        Port:          MockLLMPortHAPI,
        ContainerName: MockLLMContainerNameHAPI,
        ImageTag:      GenerateInfraImageName("mock-llm", "hapi"),
    }
}

// AIAnalysis configuration
func GetMockLLMConfigForAIAnalysis() MockLLMConfig {
    return MockLLMConfig{
        ServiceName:   "aianalysis",
        Port:          MockLLMPortAIAnalysis,
        ContainerName: MockLLMContainerNameAIAnalysis,
        ImageTag:      GenerateInfraImageName("mock-llm", "aianalysis"),
    }
}
```

---

## Image Tag Format

### **Per DD-TEST-004**:
```
Format: localhost/{infrastructure}:{consumer}-{8-char-hex-uuid}
```

### **Mock LLM Examples**:
```bash
# HAPI Integration Tests
localhost/mock-llm:hapi-a3b5c7d9

# AIAnalysis Integration Tests
localhost/mock-llm:aianalysis-1884d074
```

---

## Benefits

### **1. Parallel Test Safety** ✅
- HAPI and AIAnalysis integration tests can run in parallel
- Each service gets unique Mock LLM image tag
- No image tag collisions during parallel execution

### **2. Traceability** ✅
- Clear identification: which service built which Mock LLM image
- Easier debugging: `podman images | grep mock-llm` shows service-specific tags
- Test isolation: Each service's tests use their own image

### **3. Consistency** ✅
- Matches DataStorage, HAPI, Gateway patterns
- Same `GenerateInfraImageName()` function used across all services
- Follows established DD-TEST-004 standard

---

## Build Commands

### **HAPI Integration Tests**:
```bash
# Image tag generated automatically by GetMockLLMConfigForHAPI()
# Example: localhost/mock-llm:hapi-a3b5c7d9

podman build -t localhost/mock-llm:hapi-a3b5c7d9 \
  -f test/services/mock-llm/Dockerfile .
```

### **AIAnalysis Integration Tests**:
```bash
# Image tag generated automatically by GetMockLLMConfigForAIAnalysis()
# Example: localhost/mock-llm:aianalysis-1884d074

podman build -t localhost/mock-llm:aianalysis-1884d074 \
  -f test/services/mock-llm/Dockerfile .
```

### **E2E Tests (Kind)**:
```bash
# E2E tests use a shared image loaded into Kind
# Tag can be any unique identifier

podman build -t localhost/mock-llm:e2e-12345678 \
  -f test/services/mock-llm/Dockerfile .

kind load docker-image localhost/mock-llm:e2e-12345678 --name kubernaut-test
```

---

## Integration Test Usage

### **HAPI Integration Suite** (`test/integration/holmesgptapi/suite_test.go`):

```go
var _ = SynchronizedBeforeSuite(
    func() []byte {
        // Get HAPI-specific Mock LLM config (includes unique image tag)
        mockLLMConfig := infrastructure.GetMockLLMConfigForHAPI()

        // Build Mock LLM image with unique tag
        buildCmd := exec.Command("podman", "build",
            "-t", mockLLMConfig.ImageTag, // localhost/mock-llm:hapi-a3b5c7d9
            "-f", "test/services/mock-llm/Dockerfile",
            ".")
        Expect(buildCmd.Run()).To(Succeed())

        // Start container with unique image
        containerID, err := infrastructure.StartMockLLMContainer(
            context.Background(),
            mockLLMConfig,
            GinkgoWriter,
        )
        Expect(err).ToNot(HaveOccurred())

        return []byte(containerID)
    },
    func(data []byte) {
        // All processes wait for Mock LLM ready
        config := infrastructure.GetMockLLMConfigForHAPI()
        endpoint := infrastructure.GetMockLLMEndpoint(config)
        os.Setenv("LLM_ENDPOINT", endpoint)
    },
)
```

### **AIAnalysis Integration Suite** (`test/integration/aianalysis/suite_test.go`):

```go
var _ = SynchronizedBeforeSuite(
    func() []byte {
        // Get AIAnalysis-specific Mock LLM config (includes unique image tag)
        mockLLMConfig := infrastructure.GetMockLLMConfigForAIAnalysis()

        // Build Mock LLM image with unique tag
        buildCmd := exec.Command("podman", "build",
            "-t", mockLLMConfig.ImageTag, // localhost/mock-llm:aianalysis-1884d074
            "-f", "test/services/mock-llm/Dockerfile",
            ".")
        Expect(buildCmd.Run()).To(Succeed())

        // Start container with unique image
        containerID, err := infrastructure.StartMockLLMContainer(
            context.Background(),
            mockLLMConfig,
            GinkgoWriter,
        )
        Expect(err).ToNot(HaveOccurred())

        return []byte(containerID)
    },
    func(data []byte) {
        // All processes wait for Mock LLM ready
        config := infrastructure.GetMockLLMConfigForAIAnalysis()
        endpoint := infrastructure.GetMockLLMEndpoint(config)
        os.Setenv("LLM_ENDPOINT", endpoint)
    },
)
```

---

## Port Allocation (DD-TEST-001 v2.5)

| Service | Test Tier | Port | Image Tag Example |
|---------|-----------|------|-------------------|
| **HAPI** | Integration | 18140 | `localhost/mock-llm:hapi-a3b5c7d9` |
| **AIAnalysis** | Integration | 18141 | `localhost/mock-llm:aianalysis-1884d074` |
| **Both** | E2E (Kind) | ClusterIP (no external port) | `localhost/mock-llm:e2e-12345678` |

---

## Validation

### **Check Image Tags**:
```bash
# List Mock LLM images with unique tags
podman images | grep mock-llm

# Expected output (example):
# localhost/mock-llm  hapi-a3b5c7d9        9beee5ea49fc  5 minutes ago
# localhost/mock-llm  aianalysis-1884d074  9beee5ea49fc  5 minutes ago
```

### **Verify Container Uses Correct Image**:
```bash
# HAPI integration test
podman ps | grep mock-llm-hapi
# Should show: localhost/mock-llm:hapi-a3b5c7d9

# AIAnalysis integration test
podman ps | grep mock-llm-aianalysis
# Should show: localhost/mock-llm:aianalysis-1884d074
```

---

## References

- **DD-TEST-004**: Unique Resource Naming Strategy
- **DD-TEST-001 v2.5**: Port Allocation Strategy
- **DD-INTEGRATION-001 v2.0**: Programmatic Podman Setup
- **MOCK_LLM_MIGRATION_PLAN.md v1.6.0**: Mock LLM Migration Plan

---

## Status

✅ **COMPLETE** - Mock LLM infrastructure now DD-TEST-004 compliant

**Next Steps**:
1. Update HAPI integration suite to build Mock LLM with unique tag
2. Update AIAnalysis integration suite to build Mock LLM with unique tag
3. Run integration tests to validate parallel execution
4. Document in Phase 6 validation results
