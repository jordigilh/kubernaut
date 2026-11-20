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

package controller_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

// UT-RO-2658-008 / IT-RO-2658-204: the distributed lock key and the busy
// gate input are the same group-aware identity the WFE carries (#2658 E2b).
// A cross-group pair must not contend on one lock, and the key must equal
// creator.TargetIdentity.String so it can match WorkflowExecution.spec.
var _ = Describe("Distributed lock key identity (#2658) [DD-WE-001, AC-4]", func() {
	var rr *remediationv1.RemediationRequest

	BeforeEach(func() {
		rr = &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rr-2658", Namespace: "kubernaut-system"},
			Spec: remediationv1.RemediationRequestSpec{
				TargetResource: remediationv1.ResourceIdentifier{
					Kind: "Pod", Name: "api", Namespace: "prod", APIVersion: "v1",
				},
			},
		}
	})

	aiWithTarget := func(apiVersion string) *aianalysisv1.AIAnalysis {
		ai := &aianalysisv1.AIAnalysis{ObjectMeta: metav1.ObjectMeta{Name: "ai-2658", Namespace: "kubernaut-system"}}
		ai.Status.EnsureRCAResult().RootCauseAnalysis = &aianalysisv1.RootCauseAnalysis{
			RemediationTarget: &aianalysisv1.RemediationTarget{
				Namespace: "prod", Kind: "Ingress", Name: "web", APIVersion: apiVersion,
			},
		}
		return ai
	}

	It("uses the RCA target's own group for the lock key", func() {
		key := controller.FormatRemediationTargetStringForTest(rr, aiWithTarget("networking.k8s.io/v1"))
		Expect(key).To(Equal("prod/networking.k8s.io/v1/Ingress/web"))
		Expect(key).To(Equal(creator.TargetIdentity{
			Namespace: "prod", Kind: "Ingress", Name: "web", APIVersion: "networking.k8s.io/v1",
		}.String()), "the lock key must equal the WFE spec identity")
	})

	It("falls back to the RR's group when the RCA target carries no apiVersion", func() {
		rr.Spec.TargetResource = remediationv1.ResourceIdentifier{Kind: "Deployment", Name: "web", Namespace: "prod", APIVersion: "apps/v1"}
		key := controller.FormatRemediationTargetStringForTest(rr, aiWithTarget(""))
		Expect(key).To(Equal("prod/apps/v1/Ingress/web"))
	})

	It("renders the unknown form when neither carries an apiVersion", func() {
		rr.Spec.TargetResource = remediationv1.ResourceIdentifier{Kind: "Pod", Name: "api", Namespace: "prod"}
		key := controller.FormatRemediationTargetStringForTest(rr, aiWithTarget(""))
		Expect(key).To(Equal("prod/unresolved/unresolved/Ingress/web"))
	})

	It("keeps a cross-group pair on distinct lock keys", func() {
		first := controller.FormatRemediationTargetStringForTest(rr, aiWithTarget("networking.k8s.io/v1"))
		second := controller.FormatRemediationTargetStringForTest(rr, aiWithTarget("extensions/v1beta1"))
		Expect(first).NotTo(Equal(second))
	})

	It("returns empty when the AIAnalysis carries no remediation target", func() {
		ai := &aianalysisv1.AIAnalysis{ObjectMeta: metav1.ObjectMeta{Name: "ai-2658", Namespace: "kubernaut-system"}}
		Expect(controller.FormatRemediationTargetStringForTest(rr, ai)).To(BeEmpty())
	})
})
