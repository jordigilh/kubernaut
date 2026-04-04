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

// DS Due Diligence: F1 — EM subquery timestamp constraint causes false negatives.
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// The bug: The EM subquery in QueryROEventsBySpecHash constrains EM events
// to the same time window as RO events. When a remediation completes near
// a tier boundary (RO in tier 2, EM assessment in tier 1), the query misses
// the correlation, producing a false negative.
//
// Infrastructure: Real PostgreSQL from suite_test.go (db, logger).
package datastorage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DS Due Diligence: F1 — EM Subquery Timestamp Constraint", Label("integration", "due-diligence", "F1"), func() {
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

	AfterEach(func() {
		_, _ = db.ExecContext(testCtx,
			"DELETE FROM audit_events WHERE correlation_id LIKE $1",
			fmt.Sprintf("%%-%s%%", testID),
		)
	})

	It("IT-DS-F1-001: QueryROEventsBySpecHash returns RO event when EM hash event is OUTSIDE the query time window", func() {
		now := time.Now().UTC()
		cid := fmt.Sprintf("corr-f1-001-%s", testID)
		preHash := "sha256:pre-f1-" + testID
		postHash := "sha256:post-f1-" + testID

		// RO event at T-25h: within tier 2 window [T-90d, T-24h]
		insertAuditEvent("remediation.workflow_created", "remediation", cid,
			map[string]interface{}{
				"target_resource":           targetResource,
				"pre_remediation_spec_hash": preHash,
				"action_type":              "RestartPod",
				"signal_type":              "HighCPULoad",
				"signal_fingerprint":       "fp-" + testID,
				"outcome":                  "success",
			},
			now.Add(-25*time.Hour),
		)

		// EM hash.computed at T-23h: OUTSIDE tier 2 window (T-23h > T-24h boundary)
		// but still has the postHash that should link to the RO event
		insertAuditEvent("effectiveness.hash.computed", "effectiveness", cid,
			map[string]interface{}{
				"pre_remediation_spec_hash":  preHash,
				"post_remediation_spec_hash": postHash,
				"hash_match":                false,
			},
			now.Add(-23*time.Hour),
		)

		// Query tier 2 window: [T-90d, T-24h]
		// The RO event at T-25h is within this window.
		// The EM event at T-23h is OUTSIDE this window.
		// Bug: EM subquery has timestamp filter, so it won't find the EM event,
		// and the RO event won't be returned via the post-hash path.
		tier2Since := now.Add(-90 * 24 * time.Hour)
		tier2Until := now.Add(-24 * time.Hour)

		rows, err := rhRepo.QueryROEventsBySpecHash(testCtx, postHash, tier2Since, tier2Until)

		Expect(err).ToNot(HaveOccurred())
		Expect(rows).To(HaveLen(1),
			"Should return 1 RO event found via EM correlation across tier boundary")
		Expect(rows[0].CorrelationID).To(Equal(cid))
	})
})
