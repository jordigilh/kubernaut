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

package contextapi

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Test suite configuration
var (
	kubeClient  *kubernetes.Clientset
	namespace   string
	ctx         context.Context
	cancel      context.CancelFunc
	logger      *zap.Logger
	clusterName string
)

func TestContextAPIE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Context API E2E Test Suite (Podman + Kind)")
}

var _ = BeforeSuite(func() {
	By("Initializing E2E test suite")

	// Initialize context
	ctx, cancel = context.WithCancel(context.Background())

	// Initialize logger
	logger, _ = zap.NewDevelopment()

	// Get cluster configuration from environment
	clusterName = os.Getenv("KIND_CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "kubernaut-contextapi-e2e"
	}

	namespace = os.Getenv("CONTEXT_API_NAMESPACE")
	if namespace == "" {
		namespace = "contextapi-e2e"
	}

	logger.Info("E2E test suite initialized",
		zap.String("cluster", clusterName),
		zap.String("namespace", namespace),
	)

	// Initialize Kubernetes client
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.Getenv("HOME") + "/.kube/config"
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	Expect(err).ToNot(HaveOccurred(), "Failed to load kubeconfig")

	// Override context to use Kind cluster
	config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
		&clientcmd.ConfigOverrides{CurrentContext: "kind-" + clusterName},
	).ClientConfig()
	Expect(err).ToNot(HaveOccurred(), "Failed to override context")

	kubeClient, err = kubernetes.NewForConfig(config)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Kubernetes client")

	// Verify cluster connectivity
	By("Verifying cluster connectivity")
	_, err = kubeClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	Expect(err).ToNot(HaveOccurred(), "Failed to connect to cluster or namespace does not exist")

	logger.Info("E2E infrastructure verified",
		zap.String("cluster", clusterName),
		zap.String("namespace", namespace),
	)
})

var _ = AfterSuite(func() {
	By("Cleaning up E2E test suite")

	if cancel != nil {
		cancel()
	}

	if logger != nil {
		_ = logger.Sync()
	}
})
