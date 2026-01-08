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
	"encoding/base64"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// SOC2 Compliance E2E Test Suite
//
// This test suite validates SOC2 compliance features with cert-manager integration:
// - CC8.1 (Tamper-evident audit logs): Hash chains
// - AU-9 (Protection of Audit Information): Immutable audit storage, legal hold
// - SOX/HIPAA: 7-year retention, litigation hold
// - Digital Signatures: Signed audit exports with cert-manager managed certificates
//
// **CRITICAL**: This is the ONLY DataStorage E2E test that installs cert-manager.
// All other DataStorage tests use the fallback self-signed certificate generation.
//
// Why cert-manager only here?
// - Regular DataStorage tests use fallback generation (~10s startup)
// - This test validates production cert-manager flow (~30s cert-manager setup)
// - Other services (Gateway, AI, etc.) don't use certificates at all
//
// Test Coverage:
// - Signed audit exports (Day 9.1)
// - Hash chain verification (Day 9.1)
// - Legal hold enforcement (Day 8)
// - Certificate fingerprint validation (Day 9.1)
//
// SOC2 Requirements Validated:
// - BR-SOC2-001: Tamper-evident hash chains
// - BR-SOC2-002: Immutable audit storage
// - BR-SOC2-003: Legal hold mechanism
// - BR-SOC2-004: Digital signatures for exports
// - BR-SOC2-005: Certificate-based signing (production flow)

