package audit_test

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

type capturingStore struct {
	mu     sync.Mutex
	events []*ogenclient.AuditEventRequest
	closed bool
}

func (s *capturingStore) StoreAudit(_ context.Context, event *ogenclient.AuditEventRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *capturingStore) Flush(_ context.Context) error { return nil }

func (s *capturingStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *capturingStore) lastEvent() *ogenclient.AuditEventRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.events) == 0 {
		return nil
	}
	return s.events[len(s.events)-1]
}

var _ = Describe("StoreAdapter", func() {

	var (
		store   *capturingStore
		adapter audit.ClosableEmitter
	)

	BeforeEach(func() {
		store = &capturingStore{}
		adapter = audit.NewStoreAdapter(store, logr.Discard())
	})

	type eventMappingEntry struct {
		testID    string
		eventType audit.EventType
		detail    map[string]string
		wantType  string
	}

	eventMappings := []eventMappingEntry{
		{"UT-AF-1156-001", audit.EventAuthSuccess, map[string]string{"auth_method": "jwt", "issuer": "dex"}, "apifrontend.auth.success"},
		{"UT-AF-1156-002", audit.EventAuthFailure, map[string]string{"auth_method": "jwt", "failure_reason": "expired"}, "apifrontend.auth.failure"},
		{"UT-AF-1156-003", audit.EventRateLimitDenied, map[string]string{"limit_type": "per-user"}, "apifrontend.ratelimit.denied"},
		{"UT-AF-1156-004", audit.EventSessionCreated, map[string]string{"session_id": "sess-1", "a2a_task_id": "task-1", "join_mode": "start", "user_identity": "alice"}, "apifrontend.session.created"},
		{"UT-AF-1156-005", audit.EventSessionPhaseChanged, map[string]string{"session_id": "sess-1", "from_phase": "Pending", "to_phase": "Running"}, "apifrontend.session.phase_changed"},
		{"UT-AF-1156-006", audit.EventSessionDeleted, map[string]string{"session_id": "sess-1"}, "apifrontend.session.deleted"},
		{"UT-AF-1156-007", audit.EventSessionAutoCancelled, map[string]string{"session_id": "sess-1"}, "apifrontend.session.auto_cancelled"},
		{"UT-AF-1156-008", audit.EventSessionRetentionDeleted, map[string]string{"session_id": "sess-1"}, "apifrontend.session.retention_deleted"},
		{"UT-AF-1156-009", audit.EventSessionCompleted, map[string]string{"session_id": "sess-1", "terminal_phase": "Completed", "total_duration_ms": "12345"}, "apifrontend.session.completed"},
		{"UT-AF-1156-010", audit.EventA2ATaskStarted, map[string]string{"session_id": "sess-1", "task_id": "task-1"}, "apifrontend.a2a.task_started"},
		{"UT-AF-1156-011", audit.EventA2ATaskCompleted, map[string]string{"session_id": "sess-1", "task_id": "task-1"}, "apifrontend.a2a.task_completed"},
		{"UT-AF-1156-012", audit.EventA2ATaskFailed, map[string]string{"session_id": "sess-1", "task_id": "task-1", "error": "timeout"}, "apifrontend.a2a.task_failed"},
		{"UT-AF-1156-013", audit.EventMCPToolFailed, map[string]string{"tool_name": "kubectl_get", "error": "forbidden"}, "apifrontend.mcp.tool_failed"},
		{"UT-AF-1156-014", audit.EventMCPSessionInit, map[string]string{"mcp_session_id": "mcp-1", "protocol_version": "2025-03-26"}, "apifrontend.mcp.session_init"},
		{"UT-AF-1156-015", audit.EventConfigReloaded, map[string]string{"config_version": "v2"}, "apifrontend.config.reloaded"},
		{"UT-AF-1156-016", audit.EventConfigRejected, map[string]string{"rejection_reason": "invalid yaml"}, "apifrontend.config.rejected"},
		{"UT-AF-1156-017", audit.EventCircuitBreakerTrip, map[string]string{"circuit_name": "ds", "failure_count": "5"}, "apifrontend.circuitbreaker.trip"},
		{"UT-AF-1156-018", audit.EventImpersonation, map[string]string{"target_user": "bob"}, "apifrontend.impersonation.created"},
		{"UT-AF-1156-019", audit.EventJWTDelegation, map[string]string{"target_service": "kubernaut-agent"}, "apifrontend.jwt.delegation"},
		{"UT-AF-1156-020", audit.EventSeverityTriageCompleted, map[string]string{"severity": "critical", "source_tier": "prometheus_rules"}, "apifrontend.severity_triage.completed"},
		{"UT-AF-1156-021", audit.EventSeverityTriageFailed, map[string]string{"error": "llm timeout"}, "apifrontend.severity_triage.failed"},
		{"UT-AF-1156-022", audit.EventTriageStarted, map[string]string{"session_id": "sess-1", "persona": "sre"}, "apifrontend.triage.started"},
		{"UT-AF-1156-023", audit.EventTriageCompleted, map[string]string{"session_id": "sess-1", "triage_outcome": "delegated", "triage_duration_ms": "500"}, "apifrontend.triage.completed"},
		{"UT-AF-1156-024", audit.EventRRCreated, map[string]string{"session_id": "sess-1", "rr_name": "rr-1", "rr_namespace": "default", "fingerprint": "fp1"}, "apifrontend.rr.created"},
		{"UT-AF-1156-025", audit.EventRRDeduplicated, map[string]string{"session_id": "sess-1", "fingerprint": "abc123", "existing_rr_name": "rr-1"}, "apifrontend.rr.deduplicated"},
		{"UT-AF-1156-026", audit.EventKADelegated, map[string]string{"session_id": "sess-1", "ka_correlation_id": "ka-1", "delegation_type": "rest"}, "apifrontend.ka.delegated"},
		{"UT-AF-1156-027", audit.EventKAResultReceived, map[string]string{"session_id": "sess-1", "ka_correlation_id": "ka-1", "result_type": "rca_complete"}, "apifrontend.ka.result_received"},
		{"UT-AF-1156-028", audit.EventUserDecision, map[string]string{"session_id": "sess-1", "decision": "accept", "workflow_id": "restart-pod"}, "apifrontend.user.decision"},
		{"UT-AF-1156-029", audit.EventAuthAccessDenied, map[string]string{"tool_name": "kubectl_exec", "user_role": "viewer", "endpoint": "a2a"}, "apifrontend.auth.access_denied"},
		{"UT-AF-1156-030", audit.EventToolExecuted, map[string]string{"session_id": "sess-1", "tool_name": "kubectl_get", "execution_duration_ms": "100", "tool_outcome": "success"}, "apifrontend.tool.executed"},
	}

	Describe("Event type mapping", func() {
		for _, tc := range eventMappings {
			tc := tc
			It(tc.testID+": maps "+string(tc.eventType)+" to "+tc.wantType, func() {
				adapter.Emit(context.Background(), &audit.Event{
					Type:   tc.eventType,
					Detail: tc.detail,
				})

				evt := store.lastEvent()
				Expect(evt).NotTo(BeNil(), "expected event to be stored")
				Expect(evt.EventType).To(Equal(tc.wantType))
				Expect(string(evt.EventData.Type)).To(Equal(tc.wantType))
			})
		}
	})

	Describe("UT-AF-1156-031: event_action mapping", func() {
		type actionEntry struct {
			eventType  audit.EventType
			wantAction string
		}
		actions := []actionEntry{
			{audit.EventAuthSuccess, "authenticated"},
			{audit.EventAuthFailure, "authentication_failed"},
			{audit.EventRateLimitDenied, "denied"},
			{audit.EventSessionCreated, "created"},
			{audit.EventSessionDeleted, "deleted"},
			{audit.EventSessionPhaseChanged, "phase_changed"},
			{audit.EventSessionCompleted, "completed"},
			{audit.EventSessionAutoCancelled, "auto_cancelled"},
			{audit.EventSessionRetentionDeleted, "retention_deleted"},
			{audit.EventA2ATaskStarted, "started"},
			{audit.EventA2ATaskCompleted, "completed"},
			{audit.EventA2ATaskFailed, "failed"},
			{audit.EventToolExecuted, "executed"},
			{audit.EventAuthAccessDenied, "denied"},
			{audit.EventMCPToolFailed, "failed"},
			{audit.EventMCPSessionInit, "initialized"},
			{audit.EventConfigReloaded, "reloaded"},
			{audit.EventConfigRejected, "rejected"},
			{audit.EventCircuitBreakerTrip, "tripped"},
			{audit.EventImpersonation, "created"},
			{audit.EventJWTDelegation, "delegated"},
			{audit.EventSeverityTriageCompleted, "completed"},
			{audit.EventSeverityTriageFailed, "failed"},
			{audit.EventTriageStarted, "started"},
			{audit.EventTriageCompleted, "completed"},
			{audit.EventRRCreated, "created"},
			{audit.EventRRDeduplicated, "deduplicated"},
			{audit.EventKADelegated, "delegated"},
			{audit.EventKAResultReceived, "received"},
			{audit.EventUserDecision, "decided"},
		}
		for _, tc := range actions {
			tc := tc
			It("maps "+string(tc.eventType)+" to action "+tc.wantAction, func() {
				adapter.Emit(context.Background(), &audit.Event{
					Type:   tc.eventType,
					Detail: map[string]string{},
				})
				evt := store.lastEvent()
				Expect(evt).NotTo(BeNil())
				Expect(evt.EventAction).To(Equal(tc.wantAction))
			})
		}
	})

	Describe("UT-AF-1156-032: event_outcome for success-path events", func() {
		successEvents := []audit.EventType{
			audit.EventAuthSuccess, audit.EventSessionCreated, audit.EventSessionCompleted,
			audit.EventA2ATaskStarted, audit.EventA2ATaskCompleted, audit.EventToolExecuted,
			audit.EventConfigReloaded, audit.EventImpersonation, audit.EventJWTDelegation,
		}
		for _, et := range successEvents {
			et := et
			It(string(et)+" produces outcome success", func() {
				adapter.Emit(context.Background(), &audit.Event{Type: et, Detail: map[string]string{}})
				evt := store.lastEvent()
				Expect(evt).NotTo(BeNil())
				Expect(evt.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess))
			})
		}
	})

	Describe("UT-AF-1156-033: event_outcome for failure-path events", func() {
		failureEvents := []audit.EventType{
			audit.EventAuthFailure, audit.EventA2ATaskFailed, audit.EventMCPToolFailed,
			audit.EventSeverityTriageFailed, audit.EventConfigRejected,
			audit.EventAuthAccessDenied, audit.EventRateLimitDenied, audit.EventCircuitBreakerTrip,
		}
		for _, et := range failureEvents {
			et := et
			It(string(et)+" produces outcome failure", func() {
				adapter.Emit(context.Background(), &audit.Event{Type: et, Detail: map[string]string{}})
				evt := store.lastEvent()
				Expect(evt).NotTo(BeNil())
				Expect(evt.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure))
			})
		}
	})

	It("UT-AF-1156-034: sets actor_type=user when UserID present", func() {
		adapter.Emit(context.Background(), &audit.Event{
			Type:   audit.EventAuthSuccess,
			UserID: "alice",
			Detail: map[string]string{"auth_method": "jwt"},
		})
		evt := store.lastEvent()
		Expect(evt).NotTo(BeNil())
		Expect(evt.ActorType.Value).To(Equal("user"))
		Expect(evt.ActorID.Value).To(Equal("alice"))
	})

	It("UT-AF-1156-035: sets actor_type=service for system events", func() {
		adapter.Emit(context.Background(), &audit.Event{
			Type:   audit.EventCircuitBreakerTrip,
			Detail: map[string]string{"circuit_name": "ds", "failure_count": "5"},
		})
		evt := store.lastEvent()
		Expect(evt).NotTo(BeNil())
		Expect(evt.ActorType.Value).To(Equal("service"))
		Expect(evt.ActorID.Value).To(Equal("apifrontend"))
	})

	It("UT-AF-1156-036: uses CorrelationID when set", func() {
		adapter.Emit(context.Background(), &audit.Event{
			Type:          audit.EventSessionCreated,
			CorrelationID: "rr-abc-123",
			RequestID:     "req-456",
			Detail:        map[string]string{"session_id": "s1"},
		})
		evt := store.lastEvent()
		Expect(evt).NotTo(BeNil())
		Expect(evt.CorrelationID).To(Equal("rr-abc-123"))
	})

	It("UT-AF-1156-037: falls back to RequestID when CorrelationID empty", func() {
		adapter.Emit(context.Background(), &audit.Event{
			Type:      audit.EventAuthSuccess,
			RequestID: "req-789",
			Detail:    map[string]string{"auth_method": "jwt"},
		})
		evt := store.lastEvent()
		Expect(evt).NotTo(BeNil())
		Expect(evt.CorrelationID).To(Equal("req-789"))
	})

	It("UT-AF-1156-038: generates synthetic UUID when both empty", func() {
		adapter.Emit(context.Background(), &audit.Event{
			Type:   audit.EventAuthSuccess,
			Detail: map[string]string{"auth_method": "jwt"},
		})
		evt := store.lastEvent()
		Expect(evt).NotTo(BeNil())
		Expect(evt.CorrelationID).NotTo(BeEmpty())
		Expect(evt.CorrelationID).To(HaveLen(36))
	})

	It("UT-AF-1156-039: event_category is always apifrontend", func() {
		for _, et := range []audit.EventType{audit.EventAuthSuccess, audit.EventSessionCreated, audit.EventToolExecuted} {
			adapter.Emit(context.Background(), &audit.Event{Type: et, Detail: map[string]string{}})
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		for _, evt := range store.events {
			Expect(evt.EventCategory).To(Equal(ogenclient.AuditEventRequestEventCategoryApifrontend))
		}
	})

	It("UT-AF-1156-040: Close delegates to underlying store", func() {
		err := adapter.Close(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(store.closed).To(BeTrue())
	})
})
