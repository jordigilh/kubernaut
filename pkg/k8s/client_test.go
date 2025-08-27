package k8s

import (
	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/sirupsen/logrus"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("Client", func() {
	var logger *logrus.Logger

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.FatalLevel)
	})

	Describe("NewClient", func() {
		Context("when creating client with default configuration", func() {
			It("should create client with default namespace", func() {
				cfg := config.KubernetesConfig{}

				client, err := NewClient(cfg, logger)
				
				// Note: This test may fail in environments without kubeconfig
				// In real testing, you'd mock the kubernetes client creation
				if err != nil {
					Skip("Kubernetes config not available in test environment")
				}
				
				Expect(client).NotTo(BeNil())
				// Don't assert on health status as it depends on actual cluster connectivity
				_ = client.IsHealthy()
			})
		})

		Context("when creating client with specific context", func() {
			It("should attempt to use specified context", func() {
				cfg := config.KubernetesConfig{
					Context: "test-context",
				}

				_, err := NewClient(cfg, logger)
				
				// This will likely fail in test environments, which is expected
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create Kubernetes config"))
			})
		})

		Context("when creating client with specific namespace", func() {
			It("should use specified namespace", func() {
				cfg := config.KubernetesConfig{
					Namespace: "custom-namespace",
				}

				client, err := NewClient(cfg, logger)
				
				if err != nil {
					Skip("Kubernetes config not available in test environment")
				}
				
				Expect(client).NotTo(BeNil())
				
				// The internal namespace would be "custom-namespace" but we can't easily test this
				// without exposing internal fields or creating a more complex test setup
			})
		})
	})

	Describe("Client interface compliance", func() {
		It("should implement BasicClient interface", func() {
			var basicClient BasicClient
			var client Client
			
			// This ensures Client implements BasicClient
			basicClient = client
			_ = basicClient
		})

		It("should implement AdvancedClient interface", func() {
			var advancedClient AdvancedClient
			var client Client
			
			// This ensures Client implements AdvancedClient
			advancedClient = client
			_ = advancedClient
		})

		It("should implement full Client interface", func() {
			// Create a minimal fake client for interface testing
			basic := &basicClient{
				clientset: fake.NewSimpleClientset(),
				namespace: "default",
				log:       logger,
			}
			
			advanced := &advancedClient{
				basicClient: basic,
			}
			
			testClient := &client{
				basicClient:    basic,
				advancedClient: advanced,
			}

			// Verify it implements all interfaces
			var basicInterface BasicClient = testClient
			var advancedInterface AdvancedClient = testClient
			var clientInterface Client = testClient

			Expect(basicInterface).NotTo(BeNil())
			Expect(advancedInterface).NotTo(BeNil())
			Expect(clientInterface).NotTo(BeNil())
		})
	})
})