package http

import (
	"crypto/tls"
	"net/http"
	"time"
)

// ClientConfig holds configuration for HTTP client creation
type ClientConfig struct {
	// Timeout for HTTP requests
	Timeout time.Duration

	// MaxRetries for failed requests (future enhancement)
	MaxRetries int

	// DisableSSLVerification for development/testing
	DisableSSLVerification bool

	// MaxIdleConns controls the maximum number of idle connections
	MaxIdleConns int

	// IdleConnTimeout controls the maximum amount of time an idle connection will remain idle
	IdleConnTimeout time.Duration

	// TLSHandshakeTimeout specifies the maximum amount of time waiting for TLS handshake
	TLSHandshakeTimeout time.Duration

	// ResponseHeaderTimeout specifies the amount of time to wait for response headers
	ResponseHeaderTimeout time.Duration
}

// DefaultClientConfig returns a client configuration with sensible defaults
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Timeout:                30 * time.Second,
		MaxRetries:             3,
		DisableSSLVerification: false,
		MaxIdleConns:           10,
		IdleConnTimeout:        90 * time.Second,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  10 * time.Second,
	}
}

// NewClient creates an HTTP client with the given configuration
func NewClient(config ClientConfig) *http.Client {
	// Create transport with configured values
	transport := &http.Transport{
		MaxIdleConns:          config.MaxIdleConns,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   config.TLSHandshakeTimeout,
		ResponseHeaderTimeout: config.ResponseHeaderTimeout,
	}

	// Configure TLS if needed
	if config.DisableSSLVerification {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	return &http.Client{
		Timeout:   config.Timeout,
		Transport: transport,
	}
}

// NewClientWithTimeout creates an HTTP client with just a timeout (convenience function)
func NewClientWithTimeout(timeout time.Duration) *http.Client {
	config := DefaultClientConfig()
	config.Timeout = timeout
	return NewClient(config)
}

// NewDefaultClient creates an HTTP client with default configuration
func NewDefaultClient() *http.Client {
	return NewClient(DefaultClientConfig())
}

// Common preset configurations

// SlackClientConfig returns configuration optimized for Slack webhook calls
func SlackClientConfig() ClientConfig {
	config := DefaultClientConfig()
	config.Timeout = 10 * time.Second
	config.MaxRetries = 2
	return config
}

// PrometheusClientConfig returns configuration optimized for Prometheus API calls
func PrometheusClientConfig(timeout time.Duration) ClientConfig {
	config := DefaultClientConfig()
	config.Timeout = timeout
	config.ResponseHeaderTimeout = timeout / 2
	return config
}

// LLMClientConfig returns configuration optimized for LLM API calls
func LLMClientConfig(timeout time.Duration) ClientConfig {
	config := DefaultClientConfig()
	config.Timeout = timeout
	config.ResponseHeaderTimeout = timeout / 3 // Allow more time for LLM processing
	return config
}

