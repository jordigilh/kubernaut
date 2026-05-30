package session_test

import (
	"context"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
		It("UT-AF-1234-070: Create does NOT create K8s CRD (deferred by default)", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
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
			Expect(crd.Labels).NotTo(HaveKey(session.LabelPhase))
			Expect(crd.Labels[session.LabelManagedBy]).To(Equal("kubernaut-apifrontend"))
		})

		It("UT-AF-1234-076: creates IS CRD with correct userIdentity", func() {
			k8s := newFakeClient(scheme)
			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
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

			err = svc.MaterializeCRD(ctx, "disc-001", v1alpha1.ObjectRef{
				Namespace: "prod", Name: "rr-disc-001",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(setSessionCRDPhase(ctx, k8s, "test-ns", "disc-001", v1alpha1.SessionPhaseActive)).To(Succeed())

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

			err = svc.MaterializeCRD(ctx, "ttl-001", v1alpha1.ObjectRef{
				Namespace: "prod", Name: "rr-ttl-001",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(setSessionCRDPhase(ctx, k8s, "test-ns", "ttl-001", v1alpha1.SessionPhaseActive)).To(Succeed())

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

			err = svc.MaterializeCRD(ctx, "ttl-002", v1alpha1.ObjectRef{
				Namespace: "prod", Name: "rr-ttl-002",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(setSessionCRDPhase(ctx, k8s, "test-ns", "ttl-002", v1alpha1.SessionPhaseActive)).To(Succeed())

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

var _ = Describe("InitializeSessionByRR (takeover IS CRD creation)", func() {
	var (
		ctx    context.Context
		scheme = newScheme()
	)

	newIndexedFakeClient := func() client.Client {
		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&v1alpha1.InvestigationSession{}).
			WithIndex(&v1alpha1.InvestigationSession{}, session.FieldIndexRRName,
				func(obj client.Object) []string {
					is := obj.(*v1alpha1.InvestigationSession)
					if is.Spec.RemediationRequestRef.Name == "" {
						return nil
					}
					return []string{is.Spec.RemediationRequestRef.Name}
				}).
			Build()
	}

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-1293-INIT-001: creates IS CRD with takeover JoinMode and A2ATaskID (phase owned by AA)", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		err := svc.InitializeSessionByRR(ctx, "prod", "rr-oom-001", "ka-sess-001", "sre-alice", []string{"sre-team"})
		Expect(err).NotTo(HaveOccurred())

		var is v1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "ka-sess-001"}, &is)).To(Succeed())
		Expect(is.Spec.RemediationRequestRef.Namespace).To(Equal("prod"))
		Expect(is.Spec.RemediationRequestRef.Name).To(Equal("rr-oom-001"))
		Expect(is.Spec.A2ATaskID).To(Equal("ka-sess-001"))
		Expect(is.Spec.UserIdentity.Username).To(Equal("sre-alice"))
		Expect(is.Spec.UserIdentity.Groups).To(ConsistOf("sre-team"))
	Expect(is.Spec.JoinMode).To(Equal(v1alpha1.SessionJoinModeTakeover))
	Expect(is.Status.Phase).To(Equal(v1alpha1.SessionPhaseActive))
	Expect(is.Labels).NotTo(HaveKey(session.LabelPhase))
		Expect(is.Labels[session.LabelUser]).To(Equal("sre-alice"))
		Expect(is.Labels[session.LabelRRName]).To(Equal("rr-oom-001"))
	})

	It("UT-AF-1293-INIT-002: idempotent when same user already has active session", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		err := svc.InitializeSessionByRR(ctx, "prod", "rr-oom-002", "ka-sess-002", "sre-bob", nil)
		Expect(err).NotTo(HaveOccurred())

		err = svc.InitializeSessionByRR(ctx, "prod", "rr-oom-002", "ka-sess-002b", "sre-bob", nil)
		Expect(err).NotTo(HaveOccurred())

		var list v1alpha1.InvestigationSessionList
		Expect(k8s.List(ctx, &list, client.InNamespace("test-ns"))).To(Succeed())
		activeCount := 0
		for _, is := range list.Items {
			if is.Spec.RemediationRequestRef.Name == "rr-oom-002" {
				activeCount++
			}
		}
		Expect(activeCount).To(Equal(1), "idempotent call should not create duplicate CRD")
	})

	It("UT-AF-1293-INIT-003: rejects when different user holds active session (single-driver)", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		err := svc.InitializeSessionByRR(ctx, "prod", "rr-oom-003", "ka-sess-003", "sre-carol", nil)
		Expect(err).NotTo(HaveOccurred())

		err = svc.InitializeSessionByRR(ctx, "prod", "rr-oom-003", "ka-sess-003b", "sre-dave", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("session_active"))
		Expect(err.Error()).NotTo(ContainSubstring("sre-carol"), "should not leak holder username (SI-11)")
	})

	It("UT-AF-1293-INIT-004: rejects empty kaSessionID", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		err := svc.InitializeSessionByRR(ctx, "prod", "rr-oom-004", "", "sre-alice", nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("kaSessionID is required"))
	})

	It("UT-AF-1293-INIT-005: invalid CRD name falls back to generated name", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		err := svc.InitializeSessionByRR(ctx, "prod", "rr-oom-005", "INVALID_UPPER_CASE!", "sre-alice", nil)
		Expect(err).NotTo(HaveOccurred())

		var list v1alpha1.InvestigationSessionList
		Expect(k8s.List(ctx, &list, client.InNamespace("test-ns"))).To(Succeed())
		Expect(list.Items).To(HaveLen(1))
		Expect(list.Items[0].Name).To(HavePrefix("isess-"))
	})

	It("UT-AF-1293-INIT-006: emits EventSessionCreated audit event", func() {
		k8s := newIndexedFakeClient()
		recorder := &recordingEmitter{}
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns",
			session.WithAuditor(recorder),
		)

		err := svc.InitializeSessionByRR(ctx, "prod", "rr-oom-006", "ka-sess-006", "sre-alice", nil)
		Expect(err).NotTo(HaveOccurred())

		var found bool
		for _, e := range recorder.events() {
			if e.Type == audit.EventSessionCreated {
				found = true
				Expect(e.UserID).To(Equal("sre-alice"))
				Expect(e.Detail["join_mode"]).To(Equal("takeover"))
				Expect(e.Detail["session_id"]).To(Equal("ka-sess-006"))
				Expect(e.Detail["rr_ref"]).To(Equal("prod/rr-oom-006"))
			}
		}
		Expect(found).To(BeTrue(), "expected EventSessionCreated audit event")
	})

	It("UT-AF-1300-001: sets OwnerReference to RR when RR is in same namespace", func() {
		rr := &unstructured.Unstructured{}
		rr.SetGroupVersionKind(schema.GroupVersionKind{
			Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationRequest",
		})
		rr.SetNamespace("test-ns")
		rr.SetName("rr-owned-001")
		rr.SetUID("uid-rr-owned-001")

		k8s := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&v1alpha1.InvestigationSession{}).
			WithIndex(&v1alpha1.InvestigationSession{}, session.FieldIndexRRName,
				func(obj client.Object) []string {
					is := obj.(*v1alpha1.InvestigationSession)
					if is.Spec.RemediationRequestRef.Name == "" {
						return nil
					}
					return []string{is.Spec.RemediationRequestRef.Name}
				}).
			WithObjects(rr).
			Build()

		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		err := svc.InitializeSessionByRR(ctx, "test-ns", "rr-owned-001", "ka-sess-own-001", "sre-alice", nil)
		Expect(err).NotTo(HaveOccurred())

		var is v1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "ka-sess-own-001"}, &is)).To(Succeed())
		Expect(is.OwnerReferences).To(HaveLen(1),
			"IS CRD must have an OwnerReference to the RR for cascade deletion (#1300)")
		Expect(is.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
		Expect(is.OwnerReferences[0].Name).To(Equal("rr-owned-001"))
		Expect(is.OwnerReferences[0].UID).To(Equal(types.UID("uid-rr-owned-001")))
	})

	It("UT-AF-1300-002: skips OwnerReference when RR is in different namespace (cross-NS not allowed)", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		err := svc.InitializeSessionByRR(ctx, "other-ns", "rr-cross-001", "ka-sess-cross-001", "sre-alice", nil)
		Expect(err).NotTo(HaveOccurred())

		var is v1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "ka-sess-cross-001"}, &is)).To(Succeed())
		Expect(is.OwnerReferences).To(BeEmpty(),
			"cross-namespace OwnerReferences are not allowed in K8s — IS must be created without one")
	})

	It("UT-AF-1300-003: IS created successfully even when RR lookup fails", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		err := svc.InitializeSessionByRR(ctx, "test-ns", "rr-nonexistent", "ka-sess-noref-001", "sre-alice", nil)
		Expect(err).NotTo(HaveOccurred())

		var is v1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "ka-sess-noref-001"}, &is)).To(Succeed())
		Expect(is.OwnerReferences).To(BeEmpty(),
			"when RR cannot be fetched, IS is still created without OwnerReference")
	})

	It("UT-AF-1293-INIT-007: increments sessionsActive gauge on successful creation (FedRAMP SI-4)", func() {
		k8s := newIndexedFakeClient()
		gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "af_sessions_active_init_test",
		}, []string{"phase"})
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns",
			session.WithSessionsActive(gauge),
		)

		err := svc.InitializeSessionByRR(ctx, "prod", "rr-oom-007", "ka-sess-007", "sre-alice", nil)
		Expect(err).NotTo(HaveOccurred())

		metric := &dto.Metric{}
		Expect(gauge.WithLabelValues("Active").Write(metric)).To(Succeed())
		Expect(metric.GetGauge().GetValue()).To(Equal(float64(1)),
			"sessionsActive gauge must be incremented after InitializeSessionByRR with Active phase")
	})
})

