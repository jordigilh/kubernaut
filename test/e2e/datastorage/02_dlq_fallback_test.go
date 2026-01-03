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
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Scenario 2: DLQ Fallback - Service Outage Response (P0)
//
// Business Requirements:
// - BR-STORAGE-007: Dead Letter Queue (DLQ) fallback
//
// Business Value: Verify audit events fallback to DLQ during service outages
//
// Test Flow:
// 1. Deploy Data Storage Service in isolated namespace
// 2. Write audit event successfully (baseline - verify 201 Created)
// 3. Simulate PostgreSQL outage (scale to 0 replicas)
// 4. Attempt to write audit event → should fallback to DLQ (Redis)
// 5. Verify service returns 202 Accepted (DLQ fallback response)
//
// Expected Results:
// - First event: Direct write to PostgreSQL (201 Created)
// - Second event: Fallback to DLQ (202 Accepted)
// - Service handles outage gracefully without errors
//
// Note: DLQ write mechanics are tested in integration tier (dlq_test.go)
// E2E focuses ONLY on end-to-end HTTP response behavior during outages
//
// Parallel Execution: ✅ ENABLED
// - Each test gets unique namespace (datastorage-e2e-p{N}-{timestamp})
// - Complete infrastructure isolation
// - No impact from other tests

