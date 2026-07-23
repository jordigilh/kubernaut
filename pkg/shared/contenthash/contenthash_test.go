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

package contenthash_test

import (
	"github.com/google/uuid"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Relocated from pkg/datastorage/deterministic_uuid_test.go (#1661 Change 8a
// REFACTOR): DeterministicUUID moved from pkg/datastorage/uuid into this
// shared package so AuthWebhook can compute it locally too, without a
// parallel copy. Test IDs (UT-DS-548-*) preserved unchanged — same algorithm,
// same assertions, new home.
var _ = Describe("DeterministicUUID (#548, relocated by #1661 Change 8a)", func() {

	Context("UT-DS-548-001: produces a valid UUIDv5 from a SHA-256 content hash", func() {
		It("should return a parseable UUID with version 5", func() {
			contentHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
			result := contenthash.DeterministicUUID(contentHash)

			parsed, err := uuid.Parse(result)
			Expect(err).NotTo(HaveOccurred(), "result should be a valid UUID")
			Expect(parsed.Version()).To(Equal(uuid.Version(5)), "UUID version should be 5")
		})
	})

	Context("UT-DS-548-002: same content hash always yields same UUID (idempotent)", func() {
		hashes := []string{
			"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
			"0000000000000000000000000000000000000000000000000000000000000000",
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			"1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			"abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd",
			"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			"cafebabecafebabecafebabecafebabecafebabecafebabecafebabecafebabe",
			"0123456789012345678901234567890123456789012345678901234567890123",
			"fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
		}

		It("should return identical UUIDs for the same input across multiple calls", func() {
			for _, h := range hashes {
				first := contenthash.DeterministicUUID(h)
				second := contenthash.DeterministicUUID(h)
				Expect(first).To(Equal(second), "DeterministicUUID should be idempotent for hash %s", h)
			}
		})
	})

	Context("UT-DS-548-003: different content hashes yield different UUIDs", func() {
		DescribeTable("pairwise uniqueness",
			func(hash1, hash2 string) {
				uuid1 := contenthash.DeterministicUUID(hash1)
				uuid2 := contenthash.DeterministicUUID(hash2)
				Expect(uuid1).NotTo(Equal(uuid2), "different hashes must produce different UUIDs")
			},
			Entry("zeros vs ones",
				"0000000000000000000000000000000000000000000000000000000000000000",
				"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
			Entry("single char difference",
				"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b856"),
			Entry("completely different hashes",
				"1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
				"deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"),
		)
	})

	Context("UT-DS-548-004: UUID conforms to RFC 4122 v5 format", func() {
		It("should have version nibble=5 and variant=RFC4122", func() {
			contentHash := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
			result := contenthash.DeterministicUUID(contentHash)

			parsed, err := uuid.Parse(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(parsed.Version()).To(Equal(uuid.Version(5)))
			Expect(parsed.Variant()).To(Equal(uuid.RFC4122))
		})
	})

	Context("UT-DS-548-005: empty content hash produces a valid UUID", func() {
		It("should not panic and should return a valid UUIDv5", func() {
			Expect(func() {
				result := contenthash.DeterministicUUID("")
				parsed, err := uuid.Parse(result)
				Expect(err).NotTo(HaveOccurred())
				Expect(parsed.Version()).To(Equal(uuid.Version(5)))
			}).NotTo(Panic())
		})
	})

	Context("UT-AW-320-001: ComputeContentHash produces a SHA-256 hex digest", func() {
		It("should return a 64-char lowercase hex digest", func() {
			result := contenthash.ComputeContentHash(`{"spec":{"version":"1.0.0"}}`)
			Expect(result).To(MatchRegexp("^[0-9a-f]{64}$"))
		})

		It("should be deterministic for identical content", func() {
			content := `{"spec":{"version":"1.0.0"}}`
			Expect(contenthash.ComputeContentHash(content)).To(Equal(contenthash.ComputeContentHash(content)))
		})
	})
})
