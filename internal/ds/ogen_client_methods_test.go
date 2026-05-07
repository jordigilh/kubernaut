package ds

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
)

func newTestOgenClient(t *testing.T, handler http.Handler) *OgenClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	client, err := NewOgenClient(OgenClientConfig{
		BaseURL:   srv.URL,
		Transport: http.DefaultTransport,
		Timeout:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewOgenClient() error = %v", err)
	}
	return client
}

// UT-AF-038-031: ListWorkflows sends GET to correct path with query params
func TestOgenClient_ListWorkflows_CorrectPath(t *testing.T) {
	var capturedPath, capturedMethod string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := newTestOgenClient(t, mux)
	_, _ = client.ListWorkflows(context.Background(), ListWorkflowsOpts{Kind: "Deployment"})
	if capturedMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", capturedMethod)
	}
	if capturedPath != "/api/v1/workflows" {
		t.Errorf("path = %q, want /api/v1/workflows", capturedPath)
	}
}

// UT-AF-038-032: GetRemediationHistory sends GET to correct path
func TestOgenClient_GetRemediationHistory_CorrectPath(t *testing.T) {
	var capturedPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := newTestOgenClient(t, mux)
	_, _ = client.GetRemediationHistory(context.Background(), HistoryOpts{
		Kind: "Deployment", Name: "api", Namespace: "prod",
	})
	if capturedPath != "/api/v1/remediation-history/context" {
		t.Errorf("path = %q, want /api/v1/remediation-history/context", capturedPath)
	}
}

// UT-AF-038-033: GetEffectiveness sends GET to correct path with correlation_id
func TestOgenClient_GetEffectiveness_CorrectPath(t *testing.T) {
	var capturedPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := newTestOgenClient(t, mux)
	_, _ = client.GetEffectiveness(context.Background(), EffectivenessOpts{WorkflowID: "wf-123"})
	if capturedPath != "/api/v1/effectiveness/wf-123" {
		t.Errorf("path = %q, want /api/v1/effectiveness/wf-123", capturedPath)
	}
}

// UT-AF-038-034: GetAuditTrail sends GET to correct path
func TestOgenClient_GetAuditTrail_CorrectPath(t *testing.T) {
	var capturedPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := newTestOgenClient(t, mux)
	_, _ = client.GetAuditTrail(context.Background(), AuditTrailOpts{RRID: "rem-001"})
	if capturedPath != "/api/v1/audit/events" {
		t.Errorf("path = %q, want /api/v1/audit/events", capturedPath)
	}
}

// UT-AF-038-036: Error response returns wrapped error
func TestOgenClient_ListWorkflows_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/workflows", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal failure"}`))
	})

	client := newTestOgenClient(t, mux)
	_, err := client.ListWorkflows(context.Background(), ListWorkflowsOpts{})
	if err == nil {
		t.Fatal("ListWorkflows() expected error on 500 response")
	}
}

// UT-AF-038-037: Network failure returns wrapped error
func TestOgenClient_NetworkFailure(t *testing.T) {
	client, err := NewOgenClient(OgenClientConfig{
		BaseURL:   "http://127.0.0.1:1",
		Transport: http.DefaultTransport,
		Timeout:   100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewOgenClient() error = %v", err)
	}

	_, err = client.ListWorkflows(context.Background(), ListWorkflowsOpts{})
	if err == nil {
		t.Fatal("ListWorkflows() expected error on network failure")
	}
}

// UT-AF-PR6-001: WriteAuditEvents sends POST to correct path with batch payload
func TestOgenClient_WriteAuditEvents_CorrectPath(t *testing.T) {
	var capturedPath, capturedMethod string
	var capturedBody []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"created":2}`))
	})

	client := newTestOgenClient(t, mux)
	events := []*audit.Event{
		{
			Type:      audit.EventAuthSuccess,
			UserID:    "alice",
			RequestID: "req-001",
			Timestamp: time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
			Detail:    map[string]string{"action": "login"},
		},
		{
			Type:      audit.EventA2ATaskStarted,
			UserID:    "bob",
			RequestID: "req-002",
			Timestamp: time.Date(2026, 5, 1, 12, 1, 0, 0, time.UTC),
			Detail:    map[string]string{"task_id": "task-1"},
		},
	}

	err := client.WriteAuditEvents(context.Background(), events)
	if err != nil {
		t.Fatalf("WriteAuditEvents() error = %v", err)
	}
	if capturedMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", capturedMethod)
	}
	if capturedPath != "/api/v1/audit/events/batch" {
		t.Errorf("path = %q, want /api/v1/audit/events/batch", capturedPath)
	}

	var batch []map[string]interface{}
	if err := json.Unmarshal(capturedBody, &batch); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(batch) != 2 {
		t.Errorf("batch length = %d, want 2", len(batch))
	}
}

// UT-AF-PR6-002: WriteAuditEvents with empty slice is a no-op
func TestOgenClient_WriteAuditEvents_EmptySlice(t *testing.T) {
	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	client := newTestOgenClient(t, mux)
	err := client.WriteAuditEvents(context.Background(), []*audit.Event{})
	if err != nil {
		t.Fatalf("WriteAuditEvents() error = %v", err)
	}
	if called {
		t.Error("WriteAuditEvents() should not call server for empty batch")
	}
}

// UT-AF-PR6-003: WriteAuditEvents server error returns wrapped error
func TestOgenClient_WriteAuditEvents_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := newTestOgenClient(t, mux)
	events := []*audit.Event{
		{
			Type:      audit.EventAuthSuccess,
			UserID:    "alice",
			Timestamp: time.Now(),
		},
	}
	err := client.WriteAuditEvents(context.Background(), events)
	if err == nil {
		t.Fatal("WriteAuditEvents() expected error on 500 response")
	}
}
