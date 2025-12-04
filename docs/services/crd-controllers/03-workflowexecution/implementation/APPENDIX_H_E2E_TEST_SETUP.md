# WorkflowExecution - E2E Test Setup

**Parent Document**: [IMPLEMENTATION_PLAN_V3.0.md](./IMPLEMENTATION_PLAN_V3.0.md)
**Version**: v1.0
**Last Updated**: 2025-12-03
**Status**: ✅ Ready for Implementation

---

## Document Purpose

This appendix provides E2E test infrastructure setup for the WorkflowExecution Controller, aligned with Day 10 of [SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md).

---

## 🛠️ Kind Cluster Configuration

**Why NodePort?**

| Aspect | Port-Forward | NodePort |
|--------|--------------|----------|
| **Stability** | Crashes under concurrent load | 100% stable |
| **Performance** | Slow (proxy overhead) | Fast (direct connection) |
| **Parallelism** | Limited to ~4 processes | Unlimited (all CPUs) |

### Kind Config File

**File**: `test/infrastructure/kind-workflowexecution-config.yaml`

```yaml
# Kind cluster configuration for WorkflowExecution E2E tests
# Reference: DD-TEST-001 Port Allocation Strategy
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  # Metrics endpoint (always needed for controllers)
  - containerPort: 30188
    hostPort: 9188
    protocol: TCP
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      extraArgs:
        max-requests-inflight: "800"
        max-mutating-requests-inflight: "400"
- role: worker
  labels:
    node-role.kubernetes.io/worker: ""
```

### Port Allocation Reference

**Reference**: [DD-TEST-001](../../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md)

| Service | Internal Health | Internal Metrics | NodePort | Metrics NodePort |
|---------|-----------------|------------------|----------|------------------|
| WorkflowExecution | 8081 | 9090 | 30088 | 30188 |

---

## 🚀 E2E Test Suite Setup

**File**: `test/e2e/workflowexecution/suite_test.go`

```go
package workflowexecution

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

const (
	clusterName     = "wfe-e2e"
	namespace       = "kubernaut-system"
	metricsPort     = 9188
	testTimeout     = 10 * time.Minute
)

var (
	ctx            context.Context
	cancel         context.CancelFunc
	k8sClient      client.Client
	clientset      *kubernetes.Clientset
	kubeconfigPath string
	metricsURL     string
)

func TestWorkflowExecutionE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution E2E Suite")
}

// SynchronizedBeforeSuite ensures cluster is created ONCE across all parallel processes
var _ = SynchronizedBeforeSuite(
	// Process 1: Create cluster and deploy service
	func() []byte {
		ctx, cancel = context.WithTimeout(context.Background(), testTimeout)

		By("Creating Kind cluster")
		kubeconfigPath = filepath.Join(os.TempDir(), "wfe-e2e-kubeconfig")
		kindConfigPath := filepath.Join("..", "..", "infrastructure", "kind-workflowexecution-config.yaml")

		err := infrastructure.CreateCluster(clusterName, kubeconfigPath, kindConfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		By("Installing Tekton Pipelines")
		err = infrastructure.InstallTekton(ctx, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		By("Deploying WorkflowExecution controller")
		err = infrastructure.DeployWorkflowExecution(ctx, namespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		By("Creating test namespaces")
		err = infrastructure.CreateNamespace(ctx, kubeconfigPath, "kubernaut-test")
		Expect(err).ToNot(HaveOccurred())
		err = infrastructure.CreateNamespace(ctx, kubeconfigPath, "kubernaut-workflows")
		Expect(err).ToNot(HaveOccurred())

		By("Setting up RBAC for workflow execution")
		err = infrastructure.ApplyRBAC(ctx, kubeconfigPath, "kubernaut-workflows")
		Expect(err).ToNot(HaveOccurred())

		return []byte(kubeconfigPath)
	},
	// All processes: Connect to cluster using NodePort
	func(data []byte) {
		kubeconfigPath = string(data)
		ctx, cancel = context.WithCancel(context.Background())

		// NodePort URL - same for all parallel processes
		metricsURL = fmt.Sprintf("http://localhost:%d/metrics", metricsPort)

		By("Connecting to cluster")
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		Expect(err).ToNot(HaveOccurred())

		k8sClient, err = client.New(config, client.Options{})
		Expect(err).ToNot(HaveOccurred())

		clientset, err = kubernetes.NewForConfig(config)
		Expect(err).ToNot(HaveOccurred())

		By("Waiting for WorkflowExecution controller to be ready")
		Eventually(func() error {
			resp, err := http.Get(metricsURL)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("metrics endpoint returned %d", resp.StatusCode)
			}
			return nil
		}, 120*time.Second, 5*time.Second).Should(Succeed())

		By("Controller is ready - proceeding with tests")
	},
)

var _ = SynchronizedAfterSuite(
	// All processes
	func() {
		cancel()
	},
	// Process 1: Cleanup cluster
	func() {
		By("Cleaning up Kind cluster")
		err := infrastructure.DeleteCluster(clusterName, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		os.Remove(kubeconfigPath)
	},
)
```

