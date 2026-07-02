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

package fleetmetadatacache

import (
	"context"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// keycloakTokenEndpointFMC is Keycloak's OIDC token endpoint, exposed via
// NodePort 30557 per DD-TEST-001 (see kind-fleetmetadatacache-config.yaml).
const keycloakTokenEndpointFMC = "https://localhost:30557/realms/kubernaut-fleet/protocol/openid-connect/token"

// rawAPIServerRequest issues a GET to the real Kubernetes API server with an
// explicit Bearer token, bypassing client-go's normal credential plugin so
// the test can present exactly the token under evaluation (an exchanged
// token, or a deliberately un-exchanged one). TLS trust (CA only) is derived
// from the suite's own kubeconfig (same CA the API server's cert chains to).
//
// Critically, the suite's kubeconfig is Kind's cluster-admin config, which
// carries an mTLS client certificate for "kubernetes-admin". The API
// server's authenticator chain is a union: if a valid client cert is
// presented during the TLS handshake, x509 authentication succeeds
// regardless of the Bearer token's validity, silently defeating this test's
// entire purpose (a wrong-audience token would still get 200 as
// kubernetes-admin, not the 401 the OIDC path should produce). This function
// therefore strips CertData/CertFile/KeyData/KeyFile before building the TLS
// config so the ONLY credential in play is the Bearer header -- discovered
// live: an identical un-exchanged-token request returned 200 through this
// helper but a correct 401 via a bare curl (no client cert) against the same
// running API server.
func rawAPIServerRequest(g Gomega, path, bearerToken string) *http.Response {
	restCfg, err := config.GetConfig()
	g.Expect(err).ToNot(HaveOccurred())

	caOnlyCfg := *restCfg
	caOnlyCfg.CertData = nil
	caOnlyCfg.CertFile = ""
	caOnlyCfg.KeyData = nil
	caOnlyCfg.KeyFile = ""

	tlsCfg, err := rest.TLSConfigFor(&caOnlyCfg)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(tlsCfg.Certificates).To(BeEmpty(),
		"test bug: TLS config must not carry a client certificate, or x509 auth would mask the Bearer-token check under test")

	httpClient := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, restCfg.Host+path, nil)
	g.Expect(err).ToNot(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+bearerToken)

	resp, err := httpClient.Do(req)
	g.Expect(err).ToNot(HaveOccurred())
	return resp
}

// E2E-FMC-054-014: Proves kube-mcp-server's real RFC 8693 Standard Token
// Exchange wiring against Keycloak end-to-end (Spike S17/S18) -- driving the
// exact 3-party exchange kube-mcp-server performs internally
// (pkg/kubernetes/sts.go) and confirming the real Kubernetes API server
// honors the resulting token. E2E-FMC-054-010's sync journey already proves
// this pipeline works *indirectly* (FMC discovers resources through it); this
// test proves the exchange mechanics directly, and that the exchange step is
// not a no-op passthrough (AC-6 least privilege: an un-exchanged subject
// token must NOT work against the API server).
//
// Authority: Issue #54, ADR-068, Spike S17/S18, BR-INTEGRATION-065.
var _ = Describe("E2E-FMC-054-014: kube-mcp-server's RFC 8693 token exchange is wired to real Keycloak (AC-6)", func() {
	var subjectToken string

	BeforeEach(func() {
		By("acquiring FMC's own client_credentials token from Keycloak (the subject token)")
		var err error
		subjectToken, err = infrastructure.GetKeycloakClientCredentialsToken(infrastructure.KeycloakFleetTokenConfig{
			TokenEndpoint: keycloakTokenEndpointFMC,
			ClientID:      "kubernaut-fleet-read",
			ClientSecret:  "e2e-fleet-secret",
		})
		Expect(err).ToNot(HaveOccurred(), "failed to acquire subject token from Keycloak")
		Expect(subjectToken).ToNot(BeEmpty())
	})

	It("exchanges the subject token for a k8s-api-audience token that the real API server honors", func() {
		By("performing the RFC 8693 exchange exactly as kube-mcp-server does (client_id=kube-mcp-server, audience=k8s-api)")
		exchangedToken, err := infrastructure.ExchangeKeycloakToken(
			keycloakTokenEndpointFMC, "kube-mcp-server", "e2e-kube-mcp-server-secret",
			subjectToken, "k8s-api")
		Expect(err).ToNot(HaveOccurred(), "RFC 8693 token exchange against Keycloak failed")
		Expect(exchangedToken).ToNot(BeEmpty())

		By("presenting the exchanged token directly to the real Kubernetes API server")
		Eventually(func(g Gomega) {
			resp := rawAPIServerRequest(g, "/api/v1/namespaces", exchangedToken)
			defer func() { _ = resp.Body.Close() }()
			g.Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"the exchanged token must be accepted by the API server's OIDC authenticator "+
					"(--oidc-client-id=k8s-api) and authorized by the exchanged-identity RBAC binding "+
					"(keycloak:service-account-kubernaut-fleet-read -> view)")
		}, timeout, interval).Should(Succeed())
	})

	It("rejects the un-exchanged subject token when presented directly to the API server (exchange is not a no-op)", func() {
		By("presenting the subject token (audience=kube-mcp-server) directly, skipping the exchange step")
		resp := rawAPIServerRequest(Default, "/api/v1/namespaces", subjectToken)
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
			"a token audienced for kube-mcp-server (not k8s-api) must be rejected by the API server's "+
				"OIDC authenticator -- proving the RFC 8693 exchange step is a real security boundary, "+
				"not an inert passthrough (AC-6)")
	})
})
