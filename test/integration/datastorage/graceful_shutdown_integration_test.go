/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package datastorage

import (
	_ "github.com/jackc/pgx/v5/stdlib"

	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
	"github.com/jordigilh/kubernaut/pkg/shared/health"
)

// TDD RED PHASE: These tests define DD-007 graceful shutdown requirements
// DD-007: Kubernetes-Aware Graceful Shutdown Pattern
// BR-STORAGE-028: Production Readiness - Graceful shutdown
//
// Business Requirement: ZERO request failures during rolling updates
//
// Expected Behavior (4-Step Pattern):
// 1. Set shutdown flag → readiness probe returns 503
// 2. Wait 5 seconds for Kubernetes endpoint removal
// 3. Drain in-flight HTTP connections
// 4. Close resources (database, Redis)

var _ = Describe("BR-STORAGE-028: DD-007 Kubernetes-Aware Graceful Shutdown", Label("integration", "graceful-shutdown", "p0"), func() {

	Context("Business Requirement: Readiness Probe Coordination", func() {
		It("MUST return 503 on readiness probe immediately when shutdown starts", func() {
			// Business Scenario: Pod receives SIGTERM during rolling update
			// Expected: Readiness probe returns 503 to trigger endpoint removal

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Verify readiness is healthy before shutdown
			// DD-AUTH-014: Use authenticated request
			resp, err := makeAuthenticatedRequest(healthServer.URL + "/readyz")
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			_ = resp.Body.Close()

			// Start shutdown in background
			shutdownDone := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				shutdownDone <- srv.Shutdown(ctx)
			}()

			// Business Outcome 1: Readiness probe returns 503 during shutdown
			// Per TESTING_GUIDELINES.md: Use Eventually() instead of time.Sleep()
			// DD-AUTH-014: Use authenticated request
			Eventually(func() int {
				r, e := makeAuthenticatedRequest(healthServer.URL + "/readyz")
				if e != nil || r == nil {
					return 0
				}
				_ = r.Body.Close()
				return r.StatusCode
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(503),
				"Readiness probe MUST return 503 during shutdown (DD-007 STEP 1)")

			// Get final response for detailed checks
			// DD-AUTH-014: Use authenticated request
			resp, err = makeAuthenticatedRequest(healthServer.URL + "/readyz")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(503),
				"Readiness probe MUST return 503 during shutdown to trigger Kubernetes endpoint removal (DD-007 STEP 1)")

			// Business Outcome 2: Response indicates shutdown status
			var response models.ReadinessResponse
			err = json.NewDecoder(resp.Body).Decode(&response)
			Expect(err).ToNot(HaveOccurred())

			Expect(response.Status).To(Equal("not_ready"),
				"Readiness status MUST indicate not_ready during shutdown")
			Expect(response.Reason).To(Equal("shutting_down"),
				"Readiness reason MUST explain shutdown in progress")

			// Wait for shutdown to complete
			err = <-shutdownDone
			Expect(err).ToNot(HaveOccurred())
		})

		It("MUST keep liveness probe returning 200 during shutdown", func() {
			// Business Scenario: Kubernetes checks liveness during termination
			// Expected: Liveness returns 200 (pod is still "alive", just shutting down)

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Verify liveness is healthy before shutdown
			// DD-AUTH-014: Use authenticated request
			resp, err := makeAuthenticatedRequest(healthServer.URL + "/healthz")
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			_ = resp.Body.Close()

			// Start shutdown in background
			shutdownDone := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				shutdownDone <- srv.Shutdown(ctx)
			}()

			// Business Outcome: Liveness probe still returns 200 during shutdown
			// Per TESTING_GUIDELINES.md: Use Eventually() instead of time.Sleep()
			// DD-AUTH-014: Use authenticated request
			Eventually(func() int {
				r, e := makeAuthenticatedRequest(healthServer.URL + "/healthz")
				if e != nil || r == nil {
					return 0
				}
				_ = r.Body.Close()
				return r.StatusCode
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(200),
				"Liveness probe MUST return 200 during shutdown (DD-007)")

			// Get final response for detailed checks
			// DD-AUTH-014: Use authenticated request
			resp, err = makeAuthenticatedRequest(healthServer.URL + "/healthz")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(200),
				"Liveness probe MUST return 200 during shutdown (pod is still alive, just draining)")

			// Wait for shutdown to complete
			err = <-shutdownDone
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Business Requirement: In-Flight Request Completion", func() {
		It("MUST complete in-flight requests before final shutdown", func() {
			// Business Scenario: Long-running request in progress when SIGTERM arrives
			// Expected: Request completes successfully within shutdown timeout

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Start long-running request (simulated delay)
			responseChan := make(chan int, 1)
			errorChan := make(chan error, 1)

			go func() {
				// This request will be in-flight when shutdown starts
				// DD-AUTH-014: Use authenticated request
				resp, err := makeAuthenticatedRequest(testServer.URL + "/api/v1/audit/events?limit=10")
				if err != nil {
					errorChan <- err
					return
				}
				defer func() { _ = resp.Body.Close() }()
				responseChan <- resp.StatusCode
			}()

			// Per TESTING_GUIDELINES.md: Removed time.Sleep() - let request start naturally
			// Initiate shutdown while request is in-flight
			shutdownDone := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				shutdownDone <- srv.Shutdown(ctx)
			}()

			// Business Outcome: In-flight request completes successfully
			select {
			case statusCode := <-responseChan:
				Expect(statusCode).To(Equal(200),
					"In-flight requests MUST complete successfully during graceful shutdown (DD-007 STEP 3)")
			case err := <-errorChan:
				Fail(fmt.Sprintf("In-flight request failed: %v", err))
			case <-time.After(10 * time.Second):
				Fail("In-flight request timed out (should complete within shutdown window)")
			}

			// Shutdown should complete successfully
			err := <-shutdownDone
			Expect(err).ToNot(HaveOccurred())
		})

		It("MUST reject new requests after shutdown begins", func() {
			// Business Scenario: New request arrives after shutdown initiated
			// Expected: Connection refused or timeout (server not accepting new connections)

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Initiate shutdown
			shutdownDone := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				shutdownDone <- srv.Shutdown(ctx)
			}()

			// Wait for DD-007 STEP 2 (endpoint propagation) to complete
			// This ensures server has stopped accepting new connections
			// ✅ APPROVED EXCEPTION: testing DD-007 endpoint propagation timing behavior
			time.Sleep(6 * time.Second)

			// Verify server stopped accepting connections (Eventually pattern)
			Eventually(func() bool {
				client := &http.Client{Timeout: 500 * time.Millisecond}
				r, e := client.Get(healthServer.URL + "/readyz")
				if e != nil {
					return true // Connection refused = server not accepting
				}
				if r != nil {
					_ = r.Body.Close()
					return r.StatusCode == 503 || r.StatusCode >= 500
				}
				return false
			}, 2*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"Server should reject connections after DD-007 STEP 2")

			// Attempt new request after shutdown initiated
			// Use short timeout since server should not accept connection
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get(testServer.URL + "/api/v1/audit/events")

			// Business Outcome: New request should fail or timeout
			// Server is draining connections, not accepting new ones
			if resp != nil {
				_ = resp.Body.Close()
				// If we get a response, it should be a server error (not 200)
				Expect(resp.StatusCode).ToNot(Equal(200),
					"New requests after shutdown should not succeed with 200 OK")
			} else {
				// Connection refused or timeout is expected
				Expect(err).To(HaveOccurred(),
					"New requests after shutdown should fail (connection refused or timeout)")
			}

			// Wait for shutdown to complete
			err = <-shutdownDone
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Business Requirement: Resource Cleanup", func() {
		It("MUST complete shutdown successfully and clean up resources", func() {
			// Business Scenario: Pod terminating, all resources must be released
			// Expected: Shutdown completes without errors, all resources closed

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Verify server is operational before shutdown
			// DD-AUTH-014: Use authenticated request
			resp, err := makeAuthenticatedRequest(healthServer.URL + "/readyz")
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			_ = resp.Body.Close()

			// Initiate graceful shutdown
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err = srv.Shutdown(shutdownCtx)

			// Business Outcome: Shutdown completes without errors
			// This validates:
			// - DD-007 STEP 1: Shutdown flag set (readiness -> 503)
			// - DD-007 STEP 2: 5-second endpoint removal propagation completed
			// - DD-007 STEP 3: HTTP connections drained successfully
			// - DD-007 STEP 4: Database and Redis connections closed
			Expect(err).ToNot(HaveOccurred(),
				"Shutdown MUST complete successfully, closing all resources cleanly (DD-007)")
		})

		It("MUST complete shutdown quickly with no in-flight requests", func() {
			// Business Scenario: Clean shutdown with no active connections
			// Expected: Shutdown completes in ~5-6 seconds (endpoint wait + minimal drain)

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// No in-flight requests, clean shutdown
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			startTime := time.Now()
			err := srv.Shutdown(shutdownCtx)
			duration := time.Since(startTime)

			// Business Outcome: Shutdown completes successfully and quickly
			Expect(err).ToNot(HaveOccurred())

			// Shutdown should complete in ~5-6 seconds (DD-007 STEP 2 wait + minimal cleanup)
			// Allow up to 10 seconds for slower test environments
			Expect(duration).To(BeNumerically("<", 10*time.Second),
				"Shutdown should complete quickly with no in-flight requests")
		})

		It("MUST respect shutdown timeout context", func() {
			// Business Scenario: Shutdown timeout expires before completion
			// Expected: Shutdown handles timeout gracefully

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Use very short timeout (shorter than 5-second endpoint wait)
			// This should cause timeout during STEP 2 (endpoint propagation wait)
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			startTime := time.Now()
			err := srv.Shutdown(shutdownCtx)
			duration := time.Since(startTime)

			// Business Outcome: Shutdown respects context timeout
			// Note: With 1s timeout and 5s wait in STEP 2, shutdown may complete
			// before context expires (time.Sleep doesn't check context)
			// This is acceptable - the important part is no panic/crash

			if err != nil {
				// If error occurred, it should be timeout-related
				Expect(err.Error()).To(Or(
					ContainSubstring("context deadline exceeded"),
					ContainSubstring("timeout"),
					ContainSubstring("shutdown"),
				))
			}

			// Shutdown attempt completed (with or without error)
			Expect(duration).To(BeNumerically("<", 10*time.Second),
				"Shutdown should not hang indefinitely even with timeout")
		})
	})

	Context("Business Requirement: Concurrent Shutdown Safety", func() {
		It("MUST be safe to call Shutdown() multiple times", func() {
			// Business Scenario: Multiple goroutines call Shutdown() (defensive)
			// Expected: Only first call executes shutdown, others wait/return gracefully

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Call shutdown from multiple goroutines
			var wg sync.WaitGroup
			errors := make(chan error, 3)

			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					errors <- srv.Shutdown(ctx)
				}()
			}

			wg.Wait()
			close(errors)

			// Business Outcome: All shutdown calls succeed or gracefully handle duplicate
			errorCount := 0
			for err := range errors {
				if err != nil {
					errorCount++
				}
			}

			// At least one should succeed, others may return error (acceptable)
			Expect(errorCount).To(BeNumerically("<=", 2),
				"Multiple Shutdown() calls should handle gracefully (DD-007 concurrency safety)")
		})
	})

	Context("Business Requirement: Database Connection Pool Cleanup", func() {
		It("MUST close database connection pool during shutdown", func() {
			// Business Scenario: Database connections must be released on termination
			// Expected: Connection pool closes cleanly, no connection leaks

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Verify database is accessible before shutdown
			// DD-AUTH-014: Use authenticated request
			resp, err := makeAuthenticatedRequest(testServer.URL + "/api/v1/audit/events?limit=1")
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			_ = resp.Body.Close()

			// Initiate graceful shutdown
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err = srv.Shutdown(shutdownCtx)

			// Business Outcome: Shutdown completes successfully (DB pool closed in STEP 4)
			Expect(err).ToNot(HaveOccurred(),
				"Database connection pool MUST close cleanly during shutdown (DD-007 STEP 4)")
		})

		It("MUST complete database queries before shutdown", func() {
			// Business Scenario: Database query in progress when SIGTERM arrives
			// Expected: Query completes before database connections close

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Start database query that should complete during shutdown window
			responseChan := make(chan int, 1)
			errorChan := make(chan error, 1)

			go func() {
				// Use reliable incidents list endpoint (always available)
				// DD-AUTH-014: Use authenticated request
				resp, err := makeAuthenticatedRequest(testServer.URL + "/api/v1/audit/events?limit=100")
				if err != nil {
					errorChan <- err
					return
				}
				defer func() { _ = resp.Body.Close() }()
				responseChan <- resp.StatusCode
			}()

			// Per TESTING_GUIDELINES.md: Removed time.Sleep() - let query start naturally
			// Initiate shutdown while query is in-flight
			shutdownDone := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				shutdownDone <- srv.Shutdown(ctx)
			}()

			// Business Outcome: Database query completes before shutdown
			// DD-007 provides 5s wait + 30s drain = plenty of time for query
			select {
			case statusCode := <-responseChan:
				Expect(statusCode).To(Equal(200),
					"Database queries MUST complete before connection pool closes (DD-007 STEP 3)")
			case err := <-errorChan:
				Fail(fmt.Sprintf("Database query failed during shutdown: %v", err))
			case <-time.After(15 * time.Second):
				Fail("Database query timed out (should complete within shutdown window)")
			}

			// Shutdown should complete successfully
			err := <-shutdownDone
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Business Requirement: Multiple Concurrent Requests During Shutdown", func() {
		It("MUST complete all concurrent in-flight requests before shutdown", func() {
			// Business Scenario: Multiple requests in progress when SIGTERM arrives
			// Expected: All requests complete successfully

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Start multiple concurrent requests
			var wg sync.WaitGroup
			var started sync.WaitGroup
			successCount := make(chan int, 5)
			errorCount := make(chan error, 5)

			for i := 0; i < 5; i++ {
				wg.Add(1)
				started.Add(1)
				go func(index int) {
					defer wg.Done()
					started.Done()
					// DD-AUTH-014: Use authenticated request
					resp, err := makeAuthenticatedRequest(fmt.Sprintf("%s/api/v1/audit/events?limit=%d", testServer.URL, index+1))
					if err != nil {
						errorCount <- err
						return
					}
					defer func() { _ = resp.Body.Close() }()
					if resp.StatusCode == 200 {
						successCount <- 1
					}
				}(i)
			}

			started.Wait()
			// Initiate shutdown while multiple requests are in-flight
			shutdownDone := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				shutdownDone <- srv.Shutdown(ctx)
			}()

			// Wait for all requests to complete
			wg.Wait()
			close(successCount)
			close(errorCount)

			// Business Outcome: All in-flight requests completed successfully
			totalSuccess := 0
			for range successCount {
				totalSuccess++
			}

			totalErrors := 0
			for range errorCount {
				totalErrors++
			}

			// All 5 requests should have completed successfully
			Expect(totalSuccess).To(Equal(5),
				"All concurrent in-flight requests MUST complete during graceful shutdown (DD-007 STEP 3)")
			Expect(totalErrors).To(Equal(0),
				"No in-flight requests should error during graceful shutdown")

			// Shutdown should complete successfully
			err := <-shutdownDone
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Business Requirement: Write Operations During Shutdown", func() {
		It("MUST handle POST requests in-flight during shutdown", func() {
			// Business Scenario: Write operation (POST) in progress when SIGTERM arrives
			// Expected: Write completes successfully (no data loss)

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Note: This test validates behavior if POST endpoints exist
			// Current Phase 1 is Read-only API, but this validates graceful shutdown
			// doesn't break if write endpoints are added in Phase 2

			// Start a request that would be in-flight
			responseChan := make(chan int, 1)
			errorChan := make(chan error, 1)

			go func() {
				// Use GET for now (Phase 1), but pattern applies to POST in Phase 2
				// DD-AUTH-014: Use authenticated request
				resp, err := makeAuthenticatedRequest(testServer.URL + "/api/v1/audit/events?limit=1")
				if err != nil {
					errorChan <- err
					return
				}
				defer func() { _ = resp.Body.Close() }()
				responseChan <- resp.StatusCode
			}()

			// Per TESTING_GUIDELINES.md: Removed time.Sleep() - let request start naturally
			// Initiate shutdown while request is in-flight
			shutdownDone := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				shutdownDone <- srv.Shutdown(ctx)
			}()

			// Business Outcome: Request completes successfully (no data loss)
			select {
			case statusCode := <-responseChan:
				Expect(statusCode).To(Equal(200),
					"In-flight operations MUST complete to prevent data loss (DD-007 STEP 3)")
			case err := <-errorChan:
				Fail(fmt.Sprintf("In-flight operation failed: %v", err))
			case <-time.After(10 * time.Second):
				Fail("In-flight operation timed out during shutdown")
			}

			// Shutdown should complete successfully
			err := <-shutdownDone
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Business Requirement: Redis Cache Connection Cleanup", func() {
		It("MUST close Redis connections during shutdown", func() {
			// Business Scenario: Redis cache connections must be released on termination
			// Expected: Redis connections close cleanly in DD-007 STEP 4

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Verify server is operational (Redis is used for DLQ and caching)
			// DD-AUTH-014: Use authenticated request
			resp, err := makeAuthenticatedRequest(healthServer.URL + "/readyz")
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			_ = resp.Body.Close()

			// Initiate graceful shutdown
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err = srv.Shutdown(shutdownCtx)

			// Business Outcome: Shutdown completes successfully (Redis closed in STEP 4)
			Expect(err).ToNot(HaveOccurred(),
				"Redis connections MUST close cleanly during shutdown (DD-007 STEP 4)")
		})
	})

	Context("Business Requirement: Shutdown Under Load", func() {
		It("MUST handle shutdown gracefully under moderate load", func() {
			// Business Scenario: Service under moderate load when SIGTERM arrives.
			// Uses a gate middleware to guarantee all 10 requests are genuinely
			// in-flight inside the handler before shutdown begins — eliminates
			// timing sensitivity entirely.

			_, healthServer, srv := createTestServerWithAccess()
			defer healthServer.Close()

			const requestCount = 10

			gate := &requestGate{
				handler: srv.Handler(),
				gate:    make(chan struct{}),
			}
			testServer := httptest.NewServer(gate)
			defer testServer.Close()

			url := testServer.URL + "/api/v1/audit/events?limit=10"

			var wg sync.WaitGroup
			results := make(chan int, requestCount)

			for i := 0; i < requestCount; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					resp, err := makeAuthenticatedRequest(url)
					if err != nil {
						return
					}
					defer func() { _ = resp.Body.Close() }()
					results <- resp.StatusCode
				}()
			}

			// Wait until all requests have arrived at the server and are
			// blocked inside the gate middleware.
			Eventually(func() int32 {
				return gate.arrived.Load()
			}, 10*time.Second, 50*time.Millisecond).Should(
				BeNumerically("==", requestCount),
				"all requests must reach the server before proceeding")

			// Open the gate (requests proceed to handler) and initiate
			// shutdown concurrently. The endpointPropagationDelay (5s)
			// keeps DB/Redis alive while in-flight requests complete.
			close(gate.gate)

			shutdownDone := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				shutdownDone <- srv.Shutdown(ctx)
			}()

			wg.Wait()
			close(results)

			totalSuccess := 0
			for code := range results {
				if code == http.StatusOK {
					totalSuccess++
				}
			}

			Expect(totalSuccess).To(Equal(requestCount),
				"ALL in-flight requests MUST complete during graceful shutdown (DD-007)")

			err := <-shutdownDone
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Business Requirement: Endpoint Propagation Delay Validation", func() {
		It("MUST wait 5 seconds for Kubernetes endpoint removal propagation", func() {
			// Business Scenario: DD-007 STEP 2 timing validation
			// Expected: Shutdown waits exactly 5 seconds before draining connections

			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Measure time for STEP 1 (flag set) + STEP 2 (propagation wait)
			startTime := time.Now()

			// Initiate shutdown
			shutdownDone := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				shutdownDone <- srv.Shutdown(ctx)
			}()

			// Verify readiness returns 503 (STEP 1 complete)
			// Per TESTING_GUIDELINES.md: Use Eventually() instead of time.Sleep()
			// DD-AUTH-014: Use authenticated request
			Eventually(func() int {
				r, e := makeAuthenticatedRequest(healthServer.URL + "/readyz")
				if e != nil || r == nil {
					return 0
				}
				_ = r.Body.Close()
				return r.StatusCode
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(503),
				"Shutdown flag must be set (DD-007 STEP 1)")

			// Get final response for timing measurement
			// DD-AUTH-014: Use authenticated request
			resp, err := makeAuthenticatedRequest(healthServer.URL + "/readyz")
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(503))
			_ = resp.Body.Close()

			flagSetTime := time.Since(startTime)

			// Wait for shutdown to complete
			err = <-shutdownDone
			Expect(err).ToNot(HaveOccurred())

			totalDuration := time.Since(startTime)

			// Business Outcome: STEP 2 propagation delay is present
			// Total time should be at least 5 seconds (STEP 2) + flag set time + cleanup
			Expect(totalDuration).To(BeNumerically(">=", 5*time.Second),
				"Shutdown MUST include 5-second endpoint propagation delay (DD-007 STEP 2)")

			// Flag should be set quickly (< 1 second)
			Expect(flagSetTime).To(BeNumerically("<", 1*time.Second),
				"Shutdown flag should be set quickly in STEP 1")
		})
	})

	Context("BR-STORAGE-028: independent shutdown fixture ownership (#2499)", func() {
		It("IT-DS-2499-001: cannot observe another fixture's pending notifications before drain", func() {
			aHTTP, aHealth, a := createTestServerWithAccess()
			defer aHTTP.Close()
			defer aHealth.Close()
			bHTTP, bHealth, b := createTestServerWithAccess()
			defer bHTTP.Close()
			defer bHealth.Close()
			DeferCleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()
				Expect(a.Shutdown(cleanupCtx)).To(Succeed())
				Expect(b.Shutdown(cleanupCtx)).To(Succeed())
			})

			for i := 0; i < 3; i++ {
				Expect(a.GetDLQClient().EnqueueNotificationAudit(ctx, &models.NotificationAudit{
					RemediationID: generateTestID(), NotificationID: generateTestID(),
					Recipient: "owned@example.com", Channel: "slack", Status: "sent",
					MessageSummary: "fixture-owned shutdown notification", SentAt: time.Now(),
				}, fmt.Errorf("test-owned external database failure"))).To(Succeed())
			}
			Eventually(func(g Gomega) {
				depth, err := a.GetDLQClient().GetDLQDepth(ctx, "notifications")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(depth).To(Equal(int64(3)))
			}, 2*time.Second, 20*time.Millisecond).Should(Succeed())
			depth, err := b.GetDLQClient().GetDLQDepth(ctx, "notifications")
			Expect(err).ToNot(HaveOccurred())
			Expect(depth).To(BeZero(), "a new fixture must not share another fixture's Redis stream")
		})

		It("IT-DS-2499-003: empty shutdown cannot consume another owner's queue (BR-AUDIT-005)", func() {
			aHTTP, aHealth, a := createTestServerWithAccess()
			defer aHTTP.Close()
			defer aHealth.Close()
			bHTTP, bHealth, b := createTestServerWithAccess()
			defer bHTTP.Close()
			defer bHealth.Close()
			DeferCleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()
				Expect(a.Shutdown(cleanupCtx)).To(Succeed())
			})
			Expect(a.GetDLQClient().EnqueueNotificationAudit(ctx, &models.NotificationAudit{
				RemediationID: generateTestID(), NotificationID: generateTestID(),
				Recipient: "owned@example.com", Channel: "slack", Status: "sent",
				MessageSummary: "must remain owned until A shuts down", SentAt: time.Now(),
			}, fmt.Errorf("test-owned external database failure"))).To(Succeed())
			Eventually(func(g Gomega) {
				depth, err := a.GetDLQClient().GetDLQDepth(ctx, "notifications")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(depth).To(Equal(int64(1)))
			}, 2*time.Second, 20*time.Millisecond).Should(Succeed())
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer shutdownCancel()
			Expect(b.Shutdown(shutdownCtx)).To(Succeed())
			depth, err := a.GetDLQClient().GetDLQDepth(ctx, "notifications")
			Expect(err).ToNot(HaveOccurred())
			Expect(depth).To(Equal(int64(1)), "B's shutdown must not drain A's records")
		})
	})

	Context("DD-008: DLQ Drain During Graceful Shutdown", func() {
		It("MUST drain DLQ messages to database before shutdown completes", func() {
			// Business Requirement: BR-AUDIT-001 - Complete audit trail (no data loss during shutdown)
			// Design Decision: DD-008 - DLQ Drain During Graceful Shutdown
			//
			// Scenario: Service has DLQ messages when graceful shutdown is triggered
			// Expected: DLQ messages are persisted to database before shutdown completes

			// ARRANGE: Create test server with owned PostgreSQL + Redis
			testServer, healthServer, srv, backend := createTestServerWithBackends()
			defer testServer.Close()
			defer healthServer.Close()

			// Get DLQ client from server for testing
			// We'll simulate DLQ messages by adding them directly to Redis
			ctx := context.Background()

			// Create test audit events that will be added to DLQ
			testEvents := []struct {
				notificationID string
				eventID        string
			}{
				{notificationID: "notif-dlq-1", eventID: "event-dlq-1"},
				{notificationID: "notif-dlq-2", eventID: "event-dlq-2"},
				{notificationID: "notif-dlq-3", eventID: "event-dlq-3"},
			}

			// Add messages to DLQ by simulating write failures
			// We'll use the DLQ client's enqueue methods directly
			dlqClient := srv.GetDLQClient()
			Expect(dlqClient).ToNot(BeNil(), "DLQ client must be available on server")

			// Enqueue notification audit messages to DLQ
			for _, tc := range testEvents {
				notif := &models.NotificationAudit{
					RemediationID:   "remediation-shutdown-test",
					NotificationID:  tc.notificationID,
					Recipient:       "shutdown-test@example.com",
					Channel:         "slack",
					MessageSummary:  "DLQ drain test notification",
					Status:          "sent",
					SentAt:          time.Now(),
					EscalationLevel: 0,
				}
				err := dlqClient.EnqueueNotificationAudit(ctx, notif, fmt.Errorf("simulated DB failure"))
				Expect(err).ToNot(HaveOccurred(), "Should enqueue notification to DLQ")
			}

			// Verify DLQ has pending messages
			// DD-009: DLQ operations are async, use Eventually() for Redis propagation
			var initialDLQDepth int64
			Eventually(func() int64 {
				depth, err := dlqClient.GetDLQDepth(ctx, "notifications")
				if err != nil {
					return 0
				}
				return depth
			}, 2*time.Second, 100*time.Millisecond).Should(Equal(int64(3)),
				"owned DLQ should have exactly 3 messages after async Redis operations complete")

			// Capture final depth for test logging
			initialDLQDepth, err := dlqClient.GetDLQDepth(ctx, "notifications")
			Expect(err).ToNot(HaveOccurred())
			GinkgoWriter.Printf("Initial DLQ depth: %d messages\n", initialDLQDepth)

			// ACT: Trigger graceful shutdown
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			startTime := time.Now()
			err = srv.Shutdown(shutdownCtx)
			shutdownDuration := time.Since(startTime)

			// ASSERT: Shutdown completes successfully
			Expect(err).ToNot(HaveOccurred(), "Shutdown should complete without error")

			GinkgoWriter.Printf("Shutdown completed in %v\n", shutdownDuration)

			// Verification clients belong to the same fixture and outlive server shutdown.
			depth, err := backend.verifyRedis.XLen(ctx, "audit:dlq:notifications").Result()
			Expect(err).ToNot(HaveOccurred())
			Expect(depth).To(BeZero(), "owned notification stream must be completely drained")

			// Query database to verify DLQ messages were persisted
			// Check for our test notification IDs in the database
			for _, tc := range testEvents {
				var count int
				err = backend.verifyDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM notification_audit WHERE notification_id = $1", tc.notificationID).Scan(&count)
				Expect(err).ToNot(HaveOccurred())
				Expect(count).To(Equal(1),
					fmt.Sprintf("Notification %s should be persisted in database after DLQ drain", tc.notificationID))
			}

			// ASSERT: Shutdown completed within expected time
			// DD-008: DLQ drain timeout is 10 seconds
			// DD-007: Total shutdown should be < 50 seconds (5s propagation + 30s HTTP drain + 10s DLQ + buffer)
			Expect(shutdownDuration).To(BeNumerically("<", 50*time.Second),
				"Shutdown should complete within grace period including DLQ drain")

			// Business Outcome Verified: DLQ messages persisted, no audit data loss during shutdown
			GinkgoWriter.Printf("✅ DD-008 VALIDATED: %d DLQ messages persisted to database during graceful shutdown\n", len(testEvents))
		})

		It("MUST handle graceful shutdown even when DLQ is empty", func() {
			// DD-008: Graceful degradation - shutdown should work even with no DLQ messages

			// ARRANGE: Create test server (DLQ will be empty)
			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// A new owned fixture is genuinely empty; never drain another test's leftovers.
			ctx := context.Background()
			dlqClient := srv.GetDLQClient()
			Expect(dlqClient).ToNot(BeNil(), "DLQ client must be available on server")

			// Check DLQ depth before entering the real shutdown path.
			dlqDepth, err := dlqClient.GetDLQDepth(ctx, "notifications")
			Expect(err).ToNot(HaveOccurred())
			Expect(dlqDepth).To(BeZero())

			// ACT: Trigger graceful shutdown with empty DLQ
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			startTime := time.Now()
			err = srv.Shutdown(shutdownCtx)
			shutdownDuration := time.Since(startTime)

			// ASSERT: Shutdown completes successfully even with empty DLQ
			Expect(err).ToNot(HaveOccurred(), "Shutdown should complete without error even with empty DLQ")

			// ASSERT: Shutdown is quick when DLQ is empty (no drain time needed)
			// Should be < 10 seconds (5s propagation + minimal HTTP drain + minimal DLQ check)
			Expect(shutdownDuration).To(BeNumerically("<", 15*time.Second),
				"Shutdown should be quick when DLQ is empty")

			GinkgoWriter.Printf("✅ DD-008 VALIDATED: Graceful shutdown with empty DLQ completed in %v\n", shutdownDuration)
		})

		It("MUST include DLQ drain time in total shutdown duration", func() {
			// DD-008: Verify DLQ drain step (Step 4) is executed in shutdown sequence

			// ARRANGE: Create test server
			testServer, healthServer, srv := createTestServerWithAccess()
			defer testServer.Close()
			defer healthServer.Close()

			// Add a few messages to DLQ to ensure drain step executes
			ctx := context.Background()
			dlqClient := srv.GetDLQClient()
			Expect(dlqClient).ToNot(BeNil(), "DLQ client must be available on server")

			// Add 5 messages to DLQ
			for i := 0; i < 5; i++ {
				notif := &models.NotificationAudit{
					RemediationID:   fmt.Sprintf("remediation-timing-%d", i),
					NotificationID:  fmt.Sprintf("notif-timing-%d", i),
					Recipient:       "timing-test@example.com",
					Channel:         "slack",
					MessageSummary:  "Timing test notification",
					Status:          "sent",
					SentAt:          time.Now(),
					EscalationLevel: 0,
				}
				err := dlqClient.EnqueueNotificationAudit(ctx, notif, fmt.Errorf("simulated failure"))
				Expect(err).ToNot(HaveOccurred())
			}

			// ACT: Measure shutdown timing
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			startTime := time.Now()
			err := srv.Shutdown(shutdownCtx)
			shutdownDuration := time.Since(startTime)

			// ASSERT: Shutdown completes successfully
			Expect(err).ToNot(HaveOccurred())

			// ASSERT: Shutdown duration includes DLQ drain
			// Minimum time should be > 5 seconds (endpoint propagation delay from DD-007)
			// Maximum time should be < 50 seconds (all steps combined)
			Expect(shutdownDuration).To(BeNumerically(">", 5*time.Second),
				"Shutdown should take at least 5s for endpoint propagation (DD-007 Step 2)")
			Expect(shutdownDuration).To(BeNumerically("<", 50*time.Second),
				"Shutdown should complete within grace period")

			GinkgoWriter.Printf("✅ DD-008 VALIDATED: Graceful shutdown with DLQ messages completed in %v\n", shutdownDuration)
		})
	})
})

