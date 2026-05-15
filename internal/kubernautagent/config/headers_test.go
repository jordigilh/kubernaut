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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/config"
)

var _ = Describe("Custom Header Config — #417", func() {

	// UT-KA-417-012: Config rejects malformed header definitions
	Describe("UT-KA-417-012: Config validation rejects malformed definitions", func() {
		It("should reject a header with missing name", func() {
			defs := []config.HeaderDefinition{
				{Name: "", Value: "some-value"},
			}
			_, err := config.ParseCustomHeaders(defs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name"))
		})

		It("should reject a header with no value source", func() {
			defs := []config.HeaderDefinition{
				{Name: "x-api-key"},
			}
			_, err := config.ParseCustomHeaders(defs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly one"))
		})

		It("should reject a header with both value and secretKeyRef set", func() {
			defs := []config.HeaderDefinition{
				{Name: "x-api-key", Value: "literal", SecretKeyRef: "SECRET_ENV"},
			}
			_, err := config.ParseCustomHeaders(defs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exactly one"))
		})

		It("should reject duplicate header names", func() {
			defs := []config.HeaderDefinition{
				{Name: "Authorization", Value: "Bearer a"},
				{Name: "Authorization", Value: "Bearer b"},
			}
			_, err := config.ParseCustomHeaders(defs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate"))
		})
	})

	// UT-KA-417-011: Config rejects reserved header names
	Describe("UT-KA-417-011: Reserved header name rejection", func() {
		DescribeTable("should reject reserved header names",
			func(name string) {
				defs := []config.HeaderDefinition{
					{Name: name, Value: "some-value"},
				}
				_, err := config.ParseCustomHeaders(defs)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("reserved"))
			},
			Entry("Content-Type", "Content-Type"),
			Entry("Accept", "Accept"),
			Entry("Host", "Host"),
			Entry("User-Agent", "User-Agent"),
			Entry("content-type (case-insensitive)", "content-type"),
		)

		It("should allow non-reserved header names", func() {
			defs := []config.HeaderDefinition{
				{Name: "Authorization", Value: "Bearer token"},
				{Name: "x-api-key", Value: "key-123"},
			}
			result, err := config.ParseCustomHeaders(defs)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(2))
		})
	})

	// UT-KA-417-010: Startup validation fails fast on missing secretKeyRef
	Describe("UT-KA-417-010: Startup fail-fast on missing secret", func() {
		It("should return error when secretKeyRef env var is unset", func() {
			Expect(os.Unsetenv("KA_TEST_MISSING_SECRET")).To(Succeed())
			defs := []config.HeaderDefinition{
				{Name: "Authorization", SecretKeyRef: "KA_TEST_MISSING_SECRET"},
			}
			err := config.ValidateHeaderSources(defs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("KA_TEST_MISSING_SECRET"))
		})

		It("should return error when secretKeyRef env var is empty string", func() {
			Expect(os.Setenv("KA_TEST_EMPTY_SECRET", "")).To(Succeed())
			DeferCleanup(os.Unsetenv, "KA_TEST_EMPTY_SECRET")

			defs := []config.HeaderDefinition{
				{Name: "Authorization", SecretKeyRef: "KA_TEST_EMPTY_SECRET"},
			}
			err := config.ValidateHeaderSources(defs)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("KA_TEST_EMPTY_SECRET"))
		})

		It("should succeed when secretKeyRef env var has a value", func() {
			Expect(os.Setenv("KA_TEST_VALID_SECRET", "my-api-key")).To(Succeed())
			DeferCleanup(os.Unsetenv, "KA_TEST_VALID_SECRET")

			defs := []config.HeaderDefinition{
				{Name: "Authorization", SecretKeyRef: "KA_TEST_VALID_SECRET"},
			}
			err := config.ValidateHeaderSources(defs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not validate value or filePath sources", func() {
			defs := []config.HeaderDefinition{
				{Name: "x-tenant-id", Value: "prod"},
				{Name: "Authorization", FilePath: "/tmp/token.txt"},
			}
			err := config.ValidateHeaderSources(defs)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// UT-KA-417-013: Config accepts zero custom headers
	Describe("UT-KA-417-013: Zero custom headers is valid", func() {
		It("should return empty slice and no error for nil input", func() {
			result, err := config.ParseCustomHeaders(nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("should return empty slice and no error for empty slice input", func() {
			result, err := config.ParseCustomHeaders([]config.HeaderDefinition{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})
})
