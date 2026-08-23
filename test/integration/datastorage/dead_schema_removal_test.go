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

// Issue #2256: DataStorage: drop remaining dead schema (8 tables + 3 views,
// no code path).
//
// Follow-up to #623 (resource_action_traces / action_effectiveness_metrics).
// Migration 018 drops the remaining pre-ADR-034 per-action ML-style
// learning/effectiveness cluster: 8 tables, 3 dependent views, and 13
// stored functions with zero Go code path anywhere in the repo.
//
// These tests prove two business outcomes:
//  1. The dead objects are actually gone post-migration (not just "the
//     migration ran without erroring") -- IT-DS-2256-001..003.
//  2. SOC2 CC7.2 / BR-AUDIT-006 remediation-reconstruction from audit_events
//     alone is unaffected by the drop -- IT-DS-2256-004. This is the
//     regression guard: the live effectiveness scoring flow
//     (GetEffectivenessScore, ADR-EM-001 Principle 5, DD-017 v2.1) and the
//     RR reconstruction path both compute on demand from audit_events and
//     never touched this legacy cluster, so dropping it must not change
//     their behavior.
//
// Infrastructure: Real PostgreSQL from suite_test.go (db, logger, ctx),
// migrated through the full migrations/ chain (including 018) by
// SynchronizedBeforeSuite -- so these assertions run against the exact
// schema state production will have.
package datastorage

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// deadTables are the 8 tables (plus action_effectiveness_metrics, flagged
// dead by #623 but never dropped until now) migration 018 removes.
var deadTables = []string{
	"resource_references",
	"action_histories",
	"action_assessments",
	"effectiveness_results",
	"action_confidence_scores",
	"action_outcomes",
	"action_alternatives",
	"oscillation_patterns",
	"oscillation_detections",
	"action_effectiveness_metrics",
}

// deadViews are the 3 views that depended on the tables above.
var deadViews = []string{
	"effectiveness_trends",
	"low_confidence_actions",
	"oscillation_detection_summary",
}

// deadFunctions are the 13 stored functions orphaned by the tables above:
// the 10 named in issue #2256, plus 3 more (get_action_traces,
// get_resource_actions_base, get_recent_actions) found during preflight --
// already orphaned by migration 009 dropping resource_action_traces, but
// never cleaned up until now. All have the same zero-Go-reference profile.
var deadFunctions = []string{
	"create_assessment_for_action_trace",
	"analyze_action_oscillation",
	"detect_cascading_failures",
	"detect_ineffective_loops",
	"detect_resource_thrashing",
	"detect_scale_oscillation",
	"analyze_cascade_effects",
	"store_oscillation_detection",
	"get_action_effectiveness",
	"get_resource_id",
	"get_action_traces",
	"get_resource_actions_base",
	"get_recent_actions",
}

var _ = Describe("Issue #2256: Dead Schema Removal (Migration 018)", Label("integration", "migration", "2256"), func() {
	Context("IT-DS-2256-001: Legacy learning/effectiveness tables are gone", func() {
		It("should have zero of the 8 dead tables (+ action_effectiveness_metrics) in information_schema", func() {
			var present []string
			err := db.SelectContext(ctx, &present, `
				SELECT table_name FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = ANY($1)
			`, deadTables)
			Expect(err).ToNot(HaveOccurred())
			Expect(present).To(BeEmpty(),
				"migration 018 should have dropped all legacy learning/effectiveness tables; still present: %v", present)
		})
	})

	Context("IT-DS-2256-002: Dependent views are gone", func() {
		It("should have zero of the 3 dead views in information_schema", func() {
			var present []string
			err := db.SelectContext(ctx, &present, `
				SELECT table_name FROM information_schema.views
				WHERE table_schema = 'public' AND table_name = ANY($1)
			`, deadViews)
			Expect(err).ToNot(HaveOccurred())
			Expect(present).To(BeEmpty(),
				"migration 018 should have dropped all legacy learning/effectiveness views; still present: %v", present)
		})
	})

	Context("IT-DS-2256-003: Orphaned stored functions are gone", func() {
		It("should have zero of the 13 dead functions in information_schema (10 named in the issue + 3 found in preflight)", func() {
			var present []string
			err := db.SelectContext(ctx, &present, `
				SELECT routine_name FROM information_schema.routines
				WHERE routine_schema = 'public' AND routine_name = ANY($1)
			`, deadFunctions)
			Expect(err).ToNot(HaveOccurred())
			Expect(present).To(BeEmpty(),
				"migration 018 should have dropped all orphaned learning/effectiveness functions; still present: %v", present)
		})
	})

	Context("IT-DS-2256-004: SOC2 CC7.2 / BR-AUDIT-006 -- reconstruction from audit_events is unaffected", func() {
		var (
			auditRepo     *repository.AuditEventsRepository
			testID        string
			correlationID string
		)

		BeforeEach(func() {
			auditRepo = repository.NewAuditEventsRepository(db.DB, logger)
			testID = generateTestID()
			correlationID = fmt.Sprintf("test-2256-reconstruction-%s", testID)
		})

		AfterEach(func() {
			_, _ = db.ExecContext(ctx, "DELETE FROM audit_events WHERE correlation_id = $1", correlationID)
		})

		It("should reconstruct a remediation request purely from audit_events after the legacy cluster is dropped", func() {
			// ARRANGE: seed the same two-event chain used by BR-AUDIT-006's
			// existing reconstruction suite -- gateway signal + orchestrator
			// lifecycle -- to prove the reconstruction query path (which
			// never touched resource_references/action_histories/etc.)
			// still works unchanged.
			gatewayPayload := ogenclient.GatewayAuditPayload{
				EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewaySignalReceived,
				SignalType:  ogenclient.GatewayAuditPayloadSignalTypeAlert,
				SignalName:  "HighCPU",
				Namespace:   "default",
				Fingerprint: fmt.Sprintf("test-2256-fp-%s", testID),
			}
			gatewayEvent, err := CreateGatewaySignalReceivedEvent(correlationID, gatewayPayload)
			Expect(err).ToNot(HaveOccurred())
			_, err = auditRepo.Create(ctx, gatewayEvent)
			Expect(err).ToNot(HaveOccurred())

			orchestratorPayload := ogenclient.RemediationOrchestratorAuditPayload{
				EventType: ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleCreated,
				RrName:    fmt.Sprintf("test-2256-rr-%s", testID),
				Namespace: "default",
			}
			orchestratorEvent, err := CreateOrchestratorLifecycleCreatedEvent(correlationID, orchestratorPayload)
			Expect(err).ToNot(HaveOccurred())
			_, err = auditRepo.Create(ctx, orchestratorEvent)
			Expect(err).ToNot(HaveOccurred())

			// ACT: call the real reconstruction business logic (BR-AUDIT-006),
			// unchanged by this migration.
			events, err := reconstruction.QueryAuditEventsForReconstruction(ctx, db.DB, logger, correlationID)

			// ASSERT: complete lifecycle reconstructable from audit_events
			// alone -- proves migration 018 did not touch the SOC2 CC7.2
			// reconstruction path.
			Expect(err).ToNot(HaveOccurred(), "reconstruction query should succeed with the legacy cluster dropped")
			Expect(events).To(HaveLen(2), "both events in the correlation chain should be reconstructable")
			Expect(events[0].EventType).To(Equal("gateway.signal.received"))
			Expect(events[1].EventType).To(Equal("orchestrator.lifecycle.created"))
		})
	})
})
