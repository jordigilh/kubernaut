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
	"time"

	"github.com/google/uuid"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
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
		It("IT-DS-016-004: EM scoring data (health, metrics, hash) is complete after repository query through adapter", func() {
			// Business outcome: EM scoring data queried from real PostgreSQL is complete
			// and available to the correlation engine after passing through the adapter.
			now := time.Now().UTC()
			cid := fmt.Sprintf("corr-adapter-%s", testID)
			insertEMEvents(cid, "full", 0.85, "sha256:pre_a", "sha256:post_a", now.Add(-1*time.Hour))

			adapter := server.NewRemediationHistoryRepoAdapter(rhRepo)

			// Act: query through adapter (same path as handler orchestration)
			result, err := adapter.QueryEffectivenessEventsBatch(testCtx, []string{cid})

			// Assert: all 5 EM component events available for scoring
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(HaveKey(cid))
			Expect(result[cid]).To(HaveLen(5), "All 5 EM component events must be available for scoring")

			// Verify hash data is complete (required for spec_drift detection)
			var foundHashEvent bool
			for _, event := range result[cid] {
				if preHash, ok := event.EventData["pre_remediation_spec_hash"]; ok {
					Expect(preHash).To(Equal("sha256:pre_a"))
					postHash, hasPost := event.EventData["post_remediation_spec_hash"]
					Expect(hasPost).To(BeTrue(), "post_remediation_spec_hash must be present for spec_drift detection")
					Expect(postHash).To(Equal("sha256:post_a"))
					foundHashEvent = true
				}
			}
			Expect(foundHashEvent).To(BeTrue(), "Hash event data must survive the repository-to-server adapter boundary")
		})
	})

	// ============================================================================
	// 3. Orchestration Pipeline Tests (direct business logic calls + real DB)
	//
	// Pattern: Queries real PostgreSQL via adapter, then calls CorrelateTier1Chain
	// and DetectRegression directly. NO HTTP layer (per TESTING_GUIDELINES.md
	// anti-pattern: HTTP Testing in Integration Tests).
	// ============================================================================

	Describe("Orchestration Pipeline (Direct Business Logic + Real DB)", func() {
		var (
			adapter server.RemediationHistoryQuerier
		)

		BeforeEach(func() {
			adapter = server.NewRemediationHistoryRepoAdapter(rhRepo)
		})

		// queryAndCorrelate reproduces the handler's orchestration pipeline
		// using direct business logic calls (no HTTP):
		//   1. QueryROEventsByTarget
		//   2. QueryEffectivenessEventsBatch (batch by correlation_id)
		//   3. CorrelateTier1Chain (correlation + scoring)
		//   4. DetectRegression (hash match analysis)
		queryAndCorrelate := func(target, specHash string, since time.Time) ([]api.RemediationHistoryEntry, bool) {
			GinkgoHelper()

			// Step 1: Query RO events by target
			roEvents, err := adapter.QueryROEventsByTarget(testCtx, target, since)
			Expect(err).ToNot(HaveOccurred())

			// Step 2: Batch query EM events
			var emEvents map[string][]*server.EffectivenessEvent
			if len(roEvents) > 0 {
				correlationIDs := make([]string, 0, len(roEvents))
				for _, ro := range roEvents {
					correlationIDs = append(correlationIDs, ro.CorrelationID)
				}
				emEvents, err = adapter.QueryEffectivenessEventsBatch(testCtx, correlationIDs)
				Expect(err).ToNot(HaveOccurred())
			}

			// Step 3: Correlate into Tier 1 entries
			entries := server.CorrelateTier1Chain(roEvents, emEvents, specHash)

			// Step 4: Detect regression
			regressionDetected := server.DetectRegression(entries)

			return entries, regressionDetected
		}

		It("IT-DS-016-005: Full pipeline with reason=full produces correct assessmentReason and positive weighted score", func() {
			// Business outcome: LLM receives accurate effectiveness data from real DB.
			now := time.Now().UTC()
			cid := fmt.Sprintf("corr-full-%s", testID)
			insertROEvent(cid, targetResource, currentSpecHash, "ScaleUp", now.Add(-2*time.Hour))
			insertEMEvents(cid, "full", 0.85, currentSpecHash, "sha256:post_full_"+testID, now.Add(-2*time.Hour))

			// Act: direct business logic pipeline (no HTTP)
			entries, _ := queryAndCorrelate(targetResource, currentSpecHash, now.Add(-24*time.Hour))

			// Assert
			Expect(entries).To(HaveLen(1), "Should have 1 Tier 1 entry")
			entry := entries[0]
			Expect(entry.AssessmentReason.Set).To(BeTrue(), "assessmentReason must be set")
			Expect(string(entry.AssessmentReason.Value)).To(Equal("full"))
			Expect(entry.EffectivenessScore.Set).To(BeTrue(), "effectivenessScore must be set")
			Expect(entry.EffectivenessScore.Value).To(BeNumerically(">", 0.0),
				"Full assessment should have a positive weighted score")
			Expect(entry.WorkflowType.Value).To(Equal("ScaleUp"))
		})

		It("IT-DS-016-006: Pipeline with reason=spec_drift produces assessmentReason=spec_drift and score=0.0", func() {
			// Business outcome: LLM correctly informed that spec_drift != failure.
			now := time.Now().UTC()
			cid := fmt.Sprintf("corr-drift-%s", testID)
			insertROEvent(cid, targetResource, currentSpecHash, "ScaleUp", now.Add(-2*time.Hour))
			insertEMEvents(cid, "spec_drift", 0.0, currentSpecHash, "sha256:post_drift_"+testID, now.Add(-2*time.Hour))

			// Act
			entries, _ := queryAndCorrelate(targetResource, currentSpecHash, now.Add(-24*time.Hour))

			// Assert
			Expect(entries).To(HaveLen(1))
			entry := entries[0]
			Expect(entry.AssessmentReason.Set).To(BeTrue())
			Expect(string(entry.AssessmentReason.Value)).To(Equal("spec_drift"))
			Expect(entry.EffectivenessScore.Set).To(BeTrue())
			Expect(entry.EffectivenessScore.Value).To(BeNumerically("==", 0.0),
				"spec_drift assessment should have score 0.0 (unreliable, not failure)")
		})

		It("IT-DS-016-007: Regression detected when currentSpecHash matches preRemediationSpecHash", func() {
			// Business outcome: LLM warned about configuration regression.
			now := time.Now().UTC()
			regressionHash := currentSpecHash // Same hash means config reverted
			cid := fmt.Sprintf("corr-regr-%s", testID)
			insertROEvent(cid, targetResource, regressionHash, "ScaleUp", now.Add(-2*time.Hour))
			insertEMEvents(cid, "full", 0.7, regressionHash, "sha256:post_regr_"+testID, now.Add(-2*time.Hour))

			// Act
			entries, regressionDetected := queryAndCorrelate(targetResource, currentSpecHash, now.Add(-24*time.Hour))

			// Assert
			Expect(entries).To(HaveLen(1))
			Expect(regressionDetected).To(BeTrue(),
				"Regression must be detected when currentSpecHash matches preRemediationSpecHash")
			Expect(entries[0].HashMatch.Value).To(Equal(api.RemediationHistoryEntryHashMatchPreRemediation),
				"HashMatch should indicate preRemediation match")
		})

		It("IT-DS-016-008: Non-existent target produces empty entries and no regression", func() {
			// Business outcome: Empty history gracefully handled (no errors, no false positives).
			now := time.Now().UTC()

			// Act: query for target with no events
			entries, regressionDetected := queryAndCorrelate(
				"ghost-ns/Deployment/nonexistent-"+testID,
				"sha256:no_such_hash",
				now.Add(-24*time.Hour),
			)

			// Assert
			Expect(entries).To(BeEmpty(), "Non-existent target should produce empty entries")
			Expect(regressionDetected).To(BeFalse(),
				"No regression when there are no events")
		})
	})
})
