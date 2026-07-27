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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	roconfig "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

// newCRDOnlyRESTMapper builds a REST mapper that only knows about a synthetic
// CRD kind absent from k8s.staticGVKByKind, mirroring the "CustomWidget"/
// "example.com" fixture in pkg/shared/k8s/gvk_test.go. Used to exercise
// computeEADelays' dynamic REST-mapper fallback branch, which no existing
// test (unit, integration, or E2E) drives through this function (Issue #253
// follow-up: the E2E-FP-253-001 scenario used "Certificate", but that Kind
// resolves statically and never touches the mapper either).
func newCRDOnlyRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "example.com", Version: "v1"},
	})
	mapper.Add(
		schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "CustomWidget"},
		meta.RESTScopeNamespace,
	)
	return mapper
}

// UT-RO-253-009..012: computeEADelays composes three independently-tested
// ingredients (k8sutil.ResolveGVKForKind, creator.IsBuiltInGroup,
// AsyncPropagationConfig.ComputePropagationDelay) that, per the #253
// investigation, were never exercised together outside of IT/E2E — and both
// of those tiers resolve their test Kind ("EffectivenessAssessment" / static
// resolution, "Certificate" / static resolution) without ever reaching the
// dynamic REST-mapper fallback. These tests close that gap directly, in
// isolation, without envtest or a real operator (BR-RO-103, DD-EM-004 v2.0).
var _ = Describe("computeEADelays", func() {
	var (
		r      *Reconciler
		logger logr.Logger
	)

	BeforeEach(func() {
		logger = logr.Discard()
		r = &Reconciler{
			asyncPropagation: roconfig.AsyncPropagationConfig{
				GitOpsSyncDelay:        3 * time.Minute,
				OperatorReconcileDelay: 1 * time.Minute,
			},
		}
	})

	newRR := func(kind string) *remediationv1.RemediationRequest {
		return &remediationv1.RemediationRequest{
			Spec: remediationv1.RemediationRequestSpec{
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: kind,
					Name: "target",
				},
			},
		}
	}

	It("UT-RO-253-009: resolves a CRD kind via the dynamic REST-mapper fallback and applies the operator reconcile delay", Label("UT-RO-253-009"), func() {
		r.restMapper = newCRDOnlyRESTMapper()
		rr := newRR("CustomWidget")

		hashComputeDelay, alertCheckDelay, isCRD := r.computeEADelays(rr, nil, nil, false, logger)

		Expect(isCRD).To(BeTrue(), "example.com is not a built-in group, so the mapper-resolved GVK must be classified as a CRD")
		Expect(hashComputeDelay).NotTo(BeNil())
		Expect(hashComputeDelay.Duration).To(Equal(1 * time.Minute))
		Expect(alertCheckDelay).To(BeNil())
	})

	It("UT-RO-253-010: resolves a built-in kind statically and never treats it as a CRD, even with a mapper present", Label("UT-RO-253-010"), func() {
		r.restMapper = newCRDOnlyRESTMapper()
		rr := newRR("Pod")

		hashComputeDelay, alertCheckDelay, isCRD := r.computeEADelays(rr, nil, nil, false, logger)

		Expect(isCRD).To(BeFalse())
		Expect(hashComputeDelay).To(BeNil())
		Expect(alertCheckDelay).To(BeNil())
	})

	It("UT-RO-253-011: an unresolvable kind degrades gracefully to a sync target instead of a CRD", Label("UT-RO-253-011"), func() {
		r.restMapper = newCRDOnlyRESTMapper()
		rr := newRR("SomethingNotRegisteredAnywhere")

		hashComputeDelay, alertCheckDelay, isCRD := r.computeEADelays(rr, nil, nil, false, logger)

		Expect(isCRD).To(BeFalse())
		Expect(hashComputeDelay).To(BeNil())
		Expect(alertCheckDelay).To(BeNil())
	})

	It("UT-RO-253-012: an AI-identified remediation target overrides the signal target's kind for CRD detection", Label("UT-RO-253-012"), func() {
		r.restMapper = newCRDOnlyRESTMapper()
		rr := newRR("Deployment") // signal target: built-in, would NOT be a CRD
		dualTarget := &creator.DualTarget{
			Signal:      eav1.TargetResource{Kind: "Deployment", Name: "target"},
			Remediation: eav1.TargetResource{Kind: "CustomWidget", Name: "widget"}, // AI-identified remediation target: CRD
		}

		hashComputeDelay, _, isCRD := r.computeEADelays(rr, dualTarget, nil, false, logger)

		Expect(isCRD).To(BeTrue(), "computeEADelays must resolve GVK/isCRD from dualTarget.Remediation.Kind, not rr.Spec.TargetResource.Kind, once AI identifies a different remediation target")
		Expect(hashComputeDelay).NotTo(BeNil())
		Expect(hashComputeDelay.Duration).To(Equal(1 * time.Minute))
	})

	It("UT-RO-253-013: compounds GitOps sync delay and operator reconcile delay for a GitOps-managed CRD target", Label("UT-RO-253-013"), func() {
		r.restMapper = newCRDOnlyRESTMapper()
		rr := newRR("CustomWidget")

		hashComputeDelay, _, isCRD := r.computeEADelays(rr, nil, nil, true, logger)

		Expect(isCRD).To(BeTrue())
		Expect(hashComputeDelay).NotTo(BeNil())
		Expect(hashComputeDelay.Duration).To(Equal(4 * time.Minute))
	})
})
