package agent

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/agent"
	adkmemory "google.golang.org/adk/memory"
	adksession "google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

func testCRDScheme() *k8sruntime.Scheme {
	s := k8sruntime.NewScheme()
	_ = isv1alpha1.AddToScheme(s)
	return s
}

func newFakeCRDClient(scheme *k8sruntime.Scheme) client.Client {
	return k8sfake.NewClientBuilder().WithScheme(scheme).Build()
}

// fakeToolContext satisfies tool.Context for unit testing callbacks.
// Only context.Value is exercised; all ADK-specific methods return zero values.
type fakeToolContext struct {
	context.Context
}

func (fakeToolContext) UserContent() *genai.Content                                          { return nil }
func (fakeToolContext) InvocationID() string                                                 { return "" }
func (fakeToolContext) AgentName() string                                                    { return "" }
func (fakeToolContext) ReadonlyState() adksession.ReadonlyState                              { return nil }
func (fakeToolContext) UserID() string                                                       { return "" }
func (fakeToolContext) AppName() string                                                      { return "" }
func (fakeToolContext) SessionID() string                                                    { return "" }
func (fakeToolContext) Branch() string                                                       { return "" }
func (fakeToolContext) Artifacts() agent.Artifacts                                           { return nil }
func (fakeToolContext) State() adksession.State                                              { return nil }
func (fakeToolContext) FunctionCallID() string                                               { return "" }
func (fakeToolContext) Actions() *adksession.EventActions                                    { return nil }
func (fakeToolContext) SearchMemory(context.Context, string) (*adkmemory.SearchResponse, error) {
	return nil, nil
}
func (fakeToolContext) ToolConfirmation() *toolconfirmation.ToolConfirmation { return nil }
func (fakeToolContext) RequestConfirmation(string, any) error                { return nil }

// Compile-time interface satisfaction check.
var _ tool.Context = fakeToolContext{}

// fakeTool satisfies tool.Tool for unit testing callbacks.
type fakeTool struct {
	name string
}

func (t fakeTool) Name() string        { return t.name }
func (fakeTool) Description() string   { return "" }
func (fakeTool) IsLongRunning() bool   { return false }

// capturingAuditor captures audit events for test assertions.
type capturingAuditor struct {
	events []*audit.Event
}

func (a *capturingAuditor) Emit(_ context.Context, event *audit.Event) {
	a.events = append(a.events, event)
}

