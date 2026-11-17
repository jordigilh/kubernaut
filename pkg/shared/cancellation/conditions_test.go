package cancellation_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/cancellation"
)

var _ = Describe("Cancellation acknowledgement conditions", func() {
	It("UT-ORCH-2588-021: uses bounded exponential backoff with jitter for cancellation retries", func() {
		first := cancellation.RetryBackoff(1)
		second := cancellation.RetryBackoff(2)
		capped := cancellation.RetryBackoff(20)

		Expect(first).To(BeNumerically(">=", 900*time.Millisecond))
		Expect(first).To(BeNumerically("<=", 1100*time.Millisecond))
		Expect(second).To(BeNumerically(">=", 1800*time.Millisecond))
		Expect(second).To(BeNumerically("<=", 2200*time.Millisecond))
		Expect(capped).To(BeNumerically("<=", 30*time.Second))
	})

	It("UT-ORCH-2588-022: marks a child as requested but not yet stopped", func() {
		conditions := []metav1.Condition{}

		cancellation.SetRequested(&conditions, 7, "parent cancellation observed")

		Expect(cancellation.IsAcknowledged(conditions)).To(BeFalse())
		condition := meta.FindStatusCondition(conditions, cancellation.ConditionAcknowledged)
		Expect(condition).NotTo(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.ObservedGeneration).To(Equal(int64(7)))
	})

	It("UT-ORCH-2588-023: acknowledges verified stop", func() {
		conditions := []metav1.Condition{}

		cancellation.SetAcknowledged(&conditions, 9, "backend termination verified")

		Expect(cancellation.IsAcknowledged(conditions)).To(BeTrue())
		Expect(cancellation.IsFailed(conditions)).To(BeFalse())
	})

	It("UT-ORCH-2588-024: records stop failure without claiming acknowledgement", func() {
		conditions := []metav1.Condition{}

		cancellation.SetFailed(&conditions, 11, "backend termination could not be verified")

		Expect(cancellation.IsAcknowledged(conditions)).To(BeFalse())
		Expect(cancellation.IsFailed(conditions)).To(BeTrue())
	})

	DescribeTable("UT-ORCH-2588-025: identifies parent states that require child stop",
		func(parentPhase remediationv1.RemediationPhase, expected bool) {
			rr := &remediationv1.RemediationRequest{}
			rr.Status.OverallPhase = parentPhase
			Expect(cancellation.ParentStopRequested(rr)).To(Equal(expected))
		},
		Entry("active phase", remediationv1.PhaseExecuting, false),
		Entry("cancelling phase", remediationv1.PhaseCancelling, true),
		Entry("cancelled phase", remediationv1.PhaseCancelled, true),
		Entry("failed phase is not cancellation", remediationv1.PhaseFailed, false),
		Entry("timed out phase is not cancellation", remediationv1.PhaseTimedOut, false),
		Entry("skipped phase is not cancellation", remediationv1.PhaseSkipped, false),
	)

	It("UT-ORCH-2588-026: treats deletion as an authoritative stop request", func() {
		rr := &remediationv1.RemediationRequest{}
		now := metav1.Now()
		rr.DeletionTimestamp = &now

		Expect(cancellation.ParentStopRequested(rr)).To(BeTrue())
	})
})
