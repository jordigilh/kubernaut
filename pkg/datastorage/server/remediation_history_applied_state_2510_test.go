/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package server

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// BR-AUDIT-005 / DD-KA-2511; FedRAMP AU-3/SI-10; SOC 2 CC7.2.
var _ = Describe("#2510 applied-state classification", func() {
	DescribeTable("UT-DS-2510-003: distinguishes server-owned evidence states",
		func(current, pre, post, reason, expectedState, expectedChange string) {
			actual := classifyAppliedState(current, pre, post, reason)
			Expect(actual.appliedState).To(Equal(expectedState))
			Expect(actual.specChangeStatus).To(Equal(expectedChange))
		},
		Entry("confirmed post hash", "current", "before", "current", "Full", "confirmed", "post_hash_matches_current"),
		Entry("explicit no execution", "current", "before", "", "NoExecution", "not_confirmed", "no_execution"),
		Entry("missing current hash", "", "before", "after", "Full", "unknown", "hashes_unavailable"),
		Entry("pre hash still matches", "before", "before", "after", "Full", "not_confirmed", "pre_hash_matches_current"),
		Entry("current differs from both hashes", "current", "before", "after", "Full", "unknown", "hashes_differ_from_current"),
		Entry("pre and post contradict the current match", "same", "same", "same", "Full", "unknown", "hashes_contradictory"),
		Entry("post hash unavailable", "current", "before", "", "Full", "unknown", "hashes_unavailable"),
	)
})
