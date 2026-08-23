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

package credentials_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/credentials"
)

var _ = Describe("ResolveGCPCredentialIndirection — #686", func() {

	var (
		credDir string
		logger  = logr.Discard()
	)

	BeforeEach(func() {
		var err error
		credDir, err = os.MkdirTemp("", "cred-resolver-test-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(os.RemoveAll(credDir)).To(Succeed()) })
	})

	// -- Passthrough scenarios --

	It("UT-KA-686-001: returns service_account JSON as-is", func() {
		saJSON := `{"type":"service_account","project_id":"test","private_key_id":"k1","private_key":"-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----\n","client_email":"sa@test.iam.gserviceaccount.com"}` // pre-commit:allow-sensitive (test fixture)

		result := credentials.ResolveGCPCredentialIndirection("vertex_ai", saJSON, credDir, logger)
		Expect(result).To(Equal(saJSON))
	})

	It("UT-KA-686-002: returns authorized_user JSON as-is", func() {
		auJSON := `{"type":"authorized_user","client_id":"cid","client_secret":"cs","refresh_token":"rt"}` // pre-commit:allow-sensitive (test fixture)

		result := credentials.ResolveGCPCredentialIndirection("vertex", auJSON, credDir, logger)
		Expect(result).To(Equal(auJSON))
	})

	It("UT-KA-686-005: non-GCP provider returns content unchanged", func() {
		result := credentials.ResolveGCPCredentialIndirection("openai", "sk-test-key-123", credDir, logger) // pre-commit:allow-sensitive (test fixture)
		Expect(result).To(Equal("sk-test-key-123"))                                                         // pre-commit:allow-sensitive (test fixture)
	})

	// -- Indirection scenarios --

	It("UT-KA-686-003: follows path indirection to read target file", func() {
		targetContent := `{"type":"service_account","project_id":"resolved"}` // pre-commit:allow-sensitive (test fixture)
		targetPath := filepath.Join(credDir, "adc.json")
		Expect(os.WriteFile(targetPath, []byte(targetContent), 0600)).To(Succeed())

		result := credentials.ResolveGCPCredentialIndirection("vertex_ai", targetPath, credDir, logger)
		Expect(result).To(Equal(targetContent))
	})

	It("UT-KA-686-004: returns empty string when indirection target is missing (F-04/Gap 6)", func() {
		missingPath := filepath.Join(credDir, "does-not-exist.json")

		result := credentials.ResolveGCPCredentialIndirection("vertex_ai", missingPath, credDir, logger)
		Expect(result).To(BeEmpty())
	})

	// -- Whitespace handling --

	It("UT-KA-686-006: JSON with leading whitespace is still detected as JSON object (Gap 5)", func() {
		paddedJSON := `   {"type":"service_account","project_id":"test"}` // pre-commit:allow-sensitive (test fixture)

		result := credentials.ResolveGCPCredentialIndirection("vertex_ai", paddedJSON, credDir, logger)
		Expect(result).To(Equal(paddedJSON))
	})

	// -- Security scenarios --

	It("UT-KA-686-007: path traversal blocked — relative parent path returns empty (F-01)", func() {
		result := credentials.ResolveGCPCredentialIndirection("vertex_ai", "../../etc/passwd", credDir, logger)
		Expect(result).To(BeEmpty())
	})

	It("UT-KA-686-008: relative path blocked — ./adc.json returns empty (F-10)", func() {
		targetPath := filepath.Join(credDir, "adc.json")
		Expect(os.WriteFile(targetPath, []byte(`{"type":"service_account"}`), 0600)).To(Succeed()) // pre-commit:allow-sensitive (test fixture)

		result := credentials.ResolveGCPCredentialIndirection("vertex_ai", "./adc.json", credDir, logger)
		Expect(result).To(BeEmpty())
	})

	It("UT-KA-686-009: JSON literal (non-object) treated as path, fails validation, returns empty (F-02)", func() {
		result := credentials.ResolveGCPCredentialIndirection("vertex_ai", "null", credDir, logger)
		Expect(result).To(BeEmpty())

		result = credentials.ResolveGCPCredentialIndirection("vertex_ai", "true", credDir, logger)
		Expect(result).To(BeEmpty())

		result = credentials.ResolveGCPCredentialIndirection("vertex_ai", "123", credDir, logger)
		Expect(result).To(BeEmpty())
	})

	It("UT-KA-686-010: file size limit — target > 1MB returns empty (F-05)", func() {
		largePath := filepath.Join(credDir, "large.json")
		largeContent := strings.Repeat("x", 2*1024*1024)
		Expect(os.WriteFile(largePath, []byte(largeContent), 0600)).To(Succeed())

		result := credentials.ResolveGCPCredentialIndirection("vertex_ai", largePath, credDir, logger)
		Expect(result).To(BeEmpty())
	})
})

var _ = Describe("ResolveCredentialsFile — #2258 stale provider key-file cleanup", func() {

	var (
		credDir string
		logger  = logr.Discard()
	)

	BeforeEach(func() {
		var err error
		credDir, err = os.MkdirTemp("", "cred-resolver-keyfile-test-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(os.RemoveAll(credDir)).To(Succeed()) })
	})

	// These two providers are dead (rejected downstream regardless of endpoint,
	// per #2258) and must no longer get a preferential, provider-named
	// credential-file lookup. Proven here with two files in credDir where the
	// old provider-specific filename does NOT sort first alphabetically: if any
	// special-case fast path for "mistral"/"huggingface" still existed, it would
	// find the provider-named file directly and ignore the alphabetically-first
	// file; once removed, resolution falls through to the generic fallback loop
	// (os.ReadDir, lexicographic order) and returns the first non-empty file.

	It("UT-KA-2258-004: mistral gets no provider-specific credential file preference", func() {
		Expect(os.WriteFile(filepath.Join(credDir, "AAA_FIRST"), []byte("first-content"), 0600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(credDir, "MISTRAL_API_KEY"), []byte("mistral-content"), 0600)).To(Succeed()) // pre-commit:allow-sensitive (test fixture)

		result := credentials.ResolveCredentialsFile("mistral", credDir, logger)
		Expect(result).To(Equal("first-content"),
			"mistral is not a supported provider (#2258) -- it must fall through to the generic "+
				"alphabetical-first-file fallback, not resolve MISTRAL_API_KEY preferentially")
	})

	It("UT-KA-2258-005: huggingface gets no provider-specific credential file preference", func() {
		Expect(os.WriteFile(filepath.Join(credDir, "AAA_FIRST"), []byte("first-content"), 0600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(credDir, "HUGGINGFACEHUB_API_TOKEN"), []byte("hf-content"), 0600)).To(Succeed()) // pre-commit:allow-sensitive (test fixture)

		result := credentials.ResolveCredentialsFile("huggingface", credDir, logger)
		Expect(result).To(Equal("first-content"),
			"huggingface is not a supported provider (#2258) -- it must fall through to the generic "+
				"alphabetical-first-file fallback, not resolve HUGGINGFACEHUB_API_TOKEN preferentially")
	})

	It("UT-KA-2258-006: openai still gets its provider-specific credential file (regression guard -- real, supported provider unaffected)", func() {
		Expect(os.WriteFile(filepath.Join(credDir, "AAA_FIRST"), []byte("first-content"), 0600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(credDir, "OPENAI_API_KEY"), []byte("openai-content"), 0600)).To(Succeed()) // pre-commit:allow-sensitive (test fixture)

		result := credentials.ResolveCredentialsFile("openai", credDir, logger)
		Expect(result).To(Equal("openai-content"),
			"openai is a real, supported provider -- its provider-specific fast path must be unaffected by #2258")
	})
})