var _ = Describe("newAuditToolCallback (#1189)", func() {
	var (
		auditor  *capturingAuditor
		callback func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error)
	)

	BeforeEach(func() {
		auditor = &capturingAuditor{}
		callback = newAuditToolCallback(auditor, nil, "kubernaut-system")
	})

	It("UT-AF-1189-001: includes a2a_task_id when session CreateContext is present", func() {
		ctx := context.Background()
		ctx = session.WithCreateContext(ctx, &session.CreateContext{TaskID: "task-abc-123"})
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-alice"})

		tc := fakeToolContext{Context: ctx}
		_, err := callback(tc, fakeTool{name: "kubectl_list_events"}, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		Expect(auditor.events[0].Detail["a2a_task_id"]).To(Equal("task-abc-123"))
	})

	It("UT-AF-1189-002: omits a2a_task_id when no session CreateContext", func() {
		ctx := context.Background()
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-bob"})

		tc := fakeToolContext{Context: ctx}
		_, err := callback(tc, fakeTool{name: "kubectl_list_events"}, nil, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		_, hasTaskID := auditor.events[0].Detail["a2a_task_id"]
		Expect(hasTaskID).To(BeFalse())
	})

	It("UT-AF-1189-003: af_create_rr no longer triggers RR correlation (#1332 redesign)", func() {
		ctx := context.Background()
		ctx = session.WithCreateContext(ctx, &session.CreateContext{TaskID: "task-xyz"})
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-carol"})

		output := map[string]any{"rr_id": "rr-prod-001"}
		tc := fakeToolContext{Context: ctx}
		_, err := callback(tc, fakeTool{name: "af_create_rr"}, nil, output, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		_, hasRRID := auditor.events[0].Detail["rr_id"]
		Expect(hasRRID).To(BeFalse(), "af_create_rr removed from correlation — replaced by kubernaut_remediate")
		Expect(auditor.events[0].Detail["a2a_task_id"]).To(Equal("task-xyz"))
	})

	It("UT-AF-1189-004: omits rr_id when af_create_rr fails", func() {
		ctx := context.Background()
		ctx = session.WithCreateContext(ctx, &session.CreateContext{TaskID: "task-fail"})
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-dave"})

		tc := fakeToolContext{Context: ctx}
		toolErr := fmt.Errorf("creation failed")
		_, err := callback(tc, fakeTool{name: "af_create_rr"}, nil, nil, toolErr)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		_, hasRRID := auditor.events[0].Detail["rr_id"]
		Expect(hasRRID).To(BeFalse())
		Expect(auditor.events[0].Detail["tool_outcome"]).To(Equal("failure"))
	})

	It("UT-AF-1189-005: omits rr_id for non-af_create_rr tools", func() {
		ctx := context.Background()
		ctx = session.WithCreateContext(ctx, &session.CreateContext{TaskID: "task-other"})

		output := map[string]any{"rr_id": "should-not-appear"}
		tc := fakeToolContext{Context: ctx}
		_, err := callback(tc, fakeTool{name: "kubectl_list_events"}, nil, output, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		_, hasRRID := auditor.events[0].Detail["rr_id"]
		Expect(hasRRID).To(BeFalse())
	})

	It("UT-AF-1189-006: records user identity in audit event", func() {
		ctx := context.Background()
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-eve"})

		tc := fakeToolContext{Context: ctx}
		_, err := callback(tc, fakeTool{name: "kubectl_list"}, map[string]any{"namespace": "prod"}, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		Expect(auditor.events[0].UserID).To(Equal("sre-eve"))
		Expect(auditor.events[0].Detail["namespace"]).To(Equal("prod"))
	})
})

var _ = Describe("IS CRD creation on af_create_rr (race fix)", func() {

	newIndexedFakeClient := func() client.Client {
		return k8sfake.NewClientBuilder().
			WithScheme(testCRDScheme()).
			WithStatusSubresource(&isv1alpha1.InvestigationSession{}).
			WithIndex(&isv1alpha1.InvestigationSession{}, session.FieldIndexRRName,
				func(obj client.Object) []string {
					is := obj.(*isv1alpha1.InvestigationSession)
					if is.Spec.RemediationRequestRef.Name == "" {
						return nil
					}
					return []string{is.Spec.RemediationRequestRef.Name}
				}).
			Build()
	}

	It("UT-AF-1326-090: af_create_rr NO longer creates IS CRD (#1332 redesign)", func() {
		k8sClient := newIndexedFakeClient()
		svc := session.NewCRDSessionService(
			adksession.InMemoryService(), k8sClient, testCRDScheme(), "kubernaut-system",
		)

		auditor := &capturingAuditor{}
		cb := newAuditToolCallback(auditor, svc, "kubernaut-system")

		ctx := context.Background()
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{
			Username: "admin",
			Groups:   []string{"system:masters"},
		})

		output := map[string]any{"rr_id": "rr-abc123-def456"}
		tc := fakeToolContext{Context: ctx}
		_, err := cb(tc, fakeTool{name: "af_create_rr"}, nil, output, nil)
		Expect(err).NotTo(HaveOccurred())

		var isList isv1alpha1.InvestigationSessionList
		Expect(k8sClient.List(ctx, &isList)).To(Succeed())
		Expect(isList.Items).To(BeEmpty(), "IS CRD creation removed from callback — now done inside kubernaut_investigate")
	})

	It("UT-AF-1326-091: af_create_rr no longer sets rr_id in audit (#1332 redesign)", func() {
		auditor := &capturingAuditor{}
		cb := newAuditToolCallback(auditor, nil, "kubernaut-system")

		ctx := context.Background()
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "admin"})

		output := map[string]any{"rr_id": "rr-no-svc-091"}
		tc := fakeToolContext{Context: ctx}
		_, err := cb(tc, fakeTool{name: "af_create_rr"}, nil, output, nil)
		Expect(err).NotTo(HaveOccurred())

		Expect(auditor.events).To(HaveLen(1))
		_, hasRRID := auditor.events[0].Detail["rr_id"]
		Expect(hasRRID).To(BeFalse(), "af_create_rr removed from correlation tools")
	})

	It("UT-AF-1326-092: af_create_rr failure does not create IS CRD", func() {
		k8sClient := newIndexedFakeClient()
		svc := session.NewCRDSessionService(
			adksession.InMemoryService(), k8sClient, testCRDScheme(), "kubernaut-system",
		)

		auditor := &capturingAuditor{}
		cb := newAuditToolCallback(auditor, svc, "kubernaut-system")

		ctx := context.Background()
		ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "admin"})

		tc := fakeToolContext{Context: ctx}
		toolErr := fmt.Errorf("creation failed")
		_, err := cb(tc, fakeTool{name: "af_create_rr"}, nil, nil, toolErr)
		Expect(err).NotTo(HaveOccurred())

		var isList isv1alpha1.InvestigationSessionList
		Expect(k8sClient.List(ctx, &isList)).To(Succeed())
		Expect(isList.Items).To(BeEmpty(), "no IS CRD on af_create_rr failure")
	})

	It("UT-AF-1326-093: af_create_rr skips IS CRD when no user identity in context", func() {
		k8sClient := newIndexedFakeClient()
		svc := session.NewCRDSessionService(
			adksession.InMemoryService(), k8sClient, testCRDScheme(), "kubernaut-system",
		)

		auditor := &capturingAuditor{}
		cb := newAuditToolCallback(auditor, svc, "kubernaut-system")

		ctx := context.Background()

		output := map[string]any{"rr_id": "rr-no-identity-093"}
		tc := fakeToolContext{Context: ctx}
		_, err := cb(tc, fakeTool{name: "af_create_rr"}, nil, output, nil)
		Expect(err).NotTo(HaveOccurred())

		var isList isv1alpha1.InvestigationSessionList
		Expect(k8sClient.List(ctx, &isList)).To(Succeed())
		Expect(isList.Items).To(BeEmpty(), "no IS CRD without user identity")
	})

})

