package server

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// UT-DS-2584-001 / BR-AUDIT-005: API Frontend audit lag must remain
// distinguishable from the bounded "other" bucket.
var _ = Describe("UT-DS-2584-001: apifrontend audit category normalization", func() {
	It("preserves the registered apifrontend category", func() {
		Expect(normalizeEventCategory("apifrontend")).To(Equal("apifrontend"))
	})

	It("still bounds unknown categories to other", func() {
		Expect(normalizeEventCategory("untrusted-user-category")).To(Equal("other"))
	})

	It("does not widen the metric label with a Console event type", func() {
		Expect(normalizeEventCategory("apifrontend.console.policy_scope_read")).To(Equal("other"))
	})
})
