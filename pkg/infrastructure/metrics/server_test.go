package metrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

var _ = Describe("Server", func() {
	var logger *logrus.Logger

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel) // Reduce log noise in tests
	})

	Describe("NewServer", func() {
		It("should create a server with correct configuration", func() {
			server := NewServer("8080", logger)

			Expect(server).ToNot(BeNil())
			Expect(server.server).ToNot(BeNil())
			Expect(server.server.Addr).To(Equal(":8080"))
			Expect(server.log).ToNot(BeNil())
		})
	})

	Describe("Server lifecycle", func() {
		It("should start and stop server successfully", func() {
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
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Metrics endpoint", func() {
		It("should serve metrics in Prometheus format", func() {
			// Create server with a specific port for testing
			server := NewServer("9999", logger)

			// Start server
			server.StartAsync()
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = server.Stop(ctx)
			}()

			// Wait for server to start
			time.Sleep(200 * time.Millisecond)

			// Make request to metrics endpoint
			resp, err := http.Get("http://localhost:9999/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Check response
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/plain"))

			// Read response body
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			// Check that response contains Prometheus metrics format
			bodyStr := string(body)
			Expect(bodyStr).To(ContainSubstring("# HELP"))
			Expect(bodyStr).To(ContainSubstring("# TYPE"))
		})
	})

	Describe("Health endpoint", func() {
		It("should return OK status", func() {
			// Create server
			server := NewServer("9998", logger)

			// Start server
			server.StartAsync()
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = server.Stop(ctx)
			}()

			// Wait for server to start
			time.Sleep(200 * time.Millisecond)

			// Make request to health endpoint
			resp, err := http.Get("http://localhost:9998/health")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Check response
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Read response body
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			Expect(string(body)).To(Equal("OK"))
		})
	})

	Describe("Error handling", func() {
		It("should handle server start and stop gracefully", func() {
			// Create two servers on different ports to avoid conflicts
			server1 := NewServer("9997", logger)
			server2 := NewServer("9996", logger)

			// Start first server
			server1.StartAsync()
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = server1.Stop(ctx)
			}()

			// Wait for first server to start
			time.Sleep(100 * time.Millisecond)

			// Start second server on different port
			server2.StartAsync()

			// Stop it immediately
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			err := server2.Stop(ctx)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle stop timeout gracefully", func() {
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
			_ = server.Stop(ctx2)
		})

		It("should handle context cancellation gracefully", func() {
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
			_ = err
		})
	})

	Describe("Custom metrics", func() {
		It("should serve custom metrics correctly", func() {
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
				_ = server.Stop(ctx)
			}()

			// Wait for server to start
			time.Sleep(200 * time.Millisecond)

			// Make request to metrics endpoint
			resp, err := http.Get("http://localhost:9994/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()

			// Read response body
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			bodyStr := string(body)

			// Check that our custom metrics are present
			Expect(bodyStr).To(ContainSubstring("alerts_processed_total"))
			Expect(bodyStr).To(ContainSubstring("actions_executed_total"))
			Expect(bodyStr).To(ContainSubstring("action_processing_duration_seconds"))
			Expect(bodyStr).To(ContainSubstring(`actions_executed_total{action="test_action"}`))
		})
	})

	Describe("Concurrent access", func() {
		It("should handle multiple concurrent clients", func() {
			// Create and start server
			server := NewServer("9993", logger)
			server.StartAsync()
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = server.Stop(ctx)
			}()

			// Wait for server to start
			time.Sleep(200 * time.Millisecond)

			// Make multiple concurrent requests
			numRequests := 5
			results := make(chan error, numRequests)

			for i := 0; i < numRequests; i++ {
				go func(i int) {
					defer GinkgoRecover()
					resp, err := http.Get("http://localhost:9993/metrics")
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
				Expect(err).ToNot(HaveOccurred(), "Request %d failed", i)
			}
		})
	})

	Describe("Configuration", func() {
		It("should handle invalid port configuration", func() {
			// Test with invalid port
			server := NewServer("invalid", logger)

			// This should create the server object but fail when starting
			Expect(server).ToNot(BeNil())

			// Starting should fail, but we can't easily test this without
			// capturing the error in a different way since StartAsync doesn't return an error
			// We can at least verify the server object is created properly
			Expect(server.server.Addr).To(Equal(":invalid"))
		})
	})
})
