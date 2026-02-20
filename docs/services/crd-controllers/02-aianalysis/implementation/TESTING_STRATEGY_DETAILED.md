# Testing Strategy - AI Analysis Service (Detailed)

> **Note (ADR-056/ADR-055):** References to `EnrichmentResults.DetectedLabels` and `EnrichmentResults.OwnerChain` in this document are historical. These fields were removed per ADR-056 and ADR-055.

**Date**: 2025-12-04
**Status**: 📋 Template - Complete at Day 6-8
**Version**: 1.0
**Parent**: [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md)

---

## 🎯 **Testing Overview**

### **Test Pyramid**

```
                    ┌─────────────┐
                    │    E2E      │  10-15% (3-5 tests)
                    │  (Day 8)    │  90-95% confidence
                    ├─────────────┤
               ┌────┴─────────────┴────┐
               │    Integration        │  20-30% (15-20 tests)
               │      (Day 7)          │  80-85% confidence
               ├───────────────────────┤
          ┌────┴───────────────────────┴────┐
          │          Unit Tests             │  60-70% (40+ tests)
          │           (Day 6)               │  85-90% confidence
          └─────────────────────────────────┘
```

### **Parallel Execution Standard**

All tests run with **4 concurrent processes**:

```bash
# Unit tests
go test -p 4 ./pkg/aianalysis/...

# Ginkgo tests
ginkgo -procs=4 ./test/unit/aianalysis/...
ginkgo -procs=4 ./test/integration/aianalysis/...
```

---

## 🧪 **Day 6: Unit Tests (8h)**

### **Test Files Structure**

```
test/unit/aianalysis/
├── suite_test.go              # Ginkgo suite setup
├── controller_test.go         # Reconciler unit tests
├── validating_test.go         # Validating phase tests
├── investigating_test.go      # Investigating phase tests
├── analyzing_test.go          # Analyzing phase (Rego) tests
├── recommending_test.go       # Recommending phase tests
├── holmesgpt_client_test.go   # HolmesGPT client tests
├── rego_engine_test.go        # Rego engine unit tests
└── metrics_test.go            # Metrics recording tests
```

### **Suite Setup**

```go
// test/unit/aianalysis/suite_test.go
package aianalysis

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "k8s.io/client-go/kubernetes/scheme"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
)

func TestAIAnalysis(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "AIAnalysis Controller Suite")
}

var _ = BeforeSuite(func() {
    // Register CRD schemes
    Expect(aianalysisv1.AddToScheme(scheme.Scheme)).To(Succeed())
    Expect(signalprocessingv1.AddToScheme(scheme.Scheme)).To(Succeed())
})
```

### **Controller Unit Tests**

