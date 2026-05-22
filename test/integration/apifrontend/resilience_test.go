package apifrontend_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gobreaker "github.com/sony/gobreaker/v2"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/resilience"
)

var _ = Describe("Resilience Integration (resilience/)", func() {

	Describe("AC-21: Circuit breaker trips and recovers", func() {
		It("IT-AF-1195-031: CB trips after consecutive failures", func() {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer backend.Close()

			cbt := resilience.NewCircuitBreakerTransport(http.DefaultTransport, &resilience.CircuitBreakerConfig{
				Name:             "it-cb-031",
				MaxRequests:      1,
				Timeout:          100 * time.Millisecond,
				FailureThreshold: 3,
				FailureStatuses:  []int{500},
			})
			client := &http.Client{Transport: cbt}

			// Trigger failures to trip the CB
			for i := 0; i < 4; i++ {
				req, _ := http.NewRequest(http.MethodGet, backend.URL, nil)
				resp, _ := client.Do(req)
				if resp != nil {
					resp.Body.Close()
				}
			}

			// CB should be open -- next request should fail fast with ErrOpenState
			req, err := http.NewRequest(http.MethodGet, backend.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			_, err = client.Do(req)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, gobreaker.ErrOpenState)).To(BeTrue(),
				"expected gobreaker.ErrOpenState, got: %v", err)
		})

		It("IT-AF-1195-032: CB reopens after timeout (half-open recovery)", func() {
			requestsServed := int32(0)
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				count := atomic.AddInt32(&requestsServed, 1)
				if count <= 3 {
					w.WriteHeader(http.StatusInternalServerError)
				} else {
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer backend.Close()

			cbt := resilience.NewCircuitBreakerTransport(http.DefaultTransport, &resilience.CircuitBreakerConfig{
				Name:             "it-cb-032",
				MaxRequests:      1,
				Timeout:          200 * time.Millisecond,
				FailureThreshold: 3,
				FailureStatuses:  []int{500},
			})
			client := &http.Client{Transport: cbt}

			// Trip the CB
			for i := 0; i < 4; i++ {
				req, _ := http.NewRequest(http.MethodGet, backend.URL, nil)
				resp, _ := client.Do(req)
				if resp != nil {
					resp.Body.Close()
				}
			}

			// Wait for CB timeout to transition to half-open
			Eventually(func() error {
				req, _ := http.NewRequest(http.MethodGet, backend.URL, nil)
				resp, err := client.Do(req)
				if resp != nil {
					resp.Body.Close()
				}
				if err != nil {
					return err
				}
				return nil
			}).WithTimeout(2 * time.Second).WithPolling(250 * time.Millisecond).Should(Succeed())
		})
	})

	Describe("AC-22: Retry transport", func() {
		It("IT-AF-1195-033: retries on 503 then succeeds", func() {
			attempt := int32(0)
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				count := atomic.AddInt32(&attempt, 1)
				if count == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
				} else {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("ok"))
				}
			}))
			defer backend.Close()

			rt := resilience.NewRetryTransport(http.DefaultTransport, &resilience.RetryConfig{
				MaxAttempts:       3,
				InitialBackoff:    10 * time.Millisecond,
				MaxBackoff:        50 * time.Millisecond,
				RetryableStatuses: []int{503},
				DependencyName:    "it-retry-033",
			})
			client := &http.Client{Transport: rt}

			req, err := http.NewRequest(http.MethodGet, backend.URL, nil)
			Expect(err).NotTo(HaveOccurred())

			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(atomic.LoadInt32(&attempt)).To(BeNumerically(">=", 2),
				"should have retried at least once")
		})
	})

	Describe("AC-23: K8s dynamic client uses AF ServiceAccount (ADR-022)", func() {
		It("IT-AF-1195-034: StaticDynamicFactory returns the same client for any context", func() {
			identity := &auth.UserIdentity{
				Username: "test-user",
				Groups:   []string{"sre-team"},
			}
			ctx := auth.WithUserIdentity(context.Background(), identity)

			factory := auth.StaticDynamicFactory(nil)
			Expect(factory).NotTo(BeNil())

			dynClient, err := factory(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(dynClient).To(BeNil(), "StaticDynamicFactory(nil) returns the wrapped nil client")
		})
	})
})

