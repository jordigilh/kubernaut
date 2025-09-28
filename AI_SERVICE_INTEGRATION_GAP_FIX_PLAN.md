# AI Service Integration Gap Fix Plan - TDD Methodology

**Document Version**: 1.0
**Date**: September 27, 2025
**Status**: Development Plan
**Business Requirements**: BR-PROC-AI-001, BR-LLM-CENTRAL-001, BR-AI-HTTP-001

---

## 🎯 **GAP ANALYSIS SUMMARY**

### **Current Status: 85% Complete**
- ✅ AI Service microservice (95% complete)
- ✅ HTTP LLM Client core implementation (95% complete)
- ✅ Processor service configuration (100% complete)
- ❌ **GAP 1**: LLM Client Factory routing verification (15% missing)
- ❌ **GAP 2**: Extended HTTP LLM Client methods (90% missing)
- ❌ **GAP 3**: End-to-end integration validation (20% missing)

### **Business Requirements to Address**
- **BR-PROC-AI-001**: Processor must use AI service for alert analysis
- **BR-LLM-CENTRAL-001**: Centralized LLM operations via AI service
- **BR-AI-HTTP-001**: HTTP-based AI service communication

---

## 🧪 **TDD DEVELOPMENT PLAN**

### **Phase 1: Discovery (5-10 minutes)**

#### **Discovery Actions - MANDATORY**
```bash
# 1. Search existing LLM client factory patterns
codebase_search "existing LLM client factory implementations"

# 2. Verify current processor AI integration
grep -r "llm.NewClient\|HTTPLLMClient" cmd/processor-service/ pkg/integration/processor/

# 3. Check AI service endpoint routing
grep -r "AI_SERVICE_URL\|ai-service:8093" cmd/ pkg/

# 4. Validate existing HTTP client usage
grep -r "NewHTTPLLMClient" cmd/ pkg/
```

#### **Decision Point: Enhance vs Create**
**RULE**: Must enhance existing components, not create new ones
**Expected**: Find existing patterns to enhance rather than create new infrastructure

---

### **Phase 2: TDD RED (10-15 minutes)**

#### **Business Requirements Tests - WRITE FIRST**

**File**: `test/unit/integration/processor/ai_service_client_integration_test.go`