```go
// test/unit/aianalysis/controller_test.go
package aianalysis

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/internal/controller/aianalysis"
    "github.com/jordigilh/kubernaut/pkg/testutil"
    sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"
    ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("BR-AI-001: AIAnalysis Controller", func() {
    var (
        ctx           context.Context
        fakeClient    client.Client
        reconciler    *aianalysis.Reconciler
        mockHolmesGPT *testutil.MockHolmesGPTClient
        mockRego      *testutil.MockRegoEngine
        scheme        *runtime.Scheme
    )

    BeforeEach(func() {
        ctx = context.Background()
        scheme = testutil.NewTestScheme()

        fakeClient = fake.NewClientBuilder().
            WithScheme(scheme).
            WithStatusSubresource(&aianalysisv1.AIAnalysis{}).
            Build()

        mockHolmesGPT = testutil.NewMockHolmesGPTClient()
        mockRego = testutil.NewMockRegoEngine()

        reconciler = &aianalysis.Reconciler{
            Client:       fakeClient,
            Scheme:       scheme,
            Log:          ctrl.Log.WithName("test"),
            HolmesGPT:    mockHolmesGPT,
            RegoEngine:   mockRego,
        }
    })

    Context("Phase Transitions", func() {
        DescribeTable("should transition phases correctly",
            func(initialPhase, expectedPhase string, setupMocks func()) {
                setupMocks()

                analysis := createTestAnalysis("test-analysis", initialPhase)
                Expect(fakeClient.Create(ctx, analysis)).To(Succeed())

                _, err := reconciler.Reconcile(ctx, ctrl.Request{
                    NamespacedName: client.ObjectKeyFromObject(analysis),
                })
                Expect(err).ToNot(HaveOccurred())

                var updated aianalysisv1.AIAnalysis
                Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
                Expect(updated.Status.Phase).To(Equal(expectedPhase))
            },
            Entry("Pending → Validating",
                "", "Validating",
                func() {}),
            Entry("Validating → Investigating (valid input)",
                "Validating", "Investigating",
                func() {}),
            Entry("Investigating → Analyzing (HolmesGPT success)",
                "Investigating", "Analyzing",
                func() {
                    mockHolmesGPT.SetResponse(&testutil.IncidentResponse{
                        RootCause:        "Memory leak detected",
                        SelectedWorkflow: testutil.DefaultWorkflow(),
                        Confidence:       0.92,
                    })
                }),
            Entry("Analyzing → Recommending (Rego auto-approve)",
                "Analyzing", "Recommending",
                func() {
                    mockRego.SetDecision("auto_approve")
                }),
            Entry("Recommending → Completed",
                "Recommending", "Completed",
                func() {}),
        )
    })

    Context("Error Handling", func() {
        It("should handle CRD not found gracefully (Category A)", func() {
            // Don't create the CRD
            result, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKey{Name: "nonexistent", Namespace: "default"},
            })

            Expect(err).ToNot(HaveOccurred())
            Expect(result.Requeue).To(BeFalse())
        })

        It("should retry on HolmesGPT-API transient error (Category B)", func() {
            mockHolmesGPT.SetError(testutil.TransientError("503 Service Unavailable"))

            analysis := createTestAnalysis("test-retry", "Investigating")
            Expect(fakeClient.Create(ctx, analysis)).To(Succeed())

            result, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(analysis),
            })

            Expect(err).To(HaveOccurred())
            Expect(result.RequeueAfter).To(BeNumerically(">", 0))
        })

        It("should fail immediately on auth error (Category C)", func() {
            mockHolmesGPT.SetError(testutil.AuthError("401 Unauthorized"))

            analysis := createTestAnalysis("test-auth", "Investigating")
            Expect(fakeClient.Create(ctx, analysis)).To(Succeed())

            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(analysis),
            })

            // Should not return error (no retry), but status should be Failed
            Expect(err).ToNot(HaveOccurred())

            var updated aianalysisv1.AIAnalysis
            Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
            Expect(updated.Status.Phase).To(Equal("Failed"))
        })

        It("should use graceful degradation on Rego failure (Category E)", func() {
            mockRego.SetError(fmt.Errorf("rego: parse error"))

            analysis := createTestAnalysis("test-rego-fail", "Analyzing")
            analysis.Status.RootCause = "Test root cause"
            analysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
                WorkflowID: "restart-pod-v1",
            }
            Expect(fakeClient.Create(ctx, analysis)).To(Succeed())

            _, err := reconciler.Reconcile(ctx, ctrl.Request{
                NamespacedName: client.ObjectKeyFromObject(analysis),
            })

            Expect(err).ToNot(HaveOccurred())

            var updated aianalysisv1.AIAnalysis
            Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
            // Graceful degradation: default to manual review
            Expect(updated.Status.ApprovalRequired).To(BeTrue())
        })
    })

    Context("FailedDetections Handling", func() {
        DescribeTable("should propagate FailedDetections correctly",
            func(failedFields []string, expectValid bool) {
                analysis := createTestAnalysis("test-failed-detections", "Validating")
                analysis.Spec.AnalysisRequest.SignalContext.EnrichmentResults.DetectedLabels = &sharedtypes.DetectedLabels{
                    FailedDetections: failedFields,
                    GitOpsManaged:    false, // Could be false due to failure
                    PDBProtected:     true,
                }
                Expect(fakeClient.Create(ctx, analysis)).To(Succeed())

                _, err := reconciler.Reconcile(ctx, ctrl.Request{
                    NamespacedName: client.ObjectKeyFromObject(analysis),
                })

                if expectValid {
                    Expect(err).ToNot(HaveOccurred())
                } else {
                    // Invalid field name should cause validation error
                    var updated aianalysisv1.AIAnalysis
                    Expect(fakeClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
                    Expect(updated.Status.Phase).To(Equal("Failed"))
                }
            },
            Entry("valid: gitOpsManaged failed", []string{"gitOpsManaged"}, true),
            Entry("valid: multiple fields failed", []string{"gitOpsManaged", "pdbProtected"}, true),
            Entry("valid: empty (no failures)", []string{}, true),
            Entry("invalid: unknown field", []string{"unknownField"}, false),
            Entry("invalid: podSecurityLevel (removed)", []string{"podSecurityLevel"}, false),
        )
    })
})

// Helper functions
func createTestAnalysis(name, phase string) *aianalysisv1.AIAnalysis {
    analysis := &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      name,
            Namespace: "default",
        },
        Spec: aianalysisv1.AIAnalysisSpec{
            RemediationRequestRef: corev1.ObjectReference{
                Name:      "rr-test",
                Namespace: "default",
            },
            RemediationID: "rem-123",
            AnalysisRequest: aianalysisv1.AnalysisRequest{
                SignalContext: aianalysisv1.SignalContextInput{
                    Fingerprint:      "sha256:test123",
                    SignalType:       "OOMKilled",
                    Environment:      "production",
                    BusinessPriority: "P1",
                    TargetResource: aianalysisv1.TargetResource{
                        Kind:      "Pod",
                        Name:      "test-pod",
                        Namespace: "default",
                    },
                    EnrichmentResults: sharedtypes.EnrichmentResults{
                        DetectedLabels: &sharedtypes.DetectedLabels{
                            GitOpsManaged: true,
                            PDBProtected:  true,
                        },
                    },
                },
            },
        },
    }

    if phase != "" {
        analysis.Status.Phase = phase
    }

    return analysis
}
```