var _ = Describe("Audit callback — Intent-Based Tool Redesign (#1332)", func() {
	var (
		auditor  *capturingAuditor
		callback func(tool.Context, tool.Tool, map[string]any, map[string]any, error) (map[string]any, error)
	)

	BeforeEach(func() {
		auditor = &capturingAuditor{}
		callback = newAuditToolCallback(auditor, nil, "kubernaut-system")
	})

	Describe("A2A task-to-RR correlation for kubernaut_remediate (TI-05)", func() {
		It("UT-AF-1332-020: kubernaut_remediate success sets sc.RRName and sc.RRNamespace", func() {
			ctx := context.Background()
			sc := &session.CreateContext{TaskID: "task-rem-001"}
			ctx = session.WithCreateContext(ctx, sc)
			ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-alice"})

			output := map[string]any{"rr_id": "rr-remediate-001"}
			tc := fakeToolContext{Context: ctx}
			_, err := callback(tc, fakeTool{name: "kubernaut_remediate"}, nil, output, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(sc.RRName).To(Equal("rr-remediate-001"))
			Expect(sc.RRNamespace).To(Equal("kubernaut-system"))
			Expect(auditor.events).To(HaveLen(1))
			Expect(auditor.events[0].Detail["rr_id"]).To(Equal("rr-remediate-001"))
		})

		It("UT-AF-1332-021: kubernaut_investigate success sets sc.RRName and sc.RRNamespace", func() {
			ctx := context.Background()
			sc := &session.CreateContext{TaskID: "task-inv-001"}
			ctx = session.WithCreateContext(ctx, sc)
			ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-bob"})

			output := map[string]any{"rr_id": "rr-investigate-001"}
			tc := fakeToolContext{Context: ctx}
			_, err := callback(tc, fakeTool{name: "kubernaut_investigate"}, nil, output, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(sc.RRName).To(Equal("rr-investigate-001"))
			Expect(sc.RRNamespace).To(Equal("kubernaut-system"))
			Expect(auditor.events).To(HaveLen(1))
			Expect(auditor.events[0].Detail["rr_id"]).To(Equal("rr-investigate-001"))
		})
	})

	Describe("NO IS creation from audit callback (TI-06)", func() {
		It("UT-AF-1332-022: kubernaut_remediate does NOT create IS CRD", func() {
			k8sClient := k8sfake.NewClientBuilder().
				WithScheme(testCRDScheme()).
				WithStatusSubresource(&isv1alpha1.InvestigationSession{}).
				WithIndex(&isv1alpha1.InvestigationSession{}, session.FieldIndexRRName,
					func(obj client.Object) []string {
						is := obj.(*isv1alpha1.InvestigationSession)
						if is.Spec.RemediationRequestRef.Name == "" {
							return nil
						}
						return []string{is.Spec.RemediationRequestRef.Name}
					}).
				Build()

			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8sClient, testCRDScheme(), "kubernaut-system",
			)

			aud := &capturingAuditor{}
			cb := newAuditToolCallback(aud, svc, "kubernaut-system")

			ctx := context.Background()
			ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{
				Username: "admin",
				Groups:   []string{"system:masters"},
			})

			output := map[string]any{"rr_id": "rr-no-is-022"}
			tc := fakeToolContext{Context: ctx}
			_, err := cb(tc, fakeTool{name: "kubernaut_remediate"}, nil, output, nil)
			Expect(err).NotTo(HaveOccurred())

			var isList isv1alpha1.InvestigationSessionList
			Expect(k8sClient.List(ctx, &isList)).To(Succeed())
			Expect(isList.Items).To(BeEmpty(), "kubernaut_remediate must NOT create IS CRD")
		})

		It("UT-AF-1332-023: kubernaut_investigate does NOT create IS CRD from callback", func() {
			k8sClient := k8sfake.NewClientBuilder().
				WithScheme(testCRDScheme()).
				WithStatusSubresource(&isv1alpha1.InvestigationSession{}).
				WithIndex(&isv1alpha1.InvestigationSession{}, session.FieldIndexRRName,
					func(obj client.Object) []string {
						is := obj.(*isv1alpha1.InvestigationSession)
						if is.Spec.RemediationRequestRef.Name == "" {
							return nil
						}
						return []string{is.Spec.RemediationRequestRef.Name}
					}).
				Build()

			svc := session.NewCRDSessionService(
				adksession.InMemoryService(), k8sClient, testCRDScheme(), "kubernaut-system",
			)

			aud := &capturingAuditor{}
			cb := newAuditToolCallback(aud, svc, "kubernaut-system")

			ctx := context.Background()
			ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{
				Username: "admin",
				Groups:   []string{"system:masters"},
			})

			output := map[string]any{"rr_id": "rr-no-is-023", "session_id": "sess-023"}
			tc := fakeToolContext{Context: ctx}
			_, err := cb(tc, fakeTool{name: "kubernaut_investigate"}, nil, output, nil)
			Expect(err).NotTo(HaveOccurred())

			var isList isv1alpha1.InvestigationSessionList
			Expect(k8sClient.List(ctx, &isList)).To(Succeed())
			Expect(isList.Items).To(BeEmpty(), "kubernaut_investigate creates IS inside tool, NOT callback")
		})

		It("UT-AF-1332-024: af_create_rr name no longer triggers correlation or IS", func() {
			ctx := context.Background()
			sc := &session.CreateContext{TaskID: "task-old-tool"}
			ctx = session.WithCreateContext(ctx, sc)
			ctx = auth.WithUserIdentity(ctx, &auth.UserIdentity{Username: "sre-old"})

			output := map[string]any{"rr_id": "rr-old-tool-024"}
			tc := fakeToolContext{Context: ctx}
			_, err := callback(tc, fakeTool{name: "af_create_rr"}, nil, output, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(sc.RRName).To(BeEmpty(), "af_create_rr should no longer set correlation")
		})
	})
})
