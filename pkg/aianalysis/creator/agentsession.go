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
