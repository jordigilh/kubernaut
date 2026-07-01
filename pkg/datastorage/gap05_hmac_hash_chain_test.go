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

package datastorage_test

import (
	"context"
	"database/sql"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// ========================================
// GAP-05 (Issue #1505): Keyed HMAC-SHA256 Hash Chain
// ========================================
//
// A plain (unkeyed) SHA256 hash chain can be recomputed by anyone with
// read/write access to the database: an attacker who tampers with a row can
// also recompute a self-consistent chain. These tests prove:
//
//  1. HMAC-SHA256 hashes differ from unkeyed SHA256 hashes for the same input.
//  2. HMAC hashes are keyed (different keys => different hashes; the key is
//     required to reproduce a given hash).
//  3. CalculateHashForVerification honors each event's own HashAlgorithm,
//     supporting a mixed-algorithm chain across an HMAC key rollout.
//  4. Verifying an hmac-sha256 event without a configured key fails loudly
//     rather than silently reporting a false "tampered" mismatch.
//  5. AuditEventsRepository.Create/CreateBatch stamp hash_algorithm correctly
//     and persist it via the INSERT statement.
// ========================================

var _ = Describe("GAP-05: Keyed HMAC-SHA256 Hash Chain", func() {
	var baseEvent *repository.AuditEvent

	BeforeEach(func() {
		baseEvent = &repository.AuditEvent{
			EventID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			EventTimestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EventType:      "test.gap05.hmac",
			EventCategory:  "test",
			EventAction:    "verify",
			EventOutcome:   "success",
			CorrelationID:  "gap05-hmac-test",
		}
	})

	Describe("CalculateHashForVerification", func() {
		It("computes the legacy unkeyed SHA256 hash when HashAlgorithm is sha256-unkeyed", func() {
			event := *baseEvent
			event.HashAlgorithm = repository.HashAlgorithmSHA256Unkeyed

			hash, err := repository.CalculateHashForVerification(nil, "", &event)
			Expect(err).ToNot(HaveOccurred())
			Expect(hash).To(HaveLen(64), "SHA256 hex digest must be 64 characters")
		})

		It("treats an empty HashAlgorithm as sha256-unkeyed for backward compatibility", func() {
			event := *baseEvent
			event.HashAlgorithm = ""

			unkeyedHash, err := repository.CalculateHashForVerification(nil, "", &event)
			Expect(err).ToNot(HaveOccurred())

			explicitEvent := *baseEvent
			explicitEvent.HashAlgorithm = repository.HashAlgorithmSHA256Unkeyed
			explicitHash, err := repository.CalculateHashForVerification(nil, "", &explicitEvent)
			Expect(err).ToNot(HaveOccurred())

			Expect(unkeyedHash).To(Equal(explicitHash),
				"legacy pre-GAP-05 events (empty HashAlgorithm) must verify identically to sha256-unkeyed")
		})

		It("computes a different hash for hmac-sha256 than for sha256-unkeyed given the same event", func() {
			key := []byte("test-hmac-key-0123456789abcdef")

			unkeyedEvent := *baseEvent
			unkeyedEvent.HashAlgorithm = repository.HashAlgorithmSHA256Unkeyed
			unkeyedHash, err := repository.CalculateHashForVerification(nil, "", &unkeyedEvent)
			Expect(err).ToNot(HaveOccurred())

			hmacEvent := *baseEvent
			hmacEvent.HashAlgorithm = repository.HashAlgorithmHMACSHA256
			hmacHash, err := repository.CalculateHashForVerification(key, "", &hmacEvent)
			Expect(err).ToNot(HaveOccurred())

			Expect(hmacHash).ToNot(Equal(unkeyedHash),
				"hmac-sha256 and sha256-unkeyed must produce different digests for identical event content")
			Expect(hmacHash).To(HaveLen(64))
		})

		It("requires the correct key to reproduce an hmac-sha256 hash (proves keyed-ness)", func() {
			event := *baseEvent
			event.HashAlgorithm = repository.HashAlgorithmHMACSHA256

			hashWithKeyA, err := repository.CalculateHashForVerification([]byte("key-a-0123456789abcdef01234567"), "", &event)
			Expect(err).ToNot(HaveOccurred())

			hashWithKeyB, err := repository.CalculateHashForVerification([]byte("key-b-0123456789abcdef01234567"), "", &event)
			Expect(err).ToNot(HaveOccurred())

			Expect(hashWithKeyA).ToNot(Equal(hashWithKeyB),
				"a different key must produce a different hash for the same event — an attacker without "+
					"the key cannot forge a matching hash after tampering")
		})

		It("returns an error when verifying an hmac-sha256 event without a configured key", func() {
			event := *baseEvent
			event.HashAlgorithm = repository.HashAlgorithmHMACSHA256

			_, err := repository.CalculateHashForVerification(nil, "", &event)
			Expect(err).To(HaveOccurred(),
				"missing key must fail loudly, not silently fall back to unkeyed SHA256 (which would "+
					"misleadingly report every hmac-sha256 event as tampered)")
			Expect(err.Error()).To(ContainSubstring("hmac-sha256"))
		})

		It("returns an error for an unrecognized hash_algorithm value", func() {
			event := *baseEvent
			event.HashAlgorithm = "md5-legacy-imaginary"

			_, err := repository.CalculateHashForVerification(nil, "", &event)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unrecognized"))
		})

		It("is deterministic: same key, previous hash, and event always produce the same hash", func() {
			key := []byte("deterministic-key-0123456789ab")
			event := *baseEvent
			event.HashAlgorithm = repository.HashAlgorithmHMACSHA256

			hash1, err := repository.CalculateHashForVerification(key, "prev-hash", &event)
			Expect(err).ToNot(HaveOccurred())
			hash2, err := repository.CalculateHashForVerification(key, "prev-hash", &event)
			Expect(err).ToNot(HaveOccurred())

			Expect(hash1).To(Equal(hash2))
		})
	})

	Describe("AuditEventsRepository write-time algorithm selection", func() {
		var (
			mockDB *sql.DB
			mock   sqlmock.Sqlmock
			logger logr.Logger
			ctx    context.Context
		)

		BeforeEach(func() {
			var err error
			mockDB, mock, err = sqlmock.New()
			Expect(err).ToNot(HaveOccurred())
			logger = kubelog.NewLogger(kubelog.DefaultOptions())
			ctx = context.Background()
		})

		AfterEach(func() {
			_ = mockDB.Close()
		})

		It("stamps hash_algorithm=sha256-unkeyed and persists it when no HMAC key is configured", func() {
			repo := repository.NewAuditEventsRepository(mockDB, logger)
			Expect(repo.HMACKey()).To(BeEmpty(), "no HMAC key configured by default")

			event := &repository.AuditEvent{
				EventType:     "test.gap05.create.unkeyed",
				EventCategory: "test",
				EventAction:   "verify",
				EventOutcome:  "success",
				CorrelationID: "gap05-create-unkeyed",
				EventData:     map[string]interface{}{},
			}

			mock.ExpectBegin()
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("SELECT event_hash").
				WillReturnRows(sqlmock.NewRows([]string{"event_hash"}))
			mock.ExpectQuery("INSERT INTO audit_events").
				WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), repository.HashAlgorithmSHA256Unkeyed,
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"event_timestamp"}).AddRow(time.Now()))
			mock.ExpectCommit()

			created, err := repo.Create(ctx, event)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.HashAlgorithm).To(Equal(repository.HashAlgorithmSHA256Unkeyed))
			Expect(mock.ExpectationsWereMet()).To(Succeed())
		})

		It("stamps hash_algorithm=hmac-sha256 and persists it when an HMAC key is configured", func() {
			key := []byte("write-time-hmac-key-0123456789ab")
			repo := repository.NewAuditEventsRepository(mockDB, logger, repository.WithHMACKey(key))
			Expect(repo.HMACKey()).To(Equal(key))

			event := &repository.AuditEvent{
				EventType:     "test.gap05.create.hmac",
				EventCategory: "test",
				EventAction:   "verify",
				EventOutcome:  "success",
				CorrelationID: "gap05-create-hmac",
				EventData:     map[string]interface{}{},
			}

			mock.ExpectBegin()
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("SELECT event_hash").
				WillReturnRows(sqlmock.NewRows([]string{"event_hash"}))
			mock.ExpectQuery("INSERT INTO audit_events").
				WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
					sqlmock.AnyArg(), sqlmock.AnyArg(), repository.HashAlgorithmHMACSHA256,
					sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"event_timestamp"}).AddRow(time.Now()))
			mock.ExpectCommit()

			created, err := repo.Create(ctx, event)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.HashAlgorithm).To(Equal(repository.HashAlgorithmHMACSHA256))

			expectedHash, err := repository.CalculateHashForVerification(key, "", created)
			Expect(err).ToNot(HaveOccurred())
			Expect(created.EventHash).To(Equal(expectedHash),
				"the persisted hash must be reproducible via CalculateHashForVerification given the same key")

			Expect(mock.ExpectationsWereMet()).To(Succeed())
		})
	})

	Describe("WithHMACKey option", func() {
		It("is a no-op for a nil/empty key, preserving the legacy unkeyed default", func() {
			mockDB, _, err := sqlmock.New()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = mockDB.Close() }()

			repo := repository.NewAuditEventsRepository(mockDB, kubelog.NewLogger(kubelog.DefaultOptions()), repository.WithHMACKey(nil))
			Expect(repo.HMACKey()).To(BeEmpty())
		})
	})
})