### **Validating Phase Tests**

```go
// test/unit/aianalysis/validating_test.go
package aianalysis

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/phases"
    sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("BR-AI-020, BR-AI-021: Validating Phase Handler", func() {
    var (
        handler *phases.ValidatingHandler
    )

    BeforeEach(func() {
        handler = phases.NewValidatingHandler()
    })

    Context("Input Validation", func() {
        DescribeTable("should validate required fields",
            func(modifyAnalysis func(*aianalysisv1.AIAnalysis), expectValid bool, expectedError string) {
                analysis := createValidAnalysis()
                modifyAnalysis(analysis)

                result, err := handler.Handle(context.Background(), analysis)

                if expectValid {
                    Expect(err).ToNot(HaveOccurred())
                    Expect(result.Valid).To(BeTrue())
                } else {
                    Expect(result.Valid).To(BeFalse())
                    Expect(result.Errors).To(ContainElement(ContainSubstring(expectedError)))
                }
            },
            // Happy path
            Entry("valid: complete input",
                func(a *aianalysisv1.AIAnalysis) {},
                true, ""),
            Entry("valid: empty DetectedLabels",
                func(a *aianalysisv1.AIAnalysis) {
                    a.Spec.AnalysisRequest.SignalContext.EnrichmentResults.DetectedLabels = nil
                },
                true, ""),
            Entry("valid: empty CustomLabels",
                func(a *aianalysisv1.AIAnalysis) {
                    a.Spec.AnalysisRequest.SignalContext.EnrichmentResults.CustomLabels = nil
                },
                true, ""),

            // Required field validation
            Entry("invalid: missing Fingerprint",
                func(a *aianalysisv1.AIAnalysis) {
                    a.Spec.AnalysisRequest.SignalContext.Fingerprint = ""
                },
                false, "fingerprint is required"),
            Entry("invalid: missing SignalType",
                func(a *aianalysisv1.AIAnalysis) {
                    a.Spec.AnalysisRequest.SignalContext.SignalType = ""
                },
                false, "signalType is required"),
            Entry("invalid: missing Environment",
                func(a *aianalysisv1.AIAnalysis) {
                    a.Spec.AnalysisRequest.SignalContext.Environment = ""
                },
                false, "environment is required"),
            Entry("invalid: missing TargetResource.Kind",
                func(a *aianalysisv1.AIAnalysis) {
                    a.Spec.AnalysisRequest.SignalContext.TargetResource.Kind = ""
                },
                false, "targetResource.kind is required"),
            Entry("invalid: missing TargetResource.Name",
                func(a *aianalysisv1.AIAnalysis) {
                    a.Spec.AnalysisRequest.SignalContext.TargetResource.Name = ""
                },
                false, "targetResource.name is required"),

            // FailedDetections validation
            Entry("valid: known field in FailedDetections",
                func(a *aianalysisv1.AIAnalysis) {
                    a.Spec.AnalysisRequest.SignalContext.EnrichmentResults.DetectedLabels = &sharedtypes.DetectedLabels{
                        FailedDetections: []string{"gitOpsManaged"},
                    }
                },
                true, ""),
            Entry("invalid: unknown field in FailedDetections",
                func(a *aianalysisv1.AIAnalysis) {
                    a.Spec.AnalysisRequest.SignalContext.EnrichmentResults.DetectedLabels = &sharedtypes.DetectedLabels{
                        FailedDetections: []string{"invalidField"},
                    }
                },
                false, "invalidField"),
            Entry("invalid: podSecurityLevel in FailedDetections (removed v2.2)",
                func(a *aianalysisv1.AIAnalysis) {
                    a.Spec.AnalysisRequest.SignalContext.EnrichmentResults.DetectedLabels = &sharedtypes.DetectedLabels{
                        FailedDetections: []string{"podSecurityLevel"},
                    }
                },
                false, "podSecurityLevel"),
        )
    })
})

func createValidAnalysis() *aianalysisv1.AIAnalysis {
    return &aianalysisv1.AIAnalysis{
        Spec: aianalysisv1.AIAnalysisSpec{
            AnalysisRequest: aianalysisv1.AnalysisRequest{
                SignalContext: aianalysisv1.SignalContextInput{
                    Fingerprint:      "sha256:abc123",
                    SignalType:       "OOMKilled",
                    Environment:      "production",
                    BusinessPriority: "P1",
                    TargetResource: aianalysisv1.TargetResource{
                        Kind:      "Pod",
                        Name:      "test-pod",
                        Namespace: "default",
                    },
                    EnrichmentResults: sharedtypes.EnrichmentResults{
                        DetectedLabels: &sharedtypes.DetectedLabels{
                            GitOpsManaged: true,
                        },
                    },
                },
            },
        },
    }
}
```

