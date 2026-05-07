package ds

import (
	"net/http"
	"testing"
	"time"
)

// UT-AF-038-030
func TestNewOgenClient_ConstructsWithTransport(t *testing.T) {
	client, err := NewOgenClient(OgenClientConfig{
		BaseURL:   "http://localhost:9090",
		Transport: http.DefaultTransport,
		Timeout:   10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewOgenClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewOgenClient() returned nil")
	}
	if client.client == nil {
		t.Fatal("NewOgenClient() internal client is nil")
	}
}

// UT-AF-038-038
func TestNewOgenClient_EmptyBaseURL(t *testing.T) {
	_, err := NewOgenClient(OgenClientConfig{
		BaseURL: "",
	})
	if err == nil {
		t.Fatal("NewOgenClient() expected error for empty baseURL")
	}
}

// UT-AF-038-038
func TestNewOgenClient_DefaultTimeout(t *testing.T) {
	client, err := NewOgenClient(OgenClientConfig{
		BaseURL: "http://localhost:9090",
		Timeout: 0,
	})
	if err != nil {
		t.Fatalf("NewOgenClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewOgenClient() returned nil with zero timeout (should use default)")
	}
}

// UT-AF-038-038
func TestNewOgenClient_NilTransport(t *testing.T) {
	client, err := NewOgenClient(OgenClientConfig{
		BaseURL:   "http://localhost:9090",
		Transport: nil,
	})
	if err != nil {
		t.Fatalf("NewOgenClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewOgenClient() returned nil with nil transport (should use default)")
	}
}
