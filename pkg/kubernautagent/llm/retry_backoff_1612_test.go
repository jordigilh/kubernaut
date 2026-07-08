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

package llm_test

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
)

// Issue #1612: chatOrStream (the streaming LLM call path) has no retry
// logic. This test suite RED-proves a shared generic retry helper,
// llm.RetryWithBackoff, extracted so both ChatWithParams (#967, non-
// streaming) and the new streaming path (#1612) share one backoff-loop
// implementation instead of duplicating it.
var _ = Describe("llm.RetryWithBackoff — #1612", func() {

	Describe("UT-KA-1612-001: SafeToRetry=false stops immediately even with attempts remaining", func() {
		It("does not call attempt again after a non-retryable failure", func() {
			var calls atomic.Int64
			wantErr := errors.New("permanent failure")

			_, err := llm.RetryWithBackoff(context.Background(), 5, *fastBackoff,
				func(int) llm.AttemptResult[string] {
					calls.Add(1)
					return llm.AttemptResult[string]{Err: wantErr, SafeToRetry: false}
				})

			Expect(err).To(MatchError(wantErr))
			Expect(calls.Load()).To(Equal(int64(1)), "must not retry once SafeToRetry is false")
		})
	})

	Describe("UT-KA-1612-002: ctx cancelled mid-backoff-sleep returns ctx.Err()", func() {
		It("aborts the sleep and does not make a further attempt", func() {
			var calls atomic.Int64
			ctx, cancel := context.WithCancel(context.Background())

			slowBackoff := backoff.Config{
				BasePeriod: 50 * time.Millisecond, MaxPeriod: 200 * time.Millisecond,
				Multiplier: 2.0, JitterPercent: 0,
			}

			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()

			_, err := llm.RetryWithBackoff(ctx, 5, slowBackoff,
				func(int) llm.AttemptResult[string] {
					calls.Add(1)
					return llm.AttemptResult[string]{Err: errors.New("transient"), SafeToRetry: true}
				})

			Expect(err).To(MatchError(context.Canceled))
			Expect(calls.Load()).To(Equal(int64(1)), "cancellation during backoff sleep must pre-empt the next attempt")
		})
	})

	Describe("UT-KA-1612-003: attempts exhausted returns the last error", func() {
		It("calls attempt exactly maxAttempts times and returns the final error", func() {
			var calls atomic.Int64
			lastErr := errors.New("final attempt error")

			_, err := llm.RetryWithBackoff(context.Background(), 3, *fastBackoff,
				func(attempt int) llm.AttemptResult[string] {
					n := calls.Add(1)
					if n == 3 {
						return llm.AttemptResult[string]{Err: lastErr, SafeToRetry: true}
					}
					return llm.AttemptResult[string]{Err: errors.New("earlier error"), SafeToRetry: true}
				})

			Expect(err).To(MatchError(lastErr))
			Expect(calls.Load()).To(Equal(int64(3)))
		})
	})

	Describe("success path", func() {
		It("returns the value and nil error on the first successful attempt, without retrying", func() {
			var calls atomic.Int64

			val, err := llm.RetryWithBackoff(context.Background(), 5, *fastBackoff,
				func(int) llm.AttemptResult[string] {
					calls.Add(1)
					return llm.AttemptResult[string]{Value: "ok", Err: nil}
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("ok"))
			Expect(calls.Load()).To(Equal(int64(1)))
		})

		It("succeeds after a retryable failure on an earlier attempt", func() {
			var calls atomic.Int64

			val, err := llm.RetryWithBackoff(context.Background(), 5, *fastBackoff,
				func(int) llm.AttemptResult[string] {
					n := calls.Add(1)
					if n < 2 {
						return llm.AttemptResult[string]{Err: errors.New("transient"), SafeToRetry: true}
					}
					return llm.AttemptResult[string]{Value: "recovered"}
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("recovered"))
			Expect(calls.Load()).To(Equal(int64(2)))
		})
	})
})
