//go:build integration
// +build integration

package api_database

import (
	"context"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	contextapi "github.com/jordigilh/kubernaut/pkg/api/context"
	"github.com/jordigilh/kubernaut/pkg/api/server"
)

// Business Requirements: BR-API-DB-001 to BR-API-DB-015
// API + Database Integration with Real Components
//
// This test suite validates the API + Database integration focusing on:
// - Authentication with database lookups (performance impact)
// - Rate limiting with persistent state (database consistency)
// - Request processing with data validation (database constraints)
// - Response optimization with cached data (cache performance)
// - Error handling with database transactions (transaction behavior)
//
// Following project guidelines:
// - Use ginkgo/gomega BDD test framework
// - Real database + API integration testing (hybrid approach)
// - Assertions MUST be backed on business outcomes
// - Use controlled test scenarios that guarantee the thresholds

var _ = Describe("API + Database Integration", Ordered, func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		contextAPIServer  *server.ContextAPIServer
		contextController *contextapi.ContextController
		testLogger        *logrus.Logger
		integrationSuite  *APIDatabaseIntegrationSuite
	)

	BeforeAll(func() {
		testLogger = logrus.New()
		testLogger.SetLevel(logrus.InfoLevel)

		By("Setting up API + Database Integration Suite")
		var err error
		integrationSuite, err = NewAPIDatabaseIntegrationSuite()
		Expect(err).ToNot(HaveOccurred(), "Failed to create integration suite")

		By("Initializing Context API server components")
		contextAPIServer = integrationSuite.ContextAPIServer
		Expect(contextAPIServer).ToNot(BeNil(), "Context API server must be initialized")

		contextController = integrationSuite.ContextController
		Expect(contextController).ToNot(BeNil(), "Context controller must be initialized")

		By("Verifying database connectivity")
		Expect(integrationSuite.DatabaseConn).ToNot(BeNil(), "Database connection must be established")
		err = integrationSuite.DatabaseConn.Ping()
		Expect(err).ToNot(HaveOccurred(), "Database must be accessible")
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

	// BR-API-DB-001: Authentication Performance with Database Lookups
	// Business Requirement: Database lookups for authentication must not impact response times beyond 2s
	Describe("BR-API-DB-001: Authentication Performance with Database Lookups", func() {
		Context("when processing authenticated API requests", func() {
			It("should handle authentication database lookups within SLA", func() {
				By("Creating controlled test scenarios for authentication")
				testScenarios := integrationSuite.CreateAPITestScenarios()

				var authScenario *APITestScenario
				for _, scenario := range testScenarios {
					if scenario.RequestType == "authentication" {
						authScenario = scenario
						break
					}
				}

				Expect(authScenario).ToNot(BeNil(), "Must have authentication test scenario")

				By("Testing authentication with database integration")
				result, err := integrationSuite.TestAPIIntegration(ctx, authScenario)
				Expect(err).ToNot(HaveOccurred(), "Authentication integration test should not error")

				By("Validating business requirement: Authentication database performance")
				Expect(result.Success).To(BeTrue(), "Authentication should succeed")
				Expect(result.HTTPStatusCode).To(Equal(authScenario.ExpectedResponse), "Should return expected status code")
				Expect(result.ResponseTime).To(BeNumerically("<=", authScenario.PerformanceSLA.ResponseTimeTarget),
					"Authentication response time must meet SLA")

				By("Validating database operations execution")
				Expect(result.DatabaseOperationsExecuted).To(ContainElement("auth_lookup"),
					"Must execute database authentication lookup")
				Expect(result.CacheOperationsExecuted).To(ContainElement("auth_cache_check"),
					"Must check authentication cache")

				By("Validating SLA compliance")
				Expect(result.SLACompliant).To(BeTrue(), "Authentication must meet business SLA requirements")

				testLogger.WithFields(logrus.Fields{
					"response_time":    result.ResponseTime,
					"sla_compliant":    result.SLACompliant,
					"db_operations":    result.DatabaseOperationsExecuted,
					"cache_operations": result.CacheOperationsExecuted,
				}).Info("BR-API-DB-001: Authentication database integration validation completed")
			})
		})
	})

	// BR-API-DB-002: Rate Limiting with Persistent State
	// Business Requirement: Rate limiting must maintain state in database with 99% consistency
	Describe("BR-API-DB-002: Rate Limiting with Persistent Database State", func() {
		Context("when enforcing rate limits with database persistence", func() {
			It("should maintain rate limit state with database consistency", func() {
				By("Creating controlled test scenarios for rate limiting")
				testScenarios := integrationSuite.CreateAPITestScenarios()

				var rateLimitScenario *APITestScenario
				for _, scenario := range testScenarios {
					if scenario.RequestType == "rate_limiting" {
						rateLimitScenario = scenario
						break
					}
				}

				Expect(rateLimitScenario).ToNot(BeNil(), "Must have rate limiting test scenario")

				By("Testing rate limiting with database state persistence")
				result, err := integrationSuite.TestAPIIntegration(ctx, rateLimitScenario)
				Expect(err).ToNot(HaveOccurred(), "Rate limiting integration test should not error")

				By("Validating business requirement: Rate limiting database consistency")
				Expect(result.Success).To(BeTrue(), "Rate limiting should succeed")
				Expect(result.ResponseTime).To(BeNumerically("<=", rateLimitScenario.PerformanceSLA.ResponseTimeTarget),
					"Rate limiting response time must meet SLA")

				By("Validating database state operations")
				Expect(result.DatabaseOperationsExecuted).To(ContainElement("rate_limit_check"),
					"Must check rate limit state in database")
				Expect(result.DatabaseOperationsExecuted).To(ContainElement("rate_limit_update"),
					"Must update rate limit state in database")

				By("Validating cache performance for rate limiting")
				Expect(result.SLACompliant).To(BeTrue(), "Rate limiting must meet performance SLA")

				testLogger.WithFields(logrus.Fields{
					"response_time":       result.ResponseTime,
					"database_operations": result.DatabaseOperationsExecuted,
					"sla_compliant":       result.SLACompliant,
				}).Info("BR-API-DB-002: Rate limiting database state validation completed")
			})
		})
	})

	// BR-API-DB-003: Response Optimization with Cached Data
	// Business Requirement: Cached responses must achieve 85% hit rate and improve performance by 60%
	Describe("BR-API-DB-003: Response Optimization with Cached Data", func() {
		Context("when optimizing API responses with database caching", func() {
			It("should achieve caching performance targets with database integration", func() {
				By("Creating controlled test scenarios for response caching")
				testScenarios := integrationSuite.CreateAPITestScenarios()

				var cachingScenario *APITestScenario
				for _, scenario := range testScenarios {
					if scenario.RequestType == "caching" {
						cachingScenario = scenario
						break
					}
				}

				Expect(cachingScenario).ToNot(BeNil(), "Must have caching test scenario")

				By("Testing response caching with database optimization")
				result, err := integrationSuite.TestAPIIntegration(ctx, cachingScenario)
				Expect(err).ToNot(HaveOccurred(), "Caching integration test should not error")

				By("Validating business requirement: Response caching optimization")
				Expect(result.Success).To(BeTrue(), "Cached response should succeed")
				Expect(result.ResponseTime).To(BeNumerically("<=", cachingScenario.PerformanceSLA.ResponseTimeTarget),
					"Cached response time must meet aggressive SLA (500ms)")

				By("Validating cache operations execution")
				Expect(result.CacheOperationsExecuted).To(ContainElement("cache_get"),
					"Must attempt cache retrieval")
				Expect(result.CacheOperationsExecuted).To(ContainElement("cache_set"),
					"Must set cache for future requests")

				By("Validating database query optimization")
				Expect(result.DatabaseOperationsExecuted).To(ContainElement("context_query"),
					"Must execute optimized database query")

				By("Validating caching SLA compliance")
				Expect(result.SLACompliant).To(BeTrue(), "Caching must meet 85% hit rate and performance targets")

				testLogger.WithFields(logrus.Fields{
					"response_time":    result.ResponseTime,
					"cache_operations": result.CacheOperationsExecuted,
					"sla_compliant":    result.SLACompliant,
				}).Info("BR-API-DB-003: Response caching optimization validation completed")
			})
		})
	})

	// BR-API-DB-004: Database Transaction Error Handling
	// Business Requirement: API must handle database errors gracefully with <10% error rate during failures
	Describe("BR-API-DB-004: Database Transaction Error Handling", func() {
		Context("when database errors occur during API requests", func() {
			It("should handle database transaction errors gracefully", func() {
				By("Creating controlled test scenarios for error handling")
				testScenarios := integrationSuite.CreateAPITestScenarios()

				var errorHandlingScenario *APITestScenario
				for _, scenario := range testScenarios {
					if scenario.RequestType == "validation" && len(scenario.FailureSimulation) > 0 {
						errorHandlingScenario = scenario
						break
					}
				}

				Expect(errorHandlingScenario).ToNot(BeNil(), "Must have error handling test scenario")

				By("Testing database error handling with controlled failure")
				result, err := integrationSuite.TestAPIIntegration(ctx, errorHandlingScenario)
				Expect(err).ToNot(HaveOccurred(), "Error handling integration test should not error")

				By("Validating business requirement: Graceful error handling")
				// Should still return success (graceful degradation) or handle error appropriately
				Expect(result.HTTPStatusCode).To(BeNumerically(">=", 200), "Should handle errors gracefully")
				Expect(result.HTTPStatusCode).To(BeNumerically("<", 500), "Should avoid server errors")

				By("Validating error recovery time")
				Expect(result.ResponseTime).To(BeNumerically("<=", errorHandlingScenario.PerformanceSLA.ResponseTimeTarget),
					"Error recovery time must meet extended SLA")

				By("Validating database operations attempted")
				Expect(result.DatabaseOperationsExecuted).To(ContainElement("health_check"),
					"Must attempt database health check during error scenarios")

				testLogger.WithFields(logrus.Fields{
					"response_time":       result.ResponseTime,
					"http_status":         result.HTTPStatusCode,
					"database_operations": result.DatabaseOperationsExecuted,
					"error_handling":      "graceful_degradation",
				}).Info("BR-API-DB-004: Database error handling validation completed")
			})
		})
	})

	// BR-API-DB-005: API Performance Under Load with Database Integration
	// Business Requirement: 95% of API requests must complete within SLA under concurrent load
	Describe("BR-API-DB-005: API Performance Under Load with Database Integration", func() {
		Context("when testing API + Database performance under concurrent load", func() {
			It("should meet SLA requirements for 95% of concurrent requests", func() {
				By("Creating performance test scenarios")
				testScenarios := integrationSuite.CreateAPITestScenarios()

				// Use scenarios without failures for performance testing
				var performanceScenarios []*APITestScenario
				for _, scenario := range testScenarios {
					if len(scenario.FailureSimulation) == 0 {
						performanceScenarios = append(performanceScenarios, scenario)
					}
				}

				Expect(len(performanceScenarios)).To(BeNumerically(">=", 3), "Must have performance test scenarios")

				By("Running multiple concurrent API + Database requests")
				const numRequests = 20
				results := make([]*APIIntegrationResult, 0, numRequests*len(performanceScenarios))
				var wg sync.WaitGroup

				startTime := time.Now()

				for _, scenario := range performanceScenarios {
					for i := 0; i < numRequests; i++ {
						wg.Add(1)
						go func(testScenario *APITestScenario, index int) {
							defer wg.Done()

							requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
							defer cancel()

							result, err := integrationSuite.TestAPIIntegration(requestCtx, testScenario)
							if err == nil && result != nil {
								results = append(results, result)
							}
						}(scenario, i)
					}
				}

				wg.Wait()
				totalTime := time.Since(startTime)

				By("Analyzing SLA compliance across all concurrent requests")
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

				By("Validating business requirement: 95% SLA compliance under load")
				Expect(successfulRequests).To(BeNumerically(">", 0), "Must have successful requests")

				slaComplianceRate := float64(slaCompliantRequests) / float64(successfulRequests)
				Expect(slaComplianceRate).To(BeNumerically(">=", 0.95), "95% of requests must meet SLA")

				By("Validating API + Database throughput")
				totalRequests := numRequests * len(performanceScenarios)
				throughput := float64(successfulRequests) / totalTime.Seconds()
				Expect(throughput).To(BeNumerically(">=", 50), "Must maintain minimum throughput of 50 req/sec")

				testLogger.WithFields(logrus.Fields{
					"total_requests":         totalRequests,
					"successful_requests":    successfulRequests,
					"sla_compliant_requests": slaCompliantRequests,
					"sla_compliance_rate":    slaComplianceRate,
					"throughput_req_per_sec": throughput,
					"total_test_time":        totalTime,
				}).Info("BR-API-DB-005: API + Database performance under load validation completed")
			})
		})
	})
})
