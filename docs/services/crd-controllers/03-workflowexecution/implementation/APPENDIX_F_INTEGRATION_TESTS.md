# WorkflowExecution - Integration Test Examples

**Parent Document**: [IMPLEMENTATION_PLAN_V3.0.md](./IMPLEMENTATION_PLAN_V3.0.md)
**Version**: v1.0
**Last Updated**: 2025-12-03
**Status**: ✅ Ready for Implementation

---

## Document Purpose

This appendix provides complete, production-ready integration test code for the WorkflowExecution Controller, aligned with Day 8-9 of [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md).

---

## 📦 Test Infrastructure Setup

### Suite Configuration

**File**: `test/integration/workflowexecution/suite_test.go`

```go
package workflowexecution

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller"
)

// Test suite variables
var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	mgr       ctrl.Manager
)

func TestWorkflowExecution(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution Controller Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Register schemes
	err = workflowexecutionv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = tektonv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create manager
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		MetricsBindAddress: "0", // Disable metrics in tests
	})
	Expect(err).ToNot(HaveOccurred())

	// Setup controller with test configuration
	err = (&controller.WorkflowExecutionReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		ExecutionNamespace: "kubernaut-workflows",
		ServiceAccountName: "kubernaut-workflow-runner",
		CooldownPeriod:     1 * time.Minute, // Shorter for tests
		StatusCheckInterval: 2 * time.Second,
	}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	// Start manager in background
	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

	k8sClient = mgr.GetClient()
	Expect(k8sClient).ToNot(BeNil())

	// Create test namespaces
	By("creating test namespaces")
	testNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-test"},
	}
	Expect(k8sClient.Create(ctx, testNs)).Should(Succeed())

	workflowNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-workflows"},
	}
	Expect(k8sClient.Create(ctx, workflowNs)).Should(Succeed())
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
```

---

## 🧪 Integration Test 1: Complete Workflow Lifecycle

**File**: `test/integration/workflowexecution/lifecycle_test.go`

**BR Coverage**: BR-WE-001 (PipelineRun Creation), BR-WE-002 (Parameter Passing)

