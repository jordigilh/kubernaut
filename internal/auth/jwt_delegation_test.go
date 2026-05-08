package auth_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
)

func TestContextJWTDelegationTransport_SetsHeader(t *testing.T) {
	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	transport := &auth.ContextJWTDelegationTransport{Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
		Username: "alice",
		RawToken: "jwt-abc-123",
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if capturedHeader != "Bearer jwt-abc-123" {
		t.Errorf("Authorization = %q, want %q", capturedHeader, "Bearer jwt-abc-123")
	}
}

func TestContextJWTDelegationTransport_NoIdentity(t *testing.T) {
	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	transport := &auth.ContextJWTDelegationTransport{Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if capturedHeader != "" {
		t.Errorf("Authorization = %q, want empty (no identity)", capturedHeader)
	}
}

func TestContextJWTDelegationTransport_EmptyRawToken(t *testing.T) {
	var capturedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	transport := &auth.ContextJWTDelegationTransport{Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
		Username: "bob",
		RawToken: "",
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if capturedHeader != "" {
		t.Errorf("Authorization = %q, want empty (no raw token)", capturedHeader)
	}
}

func TestContextJWTDelegationTransport_NilBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := &auth.ContextJWTDelegationTransport{Base: nil}
	client := &http.Client{Transport: transport}

	ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
		Username: "carol",
		RawToken: "token-carol",
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (nil Base falls back to DefaultTransport)", resp.StatusCode)
	}
}

func TestContextJWTDelegationTransport_DoesNotMutateOriginalRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	transport := &auth.ContextJWTDelegationTransport{Base: http.DefaultTransport}
	client := &http.Client{Transport: transport}

	ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
		Username: "dave",
		RawToken: "token-dave",
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)

	originalAuth := req.Header.Get("Authorization")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if req.Header.Get("Authorization") != originalAuth {
		t.Error("original request was mutated (Authorization header changed)")
	}
}