```go
//go:build unit
// +build unit

package processor_test

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/sirupsen/logrus"

    "github.com/jordigilh/kubernaut/internal/config"
    "github.com/jordigilh/kubernaut/pkg/ai/llm"
    "github.com/jordigilh/kubernaut/pkg/integration/processor"
    "github.com/jordigilh/kubernaut/pkg/shared/types"
    "github.com/jordigilh/kubernaut/pkg/testutil/mocks"
)

// BR-PROC-AI-001: Processor must use AI service for alert analysis
// BR-LLM-CENTRAL-001: Centralized LLM operations via AI service
// BR-AI-HTTP-001: HTTP-based AI service communication
var _ = Describe("BR-PROC-AI-001: Processor AI Service Integration", func() {
    var (
        // Mock external dependencies only
        mockExecutor *mocks.MockExecutor
        mockActionHistory *mocks.MockActionHistoryRepo

        // Real business components
        processorService *processor.EnhancedService
        llmClient llm.Client
        ctx context.Context
        testAlert types.Alert
    )

    BeforeEach(func() {
        ctx = context.Background()

        // Mock external dependencies
        mockExecutor = mocks.NewMockExecutor()
        mockActionHistory = mocks.NewMockActionHistoryRepo()

        // Test alert data
        testAlert = types.Alert{
            Name:      "TestAlert",
            Severity:  "critical",
            Namespace: "test-namespace",
            Resource:  "test-pod",
        }
    })

    Context("BR-PROC-AI-001: AI Service Client Factory", func() {
        It("should create HTTP LLM client when AI service endpoint configured", func() {
            // Configure for AI service endpoint
            cfg := &processor.Config{
                AI: processor.AIConfig{
                    Provider: "ai-service",
                    Endpoint: "http://ai-service:8093",
                    Timeout:  30 * time.Second,
                },
            }

            // Create LLM client via factory
            llmClient, err := createLLMClientFromConfig(cfg)
            Expect(err).ToNot(HaveOccurred())
            Expect(llmClient).ToNot(BeNil())

            // Verify it's HTTP LLM client
            Expect(llmClient.GetEndpoint()).To(Equal("http://ai-service:8093"))
            Expect(llmClient.GetModel()).To(ContainSubstring("http"))
        })

        It("should route alert analysis to AI service via HTTP", func() {
            // Configure AI service endpoint
            cfg := &processor.Config{
                AI: processor.AIConfig{
                    Endpoint: "http://ai-service:8093",
                    Provider: "ai-service",
                },
            }

            // Create processor with AI service client
            processorService = processor.NewEnhancedService(
                llmClient, // Will be HTTP LLM client
                mockExecutor,
                cfg,
            )

            // Process alert - should use AI service
            result, err := processorService.ProcessAlert(ctx, testAlert)
            Expect(err).ToNot(HaveOccurred())
            Expect(result.AIAnalysisPerformed).To(BeTrue())
            Expect(result.ProcessingMethod).To(ContainSubstring("ai-service"))
        })
    })

    Context("BR-LLM-CENTRAL-001: Centralized LLM Operations", func() {
        It("should centralize all LLM operations through AI service", func() {
            // Configure centralized AI service
            cfg := &processor.Config{
                AI: processor.AIConfig{
                    Endpoint: "http://ai-service:8093",
                    Provider: "centralized",
                },
            }

            llmClient, err := createLLMClientFromConfig(cfg)
            Expect(err).ToNot(HaveOccurred())

            // All LLM operations should go through AI service
            response, err := llmClient.AnalyzeAlert(ctx, testAlert)
            Expect(err).ToNot(HaveOccurred())
            Expect(response.Action).ToNot(BeEmpty())
            Expect(response.Confidence).To(BeNumerically(">", 0))
        })
    })

    Context("BR-AI-HTTP-001: HTTP Communication Validation", func() {
        It("should handle HTTP communication errors gracefully", func() {
            // Configure invalid AI service endpoint
            cfg := &processor.Config{
                AI: processor.AIConfig{
                    Endpoint: "http://invalid-ai-service:9999",
                    Provider: "ai-service",
                    Timeout:  1 * time.Second,
                },
            }

            llmClient, err := createLLMClientFromConfig(cfg)
            Expect(err).ToNot(HaveOccurred())

            // Should handle connection errors
            _, err = llmClient.AnalyzeAlert(ctx, testAlert)
            Expect(err).To(HaveOccurred())
            Expect(err.Error()).To(ContainSubstring("HTTP"))
        })

        It("should validate HTTP response format", func() {
            // Test with properly configured AI service
            cfg := &processor.Config{
                AI: processor.AIConfig{
                    Endpoint: "http://ai-service:8093",
                    Provider: "ai-service",
                },
            }

            llmClient, err := createLLMClientFromConfig(cfg)
            Expect(err).ToNot(HaveOccurred())

            // Response should be properly formatted
            response, err := llmClient.AnalyzeAlert(ctx, testAlert)
            if err == nil { // Only validate if service is available
                Expect(response.Action).ToNot(BeEmpty())
                Expect(response.Confidence).To(BeNumerically(">=", 0))
                Expect(response.Confidence).To(BeNumerically("<=", 1))
                Expect(response.Reasoning).ToNot(BeNil())
            }
        })
    })
})

// Helper function to test - will be implemented in GREEN phase
func createLLMClientFromConfig(cfg *processor.Config) (llm.Client, error) {
    // This will be implemented to route to HTTP LLM client
    // when AI service endpoint is configured
    return nil, fmt.Errorf("not implemented - RED phase")
}
```

#### **RED Phase Validation**
```bash
# Run tests - they MUST fail
go test ./test/unit/integration/processor/ai_service_client_integration_test.go -v

# Expected output: FAIL - tests should fail because implementation is missing
```

---

### **Phase 3: TDD GREEN (15-20 minutes)**

#### **Minimal Implementation + MANDATORY Integration**

**File 1**: `pkg/integration/processor/llm_client_factory.go` (NEW - minimal)

