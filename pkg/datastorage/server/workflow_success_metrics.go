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

package server

import (
	"context"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// overlaySuccessMetrics batch-fetches on-demand success-rate aggregates for
// every workflow in workflows and overlays them onto each workflow's
// TotalExecutions/SuccessfulExecutions/ActualSuccessRate fields (Issue #1661
// Change 7, DD-WORKFLOW-018).
//
// A workflow absent from the aggregation result (never executed) keeps its
// zero-value fields (TotalExecutions=0, ActualSuccessRate=nil) -- the same
// values GetByID/List's SQL scan already produced for a column that no
// longer exists.
//
// A query failure degrades gracefully: it is logged (never silent, GA
// Readiness Audit dimension 12 - Fail-Open Safety) but does not fail the
// request, because the workflow catalog data itself is correct and complete
// without the metrics; the metrics are best-effort supplementary telemetry.
// A nil h.successMetricsRepo (no audit DB wired, e.g. some unit tests) is
// likewise a no-op.
func (h *Handler) overlaySuccessMetrics(ctx context.Context, workflows []*models.RemediationWorkflow) {
	if h.successMetricsRepo == nil || len(workflows) == 0 {
		return
	}

	workflowIDs := make([]string, len(workflows))
	for i, wf := range workflows {
		workflowIDs[i] = wf.WorkflowID
	}

	metrics, err := h.successMetricsRepo.GetSuccessMetrics(ctx, workflowIDs)
	if err != nil {
		h.logger.Error(err, "failed to compute on-demand workflow success metrics; returning workflows without them",
			"workflow_count", len(workflows))
		return
	}

	for _, wf := range workflows {
		m, ok := metrics[wf.WorkflowID]
		if !ok {
			continue
		}
		wf.TotalExecutions = m.TotalExecutions
		wf.SuccessfulExecutions = m.SuccessfulExecutions
		wf.ActualSuccessRate = m.ActualSuccessRate
	}
}
