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

package workflowexecution_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
)

// These tests use envtest to test controller behavior with a real K8s API server

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestWorkflowExecutionIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("setting up KUBEBUILDER_ASSETS if not already set")
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// First try local bin directory
		assetsPath := getFirstFoundEnvTestBinaryDir()
		if assetsPath != "" {
			os.Setenv("KUBEBUILDER_ASSETS", assetsPath)
			GinkgoWriter.Printf("Using local KUBEBUILDER_ASSETS: %s\n", assetsPath)
		} else {
			// Fall back to setup-envtest
			cmd := exec.Command("go", "run", "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest", "use", "-p", "path")
			output, err := cmd.Output()
			if err != nil {
				Skip("Unable to find or download envtest binaries. Run 'make setup-envtest' first.")
			}
			assetsPath = strings.TrimSpace(string(output))
			os.Setenv("KUBEBUILDER_ASSETS", assetsPath)
			GinkgoWriter.Printf("Using downloaded KUBEBUILDER_ASSETS: %s\n", assetsPath)
		}
	}

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

	err = workflowexecutionv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	if testEnv != nil {
		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	}
})

// ============================================================================
// Helper Functions for Integration Tests
// ============================================================================

// getFirstFoundEnvTestBinaryDir looks for envtest binaries in the local bin directory
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")

	// Check if directory exists
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return ""
	}

	// Look for a versioned subdirectory
	for _, entry := range entries {
		if entry.IsDir() {
			candidatePath := filepath.Join(basePath, entry.Name())
			// Check if kube-apiserver exists in this directory
			if _, err := os.Stat(filepath.Join(candidatePath, "kube-apiserver")); err == nil {
				return candidatePath
			}
		}
	}

	return ""
}

func createTestNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
}

func deleteTestNamespace(name string) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_ = k8sClient.Delete(ctx, ns)
}

func createWorkflowExecution(name, namespace, targetResource, workflowID string) *workflowexecutionv1alpha1.WorkflowExecution {
	wfe := &workflowexecutionv1alpha1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      "test-remediation",
				Namespace: namespace,
			},
			WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
				WorkflowID:     workflowID,
				Version:        "1.0.0",
				ContainerImage: "quay.io/kubernaut/workflow-test:v1.0.0",
			},
			TargetResource: targetResource,
			Confidence:     0.9,
		},
	}
	return wfe
}

func waitForPhase(name, namespace, expectedPhase string, timeout time.Duration) {
	Eventually(func() string {
		wfe := &workflowexecutionv1alpha1.WorkflowExecution{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, wfe)
		if err != nil {
			return ""
		}
		return wfe.Status.Phase
	}, timeout, 100*time.Millisecond).Should(Equal(expectedPhase))
}
