package session_test

import (
	"context"
	"fmt"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut-apifrontend/internal/session"
)

var _ = Describe("CRDSessionService concurrency", func() {
	var (
		svc    *session.CRDSessionService
		ctx    context.Context
		scheme = newScheme()
	)

	BeforeEach(func() {
		k8s := newFakeClient(scheme)
		svc = newTestService(k8s, scheme)
		ctx = context.Background()
	})

	It("UT-AF-250-001: concurrent session creation does not corrupt crdIndex", func() {
		const n = 20
		var wg sync.WaitGroup
		errs := make([]error, n)

		wg.Add(n)
		for i := 0; i < n; i++ {
			go func(idx int) {
				defer wg.Done()
				req := createRequestWithDefaults(
					fmt.Sprintf("conc-sess-%d", idx),
					"jane.doe",
					createConfigState(),
				)
				_, errs[idx] = svc.Create(ctx, &req)
			}(i)
		}
		wg.Wait()

		for i, err := range errs {
			Expect(err).NotTo(HaveOccurred(), "session %d failed", i)
		}

		resp, err := svc.List(ctx, &adksession.ListRequest{
			AppName: "kubernaut-apifrontend",
			UserID:  "jane.doe",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Sessions).To(HaveLen(n))
	})

	It("UT-AF-250-002: TTL cancel and reconnect race yields consistent state", func() {
		req := createRequestWithDefaults("race-sess", "jane.doe", createConfigState())
		_, err := svc.Create(ctx, &req)
		Expect(err).NotTo(HaveOccurred())

		err = svc.UpdatePhase(ctx, "race-sess", v1alpha1.SessionPhaseDisconnected, "SSE dropped", "")
		Expect(err).NotTo(HaveOccurred())

		const attempts = 10
		var wg sync.WaitGroup
		cancelErrs := make([]error, attempts)
		reconnectErrs := make([]error, attempts)

		wg.Add(attempts * 2)
		for i := 0; i < attempts; i++ {
			go func(idx int) {
				defer wg.Done()
				cancelErrs[idx] = svc.UpdatePhase(ctx, "race-sess", v1alpha1.SessionPhaseCancelled, "TTL expired", "")
			}(i)
			go func(idx int) {
				defer wg.Done()
				reconnectErrs[idx] = svc.UpdatePhase(ctx, "race-sess", v1alpha1.SessionPhaseActive, "reconnected", "")
			}(i)
		}
		wg.Wait()

		phase, err := svc.GetSessionPhase(ctx, "race-sess")
		Expect(err).NotTo(HaveOccurred())
		Expect(phase).To(BeElementOf(
			v1alpha1.SessionPhaseActive,
			v1alpha1.SessionPhaseCancelled,
		), "final phase must be one of the two racing transitions")
	})
})