### **HolmesGPT Client Unit Tests**

```go
// test/unit/aianalysis/holmesgpt_client_test.go
package aianalysis

import (
    "context"
    "net/http"
    "net/http/httptest"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/jordigilh/kubernaut/pkg/aianalysis/holmesgpt"
)

var _ = Describe("BR-AI-023: HolmesGPT Client", func() {
    var (
        ctx    context.Context
        server *httptest.Server
        client *holmesgpt.Client
    )

    AfterEach(func() {
        if server != nil {
            server.Close()
        }
    })

    Context("Request/Response Handling", func() {
        It("should send correct request to /api/v1/incident/analyze", func() {
            var receivedRequest *http.Request
            server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                receivedRequest = r
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusOK)
                w.Write([]byte(`{
                    "root_cause": "Test root cause",
                    "selected_workflow": {
                        "workflow_id": "restart-pod-v1",
                        "container_image": "ghcr.io/kubernaut/workflow-runner:v1"
                    },
                    "confidence": 0.92,
                    "target_in_owner_chain": true,
                    "warnings": []
                }`))
            }))

            client = holmesgpt.NewClient(server.URL, "test-api-key")
            ctx = context.Background()

            _, err := client.Investigate(ctx, &holmesgpt.IncidentRequest{
                Fingerprint: "test-123",
                SignalType:  "OOMKilled",
            })

            Expect(err).ToNot(HaveOccurred())
            Expect(receivedRequest.URL.Path).To(Equal("/api/v1/incident/analyze"))
            Expect(receivedRequest.Method).To(Equal("POST"))
            Expect(receivedRequest.Header.Get("Authorization")).To(Equal("Bearer test-api-key"))
        })

        It("should parse TargetInOwnerChain from response", func() {
            server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusOK)
                w.Write([]byte(`{
                    "root_cause": "Test",
                    "selected_workflow": {"workflow_id": "test-v1"},
                    "confidence": 0.85,
                    "target_in_owner_chain": false,
                    "warnings": ["Label scope mismatch"]
                }`))
            }))

            client = holmesgpt.NewClient(server.URL, "test-api-key")
            ctx = context.Background()

            resp, err := client.Investigate(ctx, &holmesgpt.IncidentRequest{})

            Expect(err).ToNot(HaveOccurred())
            Expect(resp.TargetInOwnerChain).To(BeFalse())
            Expect(resp.Warnings).To(ContainElement("Label scope mismatch"))
        })
    })

    Context("Retry Behavior", func() {
        It("should retry on 503 Service Unavailable", func() {
            callCount := 0
            server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                callCount++
                if callCount < 3 {
                    w.WriteHeader(http.StatusServiceUnavailable)
                    return
                }
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusOK)
                w.Write([]byte(`{"root_cause": "Test", "confidence": 0.9}`))
            }))

            client = holmesgpt.NewClient(server.URL, "test-api-key",
                holmesgpt.WithRetryConfig(holmesgpt.RetryConfig{
                    MaxRetries:   5,
                    InitialDelay: 10 * time.Millisecond,
                }))
            ctx = context.Background()

            _, err := client.Investigate(ctx, &holmesgpt.IncidentRequest{})

            Expect(err).ToNot(HaveOccurred())
            Expect(callCount).To(Equal(3))
        })

        It("should not retry on 401 Unauthorized", func() {
            callCount := 0
            server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                callCount++
                w.WriteHeader(http.StatusUnauthorized)
            }))

            client = holmesgpt.NewClient(server.URL, "invalid-key")
            ctx = context.Background()

            _, err := client.Investigate(ctx, &holmesgpt.IncidentRequest{})

            Expect(err).To(HaveOccurred())
            Expect(callCount).To(Equal(1)) // No retry
        })
    })
})
```

