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

package infrastructure

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Keycloak password grant", func() {
	It("UT-INFRA-FLEET-OIDC-002: requests the Fleet API Frontend user's Keycloak token with direct grant", func() {
		var received url.Values
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			received, err = url.ParseQuery(string(body))
			Expect(err).NotTo(HaveOccurred())
			Expect(r.Header.Get("Content-Type")).To(Equal("application/x-www-form-urlencoded"))
			_, _ = w.Write([]byte(`{"access_token":"keycloak-token"}`))
		}))
		defer server.Close()

		token, err := GetKeycloakPasswordToken(context.Background(), KeycloakUserTokenConfig{
			TokenEndpoint: server.URL,
			ClientID:      "kubernaut-apifrontend",
			ClientSecret:  "e2e-apifrontend-secret",
			Username:      "sre-user",
			Password:      "password",
			Scopes:        []string{"openid", "email", "profile", "groups"},
			HTTPClient:    server.Client(),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(token).To(Equal("keycloak-token"))
		Expect(received.Get("grant_type")).To(Equal("password"))
		Expect(received.Get("client_id")).To(Equal("kubernaut-apifrontend"))
		Expect(received.Get("username")).To(Equal("sre-user"))
		Expect(received.Get("scope")).To(Equal("openid email profile groups"))
	})

	It("UT-INFRA-FLEET-OIDC-003: default Fleet API Frontend config uses Keycloak", func() {
		cfg := DefaultKeycloakAFA2AConfig(30557, "kubeconfig")
		Expect(cfg.TokenEndpoint).To(Equal("https://localhost:30557/realms/kubernaut-demo/protocol/openid-connect/token"))
		Expect(cfg.ClientID).To(Equal("kubernaut-apifrontend"))
		Expect(cfg.ClientSecret).To(Equal("e2e-apifrontend-secret"))
		Expect(strings.Join(cfg.Scopes, " ")).To(Equal("openid email profile groups"))
	})
})
