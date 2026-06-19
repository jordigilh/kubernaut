package tools_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ds"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("kubernaut_get_audit_trail", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-124-001: returns aggregated audit trail with lifecycle and phases (AU-3)", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(_ context.Context, opts ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				return []ds.AuditEvent{
					{Timestamp: "2026-05-01T10:00:00Z", EventType: "gateway.signal.received", Actor: "alertmanager"},
					{Timestamp: "2026-05-01T10:05:00Z", EventType: "orchestrator.approval.granted", Actor: "bob"},
				}, nil
			},
		}
		result, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.TotalEvents).To(Equal(2))
		Expect(result.Lifecycle).NotTo(BeEmpty(), "lifecycle summary must be populated")
		Expect(result.Phases).NotTo(BeEmpty(), "phases must be populated")
	})

	It("UT-AF-124-002: empty events produce empty lifecycle and zero TotalEvents (AU-6)", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(_ context.Context, opts ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				return nil, nil
			},
		}
		result, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/missing"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.TotalEvents).To(Equal(0))
		Expect(result.Lifecycle).To(BeEmpty())
		Expect(result.Phases).To(BeEmpty())
	})

	It("UT-AF-124-003: DS unavailable", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(_ context.Context, opts ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				return nil, fmt.Errorf("connection refused")
			},
		}
		_, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/rr-1"})
		Expect(err).To(HaveOccurred())
	})

	It("UT-AF-124-004: filter by event type still works, result is aggregated (AU-2)", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(_ context.Context, opts ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				Expect(opts.EventType).To(Equal("orchestrator.approval.granted"))
				return []ds.AuditEvent{{EventType: "orchestrator.approval.granted", Actor: "bob", Timestamp: "2026-05-01T10:00:00Z"}}, nil
			},
		}
		result, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{
			RRID: "pay/rr-1", EventType: "orchestrator.approval.granted",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.TotalEvents).To(Equal(1))
		Expect(result.Phases).To(HaveLen(1))
	})

	It("UT-AF-124-005: 10 events across 3 phases produce 3 PhaseGroups in lifecycle order (AU-3)", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(_ context.Context, _ ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				return []ds.AuditEvent{
					{Timestamp: "2026-05-01T10:00:00Z", EventType: "gateway.signal.received", Actor: "alertmanager"},
					{Timestamp: "2026-05-01T10:00:01Z", EventType: "gateway.signal.deduplicated", Actor: "gateway"},
					{Timestamp: "2026-05-01T10:00:02Z", EventType: "signalprocessing.enriched", Actor: "gateway"},
					{Timestamp: "2026-05-01T10:01:00Z", EventType: "aiagent.session.started", Actor: "agent"},
					{Timestamp: "2026-05-01T10:02:00Z", EventType: "aiagent.rca.completed", Actor: "agent"},
					{Timestamp: "2026-05-01T10:02:30Z", EventType: "aiagent.llm.invoked", Actor: "agent"},
					{Timestamp: "2026-05-01T10:03:00Z", EventType: "aiagent.response.generated", Actor: "agent"},
					{Timestamp: "2026-05-01T10:04:00Z", EventType: "orchestrator.lifecycle.completed", Actor: "orchestrator"},
					{Timestamp: "2026-05-01T10:04:01Z", EventType: "workflowexecution.execution.started", Actor: "executor"},
					{Timestamp: "2026-05-01T10:05:00Z", EventType: "workflowexecution.execution.completed", Actor: "executor"},
				}, nil
			},
		}
		result, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.TotalEvents).To(Equal(10))
		Expect(result.Phases).To(HaveLen(3), "3 distinct phases: Signal Processing, Investigation, Execution")

		Expect(result.Phases[0].Phase).To(Equal("Signal Processing"))
		Expect(result.Phases[0].EventCount).To(Equal(3))
		Expect(result.Phases[1].Phase).To(Equal("Investigation"))
		Expect(result.Phases[1].EventCount).To(Equal(4))
		Expect(result.Phases[2].Phase).To(Equal("Execution"))
		Expect(result.Phases[2].EventCount).To(Equal(3))
	})

	It("UT-AF-124-006: lifecycle summary follows phase order (AU-6)", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(_ context.Context, _ ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				return []ds.AuditEvent{
					{Timestamp: "2026-05-01T10:00:00Z", EventType: "gateway.signal.received", Actor: "alertmanager"},
					{Timestamp: "2026-05-01T10:01:00Z", EventType: "aiagent.session.started", Actor: "agent"},
					{Timestamp: "2026-05-01T10:04:00Z", EventType: "orchestrator.lifecycle.completed", Actor: "orchestrator"},
				}, nil
			},
		}
		result, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Lifecycle).To(Equal("Signal Processing -> Investigation -> Execution"))
	})

	It("UT-AF-124-007: unknown event types aggregate into Other phase (AU-12)", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(_ context.Context, _ ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				return []ds.AuditEvent{
					{Timestamp: "2026-05-01T10:00:00Z", EventType: "gateway.signal.received", Actor: "alertmanager"},
					{Timestamp: "2026-05-01T10:01:00Z", EventType: "custom.unknown.event", Actor: "system"},
					{Timestamp: "2026-05-01T10:02:00Z", EventType: "another.unmapped.type", Actor: "system"},
				}, nil
			},
		}
		result, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.TotalEvents).To(Equal(3), "no events dropped from unmapped types")

		otherFound := false
		for _, p := range result.Phases {
			if p.Phase == "Other" {
				otherFound = true
				Expect(p.EventCount).To(Equal(2))
			}
		}
		Expect(otherFound).To(BeTrue(), "unknown event types must appear in Other phase")
	})

	It("UT-AF-124-008: single-event remediation produces 1 phase (AU-2)", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(_ context.Context, _ ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				return []ds.AuditEvent{
					{Timestamp: "2026-05-01T10:00:00Z", EventType: "gateway.signal.received", Actor: "alertmanager"},
				}, nil
			},
		}
		result, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.TotalEvents).To(Equal(1))
		Expect(result.Phases).To(HaveLen(1))
		Expect(result.Phases[0].Phase).To(Equal("Signal Processing"))
		Expect(result.Phases[0].EventCount).To(Equal(1))
	})

	It("UT-AF-124-010: KeyActions capped when many events produce long concatenation (AU-3)", func() {
		events := make([]ds.AuditEvent, 100)
		for i := range events {
			events[i] = ds.AuditEvent{
				Timestamp: fmt.Sprintf("2026-05-01T10:%02d:00Z", i%60),
				EventType: "aiagent.rca.completed",
				Actor:     "agent",
				Detail:    strings.Repeat("x", 100),
			}
		}
		mock := &ds.MockClient{
			GetAuditTrailFn: func(_ context.Context, _ ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				return events, nil
			},
		}
		result, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.TotalEvents).To(Equal(100))
		Expect(result.Phases).To(HaveLen(1))
		Expect(len(result.Phases[0].KeyActions)).To(BeNumerically("<=", 1024+len("... and 99 more actions")),
			"KeyActions must be capped to prevent oversized results")
	})

	It("UT-AF-124-009: phase groups include timestamps, actor, and outcome (AU-3)", func() {
		mock := &ds.MockClient{
			GetAuditTrailFn: func(_ context.Context, _ ds.AuditTrailOpts) ([]ds.AuditEvent, error) {
				return []ds.AuditEvent{
					{Timestamp: "2026-05-01T10:00:00Z", EventType: "orchestrator.approval.requested", Actor: "orchestrator", Detail: "approval requested"},
					{Timestamp: "2026-05-01T10:05:00Z", EventType: "orchestrator.approval.granted", Actor: "admin", Detail: "approved by admin"},
				}, nil
			},
		}
		result, err := tools.HandleGetAuditTrail(ctx, mock, tools.GetAuditTrailArgs{RRID: "pay/rr-1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Phases).To(HaveLen(1))

		phase := result.Phases[0]
		Expect(phase.Phase).To(Equal("Approval"))
		Expect(phase.StartTime).To(Equal("2026-05-01T10:00:00Z"))
		Expect(phase.EndTime).To(Equal("2026-05-01T10:05:00Z"))
		Expect(phase.EventCount).To(Equal(2))
		Expect(phase.Actor).NotTo(BeEmpty(), "actor attribution must be present")
		Expect(phase.KeyActions).NotTo(BeEmpty(), "key actions must summarize phase activity")
	})
})