---

## 🧪 E2E Test Helpers

**File**: `test/e2e/workflowexecution/helpers_test.go`

```go
package workflowexecution

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// CreateTestWFE creates a WorkflowExecution for E2E testing
func CreateTestWFE(
	ctx context.Context,
	c client.Client,
	name, namespace, targetResource string,
	params map[string]string,
) *workflowexecutionv1.WorkflowExecution {
	wfe := &workflowexecutionv1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: workflowexecutionv1.WorkflowExecutionSpec{
			TargetResource: targetResource,
			WorkflowRef: workflowexecutionv1.WorkflowRef{
				// Use a test workflow bundle
				ContainerImage: "ghcr.io/kubernaut/test-workflows/echo@sha256:test",
			},
			Parameters: params,
		},
	}
	Expect(c.Create(ctx, wfe)).To(Succeed())
	return wfe
}

// WaitForWFEPhase waits until WFE reaches expected phase
func WaitForWFEPhase(
	ctx context.Context,
	c client.Client,
	namespace, name string,
	expectedPhase workflowexecutionv1.Phase,
	timeout time.Duration,
) *workflowexecutionv1.WorkflowExecution {
	var wfe *workflowexecutionv1.WorkflowExecution
	Eventually(func() workflowexecutionv1.Phase {
		wfe = &workflowexecutionv1.WorkflowExecution{}
		c.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, wfe)
		return wfe.Status.Phase
	}, timeout, 2*time.Second).Should(Equal(expectedPhase))
	return wfe
}

// GetPipelineRun retrieves the PipelineRun for a target resource
func GetPipelineRun(
	ctx context.Context,
	c client.Client,
	targetResource string,
) *tektonv1.PipelineRun {
	prName := PipelineRunName(targetResource)
	pr := &tektonv1.PipelineRun{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      prName,
		Namespace: "kubernaut-workflows",
	}, pr)
	if err != nil {
		return nil
	}
	return pr
}

// SimulatePipelineRunSuccess updates PipelineRun status to succeeded
func SimulatePipelineRunSuccess(
	ctx context.Context,
	c client.Client,
	targetResource string,
) {
	prName := PipelineRunName(targetResource)
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
	pr.Status.CompletionTime = &metav1.Time{Time: time.Now()}
	Expect(c.Status().Update(ctx, pr)).To(Succeed())
}

// SimulatePipelineRunFailure updates PipelineRun status to failed
func SimulatePipelineRunFailure(
	ctx context.Context,
	c client.Client,
	targetResource, reason, message string,
) {
	prName := PipelineRunName(targetResource)
	pr := &tektonv1.PipelineRun{}
	Expect(c.Get(ctx, types.NamespacedName{
		Name:      prName,
		Namespace: "kubernaut-workflows",
	}, pr)).To(Succeed())

	pr.Status.SetCondition(&apis.Condition{
		Type:    apis.ConditionSucceeded,
		Status:  corev1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
	pr.Status.CompletionTime = &metav1.Time{Time: time.Now()}
	Expect(c.Status().Update(ctx, pr)).To(Succeed())
}

// CleanupWFE deletes a WFE and waits for cleanup
func CleanupWFE(
	ctx context.Context,
	c client.Client,
	namespace, name string,
) {
	wfe := &workflowexecutionv1.WorkflowExecution{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, wfe); err == nil {
		c.Delete(ctx, wfe)

		// Wait for deletion
		Eventually(func() bool {
			err := c.Get(ctx, types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			}, &workflowexecutionv1.WorkflowExecution{})
			return apierrors.IsNotFound(err)
		}, 30*time.Second, 1*time.Second).Should(BeTrue())
	}
}

// PipelineRunName generates deterministic PR name from target
func PipelineRunName(targetResource string) string {
	hash := sha256.Sum256([]byte(targetResource))
	return fmt.Sprintf("wfe-%x", hash[:8])
}

// RandomString generates a random string for test isolation
func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// UniqueTargetResource generates a unique target for test isolation
func UniqueTargetResource() string {
	return fmt.Sprintf("e2e-test/deployment/app-%s", RandomString(8))
}
```

