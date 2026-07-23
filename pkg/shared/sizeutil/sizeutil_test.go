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

package sizeutil_test

import (
	"math"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/sizeutil"
)

func TestSizeutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Shared Sizeutil Utility Suite")
}

// Test Tier: UNIT ONLY
// Rationale: Pure computational utility with zero external dependencies
//
// Business Requirements Enabled (not tested directly by this utility):
// - Issue #1684: CodeQL go/allocation-size-overflow remediation. Callers use
//   SafeCap instead of raw "a+b" capacity expressions so the overflow check
//   is explicit and visible to static analysis, not just practically safe.
var _ = Describe("SafeCap", func() {
	Describe("normal, non-overflowing inputs", func() {
		It("UT-SHARED-1684-001: sums two positive sizes", func() {
			Expect(sizeutil.SafeCap(3, 4)).To(Equal(7))
		})

		It("UT-SHARED-1684-002: sums a single size", func() {
			Expect(sizeutil.SafeCap(5)).To(Equal(5))
		})

		It("UT-SHARED-1684-003: sums more than two sizes", func() {
			Expect(sizeutil.SafeCap(1, 2, 3, 4)).To(Equal(10))
		})

		It("UT-SHARED-1684-004: returns 0 for no arguments", func() {
			Expect(sizeutil.SafeCap()).To(Equal(0))
		})

		It("UT-SHARED-1684-005: treats zero-value sizes as a no-op in the sum", func() {
			Expect(sizeutil.SafeCap(0, 7, 0)).To(Equal(7))
		})
	})

	Describe("overflow and invalid-input guards", func() {
		It("UT-SHARED-1684-006: returns 0 when a single size is negative", func() {
			Expect(sizeutil.SafeCap(-1)).To(Equal(0))
		})

		It("UT-SHARED-1684-007: returns 0 when any size in a longer list is negative", func() {
			Expect(sizeutil.SafeCap(3, -2, 4)).To(Equal(0))
		})

		It("UT-SHARED-1684-008: returns 0 when the running sum would overflow int", func() {
			Expect(sizeutil.SafeCap(math.MaxInt-1, 2)).To(Equal(0))
		})

		It("UT-SHARED-1684-009: returns the exact sum when it lands exactly at MaxInt", func() {
			Expect(sizeutil.SafeCap(math.MaxInt-1, 1)).To(Equal(math.MaxInt))
		})

		It("UT-SHARED-1684-010: does not overflow across more than two overflow-adjacent terms", func() {
			Expect(sizeutil.SafeCap(math.MaxInt/2, math.MaxInt/2, math.MaxInt/2)).To(Equal(0))
		})
	})
})
