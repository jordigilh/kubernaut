# Days 5-7: Integration Testing & Error Handling

**Part of**: AI Analysis Implementation Plan V1.0
**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../../IMPLEMENTATION_PLAN_V1.0.md)
**Duration**: 18-24 hours (3 days)
**Target Confidence**: 88% (Day 7 Complete)

---

## 🎯 Day 5 Objectives: Error Handling & Metrics

| Objective | Priority | BR Reference |
|-----------|----------|--------------|
| Complete error handling (5 categories) | P0 | BR-AI-021 |
| Implement Prometheus metrics | P0 | BR-AI-022 |
| Add structured logging | P0 | BR-AI-023 |
| Circuit breaker for HolmesGPT-API | P1 | BR-AI-024 |

---

## 🔧 Day 5: Error Handling Implementation

### Metrics Implementation

```go
// pkg/aianalysis/metrics/metrics.go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Reconciliation metrics
    ReconciliationsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_reconciliations_total",
            Help: "Total number of AIAnalysis reconciliations",
        },
        []string{"namespace", "phase", "result"},
    )

    PhaseDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_phase_duration_seconds",
            Help:    "Duration of each reconciliation phase",
            Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
        },
        []string{"phase"},
    )

    // HolmesGPT-API metrics
    HolmesGPTCalls = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_holmesgpt_api_calls_total",
            Help: "Total HolmesGPT-API calls",
        },
        []string{"endpoint", "status"},
    )

    HolmesGPTLatency = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_holmesgpt_api_latency_seconds",
            Help:    "HolmesGPT-API call latency",
            Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60},
        },
        []string{"endpoint"},
    )

    // Error metrics
    ErrorsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_errors_total",
            Help: "Total errors by category (A-E)",
        },
        []string{"category", "phase"},
    )

    RetriesTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_retries_total",
            Help: "Total retry attempts",
        },
        []string{"phase", "reason"},
    )

    // Rego policy metrics
    RegoPolicyEvaluations = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_rego_policy_evaluations_total",
            Help: "Rego policy evaluations",
        },
        []string{"result", "degraded"},
    )

    // Approval metrics
    ApprovalsRequired = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_approvals_required_total",
            Help: "Analyses requiring approval",
        },
        []string{"environment", "reason"},
    )

    // Data quality metrics
    TargetInOwnerChain = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_target_in_owner_chain_total",
            Help: "Target resource owner chain validation results",
        },
        []string{"result"},
    )

    FailedDetections = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_failed_detections_total",
            Help: "Detection failures by field",
        },
        []string{"field"},
    )
)

// Metrics wraps all metric operations
type Metrics struct{}

func NewMetrics() *Metrics {
    return &Metrics{}
}

// IncrementReconciliation records a reconciliation
func (m *Metrics) IncrementReconciliation(namespace, phase, result string) {
    ReconciliationsTotal.WithLabelValues(namespace, phase, result).Inc()
}

// ObservePhaseDuration records phase duration
func (m *Metrics) ObservePhaseDuration(phase string, duration float64) {
    PhaseDuration.WithLabelValues(phase).Observe(duration)
}

// IncrementError records an error by category
func (m *Metrics) IncrementError(category, phase string) {
    ErrorsTotal.WithLabelValues(category, phase).Inc()
}

// IncrementHolmesGPTCall records an API call
func (m *Metrics) IncrementHolmesGPTCall(endpoint, status string) {
    HolmesGPTCalls.WithLabelValues(endpoint, status).Inc()
}

// ObserveHolmesGPTLatency records API latency
func (m *Metrics) ObserveHolmesGPTLatency(endpoint string, latency float64) {
    HolmesGPTLatency.WithLabelValues(endpoint).Observe(latency)
}
```

### Metrics Tests

