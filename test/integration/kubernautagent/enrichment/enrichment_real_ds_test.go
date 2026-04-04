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

// Integration tests for Kubernaut Agent enrichment pipeline backed by
// real DataStorage (PostgreSQL + DS container) and real K8s (envtest).
//
// Business Requirements:
//   - BR-HAPI-016: Remediation history context for LLM prompt enrichment
//   - SOC2 CC8.1: Audit trail persistence
//
// Test Plan: docs/testing/TP-433-WIR-v1.0.md (IT-KA-433-ENR-001..006)
package enrichment_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

var _ = Describe("Kubernaut Agent Enrichment — Real DS + Real K8s (#433)", Label("integration", "enrichment"), func() {
	var (
		testCtx context.Context
		testID  string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		testID = uuid.New().String()[:8]
	})

	AfterEach(func() {
		_, _ = seedDB.ExecContext(testCtx,
			"DELETE FROM audit_events WHERE correlation_id LIKE $1",
			fmt.Sprintf("%%-%s%%", testID),
		)
	})

	// ============================================================================
	// Seed helpers — adapted from DS remediation_history_integration_test.go
	// ============================================================================

	insertAuditEvent := func(
		eventType, eventCategory, correlationID string,
		eventData map[string]interface{},
		ts time.Time,
	) {
		GinkgoHelper()
		eventDataJSON, err := json.Marshal(eventData)
		Expect(err).ToNot(HaveOccurred())

		_, err = seedDB.ExecContext(testCtx,
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
			uuid.New(), ts.Format("2006-01-02"), ts, eventType,
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

	insertEMEvents := func(correlationID, reason string, score float64, preHash, postHash string, ts time.Time) {
		GinkgoHelper()
		insertAuditEvent("effectiveness.health.assessed", "effectiveness", correlationID,
			map[string]interface{}{"assessed": true, "score": 0.85, "pod_running": true, "readiness_pass": true},
			ts.Add(1*time.Minute),
		)
		insertAuditEvent("effectiveness.alert.assessed", "effectiveness", correlationID,
			map[string]interface{}{"assessed": true, "score": 0.9, "signal_resolved": true},
			ts.Add(2*time.Minute),
		)
		insertAuditEvent("effectiveness.metrics.assessed", "effectiveness", correlationID,
			map[string]interface{}{"assessed": true, "score": 0.8, "cpu_before": 0.85, "cpu_after": 0.45},
			ts.Add(3*time.Minute),
		)
		insertAuditEvent("effectiveness.hash.computed", "effectiveness", correlationID,
			map[string]interface{}{"pre_remediation_spec_hash": preHash, "post_remediation_spec_hash": postHash},
			ts.Add(4*time.Minute),
		)
		insertAuditEvent("effectiveness.assessment.completed", "effectiveness", correlationID,
			map[string]interface{}{"reason": reason, "score": score},
			ts.Add(5*time.Minute),
		)
	}

	// ============================================================================
	// IT-KA-433-ENR-001: Full wiring chain — config to DS response + K8s owner chain
	// ============================================================================
	Describe("IT-KA-433-ENR-001: Full wiring chain — config to DS response + K8s owner chain", func() {
		It("should return remediation history from real DS and owner chain from real K8s", func() {
			target := fmt.Sprintf("it-enrichment/Pod/web-pod-1")
			corrID1 := fmt.Sprintf("ro-enr001a-%s", testID)
			corrID2 := fmt.Sprintf("ro-enr001b-%s", testID)
			now := time.Now().Add(-2 * time.Hour)

			By("Seeding 2 RO events + EM events in PostgreSQL")
			insertROEvent(corrID1, target, "sha256:pre1", "IncreaseMemory", now)
			insertEMEvents(corrID1, "full", 0.85, "sha256:pre1", "sha256:post1", now)
			insertROEvent(corrID2, target, "sha256:pre2", "RestartPod", now.Add(30*time.Minute))
			insertEMEvents(corrID2, "full", 0.90, "sha256:pre2", "sha256:post2", now.Add(30*time.Minute))

			By("Calling enricher with real infrastructure")
			result, err := enricher.Enrich(testCtx, "Pod", "web-pod-1", "it-enrichment", "sha256:pre1", "incident-enr001-"+testID)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.OwnerChain).To(HaveLen(2),
				"Pod -> ReplicaSet -> Deployment yields 2-entry chain (proves result non-nil)")

			By("Asserting remediation history from real DS")
			Expect(result.RemediationHistory.Tier1).To(HaveLen(2),
				"should return 2 remediation history Tier1 entries from real DS")

			By("Asserting owner chain from real K8s (envtest)")
			Expect(result.OwnerChain).To(HaveLen(2),
				"Pod -> ReplicaSet -> Deployment yields 2-entry chain")
			Expect(result.OwnerChain[0].Kind).To(Equal("ReplicaSet"))
			Expect(result.OwnerChain[0].Name).To(Equal("web-rs-abc"))
			Expect(result.OwnerChain[1].Kind).To(Equal("Deployment"))
			Expect(result.OwnerChain[1].Name).To(Equal("web-deploy"))
		})
	})

	// ============================================================================
	// IT-KA-433-ENR-003: Audit trace — enrichment.completed with structured EventData
	// ============================================================================
	Describe("IT-KA-433-ENR-003: Audit trace persistence — enrichment.completed", func() {
		It("should persist enrichment.completed with structured EventData to real DS", func() {
			target := fmt.Sprintf("it-enrichment/Pod/web-pod-1")
			corrID := fmt.Sprintf("ro-enr003-%s", testID)
			now := time.Now().Add(-1 * time.Hour)
			incidentID := "incident-enr003-" + testID

			By("Seeding remediation history")
			insertROEvent(corrID, target, "sha256:pre3", "IncreaseMemory", now)
			insertEMEvents(corrID, "full", 0.88, "sha256:pre3", "sha256:post3", now)

			By("Calling enricher (triggers audit event)")
			_, err := enricher.Enrich(testCtx, "Pod", "web-pod-1", "it-enrichment", "sha256:pre3", incidentID)
			Expect(err).ToNot(HaveOccurred())

			By("Querying audit_events table for enrichment.completed event")
			Eventually(func(g Gomega) {
				var eventDataRaw []byte
				err := seedDB.QueryRowContext(testCtx,
					`SELECT event_data FROM audit_events
					 WHERE event_type = 'aiagent.enrichment.completed'
					 AND event_data->>'incident_id' = $1
					 ORDER BY event_timestamp DESC LIMIT 1`,
					incidentID,
				).Scan(&eventDataRaw)
				g.Expect(err).ToNot(HaveOccurred(), "audit event should exist in PostgreSQL")

				var eventData map[string]interface{}
				g.Expect(json.Unmarshal(eventDataRaw, &eventData)).To(Succeed())
				g.Expect(eventData["root_owner_kind"]).To(Equal("Deployment"))
				g.Expect(eventData["root_owner_name"]).To(Equal("web-deploy"))
				g.Expect(eventData["incident_id"]).To(Equal(incidentID))
				g.Expect(eventData["remediation_history_fetched"]).To(BeTrue())
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())
		})
	})

	// ============================================================================
	// IT-KA-433-ENR-004: Audit trace — enrichment.failed with structured EventData
	// ============================================================================
	Describe("IT-KA-433-ENR-004: Audit trace persistence — enrichment.failed", func() {
		It("should persist enrichment.failed with structured EventData when both clients fail", func() {
			incidentID := "incident-enr004-" + testID

			By("Building a broken enricher (K8s error + DS error) with real audit store")
			brokenK8s := &errorK8sClient{err: errors.New("envtest unreachable")}
			brokenDS := &errorDSClient{err: errors.New("DS endpoint unreachable")}
			brokenEnricher := enrichment.NewEnricher(brokenK8s, brokenDS, auditStore,
				slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

			By("Calling broken enricher")
			result, err := brokenEnricher.Enrich(testCtx, "Pod", "broken-pod-"+testID, "it-enrichment", "", incidentID)
			Expect(err).ToNot(HaveOccurred(), "enricher handles failure gracefully")
			Expect(result.OwnerChain).To(BeEmpty(), "K8s failed so owner chain should be empty")
			Expect(result.RemediationHistory).To(BeNil(), "DS failed so history should be nil")

			By("Querying audit_events table for enrichment.failed event")
			Eventually(func(g Gomega) {
				var eventDataRaw []byte
				err := seedDB.QueryRowContext(testCtx,
					`SELECT event_data FROM audit_events
					 WHERE event_type = 'aiagent.enrichment.failed'
					 AND event_data->>'incident_id' = $1
					 ORDER BY event_timestamp DESC LIMIT 1`,
					incidentID,
				).Scan(&eventDataRaw)
				g.Expect(err).ToNot(HaveOccurred(), "enrichment.failed audit event should exist")

				var eventData map[string]interface{}
				g.Expect(json.Unmarshal(eventDataRaw, &eventData)).To(Succeed())
				g.Expect(eventData["reason"]).To(Equal("all_enrichment_sources_failed"))
				g.Expect(eventData["detail"]).To(ContainSubstring("envtest unreachable"))
				g.Expect(eventData["detail"]).To(ContainSubstring("DS endpoint unreachable"))
				g.Expect(eventData["affected_resource_kind"]).To(Equal("Pod"))
				g.Expect(eventData["incident_id"]).To(Equal(incidentID))
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())
		})
	})

	// ============================================================================
	// IT-KA-433-ENR-005: Partial failure — K8s fails, DS succeeds
	// ============================================================================
	Describe("IT-KA-433-ENR-005: Partial failure — K8s fails, DS succeeds", func() {
		It("should return DS history but empty owner chain, with enrichment.completed audit", func() {
			target := fmt.Sprintf("it-enrichment/StatefulSet/redis-%s", testID)
			corrID := fmt.Sprintf("ro-enr005-%s", testID)
			now := time.Now().Add(-1 * time.Hour)
			incidentID := "incident-enr005-" + testID

			By("Seeding remediation history for StatefulSet")
			insertROEvent(corrID, target, "sha256:pre5", "RestartPod", now)
			insertEMEvents(corrID, "full", 0.92, "sha256:pre5", "sha256:post5", now)

			By("Building enricher with broken K8s + real DS + real audit store")
			brokenK8s := &errorK8sClient{err: errors.New("K8s API unavailable")}
			dsAdapter := enrichment.NewDSAdapter(ogenClient)
			partialEnricher := enrichment.NewEnricher(brokenK8s, dsAdapter, auditStore,
				slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

			By("Calling enricher")
			result, err := partialEnricher.Enrich(testCtx, "StatefulSet", "redis-"+testID, "it-enrichment", "sha256:pre5", incidentID)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.OwnerChain).To(BeEmpty(), "K8s failed so owner chain should be empty")
			Expect(result.RemediationHistory.Tier1).To(HaveLen(1), "DS succeeded so history should have 1 Tier1 entry")

			By("Verifying audit event is enrichment.completed (not failed) with partial data")
			Eventually(func(g Gomega) {
				var eventDataRaw []byte
				err := seedDB.QueryRowContext(testCtx,
					`SELECT event_data FROM audit_events
					 WHERE event_type = 'aiagent.enrichment.completed'
					 AND event_data->>'incident_id' = $1
					 ORDER BY event_timestamp DESC LIMIT 1`,
					incidentID,
				).Scan(&eventDataRaw)
				g.Expect(err).ToNot(HaveOccurred())

				var eventData map[string]interface{}
				g.Expect(json.Unmarshal(eventDataRaw, &eventData)).To(Succeed())
				g.Expect(eventData["owner_chain_length"]).To(BeEquivalentTo(0))
				g.Expect(eventData["remediation_history_fetched"]).To(BeTrue())
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())
		})
	})

	// ============================================================================
	// IT-KA-433-ENR-006: Empty history from real DS
	// ============================================================================
	Describe("IT-KA-433-ENR-006: Empty history from real DS", func() {
		It("should return empty history (not nil) with enrichment.completed audit", func() {
			incidentID := "incident-enr006-" + testID
			ghostName := "ghost-" + testID

			By("Calling enricher for a target with no seeded data")
			result, err := enricher.Enrich(testCtx, "Deployment", ghostName, "it-enrichment", "sha256:none", incidentID)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RemediationHistory.Tier1).To(BeEmpty(), "no seeded data means empty Tier1 history")

			By("Verifying audit event is enrichment.completed with history_fetched=true")
			Eventually(func(g Gomega) {
				var eventDataRaw []byte
				err := seedDB.QueryRowContext(testCtx,
					`SELECT event_data FROM audit_events
					 WHERE event_type = 'aiagent.enrichment.completed'
					 AND event_data->>'incident_id' = $1
					 ORDER BY event_timestamp DESC LIMIT 1`,
					incidentID,
				).Scan(&eventDataRaw)
				g.Expect(err).ToNot(HaveOccurred())

				var eventData map[string]interface{}
				g.Expect(json.Unmarshal(eventDataRaw, &eventData)).To(Succeed())
				g.Expect(eventData["remediation_history_fetched"]).To(BeTrue(),
					"history fetch succeeded even though result is empty")
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())
		})
	})
})

// errorK8sClient always returns an error for GetOwnerChain.
type errorK8sClient struct {
	err error
}

func (e *errorK8sClient) GetOwnerChain(_ context.Context, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	return nil, e.err
}

func (e *errorK8sClient) GetSpecHash(_ context.Context, _, _, _ string) (string, error) {
	return "", e.err
}

// errorDSClient always returns an error for GetRemediationHistory.
type errorDSClient struct {
	err error
}

func (e *errorDSClient) GetRemediationHistory(_ context.Context, _, _, _, _ string) (*enrichment.RemediationHistoryResult, error) {
	return nil, e.err
}
