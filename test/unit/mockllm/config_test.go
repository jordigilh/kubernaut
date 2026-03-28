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
			os.Unsetenv("MOCK_LLM_HOST")
			os.Unsetenv("MOCK_LLM_PORT")
			os.Unsetenv("MOCK_LLM_FORCE_TEXT")
			os.Unsetenv("MOCK_LLM_LOG_LEVEL")

			cfg := config.LoadFromEnv()
			Expect(cfg.Host).To(Equal("0.0.0.0"))
			Expect(cfg.Port).To(Equal("8080"))
			Expect(cfg.ForceText).To(BeFalse())
			Expect(cfg.LogLevel).To(Equal("info"))
		})

		It("should read custom values from env vars", func() {
			os.Setenv("MOCK_LLM_HOST", "127.0.0.1")
			os.Setenv("MOCK_LLM_PORT", "9090")
			os.Setenv("MOCK_LLM_FORCE_TEXT", "true")
			os.Setenv("MOCK_LLM_LOG_LEVEL", "debug")
			defer func() {
				os.Unsetenv("MOCK_LLM_HOST")
				os.Unsetenv("MOCK_LLM_PORT")
				os.Unsetenv("MOCK_LLM_FORCE_TEXT")
				os.Unsetenv("MOCK_LLM_LOG_LEVEL")
			}()

			cfg := config.LoadFromEnv()
			Expect(cfg.Host).To(Equal("127.0.0.1"))
			Expect(cfg.Port).To(Equal("9090"))
			Expect(cfg.ForceText).To(BeTrue())
			Expect(cfg.LogLevel).To(Equal("debug"))
		})
	})
})
