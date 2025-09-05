package monitoring_test

import (
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/test/integration/shared/testenv"
)

var _ = Describe("ClientFactory", func() {
	var (
		factory    *monitoring.ClientFactory
		k8sClient  k8s.Client
		testEnv    *testenv.TestEnvironment
		logger     *logrus.Logger
		config     monitoring.MonitoringConfig
		mockServer *httptest.Server
	)

	BeforeEach(func() {
		var err error
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)

		// Setup fake K8s environment - follows established pattern
		testEnv, err = testenv.SetupFakeEnvironment()
		Expect(err).NotTo(HaveOccurred())
		Expect(testEnv).NotTo(BeNil())

		// Create fake K8s client that uses real K8s types and validation
		k8sClient = testEnv.CreateK8sClient(logger)

		// Setup a mock server for health checks
		mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/-/healthy" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
			}
		}))
	})

	AfterEach(func() {
		if mockServer != nil {
			mockServer.Close()
		}
		if testEnv != nil {
			err := testEnv.Cleanup()
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Describe("NewClientFactory", func() {
		It("should create a new client factory", func() {
			config := monitoring.MonitoringConfig{}
			factory := monitoring.NewClientFactory(config, k8sClient, logger)
			Expect(factory).NotTo(BeNil())
		})
	})

	Describe("CreateClients", func() {
		Context("when using stub clients", func() {
			BeforeEach(func() {
				config = monitoring.MonitoringConfig{
					UseProductionClients: false,
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			It("should create stub monitoring clients", func() {
				clients := factory.CreateClients()

				Expect(clients).NotTo(BeNil())
				Expect(clients.AlertClient).NotTo(BeNil())
				Expect(clients.MetricsClient).NotTo(BeNil())
				Expect(clients.SideEffectDetector).NotTo(BeNil())

				// Verify these are stub implementations
				_, isStubAlert := clients.AlertClient.(*monitoring.StubAlertClient)
				Expect(isStubAlert).To(BeTrue())

				_, isStubMetrics := clients.MetricsClient.(*monitoring.StubMetricsClient)
				Expect(isStubMetrics).To(BeTrue())

				_, isStubSideEffect := clients.SideEffectDetector.(*monitoring.StubSideEffectDetector)
				Expect(isStubSideEffect).To(BeTrue())
			})
		})

		Context("when using production clients with all enabled", func() {
			BeforeEach(func() {
				config = monitoring.MonitoringConfig{
					UseProductionClients: true,
					AlertManagerConfig: monitoring.AlertManagerConfig{
						Enabled:  true,
						Endpoint: mockServer.URL,
						Timeout:  30 * time.Second,
					},
					PrometheusConfig: monitoring.PrometheusConfig{
						Enabled:  true,
						Endpoint: mockServer.URL,
						Timeout:  30 * time.Second,
					},
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			It("should create production monitoring clients", func() {
				clients := factory.CreateClients()

				Expect(clients).NotTo(BeNil())
				Expect(clients.AlertClient).NotTo(BeNil())
				Expect(clients.MetricsClient).NotTo(BeNil())
				Expect(clients.SideEffectDetector).NotTo(BeNil())

				// Verify these are production implementations
				_, isProdAlert := clients.AlertClient.(*monitoring.AlertManagerClient)
				Expect(isProdAlert).To(BeTrue())

				_, isProdMetrics := clients.MetricsClient.(*monitoring.PrometheusClient)
				Expect(isProdMetrics).To(BeTrue())

				_, isProdSideEffect := clients.SideEffectDetector.(*monitoring.EnhancedSideEffectDetector)
				Expect(isProdSideEffect).To(BeTrue())
			})
		})

		Context("when using production clients with partial enablement", func() {
			BeforeEach(func() {
				config = monitoring.MonitoringConfig{
					UseProductionClients: true,
					AlertManagerConfig: monitoring.AlertManagerConfig{
						Enabled:  true,
						Endpoint: mockServer.URL,
						Timeout:  30 * time.Second,
					},
					PrometheusConfig: monitoring.PrometheusConfig{
						Enabled:  false, // Disabled
						Endpoint: mockServer.URL,
						Timeout:  30 * time.Second,
					},
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			It("should create mixed monitoring clients", func() {
				clients := factory.CreateClients()

				Expect(clients).NotTo(BeNil())
				Expect(clients.AlertClient).NotTo(BeNil())
				Expect(clients.MetricsClient).NotTo(BeNil())
				Expect(clients.SideEffectDetector).NotTo(BeNil())

				// AlertManager should be production, Prometheus should be stub
				_, isProdAlert := clients.AlertClient.(*monitoring.AlertManagerClient)
				Expect(isProdAlert).To(BeTrue())

				_, isStubMetrics := clients.MetricsClient.(*monitoring.StubMetricsClient)
				Expect(isStubMetrics).To(BeTrue())

				// Side effect detector should be enhanced because AlertManager is enabled
				_, isProdSideEffect := clients.SideEffectDetector.(*monitoring.EnhancedSideEffectDetector)
				Expect(isProdSideEffect).To(BeTrue())
			})
		})

		Context("when production clients are enabled but all services disabled", func() {
			BeforeEach(func() {
				config = monitoring.MonitoringConfig{
					UseProductionClients: true,
					AlertManagerConfig: monitoring.AlertManagerConfig{
						Enabled:  false,
						Endpoint: mockServer.URL,
						Timeout:  30 * time.Second,
					},
					PrometheusConfig: monitoring.PrometheusConfig{
						Enabled:  false,
						Endpoint: mockServer.URL,
						Timeout:  30 * time.Second,
					},
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			It("should fallback to stub clients", func() {
				clients := factory.CreateClients()

				Expect(clients).NotTo(BeNil())

				// All should be stub implementations
				_, isStubAlert := clients.AlertClient.(*monitoring.StubAlertClient)
				Expect(isStubAlert).To(BeTrue())

				_, isStubMetrics := clients.MetricsClient.(*monitoring.StubMetricsClient)
				Expect(isStubMetrics).To(BeTrue())

				_, isStubSideEffect := clients.SideEffectDetector.(*monitoring.StubSideEffectDetector)
				Expect(isStubSideEffect).To(BeTrue())
			})
		})
	})

	Describe("ValidateConfig", func() {
		Context("when using stub clients", func() {
			BeforeEach(func() {
				config = monitoring.MonitoringConfig{
					UseProductionClients: false,
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			It("should skip validation and succeed", func() {
				err := factory.ValidateConfig()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when using production clients with valid config", func() {
			BeforeEach(func() {
				config = monitoring.MonitoringConfig{
					UseProductionClients: true,
					AlertManagerConfig: monitoring.AlertManagerConfig{
						Enabled:  true,
						Endpoint: "http://localhost:9093",
						Timeout:  30 * time.Second,
					},
					PrometheusConfig: monitoring.PrometheusConfig{
						Enabled:  true,
						Endpoint: "http://localhost:9090",
						Timeout:  30 * time.Second,
					},
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			It("should validate successfully", func() {
				err := factory.ValidateConfig()
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when using production clients with invalid AlertManager config", func() {
			BeforeEach(func() {
				config = monitoring.MonitoringConfig{
					UseProductionClients: true,
					AlertManagerConfig: monitoring.AlertManagerConfig{
						Enabled:  true,
						Endpoint: "", // Invalid: empty endpoint
						Timeout:  30 * time.Second,
					},
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			It("should return validation error", func() {
				err := factory.ValidateConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("AlertManager endpoint is required"))
			})
		})

		Context("when using production clients with invalid timeout", func() {
			BeforeEach(func() {
				config = monitoring.MonitoringConfig{
					UseProductionClients: true,
					PrometheusConfig: monitoring.PrometheusConfig{
						Enabled:  true,
						Endpoint: "http://localhost:9090",
						Timeout:  0, // Invalid: zero timeout
					},
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			It("should return validation error", func() {
				err := factory.ValidateConfig()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Prometheus timeout must be positive"))
			})
		})
	})

	Describe("HealthCheck", func() {
		Context("when using stub clients", func() {
			BeforeEach(func() {
				config = monitoring.MonitoringConfig{
					UseProductionClients: false,
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			It("should skip health checks", func() {
				clients := factory.CreateClients()
				err := factory.HealthCheck(clients)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when using production clients with healthy services", func() {
			BeforeEach(func() {
				config = monitoring.MonitoringConfig{
					UseProductionClients: true,
					AlertManagerConfig: monitoring.AlertManagerConfig{
						Enabled:  true,
						Endpoint: mockServer.URL,
						Timeout:  30 * time.Second,
					},
					PrometheusConfig: monitoring.PrometheusConfig{
						Enabled:  true,
						Endpoint: mockServer.URL,
						Timeout:  30 * time.Second,
					},
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			It("should pass health checks", func() {
				clients := factory.CreateClients()
				err := factory.HealthCheck(clients)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when using production clients with unhealthy services", func() {
			var unhealthyServer *httptest.Server

			BeforeEach(func() {
				unhealthyServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusServiceUnavailable)
				}))

				config = monitoring.MonitoringConfig{
					UseProductionClients: true,
					AlertManagerConfig: monitoring.AlertManagerConfig{
						Enabled:  true,
						Endpoint: unhealthyServer.URL,
						Timeout:  30 * time.Second,
					},
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			AfterEach(func() {
				unhealthyServer.Close()
			})

			It("should fail health checks", func() {
				clients := factory.CreateClients()
				err := factory.HealthCheck(clients)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("AlertManager health check failed"))
			})
		})
	})

	Describe("GetConfigSummary", func() {
		BeforeEach(func() {
			config = monitoring.MonitoringConfig{
				UseProductionClients: true,
				AlertManagerConfig: monitoring.AlertManagerConfig{
					Enabled:  true,
					Endpoint: "http://localhost:9093",
					Timeout:  30 * time.Second,
				},
				PrometheusConfig: monitoring.PrometheusConfig{
					Enabled:  false,
					Endpoint: "http://localhost:9090",
					Timeout:  45 * time.Second,
				},
			}
			factory = monitoring.NewClientFactory(config, k8sClient, logger)
		})

		It("should return configuration summary", func() {
			summary := factory.GetConfigSummary()

			Expect(summary).To(HaveKey("use_production_clients"))
			Expect(summary["use_production_clients"]).To(BeTrue())

			Expect(summary).To(HaveKey("alertmanager"))
			alertmanagerConfig := summary["alertmanager"].(map[string]interface{})
			Expect(alertmanagerConfig["enabled"]).To(BeTrue())
			Expect(alertmanagerConfig["endpoint"]).To(Equal("http://localhost:9093"))
			Expect(alertmanagerConfig["timeout"]).To(Equal("30s"))

			Expect(summary).To(HaveKey("prometheus"))
			prometheusConfig := summary["prometheus"].(map[string]interface{})
			Expect(prometheusConfig["enabled"]).To(BeFalse())
			Expect(prometheusConfig["endpoint"]).To(Equal("http://localhost:9090"))
			Expect(prometheusConfig["timeout"]).To(Equal("45s"))
		})
	})

	Describe("Integration Tests", func() {
		Context("full configuration cycle", func() {
			BeforeEach(func() {
				config = monitoring.MonitoringConfig{
					UseProductionClients: true,
					AlertManagerConfig: monitoring.AlertManagerConfig{
						Enabled:  true,
						Endpoint: mockServer.URL,
						Timeout:  30 * time.Second,
					},
					PrometheusConfig: monitoring.PrometheusConfig{
						Enabled:  true,
						Endpoint: mockServer.URL,
						Timeout:  30 * time.Second,
					},
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			It("should validate, create, and health check clients successfully", func() {
				// Step 1: Validate configuration
				err := factory.ValidateConfig()
				Expect(err).NotTo(HaveOccurred())

				// Step 2: Create clients
				clients := factory.CreateClients()
				Expect(clients).NotTo(BeNil())
				Expect(clients.AlertClient).NotTo(BeNil())
				Expect(clients.MetricsClient).NotTo(BeNil())
				Expect(clients.SideEffectDetector).NotTo(BeNil())

				// Step 3: Health check
				err = factory.HealthCheck(clients)
				Expect(err).NotTo(HaveOccurred())

				// Step 4: Get summary
				summary := factory.GetConfigSummary()
				Expect(summary["use_production_clients"]).To(BeTrue())
			})
		})

		Context("fallback behavior", func() {
			BeforeEach(func() {
				config = monitoring.MonitoringConfig{
					UseProductionClients: true,
					AlertManagerConfig: monitoring.AlertManagerConfig{
						Enabled:  false, // All disabled
						Endpoint: "http://localhost:9093",
						Timeout:  30 * time.Second,
					},
					PrometheusConfig: monitoring.PrometheusConfig{
						Enabled:  false, // All disabled
						Endpoint: "http://localhost:9090",
						Timeout:  30 * time.Second,
					},
				}
				factory = monitoring.NewClientFactory(config, k8sClient, logger)
			})

			It("should gracefully fallback to stub clients", func() {
				// Validation should pass
				err := factory.ValidateConfig()
				Expect(err).NotTo(HaveOccurred())

				// Should create stub clients despite production flag
				clients := factory.CreateClients()

				_, isStubAlert := clients.AlertClient.(*monitoring.StubAlertClient)
				Expect(isStubAlert).To(BeTrue())

				_, isStubMetrics := clients.MetricsClient.(*monitoring.StubMetricsClient)
				Expect(isStubMetrics).To(BeTrue())

				_, isStubSideEffect := clients.SideEffectDetector.(*monitoring.StubSideEffectDetector)
				Expect(isStubSideEffect).To(BeTrue())

				// Health check should pass
				err = factory.HealthCheck(clients)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
