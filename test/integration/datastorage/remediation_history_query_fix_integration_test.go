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

// Issues #616 and #2490: Integration tests for QueryROEventsBySpecHash
// dual-hash matching and complete causal-chain traversal.
//
// BR-KA-016: Remediation history context for LLM prompt enrichment.
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

var _ = Describe("Issue #616/#2490: QueryROEventsBySpecHash Causal Chain", Label("integration", "issue-616", "issue-2490"), func() {
	var (
		rhRepo         *repository.RemediationHistoryRepository
		testCtx        context.Context
		testID         string
		targetResource string
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
				"action_type":               actionType,
				"signal_type":               "HighCPULoad",
				"signal_fingerprint":        "fp-" + testID,
				"outcome":                   "success",
			},
			ts,
		)
	}

	// insertClusterROEvent is used to prove that cluster scoping applies to
	// recursive candidates, not only to the current-hash anchor.
	insertClusterROEvent := func(correlationID, target, clusterID, preHash, actionType string, ts time.Time) {
		GinkgoHelper()
		eventDataJSON, err := json.Marshal(map[string]interface{}{
			"target_resource":           target,
			"pre_remediation_spec_hash": preHash,
			"action_type":               actionType,
			"signal_type":               "HighCPULoad",
			"signal_fingerprint":        "fp-" + testID,
			"outcome":                   "success",
		})
		Expect(err).ToNot(HaveOccurred())

		_, err = db.ExecContext(testCtx,
			`INSERT INTO audit_events (
				event_id, event_date, event_timestamp, event_type, event_version,
				event_category, event_action, event_outcome, correlation_id,
				resource_type, resource_id, actor_id, actor_type,
				retention_days, is_sensitive, event_data, cluster_id
			) VALUES (
				$1, $2, $3, 'remediation.workflow_created', '1.0',
				'remediation', 'create', 'success', $4,
				'test', 'test', 'test', 'system',
				90, false, $5, $6
			)`,
			uuid.New(), ts.Format("2006-01-02"), ts, correlationID, eventDataJSON, clusterID,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to insert cluster-scoped RO audit event")
	}

	insertEMHashEvent := func(correlationID, preHash, postHash string, ts time.Time) {
		GinkgoHelper()
		insertAuditEvent("effectiveness.hash.computed", "effectiveness", correlationID,
			map[string]interface{}{
				"pre_remediation_spec_hash":  preHash,
				"post_remediation_spec_hash": postHash,
				"hash_match":                 false,
			},
			ts,
		)
	}

	insertClusterEMHashEvent := func(correlationID, clusterID, preHash, postHash string, ts time.Time) {
		GinkgoHelper()
		eventDataJSON, err := json.Marshal(map[string]interface{}{
			"pre_remediation_spec_hash":  preHash,
			"post_remediation_spec_hash": postHash,
			"hash_match":                 false,
		})
		Expect(err).ToNot(HaveOccurred())

		_, err = db.ExecContext(testCtx,
			`INSERT INTO audit_events (
				event_id, event_date, event_timestamp, event_type, event_version,
				event_category, event_action, event_outcome, correlation_id,
				resource_type, resource_id, actor_id, actor_type,
				retention_days, is_sensitive, event_data, cluster_id
			) VALUES (
				$1, $2, $3, 'effectiveness.hash.computed', '1.0',
				'effectiveness', 'assess', 'success', $4,
				'test', 'test', 'test', 'system',
				90, false, $5, $6
			)`,
			uuid.New(), ts.Format("2006-01-02"), ts, correlationID, eventDataJSON, clusterID,
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to insert cluster-scoped EM hash audit event")
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
			map[string]interface{}{"reason": "Full", "score": 0.85},
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
		rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, targetResource, "", postHash, now.Add(-3*time.Hour), now)

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
		rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, targetResource, "", preHash, now.Add(-3*time.Hour), now)

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
		rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, targetResource, "", targetHash, now.Add(-4*time.Hour), now)

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
		roRows, err := rhRepo.QueryROEventsBySpecHash(testCtx, targetResource, "", postHash, now.Add(-3*time.Hour), now)
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

	It("IT-DS-2490-001: QueryROEventsBySpecHash follows the complete chained pre/post hash history", func() {
		now := time.Now().UTC()
		currentHash := "sha256:current-005-" + testID
		hash1 := "sha256:post-1-005-" + testID
		hash0 := "sha256:post-0-005-" + testID
		initialHash := "sha256:initial-005-" + testID
		cid1 := fmt.Sprintf("corr-2490-001-1-%s", testID)
		cid2 := fmt.Sprintf("corr-2490-001-2-%s", testID)
		cid3 := fmt.Sprintf("corr-2490-001-3-%s", testID)

		// Three forward changes: initialHash -> hash0 -> hash1 -> currentHash.
		insertROEvent(cid1, targetResource, initialHash, "IncreaseMemoryLimits", now.Add(-3*time.Hour))
		insertEMHashEvent(cid1, initialHash, hash0, now.Add(-179*time.Minute))
		insertROEvent(cid2, targetResource, hash0, "IncreaseMemoryLimits", now.Add(-2*time.Hour))
		insertEMHashEvent(cid2, hash0, hash1, now.Add(-119*time.Minute))
		insertROEvent(cid3, targetResource, hash1, "IncreaseMemoryLimits", now.Add(-1*time.Hour))
		insertEMHashEvent(cid3, hash1, currentHash, now.Add(-59*time.Minute))

		rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, targetResource, "", currentHash, now.Add(-4*time.Hour), now)

		Expect(err).ToNot(HaveOccurred())
		Expect(rows).To(HaveLen(3), "the current hash must expose every prior linked remediation")
		Expect(rows[0].CorrelationID).To(Equal(cid1))
		Expect(rows[1].CorrelationID).To(Equal(cid2))
		Expect(rows[2].CorrelationID).To(Equal(cid3))
	})

	It("IT-DS-2490-002: Tier 2 traverses through a Tier 1 bridge while response windows remain disjoint", func() {
		now := time.Now().UTC()
		currentHash := "sha256:current-002-" + testID
		bridgeHash := "sha256:bridge-002-" + testID
		initialHash := "sha256:initial-002-" + testID
		olderCID := fmt.Sprintf("corr-2490-002-older-%s", testID)
		recentCID := fmt.Sprintf("corr-2490-002-recent-%s", testID)

		// The recent remediation is the bridge from the current hash to the
		// older remediation. The older row must be returned only in Tier 2.
		insertROEvent(olderCID, targetResource, initialHash, "IncreaseMemoryLimits", now.Add(-48*time.Hour))
		insertEMHashEvent(olderCID, initialHash, bridgeHash, now.Add(-47*time.Hour-59*time.Minute))
		insertROEvent(recentCID, targetResource, bridgeHash, "IncreaseMemoryLimits", now.Add(-12*time.Hour))
		insertEMHashEvent(recentCID, bridgeHash, currentHash, now.Add(-11*time.Hour-59*time.Minute))

		tier1Since := now.Add(-24 * time.Hour)
		tier1Rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, targetResource, "", currentHash, tier1Since, now)
		Expect(err).ToNot(HaveOccurred())
		Expect(tier1Rows).To(HaveLen(1), "the recent bridge must appear in Tier 1")
		Expect(tier1Rows[0].CorrelationID).To(Equal(recentCID))

		// The production handler queries Tier 2 through 'now' so the recursive
		// repository query can use the recent row as a bridge, then partitions
		// the response at the Tier 1 boundary. Reproduce that production
		// coordination directly, without introducing HTTP into this integration
		// test.
		tier2Since := now.Add(-72 * time.Hour)
		tier2AllRows, err := rhRepo.QueryROEventsBySpecHash(testCtx, targetResource, "", currentHash, tier2Since, now)
		Expect(err).ToNot(HaveOccurred())
		tier2Rows := make([]repository.RawAuditRow, 0, len(tier2AllRows))
		for _, row := range tier2AllRows {
			if row.EventTimestamp.Before(tier2Since) || !row.EventTimestamp.Before(tier1Since) {
				continue
			}
			tier2Rows = append(tier2Rows, row)
		}

		tier2Summaries := server.BuildTier2Summaries(tier2Rows, nil, currentHash)
		Expect(tier2Summaries).To(HaveLen(1), "the older linked remediation must appear only in Tier 2")
		Expect(tier2Summaries[0].RemediationUID).To(Equal(olderCID))
	})

	It("IT-DS-2490-003: follows only causal post-hash edges and excludes future, sibling, malformed, and cross-target links", func() {
		now := time.Now().UTC()
		currentHash := "sha256:current-003-" + testID
		bridgeHash := "sha256:bridge-003-" + testID
		initialHash := "sha256:initial-003-" + testID
		unrelatedHash := "sha256:unrelated-003-" + testID
		futureHash := "sha256:future-003-" + testID
		priorCID := fmt.Sprintf("corr-2490-003-prior-%s", testID)
		anchorCID := fmt.Sprintf("corr-2490-003-anchor-%s", testID)
		siblingCID := fmt.Sprintf("corr-2490-003-sibling-%s", testID)
		futureCID := fmt.Sprintf("corr-2490-003-future-%s", testID)
		malformedCID := fmt.Sprintf("corr-2490-003-malformed-%s", testID)
		otherTargetCID := fmt.Sprintf("corr-2490-003-other-target-%s", testID)

		// Only prior -> anchor is causal. The other rows deliberately exercise
		// edges that must not be treated as part of the chain:
		//   - sibling: same pre-hash, but no post-hash link to the anchor
		//   - future: post-hash link exists, but the RO row is newer than anchor
		//   - malformed: no EM post-hash link at all
		//   - other target: valid hash link, but a different target resource
		insertROEvent(priorCID, targetResource, initialHash, "IncreaseMemoryLimits", now.Add(-2*time.Hour))
		insertEMHashEvent(priorCID, initialHash, bridgeHash, now.Add(-119*time.Minute))
		insertROEvent(anchorCID, targetResource, bridgeHash, "IncreaseMemoryLimits", now.Add(-1*time.Hour))
		insertEMHashEvent(anchorCID, bridgeHash, currentHash, now.Add(-59*time.Minute))
		insertROEvent(siblingCID, targetResource, bridgeHash, "RestartPod", now.Add(-90*time.Minute))
		insertEMHashEvent(siblingCID, bridgeHash, unrelatedHash, now.Add(-89*time.Minute))
		insertROEvent(futureCID, targetResource, futureHash, "ScaleUp", now.Add(-30*time.Minute))
		insertEMHashEvent(futureCID, futureHash, bridgeHash, now.Add(-29*time.Minute))
		insertROEvent(malformedCID, targetResource, bridgeHash, "RestartPod", now.Add(-3*time.Hour))
		insertROEvent(otherTargetCID, "other/Deployment/nginx", initialHash, "IncreaseMemoryLimits", now.Add(-2*time.Hour-1*time.Minute))
		insertEMHashEvent(otherTargetCID, initialHash, bridgeHash, now.Add(-2*time.Hour))

		rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, targetResource, "", currentHash, now.Add(-4*time.Hour), now)

		Expect(err).ToNot(HaveOccurred())
		Expect(rows).To(HaveLen(2), "only the causal prior and anchor remediations belong to the chain")
		Expect([]string{rows[0].CorrelationID, rows[1].CorrelationID}).To(Equal([]string{priorCID, anchorCID}))
	})

	It("IT-DS-2490-004: deduplicates and terminates a cyclic hash chain", func() {
		now := time.Now().UTC()
		hashA := "sha256:cycle-a-004-" + testID
		hashB := "sha256:cycle-b-004-" + testID
		cidA := fmt.Sprintf("corr-2490-004-a-%s", testID)
		cidB := fmt.Sprintf("corr-2490-004-b-%s", testID)

		// A -> B -> A forms a cycle. Each event must appear once and the query
		// must return without recursive expansion of the same event identities.
		insertROEvent(cidA, targetResource, hashA, "IncreaseMemoryLimits", now.Add(-3*time.Hour))
		insertEMHashEvent(cidA, hashA, hashB, now.Add(-179*time.Minute))
		insertROEvent(cidB, targetResource, hashB, "IncreaseMemoryLimits", now.Add(-2*time.Hour))
		insertEMHashEvent(cidB, hashB, hashA, now.Add(-119*time.Minute))

		rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, targetResource, "", hashB, now.Add(-4*time.Hour), now)

		Expect(err).ToNot(HaveOccurred())
		Expect(rows).To(HaveLen(2))
		Expect([]string{rows[0].CorrelationID, rows[1].CorrelationID}).To(ConsistOf(cidA, cidB))
	})

	It("IT-DS-2490-005: applies cluster isolation to recursive candidates", func() {
		now := time.Now().UTC()
		currentHash := "sha256:current-005-" + testID
		bridgeHash := "sha256:bridge-005-" + testID
		initialHash := "sha256:initial-005-" + testID
		priorA := fmt.Sprintf("corr-2490-005-prior-a-%s", testID)
		anchorA := fmt.Sprintf("corr-2490-005-anchor-a-%s", testID)
		priorB := fmt.Sprintf("corr-2490-005-prior-b-%s", testID)
		wrongEMCluster := fmt.Sprintf("corr-2490-005-wrong-em-cluster-%s", testID)

		insertClusterROEvent(priorA, targetResource, "cluster-a", initialHash, "IncreaseMemoryLimits", now.Add(-2*time.Hour))
		insertClusterEMHashEvent(priorA, "cluster-a", initialHash, bridgeHash, now.Add(-119*time.Minute))
		insertClusterROEvent(anchorA, targetResource, "cluster-a", bridgeHash, "IncreaseMemoryLimits", now.Add(-1*time.Hour))
		insertClusterEMHashEvent(anchorA, "cluster-a", bridgeHash, currentHash, now.Add(-59*time.Minute))
		insertClusterROEvent(wrongEMCluster, targetResource, "cluster-a", bridgeHash, "IncreaseMemoryLimits", now.Add(-90*time.Minute))
		insertClusterEMHashEvent(wrongEMCluster, "cluster-b", "sha256:wrong-em-pre-005-"+testID, bridgeHash, now.Add(-89*time.Minute))
		insertClusterROEvent(priorB, targetResource, "cluster-b", initialHash, "IncreaseMemoryLimits", now.Add(-2*time.Hour-1*time.Minute))
		insertClusterEMHashEvent(priorB, "cluster-b", initialHash, bridgeHash, now.Add(-2*time.Hour))

		rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, targetResource, "cluster-a", currentHash, now.Add(-4*time.Hour), now)

		Expect(err).ToNot(HaveOccurred())
		Expect(rows).To(HaveLen(2), "recursive traversal must exclude the same-target candidate from another cluster")
		Expect([]string{rows[0].CorrelationID, rows[1].CorrelationID}).To(Equal([]string{priorA, anchorA}))
	})
})
