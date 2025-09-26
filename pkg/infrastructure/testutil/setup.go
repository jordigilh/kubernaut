package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/sirupsen/logrus"

	. "github.com/onsi/gomega" //nolint:staticcheck
)

// InfrastructureConfig represents basic infrastructure configuration for testing
type InfrastructureConfig struct {
	Port                  int           `yaml:"port" default:"8080"`
	ReadTimeout           time.Duration `yaml:"read_timeout" default:"30s"`
	WriteTimeout          time.Duration `yaml:"write_timeout" default:"30s"`
	ShutdownTimeout       time.Duration `yaml:"shutdown_timeout" default:"10s"`
	EnableMetrics         bool          `yaml:"enable_metrics" default:"true"`
	MetricsPath           string        `yaml:"metrics_path" default:"/metrics"`
	HealthPath            string        `yaml:"health_path" default:"/health"`
	ReadyPath             string        `yaml:"ready_path" default:"/ready"`
	MaxRequestsPerSecond  int           `yaml:"max_requests_per_second" default:"100"`
	EnableRequestLogging  bool          `yaml:"enable_request_logging" default:"true"`
	EnableDetailedMetrics bool          `yaml:"enable_detailed_metrics" default:"false"`
	Namespace             string        `yaml:"namespace" default:"default"`
	ServiceName           string        `yaml:"service_name" default:"service"`
}

// InfrastructureTestSuiteComponents contains common test setup components for infrastructure tests
type InfrastructureTestSuiteComponents struct {
	Context    context.Context
	Logger     *logrus.Logger
	MockServer *httptest.Server
	HTTPClient *http.Client
	Config     *InfrastructureConfig
}

// InfrastructureTestSuite creates a standardized test suite setup for infrastructure tests
func InfrastructureTestSuite(testName string) *InfrastructureTestSuiteComponents {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests

	// Create HTTP client with reasonable timeouts for testing
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}

	return &InfrastructureTestSuiteComponents{
		Context:    context.Background(),
		Logger:     logger,
		HTTPClient: httpClient,
		Config:     createDefaultInfrastructureConfig(),
	}
}

// MetricsTestSuite creates a specialized setup for metrics tests
func MetricsTestSuite(testName string) *InfrastructureTestSuiteComponents {
	components := InfrastructureTestSuite(testName)

	// Create mock metrics server
	components.MockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Default handler - can be overridden per test
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
		Expect(err).NotTo(HaveOccurred())
	}))

	return components
}

// ServerTestSuite creates a specialized setup for server tests
func ServerTestSuite(testName string) *InfrastructureTestSuiteComponents {
	components := InfrastructureTestSuite(testName)

	// Enhanced config for server testing
	components.Config = &InfrastructureConfig{
		Port:            8080,
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
		EnableMetrics:   true,
		MetricsPath:     "/metrics",
		HealthPath:      "/health",
		ReadyPath:       "/ready",
	}

	return components
}

// MonitoringTestSuite creates a specialized setup for monitoring tests
func MonitoringTestSuite(testName string) *InfrastructureTestSuiteComponents {
	return MetricsTestSuite(testName)
}

// PerformanceTestSuite creates a specialized setup for performance tests
func PerformanceTestSuite(testName string) *InfrastructureTestSuiteComponents {
	components := InfrastructureTestSuite(testName)

	// Performance testing specific configuration
	components.Config = &InfrastructureConfig{
		Port:                  8080,
		ReadTimeout:           10 * time.Second,
		WriteTimeout:          10 * time.Second,
		ShutdownTimeout:       5 * time.Second,
		EnableMetrics:         true,
		MetricsPath:           "/metrics",
		MaxRequestsPerSecond:  1000,
		EnableRequestLogging:  false, // Reduce overhead
		EnableDetailedMetrics: true,
	}

	return components
}

// CleanupMockServer cleans up the mock server
func (c *InfrastructureTestSuiteComponents) CleanupMockServer() {
	if c.MockServer != nil {
		c.MockServer.Close()
		c.MockServer = nil
	}
}

// SetupMockServerWithHandler sets up a mock server with custom handler
func (c *InfrastructureTestSuiteComponents) SetupMockServerWithHandler(handler http.HandlerFunc) {
	c.CleanupMockServer()
	c.MockServer = httptest.NewServer(handler)
}

// GetMockServerURL returns the mock server URL or empty string if not set up
func (c *InfrastructureTestSuiteComponents) GetMockServerURL() string {
	if c.MockServer != nil {
		return c.MockServer.URL
	}
	return ""
}

// UpdateConfig updates the infrastructure configuration
func (c *InfrastructureTestSuiteComponents) UpdateConfig(config *InfrastructureConfig) {
	c.Config = config
}

// GetDefaultConfig returns the default infrastructure configuration
func (c *InfrastructureTestSuiteComponents) GetDefaultConfig() *InfrastructureConfig {
	return c.Config
}

// GetTestingHTTPClient returns HTTP client configured for testing
func (c *InfrastructureTestSuiteComponents) GetTestingHTTPClient() *http.Client {
	return c.HTTPClient
}

// SetHTTPClientTimeout updates the HTTP client timeout
func (c *InfrastructureTestSuiteComponents) SetHTTPClientTimeout(timeout time.Duration) {
	c.HTTPClient.Timeout = timeout
}

// createDefaultInfrastructureConfig creates default infrastructure configuration for testing
func createDefaultInfrastructureConfig() *InfrastructureConfig {
	return &InfrastructureConfig{
		Port:                  8080,
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          30 * time.Second,
		ShutdownTimeout:       10 * time.Second,
		EnableMetrics:         true,
		MetricsPath:           "/metrics",
		HealthPath:            "/health",
		ReadyPath:             "/ready",
		MaxRequestsPerSecond:  100,
		EnableRequestLogging:  true,
		EnableDetailedMetrics: false,
		Namespace:             "test",
		ServiceName:           "test-service",
	}
}
