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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
)

// ========================================
// Config.Validate() production security + input validation gap closure
// (GO-ANTIPATTERN-AUDIT-2026-07-01, Phase 4.0 — coverage gate ahead of
// decomposing Validate() into per-section validators).
//
// Every scenario asserts a business-level, operator-visible outcome tied to
// a specific FedRAMP control objective, not just branch execution:
//   - SC-8  (Transmission Confidentiality): DB/Redis traffic can never
//     silently run unencrypted in production.
//   - AC-4  (Information Flow Enforcement): cross-origin API access can
//     never be silently opened to any origin in production.
//   - SI-10 (Information Input Validation): malformed durations/sizes fail
//     fast at startup instead of silently degrading behavior at runtime.
// ========================================

// validSecureConfig returns a minimal Config that satisfies every SC-8/AC-4/
// SI-10 guard in Validate() (production environment, TLS everywhere,
// explicit CORS origin). Each test below flips exactly one field so the
// resulting failure (or, for negative controls, success) is attributable to
// that single change.
func validSecureConfig() config.Config {
	return config.Config{
		Environment: "production",
		Server: config.ServerConfig{
			Port:                     8080,
			ReadTimeout:              "30s",
			WriteTimeout:             "30s",
			ShutdownTimeout:          "60s",
			EndpointPropagationDelay: "5s",
			MaxBodySize:              "5242880",
			CORSAllowedOrigins:       []string{"https://app.example.com"},
		},
		Database: config.DatabaseConfig{
			Host:            "db.example.com",
			Port:            5432,
			Name:            "datastorage",
			User:            "ds_user",
			SSLMode:         "verify-full",
			MaxOpenConns:    25,
			ConnMaxLifetime: "5m",
			ConnMaxIdleTime: "10m",
		},
		Redis: config.RedisConfig{
			Addr: "redis.example.com:6379",
			TLS: config.RedisTLSConfig{
				Enabled: true,
				CAFile:  "/etc/redis-ca/ca.crt",
			},
		},
		Logging: config.LoggingConfig{Level: "info"},
	}
}

