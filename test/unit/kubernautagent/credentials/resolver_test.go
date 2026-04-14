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
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/credentials"
)

var _ = Describe("ResolveGCPCredentialIndirection — #686", func() {

	var (
		credDir string
		logger  *slog.Logger
	)

	BeforeEach(func() {
		var err error
		credDir, err = os.MkdirTemp("", "cred-resolver-test-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { os.RemoveAll(credDir) })

		logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{Level: slog.LevelDebug}))
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
