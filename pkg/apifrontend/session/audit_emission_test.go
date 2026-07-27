package session_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

var _ = Describe("Audit event emission – session (PR2 wiring)", func() {
	It("UT-AF-1156-057: emits session.completed when phase transitions to terminal", func() {
		scheme := newScheme()
		k8s := newFakeClient(scheme)
		recorder := &recordingEmitter{}
		svc := session.NewCRDSessionService(
			adksession.InMemoryService(), k8s, scheme, "test-ns",
			session.WithAuditor(recorder),
		)
		ctx := context.Background()

		req := createRequestWithDefaults("sess-completed-audit", "jane.doe", createConfigState())
		_, err := svc.Create(ctx, &req)
		Expect(err).NotTo(HaveOccurred())

		err = svc.MaterializeCRD(ctx, "sess-completed-audit", v1alpha1.ObjectRef{Name: "rr-audit", Namespace: "test-ns"})
		Expect(err).NotTo(HaveOccurred())
		Expect(setSessionCRDPhase(ctx, k8s, "sess-completed-audit")).To(Succeed())

		err = svc.UpdatePhase(ctx, "sess-completed-audit", v1alpha1.SessionPhaseCompleted, "investigation complete", "jane.doe")
		Expect(err).NotTo(HaveOccurred())

		var completedEvents []*audit.Event
		for _, e := range recorder.events() {
			if e.Type == audit.EventSessionCompleted {
				completedEvents = append(completedEvents, e)
			}
		}
		Expect(completedEvents).To(HaveLen(1), "expected exactly one session.completed event")
		Expect(completedEvents[0].UserID).To(Equal("jane.doe"))
		Expect(completedEvents[0].Detail).To(HaveKeyWithValue("session_id", "sess-completed-audit"))
		Expect(completedEvents[0].Detail).To(HaveKeyWithValue("terminal_phase", "Completed"))
	})

	It("UT-AF-1156-058: does NOT emit session.completed for non-terminal transitions", func() {
		scheme := newScheme()
		k8s := newFakeClient(scheme)
		recorder := &recordingEmitter{}
		svc := session.NewCRDSessionService(
			adksession.InMemoryService(), k8s, scheme, "test-ns",
			session.WithAuditor(recorder),
		)
		ctx := context.Background()

		req := createRequestWithDefaults("sess-nonterminal", "jane.doe", createConfigState())
		_, err := svc.Create(ctx, &req)
		Expect(err).NotTo(HaveOccurred())

		err = svc.MaterializeCRD(ctx, "sess-nonterminal", v1alpha1.ObjectRef{Name: "rr-nt", Namespace: "test-ns"})
		Expect(err).NotTo(HaveOccurred())
		Expect(setSessionCRDPhase(ctx, k8s, "sess-nonterminal")).To(Succeed())

		err = svc.UpdatePhase(ctx, "sess-nonterminal", v1alpha1.SessionPhaseDisconnected, "SSE dropped", "jane.doe")
		Expect(err).NotTo(HaveOccurred())

		for _, e := range recorder.events() {
			Expect(e.Type).NotTo(Equal(audit.EventSessionCompleted),
				"session.completed should not be emitted for non-terminal phase transition")
		}
	})
})
