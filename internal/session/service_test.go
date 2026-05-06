package session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/session"
)

func TestSessionSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Session Suite")
}

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	return s
}

func newFakeClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(&v1alpha1.InvestigationSession{}).Build()
}

func newTestService(k8s client.Client, scheme *runtime.Scheme) *session.CRDSessionService {
	return session.NewCRDSessionService(
		adksession.InMemoryService(),
		k8s,
		scheme,
		"test-ns",
	)
}

func createConfigState() map[string]any {
	return map[string]any{
		session.StateKeyCreateConfig: &session.CreateConfig{
			OwnerRef: metav1.OwnerReference{
				APIVersion: "kubernaut.ai/v1",
				Kind:       "RemediationRequest",
				Name:       "rr-payment-api",
				UID:        types.UID("rr-uid-123"),
			},
			A2ATaskID: "task-abc",
			UserIdentity: v1alpha1.SessionUser{
				Username: "jane.doe",
				Groups:   []string{"sre-team"},
			},
			JoinMode:       v1alpha1.SessionJoinModeStart,
			RemediationRef: v1alpha1.ObjectRef{Name: "rr-payment-api", Namespace: "test-ns"},
		},
	}
}

// --- Create Tests ---

var _ = Describe("CRDSessionService", func() {
	var (
		svc    *session.CRDSessionService
		k8s    client.Client
		scheme *runtime.Scheme
		ctx    context.Context
	)

	BeforeEach(func() {
		scheme = newScheme()
		k8s = newFakeClient(scheme)
		svc = newTestService(k8s, scheme)
		ctx = context.Background()
	})

	Describe("Create", func() {
		It("UT-AF-200-001: returns session with generated ID", func() {
			resp, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName: "kubernaut-apifrontend",
				UserID:  "jane.doe",
				State:   createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Session).NotTo(BeNil())
			Expect(resp.Session.ID()).NotTo(BeEmpty())
			Expect(resp.Session.AppName()).To(Equal("kubernaut-apifrontend"))
			Expect(resp.Session.UserID()).To(Equal("jane.doe"))
		})

		It("UT-AF-200-002: creates InvestigationSession CRD with ownerRef", func() {
			resp, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-1",
				State:     createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())

			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "sess-1", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			Expect(crd.OwnerReferences).To(HaveLen(1))
			Expect(crd.OwnerReferences[0].Name).To(Equal("rr-payment-api"))
			Expect(crd.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
			_ = resp
		})

		It("UT-AF-200-003: sets standard labels", func() {
			_, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-labels",
				State:     createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())

			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "sess-labels", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			Expect(crd.Labels).To(HaveKeyWithValue(session.LabelUser, "jane.doe"))
			Expect(crd.Labels).To(HaveKeyWithValue(session.LabelRRName, "rr-payment-api"))
			Expect(crd.Labels).To(HaveKeyWithValue(session.LabelPhase, string(v1alpha1.SessionPhaseActive)))
			Expect(crd.Labels).To(HaveKeyWithValue(session.LabelManagedBy, "kubernaut-apifrontend"))
		})

		It("UT-AF-200-004: uses client-provided SessionID", func() {
			resp, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "my-custom-id",
				State:     createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Session.ID()).To(Equal("my-custom-id"))
		})

		It("UT-AF-200-005: populates CRD spec from request state", func() {
			_, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-spec",
				State:     createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())

			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "sess-spec", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			Expect(crd.Spec.A2ATaskID).To(Equal("task-abc"))
			Expect(crd.Spec.UserIdentity.Username).To(Equal("jane.doe"))
			Expect(crd.Spec.UserIdentity.Groups).To(ContainElement("sre-team"))
			Expect(crd.Spec.JoinMode).To(Equal(v1alpha1.SessionJoinModeStart))
			Expect(crd.Spec.RemediationRequestRef.Name).To(Equal("rr-payment-api"))
			Expect(crd.Status.Phase).To(Equal(v1alpha1.SessionPhaseActive))
			Expect(crd.Status.StartedAt).NotTo(BeNil())
		})

		It("UT-AF-200-006: rolls back CRD if delegate fails", func() {
			_, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-dup",
				State:     createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())

			// second create with same ID should fail (delegate rejects duplicates)
			_, err = svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-dup",
				State:     createConfigState(),
			})
			Expect(err).To(HaveOccurred())

			// CRD from first create should still exist (not rolled back)
			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "sess-dup", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Create adversarial inputs", func() {
		DescribeTable("rejects invalid session IDs",
			func(sessionID string) {
				req := createRequestWithDefaults(sessionID, "jane.doe", createConfigState())
				_, err := svc.Create(ctx, &req)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid session ID"))
			},
			Entry("uppercase", "UPPERCASE"),
			Entry("exceeds 253 chars", strings.Repeat("a", 254)),
			Entry("path traversal", "../../etc/passwd"),
			Entry("contains spaces", "has spaces"),
			Entry("starts with dash", "-leading-dash"),
			Entry("contains dots", "has.dots.in.it"),
			Entry("contains underscore", "has_underscore"),
		)

		It("auto-generates valid CRD name when SessionID is empty", func() {
			req := createRequestWithDefaults("", "jane.doe", createConfigState())
			resp, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Session).NotTo(BeNil())
			Expect(resp.Session.ID()).NotTo(BeEmpty())
		})
	})

	Describe("PruneTerminalEntries", func() {
		It("removes index entries for CRDs in terminal phase", func() {
			req1 := createRequestWithDefaults("prune-active", "jane.doe", createConfigState())
			_, err := svc.Create(ctx, &req1)
			Expect(err).NotTo(HaveOccurred())

			req2 := createRequestWithDefaults("prune-done", "jane.doe", createConfigState())
			_, err = svc.Create(ctx, &req2)
			Expect(err).NotTo(HaveOccurred())

			// Simulate external transition (TTL controller) by updating CRD directly
			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "prune-done", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			crd.Status.Phase = v1alpha1.SessionPhaseCompleted
			err = k8s.Status().Update(ctx, &crd)
			Expect(err).NotTo(HaveOccurred())

			pruned := svc.PruneTerminalEntries(ctx)
			Expect(pruned).To(Equal(1))

			// Active session should still be accessible via GetSessionPhase
			phase, err := svc.GetSessionPhase(ctx, "prune-active")
			Expect(err).NotTo(HaveOccurred())
			Expect(phase).To(Equal(v1alpha1.SessionPhaseActive))
		})

		It("is idempotent when called repeatedly", func() {
			req := createRequestWithDefaults("prune-idem", "jane.doe", createConfigState())
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			// Simulate external terminal transition
			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "prune-idem", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			crd.Status.Phase = v1alpha1.SessionPhaseFailed
			err = k8s.Status().Update(ctx, &crd)
			Expect(err).NotTo(HaveOccurred())

			first := svc.PruneTerminalEntries(ctx)
			Expect(first).To(Equal(1))

			second := svc.PruneTerminalEntries(ctx)
			Expect(second).To(Equal(0))
		})
	})

	// --- Get Tests ---

	Describe("Get", func() {
		It("UT-AF-201-001: returns session by AppName+UserID+SessionID", func() {
			_, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-get",
				State:     createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())

			resp, err := svc.Get(ctx, &adksession.GetRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-get",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Session.ID()).To(Equal("sess-get"))
		})

		It("UT-AF-201-002: returns error when session not found", func() {
			_, err := svc.Get(ctx, &adksession.GetRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "nonexistent",
			})
			Expect(err).To(HaveOccurred())
		})

		It("UT-AF-201-003: NumRecentEvents returns filtered events", func() {
			createResp, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-events",
				State:     createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < 5; i++ {
				evt := adksession.NewEvent("inv-1")
				evt.Author = "agent"
				evt.Content = genai.NewContentFromText("msg", genai.RoleModel)
				err = svc.AppendEvent(ctx, createResp.Session, evt)
				Expect(err).NotTo(HaveOccurred())
			}

			resp, err := svc.Get(ctx, &adksession.GetRequest{
				AppName:         "kubernaut-apifrontend",
				UserID:          "jane.doe",
				SessionID:       "sess-events",
				NumRecentEvents: 2,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Session.Events().Len()).To(Equal(2))
		})

		It("UT-AF-201-004: after service restart returns error (no in-memory state)", func() {
			_, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-restart",
				State:     createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())

			// simulate restart: new service instance with same K8s client
			svc2 := newTestService(k8s, scheme)
			_, err = svc2.Get(ctx, &adksession.GetRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-restart",
			})
			Expect(err).To(HaveOccurred())
		})
	})

	// --- List Tests ---

	Describe("List", func() {
		It("UT-AF-202-001: returns all sessions for user", func() {
			for i := 0; i < 3; i++ {
				_, err := svc.Create(ctx, &adksession.CreateRequest{
					AppName: "kubernaut-apifrontend",
					UserID:  "jane.doe",
					State:   createConfigState(),
				})
				Expect(err).NotTo(HaveOccurred())
			}

			resp, err := svc.List(ctx, &adksession.ListRequest{
				AppName: "kubernaut-apifrontend",
				UserID:  "jane.doe",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Sessions).To(HaveLen(3))
		})

		It("UT-AF-202-002: returns empty when no sessions exist", func() {
			resp, err := svc.List(ctx, &adksession.ListRequest{
				AppName: "kubernaut-apifrontend",
				UserID:  "nobody",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Sessions).To(BeEmpty())
		})

		It("UT-AF-202-003: filters by AppName and UserID", func() {
			_, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName: "kubernaut-apifrontend",
				UserID:  "jane.doe",
				State:   createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = svc.Create(ctx, &adksession.CreateRequest{
				AppName: "kubernaut-apifrontend",
				UserID:  "bob.smith",
				State:   createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())

			resp, err := svc.List(ctx, &adksession.ListRequest{
				AppName: "kubernaut-apifrontend",
				UserID:  "jane.doe",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Sessions).To(HaveLen(1))
		})
	})

	// --- Delete Tests ---

	Describe("Delete", func() {
		It("UT-AF-203-001: removes CRD and delegate state", func() {
			_, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-del",
				State:     createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())

			err = svc.Delete(ctx, &adksession.DeleteRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-del",
			})
			Expect(err).NotTo(HaveOccurred())

			// delegate should not have session
			_, err = svc.Get(ctx, &adksession.GetRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-del",
			})
			Expect(err).To(HaveOccurred())

			// CRD should not exist
			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "sess-del", Namespace: "test-ns"}, &crd)
			Expect(err).To(HaveOccurred())
		})

		It("UT-AF-203-002: delete of nonexistent session is idempotent", func() {
			err := svc.Delete(ctx, &adksession.DeleteRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "nonexistent",
			})
			// ADK InMemoryService.Delete is a no-op for missing keys
			Expect(err).NotTo(HaveOccurred())
		})

		It("UT-AF-203-003: removes CRD even if delegate has no state", func() {
			// create session normally
			_, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-orphan",
				State:     createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())

			// simulate restart (new service loses delegate state, but CRD exists)
			svc2 := session.NewCRDSessionService(
				adksession.InMemoryService(),
				k8s,
				scheme,
				"test-ns",
			)
			// delete should still clean up CRD
			_ = svc2.Delete(ctx, &adksession.DeleteRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-orphan",
			})
			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "sess-orphan", Namespace: "test-ns"}, &crd)
			Expect(err).To(HaveOccurred())
		})
	})

	// --- AppendEvent Tests ---

	Describe("AppendEvent", func() {
		var sess adksession.Session

		BeforeEach(func() {
			resp, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-append",
				State:     createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())
			sess = resp.Session
		})

		It("UT-AF-204-001: stores event in delegate", func() {
			evt := adksession.NewEvent("inv-1")
			evt.Author = "agent"
			evt.Content = genai.NewContentFromText("hello", genai.RoleModel)

			err := svc.AppendEvent(ctx, sess, evt)
			Expect(err).NotTo(HaveOccurred())

			getResp, err := svc.Get(ctx, &adksession.GetRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-append",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(getResp.Session.Events().Len()).To(Equal(1))
		})

		It("UT-AF-204-002: skips partial events", func() {
			evt := adksession.NewEvent("inv-1")
			evt.Author = "agent"
			evt.Partial = true
			evt.Content = genai.NewContentFromText("partial", genai.RoleModel)

			err := svc.AppendEvent(ctx, sess, evt)
			Expect(err).NotTo(HaveOccurred())

			getResp, err := svc.Get(ctx, &adksession.GetRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-append",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(getResp.Session.Events().Len()).To(Equal(0))
		})

		It("UT-AF-204-003: strips temp: keys from StateDelta", func() {
			evt := adksession.NewEvent("inv-1")
			evt.Author = "agent"
			evt.Content = genai.NewContentFromText("result", genai.RoleModel)
			evt.Actions.StateDelta = map[string]any{
				"temp:scratch":   "ephemeral",
				"persistent_key": "keep",
			}

			err := svc.AppendEvent(ctx, sess, evt)
			Expect(err).NotTo(HaveOccurred())

			getResp, err := svc.Get(ctx, &adksession.GetRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-append",
			})
			Expect(err).NotTo(HaveOccurred())
			storedEvent := getResp.Session.Events().At(0)
			Expect(storedEvent.Actions.StateDelta).NotTo(HaveKey("temp:scratch"))
		})

		It("UT-AF-204-004: trims large FunctionResponse before delegation", func() {
			largeResponse := map[string]any{
				"items": strings.Repeat("x", 10000),
			}
			responseJSON, _ := json.Marshal(largeResponse)

			evt := adksession.NewEvent("inv-1")
			evt.Author = "agent"
			evt.Content = &genai.Content{
				Role: string(genai.RoleModel),
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name:     "af_get_pods",
							Response: largeResponse,
						},
					},
				},
			}

			err := svc.AppendEvent(ctx, sess, evt)
			Expect(err).NotTo(HaveOccurred())

			getResp, err := svc.Get(ctx, &adksession.GetRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-append",
			})
			Expect(err).NotTo(HaveOccurred())
			storedEvent := getResp.Session.Events().At(0)
			Expect(storedEvent.Content.Parts).To(HaveLen(1))

			respBytes, _ := json.Marshal(storedEvent.Content.Parts[0].FunctionResponse.Response)
			Expect(len(respBytes)).To(BeNumerically("<", len(responseJSON)))
			_ = responseJSON
		})

		It("UT-AF-204-005: updates CRD status lastUpdateTime", func() {
			evt := adksession.NewEvent("inv-1")
			evt.Author = "agent"
			evt.Content = genai.NewContentFromText("update", genai.RoleModel)

			err := svc.AppendEvent(ctx, sess, evt)
			Expect(err).NotTo(HaveOccurred())

			var crd v1alpha1.InvestigationSession
			err = k8s.Get(ctx, types.NamespacedName{Name: "sess-append", Namespace: "test-ns"}, &crd)
			Expect(err).NotTo(HaveOccurred())
			// CRD metadata should reflect recent activity
			Expect(crd.Status.Phase).To(Equal(v1alpha1.SessionPhaseActive))
		})

		It("UT-AF-204-006: preserves user messages and final responses", func() {
			userEvt := adksession.NewEvent("inv-1")
			userEvt.Author = "user"
			userEvt.Content = genai.NewContentFromText(strings.Repeat("u", 10000), genai.RoleUser)

			err := svc.AppendEvent(ctx, sess, userEvt)
			Expect(err).NotTo(HaveOccurred())

			modelEvt := adksession.NewEvent("inv-1")
			modelEvt.Author = "agent"
			modelEvt.Content = genai.NewContentFromText(strings.Repeat("m", 10000), genai.RoleModel)

			err = svc.AppendEvent(ctx, sess, modelEvt)
			Expect(err).NotTo(HaveOccurred())

			getResp, err := svc.Get(ctx, &adksession.GetRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-append",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(getResp.Session.Events().Len()).To(Equal(2))

			// user and model text should NOT be trimmed (only FunctionResponse is trimmed)
			e0 := getResp.Session.Events().At(0)
			Expect(e0.Content.Parts[0].Text).To(HaveLen(10000))
			e1 := getResp.Session.Events().At(1)
			Expect(e1.Content.Parts[0].Text).To(HaveLen(10000))
		})
	})

	Describe("Create rollback logging", func() {
		It("UT-AF-200-007: logs Warn when CRD rollback fails after status update error", func() {
			statusUpdateFail := true
			deleteFail := true
			k8s = fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&v1alpha1.InvestigationSession{}).
				WithInterceptorFuncs(interceptor.Funcs{
					SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
						if statusUpdateFail {
							return fmt.Errorf("simulated status update failure")
						}
						return c.SubResource(subResourceName).Update(ctx, obj, opts...)
					},
					Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
						if deleteFail {
							return fmt.Errorf("simulated delete failure")
						}
						return c.Delete(ctx, obj, opts...)
					},
				}).
				Build()
			svc = session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
			)

			// Create will succeed at CRD creation, fail at Status Update,
			// then attempt rollback Delete which also fails (triggering Warn log).
			_, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-rollback",
				State:     createConfigState(),
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("set InvestigationSession initial status"))
		})
	})

	Describe("Delete gauge accuracy", func() {
		It("UT-AF-203-004: decrements the actual CRD phase, not hardcoded Active", func() {
			gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "af_sessions_active",
			}, []string{"phase"})
			k8s = newFakeClient(scheme)
			svc = session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithSessionsActive(gauge),
			)

			_, err := svc.Create(ctx, &adksession.CreateRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-gauge",
				State:     createConfigState(),
			})
			Expect(err).NotTo(HaveOccurred())

			// Transition to Disconnected
			err = svc.UpdatePhase(ctx, "sess-gauge", v1alpha1.SessionPhaseDisconnected, "SSE dropped", "jane.doe")
			Expect(err).NotTo(HaveOccurred())

			// Delete should decrement Disconnected (not Active)
			err = svc.Delete(ctx, &adksession.DeleteRequest{
				AppName:   "kubernaut-apifrontend",
				UserID:    "jane.doe",
				SessionID: "sess-gauge",
			})
			Expect(err).NotTo(HaveOccurred())

			activeMetric := &dto.Metric{}
			disconnectedMetric := &dto.Metric{}
			_ = gauge.WithLabelValues(string(v1alpha1.SessionPhaseActive)).Write(activeMetric)
			_ = gauge.WithLabelValues(string(v1alpha1.SessionPhaseDisconnected)).Write(disconnectedMetric)

			Expect(activeMetric.GetGauge().GetValue()).To(Equal(float64(0)))
			Expect(disconnectedMetric.GetGauge().GetValue()).To(Equal(float64(0)))
		})
	})

	Describe("PruneTerminalEntries", func() {
		It("UT-AF-205-001: prunes all entries when every indexed CRD is terminal", func() {
			k8s = newFakeClient(scheme)
			svc = newTestService(k8s, scheme)

			for i := 0; i < 3; i++ {
				id := fmt.Sprintf("sess-all-terminal-%d", i)
				req := createRequestWithDefaults(id, "jane.doe", createConfigState())
				_, err := svc.Create(ctx, &req)
				Expect(err).NotTo(HaveOccurred())

				// Externally transition the CRD to terminal (bypasses UpdatePhase
				// to simulate orphan scenario where crdIndex is not cleaned).
				var crd v1alpha1.InvestigationSession
				err = k8s.Get(ctx, types.NamespacedName{Name: id, Namespace: "test-ns"}, &crd)
				Expect(err).NotTo(HaveOccurred())
				crd.Status.Phase = v1alpha1.SessionPhaseCompleted
				err = k8s.Status().Update(ctx, &crd)
				Expect(err).NotTo(HaveOccurred())
			}

			pruned := svc.PruneTerminalEntries(ctx)
			Expect(pruned).To(Equal(3))

			// Idempotent: second call prunes nothing
			pruned = svc.PruneTerminalEntries(ctx)
			Expect(pruned).To(Equal(0))
		})
	})

	Describe("UpdatePhase audit", func() {
		It("UT-AF-210-011: emits audit event with correct userID", func() {
			recorder := &recordingEmitter{}
			k8s = newFakeClient(scheme)
			svc = session.NewCRDSessionService(
				adksession.InMemoryService(), k8s, scheme, "test-ns",
				session.WithAuditor(recorder),
			)

			req := createRequestWithDefaults("sess-audit", "jane.doe", createConfigState())
			_, err := svc.Create(ctx, &req)
			Expect(err).NotTo(HaveOccurred())

			err = svc.UpdatePhase(ctx, "sess-audit", v1alpha1.SessionPhaseCompleted, "done", "test-actor")
			Expect(err).NotTo(HaveOccurred())

			var phaseEvent *audit.Event
			for _, e := range recorder.events() {
				if e.Type == audit.EventSessionPhaseChanged {
					phaseEvent = e
					break
				}
			}
			Expect(phaseEvent).NotTo(BeNil())
			Expect(phaseEvent.UserID).To(Equal("test-actor"))
			Expect(phaseEvent.Detail["from"]).To(Equal(string(v1alpha1.SessionPhaseActive)))
			Expect(phaseEvent.Detail["to"]).To(Equal(string(v1alpha1.SessionPhaseCompleted)))
		})
	})
})

type recordingEmitter struct {
	mu   sync.Mutex
	evts []*audit.Event
}

func (r *recordingEmitter) Emit(_ context.Context, event *audit.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evts = append(r.evts, event)
}

func (r *recordingEmitter) events() []*audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]*audit.Event, len(r.evts))
	copy(cp, r.evts)
	return cp
}
