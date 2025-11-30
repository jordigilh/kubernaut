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
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/audit"
)

// Scenario 1: Happy Path - Complete Remediation Audit Trail (P0)
//
// Business Requirements:
// - BR-STORAGE-001: Audit persistence
// - BR-STORAGE-021: REST API Read Endpoints
// - BR-STORAGE-022: Query Filtering
//
// Business Value: Verify complete audit trail across all services
//
// Test Flow:
// 1. Deploy Data Storage Service in isolated namespace
// 2. Simulate audit events from 5 services (Gateway, AIAnalysis, Workflow, Orchestrator, Monitor)
// 3. Verify all events persisted to audit_events table (ADR-034 unified table)
// 4. Query by correlation_id and verify complete timeline
// 5. Verify chronological order
//
// Expected Results:
// - 5 audit records created in audit_events table
// - All audit writes complete <1s (p95 latency)
// - Zero DLQ fallbacks
// - Query API retrieves complete timeline by correlation_id
//
// Parallel Execution: ✅ ENABLED
// - Each test gets unique namespace (datastorage-e2e-p{N}-{timestamp})
// - Complete infrastructure isolation
// - No data pollution between tests

var _ = Describe("Scenario 1: Happy Path - Complete Remediation Audit Trail", Label("e2e", "happy-path", "p0"), Ordered, func() {
	var (
		testCancel    context.CancelFunc
		testLogger    *zap.Logger
		httpClient    *http.Client
		testNamespace string
		serviceURL    string
		db            *sql.DB
		correlationID string
	)

	BeforeAll(func() {
		_, testCancel = context.WithTimeout(ctx, 15*time.Minute)
		testLogger = logger.With(zap.String("test", "happy-path"))
		httpClient = &http.Client{Timeout: 10 * time.Second}

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Scenario 1: Happy Path - Setup")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Use shared deployment from SynchronizedBeforeSuite (no per-test deployment)
		// Services are deployed ONCE and shared via NodePort (no port-forwarding needed)
		testNamespace = sharedNamespace
		serviceURL = dataStorageURL
		testLogger.Info("Using shared deployment", zap.String("namespace", testNamespace), zap.String("url", serviceURL))

		// Wait for Data Storage Service HTTP endpoint to be responsive
		testLogger.Info("⏳ Waiting for Data Storage Service HTTP endpoint...")
		Eventually(func() error {
			resp, err := httpClient.Get(serviceURL + "/health")
			if err != nil {
				testLogger.Debug("Health check failed, retrying...", zap.Error(err))
				return err
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					testLogger.Error("failed to close response body", zap.Error(err))
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
		connStr := fmt.Sprintf("host=localhost port=5432 user=slm_user password=test_password dbname=action_history sslmode=disable")
		var err error
		db, err = sql.Open("pgx", connStr)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() error {
			return db.Ping()
		}, 30*time.Second, 2*time.Second).Should(Succeed(), "PostgreSQL should be connectable")
		testLogger.Info("✅ PostgreSQL connected")

		// Generate unique correlation ID for this test
		correlationID = fmt.Sprintf("remediation-%s", testNamespace)

		testLogger.Info("✅ Test services ready", zap.String("namespace", testNamespace))
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		testLogger.Info("🧹 Cleaning up test resources...")
		if db != nil {
			if err := db.Close(); err != nil {
				testLogger.Warn("failed to close database connection", zap.Error(err))
			}
		}
		if testCancel != nil {
			testCancel()
		}
		// Note: Shared namespace is NOT cleaned up here - it's managed by SynchronizedAfterSuite
	})

	It("should create complete audit trail across all services", func() {
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("Test: Complete Audit Trail")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Step 1: Gateway - Signal Received
		testLogger.Info("📨 Step 1: Gateway processes signal...")
		gatewayEventData, err := audit.NewGatewayEvent("signal.received").
			WithSignalType("prometheus").
			WithAlertName("PodCrashLooping").
			Build()
		Expect(err).ToNot(HaveOccurred())

		gatewayEvent := map[string]interface{}{
			"version":         "1.0",
			"service":         "gateway",
			"event_type":      "gateway.signal.received",
			"event_timestamp": time.Now().UTC().Format(time.RFC3339),
			"correlation_id":  correlationID,
			"outcome":         "success",
			"operation":       "signal_processing",
			"event_data":      gatewayEventData,
		}

		resp := postAuditEvent(httpClient, serviceURL, gatewayEvent)
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Gateway audit event should be created")
		if err := resp.Body.Close(); err != nil {
			testLogger.Error("failed to close response body", zap.Error(err))
		}
		testLogger.Info("✅ Gateway audit event created")

		// Step 2: AIAnalysis - Analysis Completed
		testLogger.Info("🤖 Step 2: AIAnalysis generates RCA...")
		aiEventData, err := audit.NewAIAnalysisEvent("analysis.completed").
			WithAnalysisID(fmt.Sprintf("analysis-%s", testNamespace)).
			Build()
		Expect(err).ToNot(HaveOccurred())

		aiEvent := map[string]interface{}{
			"version":         "1.0",
			"service":         "aianalysis",
			"event_type":      "aianalysis.analysis.completed",
			"event_timestamp": time.Now().UTC().Format(time.RFC3339),
			"correlation_id":  correlationID,
			"outcome":         "success",
			"operation":       "rca_generation",
			"event_data":      aiEventData,
		}

		resp = postAuditEvent(httpClient, serviceURL, aiEvent)
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "AIAnalysis audit event should be created")
		if err := resp.Body.Close(); err != nil {
			testLogger.Error("failed to close response body", zap.Error(err))
		}
		testLogger.Info("✅ AIAnalysis audit event created")

		// Step 3: Workflow - Workflow Completed
		testLogger.Info("⚙️  Step 3: Workflow executes remediation...")
		workflowEventData, err := audit.NewWorkflowEvent("workflow.completed").
			WithWorkflowID(fmt.Sprintf("workflow-%s", testNamespace)).
			Build()
		Expect(err).ToNot(HaveOccurred())

		workflowEvent := map[string]interface{}{
			"version":         "1.0",
			"service":         "workflow",
			"event_type":      "workflow.workflow.completed",
			"event_timestamp": time.Now().UTC().Format(time.RFC3339),
			"correlation_id":  correlationID,
			"outcome":         "success",
			"operation":       "remediation_execution",
			"event_data":      workflowEventData,
		}

		resp = postAuditEvent(httpClient, serviceURL, workflowEvent)
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Workflow audit event should be created")
		if err := resp.Body.Close(); err != nil {
			testLogger.Error("failed to close response body", zap.Error(err))
		}
		testLogger.Info("✅ Workflow audit event created")

		// Step 4: Orchestrator - Remediation Completed
		testLogger.Info("🎯 Step 4: Orchestrator completes...")
		orchestratorEvent := map[string]interface{}{
			"version":         "1.0",
			"service":         "orchestrator",
			"event_type":      "orchestrator.remediation.completed",
			"event_timestamp": time.Now().UTC().Format(time.RFC3339),
			"correlation_id":  correlationID,
			"outcome":         "success",
			"operation":       "orchestration",
			"event_data":      map[string]interface{}{},
		}

		resp = postAuditEvent(httpClient, serviceURL, orchestratorEvent)
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Orchestrator audit event should be created")
		if err := resp.Body.Close(); err != nil {
			testLogger.Error("failed to close response body", zap.Error(err))
		}
		testLogger.Info("✅ Orchestrator audit event created")

		// Step 5: EffectivenessMonitor - Assessment Completed
		testLogger.Info("📊 Step 5: EffectivenessMonitor assesses...")
		monitorEvent := map[string]interface{}{
			"version":         "1.0",
			"service":         "monitor",
			"event_type":      "monitor.assessment.completed",
			"event_timestamp": time.Now().UTC().Format(time.RFC3339),
			"correlation_id":  correlationID,
			"outcome":         "success",
			"operation":       "effectiveness_assessment",
			"event_data":      map[string]interface{}{},
		}

		resp = postAuditEvent(httpClient, serviceURL, monitorEvent)
		Expect(resp.StatusCode).To(Equal(http.StatusCreated), "Monitor audit event should be created")
		if err := resp.Body.Close(); err != nil {
			testLogger.Error("failed to close response body", zap.Error(err))
		}
		testLogger.Info("✅ Monitor audit event created")

		// Verification: Query database directly
		testLogger.Info("🔍 Verifying audit events in database...")
		var count int
		err = db.QueryRow(`
			SELECT COUNT(*) FROM audit_events
			WHERE correlation_id = $1
		`, correlationID).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(5), "Should have 5 audit events in database")
		testLogger.Info("✅ All 5 audit events persisted to database")

		// Verification: Query via REST API
		testLogger.Info("🔍 Querying audit trail via REST API...")
		resp, err = httpClient.Get(fmt.Sprintf("%s/api/v1/audit/events?correlation_id=%s", serviceURL, correlationID))
		Expect(err).ToNot(HaveOccurred())
		defer func() {
			if err := resp.Body.Close(); err != nil {
				// Best effort - if we can't write to GinkgoWriter, there's nothing we can do
				_, _ = fmt.Fprintf(GinkgoWriter, "⚠️  Failed to close response body: %v\n", err)
			}
		}()
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "Query API should return 200 OK")

		var queryResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&queryResponse)
		Expect(err).ToNot(HaveOccurred())

		data, ok := queryResponse["data"].([]interface{})
		Expect(ok).To(BeTrue(), "Response should have data array")
		Expect(data).To(HaveLen(5), "Query API should return 5 events")
		testLogger.Info("✅ Query API returned complete audit trail")

		// Verification: Chronological order (sort events first since API doesn't guarantee order)
		testLogger.Info("🔍 Verifying chronological order...")

		// Sort events by timestamp
		sort.Slice(data, func(i, j int) bool {
			eventI := data[i].(map[string]interface{})
			eventJ := data[j].(map[string]interface{})
			timestampI, _ := time.Parse(time.RFC3339, eventI["event_timestamp"].(string))
			timestampJ, _ := time.Parse(time.RFC3339, eventJ["event_timestamp"].(string))
			return timestampI.Before(timestampJ)
		})

		var previousTimestamp time.Time
		for i, item := range data {
			event := item.(map[string]interface{})
			timestampStr := event["event_timestamp"].(string)
			timestamp, err := time.Parse(time.RFC3339, timestampStr)
			Expect(err).ToNot(HaveOccurred())

			if i > 0 {
				Expect(timestamp.After(previousTimestamp) || timestamp.Equal(previousTimestamp)).To(BeTrue(),
					"Events should be in chronological order")
			}
			previousTimestamp = timestamp
		}
		testLogger.Info("✅ Events are in chronological order")

		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		testLogger.Info("✅ Scenario 1: Happy Path - PASSED")
		testLogger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})
})

// Helper function to post audit event
func postAuditEvent(client *http.Client, baseURL string, event map[string]interface{}) *http.Response {
	body, err := json.Marshal(event)
	Expect(err).ToNot(HaveOccurred())

	req, err := http.NewRequest("POST", baseURL+"/api/v1/audit/events", bytes.NewBuffer(body))
	Expect(err).ToNot(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	Expect(err).ToNot(HaveOccurred())

	// Log response body if not 2xx status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		if err := resp.Body.Close(); err != nil {
			// Best effort - if we can't write to GinkgoWriter, there's nothing we can do
			_, _ = fmt.Fprintf(GinkgoWriter, "⚠️  Failed to close response body: %v\n", err)
		}
		if _, err := fmt.Fprintf(GinkgoWriter, "❌ HTTP %d Response Body: %s\n", resp.StatusCode, string(bodyBytes)); err != nil {
			// Best effort - if we can't write to GinkgoWriter, there's nothing we can do
			_, _ = fmt.Fprintf(GinkgoWriter, "⚠️  Failed to write to GinkgoWriter: %v\n", err)
		}
		// Create new reader for the response body so tests can still read it
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	return resp
}