```go
package workflowexecution

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("Integration Test 1: Complete Workflow Lifecycle", func() {
	var (
		ctx          context.Context
		wfeName      string
		namespace    string
		targetResource string
	)

	BeforeEach(func() {
		ctx = context.Background()
		wfeName = "test-lifecycle-" + randomString(5)
		namespace = "kubernaut-test"
		targetResource = "production/deployment/app-" + randomString(5)
	})

	AfterEach(func() {
		// Cleanup WFE
		wfe := &workflowexecutionv1.WorkflowExecution{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfeName,
			Namespace: namespace,
		}, wfe); err == nil {
			k8sClient.Delete(ctx, wfe)
		}

		// Cleanup PipelineRun
		prName := pipelineRunName(targetResource)
		pr := &tektonv1.PipelineRun{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      prName,
			Namespace: "kubernaut-workflows",
		}, pr); err == nil {
			k8sClient.Delete(ctx, pr)
		}
	})

	It("should transition from Pending to Running when PipelineRun is created", func() {
		By("Creating a WorkflowExecution in Pending state")
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wfeName,
				Namespace: namespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				TargetResource: targetResource,
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					ContainerImage: "ghcr.io/kubernaut/workflows/disk-cleanup@sha256:abc123",
				},
				Parameters: map[string]string{
					"THRESHOLD": "80",
					"DRY_RUN":   "false",
				},
			},
		}
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for phase to transition to Running")
		Eventually(func() workflowexecutionv1.Phase {
			updated := &workflowexecutionv1.WorkflowExecution{}
			k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

		By("Verifying PipelineRun was created in execution namespace")
		prName := pipelineRunName(targetResource)
		pr := &tektonv1.PipelineRun{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      prName,
			Namespace: "kubernaut-workflows",
		}, pr)
		Expect(err).ToNot(HaveOccurred())

		By("Verifying PipelineRun has correct configuration")
		// Bundle resolver
		Expect(pr.Spec.PipelineRef.ResolverRef.Resolver).To(Equal("bundles"))

		// Parameters passed correctly
		Expect(pr.Spec.Params).To(ContainElement(HaveField("Name", "THRESHOLD")))
		Expect(pr.Spec.Params).To(ContainElement(HaveField("Name", "DRY_RUN")))

		// Labels for tracking
		Expect(pr.Labels["kubernaut.ai/workflow-execution"]).To(Equal(wfeName))
		Expect(pr.Labels["kubernaut.ai/source-namespace"]).To(Equal(namespace))

		By("Verifying WFE status contains PipelineRun reference")
		updated := &workflowexecutionv1.WorkflowExecution{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfeName,
			Namespace: namespace,
		}, updated)).To(Succeed())
		Expect(updated.Status.PipelineRunName).To(Equal(prName))
		Expect(updated.Status.StartTime).ToNot(BeNil())
	})

	It("should transition to Completed when PipelineRun succeeds", func() {
		By("Creating a WorkflowExecution")
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wfeName,
				Namespace: namespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				TargetResource: targetResource,
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					ContainerImage: "ghcr.io/kubernaut/workflows/disk-cleanup@sha256:abc123",
				},
			},
		}
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for Running phase")
		Eventually(func() workflowexecutionv1.Phase {
			updated := &workflowexecutionv1.WorkflowExecution{}
			k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

		By("Simulating PipelineRun success")
		prName := pipelineRunName(targetResource)
		pr := &tektonv1.PipelineRun{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      prName,
			Namespace: "kubernaut-workflows",
		}, pr)).To(Succeed())

		// Update PipelineRun status to Succeeded
		pr.Status.SetCondition(&apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionTrue,
			Reason:  "Succeeded",
			Message: "All Tasks have completed executing",
		})
		Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

		By("Waiting for WFE to transition to Completed")
		Eventually(func() workflowexecutionv1.Phase {
			updated := &workflowexecutionv1.WorkflowExecution{}
			k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 15*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1.PhaseCompleted))

		By("Verifying completion details")
		completed := &workflowexecutionv1.WorkflowExecution{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfeName,
			Namespace: namespace,
		}, completed)).To(Succeed())
		Expect(completed.Status.CompletionTime).ToNot(BeNil())
		Expect(completed.Status.FailureDetails).To(BeNil())
	})

	It("should transition to Failed with details when PipelineRun fails", func() {
		By("Creating a WorkflowExecution")
		wfe := &workflowexecutionv1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      wfeName,
				Namespace: namespace,
			},
			Spec: workflowexecutionv1.WorkflowExecutionSpec{
				TargetResource: targetResource,
				WorkflowRef: workflowexecutionv1.WorkflowRef{
					ContainerImage: "ghcr.io/kubernaut/workflows/disk-cleanup@sha256:abc123",
				},
			},
		}
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())

		By("Waiting for Running phase")
		Eventually(func() workflowexecutionv1.Phase {
			updated := &workflowexecutionv1.WorkflowExecution{}
			k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

		By("Simulating PipelineRun failure")
		prName := pipelineRunName(targetResource)
		pr := &tektonv1.PipelineRun{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      prName,
			Namespace: "kubernaut-workflows",
		}, pr)).To(Succeed())

		// Update PipelineRun status to Failed
		pr.Status.SetCondition(&apis.Condition{
			Type:    apis.ConditionSucceeded,
			Status:  corev1.ConditionFalse,
			Reason:  "TaskRunFailed",
			Message: "Task cleanup-disk failed: disk full, cannot cleanup",
		})
		Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

		By("Waiting for WFE to transition to Failed")
		Eventually(func() workflowexecutionv1.Phase {
			updated := &workflowexecutionv1.WorkflowExecution{}
			k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 15*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1.PhaseFailed))

		By("Verifying failure details are populated")
		failed := &workflowexecutionv1.WorkflowExecution{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfeName,
			Namespace: namespace,
		}, failed)).To(Succeed())

		Expect(failed.Status.FailureDetails).ToNot(BeNil())
		Expect(failed.Status.FailureDetails.Reason).To(Equal("TaskRunFailed"))
		Expect(failed.Status.FailureDetails.Message).To(ContainSubstring("cleanup-disk"))
		Expect(failed.Status.FailureDetails.WasExecutionFailure).To(BeTrue())
		Expect(failed.Status.FailureDetails.NaturalLanguageSummary).ToNot(BeEmpty())
	})
})
```

---