```go
// test/unit/aianalysis/metrics_test.go
package aianalysis

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
    "github.com/prometheus/client_golang/prometheus/testutil"
)

var _ = Describe("Metrics", func() {
    var m *metrics.Metrics

    BeforeEach(func() {
        m = metrics.NewMetrics()
    })

    Describe("IncrementReconciliation", func() {
        It("should increment reconciliation counter - BR-AI-022", func() {
            m.IncrementReconciliation("default", "Validating", "success")

            count := testutil.ToFloat64(metrics.ReconciliationsTotal.WithLabelValues("default", "Validating", "success"))
            Expect(count).To(BeNumerically(">=", 1))
        })
    })

    Describe("IncrementError", func() {
        DescribeTable("tracks errors by category",
            func(category string) {
                m.IncrementError(category, "Investigating")

                count := testutil.ToFloat64(metrics.ErrorsTotal.WithLabelValues(category, "Investigating"))
                Expect(count).To(BeNumerically(">=", 1))
            },
            Entry("Category A", "A"),
            Entry("Category B", "B"),
            Entry("Category C", "C"),
            Entry("Category D", "D"),
            Entry("Category E", "E"),
        )
    })
})
```

---

## 🎯 Day 6 Objectives: Integration Test Setup

| Objective | Priority | BR Reference |
|-----------|----------|--------------|
| Set up KIND cluster | P0 | — |
| Deploy MockLLMServer | P0 | — |
| Full reconciliation loop test | P0 | BR-AI-001 |
| Create Error Handling Philosophy doc | P0 | Template |

---

## 🔧 Day 6: Integration Test Infrastructure

### KIND Cluster Setup

```go
// test/integration/aianalysis/setup_test.go
package aianalysis

import (
    "context"
    "os"
    "os/exec"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "k8s.io/client-go/kubernetes/scheme"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

var (
    k8sClient     client.Client
    testEnv       *envtest.Environment
    ctx           context.Context
    cancel        context.CancelFunc
    mockLLMServer *exec.Cmd
)

var _ = BeforeSuite(func() {
    ctx, cancel = context.WithCancel(context.Background())

    By("Starting KIND cluster with envtest")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths:     []string{"../../../../config/crd/bases"},
        ErrorIfCRDPathMissing: true,
        // Use existing KIND cluster if available
        UseExistingCluster: ptr.To(true),
    }

    cfg, err := testEnv.Start()
    Expect(err).NotTo(HaveOccurred())
    Expect(cfg).NotTo(BeNil())

    // Register AIAnalysis scheme
    err = aianalysisv1.AddToScheme(scheme.Scheme)
    Expect(err).NotTo(HaveOccurred())

    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).NotTo(HaveOccurred())

    By("Starting MockLLMServer")
    mockLLMServer = exec.CommandContext(ctx,
        "python3", "../../../../holmesgpt-api/tests/mock_llm_server.py",
        "--port", "11434",
    )
    err = mockLLMServer.Start()
    Expect(err).NotTo(HaveOccurred())

    // Wait for MockLLMServer
    Eventually(func() error {
        resp, err := http.Get("http://localhost:11434/health")
        if err != nil {
            return err
        }
        resp.Body.Close()
        return nil
    }, 30*time.Second, time.Second).Should(Succeed())

    // Set LLM environment
    os.Setenv("LLM_ENDPOINT", "http://localhost:11434")
    os.Setenv("LLM_MODEL", "mock-model")
})

var _ = AfterSuite(func() {
    By("Stopping MockLLMServer")
    if mockLLMServer != nil && mockLLMServer.Process != nil {
        mockLLMServer.Process.Kill()
    }

    By("Tearing down envtest")
    cancel()
    err := testEnv.Stop()
    Expect(err).NotTo(HaveOccurred())
})
```

### Full Reconciliation Loop Test

