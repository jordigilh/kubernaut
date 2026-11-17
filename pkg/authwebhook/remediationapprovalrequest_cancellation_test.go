package authwebhook_test

import (
	"context"
	"encoding/json"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var _ = Describe("RemediationApprovalRequest cancellation authorization", func() {
	var (
		handler   *authwebhook.RemediationApprovalRequestAuthHandler
		mockStore *MockAuditStore
	)

	BeforeEach(func() {
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		mockStore = &MockAuditStore{}
		handler = authwebhook.NewRemediationApprovalRequestAuthHandler(mockStore, nil, "kubernaut-system")
		Expect(handler.InjectDecoder(admission.NewDecoder(scheme))).To(Succeed())
	})

	buildRequest := func(username string, oldDecision remediationv1.ApprovalDecision) admission.Request {
		rr := &remediationv1.RemediationApprovalRequest{
			ObjectMeta: metav1.ObjectMeta{Name: "rar-cancel-auth", Namespace: "kubernaut-system"},
			Status: remediationv1.RemediationApprovalRequestStatus{
				Decision:  remediationv1.ApprovalDecisionCancelled,
				DecidedBy: "system",
			},
		}
		old := rr.DeepCopy()
		old.Status.Decision = oldDecision
		old.Status.DecidedBy = ""
		newRaw, err := json.Marshal(rr)
		Expect(err).NotTo(HaveOccurred())
		oldRaw, err := json.Marshal(old)
		Expect(err).NotTo(HaveOccurred())
		return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			Name:      rr.Name,
			Namespace: rr.Namespace,
			Operation: admissionv1.Update,
			UserInfo:  authv1.UserInfo{Username: username},
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		}}
	}

	It("UT-RAR-2588-001: rejects a user-supplied system cancellation", func() {
		response := handler.Handle(context.Background(), buildRequest("operator@example.com", remediationv1.ApprovalDecisionPending))
		Expect(response.Allowed).To(BeFalse())
		Expect(response.Result.Message).To(ContainSubstring("system-only"))
	})

	It("UT-RAR-2588-002: allows only the RO service account to cancel a pending RAR", func() {
		response := handler.Handle(context.Background(), buildRequest("system:serviceaccount:kubernaut-system:remediationorchestrator-controller", remediationv1.ApprovalDecisionPending))
		Expect(response.Allowed).To(BeTrue())
		Expect(mockStore.StoredEvents).To(HaveLen(1))
		event := mockStore.StoredEvents[0]
		Expect(event.EventType).To(Equal("webhook.remediationapprovalrequest.decided"))
		Expect(event.EventAction).To(Equal("approval_decided"))
		Expect(event.CorrelationID).To(Equal(""), "the fixture has no parent RR reference")
		Expect(event.ActorType.Value).To(Equal("service"))
	})

	It("UT-RAR-2588-004: attributes system cancellation to the RO caller", func() {
		response := handler.Handle(context.Background(), buildRequest("system:serviceaccount:kubernaut-system:remediationorchestrator-controller", remediationv1.ApprovalDecisionPending))
		Expect(response.Allowed).To(BeTrue())
		Expect(mockStore.StoredEvents).To(HaveLen(1))
		event := mockStore.StoredEvents[0]
		Expect(event.ActorType.Value).To(Equal("service"))
		Expect(event.ActorID.Value).To(Equal("system:serviceaccount:kubernaut-system:remediationorchestrator-controller"))
	})

	It("UT-RAR-2588-003: makes a repeated system cancellation idempotent", func() {
		response := handler.Handle(context.Background(), buildRequest("system:serviceaccount:kubernaut-system:remediationorchestrator-controller", remediationv1.ApprovalDecisionCancelled))
		Expect(response.Allowed).To(BeTrue())
	})
})