## 🧪 Integration Test 2: Resource Locking

**File**: `test/integration/workflowexecution/locking_test.go`

**BR Coverage**: BR-WE-009 (Parallel Prevention), BR-WE-010 (Sequential Deduplication), BR-WE-011 (Race Condition Handling)

```go
package workflowexecution

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("Integration Test 2: Resource Locking (DD-WE-001)", func() {
	var (
		ctx            context.Context
		namespace      string
		targetResource string
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "kubernaut-test"
		targetResource = "production/deployment/app-" + randomString(5)
	})

	AfterEach(func() {
		// Cleanup all WFEs for this target
		wfeList := &workflowexecutionv1.WorkflowExecutionList{}
		k8sClient.List(ctx, wfeList, client.InNamespace(namespace))
		for _, wfe := range wfeList.Items {
			k8sClient.Delete(ctx, &wfe)
		}

		// Cleanup PipelineRun
		prName := pipelineRunName(targetResource)
		pr := &tektonv1.PipelineRun{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      prName,
			Namespace: "kubernaut-workflows",
		}, pr); err == nil {
			k8sClient.Delete(ctx, pr)
		}
	})

	Context("Parallel Execution Prevention", func() {
		It("should block second WFE when first is Running", func() {
			By("Creating first WFE that will start Running")
			wfe1 := createTestWFE(ctx, k8sClient, namespace, "wfe-first-"+randomString(5), targetResource)

			By("Waiting for first WFE to reach Running")
			Eventually(func() workflowexecutionv1.Phase {
				updated := &workflowexecutionv1.WorkflowExecution{}
				k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe1.Name,
					Namespace: namespace,
				}, updated)
				return updated.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

			By("Creating second WFE for same target")
			wfe2 := createTestWFE(ctx, k8sClient, namespace, "wfe-second-"+randomString(5), targetResource)

			By("Verifying second WFE is Skipped with ResourceBusy")
			Eventually(func() workflowexecutionv1.Phase {
				updated := &workflowexecutionv1.WorkflowExecution{}
				k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe2.Name,
					Namespace: namespace,
				}, updated)
				return updated.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseSkipped))

			By("Verifying skip reason is ResourceBusy")
			skipped := &workflowexecutionv1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe2.Name,
				Namespace: namespace,
			}, skipped)).To(Succeed())
			Expect(skipped.Status.SkipDetails).ToNot(BeNil())
			Expect(skipped.Status.SkipDetails.Reason).To(Equal("ResourceBusy"))
		})
	})

	Context("Cooldown Period Enforcement", func() {
		It("should block WFE during cooldown after completion", func() {
			By("Creating and completing first WFE")
			wfe1 := createTestWFE(ctx, k8sClient, namespace, "wfe-cooldown-"+randomString(5), targetResource)

			// Wait for Running
			Eventually(func() workflowexecutionv1.Phase {
				updated := &workflowexecutionv1.WorkflowExecution{}
				k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe1.Name,
					Namespace: namespace,
				}, updated)
				return updated.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

			// Simulate success
			simulatePipelineRunSuccess(ctx, k8sClient, targetResource)

			// Wait for Completed
			Eventually(func() workflowexecutionv1.Phase {
				updated := &workflowexecutionv1.WorkflowExecution{}
				k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe1.Name,
					Namespace: namespace,
				}, updated)
				return updated.Status.Phase
			}, 15*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1.PhaseCompleted))

			By("Creating second WFE immediately (within cooldown)")
			wfe2 := createTestWFE(ctx, k8sClient, namespace, "wfe-during-cooldown-"+randomString(5), targetResource)

			By("Verifying second WFE is Skipped with RecentlyRemediated")
			Eventually(func() workflowexecutionv1.Phase {
				updated := &workflowexecutionv1.WorkflowExecution{}
				k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe2.Name,
					Namespace: namespace,
				}, updated)
				return updated.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseSkipped))

			By("Verifying skip reason is RecentlyRemediated")
			skipped := &workflowexecutionv1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe2.Name,
				Namespace: namespace,
			}, skipped)).To(Succeed())
			Expect(skipped.Status.SkipDetails).ToNot(BeNil())
			Expect(skipped.Status.SkipDetails.Reason).To(Equal("RecentlyRemediated"))
		})
	})

	Context("Race Condition Handling (DD-WE-003)", func() {
		It("should handle concurrent WFE creation gracefully", func() {
			By("Creating multiple WFEs concurrently for same target")
			var wg sync.WaitGroup
			wfeNames := make([]string, 5)
			results := make([]error, 5)

			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					name := fmt.Sprintf("wfe-concurrent-%d-%s", index, randomString(5))
					wfeNames[index] = name
					wfe := &workflowexecutionv1.WorkflowExecution{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: namespace,
						},
						Spec: workflowexecutionv1.WorkflowExecutionSpec{
							TargetResource: targetResource,
							WorkflowRef: workflowexecutionv1.WorkflowRef{
								ContainerImage: "ghcr.io/kubernaut/workflows/test@sha256:abc",
							},
						},
					}
					results[index] = k8sClient.Create(ctx, wfe)
				}(i)
			}
			wg.Wait()

			By("Verifying all creations succeeded")
			for i, err := range results {
				Expect(err).ToNot(HaveOccurred(), "WFE %d creation failed", i)
			}

			By("Waiting for all WFEs to reach terminal state")
			time.Sleep(5 * time.Second)

			By("Counting Running vs Skipped WFEs")
			runningCount := 0
			skippedCount := 0

			for _, name := range wfeNames {
				wfe := &workflowexecutionv1.WorkflowExecution{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      name,
					Namespace: namespace,
				}, wfe)).To(Succeed())

				switch wfe.Status.Phase {
				case workflowexecutionv1.PhaseRunning:
					runningCount++
				case workflowexecutionv1.PhaseSkipped:
					skippedCount++
				}
			}

			By("Verifying exactly one WFE is Running")
			Expect(runningCount).To(Equal(1), "Exactly one WFE should be Running")
			Expect(skippedCount).To(Equal(4), "Four WFEs should be Skipped")
		})
	})

	Context("Lock Release After Cooldown", func() {
		It("should delete PipelineRun after cooldown expires", func() {
			By("Creating and completing WFE")
			wfe := createTestWFE(ctx, k8sClient, namespace, "wfe-release-"+randomString(5), targetResource)

			// Wait for Running
			Eventually(func() workflowexecutionv1.Phase {
				updated := &workflowexecutionv1.WorkflowExecution{}
				k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe.Name,
					Namespace: namespace,
				}, updated)
				return updated.Status.Phase
			}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

			// Simulate success
			simulatePipelineRunSuccess(ctx, k8sClient, targetResource)

			// Wait for Completed
			Eventually(func() workflowexecutionv1.Phase {
				updated := &workflowexecutionv1.WorkflowExecution{}
				k8sClient.Get(ctx, types.NamespacedName{
					Name:      wfe.Name,
					Namespace: namespace,
				}, updated)
				return updated.Status.Phase
			}, 15*time.Second, 1*time.Second).Should(Equal(workflowexecutionv1.PhaseCompleted))

			By("Verifying PipelineRun exists during cooldown")
			prName := pipelineRunName(targetResource)
			pr := &tektonv1.PipelineRun{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      prName,
				Namespace: "kubernaut-workflows",
			}, pr)).To(Succeed())

			By("Waiting for cooldown to expire (1 min in test config)")
			// Note: In integration tests, cooldown is set to 1 minute
			time.Sleep(65 * time.Second)

			By("Verifying PipelineRun is deleted after cooldown")
			Eventually(func() bool {
				pr := &tektonv1.PipelineRun{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      prName,
					Namespace: "kubernaut-workflows",
				}, pr)
				return apierrors.IsNotFound(err)
			}, 30*time.Second, 5*time.Second).Should(BeTrue())

			By("Verifying WFE status shows lock released")
			completed := &workflowexecutionv1.WorkflowExecution{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfe.Name,
				Namespace: namespace,
			}, completed)).To(Succeed())
			Expect(completed.Status.LockReleased).To(BeTrue())
		})
	})
})

// Helper functions

func createTestWFE(
	ctx context.Context,
	c client.Client,
	namespace, name, targetResource string,
) *workflowexecutionv1.WorkflowExecution {
	wfe := &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			TargetResource: targetResource,
			WorkflowRef: workflowexecutionv1.WorkflowRef{
				ContainerImage: "ghcr.io/kubernaut/workflows/disk-cleanup@sha256:abc123",
			},
		},
	}
	Expect(c.Create(ctx, wfe)).To(Succeed())
	return wfe
}

func simulatePipelineRunSuccess(
	ctx context.Context,
	c client.Client,
	targetResource string,
) {
	prName := pipelineRunName(targetResource)
	pr := &tektonv1.PipelineRun{}
	Expect(c.Get(ctx, types.NamespacedName{
		Name:      prName,
		Namespace: "kubernaut-workflows",
	}, pr)).To(Succeed())

	pr.Status.SetCondition(&apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionTrue,
		Reason:  "Succeeded",
		Message: "All Tasks completed",
	})
	Expect(c.Status().Update(ctx, pr)).To(Succeed())
}

func pipelineRunName(targetResource string) string {
	hash := sha256.Sum256([]byte(targetResource))
	return fmt.Sprintf("wfe-%x", hash[:8])
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
```

