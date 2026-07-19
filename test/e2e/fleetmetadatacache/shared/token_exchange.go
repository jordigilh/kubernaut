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

package shared

import (
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// keycloakTokenEndpointFMC is Keycloak's OIDC token endpoint, exposed via
// NodePort 30557 per DD-TEST-001. Both lanes reuse the exact same Keycloak
// realm/port -- safe because each runs in its own isolated Kind cluster.
const keycloakTokenEndpointFMC = "https://localhost:30557/realms/kubernaut-fleet/protocol/openid-connect/token"

// TokenExchange proves kube-mcp-server's real RFC 8693 Standard Token
// Exchange wiring against Keycloak end to end -- driving the exact 3-party
// exchange kube-mcp-server performs internally (pkg/kubernetes/sts.go) and
// confirming the real Kubernetes API server honors the resulting token.
// SyncJourney already proves this pipeline works *indirectly* (FMC
// discovers resources through it); this scenario proves the exchange
// mechanics directly, and that the exchange step is not a no-op passthrough
// (AC-6 least privilege: an un-exchanged subject token must NOT work
// against the API server).
//
// 100% gateway-agnostic per ADR-068 Decision #9 (Key design decision): the
// RFC 8693 exchange lives entirely inside kube-mcp-server, not the gateway,
// so this scenario is identical regardless of which gateway fronts
// kube-mcp-server.
//
// {ScenarioPrefix}-014, e.g. E2E-FMC-054-014 (Kuadrant) /
// E2E-FMC-EAIGW-054-014 (EAIGW).
//
// Authority: Issue #54, ADR-068, Spike S17/S18, BR-INTEGRATION-065.
func TokenExchange(h *Harness, v Variant) bool {
	return Describe(fmt.Sprintf("%s-014: kube-mcp-server's RFC 8693 token exchange is wired to real Keycloak (AC-6)", v.ScenarioPrefix()), func() {
		var subjectToken string

		BeforeEach(func() {
			By("acquiring FMC's own client_credentials token from Keycloak (the subject token)")
			var err error
			subjectToken, err = infrastructure.GetKeycloakClientCredentialsToken(h.Ctx, infrastructure.KeycloakFleetTokenConfig{
				TokenEndpoint:  keycloakTokenEndpointFMC,
				ClientID:       "kubernaut-fleet-read",
				ClientSecret:   "e2e-fleet-secret",
				KubeconfigPath: h.KubeconfigPath,
			})
			Expect(err).ToNot(HaveOccurred(), "failed to acquire subject token from Keycloak")
			Expect(subjectToken).ToNot(BeEmpty())
		})

		It("exchanges the subject token for a k8s-api-audience token that the real API server honors", func() {
			By("performing the RFC 8693 exchange exactly as kube-mcp-server does (client_id=kube-mcp-server, audience=k8s-api)")
			exchangedToken, err := infrastructure.ExchangeKeycloakToken(
				h.KubeconfigPath, keycloakTokenEndpointFMC, "kube-mcp-server", "e2e-kube-mcp-server-secret",
				subjectToken, "k8s-api")
			Expect(err).ToNot(HaveOccurred(), "RFC 8693 token exchange against Keycloak failed")
			Expect(exchangedToken).ToNot(BeEmpty())

			By("presenting the exchanged token directly to the real Kubernetes API server")
			Eventually(func(g Gomega) {
				resp := RawAPIServerRequest(g, "/api/v1/namespaces", exchangedToken)
				defer func() { _ = resp.Body.Close() }()
				g.Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"the exchanged token must be accepted by the API server's OIDC authenticator "+
						"(--oidc-client-id=k8s-api) and authorized by the exchanged-identity RBAC binding "+
						"(keycloak:service-account-kubernaut-fleet-read -> view)")
			}, Timeout, Interval).Should(Succeed())
		})

		It("rejects the un-exchanged subject token when presented directly to the API server (exchange is not a no-op)", func() {
			By("presenting the subject token (audience=kube-mcp-server) directly, skipping the exchange step")
			resp := RawAPIServerRequest(Default, "/api/v1/namespaces", subjectToken)
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
				"a token audienced for kube-mcp-server (not k8s-api) must be rejected by the API server's "+
					"OIDC authenticator -- proving the RFC 8693 exchange step is a real security boundary, "+
					"not an inert passthrough (AC-6)")
		})
	})
}
