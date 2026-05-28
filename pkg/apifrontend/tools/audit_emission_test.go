package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

type spyEmitter struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (s *spyEmitter) Emit(_ context.Context, event *audit.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *spyEmitter) eventsByType(t audit.EventType) []*audit.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*audit.Event
	for _, e := range s.events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

var _ = Describe("Audit event emission – tool handlers (PR2 wiring)", func() {
	rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}
	eventsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

	Describe("HandleCreateRR", func() {
		It("UT-AF-1156-050: emits rr.created on successful creation", func() {
			scheme := runtime.NewScheme()
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
				map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList", eventsGVR: "EventList"})
			spy := &spyEmitter{}

			_, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "test",
			}, "alice", nil, spy)
			Expect(err).NotTo(HaveOccurred())

			events := spy.eventsByType(audit.EventRRCreated)
			Expect(events).To(HaveLen(1), "expected exactly one rr.created event")
			Expect(events[0].UserID).To(Equal("alice"))
			Expect(events[0].Detail).To(HaveKeyWithValue("namespace", "prod"))
			Expect(events[0].Detail).To(HaveKeyWithValue("kind", "Deployment"))
			Expect(events[0].Detail).To(HaveKeyWithValue("name", "web"))
		})

		It("UT-AF-1156-051: emits rr.deduplicated when existing RR found", func() {
			rr := newUnstructuredRR("prod", "rr-deploy-web-existing", "Executing", "Deployment", "web")
			scheme := runtime.NewScheme()
			client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
				map[schema.GroupVersionResource]string{rrGVR: "RemediationRequestList", eventsGVR: "EventList"}, rr)
			spy := &spyEmitter{}

			result, err := tools.HandleCreateRR(context.Background(), client, "prod", &tools.CreateRRArgs{
				Namespace: "prod", Kind: "Deployment", Name: "web", Description: "dup",
			}, "bob", nil, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.AlreadyExists).To(BeTrue())

			events := spy.eventsByType(audit.EventRRDeduplicated)
			Expect(events).To(HaveLen(1), "expected exactly one rr.deduplicated event")
			Expect(events[0].UserID).To(Equal("bob"))
			Expect(events[0].Detail).To(HaveKeyWithValue("existing_rr", "prod/rr-deploy-web-existing"))
		})
	})

	Describe("HandleInvestigation (new investigation)", func() {
		It("UT-AF-1156-052: emits ka.delegated on successful delegation", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v1/incident/analyze", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{"session_id": "sess-abc"})
			})
			mux.HandleFunc("/api/v1/incident/session/sess-abc/stream", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("event: complete\ndata: {\"summary\":\"done\"}\n\n"))
			})
			server := httptest.NewServer(mux)
			defer server.Close()

			spy := &spyEmitter{}
			kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
			_, err := tools.HandleInvestigation(context.Background(), kaClient,
				tools.InvestigateArgs{Namespace: "payments", Name: "rr-1"}, spy)
			Expect(err).NotTo(HaveOccurred())

			events := spy.eventsByType(audit.EventKADelegated)
			Expect(events).To(HaveLen(1), "expected exactly one ka.delegated event")
			Expect(events[0].Detail).To(HaveKeyWithValue("namespace", "payments"))
			Expect(events[0].Detail).To(HaveKeyWithValue("rr_name", "rr-1"))
			Expect(events[0].Detail).To(HaveKeyWithValue("delegation_type", "autonomous"))
			Expect(events[0].Detail).To(HaveKeyWithValue("ka_correlation_id", "sess-abc"))
		})
	})

	Describe("HandleInvestigation (resume completed)", func() {
		It("UT-AF-1156-053: emits ka.result_received with result_type and ka_correlation_id on completed", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v1/incident/session/sess-1", func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(ka.SessionStatus{SessionID: "sess-1", Status: "completed"})
			})
			mux.HandleFunc("/api/v1/incident/session/sess-1/result", func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(ka.IncidentResponse{SessionID: "sess-1", Summary: "OOM Kill root cause"})
			})
			server := httptest.NewServer(mux)
			defer server.Close()

			spy := &spyEmitter{}
			kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
			_, err := tools.HandleInvestigation(context.Background(), kaClient,
				tools.InvestigateArgs{SessionID: "sess-1"}, spy)
			Expect(err).NotTo(HaveOccurred())

			events := spy.eventsByType(audit.EventKAResultReceived)
			Expect(events).To(HaveLen(1), "expected exactly one ka.result_received event")
			Expect(events[0].Detail).To(HaveKeyWithValue("session_id", "sess-1"))
			Expect(events[0].Detail).To(HaveKeyWithValue("result_type", "rca_complete"),
				"result_type is required by OpenAPI schema (data-storage-v1.yaml)")
			Expect(events[0].Detail).To(HaveKeyWithValue("ka_correlation_id", "sess-1"),
				"ka_correlation_id is required by OpenAPI schema")
		})

		It("UT-AF-1156-058: emits ka.result_received with result_type=rca_failed on failure", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status": "failed",
				})
			}))
			defer server.Close()

			spy := &spyEmitter{}
			kaClient := ka.NewClient(ka.Config{BaseURL: server.URL})
			_, err := tools.HandleInvestigation(context.Background(), kaClient,
				tools.InvestigateArgs{SessionID: "sess-fail"}, spy)
			Expect(err).NotTo(HaveOccurred())

			events := spy.eventsByType(audit.EventKAResultReceived)
			Expect(events).To(HaveLen(1))
			Expect(events[0].Detail).To(HaveKeyWithValue("result_type", "rca_failed"))
			Expect(events[0].Detail).To(HaveKeyWithValue("ka_correlation_id", "sess-fail"))
		})
	})

	Describe("HandleSelectWorkflow", func() {
		It("UT-AF-1156-054: emits user.decision on workflow selection", func() {
			mockMCP := &ka.MockMCPClient{
				SelectWorkflowFn: func(_ context.Context, args ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
					return &ka.SelectWorkflowResult{Status: "accepted", Message: "ok"}, nil
				},
			}
			spy := &spyEmitter{}
			result, err := tools.HandleSelectWorkflow(context.Background(), mockMCP,
				tools.SelectWorkflowArgs{RRID: "pay/rr-1", WorkflowID: "wf-restart"}, spy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("accepted"))

			events := spy.eventsByType(audit.EventUserDecision)
			Expect(events).To(HaveLen(1), "expected exactly one user.decision event")
			Expect(events[0].Detail).To(HaveKeyWithValue("rr_id", "pay/rr-1"))
			Expect(events[0].Detail).To(HaveKeyWithValue("workflow_id", "wf-restart"))
		})
	})
})
