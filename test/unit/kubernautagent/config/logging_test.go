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
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/config"
)

var _ = Describe("Kubernaut Agent Logging Configuration — #875", func() {

	Describe("UT-KA-875-001: DefaultConfig sets logging level to INFO", func() {
		It("should default Logging.Level to INFO", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Logging.Level).To(Equal("INFO"))
		})
	})

	Describe("UT-KA-875-002: Logging level parsed from YAML config", func() {
		It("should parse logging.level from YAML", func() {
			yamlData := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
logging:
  level: "DEBUG"
`)
			cfg, err := config.Load(yamlData)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Logging.Level).To(Equal("DEBUG"))
		})
	})

	Describe("UT-KA-875-003: Validate rejects invalid log level", func() {
		It("should reject an unrecognized log level", func() {
			yamlData := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
logging:
  level: "VERBOSE"
`)
			cfg, err := config.Load(yamlData)
			Expect(err).NotTo(HaveOccurred())
			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("log level"))
		})
	})

	Describe("UT-KA-875-004: Validate accepts all valid log levels", func() {
		DescribeTable("valid log levels",
			func(level string) {
				yamlData := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
logging:
  level: "` + level + `"
`)
				cfg, err := config.Load(yamlData)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Validate()).To(Succeed())
			},
			Entry("DEBUG", "DEBUG"),
			Entry("INFO", "INFO"),
			Entry("WARN", "WARN"),
			Entry("ERROR", "ERROR"),
		)
	})

	Describe("UT-KA-875-005: SlogLevel returns correct slog.Level", func() {
		DescribeTable("level mapping",
			func(level string, expected slog.Level) {
				yamlData := []byte(`
llm:
  provider: "openai"
  model: "gpt-4o"
logging:
  level: "` + level + `"
`)
				cfg, err := config.Load(yamlData)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Logging.SlogLevel()).To(Equal(expected))
			},
			Entry("DEBUG -> slog.LevelDebug", "DEBUG", slog.LevelDebug),
			Entry("INFO -> slog.LevelInfo", "INFO", slog.LevelInfo),
			Entry("WARN -> slog.LevelWarn", "WARN", slog.LevelWarn),
			Entry("ERROR -> slog.LevelError", "ERROR", slog.LevelError),
		)
	})

	Describe("UT-KA-875-006: SlogLevel defaults to INFO for empty level", func() {
		It("should return slog.LevelInfo when level is empty", func() {
			cfg := config.DefaultConfig()
			Expect(cfg.Logging.SlogLevel()).To(Equal(slog.LevelInfo))
		})
	})
})
