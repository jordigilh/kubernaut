package ds

import (
	"context"
	"errors"
	"testing"

	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
)

func TestMockClient_ListWorkflows(t *testing.T) {
	expected := []Workflow{{ID: "wf-1", Name: "restart"}}
	m := &MockClient{
		ListWorkflowsFn: func(_ context.Context, _ ListWorkflowsOpts) ([]Workflow, error) {
			return expected, nil
		},
	}
	got, err := m.ListWorkflows(context.Background(), ListWorkflowsOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "wf-1" {
		t.Errorf("expected %v, got %v", expected, got)
	}
}

func TestMockClient_GetRemediationHistory(t *testing.T) {
	expected := []HistoricalRemediation{{ID: "r-1", Phase: "Completed"}}
	m := &MockClient{
		GetRemediationHistoryFn: func(_ context.Context, _ HistoryOpts) ([]HistoricalRemediation, error) {
			return expected, nil
		},
	}
	got, err := m.GetRemediationHistory(context.Background(), HistoryOpts{Namespace: "default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "r-1" {
		t.Errorf("expected %v, got %v", expected, got)
	}
}

func TestMockClient_GetEffectiveness(t *testing.T) {
	expected := &EffectivenessReport{SuccessRate: 0.95, SampleSize: 20}
	m := &MockClient{
		GetEffectivenessFn: func(_ context.Context, _ EffectivenessOpts) (*EffectivenessReport, error) {
			return expected, nil
		},
	}
	got, err := m.GetEffectiveness(context.Background(), EffectivenessOpts{WorkflowID: "wf-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SuccessRate != 0.95 {
		t.Errorf("expected 0.95, got %v", got.SuccessRate)
	}
}

func TestMockClient_GetAuditTrail(t *testing.T) {
	expected := []AuditEvent{{EventType: "tool_invoked", Actor: "alice"}}
	m := &MockClient{
		GetAuditTrailFn: func(_ context.Context, _ AuditTrailOpts) ([]AuditEvent, error) {
			return expected, nil
		},
	}
	got, err := m.GetAuditTrail(context.Background(), AuditTrailOpts{RRID: "rr-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Actor != "alice" {
		t.Errorf("expected %v, got %v", expected, got)
	}
}

func TestMockClient_WriteAuditEvents_DelegatesToFn(t *testing.T) {
	var called bool
	m := &MockClient{
		WriteAuditEventsFn: func(_ context.Context, events []*audit.Event) error {
			called = true
			if len(events) != 1 {
				t.Errorf("expected 1 event, got %d", len(events))
			}
			return nil
		},
	}
	err := m.WriteAuditEvents(context.Background(), []*audit.Event{{Type: "test"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("WriteAuditEventsFn was not called")
	}
}

func TestMockClient_WriteAuditEvents_NilFnReturnsNil(t *testing.T) {
	// Business outcome: when WriteAuditEventsFn is nil, writes are silently discarded
	m := &MockClient{}
	err := m.WriteAuditEvents(context.Background(), []*audit.Event{{Type: "test"}})
	if err != nil {
		t.Fatalf("expected nil error when WriteAuditEventsFn is nil, got %v", err)
	}
}

func TestMockClient_ErrorPropagation(t *testing.T) {
	// Business outcome: errors from the mock are properly propagated to callers
	expectedErr := errors.New("connection refused")
	m := &MockClient{
		ListWorkflowsFn: func(_ context.Context, _ ListWorkflowsOpts) ([]Workflow, error) {
			return nil, expectedErr
		},
	}
	_, err := m.ListWorkflows(context.Background(), ListWorkflowsOpts{})
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}
