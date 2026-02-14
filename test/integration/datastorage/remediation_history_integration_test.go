/*
Copyright 2026 Jordi Gil.

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

// Integration tests for remediation history DataStorage layer.
//
// Business Requirements:
//   - BR-HAPI-016: Remediation history context for LLM prompt enrichment
//
// Design Decisions:
//   - DD-HAPI-016 v1.1: Two-step query pattern (RO events by target, EM events by correlation_id)
//   - DD-EM-002 v1.1: spec_drift assessment reason
//
// Test Plan: docs/testing/DD-HAPI-016/TEST_PLAN.md (IT-DS-016-001 through IT-DS-016-009)
//
// Infrastructure: Real PostgreSQL from suite_test.go (db, logger).
// Pattern: Same as hash_chain_db_round_trip_test.go — direct DB inserts + repository queries.
package datastorage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BR-HAPI-016: Remediation History Integration Tests (DD-HAPI-016 v1.1)", Label("integration", "remediation-history"), Ordered, func() {
	var (
		rhRepo          *repository.RemediationHistoryRepository
		testCtx         context.Context
		testID          string
		targetResource  string
		currentSpecHash string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		testID = generateTestID()
		targetResource = fmt.Sprintf("default/Deployment/nginx-%s", testID)
		currentSpecHash = "sha256:current_" + testID

		// Use public schema for cross-process queries
		usePublicSchema()

		// Create repository backed by the real PostgreSQL instance from suite_test.go
		rhRepo = repository.NewRemediationHistoryRepository(db.DB, logger)
	})

	// ============================================================================
	// Helper: insertAuditEvent inserts a single audit event into the real database.
	// Used by all tests to seed data for repository/adapter/handler queries.
	// ============================================================================
	insertAuditEvent := func(
		eventType string,
		eventCategory string,
		correlationID string,
		eventData map[string]interface{},
		eventTimestamp time.Time,
	) {
		GinkgoHelper()
		eventDataJSON, err := json.Marshal(eventData)
		Expect(err).ToNot(HaveOccurred())

		_, err = db.ExecContext(testCtx,
			`INSERT INTO audit_events (
				event_id, event_date, event_timestamp, event_type, version,
				event_category, event_action, event_outcome, correlation_id,
				resource_type, resource_id, actor_id, actor_type,
				retention_days, is_sensitive, event_data
			) VALUES (
				$1, $2, $3, $4, '1.0',
				$5, 'create', 'success', $6,
				'test', 'test', 'test', 'system',
				90, false, $7
			)`,
			uuid.New(), eventTimestamp.Format("2006-01-02"), eventTimestamp, eventType,
			eventCategory, correlationID, eventDataJSON,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to insert audit event: %s", eventType)
	}

	// ============================================================================
	// Helper: insertROEvent inserts a remediation.workflow_created audit event.
	// ============================================================================
	insertROEvent := func(
		correlationID string,
		target string,
		preHash string,
		workflowType string,
		ts time.Time,
	) {
		GinkgoHelper()
		insertAuditEvent("remediation.workflow_created", "remediation", correlationID,
			map[string]interface{}{
				"target_resource":            target,
				"pre_remediation_spec_hash":  preHash,
				"workflow_type":              workflowType,
				"signal_type":               "HighCPULoad",
				"signal_fingerprint":         "fp-" + testID,
				"outcome":                   "success",
			},
			ts,
		)
	}

	// ============================================================================
	// Helper: insertEMEvents inserts a full set of EM component events for scoring.
	// reason: "full", "spec_drift", "partial", etc.
	// ============================================================================
	insertEMEvents := func(
		correlationID string,
		reason string,
		score float64,
		preHash string,
		postHash string,
		ts time.Time,
	) {
		GinkgoHelper()

		// effectiveness.health.assessed
		insertAuditEvent("effectiveness.health.assessed", "effectiveness", correlationID,
			map[string]interface{}{
				"assessed":      true,
				"score":         0.85,
				"pod_running":   true,
				"readiness_pass": true,
			},
			ts.Add(1*time.Minute),
		)

		// effectiveness.alert.assessed
		insertAuditEvent("effectiveness.alert.assessed", "effectiveness", correlationID,
			map[string]interface{}{
				"assessed":         true,
				"score":            0.9,
				"signal_resolved":  true,
			},
			ts.Add(2*time.Minute),
		)

		// effectiveness.metrics.assessed
		insertAuditEvent("effectiveness.metrics.assessed", "effectiveness", correlationID,
			map[string]interface{}{
				"assessed":   true,
				"score":      0.8,
				"cpu_before": 0.85,
				"cpu_after":  0.45,
			},
			ts.Add(3*time.Minute),
		)

		// effectiveness.hash.computed
		insertAuditEvent("effectiveness.hash.computed", "effectiveness", correlationID,
			map[string]interface{}{
				"pre_remediation_spec_hash":  preHash,
				"post_remediation_spec_hash": postHash,
			},
			ts.Add(4*time.Minute),
		)

		// effectiveness.assessment.completed
		insertAuditEvent("effectiveness.assessment.completed", "effectiveness", correlationID,
			map[string]interface{}{
				"reason": reason,
				"score":  score,
			},
			ts.Add(5*time.Minute),
		)
	}

	AfterEach(func() {
		// Clean up all test data seeded by this test
		_, _ = db.ExecContext(testCtx,
			"DELETE FROM audit_events WHERE correlation_id LIKE $1",
			fmt.Sprintf("%%-%s%%", testID),
		)
	})

	// ============================================================================
	// 1. Repository Layer Tests
	// ============================================================================

	Describe("Repository Layer", func() {
		It("IT-DS-016-001: QueryROEventsByTarget returns RO events filtered by target and since", func() {
			// Arrange: insert 2 RO events for our target + 1 for a different target
			now := time.Now().UTC()
			cid1 := fmt.Sprintf("corr-ro-1-%s", testID)
			cid2 := fmt.Sprintf("corr-ro-2-%s", testID)
			cidOther := fmt.Sprintf("corr-ro-other-%s", testID)

			insertROEvent(cid1, targetResource, "sha256:hash1", "ScaleUp", now.Add(-2*time.Hour))
			insertROEvent(cid2, targetResource, "sha256:hash2", "RestartPod", now.Add(-1*time.Hour))
			insertROEvent(cidOther, "other/Deployment/other", "sha256:hashX", "ScaleUp", now.Add(-1*time.Hour))

			// Act: query with 3-hour lookback
			rows, err := rhRepo.QueryROEventsByTarget(testCtx, targetResource, now.Add(-3*time.Hour))

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(rows).To(HaveLen(2), "Should return exactly 2 RO events for our target")
			Expect(rows[0].CorrelationID).To(Equal(cid1))
			Expect(rows[1].CorrelationID).To(Equal(cid2))
			Expect(rows[0].EventData["workflow_type"]).To(Equal("ScaleUp"))
			Expect(rows[1].EventData["workflow_type"]).To(Equal("RestartPod"))
		})

		It("IT-DS-016-002: QueryEffectivenessEventsBatch groups EM events by correlation_id", func() {
			// Arrange: insert EM events for 3 correlation IDs
			now := time.Now().UTC()
			cids := []string{
				fmt.Sprintf("corr-em-1-%s", testID),
				fmt.Sprintf("corr-em-2-%s", testID),
				fmt.Sprintf("corr-em-3-%s", testID),
			}
			for i, cid := range cids {
				insertEMEvents(cid, "full", 0.85, "sha256:pre"+fmt.Sprintf("%d", i), "sha256:post"+fmt.Sprintf("%d", i), now.Add(time.Duration(-3+i)*time.Hour))
			}

			// Act
			result, err := rhRepo.QueryEffectivenessEventsBatch(testCtx, cids)

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveLen(3), "Should have entries for all 3 correlation IDs")
			for _, cid := range cids {
				Expect(result[cid]).To(HaveLen(5), "Each correlation ID should have 5 EM component events")
			}
		})

		It("IT-DS-016-003: QueryROEventsBySpecHash returns RO events by pre_remediation_spec_hash in time window", func() {
			// Arrange: insert events with matching and non-matching hashes
			now := time.Now().UTC()
			matchHash := "sha256:tier2_match_" + testID
			cidMatch1 := fmt.Sprintf("corr-hash-1-%s", testID)
			cidMatch2 := fmt.Sprintf("corr-hash-2-%s", testID)
			cidNoMatch := fmt.Sprintf("corr-hash-3-%s", testID)

			insertROEvent(cidMatch1, targetResource, matchHash, "ScaleUp", now.Add(-48*time.Hour))
			insertROEvent(cidMatch2, targetResource, matchHash, "RestartPod", now.Add(-30*time.Hour))
			insertROEvent(cidNoMatch, targetResource, "sha256:different_hash", "ScaleDown", now.Add(-40*time.Hour))

			// Act: query for matchHash within 72h-24h window (Tier 2)
			rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, matchHash, now.Add(-72*time.Hour), now.Add(-24*time.Hour))

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(rows).To(HaveLen(2), "Should return exactly 2 rows matching the spec hash")
			Expect(rows[0].CorrelationID).To(Equal(cidMatch1))
			Expect(rows[1].CorrelationID).To(Equal(cidMatch2))
		})
	})

	// ============================================================================
	// 2. Adapter Layer Tests
	// ============================================================================

	Describe("Adapter Layer", func() {
		It("IT-DS-016-004: Adapter converts EffectivenessEventRow to EffectivenessEvent losslessly", func() {
			// Arrange: insert EM events and create adapter
			now := time.Now().UTC()
			cid := fmt.Sprintf("corr-adapter-%s", testID)
			insertEMEvents(cid, "full", 0.85, "sha256:pre_a", "sha256:post_a", now.Add(-1*time.Hour))

			adapter := server.NewRemediationHistoryRepoAdapter(rhRepo)

			// Act: query through adapter
			result, err := adapter.QueryEffectivenessEventsBatch(testCtx, []string{cid})

			// Assert
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKey(cid))
			Expect(result[cid]).To(HaveLen(5), "Adapter should return all 5 EM events")

			// Verify event_data is preserved through the conversion
			var foundHashEvent bool
			for _, event := range result[cid] {
				if preHash, ok := event.EventData["pre_remediation_spec_hash"]; ok {
					Expect(preHash).To(Equal("sha256:pre_a"))
					foundHashEvent = true
				}
			}
			Expect(foundHashEvent).To(BeTrue(), "hash.computed event data should be preserved through adapter conversion")
		})
	})

	// ============================================================================
	// 3. Handler Layer Tests (httptest + real DB)
	// ============================================================================

	Describe("Handler Layer (HTTP + Real DB)", func() {
		var (
			handler *server.Handler
		)

		BeforeEach(func() {
			adapter := server.NewRemediationHistoryRepoAdapter(rhRepo)
			handler = server.NewHandler(nil,
				server.WithRemediationHistoryQuerier(adapter),
				server.WithLogger(logger),
			)
		})

		// makeRequest creates an httptest request for the remediation history endpoint
		makeRequest := func(params map[string]string) (*httptest.ResponseRecorder, map[string]interface{}) {
			GinkgoHelper()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation-history/context", nil)
			q := req.URL.Query()
			for k, v := range params {
				q.Set(k, v)
			}
			req.URL.RawQuery = q.Encode()

			rec := httptest.NewRecorder()
			handler.HandleGetRemediationHistoryContext(rec, req)

			var body map[string]interface{}
			if rec.Code == http.StatusOK {
				err := json.Unmarshal(rec.Body.Bytes(), &body)
				Expect(err).ToNot(HaveOccurred(), "Failed to parse JSON response")
			}
			return rec, body
		}

		It("IT-DS-016-005: Full pipeline with reason=full returns correct assessmentReason and weighted score", func() {
			// Arrange
			now := time.Now().UTC()
			cid := fmt.Sprintf("corr-full-%s", testID)
			insertROEvent(cid, targetResource, currentSpecHash, "ScaleUp", now.Add(-2*time.Hour))
			insertEMEvents(cid, "full", 0.85, currentSpecHash, "sha256:post_full_"+testID, now.Add(-2*time.Hour))

			// Act
			rec, body := makeRequest(map[string]string{
				"targetKind":      "Deployment",
				"targetName":      fmt.Sprintf("nginx-%s", testID),
				"targetNamespace": "default",
				"currentSpecHash": currentSpecHash,
				"tier1Window":     "24h",
			})

			// Assert
			Expect(rec.Code).To(Equal(http.StatusOK))

			tier1 := body["tier1"].(map[string]interface{})
			chain := tier1["chain"].([]interface{})
			Expect(chain).To(HaveLen(1), "Should have 1 Tier 1 entry")

			entry := chain[0].(map[string]interface{})
			Expect(entry).To(HaveKey("assessmentReason"))
			Expect(entry["assessmentReason"]).To(Equal("full"))
			Expect(entry).To(HaveKey("effectivenessScore"))
			score := entry["effectivenessScore"].(float64)
			Expect(score).To(BeNumerically(">", 0.0), "Full assessment should have a positive score")
		})

		It("IT-DS-016-006: Full pipeline with reason=spec_drift returns assessmentReason=spec_drift and score=0.0", func() {
			// Arrange
			now := time.Now().UTC()
			cid := fmt.Sprintf("corr-drift-%s", testID)
			insertROEvent(cid, targetResource, currentSpecHash, "ScaleUp", now.Add(-2*time.Hour))
			insertEMEvents(cid, "spec_drift", 0.0, currentSpecHash, "sha256:post_drift_"+testID, now.Add(-2*time.Hour))

			// Act
			rec, body := makeRequest(map[string]string{
				"targetKind":      "Deployment",
				"targetName":      fmt.Sprintf("nginx-%s", testID),
				"targetNamespace": "default",
				"currentSpecHash": currentSpecHash,
				"tier1Window":     "24h",
			})

			// Assert
			Expect(rec.Code).To(Equal(http.StatusOK))

			tier1 := body["tier1"].(map[string]interface{})
			chain := tier1["chain"].([]interface{})
			Expect(chain).To(HaveLen(1))

			entry := chain[0].(map[string]interface{})
			Expect(entry["assessmentReason"]).To(Equal("spec_drift"))
			// spec_drift hard-sets score to 0.0 in EM
			Expect(entry["effectivenessScore"]).To(BeNumerically("==", 0.0),
				"spec_drift assessment should have score 0.0 (unreliable)")
		})

		It("IT-DS-016-007: Regression detected when currentSpecHash matches preRemediationSpecHash", func() {
			// Arrange: insert RO event whose preHash == currentSpecHash (regression)
			now := time.Now().UTC()
			regressionHash := currentSpecHash // Same hash means config reverted
			cid := fmt.Sprintf("corr-regr-%s", testID)
			insertROEvent(cid, targetResource, regressionHash, "ScaleUp", now.Add(-2*time.Hour))
			insertEMEvents(cid, "full", 0.7, regressionHash, "sha256:post_regr_"+testID, now.Add(-2*time.Hour))

			// Act
			rec, body := makeRequest(map[string]string{
				"targetKind":      "Deployment",
				"targetName":      fmt.Sprintf("nginx-%s", testID),
				"targetNamespace": "default",
				"currentSpecHash": currentSpecHash,
				"tier1Window":     "24h",
			})

			// Assert
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(body["regressionDetected"]).To(BeTrue(),
				"Regression should be detected when currentSpecHash matches preRemediationSpecHash")
		})

		It("IT-DS-016-008: Invalid tier1Window returns 400 Bad Request", func() {
			// Act
			req := httptest.NewRequest(http.MethodGet, "/api/v1/remediation-history/context", nil)
			q := req.URL.Query()
			q.Set("targetKind", "Deployment")
			q.Set("targetName", "nginx")
			q.Set("targetNamespace", "default")
			q.Set("currentSpecHash", "sha256:abc")
			q.Set("tier1Window", "not-a-duration")
			req.URL.RawQuery = q.Encode()

			rec := httptest.NewRecorder()
			handler.HandleGetRemediationHistoryContext(rec, req)

			// Assert
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})

		It("IT-DS-016-009: Non-existent target returns 200 with empty chains", func() {
			// Act: query for a target that has no events
			rec, body := makeRequest(map[string]string{
				"targetKind":      "Deployment",
				"targetName":      "nonexistent-" + testID,
				"targetNamespace": "ghost-ns",
				"currentSpecHash": "sha256:no_such_hash",
				"tier1Window":     "24h",
			})

			// Assert
			Expect(rec.Code).To(Equal(http.StatusOK))
			tier1 := body["tier1"].(map[string]interface{})
			chain := tier1["chain"].([]interface{})
			Expect(chain).To(BeEmpty(), "Non-existent target should return empty Tier 1 chain")

			tier2 := body["tier2"].(map[string]interface{})
			t2chain := tier2["chain"].([]interface{})
			Expect(t2chain).To(BeEmpty(), "Non-existent target should return empty Tier 2 chain")

			Expect(body["regressionDetected"]).To(BeFalse(),
				"No regression when there are no events")
		})
	})
})
