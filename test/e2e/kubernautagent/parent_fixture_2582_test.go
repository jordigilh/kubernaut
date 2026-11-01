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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
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

	analysis, err := infrastructure.EnsureAgentSessionParents(
		ctx, c.Client, c.Scheme(), as.Namespace, as.Spec)
	if err != nil {
		return fmt.Errorf("seed direct AgentSession parent chain: %w", err)
	}
	// The AIAnalysis E2E suite also runs a real AIA controller cluster-wide.
	// Mark this direct KA handoff as already entered Investigating so that
	// controller observes the fixture as pre-authorized work and does not
	// create a second deterministic AgentSession for it.
	analysis.Status.InvestigationMetadata = &aianalysisv1.InvestigationMetadata{InvestigationTime: 1}
	if err := c.Status().Update(ctx, analysis); err != nil {
		return fmt.Errorf("persist direct AgentSession investigation handoff: %w", err)
	}
	if err := controllerutil.SetControllerReference(analysis, as, c.Scheme()); err != nil {
		return fmt.Errorf("set direct AgentSession AIAnalysis owner: %w", err)
	}
	return c.Client.Create(ctx, object, opts...)
}
