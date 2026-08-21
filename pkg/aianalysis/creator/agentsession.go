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

// Package creator provides child CRD creation logic for the AIAnalysis
// controller, mirroring the pkg/remediationorchestrator/creator convention.
package creator

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
)

// AgentSessionCreator creates AgentSession CRDs from AIAnalysis, replacing
// the retired agentclient.SubmitInvestigation HTTP call (DD-AA-KA-001,
// BR-AA-KA-065.1). A narrow, single-purpose collaborator -- kept separate
// from InvestigatingHandler so the handler is never handed a raw
// client.Client, matching the existing AgentClientInterface/
// InvestigationSessionChecker/ISPhaseUpdater narrow-interface convention on
// that struct.
type AgentSessionCreator struct {
	client  client.Client
	scheme  *runtime.Scheme
	builder *handlers.RequestBuilder
}

// NewAgentSessionCreator creates a new AgentSessionCreator.
func NewAgentSessionCreator(c client.Client, s *runtime.Scheme) *AgentSessionCreator {
	return &AgentSessionCreator{
		client:  c,
		scheme:  s,
		builder: handlers.NewRequestBuilder(log.Log),
	}
}

// GetOrCreate returns the AgentSession for the given AIAnalysis, creating it
// if it does not already exist (BR-AA-KA-065.1). Idempotent: a second call
// for the same AIAnalysis returns the same object, no duplicate Create.
//
// The ownerRef is set to the AIAnalysis CR itself, not the RemediationRequest
// directly (DD-AA-KA-001 Decision section): AA already holds analysis live,
// so no new RemediationRequest RBAC grant is needed (AC-6). Cascade deletion
// still reaches the RR transitively, since RO already sets the RR as
// AIAnalysis's own owner.
func (c *AgentSessionCreator) GetOrCreate(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (*agentsessionv1.AgentSession, error) {
	logger := log.FromContext(ctx).WithValues(
		"aianalysis", analysis.Name,
		"namespace", analysis.Namespace,
	)

	if analysis.Spec.RemediationRequestRef.Name == "" {
		return nil, fmt.Errorf("cannot create AgentSession: AIAnalysis %s/%s has an empty RemediationRequestRef.Name", analysis.Namespace, analysis.Name)
	}

	name := fmt.Sprintf("as-%s", analysis.Spec.RemediationRequestRef.Name)

	existing := &agentsessionv1.AgentSession{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: analysis.Namespace}, existing)
	if err == nil {
		logger.V(1).Info("AgentSession already exists, reusing", "name", name)
		return existing, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check existing AgentSession: %w", err)
	}

	// Defensive: prevents an ownerless AgentSession if AIAnalysis isn't
	// fully persisted yet (mirrors creator.AIAnalysisCreator's RR.UID check).
	if analysis.UID == "" {
		return nil, fmt.Errorf("cannot set owner reference: AIAnalysis %s/%s has an empty UID", analysis.Namespace, analysis.Name)
	}

	as := &agentsessionv1.AgentSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: analysis.Namespace,
			// TerminalCloseFinalizer set synchronously here, not left to
			// AF's AgentSessionTerminalCloseReconciler to add reactively on
			// its own first reconcile (#2214 CI RCA, PR #2222): closes the
			// narrow bootstrap race where a delete (DeleteForCascadeCancel)
			// could otherwise land before that first reconcile observes
			// this Create, removing the object with no finalizer ever
			// attached. See the constant's doc comment for full detail.
			Finalizers: []string{agentsessionv1.TerminalCloseFinalizer},
		},
		Spec: c.builder.BuildAgentSessionSpec(analysis),
	}

	if err := controllerutil.SetControllerReference(analysis, as, c.scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := c.client.Create(ctx, as); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Concurrent create (e.g. two reconciles racing) -- re-fetch
			// rather than erroring, preserving idempotency.
			if getErr := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: analysis.Namespace}, existing); getErr == nil {
				return existing, nil
			}
		}
		return nil, fmt.Errorf("failed to create AgentSession: %w", err)
	}

	logger.Info("Created AgentSession", "name", name)
	return as, nil
}

// DeleteForRetry deletes as so that the next reconcile's GetOrCreate call
// naturally falls through to Create, giving the retry attempt a fresh
// AgentSession rather than mutating a terminal Failed object (BR-AI-009,
// DD-AA-KA-001 amendment: AgentSessionReasonCapacityExceeded retry path).
// Idempotent: a NotFound (e.g. a concurrent retry, or a delete already
// observed by another reconcile) is not an error.
//
// Explicitly removes agentsessionv1.TerminalCloseFinalizer before deleting
// (#2214 finalizer redesign): a capacity-exceeded retry is not a terminal
// outcome for the underlying investigation -- it immediately continues
// under the same rrName with a fresh AgentSession -- so it must NOT trigger
// AF's AgentSessionTerminalCloseReconciler's Cancelled-IS-closure the way
// DeleteForCascadeCancel intentionally does. This also keeps the delete
// synchronous (the deterministic as-<rrName> name must be free again
// immediately for GetOrCreate's same-named Create, which would otherwise
// collide with a terminating-but-not-yet-removed object of that name).
func (c *AgentSessionCreator) DeleteForRetry(ctx context.Context, as *agentsessionv1.AgentSession) error {
	if controllerutil.RemoveFinalizer(as, agentsessionv1.TerminalCloseFinalizer) {
		if err := c.client.Update(ctx, as); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to remove terminal-close finalizer from AgentSession %s/%s before retry delete: %w", as.Namespace, as.Name, err)
		}
	}
	if err := c.client.Delete(ctx, as); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete AgentSession %s/%s for retry: %w", as.Namespace, as.Name, err)
	}
	return nil
}

// DeleteForCascadeCancel deletes the deterministically-named AgentSession
// (as-<rrName>) for the given RemediationRequest on external cascade-cancel
// (#1421 ParentCancelled), replacing AA's retired direct IS write
// (K8sISPhaseUpdater.SetTerminalPhase) with an AgentSession delete (#2214 /
// DD-AA-KA-001 Amendment). This lets two independent, already-proven
// consumers react to the same signal without new coupling:
//   - AF's AgentSessionTerminalCloseReconciler (watching AgentSession)
//     closes the correlated InvestigationSession to Cancelled.
//   - KA's Dispatcher.cancelOnDelete stops the in-flight investigation
//     goroutine.
//
// Idempotent: a NotFound (e.g. the AgentSession was never created, or a
// concurrent cascade-cancel already deleted it) is not an error, mirroring
// DeleteForRetry's contract.
func (c *AgentSessionCreator) DeleteForCascadeCancel(ctx context.Context, rrName, namespace string) error {
	name := fmt.Sprintf("as-%s", rrName)
	as := &agentsessionv1.AgentSession{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	if err := c.client.Delete(ctx, as); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete AgentSession %s/%s for cascade-cancel: %w", namespace, name, err)
	}
	return nil
}