---

## 🧪 E2E Test: Complete Workflow

**File**: `test/e2e/workflowexecution/workflow_test.go`

```go
package workflowexecution

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("E2E Test: Complete Workflow Execution", func() {
	var (
		wfeName        string
		namespace      string
		targetResource string
	)

	BeforeEach(func() {
		wfeName = "e2e-workflow-" + RandomString(5)
		namespace = "kubernaut-test"
		targetResource = UniqueTargetResource()
	})

	AfterEach(func() {
		CleanupWFE(ctx, k8sClient, namespace, wfeName)
	})

	Describe("BR-WE-001 to BR-WE-005: Core Workflow Lifecycle", func() {
		It("should execute workflow from start to completion within SLA", func() {
			startTime := time.Now()

			By("Creating WorkflowExecution")
			wfe := CreateTestWFE(ctx, k8sClient, wfeName, namespace, targetResource, map[string]string{
				"THRESHOLD": "80",
				"DRY_RUN":   "true",
			})

			By("Verifying transition to Running")
			wfe = WaitForWFEPhase(ctx, k8sClient, namespace, wfeName,
				workflowexecutionv1.PhaseRunning, 30*time.Second)

			By("Verifying PipelineRun created in dedicated namespace")
			pr := GetPipelineRun(ctx, k8sClient, targetResource)
			Expect(pr).ToNot(BeNil())
			Expect(pr.Namespace).To(Equal("kubernaut-workflows"))

			By("Verifying parameters passed correctly")
			Expect(pr.Spec.Params).To(ContainElement(HaveField("Name", "THRESHOLD")))
			Expect(pr.Spec.Params).To(ContainElement(HaveField("Name", "DRY_RUN")))

			By("Simulating PipelineRun success")
			SimulatePipelineRunSuccess(ctx, k8sClient, targetResource)

			By("Verifying transition to Completed")
			wfe = WaitForWFEPhase(ctx, k8sClient, namespace, wfeName,
				workflowexecutionv1.PhaseCompleted, 30*time.Second)

			By("Verifying completion within SLA (30 seconds)")
			totalDuration := time.Since(startTime)
			Expect(totalDuration).To(BeNumerically("<", 30*time.Second),
				"E2E workflow should complete within 30s SLA")

			By("Verifying status details")
			Expect(wfe.Status.CompletionTime).ToNot(BeNil())
			Expect(wfe.Status.FailureDetails).To(BeNil())
		})

		It("should report failure details when workflow fails", func() {
			By("Creating WorkflowExecution")
			CreateTestWFE(ctx, k8sClient, wfeName, namespace, targetResource, nil)

			By("Waiting for Running phase")
			WaitForWFEPhase(ctx, k8sClient, namespace, wfeName,
				workflowexecutionv1.PhaseRunning, 30*time.Second)

			By("Simulating PipelineRun failure")
			SimulatePipelineRunFailure(ctx, k8sClient, targetResource,
				"TaskRunFailed", "Task cleanup-disk failed: permission denied")

			By("Waiting for Failed phase")
			wfe := WaitForWFEPhase(ctx, k8sClient, namespace, wfeName,
				workflowexecutionv1.PhaseFailed, 30*time.Second)

			By("Verifying failure details for recovery")
			Expect(wfe.Status.FailureDetails).ToNot(BeNil())
			Expect(wfe.Status.FailureDetails.Reason).To(Equal("TaskRunFailed"))
			Expect(wfe.Status.FailureDetails.Message).To(ContainSubstring("permission denied"))
			Expect(wfe.Status.FailureDetails.WasExecutionFailure).To(BeTrue())
			Expect(wfe.Status.FailureDetails.NaturalLanguageSummary).ToNot(BeEmpty())
		})
	})
})
```

