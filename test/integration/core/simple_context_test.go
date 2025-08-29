package integration

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/slm"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/jordigilh/prometheus-alerts-slm/test/integration/shared"
)

var _ = Describe("Context Size Performance Test", Ordered, func() {
	var (
		logger     *logrus.Logger
		testConfig shared.IntegrationConfig
	)

	BeforeAll(func() {
		testConfig = shared.LoadConfig()
		if testConfig.SkipIntegration {
			Skip("Integration tests disabled")
		}

		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel) // Reduce logging noise
	})

	Context("Context Size Impact Analysis", func() {
		createTestAlert := func() types.Alert {
			return types.Alert{
				Name:        "HighMemoryUsage",
				Status:      "firing",
				Severity:    "warning",
				Description: "Memory usage above 80% for deployment webapp",
				Namespace:   "production",
				Resource:    "webapp",
				Labels: map[string]string{
					"alertname":  "HighMemoryUsage",
					"deployment": "webapp",
					"namespace":  "production",
					"severity":   "warning",
				},
				Annotations: map[string]string{
					"description": "Memory usage above 80% for deployment webapp",
					"summary":     "High memory usage detected",
				},
			}
		}

		testWithContextSize := func(contextSize int) (time.Duration, string) {
			// Create SLM config with specific context size
			slmConfig := config.SLMConfig{
				Endpoint:       testConfig.OllamaEndpoint,
				Model:          testConfig.OllamaModel,
				Provider:       "localai",
				Timeout:        30 * time.Second,
				RetryCount:     1,
				Temperature:    0.3,
				MaxTokens:      500,
				MaxContextSize: contextSize,
			}

			// Create SLM client without MCP for simplicity
			slmClient, err := slm.NewClient(slmConfig, logger)
			Expect(err).ToNot(HaveOccurred())

			alert := createTestAlert()

			startTime := time.Now()
			recommendation, err := slmClient.AnalyzeAlert(context.Background(), alert)
			responseTime := time.Since(startTime)

			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())
			Expect(recommendation.Action).ToNot(BeEmpty())
			Expect(recommendation.Confidence).To(BeNumerically(">", 0))

			return responseTime, recommendation.Action
		}

		It("should test unlimited context size (baseline)", func() {
			responseTime, action := testWithContextSize(0) // Unlimited

			logger.WithFields(logrus.Fields{
				"context_size":  "unlimited",
				"response_time": responseTime,
				"action":        action,
			}).Info("Baseline performance (unlimited context)")

			Expect(responseTime).To(BeNumerically("<", 15*time.Second))
			Expect(types.IsValidAction(action)).To(BeTrue())
		})

		It("should test 8K context size", func() {
			responseTime, action := testWithContextSize(8000)

			logger.WithFields(logrus.Fields{
				"context_size":  8000,
				"response_time": responseTime,
				"action":        action,
			}).Info("8K context performance")

			Expect(responseTime).To(BeNumerically("<", 15*time.Second))
			Expect(types.IsValidAction(action)).To(BeTrue())
		})

		It("should test 4K context size", func() {
			responseTime, action := testWithContextSize(4000)

			logger.WithFields(logrus.Fields{
				"context_size":  4000,
				"response_time": responseTime,
				"action":        action,
			}).Info("4K context performance")

			Expect(responseTime).To(BeNumerically("<", 15*time.Second))
			Expect(types.IsValidAction(action)).To(BeTrue())
		})

		It("should compare performance across context sizes", func() {
			contextSizes := []int{0, 8000, 4000}
			results := make(map[int]time.Duration)

			for _, size := range contextSizes {
				responseTime, action := testWithContextSize(size)
				results[size] = responseTime

				sizeLabel := fmt.Sprintf("%d", size)
				if size == 0 {
					sizeLabel = "unlimited"
				}

				logger.WithFields(logrus.Fields{
					"context_size":  sizeLabel,
					"response_time": responseTime,
					"action":        action,
				}).Info("Context size performance comparison")
			}

			// Analyze results
			baselineTime := results[0]
			time8K := results[8000]
			time4K := results[4000]

			logger.WithFields(logrus.Fields{
				"baseline_unlimited": baselineTime,
				"8k_context":         time8K,
				"4k_context":         time4K,
				"8k_speedup_ratio":   float64(baselineTime) / float64(time8K),
				"4k_speedup_ratio":   float64(baselineTime) / float64(time4K),
			}).Info("Performance comparison summary")

			// All should complete within reasonable time
			for _, responseTime := range results {
				Expect(responseTime).To(BeNumerically("<", 20*time.Second))
			}
		})
	})
})
