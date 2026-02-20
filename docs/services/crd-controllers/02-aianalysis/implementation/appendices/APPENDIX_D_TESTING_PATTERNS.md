# Appendix D: Testing Patterns - AI Analysis Service

> **Note (ADR-056/ADR-055):** References to `EnrichmentResults.DetectedLabels` and `EnrichmentResults.OwnerChain` in this document are historical. These fields were removed per ADR-056 and ADR-055.

**Part of**: AI Analysis Implementation Plan V1.0
**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md)
**Last Updated**: 2025-12-04
**Template Source**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md v3.0

---

## 📚 Testing Overview

**Framework**: Ginkgo/Gomega BDD
**Coverage Target**: 70-75% for unit tests
**Reference**: [TESTING_GUIDELINES.md](../../../../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 🧪 Test File Organization

### Directory Structure

```
test/
├── unit/
│   └── aianalysis/
│       ├── suite_test.go              # Ginkgo suite setup
│       ├── validating_handler_test.go # ValidatingHandler tests
│       ├── investigating_handler_test.go # InvestigatingHandler tests
│       ├── analyzing_handler_test.go  # AnalyzingHandler tests
│       ├── recommending_handler_test.go # RecommendingHandler tests
│       ├── rego_evaluator_test.go     # Rego policy tests
│       └── metrics_test.go            # Metrics tests
├── integration/
│   └── aianalysis/
│       ├── suite_test.go              # Integration suite setup
│       ├── setup_test.go              # KIND cluster setup
│       ├── reconciliation_test.go     # Full reconciliation loop
│       ├── holmesgpt_integration_test.go # HolmesGPT-API integration
│       └── audit_integration_test.go  # Data Storage audit tests
└── e2e/
    └── aianalysis/
        ├── suite_test.go              # E2E suite setup
        ├── full_flow_test.go          # Complete user journey
        └── health_endpoints_test.go   # Health/metrics endpoints
```

---

## 📝 Suite Setup Templates

### Unit Test Suite

```go
// test/unit/aianalysis/suite_test.go
package aianalysis

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestAIAnalysis(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "AIAnalysis Unit Test Suite")
}
```

### Integration Test Suite

```go
// test/integration/aianalysis/suite_test.go
package aianalysis

import (
    "context"
    "os"
    "os/exec"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "k8s.io/client-go/kubernetes/scheme"
    "k8s.io/client-go/rest"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

var (
    cfg       *rest.Config
    k8sClient client.Client
    testEnv   *envtest.Environment
    ctx       context.Context
    cancel    context.CancelFunc

    // MockLLMServer process
    mockLLMProcess *exec.Cmd
    mockLLMURL     string
)

func TestAIAnalysisIntegration(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "AIAnalysis Integration Test Suite")
}

var _ = BeforeSuite(func() {
    ctx, cancel = context.WithCancel(context.Background())

    // Start KIND cluster (or use existing)
    By("Setting up KIND cluster")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths:     []string{"../../../config/crd/bases"},
        ErrorIfCRDPathMissing: true,
    }

    var err error
    cfg, err = testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    Expect(cfg).NotTo(BeNil())

    // Register scheme
    err = aianalysisv1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())

    // Create client
    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())
    Expect(k8sClient).NotTo(BeNil())

    // Start MockLLMServer
    By("Starting MockLLMServer")
    mockLLMProcess = exec.CommandContext(ctx,
        "python3", "../../../holmesgpt-api/tests/mock_llm_server.py",
        "--port", "11434",
    )
    err = mockLLMProcess.Start()
    Expect(err).NotTo(HaveOccurred())

    // Wait for MockLLMServer to be ready
    mockLLMURL = "http://localhost:11434"
    Eventually(func() error {
        resp, err := http.Get(mockLLMURL + "/health")
        if err != nil {
            return err
        }
        defer resp.Body.Close()
        return nil
    }, 30*time.Second, 1*time.Second).Should(Succeed())

    // Set environment for HolmesGPT-API
    os.Setenv("LLM_ENDPOINT", mockLLMURL)
    os.Setenv("LLM_MODEL", "mock-model")
})

var _ = AfterSuite(func() {
    By("Stopping MockLLMServer")
    if mockLLMProcess != nil && mockLLMProcess.Process != nil {
        mockLLMProcess.Process.Kill()
    }

    By("Tearing down test environment")
    cancel()
    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())
})
```

---

## 🎯 Table-Driven Test Patterns

### When to Use Table-Driven Tests

| Scenario | Use Table-Driven? | Reason |
|----------|------------------|--------|
| Multiple inputs, same logic | ✅ Yes | Reduce duplication |
| Different error conditions | ✅ Yes | Comprehensive coverage |
| Boundary conditions | ✅ Yes | Edge case coverage |
| Complex setup per test | ❌ No | Setup varies too much |
| One-off unique tests | ❌ No | No pattern to table |

### ValidatingHandler Table-Driven Tests

```go
// test/unit/aianalysis/validating_handler_test.go
package aianalysis

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
    sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("ValidatingHandler", func() {
    var handler *handlers.ValidatingHandler

    BeforeEach(func() {
        handler = handlers.NewValidatingHandler(
            handlers.WithLogger(testLogger),
            handlers.WithMetrics(testMetrics),
        )
    })

    // Table-driven tests for spec validation (BR-AI-001)
    DescribeTable("validates AIAnalysis spec fields",
        func(description string, spec aianalysisv1.AIAnalysisSpec, expectValid bool, expectedError string) {
            analysis := &aianalysisv1.AIAnalysis{
                Spec: spec,
            }

            err := handler.ValidateSpec(ctx, analysis)

            if expectValid {
                Expect(err).NotTo(HaveOccurred(), "Expected valid spec: %s", description)
            } else {
                Expect(err).To(HaveOccurred(), "Expected invalid spec: %s", description)
                Expect(err.Error()).To(ContainSubstring(expectedError))
            }
        },
        // Happy Path entries
        Entry("valid complete spec - BR-AI-001",
            "all required fields present",
            validAIAnalysisSpec(),
            true, "",
        ),
        Entry("valid spec with minimal fields - BR-AI-001",
            "only required fields",
            minimalAIAnalysisSpec(),
            true, "",
        ),

        // Edge Cases: Missing required fields
        Entry("missing signalContext - BR-AI-002",
            "signalContext is required",
            aianalysisv1.AIAnalysisSpec{},
            false, "signalContext is required",
        ),
        Entry("missing environment - BR-AI-002",
            "environment is required",
            specWithoutEnvironment(),
            false, "environment is required",
        ),
        Entry("missing targetResource - BR-AI-002",
            "targetResource is required",
            specWithoutTargetResource(),
            false, "targetResource is required",
        ),

        // Edge Cases: Invalid values
        Entry("empty environment string - BR-AI-003",
            "environment cannot be empty",
            specWithEmptyEnvironment(),
            false, "environment cannot be empty",
        ),
        Entry("environment too long - BR-AI-003",
            "environment exceeds 63 characters",
            specWithLongEnvironment(64),
            false, "environment exceeds maximum length",
        ),

        // Edge Cases: FailedDetections validation
        Entry("valid FailedDetections - DD-WORKFLOW-001",
            "known field names in FailedDetections",
            specWithFailedDetections([]string{"gitOpsManaged", "pdbProtected"}),
            true, "",
        ),
        Entry("invalid FailedDetections field - DD-WORKFLOW-001",
            "unknown field name rejected",
            specWithFailedDetections([]string{"unknownField"}),
            false, "invalid FailedDetections field: unknownField",
        ),

        // Edge Cases: nil handling
        Entry("nil detectedLabels - graceful handling",
            "nil detectedLabels is valid",
            specWithNilDetectedLabels(),
            true, "",
        ),
        Entry("nil customLabels - graceful handling",
            "nil customLabels is valid",
            specWithNilCustomLabels(),
            true, "",
        ),
    )

    // Table-driven tests for EnrichmentResults validation (BR-AI-005)
    DescribeTable("validates EnrichmentResults structure",
        func(description string, enrichment *sharedtypes.EnrichmentResults, expectValid bool) {
            analysis := &aianalysisv1.AIAnalysis{
                Spec: aianalysisv1.AIAnalysisSpec{
                    SignalContext: aianalysisv1.SignalContextInput{
                        EnrichmentResults: enrichment,
                    },
                },
            }

            err := handler.ValidateEnrichmentResults(ctx, analysis)

            if expectValid {
                Expect(err).NotTo(HaveOccurred())
            } else {
                Expect(err).To(HaveOccurred())
            }
        },
        Entry("valid complete EnrichmentResults",
            "all fields populated",
            validEnrichmentResults(),
            true,
        ),
        Entry("nil EnrichmentResults",
            "nil is invalid",
            nil,
            false,
        ),
        Entry("empty OwnerChain",
            "empty OwnerChain is valid (leaf resource)",
            enrichmentWithEmptyOwnerChain(),
            true,
        ),
    )
})

// Helper functions for test data
func validAIAnalysisSpec() aianalysisv1.AIAnalysisSpec {
    return aianalysisv1.AIAnalysisSpec{
        SignalContext: aianalysisv1.SignalContextInput{
            Environment:      "production",
            BusinessPriority: "P1",
            TargetResource: aianalysisv1.TargetResource{
                Kind:      "Pod",
                Name:      "web-app-xyz",
                Namespace: "default",
            },
            EnrichmentResults: validEnrichmentResults(),
        },
    }
}

func minimalAIAnalysisSpec() aianalysisv1.AIAnalysisSpec {
    return aianalysisv1.AIAnalysisSpec{
        SignalContext: aianalysisv1.SignalContextInput{
            Environment:      "dev",
            BusinessPriority: "P3",
            TargetResource: aianalysisv1.TargetResource{
                Kind:      "Deployment",
                Name:      "test-app",
                Namespace: "test",
            },
            EnrichmentResults: &sharedtypes.EnrichmentResults{},
        },
    }
}

func specWithFailedDetections(fields []string) aianalysisv1.AIAnalysisSpec {
    spec := validAIAnalysisSpec()
    spec.SignalContext.EnrichmentResults.DetectedLabels = &sharedtypes.DetectedLabels{
        FailedDetections: fields,
    }
    return spec
}
```

### InvestigatingHandler Table-Driven Tests

```go
// test/unit/aianalysis/investigating_handler_test.go
package aianalysis

import (
    "errors"
    "net/http"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
    "github.com/jordigilh/kubernaut/pkg/clients/holmesgpt"
)

var _ = Describe("InvestigatingHandler", func() {
    var (
        handler     *handlers.InvestigatingHandler
        mockClient  *MockHolmesGPTClient
    )

    BeforeEach(func() {
        mockClient = NewMockHolmesGPTClient()
        handler = handlers.NewInvestigatingHandler(
            handlers.WithHolmesGPTClient(mockClient),
            handlers.WithLogger(testLogger),
            handlers.WithMetrics(testMetrics),
        )
    })

    // Table-driven tests for HolmesGPT-API responses (BR-AI-006 to BR-AI-010)
    DescribeTable("processes HolmesGPT-API responses",
        func(description string, response *holmesgpt.IncidentResponse, apiErr error, expectedPhase string, expectError bool) {
            mockClient.SetResponse(response, apiErr)

            analysis := validAIAnalysis()
            analysis.Status.Phase = aianalysisv1.PhaseInvestigating

            result, err := handler.Handle(ctx, analysis)

            if expectError {
                Expect(err).To(HaveOccurred(), "Expected error: %s", description)
            } else {
                Expect(err).NotTo(HaveOccurred(), "Unexpected error: %s", description)
                Expect(analysis.Status.Phase).To(Equal(expectedPhase))
            }
            _ = result // May check RequeueAfter for retry tests
        },
        // Happy Path: Successful responses
        Entry("successful investigation - BR-AI-006",
            "HolmesGPT-API returns valid analysis",
            validIncidentResponse(),
            nil,
            aianalysisv1.PhaseAnalyzing,
            false,
        ),
        Entry("response with targetInOwnerChain=true - BR-AI-007",
            "target found in owner chain",
            responseWithTargetInOwnerChain(true),
            nil,
            aianalysisv1.PhaseAnalyzing,
            false,
        ),
        Entry("response with targetInOwnerChain=false - BR-AI-007",
            "target NOT in owner chain (data quality warning)",
            responseWithTargetInOwnerChain(false),
            nil,
            aianalysisv1.PhaseAnalyzing,
            false,
        ),
        Entry("response with warnings - BR-AI-008",
            "warnings are captured but don't block",
            responseWithWarnings([]string{"High memory pressure detected"}),
            nil,
            aianalysisv1.PhaseAnalyzing,
            false,
        ),

        // Error Cases: Transient errors (Category B)
        Entry("HolmesGPT-API 503 - retry - BR-AI-009",
            "service unavailable triggers retry",
            nil,
            &holmesgpt.APIError{StatusCode: http.StatusServiceUnavailable},
            aianalysisv1.PhaseInvestigating, // Stays in same phase
            true, // Returns error for requeue
        ),
        Entry("HolmesGPT-API 429 - rate limit - BR-AI-009",
            "rate limiting triggers retry",
            nil,
            &holmesgpt.APIError{StatusCode: http.StatusTooManyRequests},
            aianalysisv1.PhaseInvestigating,
            true,
        ),
        Entry("HolmesGPT-API timeout - retry - BR-AI-009",
            "timeout triggers retry",
            nil,
            errors.New("context deadline exceeded"),
            aianalysisv1.PhaseInvestigating,
            true,
        ),

        // Error Cases: Permanent errors (Category C)
        Entry("HolmesGPT-API 401 - auth failure - BR-AI-010",
            "authentication failure is permanent",
            nil,
            &holmesgpt.APIError{StatusCode: http.StatusUnauthorized},
            aianalysisv1.PhaseFailed,
            false, // No retry, but status updated
        ),
        Entry("HolmesGPT-API 400 - bad request - BR-AI-010",
            "bad request is permanent",
            nil,
            &holmesgpt.APIError{StatusCode: http.StatusBadRequest},
            aianalysisv1.PhaseFailed,
            false,
        ),
    )

    // Table-driven tests for retry behavior
    DescribeTable("implements retry with exponential backoff",
        func(retryCount int, expectedMinDelay, expectedMaxDelay time.Duration) {
            mockClient.SetResponse(nil, &holmesgpt.APIError{StatusCode: 503})

            analysis := validAIAnalysis()
            analysis.Status.Phase = aianalysisv1.PhaseInvestigating
            analysis.Status.RetryCount = retryCount

            result, _ := handler.Handle(ctx, analysis)

            Expect(result.RequeueAfter).To(BeNumerically(">=", expectedMinDelay))
            Expect(result.RequeueAfter).To(BeNumerically("<=", expectedMaxDelay))
        },
        Entry("first retry (attempt 0)", 0, 27*time.Second, 33*time.Second),   // ~30s ±10%
        Entry("second retry (attempt 1)", 1, 54*time.Second, 66*time.Second),  // ~60s ±10%
        Entry("third retry (attempt 2)", 2, 108*time.Second, 132*time.Second), // ~120s ±10%
        Entry("fourth retry (attempt 3)", 3, 216*time.Second, 264*time.Second), // ~240s ±10%
        Entry("fifth retry (attempt 4)", 4, 432*time.Second, 528*time.Second), // ~480s ±10%
        Entry("max retry (attempt 5+)", 5, 432*time.Second, 528*time.Second),  // Capped at 480s
    )
})

// Mock HolmesGPT client for testing
type MockHolmesGPTClient struct {
    response *holmesgpt.IncidentResponse
    err      error
}

func NewMockHolmesGPTClient() *MockHolmesGPTClient {
    return &MockHolmesGPTClient{}
}

func (m *MockHolmesGPTClient) SetResponse(response *holmesgpt.IncidentResponse, err error) {
    m.response = response
    m.err = err
}

func (m *MockHolmesGPTClient) Investigate(ctx context.Context, request *holmesgpt.IncidentRequest) (*holmesgpt.IncidentResponse, error) {
    return m.response, m.err
}

// Helper functions
func validIncidentResponse() *holmesgpt.IncidentResponse {
    return &holmesgpt.IncidentResponse{
        Analysis:           "Root cause identified: OOMKilled due to memory leak",
        TargetInOwnerChain: true,
        Confidence:         0.85,
    }
}

func responseWithTargetInOwnerChain(found bool) *holmesgpt.IncidentResponse {
    resp := validIncidentResponse()
    resp.TargetInOwnerChain = found
    return resp
}

func responseWithWarnings(warnings []string) *holmesgpt.IncidentResponse {
    resp := validIncidentResponse()
    resp.Warnings = warnings
    return resp
}
```

---

## 🔄 Integration Test Patterns

### Full Reconciliation Loop Test

```go
// test/integration/aianalysis/reconciliation_test.go
package aianalysis

import (
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

var _ = Describe("AIAnalysis Reconciliation Loop", func() {
    const (
        timeout  = 60 * time.Second
        interval = 1 * time.Second
    )

    Context("when creating a new AIAnalysis CRD", func() {
        It("should complete full reconciliation cycle - BR-AI-001", func() {
            By("Creating AIAnalysis CRD")
            analysis := &aianalysisv1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-analysis",
                    Namespace: "default",
                },
                Spec: validAIAnalysisSpec(),
            }
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            By("Waiting for Validating phase")
            Eventually(func() string {
                if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
                    return ""
                }
                return string(analysis.Status.Phase)
            }, timeout, interval).Should(Equal("Validating"))

            By("Waiting for Investigating phase")
            Eventually(func() string {
                if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
                    return ""
                }
                return string(analysis.Status.Phase)
            }, timeout, interval).Should(Equal("Investigating"))

            By("Waiting for Analyzing phase")
            Eventually(func() string {
                if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
                    return ""
                }
                return string(analysis.Status.Phase)
            }, timeout, interval).Should(Equal("Analyzing"))

            By("Waiting for Recommending phase")
            Eventually(func() string {
                if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
                    return ""
                }
                return string(analysis.Status.Phase)
            }, timeout, interval).Should(Equal("Recommending"))

            By("Waiting for Completed phase")
            Eventually(func() string {
                if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
                    return ""
                }
                return string(analysis.Status.Phase)
            }, 2*timeout, interval).Should(Equal("Completed"))

            By("Verifying final status")
            Expect(analysis.Status.Recommendations).NotTo(BeEmpty())
            Expect(analysis.Status.ApprovalRequired).To(BeTrue()) // Or false, depending on Rego
        })
    })
})
```

### HolmesGPT-API Integration Test

```go
// test/integration/aianalysis/holmesgpt_integration_test.go
package aianalysis

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/clients/holmesgpt"
)

var _ = Describe("HolmesGPT-API Integration", func() {
    var client holmesgpt.Client

    BeforeEach(func() {
        var err error
        client, err = holmesgpt.NewClient(holmesgpt.Config{
            BaseURL: "http://localhost:8080",
            Timeout: 30 * time.Second,
        })
        Expect(err).NotTo(HaveOccurred())
    })

    Context("when calling /api/v1/incident/analyze", func() {
        It("should return valid analysis - BR-AI-006", func() {
            request := &holmesgpt.IncidentRequest{
                Context: "Pod CrashLoopBackOff in production namespace",
                DetectedLabels: map[string]interface{}{
                    "gitOpsManaged": true,
                    "hpaEnabled":    true,
                },
            }

            response, err := client.Analyze(ctx, request)

            Expect(err).NotTo(HaveOccurred())
            Expect(response).NotTo(BeNil())
            Expect(response.Analysis).NotTo(BeEmpty())
            Expect(response.Confidence).To(BeNumerically(">", 0))
        })

        It("should include targetInOwnerChain in response - BR-AI-007", func() {
            request := &holmesgpt.IncidentRequest{
                Context: "Test request",
            }

            response, err := client.Analyze(ctx, request)

            Expect(err).NotTo(HaveOccurred())
            // targetInOwnerChain should be set (true or false)
            Expect(response.TargetInOwnerChain).To(BeAssignableToTypeOf(true))
        })
    })
})
```

---

## 🏃 E2E Test Patterns

### Health Endpoint Tests

```go
// test/e2e/aianalysis/health_endpoints_test.go
package aianalysis

import (
    "net/http"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

const (
    // From DD-TEST-001 port allocation
    AIAnalysisHealthURL  = "http://localhost:8184/healthz"
    AIAnalysisReadyURL   = "http://localhost:8184/readyz"
    AIAnalysisMetricsURL = "http://localhost:9184/metrics"
)

var _ = Describe("AIAnalysis E2E Health Endpoints", func() {
    Context("Health endpoints", func() {
        It("should return 200 on /healthz", func() {
            resp, err := http.Get(AIAnalysisHealthURL)
            Expect(err).NotTo(HaveOccurred())
            defer resp.Body.Close()
            Expect(resp.StatusCode).To(Equal(http.StatusOK))
        })

        It("should return 200 on /readyz when ready", func() {
            resp, err := http.Get(AIAnalysisReadyURL)
            Expect(err).NotTo(HaveOccurred())
            defer resp.Body.Close()
            Expect(resp.StatusCode).To(Equal(http.StatusOK))
        })
    })

    Context("Metrics endpoint", func() {
        It("should expose Prometheus metrics", func() {
            resp, err := http.Get(AIAnalysisMetricsURL)
            Expect(err).NotTo(HaveOccurred())
            defer resp.Body.Close()
            Expect(resp.StatusCode).To(Equal(http.StatusOK))

            body, err := io.ReadAll(resp.Body)
            Expect(err).NotTo(HaveOccurred())

            // Verify expected metrics exist
            Expect(string(body)).To(ContainSubstring("aianalysis_reconciliation_total"))
            Expect(string(body)).To(ContainSubstring("aianalysis_phase_duration_seconds"))
            Expect(string(body)).To(ContainSubstring("aianalysis_holmesgpt_api_calls_total"))
        })
    })
})
```

---

## 🧰 Test Utilities

### Test Fixtures

```go
// test/unit/aianalysis/fixtures_test.go
package aianalysis

import (
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// validAIAnalysis creates a fully valid AIAnalysis for testing
func validAIAnalysis() *aianalysisv1.AIAnalysis {
    return &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-analysis",
            Namespace: "default",
        },
        Spec: validAIAnalysisSpec(),
        Status: aianalysisv1.AIAnalysisStatus{
            Phase: aianalysisv1.PhaseValidating,
        },
    }
}

// validEnrichmentResults creates valid EnrichmentResults for testing
func validEnrichmentResults() *sharedtypes.EnrichmentResults {
    return &sharedtypes.EnrichmentResults{
        DetectedLabels: &sharedtypes.DetectedLabels{
            GitOpsManaged:   true,
            GitOpsTool:      "argocd",
            PDBProtected:    true,
            HPAEnabled:      false,
            Stateful:        false,
            HelmManaged:     true,
            NetworkIsolated: true,
            ServiceMesh:     "istio",
        },
        OwnerChain: []sharedtypes.OwnerChainEntry{
            {Namespace: "default", Kind: "Deployment", Name: "web-app"},
            {Namespace: "default", Kind: "ReplicaSet", Name: "web-app-abc123"},
        },
        CustomLabels: map[string][]string{
            "team":        {"platform"},
            "cost_center": {"engineering"},
        },
        KubernetesContext: &sharedtypes.KubernetesContext{
            Namespace: "default",
        },
    }
}

// enrichmentWithEmptyOwnerChain creates EnrichmentResults with empty OwnerChain
func enrichmentWithEmptyOwnerChain() *sharedtypes.EnrichmentResults {
    result := validEnrichmentResults()
    result.OwnerChain = []sharedtypes.OwnerChainEntry{}
    return result
}
```

### Test Logger and Metrics

```go
// test/unit/aianalysis/helpers_test.go
package aianalysis

import (
    "github.com/go-logr/logr"
    "github.com/go-logr/logr/testr"
    "testing"
)

var (
    testLogger  logr.Logger
    testMetrics *MockMetrics
)

func init() {
    // Initialize test logger (discards output)
    testLogger = logr.Discard()
}

// For tests that need to capture logs:
func testLoggerWithT(t *testing.T) logr.Logger {
    return testr.New(t)
}

// MockMetrics for testing
type MockMetrics struct {
    reconciliations int
    errors          map[string]int
}

func NewMockMetrics() *MockMetrics {
    return &MockMetrics{
        errors: make(map[string]int),
    }
}

func (m *MockMetrics) IncrementReconciliation(namespace, phase string) {
    m.reconciliations++
}

func (m *MockMetrics) IncrementError(category string) {
    m.errors[category]++
}
```

---

## ✅ Test Coverage Verification

### Commands

```bash
# Run unit tests with coverage
go test -coverprofile=coverage.out ./pkg/aianalysis/...
go tool cover -func=coverage.out | grep total

# Run integration tests
go test -v -tags=integration ./test/integration/aianalysis/...

# Run E2E tests (requires KIND cluster)
kind create cluster --name aianalysis-e2e --config test/infrastructure/kind-aianalysis-config.yaml
go test -v -tags=e2e ./test/e2e/aianalysis/...

# Generate coverage report
go tool cover -html=coverage.out -o coverage.html
```

### Coverage Targets

| Test Type | Target | Minimum |
|-----------|--------|---------|
| Unit Tests | 70-75% | 60% |
| Integration Tests | 15-20% | 10% |
| E2E Tests | <10% | 5% |

---

## 📚 Related Documents

- [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md) - Main implementation plan
- [testing-strategy.md](../../testing-strategy.md) - Testing strategy overview
- [TESTING_GUIDELINES.md](../../../../../development/business-requirements/TESTING_GUIDELINES.md) - Authoritative testing guidelines
- [APPENDIX_A_EOD_TEMPLATES.md](./APPENDIX_A_EOD_TEMPLATES.md) - EOD documentation templates