---

## 🔗 **Day 7: Integration Tests (8h)**

### **Test Files Structure**

```
test/integration/aianalysis/
├── suite_test.go                  # Ginkgo suite with KIND setup
├── reconciler_test.go             # Full reconciliation loop
├── rego_policy_test.go            # Rego ConfigMap integration
├── holmesgpt_integration_test.go  # MockLLMServer integration
└── cross_crd_test.go              # SignalProcessing → AIAnalysis flow
```

### **KIND Setup with MockLLMServer**

```go
// test/integration/aianalysis/suite_test.go
package aianalysis_integration

import (
    "context"
    "os"
    "os/exec"
    "testing"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "k8s.io/client-go/kubernetes/scheme"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/envtest"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

var (
    testEnv   *envtest.Environment
    k8sClient client.Client
    ctx       context.Context
    cancel    context.CancelFunc

    mockLLMServerCmd *exec.Cmd
    holmesGPTAPICmd  *exec.Cmd
)

func TestAIAnalysisIntegration(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "AIAnalysis Integration Suite")
}

var _ = BeforeSuite(func() {
    ctx, cancel = context.WithCancel(context.Background())

    By("starting MockLLMServer")
    mockLLMServerCmd = exec.Command("python",
        "../../../../holmesgpt-api/tests/mock_llm_server.py")
    mockLLMServerCmd.Env = append(os.Environ(), "PORT=11434")
    Expect(mockLLMServerCmd.Start()).To(Succeed())

    // Wait for MockLLMServer
    Eventually(func() error {
        _, err := http.Get("http://localhost:11434/health")
        return err
    }, 30*time.Second, 1*time.Second).Should(Succeed())

    By("starting HolmesGPT-API with MockLLM")
    holmesGPTAPICmd = exec.Command("uvicorn",
        "main:app", "--host", "0.0.0.0", "--port", "8080")
    holmesGPTAPICmd.Dir = "../../../../holmesgpt-api"
    holmesGPTAPICmd.Env = append(os.Environ(),
        "LLM_BACKEND=mock",
        "MOCK_LLM_URL=http://localhost:11434")
    Expect(holmesGPTAPICmd.Start()).To(Succeed())

    // Wait for HolmesGPT-API
    Eventually(func() error {
        _, err := http.Get("http://localhost:8080/health")
        return err
    }, 30*time.Second, 1*time.Second).Should(Succeed())

    By("bootstrapping test environment")
    testEnv = &envtest.Environment{
        CRDDirectoryPaths: []string{
            "../../../../config/crd/bases",
        },
    }

    cfg, err := testEnv.Start()
    Expect(err).ToNot(HaveOccurred())

    Expect(aianalysisv1.AddToScheme(scheme.Scheme)).To(Succeed())

    k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
    Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
    cancel()

    By("stopping MockLLMServer")
    if mockLLMServerCmd != nil && mockLLMServerCmd.Process != nil {
        mockLLMServerCmd.Process.Kill()
    }

    By("stopping HolmesGPT-API")
    if holmesGPTAPICmd != nil && holmesGPTAPICmd.Process != nil {
        holmesGPTAPICmd.Process.Kill()
    }

    By("tearing down test environment")
    Expect(testEnv.Stop()).To(Succeed())
})
```

