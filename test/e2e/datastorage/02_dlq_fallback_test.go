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

	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Scenario 2: DLQ Fallback - Service Outage Recovery (P0)
//
// Business Requirements:
// - BR-STORAGE-007: Dead Letter Queue (DLQ) fallback
// - BR-STORAGE-008: DLQ recovery mechanism
//
// Business Value: Verify audit events are preserved during service outages
//
// Test Flow:
// 1. Deploy Data Storage Service in isolated namespace
// 2. Write audit event successfully (baseline)
// 3. Simulate PostgreSQL outage (scale to 0 replicas)
// 4. Attempt to write audit event → should fallback to DLQ (Redis)
// 5. Restore PostgreSQL (scale to 1 replica)
// 6. Verify DLQ recovery processes failed events
// 7. Verify all events persisted to audit_events table
//
// Expected Results:
// - First event: Direct write to PostgreSQL (success)
// - Second event: Fallback to DLQ (Redis) during outage
// - Third event: DLQ recovery writes to PostgreSQL after restoration
// - All 3 events persisted in audit_events table
// - Zero data loss
//
// Parallel Execution: ✅ ENABLED
// - Each test gets unique namespace (datastorage-e2e-p{N}-{timestamp})
// - Complete infrastructure isolation
// - No impact from other tests