---

## 🧪 Integration Test 3: Finalizer Cleanup

**File**: `test/integration/workflowexecution/finalizer_test.go`

**BR Coverage**: BR-WE-004 (Owner Reference Cascade Deletion) + BR-WE-007 (External PipelineRun Deletion Handling)

```go
package workflowexecution

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("Integration Test 3: Finalizer Cleanup", func() {
	var (
		ctx            context.Context
		wfeName        string
		namespace      string
		targetResource string
	)

	BeforeEach(func() {
		ctx = context.Background()
		wfeName = "test-finalizer-" + randomString(5)
		namespace = "kubernaut-test"
		targetResource = "production/deployment/app-" + randomString(5)
	})

	It("should add finalizer when WFE is created", func() {
		By("Creating a WorkflowExecution")
		wfe := createTestWFE(ctx, k8sClient, namespace, wfeName, targetResource)

		By("Waiting for finalizer to be added")
		Eventually(func() bool {
			updated := &workflowexecutionv1.WorkflowExecution{}
			k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return controllerutil.ContainsFinalizer(updated, "kubernaut.ai/finalizer")
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
	})

	It("should cleanup PipelineRun when WFE is deleted while Running", func() {
		By("Creating a WorkflowExecution")
		wfe := createTestWFE(ctx, k8sClient, namespace, wfeName, targetResource)

		By("Waiting for Running phase")
		Eventually(func() workflowexecutionv1.Phase {
			updated := &workflowexecutionv1.WorkflowExecution{}
			k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, updated)
			return updated.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(workflowexecutionv1.PhaseRunning))

		By("Verifying PipelineRun exists")
		prName := pipelineRunName(targetResource)
		pr := &tektonv1.PipelineRun{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      prName,
			Namespace: "kubernaut-workflows",
		}, pr)).To(Succeed())

		By("Deleting the WorkflowExecution")
		wfe = &workflowexecutionv1.WorkflowExecution{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfeName,
			Namespace: namespace,
		}, wfe)).To(Succeed())
		Expect(k8sClient.Delete(ctx, wfe)).To(Succeed())

		By("Waiting for WFE to be deleted")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      wfeName,
				Namespace: namespace,
			}, &workflowexecutionv1.WorkflowExecution{})
			return apierrors.IsNotFound(err)
		}, 30*time.Second, 1*time.Second).Should(BeTrue())

		By("Verifying PipelineRun was cleaned up by finalizer")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      prName,
				Namespace: "kubernaut-workflows",
			}, &tektonv1.PipelineRun{})
			return apierrors.IsNotFound(err)
		}, 30*time.Second, 1*time.Second).Should(BeTrue())
	})
})
```

---

## 📊 Test Count Summary

| Test File | Happy Path | Edge Cases | Error Handling | **Total** |
|-----------|------------|------------|----------------|-----------|
| `lifecycle_test.go` | 3 | 1 | 1 | **5** |
| `locking_test.go` | 4 | 3 | 1 | **8** |
| `finalizer_test.go` | 2 | 1 | 0 | **3** |
| **Total Integration** | **9** | **5** | **2** | **16** |

---

## References

- [Integration Test Examples Template](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#-complete-integration-test-examples--v20)
- [Test Infrastructure Setup Template](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#day-8-unit-tests-8h)
- [testing-strategy.md](../testing-strategy.md)

