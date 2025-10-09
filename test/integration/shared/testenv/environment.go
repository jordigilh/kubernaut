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

package testenv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// TestEnvironment manages the test Kubernetes environment
type TestEnvironment struct {
	Environment *envtest.Environment
	Config      *rest.Config
	Client      kubernetes.Interface
	Context     context.Context
	CancelFunc  context.CancelFunc
}

// GetKubeconfigForContainer returns the kubeconfig content as a string for container use
func (te *TestEnvironment) GetKubeconfigForContainer() (string, error) {
	if te.Config == nil {
		return "", fmt.Errorf("test environment config not available")
	}

	// Use certificate-based authentication when available (more appropriate for envtest)
	var userConfig string
	if te.Config.BearerToken != "" {
		userConfig = fmt.Sprintf("    token: %s", te.Config.BearerToken)
	} else if len(te.Config.CertData) > 0 && len(te.Config.KeyData) > 0 {
		// Use client certificate authentication
		userConfig = fmt.Sprintf(`    client-certificate-data: %s
    client-key-data: %s`,
			string(te.Config.CertData), string(te.Config.KeyData))
	} else {
		return "", fmt.Errorf("no authentication method available (no bearer token or client certificates)")
	}

	kubeconfigContent := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
    insecure-skip-tls-verify: true
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
users:
- name: test-user
  user:
%s
`, te.Config.Host, userConfig)

	return kubeconfigContent, nil
}

// SetupTestEnvironment initializes a test Kubernetes environment
func SetupTestEnvironment() (*TestEnvironment, error) {
	// Get KUBEBUILDER_ASSETS from environment (set by Makefile)
	binaryDir, err := getKubeBuilderAssets()
	if err != nil {
		return nil, fmt.Errorf("failed to locate Kubernetes test binaries: %w", err)
	}

	logrus.WithField("binaryDir", binaryDir).Info("Using Kubernetes test binaries from KUBEBUILDER_ASSETS")

	// Validate that required binaries exist
	if err := validateKubeBinaries(binaryDir); err != nil {
		return nil, fmt.Errorf("invalid Kubernetes binaries in %s: %w", binaryDir, err)
	}

	// Create test environment
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{},
		ErrorIfCRDPathMissing: false,
		BinaryAssetsDirectory: binaryDir,
	}

	// Start the test environment
	cfg, err := testEnv.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start test environment: %w", err)
	}

	// Create Kubernetes client
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	env := &TestEnvironment{
		Environment: testEnv,
		Config:      cfg,
		Client:      client,
		Context:     ctx,
		CancelFunc:  cancel,
	}

	// Create default namespace
	if err := env.CreateDefaultNamespace(); err != nil {
		logrus.WithError(err).Debug("Namespace creation failed (might already exist)")
	}

	return env, nil
}

// SetupEnvironment creates a test Kubernetes environment for integration testing
// Uses real K8s cluster (envtest) by default, or fake client if USE_FAKE_K8S_CLIENT=true
func SetupEnvironment() (*TestEnvironment, error) {
	// Check if we should use fake client (for backward compatibility and fast tests)
	if os.Getenv("USE_FAKE_K8S_CLIENT") == "true" {
		logrus.Info("Using fake Kubernetes client for integration tests")
		return setupFakeK8sEnvironment()
	}

	// Use real Kubernetes environment by default
	logrus.Info("Using real Kubernetes cluster (envtest) for integration tests")
	env, err := SetupTestEnvironment()
	if err != nil {
		logrus.WithError(err).Error("Failed to setup real K8s test environment - integration tests require real environment")
		// Integration tests MUST fail if real environment is unavailable
		return nil, fmt.Errorf("integration test environment setup failed: %w", err)
	}

	return env, nil
}

// CreateDefaultNamespace creates the default namespace in the environment
func (te *TestEnvironment) CreateDefaultNamespace() error {
	// Check if we already have a default namespace (real cluster might have it)
	ns, err := te.Client.CoreV1().Namespaces().Get(te.Context, "default", metav1.GetOptions{})
	if err == nil && ns != nil {
		logrus.Debug("Default namespace already exists in test environment")
		return nil
	}

	// Create default namespace
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}

	_, err = te.Client.CoreV1().Namespaces().Create(te.Context, namespace, metav1.CreateOptions{})
	if err != nil {
		logrus.WithError(err).Error("Failed to create default namespace in test environment")
		return err
	}

	logrus.Debug("Created default namespace in test environment")
	return nil
}

// CreateK8sClient creates a k8s.Client using the test environment
func (te *TestEnvironment) CreateK8sClient(logger *logrus.Logger) k8s.Client {
	if logger == nil {
		logger = logrus.New()
	}

	logger.Debug("Creating unified K8s client for real test environment")
	return k8s.NewUnifiedClient(te.Client, config.KubernetesConfig{
		Namespace: "default",
	}, logger)
}

// Cleanup tears down the test environment
func (te *TestEnvironment) Cleanup() error {
	te.CancelFunc()

	if te.Environment != nil {
		return te.Environment.Stop()
	}
	return nil
}

// getKubeBuilderAssets returns the path to Kubernetes test binaries
func getKubeBuilderAssets() (string, error) {
	// First try environment variable (set by Makefile)
	if binaryDir := os.Getenv("KUBEBUILDER_ASSETS"); binaryDir != "" {
		logrus.WithField("source", "KUBEBUILDER_ASSETS").Debugf("Using binary directory: %s", binaryDir)
		return binaryDir, nil
	}

	// Fallback: try to find binaries in expected locations
	baseDir := filepath.Join("..", "..", "bin", "k8s")

	// Try to find a platform-specific directory
	platformDirs := []string{
		fmt.Sprintf("1.34.0-%s-%s", runtime.GOOS, runtime.GOARCH),
		fmt.Sprintf("1.33.0-%s-%s", runtime.GOOS, runtime.GOARCH),
	}

	for _, dir := range platformDirs {
		candidatePath := filepath.Join(baseDir, dir)
		if _, err := os.Stat(candidatePath); err == nil {
			logrus.WithField("source", "fallback").Debugf("Found binary directory: %s", candidatePath)
			return candidatePath, nil
		}
	}

	return "", fmt.Errorf("KUBEBUILDER_ASSETS not set and no suitable binary directory found in %s", baseDir)
}

// validateKubeBinaries checks that required Kubernetes binaries exist
func validateKubeBinaries(binaryDir string) error {
	requiredBinaries := []string{"etcd", "kube-apiserver", "kubectl"}

	for _, binary := range requiredBinaries {
		binaryPath := filepath.Join(binaryDir, binary)
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			return fmt.Errorf("required binary %s not found at %s", binary, binaryPath)
		}
	}

	logrus.WithField("binaryDir", binaryDir).Debug("All required Kubernetes binaries found")
	return nil
}
