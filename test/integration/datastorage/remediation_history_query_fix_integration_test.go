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

// Issue #616: Integration tests for QueryROEventsBySpecHash post-hash matching.
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// TP-616-v1.1: These tests validate the SQL query fix that expands
// QueryROEventsBySpecHash to match both pre_remediation_spec_hash and
// post_remediation_spec_hash (via EM correlation_id subquery).
//
// Infrastructure: Real PostgreSQL from suite_test.go (db, logger).
// Pattern: Same as remediation_history_integration_test.go — direct DB inserts + repository queries.
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

var _ = Describe("Issue #616: QueryROEventsBySpecHash Post-Hash Matching", Label("integration", "issue-616"), func() {
	var (
		rhRepo          *repository.RemediationHistoryRepository
		testCtx         context.Context
		testID          string
		targetResource  string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		testID = generateTestID()
		targetResource = fmt.Sprintf("default/Deployment/nginx-%s", testID)
		rhRepo = repository.NewRemediationHistoryRepository(db.DB, logger)
	})

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
				event_id, event_date, event_timestamp, event_type, event_version,
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

	insertROEvent := func(correlationID, target, preHash, actionType string, ts time.Time) {
		GinkgoHelper()
		insertAuditEvent("remediation.workflow_created", "remediation", correlationID,
			map[string]interface{}{
				"target_resource":           target,
				"pre_remediation_spec_hash": preHash,
				"action_type":              actionType,
				"signal_type":              "HighCPULoad",
				"signal_fingerprint":       "fp-" + testID,
				"outcome":                  "success",
			},
			ts,
		)
	}

	insertEMHashEvent := func(correlationID, preHash, postHash string, ts time.Time) {
		GinkgoHelper()
		insertAuditEvent("effectiveness.hash.computed", "effectiveness", correlationID,
			map[string]interface{}{
				"pre_remediation_spec_hash":  preHash,
				"post_remediation_spec_hash": postHash,
				"hash_match":                false,
			},
			ts,
		)
	}

	insertFullEMEvents := func(correlationID, preHash, postHash string, ts time.Time) {
		GinkgoHelper()
		insertAuditEvent("effectiveness.health.assessed", "effectiveness", correlationID,
			map[string]interface{}{"assessed": true, "score": 0.9},
			ts.Add(1*time.Minute),
		)
		insertAuditEvent("effectiveness.alert.assessed", "effectiveness", correlationID,
			map[string]interface{}{"assessed": true, "score": 0.85, "alert_resolution": map[string]interface{}{"alert_resolved": true}},
			ts.Add(2*time.Minute),
		)
		insertAuditEvent("effectiveness.metrics.assessed", "effectiveness", correlationID,
			map[string]interface{}{"assessed": true, "score": 0.8},
			ts.Add(3*time.Minute),
		)
		insertEMHashEvent(correlationID, preHash, postHash, ts.Add(4*time.Minute))
		insertAuditEvent("effectiveness.assessment.completed", "effectiveness", correlationID,
			map[string]interface{}{"reason": "full", "score": 0.85},
			ts.Add(5*time.Minute),
		)
	}

	AfterEach(func() {
		_, _ = db.ExecContext(testCtx,
			"DELETE FROM audit_events WHERE correlation_id LIKE $1",
			fmt.Sprintf("%%-%s%%", testID),
		)
	})

	It("IT-DS-616-001: QueryROEventsBySpecHash returns RO event when currentSpecHash matches post_remediation_spec_hash via EM correlation", func() {
		now := time.Now().UTC()
		cid := fmt.Sprintf("corr-616-001-%s", testID)
		preHash := "sha256:pre-001-" + testID
		postHash := "sha256:post-001-" + testID

		// RO event with pre_hash (does NOT match query hash)
		insertROEvent(cid, targetResource, preHash, "RestartPod", now.Add(-2*time.Hour))

		// EM hash event with post_hash (DOES match query hash) for same correlation_id
		insertEMHashEvent(cid, preHash, postHash, now.Add(-1*time.Hour))

		// Query with currentSpecHash=postHash: should find the RO event via post-hash subquery
		rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, postHash, now.Add(-3*time.Hour), now)

		Expect(err).ToNot(HaveOccurred())
		Expect(rows).To(HaveLen(1), "Should return 1 RO event found via post-hash EM correlation")
		Expect(rows[0].CorrelationID).To(Equal(cid))
		Expect(rows[0].EventData["pre_remediation_spec_hash"]).To(Equal(preHash))
	})

	It("IT-DS-616-002: QueryROEventsBySpecHash still returns RO events for pre-hash match (existing behavior)", func() {
		now := time.Now().UTC()
		cid := fmt.Sprintf("corr-616-002-%s", testID)
		preHash := "sha256:pre-002-" + testID

		// RO event with pre_hash (matches query hash)
		insertROEvent(cid, targetResource, preHash, "RestartPod", now.Add(-2*time.Hour))

		// Query with currentSpecHash=preHash: should find via pre-hash match
		rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, preHash, now.Add(-3*time.Hour), now)

		Expect(err).ToNot(HaveOccurred())
		Expect(rows).To(HaveLen(1), "Should return 1 RO event matching pre-hash")
		Expect(rows[0].CorrelationID).To(Equal(cid))
	})

	It("IT-DS-616-003: QueryROEventsBySpecHash returns union of pre-hash and post-hash matches for different correlation_ids", func() {
		now := time.Now().UTC()
		cidPre := fmt.Sprintf("corr-616-003-pre-%s", testID)
		cidPost := fmt.Sprintf("corr-616-003-post-%s", testID)
		targetHash := "sha256:target-003-" + testID
		otherHash := "sha256:other-003-" + testID

		// RO event 1: pre_hash=targetHash (direct match)
		insertROEvent(cidPre, targetResource, targetHash, "RestartPod", now.Add(-3*time.Hour))

		// RO event 2: pre_hash=otherHash (no direct match)
		insertROEvent(cidPost, targetResource, otherHash, "ScaleUp", now.Add(-2*time.Hour))

		// EM hash event for cidPost: post_hash=targetHash (should link this RO event to the query)
		insertEMHashEvent(cidPost, otherHash, targetHash, now.Add(-1*time.Hour))

		// Query with currentSpecHash=targetHash: should find both RO events
		rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, targetHash, now.Add(-4*time.Hour), now)

		Expect(err).ToNot(HaveOccurred())
		Expect(rows).To(HaveLen(2), "Should return 2 RO events: one from pre-hash, one from post-hash path")

		cids := []string{rows[0].CorrelationID, rows[1].CorrelationID}
		Expect(cids).To(ContainElements(cidPre, cidPost))
	})

	It("IT-DS-616-004: Full handler flow returns non-empty tier1.chain for post-hash scenario", func() {
		now := time.Now().UTC()
		cid := fmt.Sprintf("corr-616-004-%s", testID)
		preHash := "sha256:pre-004-" + testID
		postHash := "sha256:post-004-" + testID

		// RO event with pre_hash
		insertROEvent(cid, targetResource, preHash, "RestartPod", now.Add(-2*time.Hour))

		// Full EM events with post_hash for same correlation_id
		insertFullEMEvents(cid, preHash, postHash, now.Add(-1*time.Hour))

		// Query RO events by post-hash
		roRows, err := rhRepo.QueryROEventsBySpecHash(testCtx, postHash, now.Add(-3*time.Hour), now)
		Expect(err).ToNot(HaveOccurred())
		Expect(roRows).ToNot(BeEmpty(), "Should find RO events via post-hash")

		// Query EM events for correlation
		cids := make([]string, len(roRows))
		for i, row := range roRows {
			cids[i] = row.CorrelationID
		}
		emRows, err := rhRepo.QueryEffectivenessEventsBatch(testCtx, cids)
		Expect(err).ToNot(HaveOccurred())

		// Convert to EffectivenessEvent format
		emEvents := make(map[string][]*server.EffectivenessEvent)
		for cid, rows := range emRows {
			events := make([]*server.EffectivenessEvent, len(rows))
			for i, row := range rows {
				events[i] = &server.EffectivenessEvent{
					EventData: row.EventData,
				}
			}
			emEvents[cid] = events
		}

		// Correlate
		entries := server.CorrelateTier1Chain(roRows, emEvents, postHash)

		Expect(entries).ToNot(BeEmpty(), "tier1.chain should be non-empty for post-hash scenario")
		Expect(entries[0].RemediationUID).To(Equal(cid))
		Expect(entries[0].HashMatch.Set).To(BeTrue())
		Expect(entries[0].HashMatch.Value).To(Equal(api.RemediationHistoryEntryHashMatchPostRemediation))
	})
})
