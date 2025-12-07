/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workflowexecution

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// WorkflowExecution Integration Test Suite
//
// Defense-in-Depth Strategy (per 03-testing-strategy.mdc):
// - Unit tests (70%+): Business logic in isolation - 70.5% achieved
// - Integration tests (>50%): CRD operations with real K8s API
// - E2E tests (10-15%): Complete workflow validation with Tekton
//
// Integration tests focus on (EnvTest - NO Tekton CRDs):
// - CRD lifecycle with real Kubernetes API
// - CRD field storage and validation
// - Status subresource updates
// - Finalizer lifecycle
// - SkipDetails persistence (all 4 skip reasons)
// - Exponential backoff state tracking
//
// NOTE: Cross-namespace PipelineRun creation requires Tekton CRDs
// and is tested in E2E suite (test/e2e/workflowexecution/) with KIND + Tekton.
// This is by design per IMPLEMENTATION_PLAN_V3.7.md - EnvTest cannot install Tekton.

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

// Test namespaces (unique per test run for parallel safety)
const (
	DefaultNamespace          = "default"
	WorkflowExecutionNS       = "kubernaut-workflows"
	IntegrationTestNamePrefix = "int-test-"
)

func TestWorkflowExecutionIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution Controller Integration Suite (EnvTest)")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("Registering CRD schemes")
	err := workflowexecutionv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = tektonv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("Bootstrapping test environment with WorkflowExecution and Tekton CRDs")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
			// Tekton CRDs need to be installed separately or downloaded
			// For now, we'll test without actual Tekton CRDs and mock PipelineRun behavior
		},
		ErrorIfCRDPathMissing: false, // Allow missing Tekton CRDs for now
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	By("Creating controller-runtime client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("Creating namespaces for testing")
	// Create kubernaut-workflows namespace for PipelineRuns
	workflowsNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: WorkflowExecutionNS,
		},
	}
	err = k8sClient.Create(ctx, workflowsNs)
	Expect(err).NotTo(HaveOccurred())

	// Create default namespace for tests
	defaultNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultNamespace,
		},
	}
	_ = k8sClient.Create(ctx, defaultNs) // May already exist

	GinkgoWriter.Println("✅ Namespaces created: kubernaut-workflows, default")

	// NOTE: Controller is NOT started in EnvTest integration tests
	// because Tekton CRDs are not available in EnvTest.
	//
	// Integration tests focus on:
	// - CRD schema validation
	// - CRUD operations on WorkflowExecution
	// - Status field updates
	// - Field indexing (manual validation)
	//
	// Controller behavior tests are in E2E tests (KIND + Tekton)

	GinkgoWriter.Println("✅ WorkflowExecution integration test environment ready!")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Environment:")
	GinkgoWriter.Println("  • EnvTest with real Kubernetes API (etcd + kube-apiserver)")
	GinkgoWriter.Println("  • WorkflowExecution CRD installed")
	GinkgoWriter.Println("  • Controller NOT running (Tekton CRDs not available)")
	GinkgoWriter.Println("  • Tests: CRD operations, schema validation, status updates")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("NOTE: Full controller tests are in E2E suite (KIND + Tekton)")
	GinkgoWriter.Println("")
})

var _ = AfterSuite(func() {
	By("Tearing down the test environment")

	cancel()

	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

	GinkgoWriter.Println("✅ Cleanup complete")
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

// ========================================
// Test Helpers - Parallel-Safe (4 procs)
// ========================================

// createUniqueWFE creates a WorkflowExecution with unique name for parallel test isolation
func createUniqueWFE(testID, targetResource string) *workflowexecutionv1alpha1.WorkflowExecution {
	name := IntegrationTestNamePrefix + testID + "-" + time.Now().Format("150405")
	return &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: DefaultNamespace,
		},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID:     "test-workflow",
				Version:        "v1.0.0",
				ContainerImage: "ghcr.io/kubernaut/workflows/test@sha256:abc123",
			},
			TargetResource: targetResource,
		},
	}
}

// getWFE gets a WorkflowExecution by name
func getWFE(name, namespace string) (*workflowexecutionv1alpha1.WorkflowExecution, error) {
	wfe := &workflowexecutionv1alpha1.WorkflowExecution{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, wfe)
	return wfe, err
}

// deleteWFEAndWait deletes a WorkflowExecution and waits for it to be fully removed
func deleteWFEAndWait(wfe *workflowexecutionv1alpha1.WorkflowExecution, timeout time.Duration) error {
	if err := k8sClient.Delete(ctx, wfe); err != nil {
		return err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextTimeout(timeoutCtx, 100*time.Millisecond, timeout, true, func(ctx context.Context) (bool, error) {
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      wfe.Name,
			Namespace: wfe.Namespace,
		}, &workflowexecutionv1alpha1.WorkflowExecution{})

		if err != nil {
			// Object not found = deletion complete
			return true, nil
		}

		// Still exists, keep waiting
		return false, nil
	})
}

