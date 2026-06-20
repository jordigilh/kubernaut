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

package mcpclient_test

import (
	"net/http"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
)

var _ = Describe("OAuth2 Auth Transport (BR-INTEGRATION-065)", func() {
	Describe("NewOAuth2Transport", func() {
		It("UT-FLEET-AUTH-001: creates a RoundTripper with OAuth2 config", func() {
			cfg := mcpclient.OAuth2Config{
				TokenURL:     "https://dex.example.com/token",
				ClientID:     "kubernaut-ka",
				ClientSecret: "test-secret",
				Scopes:       []string{"openid"},
			}
			transport := mcpclient.NewOAuth2Transport(cfg, nil)
			Expect(transport).ToNot(BeNil())
		})

		It("UT-FLEET-AUTH-002: wraps a base transport", func() {
			base := &http.Transport{}
			cfg := mcpclient.OAuth2Config{
				TokenURL:     "https://dex.example.com/token",
				ClientID:     "kubernaut-ka",
				ClientSecret: "test-secret",
			}
			transport := mcpclient.NewOAuth2Transport(cfg, base)
			Expect(transport).ToNot(BeNil())
		})
	})

	Describe("LoadOAuth2ConfigFromFiles", func() {
		var tmpDir string

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "fleet-oauth2-*")
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			_ = os.RemoveAll(tmpDir)
		})

		It("UT-FLEET-AUTH-003: loads config from file-mounted paths", func() {
			Expect(os.WriteFile(filepath.Join(tmpDir, "token-url"), []byte("https://dex.local/token\n"), 0600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "client-id"), []byte("  kubernaut-fleet  \n"), 0600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "client-secret"), []byte("s3cr3t"), 0600)).To(Succeed())

			cfg, err := mcpclient.LoadOAuth2ConfigFromFiles(
				filepath.Join(tmpDir, "token-url"),
				filepath.Join(tmpDir, "client-id"),
				filepath.Join(tmpDir, "client-secret"),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.TokenURL).To(Equal("https://dex.local/token"))
			Expect(cfg.ClientID).To(Equal("kubernaut-fleet"))
			Expect(cfg.ClientSecret).To(Equal("s3cr3t"))
		})

		It("UT-FLEET-AUTH-004: returns error for missing file", func() {
			_, err := mcpclient.LoadOAuth2ConfigFromFiles(
				filepath.Join(tmpDir, "nonexistent"),
				filepath.Join(tmpDir, "client-id"),
				filepath.Join(tmpDir, "client-secret"),
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("token URL"))
		})
	})

	Describe("WithOAuth2Transport option", func() {
		It("UT-FLEET-AUTH-005: integrates as a client option", func() {
			cfg := mcpclient.OAuth2Config{
				TokenURL:     "https://dex.example.com/token",
				ClientID:     "kubernaut-ka",
				ClientSecret: "test-secret",
			}
			opt := mcpclient.WithOAuth2Transport(cfg)
			Expect(opt).ToNot(BeNil())
		})
	})
})
