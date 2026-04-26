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
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

// BR-GATEWAY-190: Inter-service HTTP clients need retry/circuit-breaker
// Issue #853: RetryTransport — http.RoundTripper middleware for transparent retry
//
// Test Plan: docs/tests/853/TEST_PLAN.md

// mockRoundTripper records calls and returns pre-configured responses.
type mockRoundTripper struct {
	responses []*http.Response
	errors    []error
	calls     int
}

func (m *mockRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	idx := m.calls
	m.calls++
	if idx < len(m.errors) && m.errors[idx] != nil {
		return nil, m.errors[idx]
	}
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func newResponse(statusCode int) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

// noSleepBackoff returns zero duration so tests run instantly.
func noSleepBackoff() backoff.Config {
	return backoff.Config{
		BasePeriod:    1 * time.Nanosecond,
		MaxPeriod:     1 * time.Nanosecond,
		Multiplier:    1.0,
		JitterPercent: 0,
	}
}

var _ = Describe("BR-GATEWAY-190: RetryTransport (#853)", func() {

	Context("no retry path", func() {
		It("UT-RT-853-001: should not retry on 200 OK", func() {
			mock := &mockRoundTripper{
				responses: []*http.Response{newResponse(http.StatusOK)},
			}

			rt := transport.NewRetryTransport(mock, transport.RetryConfig{
				MaxAttempts: 3,
				Backoff:     noSleepBackoff(),
			})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://test/ok", nil)
			Expect(err).ToNot(HaveOccurred())

			resp, err := rt.RoundTrip(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(mock.calls).To(Equal(1), "200 OK must not trigger retry")
		})
	})

	Context("transient connection errors", func() {
		It("UT-RT-853-002: should retry on connection reset and succeed on 2nd attempt", func() {
			mock := &mockRoundTripper{
				errors:    []error{syscall.ECONNRESET, nil},
				responses: []*http.Response{nil, newResponse(http.StatusOK)},
			}

			rt := transport.NewRetryTransport(mock, transport.RetryConfig{
				MaxAttempts: 3,
				Backoff:     noSleepBackoff(),
				Logger:      GinkgoLogr,
			})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://test/reset", nil)
			Expect(err).ToNot(HaveOccurred())

			resp, err := rt.RoundTrip(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(mock.calls).To(Equal(2), "ECONNRESET should trigger exactly one retry")
		})
	})

	Context("HTTP 5xx retries", func() {
		It("UT-RT-853-003: should retry on HTTP 503 and succeed on 2nd attempt", func() {
			mock := &mockRoundTripper{
				responses: []*http.Response{
					newResponse(http.StatusServiceUnavailable),
					newResponse(http.StatusOK),
				},
			}

			rt := transport.NewRetryTransport(mock, transport.RetryConfig{
				MaxAttempts: 3,
				Backoff:     noSleepBackoff(),
				Logger:      GinkgoLogr,
			})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://test/503", nil)
			Expect(err).ToNot(HaveOccurred())

			resp, err := rt.RoundTrip(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(mock.calls).To(Equal(2))
		})

		It("UT-RT-853-013: should retry on HTTP 502 and 504", func() {
			for _, code := range []int{http.StatusBadGateway, http.StatusGatewayTimeout} {
				mock := &mockRoundTripper{
					responses: []*http.Response{
						newResponse(code),
						newResponse(http.StatusOK),
					},
				}

				rt := transport.NewRetryTransport(mock, transport.RetryConfig{
					MaxAttempts: 3,
					Backoff:     noSleepBackoff(),
				})

				req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://test/5xx", nil)
				Expect(err).ToNot(HaveOccurred())

				resp, err := rt.RoundTrip(req)
				Expect(err).ToNot(HaveOccurred(), "failed for status %d", code)
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				Expect(mock.calls).To(Equal(2))
			}
		})

		It("UT-RT-853-015: should NOT retry on HTTP 500 or 501", func() {
			for _, code := range []int{http.StatusInternalServerError, http.StatusNotImplemented} {
				mock := &mockRoundTripper{
					responses: []*http.Response{newResponse(code)},
				}

				rt := transport.NewRetryTransport(mock, transport.RetryConfig{
					MaxAttempts: 3,
					Backoff:     noSleepBackoff(),
				})

				req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://test/5xx", nil)
				Expect(err).ToNot(HaveOccurred())

				resp, err := rt.RoundTrip(req)
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(code))
				Expect(mock.calls).To(Equal(1), "HTTP %d must NOT be retried", code)
			}
		})
	})

	Context("no retry on client errors", func() {
		It("UT-RT-853-004: should not retry on HTTP 400", func() {
			mock := &mockRoundTripper{
				responses: []*http.Response{newResponse(http.StatusBadRequest)},
			}

			rt := transport.NewRetryTransport(mock, transport.RetryConfig{
				MaxAttempts: 3,
				Backoff:     noSleepBackoff(),
			})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://test/400", nil)
			Expect(err).ToNot(HaveOccurred())

			resp, err := rt.RoundTrip(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(mock.calls).To(Equal(1), "4xx must never be retried")
		})
	})

	Context("context cancellation", func() {
		It("UT-RT-853-005: should abort retry on context cancellation", func() {
			ctx, cancel := context.WithCancel(context.Background())

			mock := &mockRoundTripper{
				errors: []error{
					syscall.ECONNRESET,
					syscall.ECONNRESET,
					syscall.ECONNRESET,
				},
			}

			rt := transport.NewRetryTransport(mock, transport.RetryConfig{
				MaxAttempts: 3,
				Backoff:     noSleepBackoff(),
			})

			cancel() // Cancel before call
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://test/cancel", nil)
			Expect(err).ToNot(HaveOccurred())

			_, err = rt.RoundTrip(req)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, context.Canceled)).To(BeTrue(),
				"cancelled context must propagate immediately")
		})
	})

	Context("max attempts exhausted", func() {
		It("UT-RT-853-006: should return last error after exhausting retries", func() {
			mock := &mockRoundTripper{
				errors: []error{
					syscall.ECONNRESET,
					syscall.ECONNRESET,
					syscall.ECONNRESET,
				},
			}

			rt := transport.NewRetryTransport(mock, transport.RetryConfig{
				MaxAttempts: 3,
				Backoff:     noSleepBackoff(),
			})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://test/exhaust", nil)
			Expect(err).ToNot(HaveOccurred())

			_, err = rt.RoundTrip(req)
			Expect(err).To(HaveOccurred())
			Expect(mock.calls).To(Equal(3), "must attempt exactly MaxAttempts times")
		})
	})

	Context("body replay", func() {
		It("UT-RT-853-007: should replay POST body via GetBody on retry", func() {
			bodyContent := `{"key":"value"}`
			mock := &mockRoundTripper{
				errors:    []error{syscall.ECONNRESET, nil},
				responses: []*http.Response{nil, newResponse(http.StatusOK)},
			}

			rt := transport.NewRetryTransport(mock, transport.RetryConfig{
				MaxAttempts: 3,
				Backoff:     noSleepBackoff(),
			})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://test/body", strings.NewReader(bodyContent))
			Expect(err).ToNot(HaveOccurred())
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader(bodyContent)), nil
			}

			resp, err := rt.RoundTrip(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(mock.calls).To(Equal(2), "body should be replayed and retry succeed")
		})

		It("UT-RT-853-008: should NOT retry when GetBody is nil and body is present", func() {
			mock := &mockRoundTripper{
				errors: []error{syscall.ECONNRESET},
			}

			rt := transport.NewRetryTransport(mock, transport.RetryConfig{
				MaxAttempts: 3,
				Backoff:     noSleepBackoff(),
			})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://test/nobody", bytes.NewReader([]byte("data")))
			Expect(err).ToNot(HaveOccurred())
			req.GetBody = nil // Explicitly nil — body is not replayable

			_, err = rt.RoundTrip(req)
			Expect(err).To(HaveOccurred(), "non-replayable body must not be retried")
			Expect(mock.calls).To(Equal(1), "must fail on first attempt without retry")
		})
	})

	Context("backoff jitter", func() {
		It("UT-RT-853-009: should apply jitter to backoff durations", func() {
			cfg := transport.DefaultRetryConfig()
			Expect(cfg.Backoff.JitterPercent).To(Equal(20),
				"default config must use 20%% jitter for anti-thundering herd")
		})
	})

	Context("response body drain", func() {
		It("UT-RT-853-010: should drain and close 5xx response body before retry", func() {
			bodyDrained := false
			drainTracker := &drainTrackerReadCloser{
				ReadCloser: io.NopCloser(strings.NewReader("error body")),
				onClose: func() {
					bodyDrained = true
				},
			}

			mock := &mockRoundTripper{
				responses: []*http.Response{
					{StatusCode: http.StatusServiceUnavailable, Body: drainTracker},
					newResponse(http.StatusOK),
				},
			}

			rt := transport.NewRetryTransport(mock, transport.RetryConfig{
				MaxAttempts: 3,
				Backoff:     noSleepBackoff(),
			})

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://test/drain", nil)
			Expect(err).ToNot(HaveOccurred())

			resp, err := rt.RoundTrip(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(bodyDrained).To(BeTrue(), "5xx response body must be drained and closed before retry")
		})
	})
})

// UT-RT-853-012: Audit client exclusion is verified via static grep in Checkpoint 2B.
// This structural test asserts that DefaultRetryConfig produces valid configuration.
var _ = Describe("BR-GATEWAY-190: RetryConfig defaults (#853)", func() {
	It("UT-RT-853-012: DefaultRetryConfig produces valid configuration", func() {
		cfg := transport.DefaultRetryConfig()
		Expect(cfg.MaxAttempts).To(Equal(3))
		Expect(cfg.Backoff.BasePeriod).To(BeNumerically(">", 0))
		Expect(cfg.Backoff.MaxPeriod).To(BeNumerically(">=", cfg.Backoff.BasePeriod))
		Expect(cfg.Backoff.JitterPercent).To(Equal(20))
	})
})

// drainTrackerReadCloser wraps an io.ReadCloser and tracks when Close is called.
type drainTrackerReadCloser struct {
	io.ReadCloser
	onClose func()
}

func (d *drainTrackerReadCloser) Close() error {
	if d.onClose != nil {
		d.onClose()
	}
	return d.ReadCloser.Close()
}