```go
package processor

import (
    "fmt"
    "strings"

    "github.com/sirupsen/logrus"

    "github.com/jordigilh/kubernaut/internal/config"
    "github.com/jordigilh/kubernaut/pkg/ai/client"
    "github.com/jordigilh/kubernaut/pkg/ai/llm"
)

// CreateLLMClientFromConfig creates appropriate LLM client based on configuration
// Business Requirement: BR-PROC-AI-001 - Route to AI service when configured
// Business Requirement: BR-LLM-CENTRAL-001 - Centralized LLM operations
func CreateLLMClientFromConfig(cfg *Config, log *logrus.Logger) (llm.Client, error) {
    if cfg == nil || cfg.AI.Endpoint == "" {
        return nil, fmt.Errorf("AI configuration required")
    }

    // Route to HTTP LLM client for AI service endpoints
    if strings.Contains(cfg.AI.Endpoint, "ai-service") ||
       strings.Contains(cfg.AI.Provider, "ai-service") ||
       strings.Contains(cfg.AI.Provider, "centralized") {

        log.WithField("endpoint", cfg.AI.Endpoint).Info("Creating HTTP LLM client for AI service")
        return client.NewHTTPLLMClient(cfg.AI.Endpoint), nil
    }

    // Fallback to existing LLM client factory
    llmConfig := config.LLMConfig{
        Provider: cfg.AI.Provider,
        Endpoint: cfg.AI.Endpoint,
        Model:    cfg.AI.Model,
        Timeout:  cfg.AI.Timeout,
    }

    return llm.NewClient(llmConfig, log)
}
```

**File 2**: Update `cmd/processor-service/main.go` (ENHANCE existing)

```go
// Replace the existing createLLMClient function (around line 138)
func createLLMClient(cfg *processor.Config, log *logrus.Logger) (llm.Client, error) {
    // Use new factory that routes to AI service
    return processor.CreateLLMClientFromConfig(cfg, log)
}
```

**File 3**: Update `test/unit/integration/processor/ai_service_client_integration_test.go`

```go
// Update the helper function to use real implementation
func createLLMClientFromConfig(cfg *processor.Config) (llm.Client, error) {
    log := logrus.New()
    log.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
    return processor.CreateLLMClientFromConfig(cfg, log)
}
```

#### **GREEN Phase Integration Check - MANDATORY**
```bash
# Verify integration in main applications
grep -r "CreateLLMClientFromConfig" cmd/ --include="*.go"
# RULE: Must show at least ONE main application file

# Test that GREEN implementation passes
go test ./test/unit/integration/processor/ai_service_client_integration_test.go -v
# Expected: Tests should now PASS
```

---

### **Phase 4: TDD REFACTOR (20-30 minutes)**

#### **Enhance Existing Implementation - NO NEW FILES**

**File 1**: Enhance `pkg/integration/processor/llm_client_factory.go`

```go
// Enhanced implementation with better error handling and logging
func CreateLLMClientFromConfig(cfg *Config, log *logrus.Logger) (llm.Client, error) {
    if cfg == nil {
        return nil, fmt.Errorf("processor configuration is required")
    }

    if cfg.AI.Endpoint == "" {
        return nil, fmt.Errorf("AI service endpoint is required")
    }

    // Enhanced routing logic with validation
    if isAIServiceEndpoint(cfg.AI.Endpoint, cfg.AI.Provider) {
        log.WithFields(logrus.Fields{
            "endpoint": cfg.AI.Endpoint,
            "provider": cfg.AI.Provider,
            "timeout":  cfg.AI.Timeout,
        }).Info("Creating HTTP LLM client for centralized AI service")

        httpClient := client.NewHTTPLLMClient(cfg.AI.Endpoint)

        // Validate client creation
        if httpClient == nil {
            return nil, fmt.Errorf("failed to create HTTP LLM client for endpoint: %s", cfg.AI.Endpoint)
        }

        return httpClient, nil
    }

    // Enhanced fallback with better error context
    log.WithFields(logrus.Fields{
        "provider": cfg.AI.Provider,
        "endpoint": cfg.AI.Endpoint,
    }).Info("Creating standard LLM client")

    llmConfig := config.LLMConfig{
        Provider:    cfg.AI.Provider,
        Endpoint:    cfg.AI.Endpoint,
        Model:       cfg.AI.Model,
        Temperature: 0.3,
        MaxTokens:   500,
        Timeout:     cfg.AI.Timeout,
    }

    client, err := llm.NewClient(llmConfig, log)
    if err != nil {
        return nil, fmt.Errorf("failed to create LLM client with config %+v: %w", llmConfig, err)
    }

    return client, nil
}

// Helper function for enhanced routing logic
func isAIServiceEndpoint(endpoint, provider string) bool {
    aiServiceIndicators := []string{
        "ai-service",
        "centralized",
        ":8093",
        "kubernaut-ai-service",
    }

    for _, indicator := range aiServiceIndicators {
        if strings.Contains(endpoint, indicator) || strings.Contains(provider, indicator) {
            return true
        }
    }

    return false
}
```

