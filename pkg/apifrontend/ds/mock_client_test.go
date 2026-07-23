package ds

import (
	"context"
	"errors"
	"testing"
)

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

func TestMockClient_ErrorPropagation(t *testing.T) {
	// Business outcome: errors from the mock are properly propagated to callers
	expectedErr := errors.New("connection refused")
	m := &MockClient{
		GetRemediationHistoryFn: func(_ context.Context, _ HistoryOpts) ([]HistoricalRemediation, error) {
			return nil, expectedErr
		},
	}
	_, err := m.GetRemediationHistory(context.Background(), HistoryOpts{})
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}
