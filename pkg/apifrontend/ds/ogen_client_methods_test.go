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

// UT-AF-038-035: GetAuditTrail sends limit=200 query parameter (AU-12)
func TestOgenClient_GetAuditTrail_SendsLimit(t *testing.T) {
	var capturedLimit string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		capturedLimit = r.URL.Query().Get("limit")
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := newTestOgenClient(t, mux)
	_, _ = client.GetAuditTrail(context.Background(), AuditTrailOpts{RRID: "rem-001"})
	if capturedLimit != "200" {
		t.Errorf("limit = %q, want \"200\"", capturedLimit)
	}
}

// UT-AF-038-036: Error response returns wrapped error
func TestOgenClient_GetRemediationHistory_ServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/remediation-history/context", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal failure"}`))
	})

	client := newTestOgenClient(t, mux)
	_, err := client.GetRemediationHistory(context.Background(), HistoryOpts{Kind: "Deployment", Name: "api", Namespace: "prod"})
	if err == nil {
		t.Fatal("GetRemediationHistory() expected error on 500 response")
	}
}

// UT-AF-1462-007: GetRemediationHistory sends currentSpecHash in query params
func TestOgenClient_GetRemediationHistory_SendsSpecHash(t *testing.T) {
	var capturedSpecHash string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		capturedSpecHash = r.URL.Query().Get("currentSpecHash")
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := newTestOgenClient(t, mux)
	_, _ = client.GetRemediationHistory(context.Background(), HistoryOpts{
		Kind: "Deployment", Name: "api", Namespace: "prod", SpecHash: "sha256:abc123",
	})
	if capturedSpecHash != "sha256:abc123" {
		t.Errorf("currentSpecHash = %q, want %q", capturedSpecHash, "sha256:abc123")
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

	_, err = client.GetRemediationHistory(context.Background(), HistoryOpts{Kind: "Deployment", Name: "api", Namespace: "prod"})
	if err == nil {
		t.Fatal("GetRemediationHistory() expected error on network failure")
	}
}

