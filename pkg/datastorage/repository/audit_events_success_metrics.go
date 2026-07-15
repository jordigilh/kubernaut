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

package repository

import (
	"context"
	"fmt"

	"github.com/lib/pq"
)

// ========================================
// ON-DEMAND WORKFLOW SUCCESS-RATE AGGREGATION (Issue #1661 Change 7)
// ========================================
// Authority: DD-WORKFLOW-018. Etcd is the single source of truth for
// RemediationWorkflow content; Postgres audit_events is the sole source for
// execution-outcome history. total_executions/successful_executions/
// actual_success_rate are no longer remediation_workflow_catalog columns
// updated by an UPDATE statement (the deleted UpdateSuccessMetrics) -- they
// are computed here, at query time, from the audit_events rows every
// WorkflowExecution reconciler already writes.
// ========================================

// WorkflowSuccessMetrics is the on-demand success-rate aggregate for one
// workflow_id, computed from audit_events. ActualSuccessRate is nil when
// TotalExecutions is zero (never executed) -- distinguishing "no data yet"
// from "0% success" the same way the nullable actual_success_rate catalog
// column did before Issue #1661 Change 7.
type WorkflowSuccessMetrics struct {
	TotalExecutions      int
	SuccessfulExecutions int
	ActualSuccessRate    *float64
}

// The two WorkflowExecution reconciler-emitted audit event types that
// determine execution outcome (BR-AUDIT-005, ADR-034). Any other event_type
// for the same workflow_id (e.g. an admission/registration audit event) is
// irrelevant to success-rate math. Named constants (rather than indexing
// into successMetricsEventTypes below) so GetSuccessMetrics' query args can't
// silently desync from "index 0 is the successful outcome" if this list is
// ever reordered or extended.
const (
	eventTypeWorkflowCompleted = "workflowexecution.workflow.completed"
	eventTypeWorkflowFailed    = "workflowexecution.workflow.failed"
)

// successMetricsEventTypes is the event_type = ANY(...) membership list for
// the query below.
var successMetricsEventTypes = []string{eventTypeWorkflowCompleted, eventTypeWorkflowFailed}

// GetSuccessMetrics aggregates audit_events into a per-workflow_id success
// rate for every ID in workflowIDs. A workflow_id with no matching
// audit_events rows (never executed) is absent from the returned map --
// callers must treat a missing key as TotalExecutions=0/ActualSuccessRate=nil,
// mirroring the nullable-column semantics UpdateSuccessMetrics used to write.
//
// BR-STORAGE-015: success-rate math (successful/total) must be correct so
// downstream workflow selection, which ranks by actual_success_rate, is not
// fed bad data.
func (r *AuditEventsRepository) GetSuccessMetrics(ctx context.Context, workflowIDs []string) (map[string]WorkflowSuccessMetrics, error) {
	if len(workflowIDs) == 0 {
		return map[string]WorkflowSuccessMetrics{}, nil
	}

	query := `
		SELECT
			event_data->>'workflow_id' AS workflow_id,
			COUNT(*) AS total_executions,
			COUNT(*) FILTER (WHERE event_type = $1) AS successful_executions
		FROM audit_events
		WHERE event_type = ANY($2)
			AND event_data->>'workflow_id' = ANY($3)
		GROUP BY event_data->>'workflow_id'
	`

	rows, err := r.db.QueryContext(ctx, query,
		eventTypeWorkflowCompleted,
		pq.Array(successMetricsEventTypes),
		pq.Array(workflowIDs),
	)
	if err != nil {
		r.logger.Error(err, "failed to aggregate workflow success metrics from audit_events")
		return nil, fmt.Errorf("failed to aggregate workflow success metrics: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]WorkflowSuccessMetrics, len(workflowIDs))
	for rows.Next() {
		var workflowID string
		var m WorkflowSuccessMetrics
		if err := rows.Scan(&workflowID, &m.TotalExecutions, &m.SuccessfulExecutions); err != nil {
			return nil, fmt.Errorf("failed to scan workflow success metrics row: %w", err)
		}
		if m.TotalExecutions > 0 {
			rate := float64(m.SuccessfulExecutions) / float64(m.TotalExecutions)
			m.ActualSuccessRate = &rate
		}
		result[workflowID] = m
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate workflow success metrics rows: %w", err)
	}

	return result, nil
}