```go
// test/integration/aianalysis/reconciliation_test.go
package aianalysis

import (
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("AIAnalysis Full Reconciliation", func() {
    const (
        timeout  = 2 * time.Minute
        interval = time.Second
    )

    Context("Complete reconciliation cycle", func() {
        var analysis *aianalysisv1.AIAnalysis

        BeforeEach(func() {
            analysis = &aianalysisv1.AIAnalysis{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "integration-test-" + randomSuffix(),
                    Namespace: "default",
                },
                Spec: aianalysisv1.AIAnalysisSpec{
                    SignalContext: aianalysisv1.SignalContextInput{
                        Environment:      "staging",
                        BusinessPriority: "P2",
                        TargetResource: aianalysisv1.TargetResource{
                            Kind:      "Pod",
                            Name:      "test-pod",
                            Namespace: "default",
                        },
                        EnrichmentResults: &sharedtypes.EnrichmentResults{
                            DetectedLabels: &sharedtypes.DetectedLabels{
                                GitOpsManaged: true,
                                PDBProtected:  true,
                            },
                            OwnerChain: []sharedtypes.OwnerChainEntry{
                                {Namespace: "default", Kind: "Deployment", Name: "test-app"},
                            },
                        },
                    },
                },
            }
        })

        AfterEach(func() {
            // Cleanup
            _ = k8sClient.Delete(ctx, analysis)
        })

        It("should complete all phases successfully - BR-AI-001", func() {
            By("Creating AIAnalysis CRD")
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            By("Waiting for Validating phase")
            Eventually(func() string {
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
                return string(analysis.Status.Phase)
            }, timeout, interval).Should(Equal("Validating"))

            By("Waiting for Investigating phase")
            Eventually(func() string {
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
                return string(analysis.Status.Phase)
            }, timeout, interval).Should(Equal("Investigating"))

            By("Waiting for Analyzing phase")
            Eventually(func() string {
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
                return string(analysis.Status.Phase)
            }, timeout, interval).Should(Equal("Analyzing"))

            By("Waiting for Recommending phase")
            Eventually(func() string {
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
                return string(analysis.Status.Phase)
            }, timeout, interval).Should(Equal("Recommending"))

            By("Waiting for Completed phase")
            Eventually(func() string {
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
                return string(analysis.Status.Phase)
            }, timeout, interval).Should(Equal("Completed"))

            By("Verifying final status")
            Expect(analysis.Status.CompletedAt).NotTo(BeZero())
            // Staging environment should auto-approve
            Expect(analysis.Status.ApprovalRequired).To(BeFalse())
        })

        It("should require approval for production - BR-AI-013", func() {
            analysis.Spec.SignalContext.Environment = "production"

            By("Creating production AIAnalysis")
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            By("Waiting for completion")
            Eventually(func() string {
                _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
                return string(analysis.Status.Phase)
            }, timeout, interval).Should(Equal("Completed"))

            By("Verifying approval required")
            Expect(analysis.Status.ApprovalRequired).To(BeTrue())
        })
    })
})
```

---

## 🎯 Day 7 Objectives: Integration Test Completion

| Objective | Priority | BR Reference |
|-----------|----------|--------------|
| HolmesGPT-API integration tests | P0 | BR-AI-006 |
| Rego policy integration tests | P0 | BR-AI-011 |
| Error recovery tests | P0 | BR-AI-009 |
| Day 7 Complete checkpoint | P0 | — |

---

## 🔧 Day 7: Integration Test Completion

### HolmesGPT-API Integration Test

