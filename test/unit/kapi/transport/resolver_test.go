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

package transport_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kapi/llm/transport"
)

var _ = Describe("Header Value Resolver — #417", func() {

	// UT-KAPI-417-004: value source inlines literal
	Describe("UT-KAPI-417-004: ResolveValue returns literal string", func() {
		It("should return the literal value unchanged", func() {
			result := transport.ResolveValue("test-key")
			Expect(result).To(Equal("test-key"))
		})

		It("should return empty string for empty input", func() {
			result := transport.ResolveValue("")
			Expect(result).To(Equal(""))
		})
	})

	// UT-KAPI-417-002: secretKeyRef resolves from environment variable
	Describe("UT-KAPI-417-002: ResolveSecretKeyRef from env var", func() {
		It("should return the env var value", func() {
			os.Setenv("KAPI_TEST_LLM_API_KEY", "secret-api-key-123")
			defer os.Unsetenv("KAPI_TEST_LLM_API_KEY")

			val, err := transport.ResolveSecretKeyRef("KAPI_TEST_LLM_API_KEY")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("secret-api-key-123"))
		})

		It("should return error for unset env var", func() {
			os.Unsetenv("KAPI_TEST_NONEXISTENT_VAR")

			_, err := transport.ResolveSecretKeyRef("KAPI_TEST_NONEXISTENT_VAR")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("KAPI_TEST_NONEXISTENT_VAR"))
		})
	})

	// UT-KAPI-417-003: secretKeyRef resolves from mounted file (simulated as env var)
	Describe("UT-KAPI-417-003: ResolveSecretKeyRef from mounted volume (env)", func() {
		It("should return the env var value set from mounted secret", func() {
			os.Setenv("KAPI_TEST_MOUNTED_SECRET", "mounted-secret-value")
			defer os.Unsetenv("KAPI_TEST_MOUNTED_SECRET")

			val, err := transport.ResolveSecretKeyRef("KAPI_TEST_MOUNTED_SECRET")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("mounted-secret-value"))
		})
	})

	// UT-KAPI-417-005: filePath reads file content
	Describe("UT-KAPI-417-005: ResolveFilePath reads file content", func() {
		It("should return trimmed file content", func() {
			tmpFile, err := os.CreateTemp("", "kapi-test-token-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.WriteString("jwt-token-xyz\n")
			Expect(err).NotTo(HaveOccurred())
			tmpFile.Close()

			val, err := transport.ResolveFilePath(tmpFile.Name())
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal("jwt-token-xyz"))
		})
	})

	// UT-KAPI-417-006: filePath re-read on token rotation
	Describe("UT-KAPI-417-006: filePath re-reads on token rotation", func() {
		It("should return updated content after file overwrite", func() {
			tmpFile, err := os.CreateTemp("", "kapi-test-rotation-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tmpFile.Name())

			err = os.WriteFile(tmpFile.Name(), []byte("token-v1"), 0644)
			Expect(err).NotTo(HaveOccurred())

			val1, err := transport.ResolveFilePath(tmpFile.Name())
			Expect(err).NotTo(HaveOccurred())
			Expect(val1).To(Equal("token-v1"))

			err = os.WriteFile(tmpFile.Name(), []byte("token-v2"), 0644)
			Expect(err).NotTo(HaveOccurred())

			val2, err := transport.ResolveFilePath(tmpFile.Name())
			Expect(err).NotTo(HaveOccurred())
			Expect(val2).To(Equal("token-v2"))
		})
	})

	// UT-KAPI-417-014: filePath missing at request time
	Describe("UT-KAPI-417-014: filePath file missing at request time", func() {
		It("should return error containing the file path", func() {
			_, err := transport.ResolveFilePath("/tmp/kapi-test-nonexistent-token.txt")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("/tmp/kapi-test-nonexistent-token.txt"))
		})

		It("should not panic or return empty string silently", func() {
			val, err := transport.ResolveFilePath("/tmp/kapi-test-nonexistent-token.txt")
			Expect(err).To(HaveOccurred())
			Expect(val).To(BeEmpty())
		})
	})

	// UT-KAPI-417-015: filePath empty file content rejected
	Describe("UT-KAPI-417-015: filePath empty file rejected", func() {
		It("should return error for empty file", func() {
			tmpFile, err := os.CreateTemp("", "kapi-test-empty-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tmpFile.Name())
			tmpFile.Close()

			_, err = transport.ResolveFilePath(tmpFile.Name())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("empty"))
		})

		It("should return error for whitespace-only file", func() {
			tmpFile, err := os.CreateTemp("", "kapi-test-ws-*")
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(tmpFile.Name())

			err = os.WriteFile(tmpFile.Name(), []byte("  \n\t\n  "), 0644)
			Expect(err).NotTo(HaveOccurred())

			_, err = transport.ResolveFilePath(tmpFile.Name())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("empty"))
		})
	})
})
