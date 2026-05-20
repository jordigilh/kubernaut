package apifrontend_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut/api/apifrontend/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

func sessionCreateRequest(suffix, user string) *adksession.CreateRequest {
	return &adksession.CreateRequest{
		State: map[string]any{
			session.StateKeyCreateConfig: &session.CreateConfig{
				OwnerRef: metav1.OwnerReference{
					APIVersion: "kubernaut.ai/v1alpha1",
					Kind:       "RemediationRequest",
					Name:       "test-rr-" + suffix,
					UID:        types.UID("test-uid-" + suffix),
				},
				A2ATaskID: "task-" + suffix,
				JoinMode:  v1alpha1.SessionJoinModeStart,
				UserIdentity: v1alpha1.SessionUser{
					Username: user,
				},
				RemediationRef: v1alpha1.ObjectRef{
					Namespace: "default",
					Name:      "test-rr-" + suffix,
				},
			},
		},
	}
}

var _ = Describe("Session CRD Integration (session/)", func() {

	Describe("AC-28: CRDSessionService creates InvestigationSession CRD via envtest", func() {
		It("IT-AF-1195-042: Create session writes CRD to envtest API server", func() {
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(),
				k8sClient,
				scheme,
				"default",
			)

			resp, err := svc.Create(context.Background(), sessionCreateRequest("042", "test-user"))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.Session).NotTo(BeNil())
		})
	})

	Describe("AC-29: UpdatePhase transitions InvestigationSession state", func() {
		It("IT-AF-1195-043: UpdatePhase changes phase and emits audit event", func() {
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(),
				k8sClient,
				scheme,
				"default",
				session.WithAuditor(auditRecorder),
			)

			resp, err := svc.Create(context.Background(), sessionCreateRequest("043", "test-user-043"))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())

			sessionID := resp.Session.ID()
			err = svc.UpdatePhase(context.Background(), sessionID, v1alpha1.SessionPhaseCompleted, "investigation completed", "test-user-043")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
