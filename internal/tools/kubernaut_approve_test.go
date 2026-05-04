package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

func newFakeRAR(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "RemediationApprovalRequest",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"remediationRequestRef": map[string]interface{}{
					"name": "rr-1",
				},
			},
			"status": map[string]interface{}{
				"phase": "Pending",
			},
		},
	}
}

var _ = Describe("kubernaut_approve", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-104-001: patches RAR status to Approved", func() {
		client := newDynamicFakeClient(newFakeRAR("payments", "rar-1"))
		result, err := tools.HandleApprove(ctx, client, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Approved",
		}, "bob")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("Approved"))
	})

	It("UT-AF-104-002: patches RAR status to Rejected", func() {
		client := newDynamicFakeClient(newFakeRAR("payments", "rar-1"))
		result, err := tools.HandleApprove(ctx, client, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Rejected",
			Reason:    "Too risky",
		}, "bob")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Status).To(Equal("Rejected"))
	})

	It("UT-AF-104-003: sets decidedBy from JWT identity", func() {
		var capturedPatch []byte
		client := newDynamicFakeClient(newFakeRAR("payments", "rar-1"))
		client.PrependReactor("patch", "remediationapprovalrequests", func(action k8stesting.Action) (bool, runtime.Object, error) {
			patchAction, _ := action.(k8stesting.PatchAction)
			capturedPatch = patchAction.GetPatch()
			return false, nil, nil
		})
		_, err := tools.HandleApprove(ctx, client, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "rar-1",
			Decision:  "Approved",
		}, "bob")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(capturedPatch)).To(ContainSubstring("bob"))
	})

	It("UT-AF-104-004: returns error when RAR not found", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleApprove(ctx, client, tools.ApproveArgs{
			Namespace: "payments",
			RARName:   "missing",
			Decision:  "Approved",
		}, "bob")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("UT-AF-104-005: supports workflowOverride in approval", func() {
		var capturedPatch []byte
		client := newDynamicFakeClient(newFakeRAR("payments", "rar-1"))
		client.PrependReactor("patch", "remediationapprovalrequests", func(action k8stesting.Action) (bool, runtime.Object, error) {
			patchAction, _ := action.(k8stesting.PatchAction)
			capturedPatch = patchAction.GetPatch()
			return false, nil, nil
		})
		_, err := tools.HandleApprove(ctx, client, tools.ApproveArgs{
			Namespace:        "payments",
			RARName:          "rar-1",
			Decision:         "Approved",
			WorkflowOverride: "fast-restart",
		}, "bob")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(capturedPatch)).To(ContainSubstring("fast-restart"))
	})
})
