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
package uuid_test

import (
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/uuid"
)

var uuidV5Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var _ = Describe("DeterministicUUID", func() {

	Describe("UT-UUID-001: Same workflow name produces same UUID v5", func() {
		It("should return a valid UUID v5 that is deterministic", func() {
			id1 := uuid.DeterministicUUID("oom-recovery")
			id2 := uuid.DeterministicUUID("oom-recovery")

			Expect(id1).To(MatchRegexp(uuidV5Regex.String()), "must be a valid UUID v5")
			Expect(id1).To(Equal(id2), "same input must produce same output")
		})
	})

	Describe("UT-UUID-002: Different workflow names produce different UUIDs", func() {
		It("should produce distinct UUIDs for distinct names", func() {
			id1 := uuid.DeterministicUUID("oom-recovery")
			id2 := uuid.DeterministicUUID("crashloop-fix")

			Expect(id1).NotTo(Equal(id2))
			Expect(id1).To(MatchRegexp(uuidV5Regex.String()))
			Expect(id2).To(MatchRegexp(uuidV5Regex.String()))
		})
	})

	Describe("UT-UUID-003: Consistent across 100 calls", func() {
		It("should return exactly one unique UUID for the same input", func() {
			results := make(map[string]bool)
			for i := 0; i < 100; i++ {
				id := uuid.DeterministicUUID("oom-recovery")
				results[id] = true
			}
			Expect(results).To(HaveLen(1), "expected exactly one unique UUID across 100 calls")
		})
	})

	Describe("UT-UUID-004: Empty workflow name produces valid UUID", func() {
		It("should return a valid UUID v5 even for empty input", func() {
			id := uuid.DeterministicUUID("")
			Expect(id).To(MatchRegexp(uuidV5Regex.String()), "empty name must still produce a valid UUID v5")
		})

		It("should produce a different UUID from any non-empty name", func() {
			empty := uuid.DeterministicUUID("")
			nonEmpty := uuid.DeterministicUUID("anything")
			Expect(empty).NotTo(Equal(nonEmpty))
		})
	})
})
