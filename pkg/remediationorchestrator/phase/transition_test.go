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

// Business Requirement: Issue #666 (RO Phase Handler Registry refactoring)
// Purpose: Characterization tests for TransitionIntent (CHAR-RO-1532) --
// establishes coverage for the previously untested transition.go before
// complexity-lint decomposition (Wave B, #1532).
package phase_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
)

func TestPhase(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RemediationOrchestrator Phase Suite")
}

var _ = Describe("TransitionType.String (CHAR-RO-1532)", func() {
	It("returns the human-readable name for each known transition type", func() {
		Expect(phase.TransitionNone.String()).To(Equal("None"))
		Expect(phase.TransitionAdvance.String()).To(Equal("Advance"))
		Expect(phase.TransitionFailed.String()).To(Equal("Failed"))
		Expect(phase.TransitionBlocked.String()).To(Equal("Blocked"))
		Expect(phase.TransitionVerifying.String()).To(Equal("Verifying"))
		Expect(phase.TransitionInheritedCompleted.String()).To(Equal("InheritedCompleted"))
		Expect(phase.TransitionInheritedFailed.String()).To(Equal("InheritedFailed"))
		Expect(phase.TransitionCompletedWithoutVerification.String()).To(Equal("CompletedWithoutVerification"))
	})

	It("returns Unknown(N) for an out-of-range transition type", func() {
		Expect(phase.TransitionType(999).String()).To(Equal("Unknown(999)"))
	})
})

var _ = Describe("TransitionIntent constructors (CHAR-RO-1532)", func() {
	It("Advance sets Type/TargetPhase/Reason", func() {
		ti := phase.Advance(phase.Executing, "workflow ready")
		Expect(ti.Type).To(Equal(phase.TransitionAdvance))
		Expect(ti.TargetPhase).To(Equal(phase.Executing))
		Expect(ti.Reason).To(Equal("workflow ready"))
	})

	It("Fail sets Type/FailurePhase/FailureErr/Reason", func() {
		err := errors.New("boom")
		ti := phase.Fail(remediationv1.FailurePhase("Analyzing"), err, "analysis failed")
		Expect(ti.Type).To(Equal(phase.TransitionFailed))
		Expect(ti.FailurePhase).To(Equal(remediationv1.FailurePhase("Analyzing")))
		Expect(ti.FailureErr).To(MatchError(err))
		Expect(ti.Reason).To(Equal("analysis failed"))
	})

	It("Block sets Type/Block", func() {
		meta := &phase.BlockMeta{Reason: "consecutive failures"}
		ti := phase.Block(meta)
		Expect(ti.Type).To(Equal(phase.TransitionBlocked))
		Expect(ti.Block).To(Equal(meta))
	})

	It("Requeue sets Type/RequeueAfter/Reason", func() {
		ti := phase.Requeue(5*time.Second, "waiting on dependency")
		Expect(ti.Type).To(Equal(phase.TransitionNone))
		Expect(ti.RequeueAfter).To(Equal(5 * time.Second))
		Expect(ti.RequeueImmediately).To(BeFalse())
		Expect(ti.Reason).To(Equal("waiting on dependency"))
	})

	It("NoOp sets Type/Reason with no requeue", func() {
		ti := phase.NoOp("nothing to do")
		Expect(ti.Type).To(Equal(phase.TransitionNone))
		Expect(ti.RequeueAfter).To(BeZero())
		Expect(ti.RequeueImmediately).To(BeFalse())
		Expect(ti.Reason).To(Equal("nothing to do"))
	})

	It("RequeueNow sets Type/RequeueImmediately/Reason", func() {
		ti := phase.RequeueNow("event cleared")
		Expect(ti.Type).To(Equal(phase.TransitionNone))
		Expect(ti.RequeueImmediately).To(BeTrue())
		Expect(ti.Reason).To(Equal("event cleared"))
	})

	It("Verify sets Type/Outcome/Reason", func() {
		ti := phase.Verify("remediationSucceeded", "workflow completed")
		Expect(ti.Type).To(Equal(phase.TransitionVerifying))
		Expect(ti.Outcome).To(Equal("remediationSucceeded"))
		Expect(ti.Reason).To(Equal("workflow completed"))
	})

	It("CompleteWithoutVerification sets Type/Reason", func() {
		ti := phase.CompleteWithoutVerification("dry-run mode")
		Expect(ti.Type).To(Equal(phase.TransitionCompletedWithoutVerification))
		Expect(ti.Reason).To(Equal("dry-run mode"))
	})

	It("InheritComplete sets Type/SourceRef/SourceKind/Reason", func() {
		ti := phase.InheritComplete("wfe-1", "WorkflowExecution", "original completed")
		Expect(ti.Type).To(Equal(phase.TransitionInheritedCompleted))
		Expect(ti.SourceRef).To(Equal("wfe-1"))
		Expect(ti.SourceKind).To(Equal("WorkflowExecution"))
		Expect(ti.Reason).To(Equal("original completed"))
	})

	It("InheritFail sets Type/FailureErr/SourceRef/SourceKind/Reason", func() {
		err := errors.New("wfe failed")
		ti := phase.InheritFail(err, "wfe-1", "WorkflowExecution", "original failed")
		Expect(ti.Type).To(Equal(phase.TransitionInheritedFailed))
		Expect(ti.FailureErr).To(MatchError(err))
		Expect(ti.SourceRef).To(Equal("wfe-1"))
		Expect(ti.SourceKind).To(Equal("WorkflowExecution"))
		Expect(ti.Reason).To(Equal("original failed"))
	})
})