---

## 🧪 E2E Test: Resource Locking Business Outcomes

**File**: `test/e2e/workflowexecution/locking_e2e_test.go`

```go
package workflowexecution

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

var _ = Describe("E2E Test: Resource Locking Business Outcomes", func() {
	var (
		namespace      string
		targetResource string
	)

	BeforeEach(func() {
		namespace = "kubernaut-test"
		targetResource = UniqueTargetResource()
	})

	Describe("BR-WE-009: Cost Savings from Duplicate Prevention", func() {
		It("should skip 90% of duplicate requests, saving remediation costs", func() {
			// Business context: Alert storm sends 10 duplicate remediations
			// Expected: 1 executes, 9 skipped = 90% efficiency
			// Cost savings: 9 × $50 (estimated manual cost) = $450

			By("Simulating alert storm: 10 concurrent requests for same target")
			var wg sync.WaitGroup
			wfeNames := make([]string, 10)

			for i := 0; i < 10; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					name := fmt.Sprintf("storm-%d-%s", index, RandomString(5))
					wfeNames[index] = name
					CreateTestWFE(ctx, k8sClient, name, namespace, targetResource, nil)
				}(i)
			}
			wg.Wait()

			By("Waiting for all WFEs to reach terminal state")
			time.Sleep(10 * time.Second)

			By("Counting execution vs skip")
			var runningCount, skippedCount int
			for _, name := range wfeNames {
				wfe := WaitForWFEPhase(ctx, k8sClient, namespace, name,
					workflowexecutionv1.Phase(""), 5*time.Second) // Any phase

				switch wfe.Status.Phase {
				case workflowexecutionv1.PhaseRunning:
					runningCount++
				case workflowexecutionv1.PhaseSkipped:
					skippedCount++
				}
			}

			By("Verifying business outcome: ≥90% efficiency")
			efficiency := float64(skippedCount) / float64(len(wfeNames)) * 100
			Expect(efficiency).To(BeNumerically(">=", 90.0),
				"Expected 90%+ skip rate for duplicate storm")

			By("Verifying exactly 1 execution")
			Expect(runningCount).To(Equal(1),
				"Exactly one remediation should execute")

			By("Calculating cost savings")
			estimatedManualCost := 50.0 // dollars per remediation
			costSavings := float64(skippedCount) * estimatedManualCost
			GinkgoWriter.Printf("Cost savings: $%.2f (%d skipped × $%.2f)\n",
				costSavings, skippedCount, estimatedManualCost)

			// Cleanup
			for _, name := range wfeNames {
				CleanupWFE(ctx, k8sClient, namespace, name)
			}
		})
	})

	Describe("BR-WE-010: Cooldown Prevents Wasteful Sequential Executions", func() {
		It("should block requests during cooldown, preventing redundant work", func() {
			// Business context: Multiple teams notice same issue and request remediation
			// Expected: First executes, subsequent blocked until cooldown

			By("Creating first remediation that will complete")
			wfe1Name := "cooldown-1-" + RandomString(5)
			CreateTestWFE(ctx, k8sClient, wfe1Name, namespace, targetResource, nil)

			By("Waiting for first to start Running")
			WaitForWFEPhase(ctx, k8sClient, namespace, wfe1Name,
				workflowexecutionv1.PhaseRunning, 30*time.Second)

			By("Completing first remediation")
			SimulatePipelineRunSuccess(ctx, k8sClient, targetResource)
			WaitForWFEPhase(ctx, k8sClient, namespace, wfe1Name,
				workflowexecutionv1.PhaseCompleted, 30*time.Second)

			By("Creating second remediation immediately (within cooldown)")
			wfe2Name := "cooldown-2-" + RandomString(5)
			CreateTestWFE(ctx, k8sClient, wfe2Name, namespace, targetResource, nil)

			By("Verifying second is skipped with RecentlyRemediated")
			wfe2 := WaitForWFEPhase(ctx, k8sClient, namespace, wfe2Name,
				workflowexecutionv1.PhaseSkipped, 30*time.Second)

			Expect(wfe2.Status.SkipDetails).ToNot(BeNil())
			Expect(wfe2.Status.SkipDetails.Reason).To(Equal("RecentlyRemediated"))

			By("Verifying business value: Prevented wasteful re-execution")
			GinkgoWriter.Printf("Business outcome: Prevented duplicate remediation during %v cooldown\n",
				5*time.Minute)

			// Cleanup
			CleanupWFE(ctx, k8sClient, namespace, wfe1Name)
			CleanupWFE(ctx, k8sClient, namespace, wfe2Name)
		})
	})
})
```