### **Rego Policy Integration Test**

```go
// test/integration/aianalysis/rego_policy_test.go
package aianalysis_integration

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

var _ = Describe("BR-AI-030 to BR-AI-033: Rego Policy Integration", func() {
    var (
        ctx       context.Context
        namespace string
    )

    BeforeEach(func() {
        ctx = context.Background()
        namespace = fmt.Sprintf("test-rego-%d", time.Now().UnixNano())

        // Create namespace
        ns := &corev1.Namespace{
            ObjectMeta: metav1.ObjectMeta{Name: namespace},
        }
        Expect(k8sClient.Create(ctx, ns)).To(Succeed())
    })

    AfterEach(func() {
        // Cleanup namespace
        ns := &corev1.Namespace{
            ObjectMeta: metav1.ObjectMeta{Name: namespace},
        }
        Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
    })

    Context("BR-AI-030: ConfigMap → Policy Load", func() {
        It("should load policy from ConfigMap and evaluate correctly", func() {
            // Create Rego policy ConfigMap
            configMap := &corev1.ConfigMap{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "ai-approval-policies",
                    Namespace: namespace,
                },
                Data: map[string]string{
                    "approval.rego": `
                        package approval

                        default require_approval = true

                        require_approval = false {
                            input.confidence >= 0.90
                            input.environment != "production"
                        }
                    `,
                },
            }
            Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

            // Create AIAnalysis with high confidence in non-prod
            analysis := createTestAnalysisInNamespace(namespace, "test-auto-approve")
            analysis.Spec.AnalysisRequest.SignalContext.Environment = "staging"
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            // Wait for reconciliation
            Eventually(func() string {
                var updated aianalysisv1.AIAnalysis
                k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)
                return updated.Status.Phase
            }, 60*time.Second, 1*time.Second).Should(Equal("Completed"))

            // Verify auto-approval (no manual review required)
            var updated aianalysisv1.AIAnalysis
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
            Expect(updated.Status.ApprovalRequired).To(BeFalse())
        })
    })

    Context("BR-AI-032: Hot-Reload Under Load", func() {
        It("should apply updated policy without restart", func() {
            // Initial policy: always require approval
            configMap := &corev1.ConfigMap{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "ai-approval-policies",
                    Namespace: namespace,
                },
                Data: map[string]string{
                    "approval.rego": `
                        package approval
                        default require_approval = true
                    `,
                },
            }
            Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

            // Create first AIAnalysis - should require approval
            analysis1 := createTestAnalysisInNamespace(namespace, "test-before-update")
            Expect(k8sClient.Create(ctx, analysis1)).To(Succeed())

            Eventually(func() bool {
                var updated aianalysisv1.AIAnalysis
                k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis1), &updated)
                return updated.Status.ApprovalRequired
            }, 60*time.Second, 1*time.Second).Should(BeTrue())

            // Update policy: auto-approve high confidence
            configMap.Data["approval.rego"] = `
                package approval
                default require_approval = true
                require_approval = false {
                    input.confidence >= 0.90
                }
            `
            Expect(k8sClient.Update(ctx, configMap)).To(Succeed())

            // Wait for policy reload
            time.Sleep(5 * time.Second)

            // Create second AIAnalysis - should auto-approve
            analysis2 := createTestAnalysisInNamespace(namespace, "test-after-update")
            Expect(k8sClient.Create(ctx, analysis2)).To(Succeed())

            Eventually(func() bool {
                var updated aianalysisv1.AIAnalysis
                k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis2), &updated)
                return updated.Status.ApprovalRequired
            }, 60*time.Second, 1*time.Second).Should(BeFalse())
        })
    })

    Context("BR-AI-031: Invalid Policy Fallback", func() {
        It("should default to require_approval=true on invalid Rego syntax", func() {
            // Create invalid Rego policy
            configMap := &corev1.ConfigMap{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "ai-approval-policies",
                    Namespace: namespace,
                },
                Data: map[string]string{
                    "approval.rego": `
                        package approval
                        // INVALID SYNTAX - missing =
                        default require_approval true
                    `,
                },
            }
            Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

            analysis := createTestAnalysisInNamespace(namespace, "test-invalid-policy")
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            // Should complete but require approval (graceful degradation)
            Eventually(func() bool {
                var updated aianalysisv1.AIAnalysis
                k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)
                return updated.Status.ApprovalRequired
            }, 60*time.Second, 1*time.Second).Should(BeTrue())
        })
    })

    Context("BR-AI-033: Policy Version Tracking", func() {
        It("should include policy hash in audit trail", func() {
            configMap := &corev1.ConfigMap{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "ai-approval-policies",
                    Namespace: namespace,
                },
                Data: map[string]string{
                    "approval.rego": `
                        package approval
                        default require_approval = false
                    `,
                },
            }
            Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

            analysis := createTestAnalysisInNamespace(namespace, "test-policy-hash")
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            Eventually(func() string {
                var updated aianalysisv1.AIAnalysis
                k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)
                return updated.Status.ApprovalContext.PolicyHash
            }, 60*time.Second, 1*time.Second).ShouldNot(BeEmpty())
        })
    })
})
```

