package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("kubernaut_cancel_remediation", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-105-001: patches RR to cancelled state", func() {
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		result, err := tools.HandleCancelRemediation(ctx, client, tools.CancelRemediationArgs{RRID: "payments/rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("Cancelled"))
	})

	It("UT-AF-105-002: returns error when RR not found", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleCancelRemediation(ctx, client, tools.CancelRemediationArgs{RRID: "payments/missing"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("UT-AF-105-003: returns error when RR already terminal", func() {
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Completed"))
		_, err := tools.HandleCancelRemediation(ctx, client, tools.CancelRemediationArgs{RRID: "payments/rr-1"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("terminal"))
	})
})
