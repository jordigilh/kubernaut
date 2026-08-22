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

		// UT-AA-2214-006 (finalizer bootstrap-race fix, CI RCA PR #2222):
		// the finalizer must be set in the SAME Create call that brings the
		// AgentSession into existence, not left for AF's reconciler to add
		// reactively -- otherwise an immediate DeleteForCascadeCancel
		// landing before that reconciler's first pass removes the object
		// with no finalizer ever attached, reproducing the same
		// delete-event race the finalizer exists to close.
		It("UT-AA-2214-006: sets agentsessionv1.TerminalCloseFinalizer at creation", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			c := creator.NewAgentSessionCreator(fakeClient, scheme)
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.RemediationRequestRef = corev1.ObjectReference{Name: "test-remediation"}

			as, err := c.GetOrCreate(ctx, analysis)
			Expect(err).ToNot(HaveOccurred())

			created := &agentsessionv1.AgentSession{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{Name: as.Name, Namespace: "default"}, created)).To(Succeed())
			Expect(created.Finalizers).To(ContainElement(agentsessionv1.TerminalCloseFinalizer))
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

		// Regression proof for #2081 (main-tracking clone of #2080):
		// handleSessionLost's 404-triggered regeneration raced
		// tryAdoptCorrelatedSession with no backoff and no cap-exhaustion
		// safety net, so a rapid cascade of legitimate KA hand-offs
		// (autonomous->interactive upgrade, then one or more AF-correlated
		// takeovers) could exhaust the 5-regeneration cap and permanently
		// fail an AIAnalysis whose underlying investigation had already
		// completed successfully. Both handleSessionLost and
		// tryAdoptCorrelatedSession are fully retired under DD-AA-KA-001 --
		// GetOrCreate has no regeneration concept, no cap, and no adoption
		// race to lose: repeated/concurrent calls for the same AIAnalysis
		// always resolve to the single already-created AgentSession
		// (BR-AA-KA-065.1/065.2), eliminating this failure mode by
		// construction rather than by tuning a cap or backoff.
		It("UT-AA-KA-065-207 (#2081 regression): rapid concurrent GetOrCreate calls for the same AIAnalysis never error and never create more than one AgentSession", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			c := creator.NewAgentSessionCreator(fakeClient, scheme)
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.RemediationRequestRef = corev1.ObjectReference{Name: "test-remediation"}

			// #2081's evidence describes several hand-offs landing in quick
			// succession; 10 concurrent callers exceeds the old 5-regeneration
			// cap that used to be exhausted by this exact shape of race.
			const concurrentCallers = 10
			results := make(chan error, concurrentCallers)
			for i := 0; i < concurrentCallers; i++ {
				go func() {
					_, err := c.GetOrCreate(ctx, analysis)
					results <- err
				}()
			}
			for i := 0; i < concurrentCallers; i++ {
				Expect(<-results).ToNot(HaveOccurred(),
					"#2081: no caller should ever see an error from a concurrent hand-off -- there is no cap left to exhaust")
			}

			list := &agentsessionv1.AgentSessionList{}
			Expect(fakeClient.List(ctx, list)).To(Succeed())
			Expect(list.Items).To(HaveLen(1),
				"#2081: concurrent GetOrCreate calls must never produce more than one AgentSession for the same AIAnalysis")
		})
	})

	// DeleteForRetry: BR-AI-009 / DD-AA-KA-001 amendment. When KA tags a
	// dispatch failure as AgentSessionReasonCapacityExceeded (a transient,
	// self-resolving backpressure condition, not a genuine investigation
	// failure), AA deletes the stale AgentSession so GetOrCreate's next
	// reconcile naturally falls through to Create -- a fresh attempt, not a
	// mutation of the terminal Failed object.
	Describe("DeleteForRetry", func() {
		It("UT-AA-KA-065-208: should delete an existing AgentSession", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			c := creator.NewAgentSessionCreator(fakeClient, scheme)
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.RemediationRequestRef = corev1.ObjectReference{Name: "test-remediation"}

			as, err := c.GetOrCreate(ctx, analysis)
			Expect(err).ToNot(HaveOccurred())

			Expect(c.DeleteForRetry(ctx, as)).To(Succeed())

			list := &agentsessionv1.AgentSessionList{}
			Expect(fakeClient.List(ctx, list)).To(Succeed())
			Expect(list.Items).To(BeEmpty(), "the stale AgentSession must be gone so the next GetOrCreate falls through to Create")
		})

		It("UT-AA-KA-065-209: should be idempotent — deleting an already-deleted (NotFound) AgentSession is not an error", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			c := creator.NewAgentSessionCreator(fakeClient, scheme)
			analysis := helpers.NewAIAnalysis("ai-test", "default")
			analysis.Spec.RemediationRequestRef = corev1.ObjectReference{Name: "test-remediation"}

			as, err := c.GetOrCreate(ctx, analysis)
			Expect(err).ToNot(HaveOccurred())

			Expect(c.DeleteForRetry(ctx, as)).To(Succeed())
			// Second call against the same, now-nonexistent object: a retry
			// race (e.g. two reconciles both observing CapacityExceeded)
			// must not surface a NotFound error.
			Expect(c.DeleteForRetry(ctx, as)).To(Succeed())
		})
	})
})
