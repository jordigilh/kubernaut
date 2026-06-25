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
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
)

var _ = Describe("Fleet OAuth2 Scope Configuration (BR-INTEGRATION-054)", func() {
	Describe("UT-FLEET-SCOPE-001 [IA-5]: Service identity requests minimal scopes by default, ensuring no over-permissioned tokens are issued", func() {
		It("defaults Scopes to [openid, groups] when empty in ReloadableOAuth2Config", func() {
			tmpDir, err := os.MkdirTemp("", "scope-default-*")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			Expect(os.WriteFile(filepath.Join(tmpDir, "client-id"), []byte("test-id"), 0o600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "client-secret"), []byte("test-secret"), 0o600)).To(Succeed())

			cfg := mcpclient.ReloadableOAuth2Config{
				TokenURL:         "http://localhost:0/token",
				ClientIDPath:     filepath.Join(tmpDir, "client-id"),
				ClientSecretPath: filepath.Join(tmpDir, "client-secret"),
				Scopes:           nil,
				TokenTimeout:     5 * time.Second,
			}

			scopes := mcpclient.DefaultFleetScopes(cfg.Scopes)
			Expect(scopes).To(Equal([]string{"openid", "groups"}))
		})

		It("passes through explicit Scopes when configured", func() {
			customScopes := []string{"openid", "profile", "fleet-admin"}
			scopes := mcpclient.DefaultFleetScopes(customScopes)
			Expect(scopes).To(Equal([]string{"openid", "profile", "fleet-admin"}))
		})
	})

	Describe("UT-FLEET-SCOPE-002 [SC-8]: Token request includes only the configured scopes and correct grant type, preventing credential scope escalation", func() {
		It("sends configured scopes in the token POST body with grant_type=client_credentials", func() {
			tmpDir, err := os.MkdirTemp("", "scope-verify-*")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = os.RemoveAll(tmpDir) }()

			Expect(os.WriteFile(filepath.Join(tmpDir, "client-id"), []byte("fleet-reader"), 0o600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "client-secret"), []byte("s3cr3t"), 0o600)).To(Succeed())

			var capturedForm url.Values
			tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				capturedForm = r.PostForm
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
			}))
			defer tokenServer.Close()

			logger := zap.New(zap.UseDevMode(true))
			cfg := mcpclient.ReloadableOAuth2Config{
				TokenURL:         tokenServer.URL,
				ClientIDPath:     filepath.Join(tmpDir, "client-id"),
				ClientSecretPath: filepath.Join(tmpDir, "client-secret"),
				Scopes:           []string{"openid", "groups"},
				TokenTimeout:     5 * time.Second,
			}

			transport, err := mcpclient.NewReloadableOAuth2Transport(cfg, http.DefaultTransport, logger)
			Expect(err).ToNot(HaveOccurred())

			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("ok"))
			}))
			defer backend.Close()

			client := &http.Client{Transport: transport}
			resp, err := client.Get(backend.URL)
			Expect(err).ToNot(HaveOccurred())
			resp.Body.Close()

			Expect(capturedForm).ToNot(BeNil(), "token server should have been called")
			Expect(capturedForm.Get("grant_type")).To(Equal("client_credentials"))
			Expect(capturedForm.Get("scope")).To(Equal("openid groups"))
		})
	})
})
