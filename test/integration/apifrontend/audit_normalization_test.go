package apifrontend_test

import (
	"context"
	"strconv"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// capturingDSClient captures batches written by BufferedAuditStore for verification.
type capturingDSClient struct {
	batches [][]*ogenclient.AuditEventRequest
}

func (c *capturingDSClient) StoreBatch(_ context.Context, events []*ogenclient.AuditEventRequest) error {
	c.batches = append(c.batches, events)
	return nil
}

func (c *capturingDSClient) allEvents() []*ogenclient.AuditEventRequest {
	var all []*ogenclient.AuditEventRequest
	for _, batch := range c.batches {
		all = append(all, batch...)
	}
	return all
}

var _ = Describe("IT-AF-1156: Audit Normalization Integration", func() {
	var (
		dsClient   *capturingDSClient
		auditStore sharedaudit.AuditStore
		adapter    audit.ClosableEmitter
	)

	BeforeEach(func() {
		dsClient = &capturingDSClient{}
		var err error
		auditStore, err = sharedaudit.NewBufferedStore(dsClient, sharedaudit.Config{
			BufferSize:    100,
			BatchSize:     10,
			FlushInterval: 50 * time.Millisecond,
			MaxRetries:    1,
		}, "apifrontend", logr.Discard())
		Expect(err).NotTo(HaveOccurred())

		adapter = audit.NewStoreAdapter(auditStore, logr.Discard())
	})

	AfterEach(func() {
		if adapter != nil {
			Expect(adapter.Close(context.Background())).To(Succeed())
		}
	})

	It("IT-AF-1156-001: session.created event round-trips through BufferedAuditStore with typed payload", func() {
		adapter.Emit(context.Background(), &audit.Event{
			Type:          audit.EventSessionCreated,
			CorrelationID: "rr-sess-001",
			UserID:        "alice",
			Detail: map[string]string{
				"session_id":    "sess-it-001",
				"a2a_task_id":   "task-it-001",
				"join_mode":     "start",
				"user_identity": "alice",
			},
		})

		Expect(auditStore.Flush(context.Background())).To(Succeed())

		Eventually(func() int { return len(dsClient.allEvents()) }).
			WithTimeout(2 * time.Second).
			WithPolling(50 * time.Millisecond).
			Should(BeNumerically(">=", 1))

		events := dsClient.allEvents()
		Expect(events).To(HaveLen(1))
		evt := events[0]
		Expect(evt.EventType).To(Equal("apifrontend.session.created"))
		Expect(evt.EventCategory).To(Equal(ogenclient.AuditEventRequestEventCategoryApifrontend))
		Expect(evt.CorrelationID).To(Equal("rr-sess-001"))
		Expect(evt.ActorType.Value).To(Equal("user"))
		Expect(evt.ActorID.Value).To(Equal("alice"))
		Expect(string(evt.EventData.Type)).To(Equal("apifrontend.session.created"))
	})

	It("IT-AF-1156-002: auth.success event round-trips with typed payload", func() {
		adapter.Emit(context.Background(), &audit.Event{
			Type:   audit.EventAuthSuccess,
			UserID: "bob",
			Detail: map[string]string{
				"auth_method": "jwt",
				"issuer":      "dex.example.com",
			},
		})

		Expect(auditStore.Flush(context.Background())).To(Succeed())

		Eventually(func() int { return len(dsClient.allEvents()) }).
			WithTimeout(2 * time.Second).
			WithPolling(50 * time.Millisecond).
			Should(BeNumerically(">=", 1))

		events := dsClient.allEvents()
		Expect(events).To(HaveLen(1))
		evt := events[0]
		Expect(evt.EventType).To(Equal("apifrontend.auth.success"))
		Expect(evt.EventAction).To(Equal("authenticated"))
		Expect(evt.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess))
	})

	It("IT-AF-1156-010: session.completed emits duration_ms with real K8s CreationTimestamp", func() {
		adkSvc := adksession.InMemoryService()
		recorder := &itRecordingEmitter{}
		svc := session.NewCRDSessionService(
			adkSvc, k8sClient, scheme, "default",
			session.WithAuditor(recorder),
		)
		ctx := context.Background()

		req := adksession.CreateRequest{
			AppName:   "kubernaut-apifrontend",
			UserID:    "it-user",
			SessionID: "sess-duration-it",
			State: map[string]any{
				session.StateKeyCreateConfig: &session.CreateConfig{
					OwnerRef: metav1.OwnerReference{
						APIVersion: "kubernaut.ai/v1",
						Kind:       "RemediationRequest",
						Name:       "owner-rr-duration-it",
						UID:        "00000000-0000-0000-0000-000000000001",
					},
					A2ATaskID:    "task-it-duration",
					UserIdentity: v1alpha1.SessionUser{Username: "it-user"},
					JoinMode:     v1alpha1.SessionJoinModeStart,
				},
			},
		}
		_, err := svc.Create(ctx, &req)
		Expect(err).NotTo(HaveOccurred())

		err = svc.MaterializeCRD(ctx, "sess-duration-it", v1alpha1.ObjectRef{Name: "rr-duration-it", Namespace: "default"})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			err = svc.UpdatePhase(ctx, "sess-duration-it", v1alpha1.SessionPhaseCompleted, "done", "it-user")
			g.Expect(err).NotTo(HaveOccurred())

			var completedEvents []*audit.Event
			for _, e := range recorder.events() {
				if e.Type == audit.EventSessionCompleted {
					completedEvents = append(completedEvents, e)
				}
			}
			g.Expect(completedEvents).To(HaveLen(1))
			g.Expect(completedEvents[0].Detail).To(HaveKey("total_duration_ms"),
				"with a real K8s API server, CreationTimestamp is set and total_duration_ms must be present")

			durationStr := completedEvents[0].Detail["total_duration_ms"]
			durationVal, parseErr := strconv.ParseInt(durationStr, 10, 64)
			g.Expect(parseErr).NotTo(HaveOccurred())
			g.Expect(durationVal).To(BeNumerically(">=", 0),
				"duration_ms should be non-negative (server-set CreationTimestamp)")
		}).WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())
	})

	It("IT-AF-1156-011: multi-event pipeline round-trips all event categories through BufferedAuditStore", func() {
		events := []audit.Event{
			{Type: audit.EventSessionCreated, CorrelationID: "rr-multi-001", UserID: "alice", Detail: map[string]string{
				"session_id": "sess-multi-001", "a2a_task_id": "task-001", "join_mode": "start", "user_identity": "alice",
			}},
			{Type: audit.EventToolExecuted, CorrelationID: "rr-multi-001", UserID: "alice", Detail: map[string]string{
				"session_id": "sess-multi-001", "tool_name": "kubernaut_investigate", "tool_outcome": "success", "execution_duration_ms": "150",
			}},
			{Type: audit.EventRRCreated, CorrelationID: "rr-multi-001", UserID: "alice", Detail: map[string]string{
				"rr_name": "rr-test-001", "rr_namespace": "default",
			}},
			{Type: audit.EventSeverityTriageCompleted, CorrelationID: "rr-multi-001", UserID: "system", Detail: map[string]string{
				"severity": "critical", "source_tier": "prometheus_rules",
			}},
			{Type: audit.EventCircuitBreakerTrip, Detail: map[string]string{
				"circuit_name": "ds", "failure_count": "5",
			}},
		}
		for i := range events {
			adapter.Emit(context.Background(), &events[i])
		}

		Expect(auditStore.Flush(context.Background())).To(Succeed())

		Eventually(func() int { return len(dsClient.allEvents()) }).
			WithTimeout(5 * time.Second).
			WithPolling(100 * time.Millisecond).
			Should(BeNumerically(">=", 4))

		all := dsClient.allEvents()

		typeSet := make(map[string]bool, len(all))
		for _, evt := range all {
			typeSet[evt.EventType] = true
			Expect(evt.EventCategory).To(Equal(ogenclient.AuditEventRequestEventCategoryApifrontend))
		}

		expected := []string{
			"apifrontend.session.created",
			"apifrontend.tool.executed",
			"apifrontend.rr.created",
			"apifrontend.severity_triage.completed",
			"apifrontend.circuitbreaker.trip",
		}
		for _, e := range expected {
			Expect(typeSet).To(HaveKey(e), "missing event type: "+e)
		}
	})

	It("IT-AF-1156-012: circuitbreaker.trip event includes state transition and dependency details", func() {
		adapter.Emit(context.Background(), &audit.Event{
			Type: audit.EventCircuitBreakerTrip,
			Detail: map[string]string{
				"dependency": "ka",
				"from_state": "closed",
				"to_state":   "open",
			},
		})

		Expect(auditStore.Flush(context.Background())).To(Succeed())

		Eventually(func() int { return len(dsClient.allEvents()) }).
			WithTimeout(2 * time.Second).
			WithPolling(50 * time.Millisecond).
			Should(BeNumerically(">=", 1))

		events := dsClient.allEvents()
		Expect(events).To(HaveLen(1))
		evt := events[0]
		Expect(evt.EventType).To(Equal("apifrontend.circuitbreaker.trip"))
		Expect(evt.EventAction).To(Equal("tripped"))
		Expect(evt.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure))
	})

	It("IT-AF-1156-003: Close flushes all buffered events", func() {
		for i := 0; i < 5; i++ {
			adapter.Emit(context.Background(), &audit.Event{
				Type:   audit.EventConfigReloaded,
				Detail: map[string]string{"config_version": "v1"},
			})
		}

		Expect(adapter.Close(context.Background())).To(Succeed())

		all := dsClient.allEvents()
		Expect(all).To(HaveLen(5))
		for _, evt := range all {
			Expect(evt.EventType).To(Equal("apifrontend.config.reloaded"))
		}

		adapter = nil
	})
})

type itRecordingEmitter struct {
	mu   sync.Mutex
	evts []*audit.Event
}

func (r *itRecordingEmitter) Emit(_ context.Context, event *audit.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evts = append(r.evts, event)
}

func (r *itRecordingEmitter) events() []*audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]*audit.Event, len(r.evts))
	copy(cp, r.evts)
	return cp
}