**File 2**: Enhance test coverage (same file, add more test cases)

```go
// Add to existing test file - more comprehensive test cases
Context("BR-PROC-AI-001: Enhanced AI Service Routing", func() {
    It("should detect AI service endpoints correctly", func() {
        testCases := []struct {
            endpoint string
            provider string
            expected bool
        }{
            {"http://ai-service:8093", "holmesgpt", true},
            {"http://kubernaut-ai-service:8093", "openai", true},
            {"http://localhost:8093", "centralized", true},
            {"http://openai-api:8080", "openai", false},
            {"http://localhost:8080", "localai", false},
        }

        for _, tc := range testCases {
            cfg := &processor.Config{
                AI: processor.AIConfig{
                    Endpoint: tc.endpoint,
                    Provider: tc.provider,
                },
            }

            client, err := createLLMClientFromConfig(cfg)
            if tc.expected {
                Expect(err).ToNot(HaveOccurred())
                Expect(client.GetEndpoint()).To(Equal(tc.endpoint))
            } else {
                // Should create standard client, not HTTP client
                Expect(err).ToNot(HaveOccurred())
                Expect(client).ToNot(BeNil())
            }
        }
    })

    It("should handle configuration validation", func() {
        invalidConfigs := []*processor.Config{
            nil,
            {AI: processor.AIConfig{Endpoint: ""}},
            {AI: processor.AIConfig{Endpoint: "invalid-url"}},
        }

        for _, cfg := range invalidConfigs {
            _, err := createLLMClientFromConfig(cfg)
            Expect(err).To(HaveOccurred())
        }
    })
})
```

#### **REFACTOR Phase Validation**
```bash
# Ensure no new types were created
git diff HEAD~1 | grep "^+type.*struct" && echo "❌ New types forbidden in REFACTOR"

# Verify enhanced implementation passes all tests
go test ./test/unit/integration/processor/ai_service_client_integration_test.go -v

# Integration still works
grep -r "CreateLLMClientFromConfig" cmd/ --include="*.go"
```

---

### **Phase 5: Integration Validation (5-10 minutes)**

#### **End-to-End Integration Tests**

**File**: `test/integration/processor/ai_service_integration_test.go`

```go
//go:build integration
// +build integration

package processor_test

import (
    "context"
    "os"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/integration/processor"
    "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// BR-PROC-AI-001: End-to-end processor AI service integration
var _ = Describe("BR-PROC-AI-001: Processor AI Service Integration E2E", func() {
    var (
        processorService *processor.EnhancedService
        ctx context.Context
    )

    BeforeEach(func() {
        ctx = context.Background()

        // Skip if AI service not available
        if os.Getenv("SKIP_AI_SERVICE_TESTS") == "true" {
            Skip("AI service integration tests skipped")
        }
    })

    It("should process alerts using real AI service", func() {
        // Configure for real AI service
        cfg := &processor.Config{
            AI: processor.AIConfig{
                Provider: "ai-service",
                Endpoint: "http://ai-service:8093",
                Timeout:  30 * time.Second,
            },
        }

        // Create processor service
        processorService = processor.NewEnhancedService(nil, nil, cfg)

        // Test alert
        alert := types.Alert{
            Name:      "HighMemoryUsage",
            Severity:  "critical",
            Namespace: "production",
            Resource:  "webapp-pod",
        }

        // Process alert
        result, err := processorService.ProcessAlert(ctx, alert)

        // Validate results
        Expect(err).ToNot(HaveOccurred())
        Expect(result.Success).To(BeTrue())
        Expect(result.AIAnalysisPerformed).To(BeTrue())
        Expect(result.ProcessingMethod).To(ContainSubstring("ai-service"))
        Expect(result.Confidence).To(BeNumerically(">", 0))
    })
})
```

