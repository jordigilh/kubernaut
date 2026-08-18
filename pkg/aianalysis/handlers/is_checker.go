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

package handlers

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
)

// terminalPhases are IS phases that indicate the session is no longer active.
var terminalPhases = map[isv1alpha1.SessionPhase]bool{
	isv1alpha1.SessionPhaseCompleted: true,
	isv1alpha1.SessionPhaseCancelled: true,
	isv1alpha1.SessionPhaseFailed:    true,
}

// ISFieldIndexRRName is the field index key for InvestigationSession's
// spec.remediationRequestRef.name used in AA MatchingFields queries.
const ISFieldIndexRRName = "spec.remediationRequestRef.name"

// K8sISPhaseUpdater implements ISPhaseUpdater by updating InvestigationSession
// CRD status via the controller-runtime client. AA calls SetTerminalPhase when
// the investigation finishes so AF's own IS bookkeeping reflects the outcome
// (#1376). DD-AA-KA-001 Amendment (Gap 1): this write-only, terminal-close
// call is the only remaining AA->IS interaction -- AF now determines
// readiness from AgentSession.Status.Interactive, not IS.Status.Phase.
type K8sISPhaseUpdater struct {
	client    client.Client
	namespace string
}

// NewK8sISPhaseUpdater creates an updater that transitions IS CRDs to a
// terminal phase in the given namespace. Uses the cached client
// (mgr.GetClient()) because the write path is less timing-sensitive than the
// read path.
func NewK8sISPhaseUpdater(c client.Client, namespace string) *K8sISPhaseUpdater {
	return &K8sISPhaseUpdater{client: c, namespace: namespace}
}

// SetTerminalPhase finds the non-terminal IS CRD for the given RR and sets its
// status.phase to the specified terminal phase. Best-effort: returns nil if no
// IS exists, if the IS is already terminal, or if the update fails. #1376.
func (u *K8sISPhaseUpdater) SetTerminalPhase(ctx context.Context, rrName string, phase isv1alpha1.SessionPhase) error {
	if rrName == "" {
		return nil
	}

	var list isv1alpha1.InvestigationSessionList
	if err := u.client.List(ctx, &list,
		client.InNamespace(u.namespace),
		client.MatchingFields{ISFieldIndexRRName: rrName},
	); err != nil {
		return fmt.Errorf("list InvestigationSessions for RR %s: %w", rrName, err)
	}

	for i := range list.Items {
		is := &list.Items[i]
		if terminalPhases[is.Status.Phase] {
			continue
		}
		is.Status.Phase = phase
		if err := u.client.Status().Update(ctx, is); err != nil {
			return fmt.Errorf("update IS %s phase to %s: %w", is.Name, phase, err)
		}
		return nil
	}
	return nil
}

// Compile-time interface assertion.
var _ ISPhaseUpdater = (*K8sISPhaseUpdater)(nil)
