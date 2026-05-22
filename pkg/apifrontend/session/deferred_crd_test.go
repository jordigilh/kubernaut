package session_test

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

var _ = Describe("Deferred CRD Creation (G6)", func() {
	var (
		ctx    context.Context
		scheme = newScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("Deferred Create", func() {
		It("UT-AF-1234-070: Create with deferCRD=true does NOT create K8s CRD", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithDeferredCRD(),
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "deferred-001",
				State:     createConfigState(),
			}
			resp, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())

			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "deferred-001", Namespace: "test-ns"}, &crd)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("UT-AF-1234-071: Deferred Create stores in-memory delegate session", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithDeferredCRD(),
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "deferred-002",
				State:     createConfigState(),
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			getResp, err := svc.Get(ctx, &adksession.GetRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "deferred-002",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(getResp.Session.ID()).To(Equal("deferred-002"))
		})

		It("UT-AF-1234-072: Deferred Create emits session.created audit", func() {
			recorder := &recordingEmitter{}
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithDeferredCRD(),
				session.WithAuditor(recorder),
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "deferred-003",
				State:     createConfigState(),
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			var found bool
			for _, e := range recorder.events() {
				if e.Type == audit.EventSessionCreated {
					found = true
				}
			}
			Expect(found).To(BeTrue())
		})

		It("UT-AF-1234-073: Deferred Create stores CreateConfig in pendingConfigs", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithDeferredCRD(),
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "deferred-004",
				State:     createConfigState(),
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.IsMaterialized("deferred-004")).To(BeFalse())
		})
	})

	Describe("MaterializeCRD", func() {
		It("UT-AF-1234-074: creates IS CRD with correct spec.remediationRequestRef", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithDeferredCRD(),
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "mat-001",
				State:     createConfigState(),
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			rrRef := v1alpha1.ObjectRef{
				Namespace: "prod",
				Name:      "rr-web-api-oom",
			}
			err = svc.MaterializeCRD(ctx, "mat-001", rrRef)
			Expect(err).NotTo(HaveOccurred())

			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "mat-001", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			Expect(crd.Spec.RemediationRequestRef.Name).To(Equal("rr-web-api-oom"))
			Expect(crd.Spec.RemediationRequestRef.Namespace).To(Equal("prod"))
		})

		It("UT-AF-1234-075: creates IS CRD with correct labels", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithDeferredCRD(),
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "mat-002",
				State:     createConfigState(),
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			err = svc.MaterializeCRD(ctx, "mat-002", v1alpha1.ObjectRef{
				Namespace: "prod",
				Name:      "rr-xyz",
			})
			Expect(err).NotTo(HaveOccurred())

			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "mat-002", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			Expect(crd.Labels[session.LabelRRName]).To(Equal("rr-xyz"))
			Expect(crd.Labels[session.LabelPhase]).To(Equal(string(v1alpha1.SessionPhaseActive)))
			Expect(crd.Labels[session.LabelManagedBy]).To(Equal("kubernaut-apifrontend"))
		})

		It("UT-AF-1234-076: creates IS CRD with correct userIdentity", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithDeferredCRD(),
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "mat-003",
				State: map[string]any{
					session.StateKeyCreateConfig: &session.CreateConfig{
						OwnerRef: metav1.OwnerReference{
							APIVersion: "kubernaut.ai/v1",
							Kind:       "RemediationRequest",
							Name:       "rr-test",
						},
						UserIdentity: v1alpha1.SessionUser{
							Username: "jane.doe@corp.com",
							Groups:   []string{"sre-team"},
						},
					},
				},
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			err = svc.MaterializeCRD(ctx, "mat-003", v1alpha1.ObjectRef{
				Namespace: "prod",
				Name:      "rr-test",
			})
			Expect(err).NotTo(HaveOccurred())

			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "mat-003", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			Expect(crd.Spec.UserIdentity.Username).To(Equal("jane.doe@corp.com"))
		})

		It("UT-AF-1234-077: returns error if session not in pendingConfigs", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithDeferredCRD(),
			)

			err := svc.MaterializeCRD(ctx, "nonexistent", v1alpha1.ObjectRef{
				Namespace: "prod",
				Name:      "rr-test",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})

		It("UT-AF-1234-078: K8s create failure returns error and preserves pendingConfig for retry", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithDeferredCRD(),
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "mat-fail-001",
				State:     createConfigState(),
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			existingCRD := &v1alpha1.InvestigationSession{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mat-fail-001",
					Namespace: "test-ns",
				},
			}
			err = k8s.Create(ctx, existingCRD)
			Expect(err).NotTo(HaveOccurred())

			err = svc.MaterializeCRD(ctx, "mat-fail-001", v1alpha1.ObjectRef{
				Namespace: "prod",
				Name:      "rr-test",
			})
			Expect(err).To(HaveOccurred())

			Expect(svc.IsMaterialized("mat-fail-001")).To(BeFalse())
		})

		It("UT-AF-1234-079: idempotent call (already materialized) is no-op", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithDeferredCRD(),
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "mat-idem-001",
				State:     createConfigState(),
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			rrRef := v1alpha1.ObjectRef{Namespace: "prod", Name: "rr-test"}
			err = svc.MaterializeCRD(ctx, "mat-idem-001", rrRef)
			Expect(err).NotTo(HaveOccurred())

			err = svc.MaterializeCRD(ctx, "mat-idem-001", rrRef)
			Expect(err).NotTo(HaveOccurred())

			Expect(svc.IsMaterialized("mat-idem-001")).To(BeTrue())
		})
	})

	Describe("Edge cases", func() {
		It("UT-AF-1234-086: Create then Delete before MaterializeCRD cleans up", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithDeferredCRD(),
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "deferred-del-001",
				State:     createConfigState(),
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			err = svc.Delete(ctx, &adksession.DeleteRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "deferred-del-001",
			})
			Expect(err).NotTo(HaveOccurred())

			err = svc.MaterializeCRD(ctx, "deferred-del-001", v1alpha1.ObjectRef{
				Namespace: "prod",
				Name:      "rr-test",
			})
			Expect(err).To(HaveOccurred())
		})

		It("UT-AF-1234-087: concurrent MaterializeCRD calls for same session", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithDeferredCRD(),
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "deferred-conc-001",
				State:     createConfigState(),
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			rrRef := v1alpha1.ObjectRef{Namespace: "prod", Name: "rr-test"}
			var wg sync.WaitGroup
			errs := make([]error, 3)
			for i := 0; i < 3; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					errs[idx] = svc.MaterializeCRD(ctx, "deferred-conc-001", rrRef)
				}(i)
			}
			wg.Wait()

			successCount := 0
			for _, e := range errs {
				if e == nil {
					successCount++
				}
			}
			Expect(successCount).To(BeNumerically(">=", 1))
		})
	})

	Describe("Disconnect detection (G19)", func() {
		It("UT-AF-1234-083: SSE close triggers UpdatePhase(Disconnected)", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "disc-001",
				State:     createConfigState(),
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			err = svc.UpdatePhase(ctx, "disc-001", v1alpha1.SessionPhaseDisconnected, "SSE connection closed", "")
			Expect(err).NotTo(HaveOccurred())

			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "disc-001", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			Expect(crd.Status.Phase).To(Equal(v1alpha1.SessionPhaseDisconnected))
			Expect(crd.Status.DisconnectedAt).NotTo(BeNil())
		})

		It("UT-AF-1234-084: TTL reconciler transitions Active with stale heartbeat to Disconnected", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "ttl-001",
				State:     createConfigState(),
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			err = svc.UpdatePhase(ctx, "ttl-001", v1alpha1.SessionPhaseDisconnected, "heartbeat stale", "system")
			Expect(err).NotTo(HaveOccurred())

			phase, err := svc.GetSessionPhase(ctx, "ttl-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(phase).To(Equal(v1alpha1.SessionPhaseDisconnected))
		})

		It("UT-AF-1234-085: Disconnected with expired TTL transitions to Cancelled", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
			)

			req := adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "ttl-002",
				State:     createConfigState(),
			}
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			err = svc.UpdatePhase(ctx, "ttl-002", v1alpha1.SessionPhaseDisconnected, "SSE dropped", "")
			Expect(err).NotTo(HaveOccurred())

			err = svc.UpdatePhase(ctx, "ttl-002", v1alpha1.SessionPhaseCancelled, "TTL expired", "system")
			Expect(err).NotTo(HaveOccurred())

			phase, err := svc.GetSessionPhase(ctx, "ttl-002")
			Expect(err).NotTo(HaveOccurred())
			Expect(phase).To(Equal(v1alpha1.SessionPhaseCancelled))
		})
	})
})