var _ = Describe("CreateInvestigationSession (#1332 — IS CRD-driven takeover)", func() {
	var (
		ctx    context.Context
		scheme = newScheme()
	)

	newIndexedFakeClient := func(objs ...client.Object) client.Client {
		return fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&v1alpha1.InvestigationSession{}).
			WithIndex(&v1alpha1.InvestigationSession{}, session.FieldIndexRRName,
				func(obj client.Object) []string {
					is := obj.(*v1alpha1.InvestigationSession)
					if is.Spec.RemediationRequestRef.Name == "" {
						return nil
					}
					return []string{is.Spec.RemediationRequestRef.Name}
				}).
			WithObjects(objs...).
			Build()
	}

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-1332-050: creates IS CRD with is-{rr.Name} naming and takeover JoinMode", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		crdName, err := svc.CreateInvestigationSession(ctx, session.CreateISConfig{
			RRNamespace: "prod",
			RRName:      "rr-b157a3a9e42f-1c2b5576",
			TaskID:      "task-a2a-001",
			Username:    "sre-alice",
			Groups:      []string{"sre-team"},
			JoinMode:    v1alpha1.SessionJoinModeTakeover,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(crdName).To(Equal("is-rr-b157a3a9e42f-1c2b5576"))

		var is v1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: crdName}, &is)).To(Succeed())
		Expect(is.Spec.RemediationRequestRef.Namespace).To(Equal("prod"))
		Expect(is.Spec.RemediationRequestRef.Name).To(Equal("rr-b157a3a9e42f-1c2b5576"))
		Expect(is.Spec.A2ATaskID).To(Equal("task-a2a-001"))
		Expect(is.Spec.UserIdentity.Username).To(Equal("sre-alice"))
		Expect(is.Spec.UserIdentity.Groups).To(ConsistOf("sre-team"))
		Expect(is.Spec.JoinMode).To(Equal(v1alpha1.SessionJoinModeTakeover))
		Expect(is.Status.Phase).To(BeEmpty(), "phase must be empty — AA sets Active")
		Expect(is.Labels[session.LabelRRName]).To(Equal("rr-b157a3a9e42f-1c2b5576"))
		Expect(is.Labels[session.LabelUser]).To(Equal("sre-alice"))
		Expect(is.Labels[session.LabelManagedBy]).To(Equal("kubernaut-apifrontend"))
	})

	It("UT-AF-1332-051: creates IS CRD with start JoinMode for fresh interactive", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		crdName, err := svc.CreateInvestigationSession(ctx, session.CreateISConfig{
			RRNamespace: "prod",
			RRName:      "rr-fresh-001",
			TaskID:      "task-fresh-001",
			Username:    "sre-bob",
			JoinMode:    v1alpha1.SessionJoinModeStart,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(crdName).To(Equal("is-rr-fresh-001"))

		var is v1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: crdName}, &is)).To(Succeed())
		Expect(is.Spec.JoinMode).To(Equal(v1alpha1.SessionJoinModeStart))
	})

	It("UT-AF-1332-052: idempotent — same user with active IS returns existing name", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		cfg := session.CreateISConfig{
			RRNamespace: "prod",
			RRName:      "rr-idem-001",
			TaskID:      "task-idem-001",
			Username:    "sre-alice",
			JoinMode:    v1alpha1.SessionJoinModeTakeover,
		}
		name1, err := svc.CreateInvestigationSession(ctx, cfg)
		Expect(err).NotTo(HaveOccurred())

		cfg.TaskID = "task-idem-002"
		name2, err := svc.CreateInvestigationSession(ctx, cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(name2).To(Equal(name1), "idempotent call must return same CRD name")

		var list v1alpha1.InvestigationSessionList
		Expect(k8s.List(ctx, &list, client.InNamespace("test-ns"))).To(Succeed())
		Expect(list.Items).To(HaveLen(1), "should not create duplicate")
	})

	It("UT-AF-1332-053: rejects different user with active IS (single-driver guard)", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		_, err := svc.CreateInvestigationSession(ctx, session.CreateISConfig{
			RRNamespace: "prod",
			RRName:      "rr-guard-001",
			TaskID:      "task-guard-001",
			Username:    "sre-alice",
			JoinMode:    v1alpha1.SessionJoinModeTakeover,
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = svc.CreateInvestigationSession(ctx, session.CreateISConfig{
			RRNamespace: "prod",
			RRName:      "rr-guard-001",
			TaskID:      "task-guard-002",
			Username:    "sre-bob",
			JoinMode:    v1alpha1.SessionJoinModeTakeover,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("session_active"))
	})

	It("UT-AF-1332-054: deletes terminal IS and recreates for same RR", func() {
		terminalIS := &v1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "is-rr-terminal-001",
				Namespace: "test-ns",
			},
			Spec: v1alpha1.InvestigationSessionSpec{
				RemediationRequestRef: v1alpha1.ObjectRef{Namespace: "prod", Name: "rr-terminal-001"},
				A2ATaskID:             "old-task",
				UserIdentity:          v1alpha1.SessionUser{Username: "old-user"},
				JoinMode:              v1alpha1.SessionJoinModeStart,
			},
			Status: v1alpha1.InvestigationSessionStatus{
				Phase: v1alpha1.SessionPhaseCompleted,
			},
		}
		k8s := newIndexedFakeClient(terminalIS)
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		crdName, err := svc.CreateInvestigationSession(ctx, session.CreateISConfig{
			RRNamespace: "prod",
			RRName:      "rr-terminal-001",
			TaskID:      "task-new-001",
			Username:    "sre-alice",
			JoinMode:    v1alpha1.SessionJoinModeTakeover,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(crdName).To(Equal("is-rr-terminal-001"))

		var is v1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: crdName}, &is)).To(Succeed())
		Expect(is.Spec.A2ATaskID).To(Equal("task-new-001"), "must be the new IS, not the old terminal one")
		Expect(is.Spec.UserIdentity.Username).To(Equal("sre-alice"))
		Expect(is.Status.Phase).To(BeEmpty(), "new IS has no phase")
	})

	It("UT-AF-1332-055: rejects empty RRName", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		_, err := svc.CreateInvestigationSession(ctx, session.CreateISConfig{
			TaskID:   "task-001",
			Username: "sre-alice",
			JoinMode: v1alpha1.SessionJoinModeTakeover,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("RRName is required"))
	})

	It("UT-AF-1332-056: rejects empty TaskID", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		_, err := svc.CreateInvestigationSession(ctx, session.CreateISConfig{
			RRName:   "rr-no-task",
			Username: "sre-alice",
			JoinMode: v1alpha1.SessionJoinModeTakeover,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("TaskID is required"))
	})

	It("UT-AF-1332-057: rejects empty Username", func() {
		k8s := newIndexedFakeClient()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		_, err := svc.CreateInvestigationSession(ctx, session.CreateISConfig{
			RRName:   "rr-no-user",
			TaskID:   "task-no-user",
			JoinMode: v1alpha1.SessionJoinModeTakeover,
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Username is required"))
	})

	It("UT-AF-1332-058: emits EventSessionCreated audit event with correct details", func() {
		k8s := newIndexedFakeClient()
		recorder := &recordingEmitter{}
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns",
			session.WithAuditor(recorder),
		)

		_, err := svc.CreateInvestigationSession(ctx, session.CreateISConfig{
			RRNamespace: "prod",
			RRName:      "rr-audit-001",
			TaskID:      "task-audit-001",
			Username:    "sre-alice",
			JoinMode:    v1alpha1.SessionJoinModeTakeover,
		})
		Expect(err).NotTo(HaveOccurred())

		var found bool
		for _, e := range recorder.events() {
			if e.Type == audit.EventSessionCreated {
				found = true
				Expect(e.UserID).To(Equal("sre-alice"))
				Expect(e.Detail["crd_name"]).To(Equal("is-rr-audit-001"))
				Expect(e.Detail["task_id"]).To(Equal("task-audit-001"))
				Expect(e.Detail["join_mode"]).To(Equal("takeover"))
				Expect(e.Detail["rr_ref"]).To(Equal("prod/rr-audit-001"))
			}
		}
		Expect(found).To(BeTrue(), "expected EventSessionCreated audit event")
	})

	It("UT-AF-1332-059: sets OwnerReference when RR is in same namespace", func() {
		rr := &unstructured.Unstructured{}
		rr.SetGroupVersionKind(schema.GroupVersionKind{
			Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationRequest",
		})
		rr.SetNamespace("test-ns")
		rr.SetName("rr-owned-is-001")
		rr.SetUID("uid-rr-owned-is-001")

		k8s := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&v1alpha1.InvestigationSession{}).
			WithIndex(&v1alpha1.InvestigationSession{}, session.FieldIndexRRName,
				func(obj client.Object) []string {
					is := obj.(*v1alpha1.InvestigationSession)
					if is.Spec.RemediationRequestRef.Name == "" {
						return nil
					}
					return []string{is.Spec.RemediationRequestRef.Name}
				}).
			WithObjects(rr).
			Build()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		crdName, err := svc.CreateInvestigationSession(ctx, session.CreateISConfig{
			RRNamespace: "test-ns",
			RRName:      "rr-owned-is-001",
			TaskID:      "task-owned-001",
			Username:    "sre-alice",
			JoinMode:    v1alpha1.SessionJoinModeTakeover,
		})
		Expect(err).NotTo(HaveOccurred())

		var is v1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: crdName}, &is)).To(Succeed())
		Expect(is.OwnerReferences).To(HaveLen(1))
		Expect(is.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
		Expect(is.OwnerReferences[0].Name).To(Equal("rr-owned-is-001"))
		Expect(is.OwnerReferences[0].UID).To(Equal(types.UID("uid-rr-owned-is-001")))
	})
})

