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

package controller

import (
	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Gomega DSL convention

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	emtypes "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

// UT-EM-193-006 (Issue #193, DD-EM-005): isAlertDecay interaction with
// now-reachable cluster-scoped metrics.
//
// Before this issue, ea.Status.Components.MetricsAssessed was unconditionally
// false for every cluster-scoped (Node/PersistentVolume) target, so
// isAlertDecay's "metrics regressed" guard clause
// (MetricsAssessed && MetricsScore<=0 -> return false) was dead code for
// those targets -- it could never evaluate true. Now that assessMetrics
// dispatches to buildNodeMetricQuerySpecs/buildPVMetricQuerySpecs for
// cluster-scoped Kinds, MetricsAssessed can become true for a Node target,
// making this guard reachable for the first time. This test documents that
// the guard behaves correctly once reachable: a healthy, spec-stable Node
// with a real metrics regression must NOT be misclassified as Prometheus
// alert decay.
var _ = Describe("isAlertDecay: cluster-scoped metrics reachability (Issue #193, BR-EM-012)", func() {
	It("returns false for a Node target that is healthy and hash-stable but has a real metrics regression", func() {
		r := &Reconciler{}

		healthScore := 1.0
		metricsScore := 0.0 // regressed/no-improvement, <= 0.0
		ea := &eav1.EffectivenessAssessment{
			Spec: eav1.EffectivenessAssessmentSpec{
				SignalTarget: eav1.TargetResource{
					Kind: "Node",
					Name: "worker-1",
					// Namespace intentionally empty: cluster-scoped target.
				},
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Components: eav1.EAComponents{
					HealthAssessed: true,
					HealthScore:    &healthScore,
					HashComputed:   true,
					// Newly reachable for a Node target as of this issue:
					// previously MetricsAssessed was always false at cluster scope.
					MetricsAssessed: true,
					MetricsScore:    &metricsScore,
				},
			},
		}

		alertScore := 0.0 // alert still firing
		ar := alertAssessResult{
			Component: emtypes.ComponentResult{
				Component: emtypes.ComponentAlert,
				Assessed:  true,
				Score:     &alertScore,
			},
		}

		Expect(r.isAlertDecay(ea, ar)).To(BeFalse(),
			"a real metrics regression (MetricsScore<=0) on a cluster-scoped target must suppress alert-decay "+
				"classification, not just on namespace-scoped targets -- the guard must be reachable and correct "+
				"now that Kind=Node can produce MetricsAssessed=true")
	})

	It("returns true for a Node target that is healthy, hash-stable, and has no metrics regression (decay detected)", func() {
		r := &Reconciler{}

		healthScore := 1.0
		metricsScore := 0.6 // improved, > 0.0
		ea := &eav1.EffectivenessAssessment{
			Spec: eav1.EffectivenessAssessmentSpec{
				SignalTarget: eav1.TargetResource{
					Kind: "Node",
					Name: "worker-1",
				},
			},
			Status: eav1.EffectivenessAssessmentStatus{
				Components: eav1.EAComponents{
					HealthAssessed:  true,
					HealthScore:     &healthScore,
					HashComputed:    true,
					MetricsAssessed: true,
					MetricsScore:    &metricsScore,
				},
			},
		}

		alertScore := 0.0 // alert still firing despite health/metrics improvement
		ar := alertAssessResult{
			Component: emtypes.ComponentResult{
				Component: emtypes.ComponentAlert,
				Assessed:  true,
				Score:     &alertScore,
			},
		}

		Expect(r.isAlertDecay(ea, ar)).To(BeTrue(),
			"a healthy, stable, metrics-improved Node target with a still-firing alert is the intended "+
				"Prometheus-alert-decay case (Issue #369, BR-EM-012), now reachable at cluster scope too")
	})
})
