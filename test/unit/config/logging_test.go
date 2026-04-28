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

package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jordigilh/kubernaut/internal/config"
)

var _ = Describe("Shared LoggingConfig — BR-PLATFORM-875", func() {

	Describe("UT-CFG-875-001: DefaultLoggingConfig returns INFO", func() {
		It("should default to INFO", func() {
			cfg := config.DefaultLoggingConfig()
			Expect(cfg.Level).To(Equal("INFO"))
		})
	})

	Describe("UT-CFG-875-002: ZapLevel maps correctly", func() {
		DescribeTable("level mapping",
			func(level string, expected zapcore.Level) {
				cfg := config.LoggingConfig{Level: level}
				Expect(cfg.ZapLevel()).To(Equal(expected))
			},
			Entry("DEBUG -> DebugLevel", "DEBUG", zapcore.DebugLevel),
			Entry("INFO -> InfoLevel", "INFO", zapcore.InfoLevel),
			Entry("WARN -> WarnLevel", "WARN", zapcore.WarnLevel),
			Entry("ERROR -> ErrorLevel", "ERROR", zapcore.ErrorLevel),
			Entry("debug (lowercase) -> DebugLevel", "debug", zapcore.DebugLevel),
			Entry("empty string -> InfoLevel", "", zapcore.InfoLevel),
			Entry("unknown -> InfoLevel", "TRACE", zapcore.InfoLevel),
		)
	})

	Describe("UT-CFG-875-003: NewAtomicLevel creates correct AtomicLevel", func() {
		DescribeTable("atomic level creation",
			func(level string, expected zapcore.Level) {
				cfg := config.LoggingConfig{Level: level}
				al := cfg.NewAtomicLevel()
				Expect(al.Level()).To(Equal(expected))
			},
			Entry("DEBUG", "DEBUG", zapcore.DebugLevel),
			Entry("INFO", "INFO", zapcore.InfoLevel),
			Entry("WARN", "WARN", zapcore.WarnLevel),
			Entry("ERROR", "ERROR", zapcore.ErrorLevel),
		)
	})

	Describe("UT-CFG-875-004: Validate accepts valid levels", func() {
		DescribeTable("valid levels",
			func(level string) {
				cfg := config.LoggingConfig{Level: level}
				Expect(cfg.Validate()).To(Succeed())
			},
			Entry("DEBUG", "DEBUG"),
			Entry("INFO", "INFO"),
			Entry("WARN", "WARN"),
			Entry("ERROR", "ERROR"),
			Entry("empty (not yet set)", ""),
		)
	})

	Describe("UT-CFG-875-005: Validate rejects invalid levels", func() {
		DescribeTable("invalid levels",
			func(level string) {
				cfg := config.LoggingConfig{Level: level}
				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid log level"))
			},
			Entry("VERBOSE", "VERBOSE"),
			Entry("TRACE", "TRACE"),
			Entry("FATAL", "FATAL"),
			Entry("warning", "warning"),
		)
	})

	Describe("UT-CFG-875-006: ParseAndSetLevel hot-reload helper", func() {
		It("should update AtomicLevel from INFO to DEBUG", func() {
			al := zap.NewAtomicLevelAt(zapcore.InfoLevel)
			err := config.ParseAndSetLevel(al, "DEBUG")
			Expect(err).NotTo(HaveOccurred())
			Expect(al.Level()).To(Equal(zapcore.DebugLevel))
		})

		It("should update AtomicLevel from DEBUG to ERROR", func() {
			al := zap.NewAtomicLevelAt(zapcore.DebugLevel)
			err := config.ParseAndSetLevel(al, "ERROR")
			Expect(err).NotTo(HaveOccurred())
			Expect(al.Level()).To(Equal(zapcore.ErrorLevel))
		})

		It("should handle case-insensitive input", func() {
			al := zap.NewAtomicLevelAt(zapcore.InfoLevel)
			err := config.ParseAndSetLevel(al, "warn")
			Expect(err).NotTo(HaveOccurred())
			Expect(al.Level()).To(Equal(zapcore.WarnLevel))
		})

		It("should handle whitespace-padded input", func() {
			al := zap.NewAtomicLevelAt(zapcore.InfoLevel)
			err := config.ParseAndSetLevel(al, "  DEBUG  ")
			Expect(err).NotTo(HaveOccurred())
			Expect(al.Level()).To(Equal(zapcore.DebugLevel))
		})

		It("should reject invalid levels without modifying AtomicLevel", func() {
			al := zap.NewAtomicLevelAt(zapcore.InfoLevel)
			err := config.ParseAndSetLevel(al, "VERBOSE")
			Expect(err).To(HaveOccurred())
			Expect(al.Level()).To(Equal(zapcore.InfoLevel))
		})

		It("should be a no-op for empty string", func() {
			al := zap.NewAtomicLevelAt(zapcore.WarnLevel)
			err := config.ParseAndSetLevel(al, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(al.Level()).To(Equal(zapcore.WarnLevel))
		})
	})
})
