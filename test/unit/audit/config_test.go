package audit

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	audit "github.com/jordigilh/kubernaut/pkg/audit"
)

var _ = Describe("audit.Config", func() {
	Describe("audit.DefaultConfig", func() {
		It("should return sensible defaults", func() {
			config := audit.DefaultConfig()

			Expect(config.BufferSize).To(Equal(10000))
			Expect(config.BatchSize).To(Equal(1000))
			Expect(config.FlushInterval).To(Equal(1 * time.Second))
			Expect(config.MaxRetries).To(Equal(3))
		})
	})

	Describe("audit.RecommendedConfig", func() {
		It("should return gateway config with 2x buffer", func() {
			config := audit.RecommendedConfig("gateway")

			Expect(config.BufferSize).To(Equal(30000)) // DD-AUDIT-004: MEDIUM tier (updated from 20000)
			Expect(config.BatchSize).To(Equal(1000))
			Expect(config.FlushInterval).To(Equal(1 * time.Second))
			Expect(config.MaxRetries).To(Equal(3))
		})

		It("should return ai-analysis config with 1.5x buffer", func() {
			config := audit.RecommendedConfig("ai-analysis")

			Expect(config.BufferSize).To(Equal(20000)) // DD-AUDIT-004: LOW tier (updated from 15000)
			Expect(config.BatchSize).To(Equal(1000))
			Expect(config.FlushInterval).To(Equal(1 * time.Second))
			Expect(config.MaxRetries).To(Equal(3))
		})

		It("should return default config for unknown service", func() {
			config := audit.RecommendedConfig("unknown-service")

			Expect(config.BufferSize).To(Equal(30000)) // DD-AUDIT-004: MEDIUM tier default (updated from 10000)
			Expect(config.BatchSize).To(Equal(1000))
			Expect(config.FlushInterval).To(Equal(1 * time.Second))
			Expect(config.MaxRetries).To(Equal(3))
		})
	})

	Describe("Validate", func() {
		var config audit.Config

		BeforeEach(func() {
			config = audit.DefaultConfig()
		})

		It("should validate a valid config", func() {
			err := config.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject negative buffer_size", func() {
			config.BufferSize = -1
			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("buffer_size must be positive"))
		})

		It("should reject zero buffer_size", func() {
			config.BufferSize = 0
			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("buffer_size must be positive"))
		})

		It("should reject negative batch_size", func() {
			config.BatchSize = -1
			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("batch_size must be positive"))
		})

		It("should reject zero batch_size", func() {
			config.BatchSize = 0
			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("batch_size must be positive"))
		})

		It("should reject batch_size > buffer_size", func() {
			config.BatchSize = 20000
			config.BufferSize = 10000
			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("batch_size"))
			Expect(err.Error()).To(ContainSubstring("buffer_size"))
		})

		It("should reject negative flush_interval", func() {
			config.FlushInterval = -1 * time.Second
			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("flush_interval must be positive"))
		})

		It("should reject zero flush_interval", func() {
			config.FlushInterval = 0
			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("flush_interval must be positive"))
		})

		It("should reject negative max_retries", func() {
			config.MaxRetries = -1
			err := config.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("max_retries must be non-negative"))
		})

		It("should allow zero max_retries", func() {
			config.MaxRetries = 0
			err := config.Validate()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
