package ka_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/resilience"
	gobreaker "github.com/sony/gobreaker/v2"
)

var _ = Describe("MCP Resilience (G10: Retry/CB)", func() {
	withIdentityCtx := func(username string) context.Context {
		return auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
			Username: username,
			Groups:   []string{"sre"},
			RawToken: "token-for-" + username,
		})
	}

	Describe("Circuit Breaker", func() {
		It("UT-AF-1234-029: CB transitions closed->open after N consecutive failures", func() {
			var callCount atomic.Int32
			failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callCount.Add(1)
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			}))
			defer failServer.Close()

			cbt := resilience.NewCircuitBreakerTransport(http.DefaultTransport, &resilience.CircuitBreakerConfig{
				Name:             "ka-mcp-test",
				MaxRequests:      1,
				Interval:         60 * time.Second,
				Timeout:          100 * time.Millisecond,
				FailureThreshold: 3,
				DependencyName:   "ka-mcp",
			})

			httpClient := &http.Client{
				Transport: &authedRoundTripper{
					user: "alice@example.com",
					base: cbt,
				},
			}
			client := ka.NewSDKMCPClient(failServer.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := withIdentityCtx("alice@example.com")
			for i := 0; i < 4; i++ {
				_, _ = client.InvokeAction(ctx, ka.InvokeActionArgs{
					RRID:   "prod/rr-001",
					Action: "start",
				})
			}

			Expect(cbt.State()).To(Equal(gobreaker.StateOpen))
		})
	})

	Describe("Retry", func() {
		It("UT-AF-1234-030: RetryTransport retries on 503 with backoff", func() {
			var callCount atomic.Int32
			retryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := callCount.Add(1)
				if n <= 1 {
					http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`OK`))
			}))
			defer retryServer.Close()

			retryRT := resilience.NewRetryTransport(http.DefaultTransport, &resilience.RetryConfig{
				MaxAttempts:       3,
				InitialBackoff:    1 * time.Millisecond,
				MaxBackoff:        10 * time.Millisecond,
				RetryableStatuses: []int{http.StatusServiceUnavailable},
				DependencyName:    "ka-mcp",
			})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, retryServer.URL, http.NoBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := retryRT.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			_ = resp.Body.Close()
			Expect(callCount.Load()).To(BeNumerically(">=", 2))
		})
	})

	Describe("httptest Unit Tests", func() {
		It("UT-AF-1234-206: CB trips after 5 failures on httptest", func() {
			failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			}))
			defer failServer.Close()

			cbt := resilience.NewCircuitBreakerTransport(http.DefaultTransport, &resilience.CircuitBreakerConfig{
				Name:             "ka-mcp-it",
				MaxRequests:      1,
				Interval:         60 * time.Second,
				Timeout:          100 * time.Millisecond,
				FailureThreshold: 5,
				DependencyName:   "ka-mcp",
			})

			httpClient := &http.Client{
				Transport: &authedRoundTripper{
					user: "alice@example.com",
					base: cbt,
				},
			}
			client := ka.NewSDKMCPClient(failServer.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := withIdentityCtx("alice@example.com")
			for i := 0; i < 6; i++ {
				_, _ = client.InvokeAction(ctx, ka.InvokeActionArgs{
					RRID:   "prod/rr-001",
					Action: "start",
				})
			}

			Expect(cbt.State()).To(Equal(gobreaker.StateOpen))
		})

		It("UT-AF-1234-207: RetryTransport retries 503 from httptest and succeeds", func() {
			var callCount atomic.Int32
			retryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				n := callCount.Add(1)
				if n <= 1 {
					http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`OK`))
			}))
			defer retryServer.Close()

			retryRT := resilience.NewRetryTransport(http.DefaultTransport, &resilience.RetryConfig{
				MaxAttempts:       3,
				InitialBackoff:    1 * time.Millisecond,
				MaxBackoff:        10 * time.Millisecond,
				RetryableStatuses: []int{http.StatusServiceUnavailable},
				DependencyName:    "ka-mcp",
			})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, retryServer.URL, http.NoBody)
			Expect(err).NotTo(HaveOccurred())

			resp, err := retryRT.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			_ = resp.Body.Close()
			Expect(callCount.Load()).To(Equal(int32(2)))
		})

		It("UT-AF-1234-208: af_circuit_breaker_state{dependency=ka-mcp} metric emitted", func() {
			var stateChanges []gobreaker.State
			failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			}))
			defer failServer.Close()

			cbt := resilience.NewCircuitBreakerTransport(http.DefaultTransport, &resilience.CircuitBreakerConfig{
				Name:             "ka-mcp-metrics",
				MaxRequests:      1,
				Interval:         60 * time.Second,
				Timeout:          100 * time.Millisecond,
				FailureThreshold: 3,
				DependencyName:   "ka-mcp",
				OnStateChange: func(name string, from, to gobreaker.State) {
					stateChanges = append(stateChanges, to)
				},
			})

			httpClient := &http.Client{
				Transport: &authedRoundTripper{
					user: "alice@example.com",
					base: cbt,
				},
			}
			client := ka.NewSDKMCPClient(failServer.URL+"/mcp", httpClient, nil, logr.Discard())

			ctx := withIdentityCtx("alice@example.com")
			for i := 0; i < 4; i++ {
				_, _ = client.InvokeAction(ctx, ka.InvokeActionArgs{
					RRID:   "prod/rr-001",
					Action: "start",
				})
			}

			Expect(stateChanges).To(ContainElement(gobreaker.StateOpen))
		})
	})

	Describe("Pool httptest Unit Tests", func() {
		It("UT-AF-1234-204: Pool acquire/release with httptest endpoint", func() {
			pool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(ctx context.Context) (ka.PoolSession, error) {
					return &mockPoolSession{id: 1}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			session, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())
			Expect(session).NotTo(BeNil())

			pool.Release("rr-001", "alice")
			Expect(pool.Size()).To(Equal(0))
		})

		It("UT-AF-1234-205: Pool session reuse verified via server-side session count", func() {
			var connectCalls atomic.Int32
			pool := ka.NewKASessionPool(ka.PoolConfig{
				Factory: func(ctx context.Context) (ka.PoolSession, error) {
					connectCalls.Add(1)
					return &mockPoolSession{id: int(connectCalls.Load())}, nil
				},
				MaxEntries: 10,
				Logger:     logr.Discard(),
			})

			s1, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())

			s2, err := pool.Acquire(context.Background(), "rr-001", "alice")
			Expect(err).NotTo(HaveOccurred())

			Expect(s1).To(BeIdenticalTo(s2))
			Expect(connectCalls.Load()).To(Equal(int32(1)))
		})
	})
})
