package ds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
