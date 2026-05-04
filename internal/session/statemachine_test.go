package session_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/types"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut-apifrontend/internal/session"
)

var _ = Describe("State Machine", func() {
	Describe("ValidateTransition", func() {
		It("UT-AF-210-001: Active -> Completed", func() {
			err := session.ValidateTransition(v1alpha1.SessionPhaseActive, v1alpha1.SessionPhaseCompleted)
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-210-002: Active -> Cancelled", func() {
			err := session.ValidateTransition(v1alpha1.SessionPhaseActive, v1alpha1.SessionPhaseCancelled)
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-210-003: Active -> Failed", func() {
			err := session.ValidateTransition(v1alpha1.SessionPhaseActive, v1alpha1.SessionPhaseFailed)
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-210-004: Active -> Disconnected", func() {
			err := session.ValidateTransition(v1alpha1.SessionPhaseActive, v1alpha1.SessionPhaseDisconnected)
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-210-005: Disconnected -> Active", func() {
			err := session.ValidateTransition(v1alpha1.SessionPhaseDisconnected, v1alpha1.SessionPhaseActive)
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-210-006: Terminal phase rejects transition", func() {
			for _, terminal := range []v1alpha1.SessionPhase{
				v1alpha1.SessionPhaseCompleted,
				v1alpha1.SessionPhaseCancelled,
				v1alpha1.SessionPhaseFailed,
			} {
				err := session.ValidateTransition(terminal, v1alpha1.SessionPhaseActive)
				Expect(err).To(HaveOccurred(), "transition from %s should be rejected", terminal)
			}
		})

		It("UT-AF-210-007: Disconnected -> Cancelled", func() {
			err := session.ValidateTransition(v1alpha1.SessionPhaseDisconnected, v1alpha1.SessionPhaseCancelled)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UpdatePhase", func() {
		var (
			svc    *session.CRDSessionService
			scheme = newScheme()
			ctx    = context.Background()
		)

		BeforeEach(func() {
			k8s := newFakeClient(scheme)
			svc = newTestService(k8s, scheme)
		})

		createSession := func(id string) {
			req := session.CreateRequestWithDefaults(id, "jane.doe", createConfigState())
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())
		}

		It("UT-AF-210-008: updates CRD status and phase label", func() {
			k8s := newFakeClient(scheme)
			svc = newTestService(k8s, scheme)
			createSession("sess-phase")

			err := svc.UpdatePhase(ctx, "sess-phase", v1alpha1.SessionPhaseCompleted, "investigation done")
			Expect(err).NotTo(HaveOccurred())

			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "sess-phase", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			Expect(crd.Status.Phase).To(Equal(v1alpha1.SessionPhaseCompleted))
			Expect(crd.Status.Message).To(Equal("investigation done"))
			Expect(crd.Labels[session.LabelPhase]).To(Equal(string(v1alpha1.SessionPhaseCompleted)))
		})

		It("UT-AF-210-009: sets completedAt on terminal phase", func() {
			k8s := newFakeClient(scheme)
			svc = newTestService(k8s, scheme)
			createSession("sess-terminal")

			err := svc.UpdatePhase(ctx, "sess-terminal", v1alpha1.SessionPhaseCompleted, "done")
			Expect(err).NotTo(HaveOccurred())

			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "sess-terminal", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			Expect(crd.Status.CompletedAt).NotTo(BeNil())
		})

		It("UT-AF-210-010: sets disconnectedAt and reconnectedAt", func() {
			k8s := newFakeClient(scheme)
			svc = newTestService(k8s, scheme)
			createSession("sess-disconnect")

			// Disconnect
			err := svc.UpdatePhase(ctx, "sess-disconnect", v1alpha1.SessionPhaseDisconnected, "SSE dropped")
			Expect(err).NotTo(HaveOccurred())

			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "sess-disconnect", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			Expect(crd.Status.DisconnectedAt).NotTo(BeNil())

			// Reconnect
			err = svc.UpdatePhase(ctx, "sess-disconnect", v1alpha1.SessionPhaseActive, "reconnected")
			Expect(err).NotTo(HaveOccurred())

			err = k8s.Get(ctx, types.NamespacedName{Name: "sess-disconnect", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			Expect(crd.Status.ReconnectedAt).NotTo(BeNil())
			Expect(crd.Status.ConnectionState).To(Equal(v1alpha1.ConnectionStateConnected))
		})
	})
})
