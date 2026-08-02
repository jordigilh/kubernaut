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

// Package datastorage contains unit tests for the DataStorage service.
// BR-KA-016: Remediation history context for LLM prompt enrichment.
// DD-KA-016 v1.4: Both tiers query by spec hash for causal chain integrity (#586).
package datastorage_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

var _ = Describe("RemediationHistoryRepository", func() {
	var (
		mockDB  *sql.DB
		sqlMock sqlmock.Sqlmock
		repo    *repository.RemediationHistoryRepository
		logger  logr.Logger
		ctx     context.Context
	)

	BeforeEach(func() {
		var err error
		mockDB, sqlMock, err = sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		Expect(err).ToNot(HaveOccurred())

		logger = kubelog.NewLogger(kubelog.DevelopmentOptions())
		ctx = context.Background()

		repo = repository.NewRemediationHistoryRepository(mockDB, logger)
	})

	AfterEach(func() {
		Expect(sqlMock.ExpectationsWereMet()).To(Succeed())
		_ = mockDB.Close()
	})

	// =========================================================================
	// UT-RH-005 to UT-RH-008: QueryEffectivenessEventsBatch
	// BR-KA-016: Batch query EM component events by correlation_id
	// DD-KA-016 v1.1: Two-step query pattern
	// =========================================================================
	Describe("QueryEffectivenessEventsBatch", func() {
		Context("when EM events exist for correlation IDs", func() {
			It("UT-RH-005: should return events grouped by correlation_id", func() {
				eventData1, _ := json.Marshal(map[string]interface{}{
					"event_type": "effectiveness.health.assessed",
					"assessed":   true,
					"score":      0.8,
					"details":    "Pod running, readiness passing",
				})
				eventData2, _ := json.Marshal(map[string]interface{}{
					"event_type":                 "effectiveness.hash.computed",
					"pre_remediation_spec_hash":  "sha256:aabb1122",
					"post_remediation_spec_hash": "sha256:ccdd3344",
					"hash_match":                 false,
				})

				rows := sqlmock.NewRows([]string{
					"correlation_id", "event_type", "event_data",
				}).AddRow(
					"rr-abc-123", "effectiveness.health.assessed", eventData1,
				).AddRow(
					"rr-abc-123", "effectiveness.hash.computed", eventData2,
				)

				sqlMock.ExpectQuery(`SELECT correlation_id, event_type, event_data FROM audit_events`).
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				correlationIDs := []string{"rr-abc-123"}
				results, err := repo.QueryEffectivenessEventsBatch(ctx, correlationIDs)

				Expect(err).ToNot(HaveOccurred())
				Expect(results).To(HaveKey("rr-abc-123"))
				Expect(results["rr-abc-123"]).To(HaveLen(2))
			})
		})

		Context("when multiple correlation IDs have events", func() {
			It("UT-RH-006: should group events correctly per correlation_id", func() {
				eventDataA, _ := json.Marshal(map[string]interface{}{
					"event_type": "effectiveness.health.assessed",
					"assessed":   true,
					"score":      0.8,
				})
				eventDataB, _ := json.Marshal(map[string]interface{}{
					"event_type": "effectiveness.alert.assessed",
					"assessed":   true,
					"score":      0.0,
				})

				rows := sqlmock.NewRows([]string{
					"correlation_id", "event_type", "event_data",
				}).AddRow(
					"rr-abc-123", "effectiveness.health.assessed", eventDataA,
				).AddRow(
					"rr-def-456", "effectiveness.alert.assessed", eventDataB,
				)

				sqlMock.ExpectQuery(`SELECT correlation_id, event_type, event_data FROM audit_events`).
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				correlationIDs := []string{"rr-abc-123", "rr-def-456"}
				results, err := repo.QueryEffectivenessEventsBatch(ctx, correlationIDs)

				Expect(err).ToNot(HaveOccurred())
				Expect(results).To(HaveKey("rr-abc-123"))
				Expect(results).To(HaveKey("rr-def-456"))
				Expect(results["rr-abc-123"]).To(HaveLen(1))
				Expect(results["rr-def-456"]).To(HaveLen(1))
			})
		})

		Context("when event_data JSONB does not contain event_type", func() {
			It("UT-RH-013: should merge event_type column into EventData for downstream routing", func() {
				// RC-2 FIX: The audit_events table stores event_type as a column, not inside event_data JSONB.
				// BuildEffectivenessResponse (effectiveness_handler.go) routes on eventData["event_type"].
				// E2E tests (and potentially production) insert events where event_data omits event_type.
				// The repository MUST merge the column value into EventData for correct routing.
				eventDataWithoutType, _ := json.Marshal(map[string]interface{}{
					"assessed": true,
					"score":    0.85,
				})

				rows := sqlmock.NewRows([]string{
					"correlation_id", "event_type", "event_data",
				}).AddRow(
					"rr-rc2-test", "effectiveness.health.assessed", eventDataWithoutType,
				)

				sqlMock.ExpectQuery(`SELECT correlation_id, event_type, event_data FROM audit_events`).
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				correlationIDs := []string{"rr-rc2-test"}
				results, err := repo.QueryEffectivenessEventsBatch(ctx, correlationIDs)

				Expect(err).ToNot(HaveOccurred())
				Expect(results).To(HaveKey("rr-rc2-test"))
				Expect(results["rr-rc2-test"]).To(HaveLen(1))
				// Key assertion: event_type from column must be present in EventData
				Expect(results["rr-rc2-test"][0].EventData).To(HaveKeyWithValue("event_type", "effectiveness.health.assessed"),
					"event_type column must be merged into EventData for BuildEffectivenessResponse routing")
			})
		})

		// #211: ORDER BY must include event_id tiebreaker for deterministic ordering
		Context("deterministic ordering (#211)", func() {
			It("UT-DS-211-002: should order by event_timestamp ASC, event_id ASC", func() {
				rows := sqlmock.NewRows([]string{
					"correlation_id", "event_type", "event_data",
				})

				sqlMock.ExpectQuery(`ORDER BY event_timestamp ASC, event_id ASC`).
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				correlationIDs := []string{"rr-order-test"}
				results, err := repo.QueryEffectivenessEventsBatch(ctx, correlationIDs)

				Expect(err).ToNot(HaveOccurred())
				Expect(results).To(BeEmpty())
			})
		})

		Context("when no events exist for correlation IDs", func() {
			It("UT-RH-007: should return empty map", func() {
				rows := sqlmock.NewRows([]string{
					"correlation_id", "event_type", "event_data",
				})

				sqlMock.ExpectQuery(`SELECT correlation_id, event_type, event_data FROM audit_events`).
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				correlationIDs := []string{"rr-nonexistent"}
				results, err := repo.QueryEffectivenessEventsBatch(ctx, correlationIDs)

				Expect(err).ToNot(HaveOccurred())
				Expect(results).To(BeEmpty())
			})
		})

		Context("when database returns an error", func() {
			It("UT-RH-008: should propagate the error", func() {
				sqlMock.ExpectQuery(`SELECT correlation_id, event_type, event_data FROM audit_events`).
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnError(sql.ErrConnDone)

				correlationIDs := []string{"rr-abc-123"}
				results, err := repo.QueryEffectivenessEventsBatch(ctx, correlationIDs)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(sql.ErrConnDone))
				Expect(results).To(BeNil())
			})
		})
	})

	// =========================================================================
	// UT-RH-009 to UT-RH-012: QueryROEventsBySpecHash
	// BR-KA-016: Both Tier 1 and Tier 2 query by spec hash (DD-KA-016 v1.4, #586)
	// =========================================================================
	Describe("QueryROEventsBySpecHash", func() {
		var (
			targetResource string
			clusterID      string
			specHash       string
			since          time.Time
			until          time.Time
		)

		BeforeEach(func() {
			targetResource = "prod/Deployment/my-app"
			clusterID = ""
			specHash = "sha256:aabb1122"
			since = time.Now().Add(-90 * 24 * time.Hour) // 90 days ago
			until = time.Now().Add(-24 * time.Hour)       // 24h ago (beyond tier 1)
		})

		Context("when historical events match the spec hash", func() {
			It("UT-RH-009: should return RO events matching pre_remediation_spec_hash", func() {
				eventData, _ := json.Marshal(map[string]interface{}{
					"target_resource":           "prod/Deployment/my-app",
					"pre_remediation_spec_hash": "sha256:aabb1122",
					"action_type":             "ScaleUp",
				})

				rows := sqlmock.NewRows([]string{
					"event_type", "event_data", "event_timestamp", "correlation_id",
				}).AddRow(
					"remediation.workflow_created", eventData,
					time.Now().Add(-21*24*time.Hour), "rr-old-001",
				)

				sqlMock.ExpectQuery(`SELECT event_type, event_data, event_timestamp, correlation_id FROM`).
					WithArgs(targetResource, clusterID, specHash, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				results, err := repo.QueryROEventsBySpecHash(ctx, targetResource, clusterID, specHash, since, until)

				Expect(err).ToNot(HaveOccurred())
				Expect(results).To(HaveLen(1))
				Expect(results[0].CorrelationID).To(Equal("rr-old-001"))
				Expect(results[0].EventData["pre_remediation_spec_hash"]).To(Equal("sha256:aabb1122"))
			})
		})

		Context("when no historical events match", func() {
			It("UT-RH-010: should return empty slice", func() {
				rows := sqlmock.NewRows([]string{
					"event_type", "event_data", "event_timestamp", "correlation_id",
				})

				sqlMock.ExpectQuery(`SELECT event_type, event_data, event_timestamp, correlation_id FROM`).
					WithArgs(targetResource, clusterID, specHash, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				results, err := repo.QueryROEventsBySpecHash(ctx, targetResource, clusterID, specHash, since, until)

				Expect(err).ToNot(HaveOccurred())
				Expect(results).To(BeEmpty())
			})
		})

		Context("when database returns an error", func() {
			It("UT-RH-011: should propagate the error", func() {
				sqlMock.ExpectQuery(`SELECT event_type, event_data, event_timestamp, correlation_id FROM`).
					WithArgs(targetResource, clusterID, specHash, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnError(sql.ErrConnDone)

				results, err := repo.QueryROEventsBySpecHash(ctx, targetResource, clusterID, specHash, since, until)

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(sql.ErrConnDone))
				Expect(results).To(BeNil())
			})
		})

		Context("when row scanning fails", func() {
			It("UT-RH-012: should return error on malformed event_data", func() {
				rows := sqlmock.NewRows([]string{
					"event_type", "event_data", "event_timestamp", "correlation_id",
				}).AddRow(
					"remediation.workflow_created",
					[]byte(`{invalid json`),
					time.Now().Add(-21*24*time.Hour),
					"rr-bad-001",
				)

				sqlMock.ExpectQuery(`SELECT event_type, event_data, event_timestamp, correlation_id FROM`).
					WithArgs(targetResource, clusterID, specHash, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				results, err := repo.QueryROEventsBySpecHash(ctx, targetResource, clusterID, specHash, since, until)

				Expect(err).To(HaveOccurred())
				Expect(results).To(BeNil())
			})
		})

		// #211: ORDER BY must include event_id tiebreaker for deterministic ordering
		// when multiple events share the same event_timestamp.
		Context("deterministic ordering (#211)", func() {
			It("UT-DS-211-003: should order by event_timestamp ASC, event_id ASC", func() {
				rows := sqlmock.NewRows([]string{
					"event_type", "event_data", "event_timestamp", "correlation_id",
				})

				sqlMock.ExpectQuery(`ORDER BY event_timestamp ASC, event_id ASC`).
					WithArgs(targetResource, clusterID, specHash, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				results, err := repo.QueryROEventsBySpecHash(ctx, targetResource, clusterID, specHash, since, until)

				Expect(err).ToNot(HaveOccurred())
				Expect(results).To(BeEmpty())
			})
		})

		// GAP-DS-2: Query must filter by event_type to exclude EM events (effectiveness.hash.computed)
		// that also carry pre_remediation_spec_hash. Only remediation.workflow_created should be returned.
		Context("event_type filter (GAP-DS-2)", func() {
			It("UT-RH-013: should filter by event_type=remediation.workflow_created (BR-KA-016)", func() {
				eventData, _ := json.Marshal(map[string]interface{}{
					"target_resource":           "prod/Deployment/my-app",
					"pre_remediation_spec_hash": "sha256:aabb1122",
					"action_type":             "ScaleUp",
				})

				rows := sqlmock.NewRows([]string{
					"event_type", "event_data", "event_timestamp", "correlation_id",
				}).AddRow(
					"remediation.workflow_created", eventData,
					time.Now().Add(-21*24*time.Hour), "rr-old-002",
				)

				// Regex requires event_type filter - without it, query would leak EM events
				sqlMock.ExpectQuery(`event_type = 'remediation\.workflow_created'`).
					WithArgs(targetResource, clusterID, specHash, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				results, err := repo.QueryROEventsBySpecHash(ctx, targetResource, clusterID, specHash, since, until)

				Expect(err).ToNot(HaveOccurred())
				Expect(results).To(HaveLen(1))
				Expect(results[0].EventType).To(Equal("remediation.workflow_created"))
				Expect(results[0].CorrelationID).To(Equal("rr-old-002"))
			})
		})

		// =====================================================================
		// UT-DS-1802-001: Target-resource and cluster scoping (Issue #1802)
		// BR-ORCH-042.5: Ineffective-chain detection must not cross target
		// resource boundaries. Two unrelated Deployments sharing an identical
		// Pod spec (and thus an identical spec hash) must not be treated as
		// the same remediation target.
		// =====================================================================
		Context("target-resource scoping (Issue #1802)", func() {
			It("UT-DS-1802-001: should exclude a same-hash row belonging to a different target resource", func() {
				// Only the ns-a row should be returned when querying for ns-a's target,
				// even though an ns-b row shares the exact same spec hash.
				eventData, _ := json.Marshal(map[string]interface{}{
					"target_resource":           "ns-a/Deployment/app",
					"pre_remediation_spec_hash": specHash,
					"action_type":              "ScaleUp",
				})

				rows := sqlmock.NewRows([]string{
					"event_type", "event_data", "event_timestamp", "correlation_id",
				}).AddRow(
					"remediation.workflow_created", eventData,
					time.Now().Add(-21*24*time.Hour), "rr-ns-a-001",
				)

				sqlMock.ExpectQuery(`event_data->>'target_resource' = \$1`).
					WithArgs("ns-a/Deployment/app", "", specHash, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				results, err := repo.QueryROEventsBySpecHash(ctx, "ns-a/Deployment/app", "", specHash, since, until)

				Expect(err).ToNot(HaveOccurred())
				Expect(results).To(HaveLen(1), "only the requested target's row should be returned, not the cross-namespace ns-b row sharing the same hash")
				Expect(results[0].CorrelationID).To(Equal("rr-ns-a-001"))
			})

			It("UT-DS-1802-001b: should scope by cluster_id when specified (main-only fleet isolation)", func() {
				eventData, _ := json.Marshal(map[string]interface{}{
					"target_resource":           "ns-a/Deployment/app",
					"pre_remediation_spec_hash": specHash,
					"action_type":              "ScaleUp",
				})

				rows := sqlmock.NewRows([]string{
					"event_type", "event_data", "event_timestamp", "correlation_id",
				}).AddRow(
					"remediation.workflow_created", eventData,
					time.Now().Add(-21*24*time.Hour), "rr-cluster-a-001",
				)

				sqlMock.ExpectQuery(`cluster_id = \$2`).
					WithArgs("ns-a/Deployment/app", "cluster-a", specHash, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				results, err := repo.QueryROEventsBySpecHash(ctx, "ns-a/Deployment/app", "cluster-a", specHash, since, until)

				Expect(err).ToNot(HaveOccurred())
				Expect(results).To(HaveLen(1), "only cluster-a's row should be returned when clusterID is specified")
				Expect(results[0].CorrelationID).To(Equal("rr-cluster-a-001"))
			})

			It("UT-DS-1802-001c: should not filter by cluster when clusterID is empty (backward-compatible / release/v1.5 semantics)", func() {
				eventData, _ := json.Marshal(map[string]interface{}{
					"target_resource":           "ns-a/Deployment/app",
					"pre_remediation_spec_hash": specHash,
					"action_type":              "ScaleUp",
				})

				rows := sqlmock.NewRows([]string{
					"event_type", "event_data", "event_timestamp", "correlation_id",
				}).AddRow(
					"remediation.workflow_created", eventData,
					time.Now().Add(-21*24*time.Hour), "rr-any-cluster-001",
				)

				sqlMock.ExpectQuery(`\(\$2 = '' OR cluster_id = \$2\)`).
					WithArgs("ns-a/Deployment/app", "", specHash, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnRows(rows)

				results, err := repo.QueryROEventsBySpecHash(ctx, "ns-a/Deployment/app", "", specHash, since, until)

				Expect(err).ToNot(HaveOccurred())
				Expect(results).To(HaveLen(1), "empty clusterID must not exclude any cluster (unscoped, matches release/v1.5 behavior)")
			})
		})
	})
})
