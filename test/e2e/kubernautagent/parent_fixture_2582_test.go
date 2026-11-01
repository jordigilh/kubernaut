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

package kubernautagent

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// agentSessionParentFixtureClient supplies the persisted RR -> AIAnalysis
// handoff required by direct AgentSession E2E fixtures. Production parent gates
// remain enabled; this wrapper only models the parent objects that AA would
// have created before handing the child to KA.
type agentSessionParentFixtureClient struct {
	client.Client
}

func (c *agentSessionParentFixtureClient) Create(ctx context.Context, object client.Object, opts ...client.CreateOption) error {
	as, ok := object.(*agentsessionv1.AgentSession)
	if !ok || as.Spec.RemediationRequestRef.Name == "" || as.Spec.IncidentID == "" || as.Spec.RemediationID == "" || metav1.GetControllerOf(as) != nil {
		return c.Client.Create(ctx, object, opts...)
	}

	analysis, err := infrastructure.EnsureAgentSessionParentsWaiting(
		ctx, c.Client, c.Scheme(), as.Namespace, as.Spec)
	if err != nil {
		return fmt.Errorf("seed direct AgentSession parent chain: %w", err)
	}
	// The AIAnalysis E2E suite also runs a real AIA controller cluster-wide.
	// Persist the direct handoff through a fresh read so an initial Pending
	// reconcile cannot overwrite the authorized Investigating phase.
	if err := persistDirectParentHandoff(ctx, c.Client, analysis, as.Name); err != nil {
		return fmt.Errorf("persist direct AgentSession investigation handoff: %w", err)
	}
	if err := controllerutil.SetControllerReference(analysis, as, c.Scheme()); err != nil {
		return fmt.Errorf("set direct AgentSession AIAnalysis owner: %w", err)
	}
	if err := c.Client.Create(ctx, object, opts...); err != nil {
		return err
	}
	if err := persistDirectParentHandoffUID(ctx, c.Client, analysis, as.UID); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("persist direct AgentSession recovery UID: parent AIAnalysis was deleted")
		}
		return fmt.Errorf("persist direct AgentSession recovery UID: %w", err)
	}
	if err := authorizeDirectParent(ctx, c.Client, as); err != nil {
		return fmt.Errorf("authorize direct AgentSession parent phase: %w", err)
	}
	return nil
}

func authorizeDirectParent(ctx context.Context, k8sClient client.Client, as *agentsessionv1.AgentSession) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		parent := &remediationv1.RemediationRequest{}
		key := client.ObjectKey{Name: as.Spec.RemediationRequestRef.Name, Namespace: as.Namespace}
		if err := k8sClient.Get(ctx, key, parent); err != nil {
			return err
		}
		parent.Status.OverallPhase = remediationv1.PhaseAnalyzing
		return k8sClient.Status().Update(ctx, parent)
	})
}

func persistDirectParentHandoff(ctx context.Context, k8sClient client.Client, analysis *aianalysisv1.AIAnalysis, agentSessionName string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current := &aianalysisv1.AIAnalysis{}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), current); err != nil {
			return err
		}
		current.Status.Phase = aianalysisv1.PhaseInvestigating
		current.Status.KASession = &aianalysisv1.KASession{
			AgentSessionRef: &corev1.ObjectReference{Name: agentSessionName, Namespace: current.Namespace},
		}
		current.Status.InvestigationMetadata = &aianalysisv1.InvestigationMetadata{InvestigationTime: 1}
		return k8sClient.Status().Update(ctx, current)
	})
}

func persistDirectParentHandoffUID(ctx context.Context, k8sClient client.Client, analysis *aianalysisv1.AIAnalysis, uid types.UID) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current := &aianalysisv1.AIAnalysis{}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), current); err != nil {
			return err
		}
		if current.Status.KASession == nil || current.Status.KASession.AgentSessionRef == nil {
			return fmt.Errorf("AIAnalysis %s/%s lost its AgentSession recovery reference", current.Namespace, current.Name)
		}
		current.Status.KASession.AgentSessionRef.UID = uid
		current.Status.Phase = aianalysisv1.PhaseInvestigating
		current.Status.InvestigationMetadata = &aianalysisv1.InvestigationMetadata{InvestigationTime: 1}
		return k8sClient.Status().Update(ctx, current)
	})
}
