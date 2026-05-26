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
// querying InvestigationSession CRDs via a controller-runtime client using
// a field index on spec.remediationRequestRef.name. BR-INTERACTIVE-010.
type K8sInvestigationSessionChecker struct {
	client    client.Client
	namespace string
}

// NewK8sInvestigationSessionChecker creates a checker that queries IS CRDs
// in the given namespace.
func NewK8sInvestigationSessionChecker(c client.Client, namespace string) *K8sInvestigationSessionChecker {
	return &K8sInvestigationSessionChecker{client: c, namespace: namespace}
}

// HasActiveSession returns true if an InvestigationSession CRD with Active phase
// exists for the given RR name.
func (k *K8sInvestigationSessionChecker) HasActiveSession(ctx context.Context, rrName string) (bool, error) {
	if rrName == "" {
		return false, nil
	}

	var list isv1alpha1.InvestigationSessionList
	if err := k.client.List(ctx, &list,
		client.InNamespace(k.namespace),
		client.MatchingFields{ISFieldIndexRRName: rrName},
	); err != nil {
		return false, fmt.Errorf("list InvestigationSessions for RR %s: %w", rrName, err)
	}

	for i := range list.Items {
		phase := list.Items[i].Status.Phase
		if phase == isv1alpha1.SessionPhaseActive || phase == isv1alpha1.SessionPhaseDisconnected {
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
