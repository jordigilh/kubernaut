package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/jordigilh/kubernaut/internal/config"
)

// PlatformTestSuiteComponents contains common test setup components for platform tests
type PlatformTestSuiteComponents struct {
	Context       context.Context
	Logger        *logrus.Logger
	FakeClientset *fake.Clientset
	MockServer    *httptest.Server
}

// PlatformTestSuite creates a standardized test suite setup for platform tests
func PlatformTestSuite(testName string) *PlatformTestSuiteComponents {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests

	return &PlatformTestSuiteComponents{
		Context:       context.Background(),
		Logger:        logger,
		FakeClientset: fake.NewSimpleClientset(),
	}
}

// K8sTestSuite creates a standardized test suite setup for Kubernetes tests
func K8sTestSuite(testName string) *PlatformTestSuiteComponents {
	return PlatformTestSuite(testName)
}

// MonitoringTestSuite creates a standardized test suite setup for monitoring tests
func MonitoringTestSuite(testName string) *PlatformTestSuiteComponents {
	return PlatformTestSuite(testName)
}

// ExecutorTestSuite creates a standardized test suite setup for executor tests
func ExecutorTestSuite(testName string) *PlatformTestSuiteComponents {
	return PlatformTestSuite(testName)
}

// CreateMockServer creates and starts a mock HTTP server for testing
func (c *PlatformTestSuiteComponents) CreateMockServer(handler http.Handler) {
	c.MockServer = httptest.NewServer(handler)
}

// CloseMockServer safely closes the mock server if it exists
func (c *PlatformTestSuiteComponents) CloseMockServer() {
	if c.MockServer != nil {
		c.MockServer.Close()
		c.MockServer = nil
	}
}

// CreateK8sConfig creates a standard Kubernetes configuration for testing
func (c *PlatformTestSuiteComponents) CreateK8sConfig() config.KubernetesConfig {
	return config.KubernetesConfig{
		Namespace: "test-namespace",
	}
}
