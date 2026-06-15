package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("kubernaut_cancel_remediation", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-105-001: patches RR to cancelled state", func() {
		client := newTypedFakeClient(newTypedRR("payments", "rr-1", "Executing"))
		result, err := tools.HandleCancelRemediation(ctx, client, tools.CancelRemediationArgs{RRID: "rr-1", Namespace: "payments"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("Cancelled"))
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
