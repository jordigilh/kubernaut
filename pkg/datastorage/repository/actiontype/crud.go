/*
Copyright 2025 Jordi Gil.

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

package actiontype

import (
	"context"
	"fmt"
)

// ActionTypeExists checks whether the given action type is active in the action_type_taxonomy table.
// DD-WORKFLOW-016 GAP-4: Explicit validation before DB FK constraint for clean 400 errors.
// BR-WORKFLOW-007: Disabled action types are not considered to exist for new workflow references.
//
// #1661 Phase A4: this is the sole surviving method on Repository --
// Create/GetByName/UpdateDescription/Disable/ForceDisable/CountActiveWorkflows/
// ListActive (and their result types/sentinel errors) were deleted alongside
// the createActionType/updateActionType/disableActionType handlers (Phase A3)
// and the CountActiveWorkflows->workflowCache port (Phase A1). ActionTypeExists
// survives because it is still consumed by HandleCreateWorkflow's
// validateActionType (RW-side handler, deferred to Phase B) via the
// server.ActionTypeValidator interface. Once Phase B deletes HandleCreateWorkflow,
// this method — and the whole actiontype repository package — becomes dead
// and should be deleted (tracked as part of Phase B/C scope).
func (r *Repository) ActionTypeExists(ctx context.Context, actionType string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM action_type_taxonomy WHERE action_type = $1 AND status = 'Active')",
		actionType,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("action type taxonomy lookup: %w", err)
	}
	return exists, nil
}
