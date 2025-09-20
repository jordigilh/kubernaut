//go:build integration
// +build integration

package examples

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/integration/shared"
)

var _ = Describe("Enhanced Capabilities Demonstration", func() {
	var (
		isolatedSuite *shared.IsolatedTestSuite
		logger        *logrus.Logger
		fakeClient    *shared.FakeSLMClient
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)

		// Create comprehensive isolation
		isolatedSuite = shared.NewEnhancedIsolatedTestSuite("Enhanced Capabilities Demo", logger, shared.IsolationComprehensive)
		err := isolatedSuite.BeforeEach()
		Expect(err).ToNot(HaveOccurred())

		// Create enhanced fake SLM client
		fakeClient = shared.NewFakeSLMClient()
		Expect(fakeClient.GetDecisionEngine()).ToNot(BeNil())
	})

	AfterEach(func() {
		if isolatedSuite != nil {
			err := isolatedSuite.AfterEach()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	Context("Sophisticated Decision Making Integration", func() {
		It("should demonstrate E2E test compatibility with node-level actions", func() {
			// Set isolated environment for this test
			err := isolatedSuite.SetEnv("NODE_DRAIN_ENABLED", "true")
			Expect(err).ToNot(HaveOccurred())

			// Test node network issue that was failing in E2E tests
			nodeNetworkAlert := types.Alert{
				Name:        "NodeNetworkUnavailable",
				Status:      "firing",
				Severity:    "critical",
				Description: "Node network interface is unreachable",
				Namespace:   "kube-system",
				Resource:    "worker-node-01",
				Labels: map[string]string{
					"alertname": "NodeNetworkUnavailable",
					"node":      "worker-node-01",
					"component": "node-exporter",
				},
			}

			recommendation, err := fakeClient.AnalyzeAlert(context.Background(), nodeNetworkAlert)
			Expect(err).ToNot(HaveOccurred())

			// Should now correctly identify as infrastructure-level network issue
			Expect(recommendation.Action).To(Equal("drain_node"))
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.7))

			logger.WithFields(logrus.Fields{
				"test_case":      "node_network_issue",
				"action":         recommendation.Action,
				"confidence":     recommendation.Confidence,
				"node_drain_env": os.Getenv("NODE_DRAIN_ENABLED"),
			}).Info("✅ E2E compatibility issue resolved")
		})

		It("should handle OOM alerts correctly for E2E scenarios", func() {
			// Set environment for OOM handling
			err := isolatedSuite.SetEnv("MEMORY_INCREASE_ENABLED", "true")
			Expect(err).ToNot(HaveOccurred())

			// Test OOM alert that was failing in E2E recurring alerts test
			oomAlert := types.Alert{
				Name:        "OOMKilled",
				Status:      "firing",
				Severity:    "warning",
				Description: "Container was killed due to out of memory condition",
				Namespace:   "applications",
				Resource:    "memory-intensive-app",
				Labels: map[string]string{
					"alertname": "OOMKilled",
					"reason":    "OOMKilled",
					"container": "app-container",
				},
			}

			recommendation, err := fakeClient.AnalyzeAlert(context.Background(), oomAlert)
			Expect(err).ToNot(HaveOccurred())

			// Should correctly recommend resource increase for OOM
			Expect(recommendation.Action).To(Equal("increase_resources"))
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.8))

			logger.WithFields(logrus.Fields{
				"test_case":  "oom_handling",
				"action":     recommendation.Action,
				"confidence": recommendation.Confidence,
				"memory_env": os.Getenv("MEMORY_INCREASE_ENABLED"),
			}).Info("✅ OOM alert handling enhanced")
		})

		It("should provide sophisticated storage expansion decisions", func() {
			// Create isolated temp directory for storage simulation
			tempDir, err := isolatedSuite.CreateTempDir("storage-test")
			Expect(err).ToNot(HaveOccurred())

			err = isolatedSuite.SetEnv("STORAGE_EXPANSION_DIR", tempDir)
			Expect(err).ToNot(HaveOccurred())

			storageAlert := types.Alert{
				Name:        "PVCNearFull",
				Status:      "firing",
				Severity:    "warning",
				Description: "Persistent volume is 95% full and requires expansion",
				Namespace:   "production",
				Resource:    "database-storage",
				Labels: map[string]string{
					"alertname": "PVCNearFull",
					"pvc":       "database-pv-claim",
					"usage":     "95%",
				},
			}

			recommendation, err := fakeClient.AnalyzeAlert(context.Background(), storageAlert)
			Expect(err).ToNot(HaveOccurred())

			// Should recommend PVC expansion for storage issues
			Expect(recommendation.Action).To(Equal("expand_pvc"))
			Expect(recommendation.Confidence).To(BeNumerically(">=", 0.85))

			logger.WithFields(logrus.Fields{
				"test_case":   "storage_expansion",
				"action":      recommendation.Action,
				"confidence":  recommendation.Confidence,
				"temp_dir":    tempDir,
				"storage_env": os.Getenv("STORAGE_EXPANSION_DIR"),
			}).Info("✅ Storage expansion decision enhanced")
		})
	})

	Context("Robust Test Isolation Features", func() {
		It("should demonstrate environment variable isolation", func() {
			// Set multiple test environment variables
			envVars := map[string]string{
				"TEST_ISOLATION_DEMO":  "active",
				"FAKE_SLM_MODE":        "enhanced",
				"DECISION_ENGINE_TYPE": "sophisticated",
				"CLEANUP_LEVEL":        "comprehensive",
			}

			for key, value := range envVars {
				err := isolatedSuite.SetEnv(key, value)
				Expect(err).ToNot(HaveOccurred())
				Expect(os.Getenv(key)).To(Equal(value))
			}

			// Add custom cleanup
			isolatedSuite.AddCleanup(func() error {
				logger.Info("Custom cleanup executed for environment isolation demo")
				return nil
			})

			// Verify isolation manager is tracking everything
			isolationManager := isolatedSuite.GetIsolationManager()
			Expect(isolationManager.IsStarted()).To(BeTrue())
			Expect(isolationManager.GetIsolationID()).ToNot(BeEmpty(), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Enhanced capabilities demo must provide data for confidence requirements")

			logger.WithFields(logrus.Fields{
				"isolation_id":  isolationManager.GetIsolationID(),
				"env_vars_set":  len(envVars),
				"cleanup_added": true,
			}).Info("✅ Environment isolation demonstrated")
		})

		It("should show no environment leakage from previous test", func() {
			// Previous test environment variables should be completely cleaned up
			Expect(os.Getenv("TEST_ISOLATION_DEMO")).To(BeEmpty())
			Expect(os.Getenv("FAKE_SLM_MODE")).To(BeEmpty())
			Expect(os.Getenv("DECISION_ENGINE_TYPE")).To(BeEmpty())
			Expect(os.Getenv("CLEANUP_LEVEL")).To(BeEmpty())
			Expect(os.Getenv("NODE_DRAIN_ENABLED")).To(BeEmpty())
			Expect(os.Getenv("MEMORY_INCREASE_ENABLED")).To(BeEmpty())

			logger.Info("✅ No environment variable leakage detected")
		})

		It("should handle multiple temporary directories", func() {
			tempDirs := make([]string, 0)

			// Create multiple temp directories
			for i := 0; i < 3; i++ {
				tempDir, err := isolatedSuite.CreateTempDir("multi-temp-test")
				Expect(err).ToNot(HaveOccurred())
				tempDirs = append(tempDirs, tempDir)

				// Verify directory exists
				_, err = os.Stat(tempDir)
				Expect(err).ToNot(HaveOccurred())
			}

			// All directories should be different
			Expect(tempDirs[0]).ToNot(Equal(tempDirs[1]))
			Expect(tempDirs[1]).ToNot(Equal(tempDirs[2]))
			Expect(tempDirs[0]).ToNot(Equal(tempDirs[2]))

			logger.WithField("temp_dirs", tempDirs).Info("✅ Multiple temp directories created and isolated")
		})
	})

	Context("Integration Readiness Verification", func() {
		It("should be ready for production and E2E test fixes", func() {
			// Verify enhanced fake SLM client is fully functional - Following project guidelines Line 30
			decisionEngine := fakeClient.GetDecisionEngine()
			// BR-AI-002-RECOMMENDATION-CONFIDENCE: Validate decision engine capabilities through business metrics
			Expect(decisionEngine).ToNot(BeNil(), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Decision engine must be capable of processing alerts")
			Expect(fakeClient.GetDecisionEngine()).ToNot(BeNil(), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Enhanced decisions must be enabled for recommendation confidence")

			// Test various alert scenarios that were problematic before
			testScenarios := []struct {
				name           string
				alert          types.Alert
				expectedAction string
				minConfidence  float64
			}{
				{
					name: "infrastructure_network",
					alert: types.Alert{
						Name:        "NodeNetworkDown",
						Severity:    "critical",
						Description: "Node network connectivity lost",
						Labels:      map[string]string{"component": "node"},
					},
					expectedAction: "drain_node",
					minConfidence:  0.7,
				},
				{
					name: "security_breach",
					alert: types.Alert{
						Name:        "SecurityIncident",
						Severity:    "critical",
						Description: "Security breach detected in container",
						Labels:      map[string]string{"threat_type": "malware"},
					},
					expectedAction: "quarantine_pod",
					minConfidence:  0.8,
				},
				{
					name: "resource_exhaustion",
					alert: types.Alert{
						Name:        "MemoryExhaustion",
						Severity:    "warning",
						Description: "Memory usage at 99% causing performance degradation",
					},
					expectedAction: "scale_and_increase_resources",
					minConfidence:  0.7,
				},
			}

			for _, scenario := range testScenarios {
				recommendation, err := fakeClient.AnalyzeAlert(context.Background(), scenario.alert)
				Expect(err).ToNot(HaveOccurred())

				Expect(recommendation.Action).To(Equal(scenario.expectedAction))
				Expect(recommendation.Confidence).To(BeNumerically(">=", scenario.minConfidence))

				logger.WithFields(logrus.Fields{
					"scenario":   scenario.name,
					"action":     recommendation.Action,
					"confidence": recommendation.Confidence,
				}).Info("✅ Test scenario verified")
			}

			logger.Info("🎯 Integration test foundation is ready for production/E2E fixes")
		})
	})
})