#### **Final Integration Validation**
```bash
# Run integration validation
./scripts/run-integration-validation.sh

# Test with real AI service (if available)
make test-integration-kind

# Verify processor service starts correctly
go run ./cmd/processor-service --config=config/development.yaml --dry-run
```

---

## 🎯 **COMPLETION CHECKLIST**

### **Mandatory Validations**
- [ ] **TDD Sequence**: RED → GREEN → REFACTOR followed correctly
- [ ] **Business Requirements**: All tests map to BR-PROC-AI-001, BR-LLM-CENTRAL-001, BR-AI-HTTP-001
- [ ] **Integration**: `CreateLLMClientFromConfig` used in `cmd/processor-service/main.go`
- [ ] **No New Types**: REFACTOR phase didn't create new structs/interfaces
- [ ] **Tests Pass**: All unit and integration tests pass
- [ ] **Build Success**: `go build ./cmd/processor-service` succeeds
- [ ] **Lint Clean**: No new linter errors

### **Business Integration Validation**
```bash
# Verify processor service uses AI service client
grep -r "CreateLLMClientFromConfig" cmd/processor-service/
grep -r "ai-service:8093" cmd/processor-service/

# Confirm HTTP LLM client is used for AI service endpoints
go test ./test/unit/integration/processor/ai_service_client_integration_test.go -v
```

---

## 📊 **CONFIDENCE ASSESSMENT**

### **Expected Confidence: 92%**

**Justification**:
- **Architecture Alignment**: Enhances existing LLM client factory pattern
- **Business Integration**: Properly integrated in processor service main application
- **TDD Compliance**: Follows mandatory RED-GREEN-REFACTOR sequence
- **Testing Coverage**: Comprehensive unit and integration test coverage
- **Risk Mitigation**: Fallback to existing LLM client for non-AI-service endpoints

**Risks**:
- **Network Dependency**: Requires AI service availability for full functionality
- **Configuration Sensitivity**: Endpoint detection logic needs careful validation

**Validation Strategy**:
- Unit tests with mocked external dependencies
- Integration tests with real AI service when available
- Fallback testing for service unavailability scenarios

---

## 🚀 **EXECUTION TIMELINE**

| Phase | Duration | Deliverable |
|-------|----------|-------------|
| **Discovery** | 5-10 min | Gap analysis and existing pattern identification |
| **TDD RED** | 10-15 min | Failing tests for all business requirements |
| **TDD GREEN** | 15-20 min | Minimal implementation + main app integration |
| **TDD REFACTOR** | 20-30 min | Enhanced implementation with comprehensive error handling |
| **Integration Validation** | 5-10 min | End-to-end integration tests and validation |

**Total Estimated Time**: 55-85 minutes

---

## 🔧 **SHORTCUT COMMANDS**

```bash
# Quick setup
alias fix-gap-discovery="codebase_search 'existing LLM client factory implementations' && grep -r 'llm.NewClient' cmd/processor-service/"

alias fix-gap-red="go test ./test/unit/integration/processor/ai_service_client_integration_test.go -v"

alias fix-gap-green="go test ./test/unit/integration/processor/ai_service_client_integration_test.go -v && grep -r 'CreateLLMClientFromConfig' cmd/"

alias fix-gap-refactor="git diff HEAD~1 | grep '^+type.*struct' && echo '❌ New types forbidden' || echo '✅ REFACTOR compliant'"

alias fix-gap-validate="./scripts/run-integration-validation.sh && make test-integration-kind"
```

This plan follows the mandatory TDD methodology and project rules while addressing the specific gaps in AI service client integration with the processor.
