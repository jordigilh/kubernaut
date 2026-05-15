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

// White-box test: lives in package server to access unexported loadSigningCertificate.
// Same pattern as shutdown_test.go (DX-M1).
package server

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/cert"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

var _ = Describe("#1048 Phase 5 / AU-9: loadSigningCertificate fail-hard", func() {
	var logger = kubelog.NewLogger(kubelog.DefaultOptions())

	Context("valid certificate", func() {
		It("UT-DS-1048-P5-007: should load a valid self-signed cert and return a Signer", func() {
			tmpDir := GinkgoT().TempDir()
			pair, err := cert.GenerateSelfSigned(cert.CertificateOptions{
				CommonName: "test-signer",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(os.WriteFile(filepath.Join(tmpDir, "tls.crt"), pair.CertPEM, 0600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "tls.key"), pair.KeyPEM, 0600)).To(Succeed())

			signer, err := loadSigningCertificate(logger, tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(signer).NotTo(BeNil())
			Expect(signer.GetAlgorithm()).To(Equal("SHA256withRSA"))
		})
	})

	Context("missing certificate file", func() {
		It("UT-DS-1048-P5-008: should fail with AU-9 error when tls.crt is missing", func() {
			tmpDir := GinkgoT().TempDir()
			// Only write the key, not the cert
			pair, err := cert.GenerateSelfSigned(cert.CertificateOptions{
				CommonName: "test-signer",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(tmpDir, "tls.key"), pair.KeyPEM, 0600)).To(Succeed())

			signer, err := loadSigningCertificate(logger, tmpDir)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("signing certificate not found"))
			Expect(err.Error()).To(ContainSubstring("AU-9"))
			Expect(signer).To(BeNil())
		})
	})

	Context("missing key file", func() {
		It("UT-DS-1048-P5-009: should fail with AU-9 error when tls.key is missing", func() {
			tmpDir := GinkgoT().TempDir()
			pair, err := cert.GenerateSelfSigned(cert.CertificateOptions{
				CommonName: "test-signer",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(tmpDir, "tls.crt"), pair.CertPEM, 0600)).To(Succeed())

			signer, err := loadSigningCertificate(logger, tmpDir)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("signing key not found"))
			Expect(err.Error()).To(ContainSubstring("AU-9"))
			Expect(signer).To(BeNil())
		})
	})

	Context("corrupt PEM data", func() {
		It("UT-DS-1048-P5-010: should fail when tls.crt contains invalid PEM", func() {
			tmpDir := GinkgoT().TempDir()
			pair, err := cert.GenerateSelfSigned(cert.CertificateOptions{
				CommonName: "test-signer",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(os.WriteFile(filepath.Join(tmpDir, "tls.crt"), []byte("NOT-VALID-PEM"), 0600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "tls.key"), pair.KeyPEM, 0600)).To(Succeed())

			signer, err := loadSigningCertificate(logger, tmpDir)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid or corrupt"))
			Expect(signer).To(BeNil())
		})
	})

	Context("empty directory", func() {
		It("UT-DS-1048-P5-011: should fail when cert directory is empty", func() {
			tmpDir := GinkgoT().TempDir()

			signer, err := loadSigningCertificate(logger, tmpDir)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("signing certificate not found"))
			Expect(signer).To(BeNil())
		})
	})
})
