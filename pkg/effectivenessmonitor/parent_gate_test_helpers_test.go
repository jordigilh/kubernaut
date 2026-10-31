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

package effectivenessmonitor_test

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// newEMTestScheme mirrors the production EM scheme for unit reconciler tests.
// The parent lifecycle gate is deliberately fail-closed, so every fake reader
// that exercises Reconcile must be able to decode RemediationRequest.
func newEMTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	if err := eav1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := remediationv1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	return scheme
}

// parentRequestsForEAs supplies the persisted parent phase expected by the
// pre-existing EA behavior tests. Production tests that exercise cancellation
// use explicit parent phases instead of this compatibility fixture.
func parentRequestsForEAs(objects ...client.Object) []client.Object {
	parents := make([]client.Object, 0, len(objects))
	for _, object := range objects {
		ea, ok := object.(*eav1.EffectivenessAssessment)
		if !ok || ea.Spec.CorrelationID == "" {
			continue
		}
		parents = append(parents, &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ea.Spec.CorrelationID,
				Namespace: ea.Namespace,
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseVerifying,
			},
		})
	}
	return parents
}

func parentRequestForEA(ea *eav1.EffectivenessAssessment) client.Object {
	parents := parentRequestsForEAs(ea)
	if len(parents) == 0 {
		return nil
	}
	return parents[0]
}
