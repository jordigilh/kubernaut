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
	"fmt"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	workflowexecutioncontroller "github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
)

// Test suite variables
var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestWorkflowExecutionIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "WorkflowExecution Controller Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.Background())

	By("setting up envtest environment")
	// Use setup-envtest to get binaries if KUBEBUILDER_ASSETS is not set
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		By("running setup-envtest to get binaries")
		cmd := exec.Command("go", "run", "sigs.k8s.io/controller-runtime/tools/setup-envtest@latest", "use", "1.28.x", "--bin-dir", "/tmp/envtest-bins", "-p", "path")
		output, err := cmd.Output()
		if err != nil {
			Skip(fmt.Sprintf("setup-envtest not available, skipping integration tests: %v", err))
		}
		envtestPath := strings.TrimSpace(string(output))
		os.Setenv("KUBEBUILDER_ASSETS", envtestPath)
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

	// Register schemes
	err = workflowexecutionv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = tektonv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create manager
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics in tests
		},
	})
	Expect(err).ToNot(HaveOccurred())

	// Setup controller with test configuration
	reconciler := workflowexecutioncontroller.NewWorkflowExecutionReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		"kubernaut-workflows",
	)
	reconciler.CooldownPeriod = 1 * time.Minute // Shorter for tests
	err = reconciler.SetupWithManager(mgr)
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