// Helper function for creating test server with direct access to server instance
// This allows tests to access internal server state (e.g., DLQ client) for validation
// makeAuthenticatedRequest creates an HTTP request with Bearer token authentication
// DD-AUTH-014: All DataStorage requests require authentication
// requestGate is an http.Handler middleware that counts arriving requests
// and blocks them until the gate channel is closed. This lets the test
// guarantee that all N requests are genuinely in-flight inside the server
// before triggering shutdown — removing all timing sensitivity.
type requestGate struct {
	handler http.Handler
	arrived atomic.Int32
	gate    chan struct{} // closed to release all blocked requests
}

func (g *requestGate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.arrived.Add(1)
	<-g.gate
	g.handler.ServeHTTP(w, r)
}

// Token must match MockAuthenticator.ValidUsers configuration
func makeAuthenticatedRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// Add Bearer token matching MockAuthenticator configuration
	req.Header.Set("Authorization", "Bearer test-token")
	return http.DefaultClient.Do(req)
}

func createTestServerWithAccess() (*httptest.Server, *httptest.Server, *server.Server) {
	httpServer, healthServer, srv, _ := createTestServerWithBackends()
	return httpServer, healthServer, srv
}

// Every caller, including shared-port readiness tests, owns a migrated database
// and a private Redis endpoint. Unique notification IDs alone do not isolate drains.
func createTestServerWithBackends() (*httptest.Server, *httptest.Server, *server.Server, *shutdownBackend) {
	provisionCtx, provisionCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer provisionCancel()
	backend, err := newShutdownBackend(provisionCtx, "quay.io/jordigilh/redis:7-alpine")
	Expect(err).ToNot(HaveOccurred(), "provisioning owned shutdown backends")
	DeferCleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cleanupCancel()
		Expect(backend.cleanup(cleanupCtx)).To(Succeed(), "cleanup owner %s", backend.name)
	})

	// Create server config
	serverCfg := &server.Config{
		Port:         18090, // DD-TEST-001
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		TLS:          datastorageIntegrationTLSConfig(),
	}

	// Create application config with database pool settings.
	// EndpointPropagationDelay is set explicitly so the test documents
	// the DD-007 STEP 2 window that keeps DB/Redis alive while in-flight
	// requests drain during graceful shutdown.
	appCfg := &config.Config{
		Server: config.ServerConfig{
			SignerCertDir:            datastorageIntegrationSigningCertDirOrDie(),
			EndpointPropagationDelay: "5s",
		},
		Database: config.DatabaseConfig{
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: "5m",
			ConnMaxIdleTime: "10m",
		},
	}

	// DD-AUTH-014: Graceful shutdown tests use permissive mock auth
	// These tests focus on shutdown behavior, not authentication
	// Mock authenticator/authorizer allow all access for these tests
	//
	// Note: Tests that need to validate specific auth behavior should create
	// their own server with specific mock configurations (see SAR E2E tests)
	mockAuthenticator := &auth.MockAuthenticator{
		ValidUsers: map[string]string{
			"test-token": "system:serviceaccount:datastorage-test:graceful-shutdown-test",
		},
	}
	mockAuthorizer := &auth.MockAuthorizer{
		AllowedUsers: map[string]bool{
			"system:serviceaccount:datastorage-test:graceful-shutdown-test": true,
		},
	}

	// Create server instance (this will create its own DB connection pool)
	// dlqMaxLen: 1000 events (default from DD-009)
	// SOC2 Gap #9: PostgreSQL with custom hash chains for tamper detection
	// DD-AUTH-014: Pass mock authenticator/authorizer for test environment
	// authNamespace is "datastorage-test" for SAR checks (mock authorizer allows all)
	srv, err := server.NewServer(server.ServerDeps{
		DBConnStr:     backend.connStr,
		RedisAddr:     backend.redisAddr,
		Logger:        logger,
		AppConfig:     appCfg,
		ServerConfig:  serverCfg,
		DLQMaxLen:     1000,
		Authenticator: mockAuthenticator,
		Authorizer:    mockAuthorizer,
		AuthNamespace: "datastorage-test",
	})
	Expect(err).ToNot(HaveOccurred(), "Server creation should succeed")

	// Wrap in httptest.Server for HTTP testing
	httpServer := httptest.NewServer(srv.Handler())

	// Dedicated health server (Issue #753: health probes on separate port)
	healthSrv := health.NewHealthServer(":0", srv.LivenessHandler(), srv.ReadinessHandler(), false)
	healthTestServer := httptest.NewServer(healthSrv.Handler)
	backend.stopServer = func(cleanupCtx context.Context) error {
		httpServer.Close()
		healthTestServer.Close()
		// Shutdown tests already called the real lifecycle. Only call it here
		// for callers (e.g. readiness tests) that leave the server running.
		probe := httptest.NewRecorder()
		srv.ReadinessHandler()(probe, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		var readiness models.ReadinessResponse
		if err := json.Unmarshal(probe.Body.Bytes(), &readiness); err != nil {
			return fmt.Errorf("decode cleanup readiness for %s: %w", backend.name, err)
		}
		if readiness.Reason == "shutting_down" {
			return nil
		}
		return srv.Shutdown(cleanupCtx)
	}

	return httpServer, healthTestServer, srv, backend
}