---

## 🏭 Infrastructure Helpers

**File**: `test/infrastructure/workflowexecution.go`

```go
package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// CreateCluster creates a Kind cluster for E2E tests
func CreateCluster(name, kubeconfigPath, kindConfigPath string, w io.Writer) error {
	cmd := exec.Command("kind", "create", "cluster",
		"--name", name,
		"--kubeconfig", kubeconfigPath,
		"--config", kindConfigPath,
		"--wait", "120s",
	)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

// DeleteCluster deletes a Kind cluster
func DeleteCluster(name string, w io.Writer) error {
	cmd := exec.Command("kind", "delete", "cluster", "--name", name)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

// InstallTekton installs Tekton Pipelines into the cluster
func InstallTekton(ctx context.Context, kubeconfigPath string, w io.Writer) error {
	tektonURL := "https://storage.googleapis.com/tekton-releases/pipeline/latest/release.yaml"
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", tektonURL)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return err
	}

	// Wait for Tekton to be ready
	return waitForDeployment(ctx, kubeconfigPath, "tekton-pipelines",
		"tekton-pipelines-controller", 120*time.Second, w)
}

// DeployWorkflowExecution deploys the controller to the cluster
func DeployWorkflowExecution(ctx context.Context, namespace, kubeconfigPath string, w io.Writer) error {
	// Apply CRDs
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", "config/crd/bases/")
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return err
	}

	// Apply deployment
	cmd = exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", "deploy/workflowexecution/")
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return err
	}

	return waitForDeployment(ctx, kubeconfigPath, namespace,
		"workflow-execution", 120*time.Second, w)
}

// CreateNamespace creates a namespace
func CreateNamespace(ctx context.Context, kubeconfigPath, name string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"create", "namespace", name, "--dry-run=client", "-o", "yaml")
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	apply := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", "-")
	apply.Stdin = bytes.NewReader(output)
	return apply.Run()
}

// ApplyRBAC applies RBAC for workflow execution
func ApplyRBAC(ctx context.Context, kubeconfigPath, namespace string) error {
	rbac := fmt.Sprintf(`
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-workflow-runner
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-workflow-runner
subjects:
- kind: ServiceAccount
  name: kubernaut-workflow-runner
  namespace: %s
roleRef:
  kind: ClusterRole
  name: kubernaut-workflow-runner
  apiGroup: rbac.authorization.k8s.io
`, namespace, namespace)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", "-")
	cmd.Stdin = strings.NewReader(rbac)
	return cmd.Run()
}

func waitForDeployment(ctx context.Context, kubeconfigPath, namespace, name string,
	timeout time.Duration, w io.Writer) error {

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "status", "deployment/"+name,
			"-n", namespace, "--timeout=10s")
		cmd.Stdout = w
		cmd.Stderr = w
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("deployment %s/%s not ready after %v", namespace, name, timeout)
}
```

---

## 📋 E2E Test Execution

### Run E2E Tests

```bash
# Create cluster and run E2E tests
make test-e2e-workflowexecution

# Or with Ginkgo directly (parallel with 4 procs)
ginkgo -p -procs=4 -v ./test/e2e/workflowexecution/...

# Run specific test
ginkgo -v -focus="should execute workflow from start to completion" ./test/e2e/workflowexecution/...
```

### E2E Test Makefile Target

```makefile
.PHONY: test-e2e-workflowexecution
test-e2e-workflowexecution:
	@echo "Running WorkflowExecution E2E tests..."
	ginkgo -p -procs=4 -v --timeout=20m ./test/e2e/workflowexecution/...
```

---

## References

- [E2E Test Setup Template](../../../../services/SERVICE_IMPLEMENTATION_PLAN_TEMPLATE.md#e2e-test-setup-2h)
- [DD-TEST-001: Port Allocation Strategy](../../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md)
- [Kind Cluster Test Template](../../../../testing/KIND_CLUSTER_TEST_TEMPLATE.md)