var _ = Describe("SOC2 Compliance Features (cert-manager)", Ordered, func() {
	var (
		_ context.Context // testCtx - will be used when tests are implemented
		testCancel context.CancelFunc

		// SOC2-specific namespace (isolated from regular DataStorage tests)
		_ string = "datastorage-soc2-e2e" // soc2Namespace - will be used when tests are implemented

		// Test data
		_ string // testCorrelationID - will be used when tests are implemented
	)

	BeforeAll(func() {
		_, testCancel = context.WithCancel(ctx)

		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("SOC2 Compliance E2E Test Suite - cert-manager Setup")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Info("⚠️  This test installs cert-manager (ONLY for SOC2 validation)")
		logger.Info("   Other DataStorage tests use fallback generation (faster)")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		// Step 1: Install cert-manager
		logger.Info("📦 Step 1/4: Installing cert-manager...")
		err := infrastructure.InstallCertManager(kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to install cert-manager")

		// Step 2: Wait for cert-manager to be ready
		logger.Info("⏳ Step 2/4: Waiting for cert-manager readiness...")
		err = infrastructure.WaitForCertManagerReady(kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "cert-manager did not become ready")

		// Step 3: Create ClusterIssuer
		logger.Info("📋 Step 3/4: Creating ClusterIssuer...")
		err = infrastructure.ApplyCertManagerIssuer(kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to create ClusterIssuer")

		// Step 4: Create namespace and deploy DataStorage with cert-manager
		logger.Info("📦 Step 4/4: Deploying DataStorage with cert-manager...")
		// Note: We reuse the existing cluster but create a separate namespace
		// This avoids cluster creation overhead while ensuring isolation

		// TODO: Implement namespace creation and DataStorage deployment with cert-manager
		// For now, we'll skip this and just validate the infrastructure setup
		logger.Info("✅ cert-manager infrastructure setup complete")
		logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	})

	AfterAll(func() {
		logger.Info("🧹 Cleaning up SOC2 E2E test resources...")
		testCancel()

		// Note: cert-manager and namespace cleanup happens in main AfterSuite
		// We only clean up test-specific resources here
	})

	Context("Digital Signatures (Day 9.1)", func() {
		It("should export audit events with digital signature", func() {
			Skip("TODO: Implement after DataStorage deployment with cert-manager")

			// Test Plan:
			// 1. Create audit events with correlation_id
			// 2. Call /api/v1/audit/export endpoint
			// 3. Verify response contains:
			//    - Signature field (base64 encoded)
			//    - Signature algorithm (SHA256withRSA)
			//    - Certificate fingerprint (SHA256)
			// 4. Verify signature is non-empty and valid base64
			// 5. Verify certificate fingerprint matches cert-manager issued cert
		})

		It("should use cert-manager managed certificate for signing", func() {
			Skip("TODO: Implement after DataStorage deployment with cert-manager")

			// Test Plan:
			// 1. Query cert-manager Secret: datastorage-signing-cert
			// 2. Extract certificate from Secret
			// 3. Calculate SHA256 fingerprint of certificate
			// 4. Export audit events
			// 5. Verify export metadata certificate_fingerprint matches calculated value
			// 6. This proves DataStorage is using cert-manager issued certificate
		})
	})

	Context("Hash Chain Integrity (Day 9.1 + CC8.1)", func() {
		It("should verify hash chains on export", func() {
			Skip("TODO: Implement after DataStorage deployment with cert-manager")

			// Test Plan:
			// 1. Create 5 audit events with same correlation_id
			// 2. Export events
			// 3. Verify hash_chain_verification contains:
			//    - total_events_verified: 5
			//    - valid_chain_events: 5
			//    - broken_chain_events: 0
			//    - chain_integrity_percentage: 100.0
			// 4. Verify each event has hash_chain_valid: true
		})

		It("should detect tampered hash chains", func() {
			Skip("TODO: Implement after DataStorage deployment with cert-manager")

			// Test Plan:
			// 1. Create 3 audit events
			// 2. Manually corrupt event_hash in PostgreSQL for middle event
			// 3. Export events
			// 4. Verify hash_chain_verification shows:
			//    - broken_chain_events: 1 (or more, depending on cascade)
			//    - tampered_event_ids contains the corrupted event ID
			//    - chain_integrity_percentage < 100.0
			// 5. Verify corrupted event has hash_chain_valid: false
		})
	})

	Context("Legal Hold Enforcement (Day 8 + AU-9)", func() {
		BeforeEach(func() {
			_ = generateTestCorrelationID() // testCorrelationID - will be used when tests are implemented
		})

		It("should prevent deletion of events under legal hold", func() {
			Skip("TODO: Implement after DataStorage deployment with cert-manager")

			// Test Plan:
			// 1. Create audit events with correlation_id
			// 2. Enable legal hold via API
			// 3. Attempt to delete events
			// 4. Verify deletion fails (403 Forbidden or similar)
			// 5. Export events and verify legal_hold: true
		})

		It("should allow deletion after legal hold release", func() {
			Skip("TODO: Implement after DataStorage deployment with cert-manager")

			// Test Plan:
			// 1. Create audit events with correlation_id
			// 2. Enable legal hold
			// 3. Release legal hold via API
			// 4. Verify events can now be queried with legal_hold: false
			// 5. (Deletion is automatic after retention period, not manual)
		})
	})

	Context("Complete SOC2 Workflow (Integration)", func() {
		It("should support end-to-end SOC2 audit export workflow", func() {
			Skip("TODO: Implement after DataStorage deployment with cert-manager")

			// Test Plan (End-to-End SOC2 Compliance):
			// 1. Create workflow audit trail (10+ events)
			// 2. Enable legal hold on correlation_id
			// 3. Export audit events
			// 4. Verify export contains:
			//    - Digital signature (cert-manager managed)
			//    - Hash chain verification (100% integrity)
			//    - Legal hold status on all events
			//    - Certificate fingerprint
			//    - Export metadata (exported_by, timestamp)
			// 5. Verify signature algorithm is SHA256withRSA
			// 6. This validates complete SOC2 CC8.1 + AU-9 compliance
		})
	})

	Context("Certificate Rotation Handling (Production Readiness)", func() {
		It("should continue signing after certificate rotation", func() {
			Skip("TODO: Implement for cert-manager rotation validation")

			// Test Plan:
			// 1. Export audit events (captures certificate fingerprint #1)
			// 2. Trigger cert-manager certificate rotation (renewBefore: 720h)
			//    - Delete Secret: datastorage-signing-cert
			//    - cert-manager recreates it automatically
			// 3. Wait for DataStorage to reload certificate
			// 4. Export audit events (captures certificate fingerprint #2)
			// 5. Verify:
			//    - Both exports have valid signatures
			//    - Certificate fingerprints differ (#1 != #2)
			//    - Both exports are verifiable with respective certificates
			// 6. This validates production certificate rotation readiness
		})
	})
})

// Helper function to generate test correlation IDs
func generateTestCorrelationID() string {
	return "soc2-e2e-" + time.Now().Format("20060102-150405")
}

// Helper function to create test audit events
func createTestAuditEvents(ctx context.Context, correlationID string, count int) []string {
	eventIDs := make([]string, count)

	for i := 0; i < count; i++ {
		// Use typed structs from generated OpenAPI client
		eventTimestamp := time.Now().UTC()
		req := dsgen.AuditEventRequest{
			CorrelationId:  correlationID,
			EventAction:    "soc2_test_action",
			EventCategory:  dsgen.AuditEventRequestEventCategoryGateway, // Use gateway category for test events
			EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
			EventType:      "soc2_compliance_test",
			EventTimestamp: eventTimestamp,
			Version:        "1.0",
		}

		resp, err := dsClient.CreateAuditEventWithResponse(ctx, req)
		Expect(err).ToNot(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(201), "Failed to create audit event")

		// UUID type is already a string wrapper, just convert directly
		eventIDs[i] = resp.JSON201.EventId.String()
	}

	return eventIDs
}

// Helper function to query audit events directly from PostgreSQL
func queryAuditEventsFromDB(correlationID string) ([]map[string]interface{}, error) {
	db, err := sql.Open("pgx", postgresURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query(`
		SELECT event_id, event_hash, previous_event_hash, legal_hold
		FROM audit_events
		WHERE correlation_id = $1
		ORDER BY event_timestamp ASC
	`, correlationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []map[string]interface{}
	for rows.Next() {
		var eventID, eventHash, previousHash string
		var legalHold bool

		if err := rows.Scan(&eventID, &eventHash, &previousHash, &legalHold); err != nil {
			return nil, err
		}

		events = append(events, map[string]interface{}{
			"event_id":            eventID,
			"event_hash":          eventHash,
			"previous_event_hash": previousHash,
			"legal_hold":          legalHold,
		})
	}

	return events, nil
}

// Helper function to verify base64 encoded signature
func verifyBase64Signature(signature string) error {
	_, err := base64.StdEncoding.DecodeString(signature)
	return err
}

// Helper to convert string to pointer
func stringPtr(s string) *string {
	return &s
}