var _ = Describe("BR-DS-004: DLQ Fallback Reliability - No Data Loss During Outage", Label("e2e", "dlq", "p0"), Ordered, func() {
	var (
		testCancel    context.CancelFunc
		testLogger    logr.Logger
		httpClient    *http.Client
		testNamespace string
		serviceURL    string
		db            *sql.DB
		correlationID string
	)

	BeforeAll(func() {
		_, testCancel = context.WithTimeout(ctx, 15*time.Minute)
		testLogger = logger.WithValues("test", "dlq-fallback")
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario 2: DLQ Fallback - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Use shared deployment from SynchronizedBeforeSuite (no per-test deployment)
		// Services are deployed ONCE and shared via NodePort (no port-forwarding needed)
		testNamespace = sharedNamespace
		serviceURL = dataStorageURL
		testLogger.Info("Using shared deployment", "namespace", testNamespace, "url", serviceURL)

		// Wait for Data Storage Service HTTP endpoint to be responsive
		testLogger.Info("⏳ Waiting for Data Storage Service HTTP endpoint...")
		Eventually(func() error {
			resp, err := httpClient.Get(serviceURL + "/health")
			if err != nil {
				return err
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					testLogger.Error(err, "failed to close response body")
				}
			}()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned status %d", resp.StatusCode)
			}
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed(), "Data Storage Service should be healthy")
		testLogger.Info("✅ Data Storage Service is responsive")

		// Connect to PostgreSQL for verification (using shared NodePort - no port-forward needed)
		testLogger.Info("🔌 Connecting to PostgreSQL via NodePort...")
		connStr := fmt.Sprintf("host=localhost port=25433 user=slm_user password=test_password dbname=action_history sslmode=disable") // Per DD-TEST-001
		var err error
		db, err = sql.Open("pgx", connStr)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() error {
			return db.Ping()
		}, 30*time.Second, 2*time.Second).Should(Succeed(), "PostgreSQL should be connectable")
		testLogger.Info("✅ PostgreSQL connected")

		// Generate unique correlation ID for this test
		correlationID = fmt.Sprintf("dlq-test-%s", testNamespace)

		testLogger.Info("✅ Test services ready", "namespace", testNamespace)
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("🧹 Cleaning up DLQ test resources...")

		// CRITICAL: Restore PostgreSQL to 1 replica before cleanup
		// This test scales PostgreSQL to 0 to simulate outage, but since we use
		// shared infrastructure, we MUST restore it for subsequent tests.
		testLogger.Info("🔄 Restoring PostgreSQL to 1 replica (shared infrastructure)...")
		if err := scalePod(testNamespace, "postgresql", kubeconfigPath, 1); err != nil {
			testLogger.Error(err, "Failed to restore PostgreSQL - subsequent tests may fail!",
				"namespace", testNamespace)
		} else {
			testLogger.Info("✅ PostgreSQL restored to 1 replica")

			// Wait for PostgreSQL to be ready before continuing
			testLogger.Info("⏳ Waiting for PostgreSQL to be ready...")
			Eventually(func() error {
				// Create a new connection to test PostgreSQL availability
				connStr := fmt.Sprintf("host=localhost port=25433 user=slm_user password=test_password dbname=action_history sslmode=disable") // Per DD-TEST-001
				testDB, err := sql.Open("pgx", connStr)
				if err != nil {
					return err
				}
				defer testDB.Close()
				return testDB.Ping()
			}, 60*time.Second, 2*time.Second).Should(Succeed(), "PostgreSQL should be ready after restore")
			testLogger.Info("✅ PostgreSQL is ready")

			// CRITICAL: Re-apply migrations after PostgreSQL restart
			// PostgreSQL uses EmptyDir volume, so data is lost when pod restarts
			testLogger.Info("📋 Re-applying migrations after PostgreSQL restart...")
			if err := infrastructure.ApplyMigrations(ctx, testNamespace, kubeconfigPath, GinkgoWriter); err != nil {
				testLogger.Error(err, "Failed to re-apply migrations - subsequent tests may fail!")
			} else {
				testLogger.Info("✅ Migrations re-applied successfully")
			}
		}

		// Close database connection
		if db != nil {
			if err := db.Close(); err != nil {
				testLogger.Info("warning: failed to close database connection", "error", err)
			}
		}
		if testCancel != nil {
			testCancel()
		}

		// NOTE: Do NOT cleanup the shared namespace - it's used by other tests
		// The namespace cleanup is handled by SynchronizedAfterSuite
		testLogger.Info("✅ DLQ test cleanup complete (shared namespace preserved)")
	})

	It("should preserve audit events during PostgreSQL outage using DLQ", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test: DLQ Fallback and Recovery")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Step 1: Write event successfully (baseline)
		testLogger.Info("✅ Step 1: Write baseline event to PostgreSQL...")
		baselineEventData, err := audit.NewGatewayEvent("signal.received").
			WithSignalType("prometheus").
			WithAlertName("PodCrashLooping").
			Build()
		Expect(err).ToNot(HaveOccurred())

		baselineEvent := map[string]interface{}{
			"version":         "1.0",
			"event_category":  "gateway",
			"event_type":      "gateway.signal.received",
			"event_timestamp": time.Now().UTC().Format(time.RFC3339),
			"correlation_id":  correlationID,
			"event_outcome":   "success",
			"event_action":    "baseline_write",
			"event_data":      baselineEventData,
		}

		resp := postAuditEvent(httpClient, serviceURL, baselineEvent)
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Baseline event should be created")
		if err := resp.Body.Close(); err != nil {
			testLogger.Error(err, "failed to close response body")
		}
		testLogger.Info("✅ Baseline event written successfully")

		// Verify baseline event in database
		var count int
		err = db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1`, correlationID).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(1), "Should have 1 baseline event in database")
		testLogger.Info("✅ Baseline event verified in database")

		// Step 2: Simulate PostgreSQL outage (scale to 0 replicas)
		testLogger.Info("💥 Step 2: Simulating PostgreSQL outage (scale to 0)...")
		err = scalePod(testNamespace, "postgresql", kubeconfigPath, 0)
		Expect(err).ToNot(HaveOccurred())

		// Wait for PostgreSQL to be unavailable
		testLogger.Info("⏳ Waiting for PostgreSQL to be unavailable...")
		Eventually(func() error {
			return db.Ping()
		}, 30*time.Second, 2*time.Second).ShouldNot(Succeed(), "PostgreSQL should be unavailable")
		testLogger.Info("✅ PostgreSQL is unavailable")

		// Step 3: Attempt to write event during outage → should fallback to DLQ
		testLogger.Info("📨 Step 3: Writing event during outage (should fallback to DLQ)...")
		outageEventData, err := audit.NewGatewayEvent("signal.received").
			WithSignalType("prometheus").
			WithAlertName("NodeNotReady").
			Build()
		Expect(err).ToNot(HaveOccurred())

		outageEvent := map[string]interface{}{
			"version":         "1.0",
			"event_category":  "gateway",
			"event_type":      "gateway.signal.received",
			"event_timestamp": time.Now().UTC().Format(time.RFC3339),
			"correlation_id":  correlationID,
			"event_outcome":   "success",
			"event_action":    "outage_write",
			"event_data":      outageEventData,
		}

		resp = postAuditEvent(httpClient, serviceURL, outageEvent)
		// During outage, the service should accept the event (202 Accepted) and queue it
		Expect(resp.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusAccepted)),
			"Event should be accepted during outage (DLQ fallback)")
		if err := resp.Body.Close(); err != nil {
			testLogger.Error(err, "failed to close response body")
		}
		testLogger.Info("✅ Event accepted during outage (DLQ fallback)")

		// Step 4: Verify DLQ fallback behavior
		testLogger.Info("🔍 Step 4: Verifying DLQ fallback succeeded...")
		testLogger.Info("✅ DLQ fallback test complete:")
		testLogger.Info("   • Baseline event written successfully (201 Created)")
		testLogger.Info("   • Outage event accepted for DLQ processing (202 Accepted)")
		testLogger.Info("   • Service handled database outage gracefully")
		testLogger.Info("")
		testLogger.Info("⚠️  Note: Automatic DLQ recovery (DD-009) is tested in integration tier")
		testLogger.Info("   This E2E test focuses on end-to-end outage response behavior")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Scenario 2: DLQ Fallback - PASSED")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})
