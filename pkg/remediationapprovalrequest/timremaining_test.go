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

package remediationapprovalrequest

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestComputeTimeRemaining(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ComputeTimeRemaining Suite")
}

var _ = Describe("ComputeTimeRemaining", func() {
	DescribeTable("edge cases and format verification",
		func(requiredBy, now time.Time, expected string) {
			result := ComputeTimeRemaining(requiredBy, now)
			Expect(result).To(Equal(expected))
		},
		Entry("deadline exactly now (boundary)",
			time.Date(2025, 2, 22, 12, 0, 0, 0, time.UTC),
			time.Date(2025, 2, 22, 12, 0, 0, 0, time.UTC),
			"0s"),
		Entry("deadline 1 second away",
			time.Date(2025, 2, 22, 12, 0, 1, 0, time.UTC),
			time.Date(2025, 2, 22, 12, 0, 0, 0, time.UTC),
			"1s"),
		Entry("deadline 1 hour away",
			time.Date(2025, 2, 22, 13, 0, 0, 0, time.UTC),
			time.Date(2025, 2, 22, 12, 0, 0, 0, time.UTC),
			"1h0m0s"),
		Entry("deadline already passed (negative) returns 0s",
			time.Date(2025, 2, 22, 11, 0, 0, 0, time.UTC),
			time.Date(2025, 2, 22, 12, 0, 0, 0, time.UTC),
			"0s"),
		Entry("deadline 90 seconds away",
			time.Date(2025, 2, 22, 12, 1, 30, 0, time.UTC),
			time.Date(2025, 2, 22, 12, 0, 0, 0, time.UTC),
			"1m30s"),
		Entry("deadline 45 seconds away",
			time.Date(2025, 2, 22, 12, 0, 45, 0, time.UTC),
			time.Date(2025, 2, 22, 12, 0, 0, 0, time.UTC),
			"45s"),
	)

	It("uses Go time.Duration.String() format", func() {
		// Verify format: "1m30s", "45s", "0s" per Bug Fix 4 REFACTOR
		now := time.Date(2025, 2, 22, 12, 0, 0, 0, time.UTC)
		Expect(ComputeTimeRemaining(now.Add(90*time.Second), now)).To(Equal("1m30s"))
		Expect(ComputeTimeRemaining(now.Add(45*time.Second), now)).To(Equal("45s"))
		Expect(ComputeTimeRemaining(now, now)).To(Equal("0s"))
		Expect(ComputeTimeRemaining(now.Add(-1*time.Second), now)).To(Equal("0s"))
	})
})
