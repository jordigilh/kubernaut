/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package transport

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/circuitbreaker"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

var _ = Describe("CircuitBreakerTransport — OPS-2 / BR-AI-982", func() {

	Describe("UT-CB-001: Disabled config returns passthrough", func() {
		It("should return the inner transport unchanged when Enabled is false", func() {
			inner := http.DefaultTransport
			result := transport.NewCircuitBreakerTransport(inner, transport.CircuitBreakerConfig{
				Enabled: false,
				Name:    "test",
			})
			Expect(result).To(BeIdenticalTo(inner))
		})
	})

	Describe("UT-CB-002: Successful requests pass through", func() {
		It("should forward 200 responses without interference", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			rt := transport.NewCircuitBreakerTransport(server.Client().Transport, transport.CircuitBreakerConfig{
				Enabled:          true,
				Name:             "test-ok",
				MaxRequests:      3,
				Interval:         10 * time.Second,
				Timeout:          1 * time.Second,
				FailureThreshold: 3,
				FailureRatio:     0.5,
			})

			for i := 0; i < 5; i++ {
				req, err := http.NewRequest(http.MethodGet, server.URL, nil)
				Expect(err).NotTo(HaveOccurred())
				resp, err := rt.RoundTrip(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				resp.Body.Close()
			}
		})
	})

	Describe("UT-CB-003: Circuit opens after failure threshold", func() {
		It("should return ErrOpenState after sufficient 503 responses", func() {
			var callCount atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callCount.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()

			rt := transport.NewCircuitBreakerTransport(server.Client().Transport, transport.CircuitBreakerConfig{
				Enabled:          true,
				Name:             "test-503",
				MaxRequests:      1,
				Interval:         60 * time.Second,
				Timeout:          30 * time.Second,
				FailureThreshold: 3,
				FailureRatio:     0.5,
			})

			for i := 0; i < 5; i++ {
				req, err := http.NewRequest(http.MethodGet, server.URL, nil)
				Expect(err).NotTo(HaveOccurred())
				_, _ = rt.RoundTrip(req)
			}

			req, err := http.NewRequest(http.MethodGet, server.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			_, err = rt.RoundTrip(req)
			Expect(err).To(MatchError(circuitbreaker.ErrOpenState))

			Expect(callCount.Load()).To(BeNumerically("<", 6),
				"circuit should have opened, preventing some requests from reaching the server")
		})
	})

	Describe("UT-CB-004: 4xx responses do not trip the circuit", func() {
		It("should keep the circuit closed on client errors", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			rt := transport.NewCircuitBreakerTransport(server.Client().Transport, transport.CircuitBreakerConfig{
				Enabled:          true,
				Name:             "test-404",
				MaxRequests:      1,
				Interval:         60 * time.Second,
				Timeout:          30 * time.Second,
				FailureThreshold: 3,
				FailureRatio:     0.5,
			})

			for i := 0; i < 10; i++ {
				req, err := http.NewRequest(http.MethodGet, server.URL, nil)
				Expect(err).NotTo(HaveOccurred())
				resp, err := rt.RoundTrip(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
				resp.Body.Close()
			}

			cbt, ok := rt.(*transport.CircuitBreakerTransport)
			Expect(ok).To(BeTrue())
			Expect(cbt.State()).To(Equal(circuitbreaker.StateClosed))
		})
	})

	Describe("UT-CB-005: OnStateChange callback fires", func() {
		It("should invoke the callback when the circuit transitions", func() {
			var transitions []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			}))
			defer server.Close()

			rt := transport.NewCircuitBreakerTransport(server.Client().Transport, transport.CircuitBreakerConfig{
				Enabled:          true,
				Name:             "test-cb",
				MaxRequests:      1,
				Interval:         60 * time.Second,
				Timeout:          30 * time.Second,
				FailureThreshold: 3,
				FailureRatio:     0.5,
				OnStateChange: func(name string, from, to circuitbreaker.State) {
					transitions = append(transitions, fmt.Sprintf("%s->%s", from.String(), to.String()))
				},
			})

			for i := 0; i < 5; i++ {
				req, err := http.NewRequest(http.MethodGet, server.URL, nil)
				Expect(err).NotTo(HaveOccurred())
				_, _ = rt.RoundTrip(req)
			}

			Expect(transitions).To(ContainElement("closed->open"))
		})
	})

	Describe("UT-CB-006: Logger-based state change logging", func() {
		It("should not panic when logger is set instead of OnStateChange", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusGatewayTimeout)
			}))
			defer server.Close()

			rt := transport.NewCircuitBreakerTransport(server.Client().Transport, transport.CircuitBreakerConfig{
				Enabled:          true,
				Name:             "test-log",
				MaxRequests:      1,
				Interval:         60 * time.Second,
				Timeout:          30 * time.Second,
				FailureThreshold: 3,
				FailureRatio:     0.5,
				Logger:           logr.Discard(),
			})

			Expect(func() {
				for i := 0; i < 5; i++ {
					req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
					_, _ = rt.RoundTrip(req)
				}
			}).NotTo(Panic())

			cbt, ok := rt.(*transport.CircuitBreakerTransport)
			Expect(ok).To(BeTrue())
			Expect(cbt.State()).To(Equal(circuitbreaker.StateOpen))
		})
	})

	Describe("UT-CB-007: DefaultCircuitBreakerConfig returns sensible defaults", func() {
		It("should match gateway breaker pattern", func() {
			cfg := transport.DefaultCircuitBreakerConfig("llm")
			Expect(cfg.Enabled).To(BeFalse())
			Expect(cfg.Name).To(Equal("llm"))
			Expect(cfg.MaxRequests).To(Equal(uint32(3)))
			Expect(cfg.Interval).To(Equal(10 * time.Second))
			Expect(cfg.Timeout).To(Equal(30 * time.Second))
			Expect(cfg.FailureThreshold).To(Equal(uint32(10)))
			Expect(cfg.FailureRatio).To(Equal(0.5))
		})
	})

	Describe("UT-CB-008: 503 response body is still returned when circuit is closed", func() {
		It("should return the original response even on failure status", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()

			rt := transport.NewCircuitBreakerTransport(server.Client().Transport, transport.CircuitBreakerConfig{
				Enabled:          true,
				Name:             "test-body",
				MaxRequests:      1,
				Interval:         60 * time.Second,
				Timeout:          30 * time.Second,
				FailureThreshold: 100,
				FailureRatio:     0.99,
			})

			req, err := http.NewRequest(http.MethodGet, server.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			resp, err := rt.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))
			resp.Body.Close()
		})
	})
})
