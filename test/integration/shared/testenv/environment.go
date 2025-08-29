package testenv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/sirupsen/logrus"
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

// SetupFakeEnvironment creates a fake Kubernetes environment for testing
func SetupFakeEnvironment() (*TestEnvironment, error) {
	return setupFakeK8sEnvironment()
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
