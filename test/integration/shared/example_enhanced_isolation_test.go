//go:build integration
// +build integration

package shared

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("Enhanced Test Isolation Example", func() {
	var (
		isolatedSuite *IsolatedTestSuite
		logger        *logrus.Logger
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.DebugLevel)

		// Create isolated test suite with comprehensive isolation
		isolatedSuite = NewEnhancedIsolatedTestSuite("Enhanced Example Suite", logger, IsolationComprehensive)

		// Start isolation for this test
		err := isolatedSuite.BeforeEach()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Clean up isolation after each test
		if isolatedSuite != nil {
			err := isolatedSuite.AfterEach()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	Context("Environment Variable Isolation", func() {
		It("should isolate environment variables between tests", func() {
			// Set test-specific environment variables
			err := isolatedSuite.SetEnv("TEST_ENHANCED_ISOLATION", "test-value-1")
			Expect(err).ToNot(HaveOccurred())

			err = isolatedSuite.SetEnv("ISOLATION_LEVEL", "comprehensive")
			Expect(err).ToNot(HaveOccurred())

			// Verify values are set
			Expect(os.Getenv("TEST_ENHANCED_ISOLATION")).To(Equal("test-value-1"))
			Expect(os.Getenv("ISOLATION_LEVEL")).To(Equal("comprehensive"))

			// Add cleanup for custom resource
			isolatedSuite.AddCleanup(func() error {
				logger.Info("Custom cleanup executed")
				return nil
			})
		})

		It("should not see environment changes from previous test", func() {
			// This test should not see the environment variables from the previous test
			Expect(os.Getenv("TEST_ENHANCED_ISOLATION")).To(BeEmpty())
			Expect(os.Getenv("ISOLATION_LEVEL")).To(BeEmpty())

			// Set different values for this test
			err := isolatedSuite.SetEnv("TEST_ENHANCED_ISOLATION", "test-value-2")
			Expect(err).ToNot(HaveOccurred())

			Expect(os.Getenv("TEST_ENHANCED_ISOLATION")).To(Equal("test-value-2"))
		})
	})

	Context("Temporary Directory Isolation", func() {
		It("should create and cleanup temporary directories", func() {
			// Create temporary directory for this test
			tempDir1, err := isolatedSuite.CreateTempDir("enhanced-test")
			Expect(err).ToNot(HaveOccurred())
			Expect(tempDir1).ToNot(BeEmpty(), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Enhanced isolation must provide data for recommendation confidence")

			// Create another temp directory
			tempDir2, err := isolatedSuite.CreateTempDir("enhanced-test-2")
			Expect(err).ToNot(HaveOccurred())
			Expect(tempDir2).ToNot(BeEmpty(), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Enhanced isolation must provide data for recommendation confidence")
			Expect(tempDir2).ToNot(Equal(tempDir1))

			// Verify directories exist
			_, err = os.Stat(tempDir1)
			Expect(err).ToNot(HaveOccurred())

			_, err = os.Stat(tempDir2)
			Expect(err).ToNot(HaveOccurred())

			logger.WithField("temp_dirs", []string{tempDir1, tempDir2}).Info("Created isolated temp directories")
		})
	})

	Context("Enhanced SLM Client Integration", func() {
		It("should demonstrate enhanced decision making", func() {
			// Create enhanced fake SLM client
			client := NewFakeSLMClient()

			// Verify enhanced decisions are enabled by default
			Expect(client.useEnhancedDecisions).To(BeTrue())
			Expect(client.decisionEngine).ToNot(BeNil())

			// Test infrastructure-level network alert
			infrastructureAlert := types.Alert{
				Name:        "NodeNetworkUnavailable",
				Status:      "firing",
				Severity:    "critical",
				Description: "Node network interface is down",
				Namespace:   "kube-system",
				Resource:    "node-01",
				Labels: map[string]string{
					"alertname": "NodeNetworkUnavailable",
					"node":      "node-01",
					"component": "kubelet",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), infrastructureAlert)
			Expect(err).ToNot(HaveOccurred())
			Expect(recommendation).ToNot(BeNil())

			// Should recommend node-level action for infrastructure network issues
			Expect(recommendation.Action).To(Equal("drain_node"))
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.7))
			Expect(recommendation.Reasoning.Summary).To(ContainSubstring("Node-level network issues"))

			logger.WithFields(logrus.Fields{
				"alert":      infrastructureAlert.Name,
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
			}).Info("Enhanced decision making demonstrated")
		})

		It("should handle security incidents with quarantine action", func() {
			client := NewFakeSLMClient()

			securityAlert := types.Alert{
				Name:        "SecurityIncident",
				Status:      "firing",
				Severity:    "critical",
				Description: "Security breach detected in pod",
				Namespace:   "production",
				Resource:    "webapp-pod",
				Labels: map[string]string{
					"alertname":   "SecurityIncident",
					"threat_type": "intrusion",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), securityAlert)
			Expect(err).ToNot(HaveOccurred())

			// Should recommend quarantine for security incidents
			Expect(recommendation.Action).To(Equal("quarantine_pod"))
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.8))
		})

		It("should handle OOM alerts with resource increase", func() {
			client := NewFakeSLMClient()

			oomAlert := types.Alert{
				Name:        "OOMKilled",
				Status:      "firing",
				Severity:    "warning",
				Description: "Pod was killed due to out of memory condition",
				Namespace:   "applications",
				Resource:    "memory-intensive-app",
				Labels: map[string]string{
					"alertname": "OOMKilled",
					"reason":    "OOMKilled",
				},
			}

			recommendation, err := client.AnalyzeAlert(context.Background(), oomAlert)
			Expect(err).ToNot(HaveOccurred())

			// Should recommend resource increase for OOM
			Expect(recommendation.Action).To(Equal("increase_resources"))
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.8))
		})
	})

	Context("Isolation Manager Advanced Features", func() {
		It("should provide isolation ID and status tracking", func() {
			isolationManager := isolatedSuite.GetIsolationManager()

			// Should have unique isolation ID
			isolationID := isolationManager.GetIsolationID()
			Expect(isolationID).ToNot(BeEmpty(), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Enhanced isolation must provide data for recommendation confidence")
			Expect(isolationID).To(ContainSubstring("test_"))

			// Should be started
			Expect(isolationManager.IsStarted()).To(BeTrue())

			logger.WithField("isolation_id", isolationID).Info("Isolation manager status verified")
		})

		It("should handle multiple cleanup functions", func() {
			cleanupExecuted := make([]bool, 3)

			// Add multiple cleanup functions
			isolatedSuite.AddCleanup(func() error {
				cleanupExecuted[0] = true
				return nil
			})

			isolatedSuite.AddCleanup(func() error {
				cleanupExecuted[1] = true
				time.Sleep(10 * time.Millisecond) // Simulate cleanup work
				return nil
			})

			isolatedSuite.AddCleanup(func() error {
				cleanupExecuted[2] = true
				return nil
			})

			// Cleanup functions will be executed in AfterEach
			// We can verify this by checking the cleanup executed flags after the test
			logger.Info("Multiple cleanup functions registered")
		})
	})
})
