package testenv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
	// Use KUBEBUILDER_ASSETS if set, otherwise use relative path to bin directory
	binaryDir := os.Getenv("KUBEBUILDER_ASSETS")
	if binaryDir == "" {
		binaryDir = filepath.Join("..", "..", "bin", "k8s")
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
