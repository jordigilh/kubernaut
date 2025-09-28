package main

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// Business Requirements: BR-AI-MAIN-001 - Main application must use AI-integrated workflow engine
var _ = Describe("Dynamic Toolset Server AI Integration - Business Requirements", func() {
	var (
		logger *logrus.Logger
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce test noise
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	})

	AfterEach(func() {
		cancel()
	})

	Describe("BR-AI-MAIN-001: AI Service Integrator initialization", func() {
		It("should create a real AI Service Integrator instead of nil", func() {
			// Arrange: Create AI config for testing
			aiConfig := &config.Config{
				SLM: config.LLMConfig{
					Endpoint: "http://192.168.1.169:8080",
					Model:    "test-model",
				},
				AIServices: config.AIServicesConfig{
					HolmesGPT: config.HolmesGPTConfig{
						Endpoint: "http://test-holmesgpt:8080",
						Enabled:  true,
					},
				},
				VectorDB: config.VectorDBConfig{
					Enabled: true,
					Backend: "memory", // Use memory backend for testing
				},
			}

			// Act: Create AI Service Integrator (simulating main.go logic)
			aiIntegrator, err := createAIServiceIntegrator(ctx, aiConfig, logger)

			// Assert: Should create real AI integrator, not nil
			Expect(err).ToNot(HaveOccurred(), "Failed to create AI Service Integrator")
			Expect(aiIntegrator).ToNot(BeNil(), "AI Service Integrator should not be nil")
			Expect(aiIntegrator).To(BeAssignableToTypeOf(&engine.AIServiceIntegrator{}),
				"Should return actual AIServiceIntegrator implementation")
		})

		It("should handle missing AI configuration gracefully", func() {
			// Act: Create AI integrator with nil config (graceful degradation)
			aiIntegrator, err := createAIServiceIntegrator(ctx, nil, logger)

			// Assert: Should still create integrator with fallback behavior
			Expect(err).ToNot(HaveOccurred(), "Should handle missing config gracefully")
			Expect(aiIntegrator).ToNot(BeNil(), "Should create fallback AI integrator")
		})

		It("should enable vector database integration when configured", func() {
			// Arrange: Config with vector DB enabled
			aiConfig := &config.Config{
				VectorDB: config.VectorDBConfig{
					Enabled: true,
					Backend: "memory",
				},
			}

			// Act: Create AI integrator
			aiIntegrator, err := createAIServiceIntegrator(ctx, aiConfig, logger)

			// Assert: Vector DB should be integrated
			Expect(err).ToNot(HaveOccurred())
			Expect(aiIntegrator).ToNot(BeNil())

			// Business requirement: AI integrator should have access to vector DB capabilities
			status, err := aiIntegrator.GetServiceStatus(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(status.VectorDBEnabled).To(BeTrue(), "Vector database should be enabled")
		})
	})

	Describe("BR-AI-MAIN-002: Production workflow integration", func() {
		It("should use AI-integrated workflow engine in production contexts", func() {
			// This test validates the integration pattern that should be used in production
			// Arrange: Production-like configuration
			aiConfig := &config.Config{
				SLM: config.LLMConfig{
					Endpoint: "http://192.168.1.169:8080",
					Model:    "granite3.1-dense:8b",
				},
				VectorDB: config.VectorDBConfig{
					Enabled: true,
					Backend: "memory",
				},
			}

			// Act: Create AI integrator
			aiIntegrator, err := createAIServiceIntegrator(ctx, aiConfig, logger)
			Expect(err).ToNot(HaveOccurred())
			Expect(aiIntegrator).ToNot(BeNil())

			// Business requirement: Should be able to create workflow engines with AI integration
			// This simulates what the main application should do
			status, err := aiIntegrator.GetServiceStatus(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(status).ToNot(BeNil(), "Should provide service status for monitoring")
		})
	})
})

func TestDynamicToolsetServerIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dynamic Toolset Server AI Integration Suite")
}
