/*
Copyright 2025 Jordi Gil.

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

package datastorage

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository/txretry"
)

var _ = Describe("Serializable Transaction Retry [BR-WORKFLOW-007]", func() {

	ctx := context.Background()

	Describe("WithSerializableRetry", func() {

		It("UT-AT-RETRY-001: succeeds on first attempt without retrying", func() {
			var calls int32
			err := txretry.WithSerializableRetry(ctx, 3, func() error {
				atomic.AddInt32(&calls, 1)
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(atomic.LoadInt32(&calls)).To(Equal(int32(1)))
		})

		It("UT-AT-RETRY-002: retries on SQLSTATE 40001 and succeeds", func() {
			var calls int32
			err := txretry.WithSerializableRetry(ctx, 3, func() error {
				n := atomic.AddInt32(&calls, 1)
				if n <= 2 {
					return &pgconn.PgError{Code: "40001", Message: "could not serialize access"}
				}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(atomic.LoadInt32(&calls)).To(Equal(int32(3)))
		})

		It("UT-AT-RETRY-003: gives up after maxRetries exhausted", func() {
			var calls int32
			err := txretry.WithSerializableRetry(ctx, 3, func() error {
				atomic.AddInt32(&calls, 1)
				return &pgconn.PgError{Code: "40001", Message: "could not serialize access"}
			})
			Expect(err).To(HaveOccurred())
			var pgErr *pgconn.PgError
			Expect(errors.As(err, &pgErr)).To(BeTrue())
			Expect(pgErr.Code).To(Equal("40001"))
			// 1 initial + 3 retries = 4 total calls
			Expect(atomic.LoadInt32(&calls)).To(Equal(int32(4)))
		})

		It("UT-AT-RETRY-004: does not retry on non-40001 errors", func() {
			var calls int32
			nonRetryableErr := errors.New("some other database error")
			err := txretry.WithSerializableRetry(ctx, 3, func() error {
				atomic.AddInt32(&calls, 1)
				return nonRetryableErr
			})
			Expect(err).To(MatchError(nonRetryableErr))
			Expect(atomic.LoadInt32(&calls)).To(Equal(int32(1)))
		})

		It("UT-AT-RETRY-005: does not retry on non-40001 PgError codes", func() {
			var calls int32
			err := txretry.WithSerializableRetry(ctx, 3, func() error {
				atomic.AddInt32(&calls, 1)
				return &pgconn.PgError{Code: "23505", Message: "unique violation"}
			})
			Expect(err).To(HaveOccurred())
			var pgErr *pgconn.PgError
			Expect(errors.As(err, &pgErr)).To(BeTrue())
			Expect(pgErr.Code).To(Equal("23505"))
			Expect(atomic.LoadInt32(&calls)).To(Equal(int32(1)))
		})

		It("UT-AT-RETRY-006: respects context cancellation between retries", func() {
			cancelCtx, cancel := context.WithCancel(ctx)
			var calls int32
			err := txretry.WithSerializableRetry(cancelCtx, 3, func() error {
				n := atomic.AddInt32(&calls, 1)
				if n == 1 {
					cancel()
					return &pgconn.PgError{Code: "40001", Message: "could not serialize access"}
				}
				return nil
			})
			Expect(err).To(HaveOccurred())
			Expect(atomic.LoadInt32(&calls)).To(Equal(int32(1)))
		})

		It("UT-AT-RETRY-007: retries wrapped 40001 errors", func() {
			var calls int32
			err := txretry.WithSerializableRetry(ctx, 3, func() error {
				n := atomic.AddInt32(&calls, 1)
				if n == 1 {
					pgErr := &pgconn.PgError{Code: "40001", Message: "could not serialize access"}
					return errors.Join(errors.New("commit disable"), pgErr)
				}
				return nil
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(atomic.LoadInt32(&calls)).To(Equal(int32(2)))
		})
	})
})
