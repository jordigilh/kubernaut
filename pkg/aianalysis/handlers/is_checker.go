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

// K8sInvestigationSessionChecker implements InvestigationSessionChecker by
// querying InvestigationSession CRDs via a direct API reader (bypassing the
// informer cache) using field selectors. BR-INTERACTIVE-010.
//
// The checker uses a direct API reader rather than the cached client to avoid
// missing recently created IS CRDs due to informer sync delay. This is critical
// because the IS CRD is created by AF immediately after the RR, and the AA
// controller must detect it on the very first reconcile to set interactive=true.
//
// HasActiveSession checks for IS CRD existence in any non-terminal phase
// (including empty/unset phase for freshly created CRDs). The phase lifecycle
// is owned by AA — AF creates the IS without a phase, and AA sets it to Active
// after acknowledging the interactive session.
//
// Error policy: callers use fail-open semantics — on list errors, investigations
// proceed in autonomous mode rather than blocking. This is deliberate: transient
// K8s API failures must not prevent autonomous investigations from running.
type K8sInvestigationSessionChecker struct {
	reader    client.Reader
	namespace string
}

// NewK8sInvestigationSessionChecker creates a checker that queries IS CRDs
// in the given namespace. The reader should be a direct API reader
// (mgr.GetAPIReader()) to bypass the informer cache.
func NewK8sInvestigationSessionChecker(r client.Reader, namespace string) *K8sInvestigationSessionChecker {
	return &K8sInvestigationSessionChecker{reader: r, namespace: namespace}
}

// terminalPhases are IS phases that indicate the session is no longer active.
// An IS in a terminal phase should not trigger interactive mode.
var terminalPhases = map[isv1alpha1.SessionPhase]bool{
	isv1alpha1.SessionPhaseCompleted: true,
	isv1alpha1.SessionPhaseCancelled: true,
	isv1alpha1.SessionPhaseFailed:    true,
}

// HasActiveSession returns true if a non-terminal InvestigationSession CRD
// exists for the given RR name. Non-terminal includes empty phase (freshly
// created by AF before AA has acknowledged it), Active, and Disconnected.
func (k *K8sInvestigationSessionChecker) HasActiveSession(ctx context.Context, rrName string) (bool, error) {
	if rrName == "" {
		return false, nil
	}

	var list isv1alpha1.InvestigationSessionList
	if err := k.reader.List(ctx, &list,
		client.InNamespace(k.namespace),
		client.MatchingFields{ISFieldIndexRRName: rrName},
	); err != nil {
		return false, fmt.Errorf("list InvestigationSessions for RR %s: %w", rrName, err)
	}

	for i := range list.Items {
		if !terminalPhases[list.Items[i].Status.Phase] {
			return true, nil
		}
	}
	return false, nil
}

// ISFieldIndexRRName is the field index key for InvestigationSession's
// spec.remediationRequestRef.name used in AA MatchingFields queries.
const ISFieldIndexRRName = "spec.remediationRequestRef.name"

// Compile-time interface assertion.
var _ InvestigationSessionChecker = (*K8sInvestigationSessionChecker)(nil)

// FindSessionPhase returns the phase of any InvestigationSession CRD for the given
// RR name, regardless of whether it is active or terminal. Returns ("", false, nil)
// if no IS exists at all. Used by handleSessionPollCancelled to distinguish "IS
// deleted" (user cancelled) from "IS in terminal phase" (completion race).
func (k *K8sInvestigationSessionChecker) FindSessionPhase(ctx context.Context, rrName string) (isv1alpha1.SessionPhase, bool, error) {
	if rrName == "" {
		return "", false, nil
	}

	var list isv1alpha1.InvestigationSessionList
	if err := k.reader.List(ctx, &list,
		client.InNamespace(k.namespace),
		client.MatchingFields{ISFieldIndexRRName: rrName},
	); err != nil {
		return "", false, fmt.Errorf("list InvestigationSessions for RR %s: %w", rrName, err)
	}

	if len(list.Items) == 0 {
		return "", false, nil
	}
	return list.Items[0].Status.Phase, true, nil
}

// K8sISPhaseUpdater implements ISPhaseUpdater by updating InvestigationSession
// CRD status via the controller-runtime client. AA calls SetActivePhase after
// submitting to KA with interactive=true to signal AF that the session is ready.
type K8sISPhaseUpdater struct {
	client    client.Client
	namespace string
}

// NewK8sISPhaseUpdater creates an updater that transitions IS CRDs to Active
// in the given namespace. Uses the cached client (mgr.GetClient()) because the
// write path is less timing-sensitive than the read path.
func NewK8sISPhaseUpdater(c client.Client, namespace string) *K8sISPhaseUpdater {
	return &K8sISPhaseUpdater{client: c, namespace: namespace}
}

// SetActivePhase finds the non-terminal IS CRD for the given RR and sets its
// status.phase to Active. Best-effort: returns nil if no IS exists or if the
// update fails (callers log but do not block on failure).
func (u *K8sISPhaseUpdater) SetActivePhase(ctx context.Context, rrName string) error {
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
		if terminalPhases[is.Status.Phase] || is.Status.Phase == isv1alpha1.SessionPhaseActive {
			continue
		}
		is.Status.Phase = isv1alpha1.SessionPhaseActive
		if err := u.client.Status().Update(ctx, is); err != nil {
			return fmt.Errorf("update IS %s phase to Active: %w", is.Name, err)
		}
		return nil
	}
	return nil
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
