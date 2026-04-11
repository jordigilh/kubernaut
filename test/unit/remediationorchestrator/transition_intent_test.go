/*
Copyright 2025 Jordi Gil.

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

package remediationorchestrator

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

var _ = Describe("Issue #666 - TransitionIntent (BR-ORCH-025)", func() {

	// ========================================
	// Constructor Tests
	// ========================================
	Describe("Constructors", func() {

		It("UT-TI-001: Advance sets TransitionAdvance and target phase", func() {
			intent := phase.Advance(phase.Executing, "WFE created successfully")
			Expect(intent.Type).To(Equal(phase.TransitionAdvance))
			Expect(intent.TargetPhase).To(Equal(phase.Executing))
			Expect(intent.Reason).To(Equal("WFE created successfully"))
		})

		It("UT-TI-002: Fail sets TransitionFailed with failure phase and error", func() {
			err := errors.New("SP creation timeout")
			intent := phase.Fail(remediationv1.FailurePhaseSignalProcessing, err, "SP timed out")
			Expect(intent.Type).To(Equal(phase.TransitionFailed))
			Expect(intent.FailurePhase).To(Equal(remediationv1.FailurePhaseSignalProcessing))
			Expect(intent.FailureErr).To(MatchError("SP creation timeout"))
			Expect(intent.Reason).To(Equal("SP timed out"))
		})

		It("UT-TI-003: Block sets TransitionBlocked with BlockMeta", func() {
			meta := &phase.BlockMeta{
				Reason:       "ConsecutiveFailures",
				Message:      "3 consecutive failures on target",
				RequeueAfter: 30 * time.Second,
				FromPhase:    phase.Pending,
			}
			intent := phase.Block(meta)
			Expect(intent.Type).To(Equal(phase.TransitionBlocked))
			Expect(intent.Block).To(Equal(meta))
			Expect(intent.Block.Reason).To(Equal("ConsecutiveFailures"))
			Expect(intent.Block.FromPhase).To(Equal(phase.Pending))
		})

		It("UT-TI-004: Requeue sets TransitionNone with positive duration", func() {
			intent := phase.Requeue(5*time.Second, "AI still in progress")
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(5 * time.Second))
			Expect(intent.Reason).To(Equal("AI still in progress"))
		})

		It("UT-TI-005: NoOp sets TransitionNone with zero duration", func() {
			intent := phase.NoOp("already terminal")
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(BeZero())
			Expect(intent.Reason).To(Equal("already terminal"))
		})

		It("UT-TI-006: Verify sets TransitionVerifying with outcome", func() {
			intent := phase.Verify("remediationSucceeded", "WFE completed successfully")
			Expect(intent.Type).To(Equal(phase.TransitionVerifying))
			Expect(intent.Outcome).To(Equal("remediationSucceeded"))
			Expect(intent.Reason).To(Equal("WFE completed successfully"))
		})

		It("UT-TI-007: InheritComplete sets TransitionInheritedCompleted with source metadata", func() {
			intent := phase.InheritComplete("original-rr-1", "RemediationRequest", "inherited from original")
			Expect(intent.Type).To(Equal(phase.TransitionInheritedCompleted))
			Expect(intent.SourceRef).To(Equal("original-rr-1"))
			Expect(intent.SourceKind).To(Equal("RemediationRequest"))
			Expect(intent.Reason).To(Equal("inherited from original"))
		})

		It("UT-TI-008: InheritFail sets TransitionInheritedFailed with error and source metadata", func() {
			err := errors.New("original resource failed")
			intent := phase.InheritFail(err, "original-wfe-1", "WorkflowExecution", "inherited failure")
			Expect(intent.Type).To(Equal(phase.TransitionInheritedFailed))
			Expect(intent.FailureErr).To(MatchError("original resource failed"))
			Expect(intent.SourceRef).To(Equal("original-wfe-1"))
			Expect(intent.SourceKind).To(Equal("WorkflowExecution"))
			Expect(intent.Reason).To(Equal("inherited failure"))
		})

		It("UT-TI-026: RequeueNow sets TransitionNone with RequeueImmediately", func() {
			intent := phase.RequeueNow("event-based block cleared")
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueImmediately).To(BeTrue())
			Expect(intent.RequeueAfter).To(BeZero())
			Expect(intent.Reason).To(Equal("event-based block cleared"))
		})
	})

	// ========================================
	// Zero-value isolation: constructors must not set unrelated fields
	// ========================================
	Describe("Zero-value isolation", func() {

		It("UT-TI-027: Advance leaves failure/block/requeue/source/outcome fields zero", func() {
			intent := phase.Advance(phase.Processing, "test")
			Expect(intent.FailurePhase).To(BeEmpty())
			Expect(intent.FailureErr).To(BeNil())
			Expect(intent.Block).To(BeNil())
			Expect(intent.RequeueAfter).To(BeZero())
			Expect(intent.RequeueImmediately).To(BeFalse())
			Expect(intent.Outcome).To(BeEmpty())
			Expect(intent.SourceRef).To(BeEmpty())
			Expect(intent.SourceKind).To(BeEmpty())
		})

		It("UT-TI-028: Fail leaves target/block/requeue/source/outcome fields zero", func() {
			intent := phase.Fail(remediationv1.FailurePhaseApproval, errors.New("x"), "test")
			Expect(intent.TargetPhase).To(BeEmpty())
			Expect(intent.Block).To(BeNil())
			Expect(intent.RequeueAfter).To(BeZero())
			Expect(intent.RequeueImmediately).To(BeFalse())
			Expect(intent.Outcome).To(BeEmpty())
			Expect(intent.SourceRef).To(BeEmpty())
			Expect(intent.SourceKind).To(BeEmpty())
		})

		It("UT-TI-029: Block leaves target/failure/requeue/source/outcome fields zero", func() {
			intent := phase.Block(&phase.BlockMeta{Reason: "test", FromPhase: phase.Pending})
			Expect(intent.TargetPhase).To(BeEmpty())
			Expect(intent.FailurePhase).To(BeEmpty())
			Expect(intent.FailureErr).To(BeNil())
			Expect(intent.RequeueAfter).To(BeZero())
			Expect(intent.RequeueImmediately).To(BeFalse())
			Expect(intent.Outcome).To(BeEmpty())
			Expect(intent.SourceRef).To(BeEmpty())
			Expect(intent.SourceKind).To(BeEmpty())
		})

		It("UT-TI-030: Requeue leaves target/failure/block fields zero", func() {
			intent := phase.Requeue(5*time.Second, "test")
			Expect(intent.TargetPhase).To(BeEmpty())
			Expect(intent.FailurePhase).To(BeEmpty())
			Expect(intent.FailureErr).To(BeNil())
			Expect(intent.Block).To(BeNil())
			Expect(intent.RequeueImmediately).To(BeFalse())
		})

		It("UT-TI-031: RequeueNow leaves target/failure/block/duration fields zero", func() {
			intent := phase.RequeueNow("test")
			Expect(intent.TargetPhase).To(BeEmpty())
			Expect(intent.FailurePhase).To(BeEmpty())
			Expect(intent.FailureErr).To(BeNil())
			Expect(intent.Block).To(BeNil())
			Expect(intent.RequeueAfter).To(BeZero())
		})

		It("UT-TI-036: Verify leaves failure/block/requeue/source fields zero", func() {
			intent := phase.Verify("success", "test")
			Expect(intent.TargetPhase).To(BeEmpty())
			Expect(intent.FailurePhase).To(BeEmpty())
			Expect(intent.FailureErr).To(BeNil())
			Expect(intent.Block).To(BeNil())
			Expect(intent.RequeueAfter).To(BeZero())
			Expect(intent.RequeueImmediately).To(BeFalse())
			Expect(intent.SourceRef).To(BeEmpty())
			Expect(intent.SourceKind).To(BeEmpty())
		})

		It("UT-TI-037: InheritComplete leaves failure/block/requeue/outcome fields zero", func() {
			intent := phase.InheritComplete("ref", "kind", "test")
			Expect(intent.TargetPhase).To(BeEmpty())
			Expect(intent.FailurePhase).To(BeEmpty())
			Expect(intent.FailureErr).To(BeNil())
			Expect(intent.Block).To(BeNil())
			Expect(intent.RequeueAfter).To(BeZero())
			Expect(intent.RequeueImmediately).To(BeFalse())
			Expect(intent.Outcome).To(BeEmpty())
		})

		It("UT-TI-038: InheritFail leaves target/block/requeue/outcome fields zero", func() {
			intent := phase.InheritFail(errors.New("x"), "ref", "kind", "test")
			Expect(intent.TargetPhase).To(BeEmpty())
			Expect(intent.FailurePhase).To(BeEmpty())
			Expect(intent.Block).To(BeNil())
			Expect(intent.RequeueAfter).To(BeZero())
			Expect(intent.RequeueImmediately).To(BeFalse())
			Expect(intent.Outcome).To(BeEmpty())
		})
	})

	// ========================================
	// Validate Tests
	// ========================================
	Describe("Validate", func() {

		It("UT-TI-009: Advance without TargetPhase fails validation", func() {
			intent := phase.TransitionIntent{
				Type:   phase.TransitionAdvance,
				Reason: "missing target",
			}
			Expect(intent.Validate()).To(HaveOccurred())
			Expect(intent.Validate().Error()).To(ContainSubstring("TargetPhase"))
		})

		It("UT-TI-010: Advance with TargetPhase passes validation", func() {
			intent := phase.Advance(phase.Processing, "start processing")
			Expect(intent.Validate()).To(Succeed())
		})

		It("UT-TI-011: Failed without FailurePhase fails validation", func() {
			intent := phase.TransitionIntent{
				Type:   phase.TransitionFailed,
				Reason: "missing failure phase",
			}
			Expect(intent.Validate()).To(HaveOccurred())
			Expect(intent.Validate().Error()).To(ContainSubstring("FailurePhase"))
		})

		It("UT-TI-012: Failed with FailurePhase passes validation", func() {
			intent := phase.Fail(remediationv1.FailurePhaseAIAnalysis, errors.New("oops"), "ai failed")
			Expect(intent.Validate()).To(Succeed())
		})

		It("UT-TI-013: Blocked without BlockMeta fails validation", func() {
			intent := phase.TransitionIntent{
				Type:   phase.TransitionBlocked,
				Reason: "missing block metadata",
			}
			Expect(intent.Validate()).To(HaveOccurred())
			Expect(intent.Validate().Error()).To(ContainSubstring("Block"))
		})

		It("UT-TI-014: Blocked with BlockMeta passes validation", func() {
			intent := phase.Block(&phase.BlockMeta{
				Reason:       "ResourceBusy",
				Message:      "WFE active on target",
				RequeueAfter: 30 * time.Second,
				FromPhase:    phase.Analyzing,
			})
			Expect(intent.Validate()).To(Succeed())
		})

		It("UT-TI-015: NoOp passes validation", func() {
			intent := phase.NoOp("terminal")
			Expect(intent.Validate()).To(Succeed())
		})

		It("UT-TI-016: Requeue passes validation", func() {
			intent := phase.Requeue(10*time.Second, "polling")
			Expect(intent.Validate()).To(Succeed())
		})

		It("UT-TI-017: Verifying passes validation", func() {
			intent := phase.Verify("success", "done")
			Expect(intent.Validate()).To(Succeed())
		})

		It("UT-TI-018: InheritedCompleted with source metadata passes validation", func() {
			intent := phase.InheritComplete("original-rr-1", "RemediationRequest", "inherited")
			Expect(intent.Validate()).To(Succeed())
		})

		It("UT-TI-019: InheritedFailed with source metadata passes validation", func() {
			intent := phase.InheritFail(errors.New("failed"), "original-wfe", "WorkflowExecution", "inherited failure")
			Expect(intent.Validate()).To(Succeed())
		})

		It("UT-TI-039: InheritedCompleted without SourceRef fails validation", func() {
			intent := phase.TransitionIntent{
				Type:       phase.TransitionInheritedCompleted,
				SourceKind: "RemediationRequest",
				Reason:     "missing source ref",
			}
			err := intent.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("SourceRef"))
		})

		It("UT-TI-040: InheritedFailed without SourceKind fails validation", func() {
			intent := phase.TransitionIntent{
				Type:      phase.TransitionInheritedFailed,
				SourceRef: "original-rr",
				Reason:    "missing source kind",
			}
			err := intent.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("SourceKind"))
		})

		It("UT-TI-032: RequeueNow passes validation", func() {
			intent := phase.RequeueNow("block cleared")
			Expect(intent.Validate()).To(Succeed())
		})

		It("UT-TI-033: unknown TransitionType fails validation", func() {
			intent := phase.TransitionIntent{Type: phase.TransitionType(99), Reason: "invalid"}
			err := intent.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown TransitionType"))
		})
	})

	// ========================================
	// Helper Methods
	// ========================================
	Describe("Helper methods", func() {

		It("UT-TI-020: IsNoOp returns true for zero-duration TransitionNone", func() {
			intent := phase.NoOp("done")
			Expect(intent.IsNoOp()).To(BeTrue())
		})

		It("UT-TI-021: IsNoOp returns false for Requeue", func() {
			intent := phase.Requeue(5*time.Second, "wait")
			Expect(intent.IsNoOp()).To(BeFalse())
		})

		It("UT-TI-022: IsNoOp returns false for non-None types", func() {
			intent := phase.Advance(phase.Processing, "advance")
			Expect(intent.IsNoOp()).To(BeFalse())
		})

		It("UT-TI-023: IsRequeue returns true for positive-duration TransitionNone", func() {
			intent := phase.Requeue(10*time.Second, "poll")
			Expect(intent.IsRequeue()).To(BeTrue())
		})

		It("UT-TI-024: IsRequeue returns false for NoOp", func() {
			intent := phase.NoOp("terminal")
			Expect(intent.IsRequeue()).To(BeFalse())
		})

		It("UT-TI-025: IsRequeue returns false for non-None types", func() {
			intent := phase.Verify("success", "done")
			Expect(intent.IsRequeue()).To(BeFalse())
		})

		It("UT-TI-034: RequeueNow is not a NoOp", func() {
			intent := phase.RequeueNow("immediate")
			Expect(intent.IsNoOp()).To(BeFalse())
		})

		It("UT-TI-035: RequeueNow is a Requeue", func() {
			intent := phase.RequeueNow("immediate")
			Expect(intent.IsRequeue()).To(BeTrue())
		})
	})

	// ========================================
	// TransitionType.String()
	// ========================================
	Describe("TransitionType.String()", func() {

		DescribeTable("returns human-readable names",
			func(tt phase.TransitionType, expected string) {
				Expect(tt.String()).To(Equal(expected))
			},
			Entry("None", phase.TransitionNone, "None"),
			Entry("Advance", phase.TransitionAdvance, "Advance"),
			Entry("Failed", phase.TransitionFailed, "Failed"),
			Entry("Blocked", phase.TransitionBlocked, "Blocked"),
			Entry("Verifying", phase.TransitionVerifying, "Verifying"),
			Entry("InheritedCompleted", phase.TransitionInheritedCompleted, "InheritedCompleted"),
			Entry("InheritedFailed", phase.TransitionInheritedFailed, "InheritedFailed"),
		)
	})
})
