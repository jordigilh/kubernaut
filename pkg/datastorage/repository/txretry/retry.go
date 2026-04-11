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

// Package txretry provides retry logic for PostgreSQL retryable transaction errors.
//
// PostgreSQL may abort transactions with two retryable error classes:
//   - SQLSTATE 40001 (serialization_failure): concurrent read/write dependency
//     conflicts under sql.LevelSerializable.
//   - SQLSTATE 40P01 (deadlock_detected): lock-ordering deadlock detected by
//     the PostgreSQL deadlock detector.
//
// Both cases are safe to retry because PostgreSQL guarantees that at least one
// of the deadlocked transactions is rolled back completely.
//
// Issue #667 / BR-STORAGE-041: Extended to cover both 40001 and 40P01.
package txretry

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// WithSerializableRetry executes fn and retries up to maxRetries times when
// PostgreSQL returns a retryable error: SQLSTATE 40001 (serialization_failure)
// or SQLSTATE 40P01 (deadlock_detected). Non-retryable errors are returned
// immediately. The context is checked between retries to allow cancellation.
func WithSerializableRetry(ctx context.Context, maxRetries int, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if !isRetryablePostgresError(lastErr) {
			return lastErr
		}
		if attempt < maxRetries {
			if err := ctx.Err(); err != nil {
				return lastErr
			}
			time.Sleep(backoff(attempt))
		}
	}
	return lastErr
}

// isRetryablePostgresError returns true for PostgreSQL errors that are safe to retry:
// 40001 (serialization_failure) and 40P01 (deadlock_detected).
func isRetryablePostgresError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40001" || pgErr.Code == "40P01")
}

func backoff(attempt int) time.Duration {
	// 1ms, 2ms, 4ms — short jitter-free backoff suitable for serialization retries
	d := time.Millisecond << uint(attempt)
	if d > 50*time.Millisecond {
		d = 50 * time.Millisecond
	}
	return d
}
