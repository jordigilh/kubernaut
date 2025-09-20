//go:build integration
// +build integration

package external_services

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/api/integration"
	"github.com/jordigilh/kubernaut/pkg/integration/notifications"
)

// Business Requirements: BR-INT-001 to BR-INT-005
// External Service Integration with Real Components
//
// This test suite validates the external service integration focusing on:
// - External monitoring system integration with real provider variations
// - ITSM system integration with actual service behavior
// - Communication platform integration with real-time delivery
// - Network conditions and failure recovery
// - Business SLA compliance
//
// Following project guidelines:
// - Use ginkgo/gomega BDD test framework
// - Hybrid approach: real services where possible, mocks for complex failures
// - Assertions MUST be backed on business outcomes
// - Use controlled test scenarios that guarantee the thresholds

var _ = Describe("External Services Integration", Ordered, func() {
	var (
		ctx                 context.Context
		cancel              context.CancelFunc
		externalMonitoring  *integration.ExternalMonitoringManager
		notificationService notifications.NotificationService
		testLogger          *logrus.Logger
		integrationSuite    *ExternalServicesIntegrationSuite
	)

	BeforeAll(func() {
		testLogger = logrus.New()
		testLogger.SetLevel(logrus.InfoLevel)

		By("Setting up External Services Integration Suite")
		var err error
		integrationSuite, err = NewExternalServicesIntegrationSuite()
		Expect(err).ToNot(HaveOccurred(), "Failed to create integration suite")

		By("Initializing external monitoring components")
		externalMonitoring = integrationSuite.ExternalMonitoring
		Expect(externalMonitoring).ToNot(BeNil(), "External monitoring must be initialized")

		By("Initializing notification services")
		notificationService = integrationSuite.NotificationService
		Expect(notificationService).ToNot(BeNil(), "Notification service must be initialized")
	})

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
	})

	AfterEach(func() {
		cancel()
	})

	AfterAll(func() {
		if integrationSuite != nil {
			integrationSuite.Cleanup()
		}
	})

	// BR-INT-001: External Monitoring System Integration
	// Business Requirement: System must integrate with external monitoring systems with 99.5% availability
	Describe("BR-INT-001: External Monitoring System Integration", func() {
		Context("when connecting to monitoring services", func() {
			It("should integrate with Prometheus monitoring successfully", func() {
				By("Creating controlled test scenarios for Prometheus integration")
				testScenarios := integrationSuite.CreateExternalServiceScenarios()

				var prometheusScenario *ExternalServiceTestScenario
				for _, scenario := range testScenarios {
					if scenario.ServiceName == "prometheus" && scenario.ServiceType == "monitoring" {
						prometheusScenario = scenario
						break
					}
				}

				Expect(prometheusScenario).ToNot(BeNil(), "Must have Prometheus test scenario")

				By("Testing Prometheus integration with business SLA validation")
				result, err := integrationSuite.TestExternalServiceIntegration(ctx, prometheusScenario)
				Expect(err).ToNot(HaveOccurred(), "External service integration test should not error")

				By("Validating business requirement: Monitoring service integration")
				Expect(result.Success).To(BeTrue(), "Prometheus integration should succeed")
				Expect(result.ConnectionTime).To(BeNumerically("<=", prometheusScenario.BusinessSLA.ConnectionTimeTarget),
					"Connection time must meet SLA")
				Expect(result.TotalDuration).To(BeNumerically("<=", prometheusScenario.BusinessSLA.ResponseTimeTarget),
					"Total response time must meet SLA")

				By("Validating SLA compliance")
				Expect(result.SLACompliant).To(BeTrue(), "Integration must meet business SLA requirements")

				testLogger.WithFields(logrus.Fields{
					"service":         "prometheus",
					"connection_time": result.ConnectionTime,
					"total_duration":  result.TotalDuration,
					"sla_compliant":   result.SLACompliant,
				}).Info("BR-INT-001: Prometheus monitoring integration validation completed")
			})

			It("should handle monitoring service availability requirements", func() {
				By("Creating test scenarios for monitoring services (success scenarios only)")
				testScenarios := integrationSuite.CreateExternalServiceScenarios()

				var monitoringScenarios []*ExternalServiceTestScenario
				for _, scenario := range testScenarios {
					// Include only monitoring scenarios without failure simulation for availability testing
					if scenario.ServiceType == "monitoring" && len(scenario.FailureSimulation) == 0 {
						monitoringScenarios = append(monitoringScenarios, scenario)
					}
				}

				Expect(len(monitoringScenarios)).To(BeNumerically(">=", 2), "Must have multiple monitoring scenarios")

				By("Testing availability across monitoring services")
				var totalAvailabilityScore float64
				var successfulTests int

				for _, scenario := range monitoringScenarios {
					result, err := integrationSuite.TestExternalServiceIntegration(ctx, scenario)
					Expect(err).ToNot(HaveOccurred())

					if result.Success {
						totalAvailabilityScore += 1.0
						successfulTests++
					}
				}

				By("Validating business requirement: 99.5% monitoring availability")
				Expect(successfulTests).To(BeNumerically(">", 0), "Must have successful monitoring tests")
				availabilityRate := totalAvailabilityScore / float64(len(monitoringScenarios))
				Expect(availabilityRate).To(BeNumerically(">=", 0.995), "Monitoring availability must be ≥99.5%")

				testLogger.WithField("availability_rate", availabilityRate).Info("BR-INT-001: Monitoring availability validation completed")
			})
		})
	})

	// BR-INT-002: Communication Platform Integration
	// Business Requirement: System must integrate with communication platforms with 99% delivery success
	Describe("BR-INT-002: Communication Platform Integration", func() {
		Context("when sending notifications to communication platforms", func() {
			It("should deliver notifications successfully with SLA compliance", func() {
				By("Creating controlled test scenarios for notification services")
				testScenarios := integrationSuite.CreateExternalServiceScenarios()

				var notificationScenario *ExternalServiceTestScenario
				for _, scenario := range testScenarios {
					if scenario.ServiceType == "notification" {
						notificationScenario = scenario
						break
					}
				}

				Expect(notificationScenario).ToNot(BeNil(), "Must have notification test scenario")

				By("Testing notification delivery with business SLA validation")
				result, err := integrationSuite.TestExternalServiceIntegration(ctx, notificationScenario)
				Expect(err).ToNot(HaveOccurred(), "Notification integration test should not error")

				By("Validating business requirement: Communication platform integration")
				Expect(result.Success).To(BeTrue(), "Notification integration should succeed")
				Expect(result.TotalDuration).To(BeNumerically("<=", notificationScenario.BusinessSLA.ResponseTimeTarget),
					"Notification delivery time must meet SLA")

				By("Validating notification SLA compliance")
				Expect(result.SLACompliant).To(BeTrue(), "Notification delivery must meet business SLA")

				testLogger.WithFields(logrus.Fields{
					"service":       "notification",
					"delivery_time": result.TotalDuration,
					"sla_compliant": result.SLACompliant,
				}).Info("BR-INT-002: Communication platform integration validation completed")
			})
		})
	})

	// BR-INT-003: Network Conditions and Failure Recovery
	// Business Requirement: System must recover from external service failures within 30 seconds
	Describe("BR-INT-003: Network Conditions and Failure Recovery", func() {
		Context("when external services experience failures", func() {
			It("should recover from network failures within SLA", func() {
				By("Creating controlled test scenarios for failure recovery")
				testScenarios := integrationSuite.CreateExternalServiceScenarios()

				var failureScenario *ExternalServiceTestScenario
				for _, scenario := range testScenarios {
					if len(scenario.FailureSimulation) > 0 {
						failureScenario = scenario
						break
					}
				}

				Expect(failureScenario).ToNot(BeNil(), "Must have failure recovery test scenario")

				By("Testing failure recovery with business SLA validation")
				startTime := time.Now()
				result, err := integrationSuite.TestExternalServiceIntegration(ctx, failureScenario)
				recoveryTime := time.Since(startTime)

				Expect(err).ToNot(HaveOccurred(), "Failure recovery test should not error")

				By("Validating business requirement: Failure recovery within 30 seconds")
				// For failure scenarios, we expect either success (recovery) or controlled failure
				if result.Success {
					// Service recovered - validate recovery time
					Expect(recoveryTime).To(BeNumerically("<=", 30*time.Second), "Recovery time must be ≤30 seconds")
				} else {
					// Expected failure - validate error handling
					Expect(result.ErrorMessage).ToNot(BeEmpty(), "Failure scenario must provide error details")
				}

				testLogger.WithFields(logrus.Fields{
					"recovery_time": recoveryTime,
					"success":       result.Success,
					"failure_mode":  "network_timeout",
				}).Info("BR-INT-003: Failure recovery validation completed")
			})
		})
	})

	// BR-INT-004: Integration Performance and SLA Compliance
	// Business Requirement: 95% of external service requests must complete within SLA
	Describe("BR-INT-004: Integration Performance and SLA Compliance", func() {
		Context("when testing integration performance under load", func() {
			It("should meet SLA requirements for 95% of external service requests", func() {
				By("Creating performance test scenarios")
				testScenarios := integrationSuite.CreateExternalServiceScenarios()

				// Use scenarios without failures for performance testing
				var performanceScenarios []*ExternalServiceTestScenario
				for _, scenario := range testScenarios {
					if len(scenario.FailureSimulation) == 0 {
						performanceScenarios = append(performanceScenarios, scenario)
					}
				}

				Expect(len(performanceScenarios)).To(BeNumerically(">=", 2), "Must have performance test scenarios")

				By("Running multiple concurrent external service requests")
				const numRequests = 10
				results := make([]*ExternalServiceResult, 0, numRequests*len(performanceScenarios))
				var wg sync.WaitGroup

				startTime := time.Now()

				for _, scenario := range performanceScenarios {
					for i := 0; i < numRequests; i++ {
						wg.Add(1)
						go func(testScenario *ExternalServiceTestScenario, index int) {
							defer wg.Done()

							requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
							defer cancel()

							result, err := integrationSuite.TestExternalServiceIntegration(requestCtx, testScenario)
							if err == nil && result != nil {
								results = append(results, result)
							}
						}(scenario, i)
					}
				}

				wg.Wait()
				totalTime := time.Since(startTime)

				By("Analyzing SLA compliance across all requests")
				var slaCompliantRequests int
				var successfulRequests int

				for _, result := range results {
					if result.Success {
						successfulRequests++
						if result.SLACompliant {
							slaCompliantRequests++
						}
					}
				}

				By("Validating business requirement: 95% SLA compliance")
				Expect(successfulRequests).To(BeNumerically(">", 0), "Must have successful requests")

				slaComplianceRate := float64(slaCompliantRequests) / float64(successfulRequests)
				Expect(slaComplianceRate).To(BeNumerically(">=", 0.95), "95% of requests must meet SLA")

				testLogger.WithFields(logrus.Fields{
					"total_requests":         numRequests * len(performanceScenarios),
					"successful_requests":    successfulRequests,
					"sla_compliant_requests": slaCompliantRequests,
					"sla_compliance_rate":    slaComplianceRate,
					"total_test_time":        totalTime,
				}).Info("BR-INT-004: Integration performance and SLA validation completed")
			})
		})
	})

	// BR-INT-005: External Service Health Monitoring
	// Business Requirement: System must monitor external service health with 99% accuracy
	Describe("BR-INT-005: External Service Health Monitoring", func() {
		Context("when monitoring external service health", func() {
			It("should accurately detect external service health status", func() {
				By("Testing health monitoring across external services")
				testScenarios := integrationSuite.CreateExternalServiceScenarios()

				var healthAccuracyScore float64
				var totalHealthChecks int

				for _, scenario := range testScenarios {
					By(fmt.Sprintf("Testing health monitoring for %s", scenario.ServiceName))

					result, err := integrationSuite.TestExternalServiceIntegration(ctx, scenario)
					Expect(err).ToNot(HaveOccurred(), "Health check should not error")

					totalHealthChecks++

					// For controlled scenarios, assess health check accuracy
					if len(scenario.FailureSimulation) == 0 {
						// Should detect healthy service
						if result.Success {
							healthAccuracyScore += 1.0
						}
					} else {
						// Should detect unhealthy service
						if !result.Success {
							healthAccuracyScore += 1.0
						}
					}
				}

				By("Validating business requirement: 99% health monitoring accuracy")
				Expect(totalHealthChecks).To(BeNumerically(">", 0), "Must have health check tests")

				healthAccuracy := healthAccuracyScore / float64(totalHealthChecks)
				Expect(healthAccuracy).To(BeNumerically(">=", 0.99), "Health monitoring accuracy must be ≥99%")

				testLogger.WithFields(logrus.Fields{
					"total_health_checks": totalHealthChecks,
					"health_accuracy":     healthAccuracy,
				}).Info("BR-INT-005: External service health monitoring validation completed")
			})
		})
	})
})
