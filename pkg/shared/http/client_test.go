package http

import (
	"testing"
	"time"
)

func TestDefaultClientConfig(t *testing.T) {
	config := DefaultClientConfig()

	if config.Timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", config.Timeout)
	}

	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", config.MaxRetries)
	}

	if config.DisableSSLVerification {
		t.Error("Expected DisableSSLVerification to be false")
	}

	if config.MaxIdleConns != 10 {
		t.Errorf("Expected MaxIdleConns 10, got %d", config.MaxIdleConns)
	}
}

func TestNewClient(t *testing.T) {
	config := ClientConfig{
		Timeout:                30 * time.Second,
		MaxRetries:             2,
		DisableSSLVerification: false,
		MaxIdleConns:           5,
		IdleConnTimeout:        60 * time.Second,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  5 * time.Second,
	}

	client := NewClient(config)

	if client == nil {
		t.Fatal("Expected client to be created")
	}

	if client.Timeout != config.Timeout {
		t.Errorf("Expected timeout %v, got %v", config.Timeout, client.Timeout)
	}

	// Check that transport is configured
	if client.Transport == nil {
		t.Error("Expected transport to be configured")
	}
}

func TestNewClientWithTimeout(t *testing.T) {
	timeout := 15 * time.Second
	client := NewClientWithTimeout(timeout)

	if client == nil {
		t.Fatal("Expected client to be created")
	}

	if client.Timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, client.Timeout)
	}
}

func TestNewDefaultClient(t *testing.T) {
	client := NewDefaultClient()

	if client == nil {
		t.Fatal("Expected client to be created")
	}

	if client.Timeout != 30*time.Second {
		t.Errorf("Expected default timeout 30s, got %v", client.Timeout)
	}
}

func TestSlackClientConfig(t *testing.T) {
	config := SlackClientConfig()

	if config.Timeout != 10*time.Second {
		t.Errorf("Expected Slack timeout 10s, got %v", config.Timeout)
	}

	if config.MaxRetries != 2 {
		t.Errorf("Expected Slack MaxRetries 2, got %d", config.MaxRetries)
	}
}

func TestPrometheusClientConfig(t *testing.T) {
	timeout := 20 * time.Second
	config := PrometheusClientConfig(timeout)

	if config.Timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, config.Timeout)
	}

	expectedResponseTimeout := timeout / 2
	if config.ResponseHeaderTimeout != expectedResponseTimeout {
		t.Errorf("Expected ResponseHeaderTimeout %v, got %v", expectedResponseTimeout, config.ResponseHeaderTimeout)
	}
}

func TestLLMClientConfig(t *testing.T) {
	timeout := 60 * time.Second
	config := LLMClientConfig(timeout)

	if config.Timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, config.Timeout)
	}

	expectedResponseTimeout := timeout / 3
	if config.ResponseHeaderTimeout != expectedResponseTimeout {
		t.Errorf("Expected ResponseHeaderTimeout %v, got %v", expectedResponseTimeout, config.ResponseHeaderTimeout)
	}
}

func TestNewClientWithSSLDisabled(t *testing.T) {
	config := DefaultClientConfig()
	config.DisableSSLVerification = true

	client := NewClient(config)

	if client == nil {
		t.Fatal("Expected client to be created")
	}

	// We can't easily test the TLS config without making actual requests,
	// but we can ensure the client was created successfully
	if client.Transport == nil {
		t.Error("Expected transport to be configured")
	}
}

// Benchmark tests
func BenchmarkNewClient(b *testing.B) {
	config := DefaultClientConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewClient(config)
	}
}

func BenchmarkNewClientWithTimeout(b *testing.B) {
	timeout := 10 * time.Second

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewClientWithTimeout(timeout)
	}
}

