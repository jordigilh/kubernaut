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
package mockllm_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/config"
)

var _ = Describe("Environment Config", func() {

	Describe("UT-MOCK-032-001: LoadFromEnv reads env vars with correct defaults", func() {
		It("should return defaults when no env vars are set", func() {
			Expect(os.Unsetenv("MOCK_LLM_HOST")).To(Succeed())
			Expect(os.Unsetenv("MOCK_LLM_PORT")).To(Succeed())
			Expect(os.Unsetenv("MOCK_LLM_FORCE_TEXT")).To(Succeed())
			Expect(os.Unsetenv("MOCK_LLM_LOG_LEVEL")).To(Succeed())
			Expect(os.Unsetenv("MOCK_LLM_MODE")).To(Succeed())

			cfg := config.LoadFromEnv()
			Expect(cfg.Host).To(Equal("0.0.0.0"))
			Expect(cfg.Port).To(Equal("8080"))
			Expect(cfg.ForceText).To(BeFalse())
			Expect(cfg.Mode).To(Equal(config.ModeFull))
			Expect(cfg.LogLevel).To(Equal("info"))
		})

		It("should read custom values from env vars", func() {
			Expect(os.Setenv("MOCK_LLM_HOST", "127.0.0.1")).To(Succeed())
			Expect(os.Setenv("MOCK_LLM_PORT", "9090")).To(Succeed())
			Expect(os.Setenv("MOCK_LLM_FORCE_TEXT", "true")).To(Succeed())
			Expect(os.Setenv("MOCK_LLM_LOG_LEVEL", "debug")).To(Succeed())
			Expect(os.Unsetenv("MOCK_LLM_MODE")).To(Succeed())
			DeferCleanup(func() {
				Expect(os.Unsetenv("MOCK_LLM_HOST")).To(Succeed())
				Expect(os.Unsetenv("MOCK_LLM_PORT")).To(Succeed())
				Expect(os.Unsetenv("MOCK_LLM_FORCE_TEXT")).To(Succeed())
				Expect(os.Unsetenv("MOCK_LLM_LOG_LEVEL")).To(Succeed())
			})

			cfg := config.LoadFromEnv()
			Expect(cfg.Host).To(Equal("127.0.0.1"))
			Expect(cfg.Port).To(Equal("9090"))
			Expect(cfg.ForceText).To(BeTrue())
			Expect(cfg.Mode).To(Equal(config.ModeAutonomous))
			Expect(cfg.LogLevel).To(Equal("debug"))
		})
	})

	Describe("UT-MOCK-032-002: MOCK_LLM_MODE env var", func() {
		AfterEach(func() {
			Expect(os.Unsetenv("MOCK_LLM_MODE")).To(Succeed())
			Expect(os.Unsetenv("MOCK_LLM_FORCE_TEXT")).To(Succeed())
		})

		It("should use explicit MOCK_LLM_MODE when set", func() {
			Expect(os.Setenv("MOCK_LLM_MODE", "interactive")).To(Succeed())
			cfg := config.LoadFromEnv()
			Expect(cfg.Mode).To(Equal(config.ModeInteractive))
		})

		It("should derive autonomous from FORCE_TEXT=true when MODE is unset", func() {
			Expect(os.Unsetenv("MOCK_LLM_MODE")).To(Succeed())
			Expect(os.Setenv("MOCK_LLM_FORCE_TEXT", "true")).To(Succeed())
			cfg := config.LoadFromEnv()
			Expect(cfg.Mode).To(Equal(config.ModeAutonomous))
		})

		It("should derive full from FORCE_TEXT=false when MODE is unset", func() {
			Expect(os.Unsetenv("MOCK_LLM_MODE")).To(Succeed())
			Expect(os.Unsetenv("MOCK_LLM_FORCE_TEXT")).To(Succeed())
			cfg := config.LoadFromEnv()
			Expect(cfg.Mode).To(Equal(config.ModeFull))
		})

		It("should prefer explicit MODE over FORCE_TEXT derivation", func() {
			Expect(os.Setenv("MOCK_LLM_MODE", "full")).To(Succeed())
			Expect(os.Setenv("MOCK_LLM_FORCE_TEXT", "true")).To(Succeed())
			cfg := config.LoadFromEnv()
			Expect(cfg.Mode).To(Equal(config.ModeFull))
		})
	})

	Describe("UT-MOCK-032-003: ResolveMode logic", func() {
		It("should return explicit mode when provided", func() {
			Expect(config.ResolveMode("interactive", false)).To(Equal(config.ModeInteractive))
			Expect(config.ResolveMode("autonomous", false)).To(Equal(config.ModeAutonomous))
			Expect(config.ResolveMode("full", true)).To(Equal(config.ModeFull))
		})

		It("should derive from forceText when explicit is empty", func() {
			Expect(config.ResolveMode("", true)).To(Equal(config.ModeAutonomous))
			Expect(config.ResolveMode("", false)).To(Equal(config.ModeFull))
		})
	})

	Describe("UT-MOCK-032-004: ValidateMode rejects unknown values", func() {
		It("should accept valid modes", func() {
			Expect(config.ValidateMode(config.ModeInteractive)).To(Succeed())
			Expect(config.ValidateMode(config.ModeAutonomous)).To(Succeed())
			Expect(config.ValidateMode(config.ModeFull)).To(Succeed())
		})

		It("should reject empty string", func() {
			Expect(config.ValidateMode("")).To(MatchError(ContainSubstring("MOCK_LLM_MODE invalid")))
		})

		It("should reject unknown mode values", func() {
			Expect(config.ValidateMode("unknown")).To(MatchError(ContainSubstring("MOCK_LLM_MODE invalid")))
			Expect(config.ValidateMode("INTERACTIVE")).To(MatchError(ContainSubstring("MOCK_LLM_MODE invalid")))
		})

		It("should reject adversarial input", func() {
			Expect(config.ValidateMode("interactive; rm -rf /")).To(MatchError(ContainSubstring("MOCK_LLM_MODE invalid")))
			Expect(config.ValidateMode("full\x00interactive")).To(MatchError(ContainSubstring("MOCK_LLM_MODE invalid")))
		})
	})
})