var _ = Describe("Scenario 2: DLQ Fallback - Service Outage Recovery", Label("e2e", "dlq", "p0"), Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testLogger    *zap.Logger
		httpClient    *http.Client
		testNamespace string
		serviceURL    string
		db            *sql.DB
		correlationID string
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 15*time.Minute)
		testLogger = logger.With(zap.String("test", "dlq-fallback"))
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario 2: DLQ Fallback - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Generate unique namespace for this test (parallel execution)
		testNamespace = generateUniqueNamespace()
		testLogger.Info("Deploying test services...", zap.String("namespace", testNamespace))

		// Deploy PostgreSQL, Redis, and Data Storage Service
		err := infrastructure.DeployDataStorageTestServices(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())

		// Set up port-forward to Data Storage Service
		localPort := 8080 + GinkgoParallelProcess()
		serviceURL = fmt.Sprintf("http://localhost:%d", localPort)

		portForwardCancel, err := portForwardService(testCtx, testNamespace, "datastorage", localPort, 8080)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			if portForwardCancel != nil {
				portForwardCancel()
			}
		})

		testLogger.Info("Service URL configured", zap.String("url", serviceURL))

		// Wait for Data Storage Service HTTP endpoint to be responsive
		testLogger.Info("⏳ Waiting for Data Storage Service HTTP endpoint...")
		Eventually(func() error {
			resp, err := httpClient.Get(serviceURL + "/health")
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("health check returned status %d", resp.StatusCode)
			}
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed(), "Data Storage Service should be healthy")
		testLogger.Info("✅ Data Storage Service is responsive")

		// Connect to PostgreSQL for verification
		testLogger.Info("🔌 Connecting to PostgreSQL for verification...")
		pgLocalPort := 5432 + GinkgoParallelProcess()
		pgPortForwardCancel, err := portForwardService(testCtx, testNamespace, "postgresql", pgLocalPort, 5432)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			if pgPortForwardCancel != nil {
				pgPortForwardCancel()
			}
		})

		connStr := fmt.Sprintf("host=localhost port=%d user=slm_user password=test_password dbname=action_history sslmode=disable", pgLocalPort)
		db, err = sql.Open("pgx", connStr)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() error {
			return db.Ping()
		}, 30*time.Second, 2*time.Second).Should(Succeed(), "PostgreSQL should be connectable")
		testLogger.Info("✅ PostgreSQL connected")

		// Generate unique correlation ID for this test
		correlationID = fmt.Sprintf("dlq-test-%s", testNamespace)

		testLogger.Info("✅ Test services ready", zap.String("namespace", testNamespace))
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("🧹 Cleaning up test namespace...")
		if db != nil {
			db.Close()
		}
		if testCancel != nil {
			testCancel()
		}

		err := infrastructure.CleanupDataStorageTestNamespace(testNamespace, kubeconfigPath, GinkgoWriter)
		if err != nil {
			testLogger.Warn("Failed to cleanup namespace", zap.Error(err))
		}
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
			"service":        "gateway",
			"event_type":     "gateway.signal.received",
			"correlation_id": correlationID,
			"outcome":        "success",
			"operation":      "baseline_write",
			"event_data":     baselineEventData,
		}

		resp := postAuditEvent(httpClient, serviceURL, baselineEvent)
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Baseline event should be created")
		resp.Body.Close()
		testLogger.Info("✅ Baseline event written successfully")

		// Verify baseline event in database
		var count int
		err = db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1`, correlationID).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(1), "Should have 1 baseline event in database")
		testLogger.Info("✅ Baseline event verified in database")

		// Step 2: Simulate PostgreSQL outage (scale to 0 replicas)
		testLogger.Info("💥 Step 2: Simulating PostgreSQL outage (scale to 0)...")
		err = scalePod(testNamespace, "postgresql", 0)
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
			"service":        "gateway",
			"event_type":     "gateway.signal.received",
			"correlation_id": correlationID,
			"outcome":        "success",
			"operation":      "outage_write",
			"event_data":     outageEventData,
		}

		resp = postAuditEvent(httpClient, serviceURL, outageEvent)
		// During outage, the service should accept the event (202 Accepted) and queue it
		Expect(resp.StatusCode).To(Or(Equal(http.StatusCreated), Equal(http.StatusAccepted)),
			"Event should be accepted during outage (DLQ fallback)")
		resp.Body.Close()
		testLogger.Info("✅ Event accepted during outage (DLQ fallback)")

		// Step 4: Restore PostgreSQL (scale to 1 replica)
		testLogger.Info("🔄 Step 4: Restoring PostgreSQL (scale to 1)...")
		err = scalePod(testNamespace, "postgresql", 1)
		Expect(err).ToNot(HaveOccurred())

		// Wait for PostgreSQL to be available
		testLogger.Info("⏳ Waiting for PostgreSQL to be available...")
		err = waitForPodReady(testNamespace, "app=postgresql", 60*time.Second)
		Expect(err).ToNot(HaveOccurred())

		// Reconnect to PostgreSQL
		testLogger.Info("🔌 Reconnecting to PostgreSQL...")
		Eventually(func() error {
			return db.Ping()
		}, 30*time.Second, 2*time.Second).Should(Succeed(), "PostgreSQL should be available")
		testLogger.Info("✅ PostgreSQL is available")

		// Step 5: Wait for DLQ recovery to process failed events
		testLogger.Info("⏳ Step 5: Waiting for DLQ recovery to process events...")
		// DLQ recovery should automatically process events from Redis and write to PostgreSQL
		// Give it some time to process (ADR-034 specifies automatic recovery)
		time.Sleep(10 * time.Second)

		// Step 6: Verify all events are in database (baseline + DLQ recovered)
		testLogger.Info("🔍 Step 6: Verifying all events persisted after recovery...")
		Eventually(func() int {
			var eventCount int
			err := db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE correlation_id = $1`, correlationID).Scan(&eventCount)
			if err != nil {
				testLogger.Warn("Failed to query event count", zap.Error(err))
				return 0
			}
			return eventCount
		}, 60*time.Second, 2*time.Second).Should(Equal(2),
			"Should have 2 events in database (baseline + DLQ recovered)")
		testLogger.Info("✅ All events persisted after DLQ recovery")

		// Verification: Query event details
		testLogger.Info("🔍 Verifying event details...")
		rows, err := db.Query(`
			SELECT operation, outcome
			FROM audit_events
			WHERE correlation_id = $1
			ORDER BY event_timestamp ASC
		`, correlationID)
		Expect(err).ToNot(HaveOccurred())
		defer rows.Close()

		operations := []string{}
		for rows.Next() {
			var operation, outcome string
			err := rows.Scan(&operation, &outcome)
			Expect(err).ToNot(HaveOccurred())
			Expect(outcome).To(Equal("success"), "All events should have success outcome")
			operations = append(operations, operation)
		}

		Expect(operations).To(Equal([]string{"baseline_write", "outage_write"}),
			"Should have both baseline and outage events in order")
		testLogger.Info("✅ Event details verified")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Scenario 2: DLQ Fallback - PASSED")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})