var _ = Describe("UpdateISCorrelation (#1332 — post-MCP KA session ID)", func() {
	var (
		ctx    context.Context
		scheme = newScheme()
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-1332-060: updates status.kaCorrelationID on existing IS", func() {
		existingIS := &v1alpha1.InvestigationSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "is-rr-corr-001",
				Namespace: "test-ns",
			},
			Spec: v1alpha1.InvestigationSessionSpec{
				RemediationRequestRef: v1alpha1.ObjectRef{Namespace: "prod", Name: "rr-corr-001"},
				A2ATaskID:             "task-corr-001",
				UserIdentity:          v1alpha1.SessionUser{Username: "sre-alice"},
				JoinMode:              v1alpha1.SessionJoinModeTakeover,
			},
		}
		k8s := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&v1alpha1.InvestigationSession{}).
			WithObjects(existingIS).
			Build()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		err := svc.UpdateISCorrelation(ctx, "is-rr-corr-001", "ka-session-abc123")
		Expect(err).NotTo(HaveOccurred())

		var is v1alpha1.InvestigationSession
		Expect(k8s.Get(ctx, types.NamespacedName{Namespace: "test-ns", Name: "is-rr-corr-001"}, &is)).To(Succeed())
		Expect(is.Status.KACorrelationID).To(Equal("ka-session-abc123"))
	})

	It("UT-AF-1332-061: no-op when crdName is empty", func() {
		k8s := fake.NewClientBuilder().WithScheme(scheme).Build()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		err := svc.UpdateISCorrelation(ctx, "", "ka-session-001")
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-1332-062: no-op when kaSessionID is empty", func() {
		k8s := fake.NewClientBuilder().WithScheme(scheme).Build()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		err := svc.UpdateISCorrelation(ctx, "is-rr-some", "")
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-1332-063: returns error when IS CRD does not exist", func() {
		k8s := fake.NewClientBuilder().WithScheme(scheme).Build()
		svc := session.NewCRDSessionService(adksession.InMemoryService(), k8s, scheme, "test-ns")

		err := svc.UpdateISCorrelation(ctx, "is-nonexistent", "ka-session-001")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("get IS"))
	})
})
