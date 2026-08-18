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

package aianalysis_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/creator"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// Unit Tests: AgentSessionCreator
// DD-AA-KA-001, BR-AA-KA-065.1/065.2: AA creates AgentSession in place of the
// retired SubmitInvestigation HTTP call -- idempotent Get-or-Create with an
// ownerRef to the AIAnalysis CR (not the RR directly, see DD-AA-KA-001
// Decision section: no new RemediationRequest RBAC needed for AA; cascade GC
// still reaches the RR transitively via AIAnalysis's own RR ownerRef).
var _ = Describe("AgentSessionCreator", func() {
	var (
		scheme *runtime.Scheme
		ctx    context.Context
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(agentsessionv1.AddToScheme(scheme)).To(Succeed())
		ctx = context.Background()
	})

	Describe("NewAgentSessionCreator", func() {
		It("should return a non-nil creator", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			c := creator.NewAgentSessionCreator(fakeClient, scheme)

			Expect(c).To(BeAssignableToTypeOf(&creator.AgentSessionCreator{}))
		})
	})

	Describe("GetOrCreate", func() {
		It("UT-AA-KA-065-201: should generate deterministic name 'as-{rr.Name}'", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			c := creator.NewAgentSessionCreator(fakeClient, scheme)
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.RemediationRequestRef = corev1.ObjectReference{Name: "test-remediation"}

			as, err := c.GetOrCreate(ctx, analysis)

			Expect(err).ToNot(HaveOccurred())
			Expect(as.Name).To(Equal("as-test-remediation"))
			Expect(as.Namespace).To(Equal("default"))
		})

		It("UT-AA-KA-065-202: should set ownerRef to the AIAnalysis CR (not the RR) for cascade deletion", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			c := creator.NewAgentSessionCreator(fakeClient, scheme)
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.RemediationRequestRef = corev1.ObjectReference{Name: "test-remediation"}

			as, err := c.GetOrCreate(ctx, analysis)
			Expect(err).ToNot(HaveOccurred())

			created := &agentsessionv1.AgentSession{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: as.Name, Namespace: "default"}, created)).To(Succeed())
			Expect(created.OwnerReferences).To(HaveLen(1))
			Expect(created.OwnerReferences[0].Kind).To(Equal("AIAnalysis"))
			Expect(created.OwnerReferences[0].Name).To(Equal(analysis.Name))
			Expect(created.OwnerReferences[0].UID).To(Equal(analysis.UID))
			Expect(created.OwnerReferences[0].Controller).ToNot(BeNil())
			Expect(*created.OwnerReferences[0].Controller).To(BeTrue())
			Expect(created.OwnerReferences[0].BlockOwnerDeletion).ToNot(BeNil())
			Expect(*created.OwnerReferences[0].BlockOwnerDeletion).To(BeTrue())
		})

		It("UT-AA-KA-065-203: should populate Spec via BuildAgentSessionSpec (no content loss from the retired HTTP body)", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			c := creator.NewAgentSessionCreator(fakeClient, scheme)
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.RemediationRequestRef = corev1.ObjectReference{Name: "test-remediation"}
			analysis.Spec.AnalysisRequest.SignalContext.SignalName = "OOMKilled"

			as, err := c.GetOrCreate(ctx, analysis)

			Expect(err).ToNot(HaveOccurred())
			Expect(as.Spec.SignalName).To(Equal("OOMKilled"))
			Expect(as.Spec.IncidentID).To(Equal("ai-test"))
			Expect(as.Spec.RemediationRequestRef.Name).To(Equal("test-remediation"))
			Expect(as.Spec.RemediationRequestRef.Namespace).To(Equal("default"))
		})

		It("UT-AA-KA-065-204: should return the existing AgentSession when already created (idempotency)", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			c := creator.NewAgentSessionCreator(fakeClient, scheme)
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.RemediationRequestRef = corev1.ObjectReference{Name: "test-remediation"}

			first, err := c.GetOrCreate(ctx, analysis)
			Expect(err).ToNot(HaveOccurred())

			second, err := c.GetOrCreate(ctx, analysis)
			Expect(err).ToNot(HaveOccurred())

			Expect(second.Name).To(Equal(first.Name))
			Expect(second.UID).To(Equal(first.UID))

			// Only one AgentSession should ever exist for this AIAnalysis.
			list := &agentsessionv1.AgentSessionList{}
			Expect(fakeClient.List(ctx, list)).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
		})

		It("UT-AA-KA-065-205: should error when RemediationRequestRef.Name is empty (validation)", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			c := creator.NewAgentSessionCreator(fakeClient, scheme)
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			// RemediationRequestRef intentionally left empty.

			_, err := c.GetOrCreate(ctx, analysis)

			Expect(err).To(HaveOccurred())
		})

		It("UT-AA-KA-065-206: should error when AIAnalysis has an empty UID (defensive -- cannot set owner reference)", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			c := creator.NewAgentSessionCreator(fakeClient, scheme)
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.RemediationRequestRef = corev1.ObjectReference{Name: "test-remediation"}
			analysis.UID = ""

			_, err := c.GetOrCreate(ctx, analysis)

			Expect(err).To(HaveOccurred())
		})
	})
})
