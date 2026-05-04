package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

var _ = Describe("kubernaut_get_remediation", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-102-001: returns full RR detail by name", func() {
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		result, err := tools.HandleGetRemediation(ctx, client, tools.GetRemediationArgs{Namespace: "payments", Name: "rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Name).To(Equal("rr-1"))
		Expect(result.Phase).To(Equal("Executing"))
	})

	It("UT-AF-102-002: returns not-found when RR missing", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleGetRemediation(ctx, client, tools.GetRemediationArgs{Namespace: "payments", Name: "missing"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("UT-AF-102-003: returns 403 on namespace access denied", func() {
		client := newDynamicFakeClient()
		client.PrependReactor("get", "remediationrequests", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, newForbiddenError("remediationrequests")
		})
		_, err := tools.HandleGetRemediation(ctx, client, tools.GetRemediationArgs{Namespace: "forbidden", Name: "rr-1"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("access denied"))
	})

	It("UT-AF-102-004: accepts rr_id shorthand", func() {
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Pending"))
		result, err := tools.HandleGetRemediation(ctx, client, tools.GetRemediationArgs{RRID: "payments/rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Namespace).To(Equal("payments"))
		Expect(result.Name).To(Equal("rr-1"))
	})
})
