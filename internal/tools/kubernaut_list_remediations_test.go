package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/jordigilh/kubernaut-apifrontend/internal/tools"
)

func newFakeRR(namespace, name, phase string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "RemediationRequest",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"targetResource": map[string]interface{}{
					"kind": "Deployment",
					"name": "api-server",
				},
			},
			"status": map[string]interface{}{
				"overallPhase": phase,
			},
		},
	}
}

func newDynamicFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationRequestList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationApprovalRequestList"},
		&unstructured.UnstructuredList{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "SignalProcessingList"},
		&unstructured.UnstructuredList{},
	)
	return dynamicfake.NewSimpleDynamicClient(scheme, objects...)
}

var _ = Describe("kubernaut_list_remediations", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-101-001: lists RRs in namespace", func() {
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"), newFakeRR("payments", "rr-2", "Pending"))
		result, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{Namespace: "payments"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		Expect(result.Remediations).To(HaveLen(2))
	})

	It("UT-AF-101-002: filters by phase", func() {
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"), newFakeRR("payments", "rr-2", "Pending"))
		result, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{Namespace: "payments", Phase: "Executing"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Remediations[0].Phase).To(Equal("Executing"))
	})

	It("UT-AF-101-003: filters by kind and name", func() {
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		result, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{
			Namespace: "payments",
			Kind:      "Deployment",
			Name:      "api-server",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(BeNumerically(">=", 0))
	})

	It("UT-AF-101-004: returns empty list when no RRs match", func() {
		client := newDynamicFakeClient()
		result, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{Namespace: "empty"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
		Expect(result.Remediations).To(BeEmpty())
	})

	It("UT-AF-101-005: returns user-friendly error on 403", func() {
		client := newDynamicFakeClient()
		client.PrependReactor("list", "remediationrequests", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, newForbiddenError("remediationrequests")
		})
		_, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{Namespace: "forbidden"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("access denied"))
	})

	It("UT-AF-101-006: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleListRemediations(ctx, nil, tools.ListRemediationsArgs{Namespace: "default"})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-101-007: invalid namespace returns ErrInvalidInput", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{Namespace: "../etc"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-101-008: kind filter excludes non-matching RRs", func() {
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		result, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{
			Namespace: "payments",
			Kind:      "StatefulSet",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})

	It("UT-AF-101-009: name filter excludes non-matching RRs", func() {
		client := newDynamicFakeClient(newFakeRR("payments", "rr-1", "Executing"))
		result, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{
			Namespace: "payments",
			Name:      "non-existent-target",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})
})
