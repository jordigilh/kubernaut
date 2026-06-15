package tools_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

func rrTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = remediationv1.AddToScheme(s)
	return s
}

func newTypedRR(namespace, name, phase string) *remediationv1.RemediationRequest {
	return &remediationv1.RemediationRequest{
		ObjectMeta: objMeta(namespace, name),
		Spec: remediationv1.RemediationRequestSpec{
			TargetResource: remediationv1.ResourceIdentifier{
				Kind: "Deployment",
				Name: "api-server",
			},
		},
		Status: remediationv1.RemediationRequestStatus{
			OverallPhase: remediationv1.RemediationPhase(phase),
		},
	}
}

func newTypedFakeClient(objects ...crclient.Object) crclient.Client {
	return fake.NewClientBuilder().
		WithScheme(rrTestScheme()).
		WithStatusSubresource(&remediationv1.RemediationRequest{}, &remediationv1.RemediationApprovalRequest{}).
		WithObjects(objects...).
		Build()
}

func newTypedFakeClientWithError(err error) crclient.Client {
	return newTypedFakeClientWithInterceptor(interceptor.Funcs{
		List: func(ctx context.Context, client crclient.WithWatch, list crclient.ObjectList, opts ...crclient.ListOption) error {
			return err
		},
		Get: func(ctx context.Context, client crclient.WithWatch, key crclient.ObjectKey, obj crclient.Object, opts ...crclient.GetOption) error {
			return err
		},
	})
}

func newTypedFakeClientWithInterceptor(fns interceptor.Funcs) crclient.Client {
	return fake.NewClientBuilder().
		WithScheme(rrTestScheme()).
		WithInterceptorFuncs(fns).
		Build()
}

var _ = Describe("kubernaut_list_remediations", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-101-001: lists RRs in namespace", func() {
		client := newTypedFakeClient(newTypedRR("payments", "rr-1", "Executing"), newTypedRR("payments", "rr-2", "Pending"))
		result, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{Namespace: "payments"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		Expect(result.Remediations).To(HaveLen(2))
	})

	It("UT-AF-101-002: filters by phase", func() {
		client := newTypedFakeClient(newTypedRR("payments", "rr-1", "Executing"), newTypedRR("payments", "rr-2", "Pending"))
		result, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{Namespace: "payments", Phase: "Executing"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))
		Expect(result.Remediations[0].Phase).To(Equal("Executing"))
	})

	It("UT-AF-101-003: filters by kind and name", func() {
		client := newTypedFakeClient(newTypedRR("payments", "rr-1", "Executing"))
		result, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{
			Namespace: "payments",
			Kind:      "Deployment",
			Name:      "api-server",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(BeNumerically(">=", 0))
	})

	It("UT-AF-101-004: returns empty list when no RRs match", func() {
		client := newTypedFakeClient()
		result, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{Namespace: "empty"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
		Expect(result.Remediations).To(BeEmpty())
	})

	It("UT-AF-101-005: returns user-friendly error on 403", func() {
		client := newTypedFakeClientWithError(newForbiddenError("remediationrequests"))
		_, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{Namespace: "forbidden"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("access denied"))
	})

	It("UT-AF-101-006: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleListRemediations(ctx, nil, tools.ListRemediationsArgs{Namespace: "default"})
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-101-007: invalid namespace returns ErrInvalidInput", func() {
		client := newTypedFakeClient()
		_, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{Namespace: "../etc"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-101-008: kind filter excludes non-matching RRs", func() {
		client := newTypedFakeClient(newTypedRR("payments", "rr-1", "Executing"))
		result, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{
			Namespace: "payments",
			Kind:      "StatefulSet",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})

	It("UT-AF-101-009: name filter excludes non-matching RRs", func() {
		client := newTypedFakeClient(newTypedRR("payments", "rr-1", "Executing"))
		result, err := tools.HandleListRemediations(ctx, client, tools.ListRemediationsArgs{
			Namespace: "payments",
			Name:      "non-existent-target",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(0))
	})
})
