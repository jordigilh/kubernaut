package datastorage_test

import (
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UT-DS-1048-P5: Retention Configuration", func() {
	Describe("UT-DS-1048-P5-020: RetentionConfig defaults", func() {
		It("should default interval to 24h when empty", func() {
			cfg := config.RetentionConfig{}
			Expect(cfg.GetInterval()).To(Equal(24 * time.Hour))
		})

		It("should default batchSize to 1000 when zero", func() {
			cfg := config.RetentionConfig{}
			Expect(cfg.GetBatchSize()).To(Equal(1000))
		})

		It("should default defaultDays to 2555 (ADR-034) when zero", func() {
			cfg := config.RetentionConfig{}
			Expect(cfg.GetDefaultDays()).To(Equal(2555))
		})

		It("should clamp defaultDays to 2555 when exceeding max", func() {
			cfg := config.RetentionConfig{DefaultDays: 9999}
			Expect(cfg.GetDefaultDays()).To(Equal(2555))
		})
	})

	Describe("UT-DS-1048-P5-021: RetentionConfig custom values", func() {
		It("should parse custom interval", func() {
			cfg := config.RetentionConfig{Interval: "12h"}
			Expect(cfg.GetInterval()).To(Equal(12 * time.Hour))
		})

		It("should accept custom batchSize", func() {
			cfg := config.RetentionConfig{BatchSize: 500}
			Expect(cfg.GetBatchSize()).To(Equal(500))
		})

		It("should accept custom defaultDays within range", func() {
			cfg := config.RetentionConfig{DefaultDays: 90}
			Expect(cfg.GetDefaultDays()).To(Equal(90))
		})
	})

	Describe("Retention config validation", func() {
		It("should reject invalid interval duration", func() {
			cfg := &config.Config{
				Server: config.ServerConfig{Port: 8080, Host: "0.0.0.0"},
				Database: config.DatabaseConfig{
					Host: "localhost", Port: 5432, Name: "test", User: "test",
					StatementTimeout: "30s", LockTimeout: "10s",
				},
				Redis: config.RedisConfig{Addr: "localhost:6379"},
				Retention: config.RetentionConfig{Interval: "not-a-duration"},
			}
			err := cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("retention interval"))
		})

		It("should accept valid retention config", func() {
			cfg := &config.Config{
				Server: config.ServerConfig{Port: 8080, Host: "0.0.0.0"},
				Database: config.DatabaseConfig{
					Host: "localhost", Port: 5432, Name: "test", User: "test",
					StatementTimeout: "30s", LockTimeout: "10s",
				},
				Redis: config.RedisConfig{Addr: "localhost:6379"},
				Retention: config.RetentionConfig{
					Enabled:     false,
					Interval:    "24h",
					BatchSize:   1000,
					DefaultDays: 2555,
				},
			}
			err := cfg.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
