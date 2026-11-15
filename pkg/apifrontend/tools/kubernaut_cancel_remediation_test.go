package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("kubernaut_cancel_remediation", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-ORCH-2588-001: records cancellation without terminalizing the RR", func() {
		client := newTypedFakeClient(newTypedRR("payments", "rr-1", "Executing"))
		result, err := tools.HandleCancelRemediation(ctx, client, tools.CancelRemediationArgs{RRID: "rr-1", Namespace: "payments"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("Cancelling"))

		var rr remediationv1.RemediationRequest
		Expect(client.Get(ctx, crclient.ObjectKey{Namespace: "payments", Name: "rr-1"}, &rr)).To(Succeed())
		Expect(rr.Status.OverallPhase).To(Equal(remediationv1.PhaseCancelling))
		Expect(rr.Status.CompletedAt).To(BeNil())
		Expect(rr.Finalizers).To(ContainElement("kubernaut.ai/remediationrequest-cancellation"))
	})

	It("UT-ORCH-2588-002: treats repeated cancellation requests as idempotent", func() {
		client := newTypedFakeClient(newTypedRR("payments", "rr-1", "Executing"))
		args := tools.CancelRemediationArgs{RRID: "rr-1", Namespace: "payments"}

		first, err := tools.HandleCancelRemediation(ctx, client, args)
		Expect(err).NotTo(HaveOccurred())
		second, err := tools.HandleCancelRemediation(ctx, client, args)
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Status).To(Equal("Cancelling"))
		Expect(second.Status).To(Equal("Cancelling"))
	})

	It("UT-AF-105-002: returns error when RR not found", func() {
		client := newTypedFakeClient()
		_, err := tools.HandleCancelRemediation(ctx, client, tools.CancelRemediationArgs{RRID: "rr-missing", Namespace: "payments"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("UT-AF-105-003: returns error when RR already terminal", func() {
		client := newTypedFakeClient(newTypedRR("payments", "rr-1", "Completed"))
		_, err := tools.HandleCancelRemediation(ctx, client, tools.CancelRemediationArgs{RRID: "rr-1", Namespace: "payments"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("terminal"))
	})
})