---

## 🚀 **Day 8: E2E Tests (8h)**

### **E2E Test Scenarios**

```go
// test/e2e/aianalysis/workflow_selection_test.go
package aianalysis_e2e

import (
    "context"
    "time"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("E2E: AIAnalysis Workflow Selection", func() {
    Context("Auto-Approval Flow", func() {
        It("should complete analysis and auto-approve in non-production", func() {
            analysis := createE2EAnalysis("e2e-auto-approve", "staging", 0.95)
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            // Wait for completion
            Eventually(func() string {
                var updated aianalysisv1.AIAnalysis
                k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)
                return updated.Status.Phase
            }, 2*time.Minute, 5*time.Second).Should(Equal("Completed"))

            // Verify auto-approved
            var completed aianalysisv1.AIAnalysis
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &completed)).To(Succeed())

            Expect(completed.Status.ApprovalRequired).To(BeFalse())
            Expect(completed.Status.SelectedWorkflow).ToNot(BeNil())
            Expect(completed.Status.SelectedWorkflow.WorkflowID).ToNot(BeEmpty())
            Expect(completed.Status.Confidence).To(BeNumerically(">=", 0.80))
        })
    })

    Context("Manual Approval Flow", func() {
        It("should require approval in production environment", func() {
            analysis := createE2EAnalysis("e2e-manual-approve", "production", 0.92)
            Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

            // Wait for analyzing phase (before approval decision)
            Eventually(func() string {
                var updated aianalysisv1.AIAnalysis
                k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)
                return updated.Status.Phase
            }, 2*time.Minute, 5*time.Second).Should(Equal("Recommending"))

            // Verify approval required
            var updated aianalysisv1.AIAnalysis
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), &updated)).To(Succeed())
            Expect(updated.Status.ApprovalRequired).To(BeTrue())
            Expect(updated.Status.ApprovalContext).ToNot(BeNil())
            Expect(updated.Status.ApprovalContext.Reason).To(ContainSubstring("production"))
        })
    })

    Context("Recovery Flow", func() {
        It("should handle recovery attempt with previous execution context", func() {
            // First attempt - will "fail"
            firstAnalysis := createE2EAnalysis("e2e-recovery-first", "staging", 0.90)
            Expect(k8sClient.Create(ctx, firstAnalysis)).To(Succeed())

            Eventually(func() string {
                var updated aianalysisv1.AIAnalysis
                k8sClient.Get(ctx, client.ObjectKeyFromObject(firstAnalysis), &updated)
                return updated.Status.Phase
            }, 2*time.Minute, 5*time.Second).Should(Equal("Completed"))

            // Recovery attempt with previous context
            recoveryAnalysis := createE2EAnalysis("e2e-recovery-second", "staging", 0.90)
            recoveryAnalysis.Spec.AnalysisRequest.IsRecoveryAttempt = true
            recoveryAnalysis.Spec.AnalysisRequest.RecoveryAttemptNumber = 2
            recoveryAnalysis.Spec.AnalysisRequest.PreviousExecutions = []aianalysisv1.PreviousExecution{
                {
                    WorkflowID:    "restart-pod-v1",
                    Status:        "Failed",
                    FailureReason: "Pod evicted during restart",
                    ExecutedAt:    metav1.Now(),
                },
            }
            Expect(k8sClient.Create(ctx, recoveryAnalysis)).To(Succeed())

            // Should complete with different workflow selection
            Eventually(func() string {
                var updated aianalysisv1.AIAnalysis
                k8sClient.Get(ctx, client.ObjectKeyFromObject(recoveryAnalysis), &updated)
                return updated.Status.Phase
            }, 2*time.Minute, 5*time.Second).Should(Equal("Completed"))

            var completed aianalysisv1.AIAnalysis
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(recoveryAnalysis), &completed)).To(Succeed())

            // Recovery should suggest alternative workflow
            Expect(completed.Status.SelectedWorkflow.WorkflowID).ToNot(Equal("restart-pod-v1"))
        })
    })
})
```

