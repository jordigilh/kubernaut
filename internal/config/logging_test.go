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

	Describe("UT-CFG-1330-001: DefaultLoggingConfig returns Format json (AU-3)", func() {
		It("should default to json for machine-parseable audit trail", func() {
			cfg := config.DefaultLoggingConfig()
			Expect(cfg.Format).To(Equal("json"),
				"AU-3: default log format must be JSON for structured audit records")
		})
	})

	Describe("UT-CFG-1330-002: Validate accepts valid formats, rejects invalid (CM-3)", func() {
		DescribeTable("valid formats",
			func(format string) {
				cfg := config.LoggingConfig{Level: "INFO", Format: format}
				Expect(cfg.Validate()).To(Succeed())
			},
			Entry("json", "json"),
			Entry("console", "console"),
			Entry("JSON (case-insensitive)", "JSON"),
			Entry("Console (case-insensitive)", "Console"),
			Entry("empty (backward compat)", ""),
		)

		DescribeTable("invalid formats",
			func(format string) {
				cfg := config.LoggingConfig{Level: "INFO", Format: format}
				err := cfg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("log format"))
			},
			Entry("yaml", "yaml"),
			Entry("text", "text"),
			Entry("logfmt", "logfmt"),
			Entry("XML", "XML"),
		)
	})

	Describe("UT-CFG-1330-003: IsConsoleFormat returns correct boolean (AU-3)", func() {
		DescribeTable("format detection",
			func(format string, expected bool) {
				cfg := config.LoggingConfig{Format: format}
				Expect(cfg.IsConsoleFormat()).To(Equal(expected))
			},
			Entry("console -> true", "console", true),
			Entry("CONSOLE -> true (case-insensitive)", "CONSOLE", true),
			Entry("json -> false", "json", false),
			Entry("JSON -> false (case-insensitive)", "JSON", false),
			Entry("empty -> false (default JSON)", "", false),
		)
	})

	Describe("UT-CFG-1330-004: empty Format defaults to JSON behavior (AU-3)", func() {
		It("should treat empty format as JSON", func() {
			cfg := config.LoggingConfig{Format: ""}
			Expect(cfg.IsConsoleFormat()).To(BeFalse(),
				"AU-3: empty format must default to JSON for production audit compliance")
		})
	})

	Describe("UT-CFG-875-007: ParseAndSetLevel hot-reload helper", func() {
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