```go
// test/integration/aianalysis/holmesgpt_integration_test.go
package aianalysis

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/aianalysis/client"
)

var _ = Describe("HolmesGPT-API Integration", func() {
    var hgClient *client.HolmesGPTClient

    BeforeEach(func() {
        hgClient = client.NewHolmesGPTClient(client.Config{
            BaseURL: "http://localhost:8080", // HolmesGPT-API in KIND
            Timeout: 60 * time.Second,
        })
    })

    Context("Incident Analysis", func() {
        It("should return valid analysis - BR-AI-006", func() {
            resp, err := hgClient.Investigate(ctx, &client.IncidentRequest{
                Context: "Pod CrashLoopBackOff in staging",
                DetectedLabels: map[string]interface{}{
                    "gitOpsManaged": true,
                },
            })

            Expect(err).NotTo(HaveOccurred())
            Expect(resp.Analysis).NotTo(BeEmpty())
            Expect(resp.Confidence).To(BeNumerically(">", 0))
        })

        It("should include targetInOwnerChain - BR-AI-007", func() {
            resp, err := hgClient.Investigate(ctx, &client.IncidentRequest{
                Context: "Test",
            })

            Expect(err).NotTo(HaveOccurred())
            // Should be true or false, not nil
            Expect(resp.TargetInOwnerChain).To(BeAssignableToTypeOf(true))
        })
    })

    Context("Recovery Suggestions", func() {
        It("should return workflow recommendations - BR-AI-016", func() {
            resp, err := hgClient.GetRecoverySuggestions(ctx, &client.RecoveryRequest{
                IncidentContext: "OOM detected",
                Environment:     "staging",
            })

            Expect(err).NotTo(HaveOccurred())
            // MockLLMServer should return at least one recommendation
            Expect(resp.RecommendedWorkflows).NotTo(BeEmpty())
        })
    })
})
```

### Rego Policy Integration Test

```go
// test/integration/aianalysis/rego_integration_test.go
package aianalysis

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

var _ = Describe("Rego Policy Integration", func() {
    var evaluator *rego.Evaluator

    BeforeEach(func() {
        // Use policies from ConfigMap in KIND cluster
        evaluator = rego.NewEvaluator(rego.Config{
            PolicyDir: "../../../../config/rego/aianalysis",
        })
    })

    Context("Approval Policy", func() {
        It("should auto-approve staging - BR-AI-013", func() {
            result, err := evaluator.Evaluate(ctx, &rego.PolicyInput{
                Environment:        "staging",
                TargetInOwnerChain: true,
                FailedDetections:   []string{},
            })

            Expect(err).NotTo(HaveOccurred())
            Expect(result.ApprovalRequired).To(BeFalse())
            Expect(result.Degraded).To(BeFalse())
        })

        It("should require approval for production with data quality issues - BR-AI-013", func() {
            result, err := evaluator.Evaluate(ctx, &rego.PolicyInput{
                Environment:        "production",
                TargetInOwnerChain: false, // Data quality issue
                FailedDetections:   []string{"gitOpsManaged"},
            })

            Expect(err).NotTo(HaveOccurred())
            Expect(result.ApprovalRequired).To(BeTrue())
        })
    })
})
```

---

## ⭐ Day 7 Complete Checkpoint

### Validation Commands

```bash
# Build verification
go build ./pkg/aianalysis/...

# Unit tests
go test -v ./test/unit/aianalysis/...

# Integration tests (requires KIND cluster)
kind create cluster --name aianalysis-e2e --config test/infrastructure/kind-aianalysis-config.yaml
go test -v -tags=integration ./test/integration/aianalysis/...

# Coverage
go test -coverprofile=coverage.out ./pkg/aianalysis/...
go tool cover -func=coverage.out | grep total
```

### Day 7 Confidence Assessment

| Component | Target | Actual | Notes |
|-----------|--------|--------|-------|
| Unit Tests | 95% | — | All handlers tested |
| Integration Tests | 90% | — | Full loop working |
| Error Handling | 95% | — | All 5 categories implemented |
| Metrics | 90% | — | All metrics implemented |
| **Overall** | **88%** | — | Day 7 target |

### EOD Documentation

Create: `docs/services/crd-controllers/02-aianalysis/implementation/phase0/03-day7-complete.md`

---

## 📚 Related Documents

- [DAY_03_04_ANALYZING_RECOMMENDING.md](./DAY_03_04_ANALYZING_RECOMMENDING.md) - Previous days
- [DAY_08_10_E2E_POLISH.md](./DAY_08_10_E2E_POLISH.md) - Next phase
- [APPENDIX_B_ERROR_HANDLING_PHILOSOPHY.md](../appendices/APPENDIX_B_ERROR_HANDLING_PHILOSOPHY.md) - Error patterns