var _ = Describe("TransitionIntent.Validate (CHAR-RO-1532)", func() {
	DescribeTable("valid intents",
		func(ti phase.TransitionIntent) {
			Expect(ti.Validate()).To(Succeed())
		},
		Entry("Advance with TargetPhase set", phase.Advance(phase.Executing, "r")),
		Entry("Fail with FailurePhase set", phase.Fail(remediationv1.FailurePhase("Analyzing"), errors.New("e"), "r")),
		Entry("Block with metadata set", phase.Block(&phase.BlockMeta{Reason: "r"})),
		Entry("Verify (Outcome optional)", phase.Verify("", "r")),
		Entry("CompleteWithoutVerification (Reason optional)", phase.CompleteWithoutVerification("")),
		Entry("InheritComplete with SourceRef/SourceKind set", phase.InheritComplete("ref", "kind", "r")),
		Entry("InheritFail with SourceRef/SourceKind set", phase.InheritFail(errors.New("e"), "ref", "kind", "r")),
		Entry("NoOp (TransitionNone, no additional requirements)", phase.NoOp("r")),
		Entry("Requeue (TransitionNone, no additional requirements)", phase.Requeue(time.Second, "r")),
	)

	DescribeTable("invalid intents",
		func(ti phase.TransitionIntent, expectedSubstring string) {
			err := ti.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(expectedSubstring))
		},
		Entry("Advance missing TargetPhase",
			phase.TransitionIntent{Type: phase.TransitionAdvance}, "TransitionAdvance requires TargetPhase"),
		Entry("Failed missing FailurePhase",
			phase.TransitionIntent{Type: phase.TransitionFailed}, "TransitionFailed requires FailurePhase"),
		Entry("Blocked missing Block metadata",
			phase.TransitionIntent{Type: phase.TransitionBlocked}, "TransitionBlocked requires Block metadata"),
		Entry("InheritedCompleted missing SourceRef",
			phase.TransitionIntent{Type: phase.TransitionInheritedCompleted, SourceKind: "WorkflowExecution"},
			"TransitionInheritedCompleted requires SourceRef and SourceKind"),
		Entry("InheritedCompleted missing SourceKind",
			phase.TransitionIntent{Type: phase.TransitionInheritedCompleted, SourceRef: "wfe-1"},
			"TransitionInheritedCompleted requires SourceRef and SourceKind"),
		Entry("InheritedFailed missing SourceRef and SourceKind",
			phase.TransitionIntent{Type: phase.TransitionInheritedFailed},
			"TransitionInheritedFailed requires SourceRef and SourceKind"),
		Entry("unknown TransitionType",
			phase.TransitionIntent{Type: phase.TransitionType(999)}, fmt.Sprintf("unknown TransitionType: %d", 999)),
	)
})

var _ = Describe("TransitionIntent.IsNoOp / IsRequeue (CHAR-RO-1532)", func() {
	It("IsNoOp is true only for TransitionNone with no requeue of any kind", func() {
		Expect(phase.NoOp("r").IsNoOp()).To(BeTrue())
		Expect(phase.Requeue(time.Second, "r").IsNoOp()).To(BeFalse())
		Expect(phase.RequeueNow("r").IsNoOp()).To(BeFalse())
		Expect(phase.Advance(phase.Executing, "r").IsNoOp()).To(BeFalse())
	})

	It("IsRequeue is true for TransitionNone with a timed or immediate requeue", func() {
		Expect(phase.Requeue(time.Second, "r").IsRequeue()).To(BeTrue())
		Expect(phase.RequeueNow("r").IsRequeue()).To(BeTrue())
		Expect(phase.NoOp("r").IsRequeue()).To(BeFalse())
		Expect(phase.Advance(phase.Executing, "r").IsRequeue()).To(BeFalse())
	})
})