var _ = Describe("Config.Validate() — production security enforcement and input validation", func() {

	Describe("SC-8: Transmission Confidentiality", func() {
		It("UT-DS-CFG-001: rejects sslMode=disable in production, preventing unencrypted DB traffic from silently starting", func() {
			cfg := validSecureConfig()
			cfg.Database.SSLMode = "disable"

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sslMode"))
			Expect(err.Error()).To(ContainSubstring("production"))
		})

		It("UT-DS-CFG-002: rejects an empty sslMode in production (same guard as explicit disable)", func() {
			cfg := validSecureConfig()
			cfg.Database.SSLMode = ""

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sslMode"))
		})

		It("UT-DS-CFG-003: rejects Redis TLS disabled in production, preventing unencrypted cache/session traffic from silently starting", func() {
			cfg := validSecureConfig()
			cfg.Redis.TLS.Enabled = false

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("redis"))
			Expect(err.Error()).To(ContainSubstring("production"))
		})

		It("UT-DS-CFG-004: rejects Redis TLS enabled without a caFile, even outside production — a half-configured TLS setup is never silently accepted", func() {
			cfg := validSecureConfig()
			cfg.Environment = "" // non-production: proves this guard is unconditional, not prod-gated
			cfg.Redis.TLS.CAFile = ""

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("caFile"))
		})
	})

	Describe("AC-4: Information Flow Enforcement", func() {
		It("UT-DS-CFG-005: rejects CORS wildcard origin in production, preventing the API from silently accepting cross-origin requests from any origin", func() {
			cfg := validSecureConfig()
			cfg.Server.CORSAllowedOrigins = []string{"*"}

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("CORS"))
			Expect(err.Error()).To(ContainSubstring("production"))
		})
	})

	Describe("SC-8/AC-4 negative controls — guards are production-gated, not blanket-restrictive", func() {
		It("UT-DS-CFG-006: allows sslMode=disable outside production, preserving dev/staging usability", func() {
			cfg := validSecureConfig()
			cfg.Environment = ""
			cfg.Database.SSLMode = "disable"

			Expect(cfg.Validate()).ToNot(HaveOccurred())
		})

		It("UT-DS-CFG-007: allows Redis TLS disabled outside production, preserving dev/staging usability", func() {
			cfg := validSecureConfig()
			cfg.Environment = ""
			cfg.Redis.TLS.Enabled = false
			cfg.Redis.TLS.CAFile = ""

			Expect(cfg.Validate()).ToNot(HaveOccurred())
		})

		It("UT-DS-CFG-008: allows CORS wildcard origin outside production, preserving dev/staging usability", func() {
			cfg := validSecureConfig()
			cfg.Environment = ""
			cfg.Server.CORSAllowedOrigins = []string{"*"}

			Expect(cfg.Validate()).ToNot(HaveOccurred())
		})
	})

	Describe("SI-10: Information Input Validation — malformed durations fail fast at startup", func() {
		It("UT-DS-CFG-009: rejects an invalid server readTimeout instead of the server starting with an unparseable timeout", func() {
			cfg := validSecureConfig()
			cfg.Server.ReadTimeout = "not-a-duration"

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("readTimeout"))
		})

		It("UT-DS-CFG-010: rejects an invalid server writeTimeout instead of the server starting with an unparseable timeout", func() {
			cfg := validSecureConfig()
			cfg.Server.WriteTimeout = "not-a-duration"

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("writeTimeout"))
		})

		It("UT-DS-CFG-011: rejects an invalid database connMaxLifetime instead of silently disabling connection recycling", func() {
			cfg := validSecureConfig()
			cfg.Database.ConnMaxLifetime = "not-a-duration"

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connMaxLifetime"))
		})

		It("UT-DS-CFG-012: rejects an invalid database connMaxIdleTime instead of silently disabling idle-connection recycling", func() {
			cfg := validSecureConfig()
			cfg.Database.ConnMaxIdleTime = "not-a-duration"

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connMaxIdleTime"))
		})

		It("UT-DS-CFG-013: rejects an invalid server shutdownTimeout instead of the operator's drain budget silently defaulting at runtime", func() {
			cfg := validSecureConfig()
			cfg.Server.ShutdownTimeout = "not-a-duration"

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("shutdownTimeout"))
		})

		It("UT-DS-CFG-014: rejects an invalid server endpointPropagationDelay instead of silently defaulting the K8s endpoint-removal wait", func() {
			cfg := validSecureConfig()
			cfg.Server.EndpointPropagationDelay = "not-a-duration"

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("endpointPropagationDelay"))
		})

		It("UT-DS-CFG-015: rejects a non-integer server maxBodySize instead of silently falling back to an unbounded request body", func() {
			cfg := validSecureConfig()
			cfg.Server.MaxBodySize = "not-a-number"

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("maxBodySize"))
		})
	})

	Describe("SI-10: CORS allowlist entry format validation", func() {
		It("UT-DS-CFG-016: rejects an empty or whitespace-only CORS origin entry with a per-index, actionable error", func() {
			cfg := validSecureConfig()
			cfg.Server.CORSAllowedOrigins = []string{"   "}

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("corsAllowedOrigins[0]"))
			Expect(err.Error()).To(ContainSubstring("empty or whitespace-only"))
		})

		It("UT-DS-CFG-017: rejects a CORS origin missing the http(s):// scheme, preventing a malformed allowlist entry from silently producing a non-functioning policy", func() {
			cfg := validSecureConfig()
			cfg.Server.CORSAllowedOrigins = []string{"ftp://bad.example.com"}

			err := cfg.Validate()

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("corsAllowedOrigins[0]"))
			Expect(err.Error()).To(ContainSubstring("http://"))
		})
	})
})
