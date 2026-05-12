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

var _ = Describe("kubernaut_submit_signal", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-103-001: creates SignalProcessing CRD", func() {
		client := newDynamicFakeClient()
		result, err := tools.HandleSubmitSignal(ctx, client, tools.SubmitSignalArgs{
			Namespace:   "payments",
			Kind:        "Deployment",
			Name:        "api-server",
			Description: "High error rate",
		}, "alice", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.SignalName).NotTo(BeEmpty())
	})

	It("UT-AF-103-002: populates spec fields from input", func() {
		var capturedObj *unstructured.Unstructured
		client := newDynamicFakeClient()
		client.PrependReactor("create", "signalprocessings", func(action k8stesting.Action) (bool, runtime.Object, error) {
			createAction, _ := action.(k8stesting.CreateAction)
			capturedObj, _ = createAction.GetObject().(*unstructured.Unstructured)
			return false, nil, nil
		})
		_, err := tools.HandleSubmitSignal(ctx, client, tools.SubmitSignalArgs{
			Namespace:   "payments",
			Kind:        "Deployment",
			Name:        "api-server",
			Description: "High error rate",
			Severity:    "critical",
		}, "alice", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(capturedObj).NotTo(BeNil())
		spec, _, _ := unstructured.NestedMap(capturedObj.Object, "spec")
		Expect(spec["kind"]).To(Equal("Deployment"))
		Expect(spec["name"]).To(Equal("api-server"))
	})

	It("UT-AF-103-003: sets reportedBy from JWT identity", func() {
		client := newDynamicFakeClient()
		result, err := tools.HandleSubmitSignal(ctx, client, tools.SubmitSignalArgs{
			Namespace:   "payments",
			Kind:        "Deployment",
			Name:        "api-server",
			Description: "CPU spike",
		}, "alice", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Message).To(ContainSubstring("alice"))
	})

	It("UT-AF-103-004: returns error on duplicate signal", func() {
		client := newDynamicFakeClient()
		client.PrependReactor("create", "signalprocessings", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, newConflictError("signalprocessings")
		})
		_, err := tools.HandleSubmitSignal(ctx, client, tools.SubmitSignalArgs{
			Namespace:   "payments",
			Kind:        "Deployment",
			Name:        "api-server",
			Description: "duplicate",
		}, "alice", nil)
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-103-005: returns 403 when user cannot create SP in namespace", func() {
		client := newDynamicFakeClient()
		client.PrependReactor("create", "signalprocessings", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, newForbiddenError("signalprocessings")
		})
		_, err := tools.HandleSubmitSignal(ctx, client, tools.SubmitSignalArgs{
			Namespace:   "forbidden",
			Kind:        "Deployment",
			Name:        "api-server",
			Description: "error",
		}, "alice", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("access denied"))
	})

	It("UT-AF-103-006: nil client returns ErrK8sUnavailable", func() {
		_, err := tools.HandleSubmitSignal(ctx, nil, tools.SubmitSignalArgs{
			Namespace:   "default",
			Kind:        "Deployment",
			Name:        "api-server",
			Description: "test",
		}, "alice", nil)
		Expect(err).To(MatchError(tools.ErrK8sUnavailable))
	})

	It("UT-AF-103-007: invalid namespace returns ErrInvalidInput", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleSubmitSignal(ctx, client, tools.SubmitSignalArgs{
			Namespace:   "../etc",
			Kind:        "Deployment",
			Name:        "api-server",
			Description: "test",
		}, "alice", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-103-008: empty kind returns ErrInvalidInput", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleSubmitSignal(ctx, client, tools.SubmitSignalArgs{
			Namespace:   "default",
			Kind:        "",
			Name:        "api-server",
			Description: "test",
		}, "alice", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-103-009: invalid severity returns ErrInvalidInput", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleSubmitSignal(ctx, client, tools.SubmitSignalArgs{
			Namespace:   "default",
			Kind:        "Deployment",
			Name:        "api-server",
			Description: "test",
			Severity:    "catastrophic",
		}, "alice", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})

	It("UT-AF-103-010: invalid resource name returns ErrInvalidInput", func() {
		client := newDynamicFakeClient()
		_, err := tools.HandleSubmitSignal(ctx, client, tools.SubmitSignalArgs{
			Namespace:   "default",
			Kind:        "Deployment",
			Name:        "INVALID NAME!!",
			Description: "test",
		}, "alice", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid input"))
	})
})
