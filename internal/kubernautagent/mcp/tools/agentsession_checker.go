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

package tools

import (
	"context"
	"fmt"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sAgentSessionExistenceChecker implements AgentSessionExistenceChecker by
// querying the K8s API for the AgentSession CRD deterministically named
// "as-<rrID>" -- the exact naming convention AgentSessionCreator.GetOrCreate
// uses (pkg/aianalysis/creator/agentsession.go: fmt.Sprintf("as-%s",
// analysis.Spec.RemediationRequestRef.Name)). Follows the same pattern as
// K8sRRExistenceChecker (rr_checker.go).
type K8sAgentSessionExistenceChecker struct {
	client    client.Client
	namespace string
}

// NewK8sAgentSessionExistenceChecker creates a checker backed by a
// controller-runtime client. The client's scheme must have agentsessionv1
// registered (buildMCPControllerClient, cmd/kubernautagent/routes.go).
func NewK8sAgentSessionExistenceChecker(c client.Client, namespace string) *K8sAgentSessionExistenceChecker {
	return &K8sAgentSessionExistenceChecker{client: c, namespace: namespace}
}

func (c *K8sAgentSessionExistenceChecker) AgentSessionExists(ctx context.Context, rrID string) (bool, error) {
	name := fmt.Sprintf("as-%s", rrID)
	var as agentsessionv1.AgentSession
	err := c.client.Get(ctx, client.ObjectKey{Namespace: c.namespace, Name: name}, &as)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

var _ AgentSessionExistenceChecker = (*K8sAgentSessionExistenceChecker)(nil)
