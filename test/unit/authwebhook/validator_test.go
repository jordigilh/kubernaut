package authwebhook_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/authwebhook"
)

var _ = Describe("Validator", func() {
	Describe("ValidateReason", func() {
		Context("when reason has sufficient words", func() {
			It("should pass validation with exactly minimum words", func() {
				reason := "one two three four five six seven eight nine ten"
				err := authwebhook.ValidateReason(reason, 10)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass validation with more than minimum words", func() {
				reason := "Investigation complete after root cause analysis confirmed memory leak in payment service pod"
				err := authwebhook.ValidateReason(reason, 10)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when reason is too short", func() {
			It("should return error with word count", func() {
				reason := "Fixed it now"
				err := authwebhook.ValidateReason(reason, 10)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("minimum 10 words required"))
				Expect(err.Error()).To(ContainSubstring("got 3"))
			})

			It("should return error for single word", func() {
				reason := "Fixed"
				err := authwebhook.ValidateReason(reason, 10)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when reason is empty", func() {
			It("should return error", func() {
				err := authwebhook.ValidateReason("", 10)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("reason cannot be empty"))
			})
		})

		Context("when reason is only whitespace", func() {
			It("should return error", func() {
				err := authwebhook.ValidateReason("   ", 10)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("reason cannot be empty"))
			})
		})

		Context("when minimum words is invalid", func() {
			It("should return error for negative minimum", func() {
				reason := "valid reason text"
				err := authwebhook.ValidateReason(reason, -1)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("minimum words must be positive"))
			})

			It("should return error for zero minimum", func() {
				reason := "valid reason text"
				err := authwebhook.ValidateReason(reason, 0)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("minimum words must be positive"))
			})
		})
	})

	Describe("ValidateTimestamp", func() {
		Context("when timestamp is current", func() {
			It("should pass validation for recent timestamp", func() {
				ts := time.Now().Add(-30 * time.Second)
				err := authwebhook.ValidateTimestamp(ts)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should pass validation for timestamp at boundary (5 minutes)", func() {
				ts := time.Now().Add(-4*time.Minute - 59*time.Second)
				err := authwebhook.ValidateTimestamp(ts)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when timestamp is in the future", func() {
			It("should return error for future timestamp", func() {
				ts := time.Now().Add(1 * time.Hour)
				err := authwebhook.ValidateTimestamp(ts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timestamp cannot be in the future"))
			})

			It("should return error even for slight future timestamp", func() {
				ts := time.Now().Add(1 * time.Second)
				err := authwebhook.ValidateTimestamp(ts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timestamp cannot be in the future"))
			})
		})

		Context("when timestamp is too old", func() {
			It("should return error for timestamp older than 5 minutes", func() {
				ts := time.Now().Add(-10 * time.Minute)
				err := authwebhook.ValidateTimestamp(ts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timestamp too old"))
			})

			It("should return error for very old timestamp", func() {
				ts := time.Now().Add(-24 * time.Hour)
				err := authwebhook.ValidateTimestamp(ts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timestamp too old"))
			})
		})

		Context("when timestamp is zero", func() {
			It("should return error", func() {
				ts := time.Time{}
				err := authwebhook.ValidateTimestamp(ts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("timestamp cannot be zero"))
			})
		})
	})
})