---

## 📊 **Test Coverage Targets**

| Component | Unit | Integration | E2E | Total |
|-----------|------|-------------|-----|-------|
| Controller | 15 tests | 5 tests | 2 tests | 22 |
| ValidatingHandler | 9 tests | - | - | 9 |
| InvestigatingHandler | 9 tests | 3 tests | 1 test | 13 |
| RegoEngine | 8 tests | 4 tests | - | 12 |
| HolmesGPTClient | 8 tests | 2 tests | - | 10 |
| **Total** | **49** | **14** | **3** | **66** |

---

## 📋 **Test Execution Checklist**

### **Day 6 EOD**
- [ ] Unit test suite compiles
- [ ] All 49 unit tests passing
- [ ] 70%+ code coverage
- [ ] Parallel execution working (4 procs)

### **Day 7 EOD**
- [ ] KIND cluster running
- [ ] MockLLMServer running
- [ ] All 14 integration tests passing
- [ ] Rego policy tests cover all 4 scenarios

### **Day 8 EOD**
- [ ] All 3 E2E scenarios passing
- [ ] Health/metrics endpoints validated
- [ ] Recovery flow verified end-to-end

---

## 📚 **References**

- [testing-strategy.md](../testing-strategy.md) - High-level testing strategy
- [TESTING_GUIDELINES.md](../../../../development/business-requirements/TESTING_GUIDELINES.md) - Authoritative testing standards
- [03-testing-strategy.mdc](../../../../.cursor/rules/03-testing-strategy.mdc) - Testing rules
- [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md) - Parent implementation plan

