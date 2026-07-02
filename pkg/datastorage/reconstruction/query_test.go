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

package reconstruction_test

import (
	"context"
	"database/sql"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubelog "github.com/jordigilh/kubernaut/pkg/log"

	reconstructionpkg "github.com/jordigilh/kubernaut/pkg/datastorage/reconstruction"
)

// ========================================
// CHARACTERIZATION TESTS: QueryAuditEventsForReconstruction
// 📋 Business Requirement: BR-AUDIT-005 v2.0 (RR Reconstruction Support)
// 📋 Compliance: SOC2 CC8.1 — complete remediation-request reconstruction
// from audit traces via correlation_id.
//
// Written per GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3 (Phase 6, DataStorage)
// coverage-before-refactor mandate: this function (cyclomatic 35, the single
// highest DataStorage offender) had only integration-level coverage before
// this file. These tests pin down its current per-event-type decoding
// behavior so the planned decomposition (registry-style decoder dispatch,
// mirroring the buildEventData fix from Phase 4/Wave 1) is provably
// behavior-preserving.
// ========================================
var _ = Describe("QueryAuditEventsForReconstruction", func() {
	var (
		mockDB *sql.DB
		mock   sqlmock.Sqlmock
		ctx    context.Context
	)

	auditEventColumns := []string{
		"event_id", "event_version", "event_type", "event_category", "event_action",
		"correlation_id", "event_timestamp", "event_outcome", "severity",
		"resource_type", "resource_id", "actor_type", "actor_id", "parent_event_id",
		"event_data", "event_date", "namespace", "cluster_name",
		"duration_ms", "error_code", "error_message",
	}

	BeforeEach(func() {
		var err error
		mockDB, mock, err = sqlmock.New()
		Expect(err).ToNot(HaveOccurred())
		ctx = context.Background()
	})

	AfterEach(func() {
		_ = mockDB.Close()
	})

	It("returns an error without querying when the database connection is nil", func() {
		events, err := reconstructionpkg.QueryAuditEventsForReconstruction(ctx, nil, kubelog.NewLogger(kubelog.DefaultOptions()), "corr-nil-db")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("database connection is nil"))
		Expect(events).To(BeNil())
	})

	It("wraps and returns the underlying error when the query itself fails", func() {
		mock.ExpectQuery("SELECT").
			WithArgs("corr-query-fail").
			WillReturnError(sql.ErrConnDone)

		events, err := reconstructionpkg.QueryAuditEventsForReconstruction(ctx, mockDB, kubelog.NewLogger(kubelog.DefaultOptions()), "corr-query-fail")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to query audit events"))
		Expect(events).To(BeNil())
		Expect(mock.ExpectationsWereMet()).To(Succeed())
	})

	It("returns an empty, non-nil slice when no reconstruction-relevant events exist for the correlation ID", func() {
		mock.ExpectQuery("SELECT").
			WithArgs("corr-empty").
			WillReturnRows(sqlmock.NewRows(auditEventColumns))

		events, err := reconstructionpkg.QueryAuditEventsForReconstruction(ctx, mockDB, kubelog.NewLogger(kubelog.DefaultOptions()), "corr-empty")
		Expect(err).ToNot(HaveOccurred())
		Expect(events).To(BeEmpty())
	})

	DescribeTable("decodes each reconstruction-relevant event type into its typed payload variant",
		func(eventType string, eventDataJSON string) {
			ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			mock.ExpectQuery("SELECT").
				WithArgs("corr-"+eventType).
				WillReturnRows(sqlmock.NewRows(auditEventColumns).AddRow(
					"11111111-1111-1111-1111-111111111111", "1.0", eventType, "test", "action",
					"corr-"+eventType, ts, "success", nil,
					nil, nil, nil, nil, nil,
					[]byte(eventDataJSON), nil, nil, nil,
					nil, nil, nil,
				))

			events, err := reconstructionpkg.QueryAuditEventsForReconstruction(ctx, mockDB, kubelog.NewLogger(kubelog.DefaultOptions()), "corr-"+eventType)
			Expect(err).ToNot(HaveOccurred())
			Expect(events).To(HaveLen(1))
			Expect(events[0].EventType).To(Equal(eventType))

			gatewayPayload, isGateway := events[0].EventData.GetGatewayAuditPayload()
			orchestratorPayload, isOrchestrator := events[0].EventData.GetRemediationOrchestratorAuditPayload()
			aiPayload, isAI := events[0].EventData.GetAIAnalysisAuditPayload()
			wePayload, isWE := events[0].EventData.GetWorkflowExecutionAuditPayload()

			switch eventType {
			case "gateway.signal.received":
				Expect(isGateway).To(BeTrue())
				Expect(gatewayPayload).ToNot(BeZero())
			case "orchestrator.lifecycle.created", "orchestrator.lifecycle.completed", "orchestrator.lifecycle.failed":
				Expect(isOrchestrator).To(BeTrue())
				Expect(orchestratorPayload).ToNot(BeZero())
			case "aianalysis.analysis.completed":
				Expect(isAI).To(BeTrue())
				Expect(aiPayload).ToNot(BeZero())
			case "workflowexecution.selection.completed", "workflowexecution.execution.started":
				Expect(isWE).To(BeTrue())
				Expect(wePayload).ToNot(BeZero())
			}
		},
		Entry("gateway.signal.received", "gateway.signal.received",
			`{"event_type":"gateway.signal.received","signal_type":"alert","signal_name":"HighCPU","namespace":"default","fingerprint":"fp-1"}`),
		Entry("orchestrator.lifecycle.created", "orchestrator.lifecycle.created",
			`{"event_type":"orchestrator.lifecycle.created","rr_name":"rr-1","namespace":"default"}`),
		Entry("orchestrator.lifecycle.completed", "orchestrator.lifecycle.completed",
			`{"event_type":"orchestrator.lifecycle.completed","rr_name":"rr-1","namespace":"default"}`),
		Entry("orchestrator.lifecycle.failed", "orchestrator.lifecycle.failed",
			`{"event_type":"orchestrator.lifecycle.failed","rr_name":"rr-1","namespace":"default"}`),
		Entry("aianalysis.analysis.completed", "aianalysis.analysis.completed",
			`{"event_type":"aianalysis.analysis.completed","analysis_name":"aa-1","namespace":"default","phase":"Completed","approval_required":false,"degraded_mode":false,"warnings_count":0}`),
		Entry("workflowexecution.selection.completed", "workflowexecution.selection.completed",
			`{"event_type":"workflowexecution.selection.completed","workflow_id":"wf-1","workflow_version":"v1","target_resource":"default/pod-1","phase":"Pending","container_image":"img","execution_name":"we-1"}`),
		Entry("workflowexecution.execution.started", "workflowexecution.execution.started",
			`{"event_type":"workflowexecution.execution.started","workflow_id":"wf-1","workflow_version":"v1","target_resource":"default/pod-1","phase":"Running","container_image":"img","execution_name":"we-1"}`),
	)

	It("skips (does not error on) an unsupported event type reaching the scan, as defense-in-depth", func() {
		ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		mock.ExpectQuery("SELECT").
			WithArgs("corr-unsupported").
			WillReturnRows(sqlmock.NewRows(auditEventColumns).
				AddRow(
					"11111111-1111-1111-1111-111111111111", "1.0", "gateway.signal.received", "test", "action",
					"corr-unsupported", ts, "success", nil,
					nil, nil, nil, nil, nil,
					[]byte(`{"event_type":"gateway.signal.received","signal_type":"alert","signal_name":"HighCPU","namespace":"default","fingerprint":"fp-1"}`), nil, nil, nil,
					nil, nil, nil,
				).
				AddRow(
					"22222222-2222-2222-2222-222222222222", "1.0", "some.unsupported.type", "test", "action",
					"corr-unsupported", ts.Add(time.Second), "success", nil,
					nil, nil, nil, nil, nil,
					[]byte(`{}`), nil, nil, nil,
					nil, nil, nil,
				))

		events, err := reconstructionpkg.QueryAuditEventsForReconstruction(ctx, mockDB, kubelog.NewLogger(kubelog.DefaultOptions()), "corr-unsupported")
		Expect(err).ToNot(HaveOccurred())
		Expect(events).To(HaveLen(1), "the unsupported-type row must be skipped, not errored on")
		Expect(events[0].EventType).To(Equal("gateway.signal.received"))
	})

	It("wraps and returns an error when a row's event_data JSON is malformed for its declared event type", func() {
		ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		mock.ExpectQuery("SELECT").
			WithArgs("corr-bad-json").
			WillReturnRows(sqlmock.NewRows(auditEventColumns).AddRow(
				"11111111-1111-1111-1111-111111111111", "1.0", "gateway.signal.received", "test", "action",
				"corr-bad-json", ts, "success", nil,
				nil, nil, nil, nil, nil,
				[]byte(`{not-valid-json`), nil, nil, nil,
				nil, nil, nil,
			))

		events, err := reconstructionpkg.QueryAuditEventsForReconstruction(ctx, mockDB, kubelog.NewLogger(kubelog.DefaultOptions()), "corr-bad-json")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to unmarshal"))
		Expect(events).To(BeNil())
	})

	It("preserves chronological (timestamp-ascending) ordering across multiple event types for the same correlation ID", func() {
		t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		t1 := t0.Add(1 * time.Minute)
		mock.ExpectQuery("SELECT").
			WithArgs("corr-order").
			WillReturnRows(sqlmock.NewRows(auditEventColumns).
				AddRow(
					"11111111-1111-1111-1111-111111111111", "1.0", "gateway.signal.received", "test", "action",
					"corr-order", t0, "success", nil,
					nil, nil, nil, nil, nil,
					[]byte(`{"event_type":"gateway.signal.received","signal_type":"alert","signal_name":"HighCPU","namespace":"default","fingerprint":"fp-1"}`), nil, nil, nil,
					nil, nil, nil,
				).
				AddRow(
					"22222222-2222-2222-2222-222222222222", "1.0", "aianalysis.analysis.completed", "test", "action",
					"corr-order", t1, "success", nil,
					nil, nil, nil, nil, nil,
					[]byte(`{"event_type":"aianalysis.analysis.completed","analysis_name":"aa-1","namespace":"default","phase":"Completed","approval_required":false,"degraded_mode":false,"warnings_count":0}`), nil, nil, nil,
					nil, nil, nil,
				))

		events, err := reconstructionpkg.QueryAuditEventsForReconstruction(ctx, mockDB, kubelog.NewLogger(kubelog.DefaultOptions()), "corr-order")
		Expect(err).ToNot(HaveOccurred())
		Expect(events).To(HaveLen(2))
		Expect(events[0].EventType).To(Equal("gateway.signal.received"))
		Expect(events[1].EventType).To(Equal("aianalysis.analysis.completed"))
		Expect(events[0].EventTimestamp).To(BeTemporally("<", events[1].EventTimestamp))
	})
})

var _ = Describe("IsReconstructionRelevant", func() {
	It("returns true for each of the documented reconstruction-relevant event types", func() {
		for _, eventType := range []string{
			"gateway.signal.received",
			"aianalysis.analysis.completed",
			"workflowexecution.selection.completed",
			"workflowexecution.execution.started",
			"orchestrator.lifecycle.created",
			"orchestrator.lifecycle.completed",
			"orchestrator.lifecycle.failed",
		} {
			Expect(reconstructionpkg.IsReconstructionRelevant(eventType)).To(BeTrue(), eventType)
		}
	})

	It("returns false for an unrecognized event type", func() {
		Expect(reconstructionpkg.IsReconstructionRelevant("some.other.event")).To(BeFalse())
	})
})
