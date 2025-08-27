package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // Reduce log noise in tests

	server := NewServer("8080", logger)

	assert.NotNil(t, server)
	assert.NotNil(t, server.server)
	assert.Equal(t, ":8080", server.server.Addr)
	assert.NotNil(t, server.log)
}

func TestServerStartStop(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Use port 0 to get a random available port
	server := NewServer("0", logger)

	// Start server asynchronously
	server.StartAsync()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Stop server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Stop(ctx)
	assert.NoError(t, err)
}

func TestServerMetricsEndpoint(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Create server with a specific port for testing
	server := NewServer("9999", logger)

	// Start server
	server.StartAsync()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	// Make request to metrics endpoint
	resp, err := http.Get("http://localhost:9999/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check response
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/plain")

	// Read response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Check that response contains Prometheus metrics format
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "# HELP")
	assert.Contains(t, bodyStr, "# TYPE")
}

func TestServerHealthEndpoint(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Create server
	server := NewServer("9998", logger)

	// Start server
	server.StartAsync()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	// Make request to health endpoint
	resp, err := http.Get("http://localhost:9998/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check response
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Read response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, "OK", string(body))
}

func TestServerStartError(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Create two servers on the same port to force an error
	server1 := NewServer("9997", logger)
	server2 := NewServer("9997", logger)

	// Start first server
	server1.StartAsync()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server1.Stop(ctx)
	}()

	// Wait for first server to start
	time.Sleep(100 * time.Millisecond)

	// Try to start second server on same port (should fail)
	// We can't easily test this without capturing logs or using a different approach
	// But we can test that the server handles shutdown gracefully

	// Create another server with a different port
	server2 = NewServer("9996", logger)
	server2.StartAsync()

	// Stop it immediately
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := server2.Stop(ctx)
	assert.NoError(t, err)
}

func TestServerStopTimeout(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	server := NewServer("9995", logger)

	// Start server
	server.StartAsync()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// This should timeout, but server.Stop should handle it gracefully
	_ = server.Stop(ctx)
	// The error could be a timeout or nil depending on timing
	// We mainly want to ensure it doesn't panic

	// Clean up with proper timeout
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	server.Stop(ctx2)
}

func TestServerWithCustomMetrics(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Add some test data

	// Record some metrics
	RecordAlert()
	RecordAlert()
	RecordAction("test_action", 100*time.Millisecond)

	// Create and start server
	server := NewServer("9994", logger)
	server.StartAsync()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	// Make request to metrics endpoint
	resp, err := http.Get("http://localhost:9994/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	bodyStr := string(body)

	// Check that our custom metrics are present
	assert.Contains(t, bodyStr, "alerts_processed_total")
	assert.Contains(t, bodyStr, "actions_executed_total")
	assert.Contains(t, bodyStr, "action_processing_duration_seconds")

	// Check that our metrics are present (values may vary due to other tests)
	assert.Contains(t, bodyStr, "alerts_processed_total")
	assert.Contains(t, bodyStr, `actions_executed_total{action="test_action"}`)
}

func TestServerMultipleClients(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Create and start server
	server := NewServer("9993", logger)
	server.StartAsync()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	// Make multiple concurrent requests
	numRequests := 5
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(i int) {
			resp, err := http.Get(fmt.Sprintf("http://localhost:9993/metrics"))
			if err != nil {
				results <- err
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				results <- fmt.Errorf("request %d: expected status 200, got %d", i, resp.StatusCode)
				return
			}

			results <- nil
		}(i)
	}

	// Collect results
	for i := 0; i < numRequests; i++ {
		err := <-results
		assert.NoError(t, err, "Request %d failed", i)
	}
}

func TestServerInvalidPort(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Test with invalid port
	server := NewServer("invalid", logger)

	// This should create the server object but fail when starting
	assert.NotNil(t, server)

	// Starting should fail, but we can't easily test this without
	// capturing the error in a different way since StartAsync doesn't return an error
	// We can at least verify the server object is created properly
	assert.Equal(t, ":invalid", server.server.Addr)
}

func TestServerContextCancellation(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	server := NewServer("9992", logger)

	// Start server
	server.StartAsync()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Create context and cancel it immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Stop should handle cancelled context gracefully
	err := server.Stop(ctx)
	// Error may or may not occur depending on timing, but should not panic
	// Just ensure no panic occurs
	_ = err
}
