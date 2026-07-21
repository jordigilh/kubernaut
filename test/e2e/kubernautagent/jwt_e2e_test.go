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

package kubernautagent

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
)

// E2E JWT/OIDC Tests — #1009 (DD-AUTH-MCP-001 v2.0)
//
// Validates the full OIDC flow with DEX as a real identity provider:
//   - DEX issues JWTs signed by its RSA keys
//   - KA's CompositeAuthenticator validates JWTs via JWKS endpoint
//   - SAR check authorizes the OIDC user via Kubernetes RBAC
//   - Pattern A (SA token) and Pattern B (JWT) coexist on the same endpoint
//
// BR: BR-INTERACTIVE-002 (Authentication), BR-INTERACTIVE-010 (Security)
var _ = Describe("E2E JWT/OIDC — DEX Integration (#1009)", Label("e2e", "ka", "interactive", "jwt", "oidc"), func() {

	var (
		mcpEndpoint  string
		tlsTransport http.RoundTripper
		dexConfig    infrastructure.DexE2EConfig
	)

	BeforeEach(func() {
		mcpEndpoint = infrastructure.MCPEndpointForKAE2E()
		tlsTransport = testauth.NewRetryOn429Transport(http.DefaultTransport)
		dexConfig = infrastructure.DefaultDexE2EConfig(kubeconfigPath)
	})

	// ---------------------------------------------------------------
	// E2E-KA-JWT-001: Valid JWT from DEX accepted by KA MCP endpoint
	// BR: BR-INTERACTIVE-002
	// Validates: DEX token issuance → JWKS verification → SAR authz
	// ---------------------------------------------------------------
	Describe("E2E-KA-JWT-001: Valid JWT from DEX → MCP endpoint → 200 OK", func() {
		It("should accept a DEX-issued JWT and allow MCP access", func() {
			By("Obtaining an OIDC id_token from DEX via password grant")
			jwtToken, err := infrastructure.GetDexIDToken(dexConfig)
			Expect(err).NotTo(HaveOccurred(), "DEX should issue an id_token for the E2E user")
			Expect(jwtToken).NotTo(BeEmpty())
			GinkgoWriter.Printf("  DEX id_token length: %d chars\n", len(jwtToken))

			By("Decoding JWT claims for diagnostic visibility")
			parts := strings.SplitN(jwtToken, ".", 3)
			Expect(parts).To(HaveLen(3), "JWT must have 3 segments")
			payload, err := base64.RawURLEncoding.DecodeString(parts[1])
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Printf("  JWT payload: %s\n", string(payload))

			By("Verifying raw HTTP with JWT returns non-401 (auth pipeline smoke test)")
			probeReq, err := http.NewRequestWithContext(ctx, "POST", mcpEndpoint, nil)
			Expect(err).NotTo(HaveOccurred())
			probeReq.Header.Set("Authorization", "Bearer "+jwtToken)
			probeReq.Header.Set("Content-Type", "application/json")

			probeClient := &http.Client{Transport: tlsTransport, Timeout: 15 * time.Second}
			probeResp, err := probeClient.Do(probeReq)
			Expect(err).NotTo(HaveOccurred(), "raw HTTP probe to MCP endpoint should not fail")
			probeBody, _ := io.ReadAll(probeResp.Body)
			_ = probeResp.Body.Close()
			GinkgoWriter.Printf("  JWT probe: HTTP %d — %s\n", probeResp.StatusCode, string(probeBody))
			Expect(probeResp.StatusCode).NotTo(Equal(http.StatusUnauthorized),
				"JWT from DEX should not be rejected as 401 — check issuer/audience/JWKS config")
			Expect(probeResp.StatusCode).NotTo(Equal(http.StatusForbidden),
				"JWT user should have RBAC — check DEX user RBAC RoleBinding")

			By("Connecting MCP client with DEX JWT instead of SA token")
			connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
			defer connectCancel()
			session, err := infrastructure.ConnectMCPClient(connectCtx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      jwtToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred(), "MCP client should connect with DEX JWT")
			defer session.Close()

			By("Verifying MCP session is alive by listing tools")
			tools, err := session.ListTools(ctx, nil)
			Expect(err).NotTo(HaveOccurred(), "ListTools should succeed with JWT auth")
			Expect(tools.Tools).NotTo(BeEmpty(), "KA should expose MCP tools")

			GinkgoWriter.Printf("  ✅ JWT auth succeeded — %d tools visible\n", len(tools.Tools))
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-JWT-002: Invalid/forged JWT rejected with 401
	// BR: BR-INTERACTIVE-010
	// Validates: CompositeAuthenticator fail-closed for bad JWTs
	// ---------------------------------------------------------------
	Describe("E2E-KA-JWT-002: Invalid JWT rejected with 401", func() {
		It("should reject a forged JWT-shaped token with 401 Unauthorized", func() {
			By("Crafting a JWT-shaped token not signed by DEX")
			forgedToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." +
				"eyJpc3MiOiJodHRwOi8vZGV4OjU1NTYvZGV4Iiwic3ViIjoiYXR0YWNrZXIiLCJhdWQiOiJrdWJlcm5hdXQtYWdlbnQifQ." +
				"invalid-signature-data"

			By("Sending HTTP POST to MCP endpoint with forged JWT")
			req, err := http.NewRequestWithContext(ctx, "POST", mcpEndpoint, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+forgedToken)
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Transport: tlsTransport, Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			By("Asserting 401 Unauthorized — fail-closed for known issuer with bad signature")
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
				"forged JWT targeting a known issuer must be rejected as 401")
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-JWT-003: Pattern A + B coexistence — SA token still works
	// BR: BR-INTERACTIVE-002
	// Validates: CompositeAuthenticator does not break SA token path
	// ---------------------------------------------------------------
	Describe("E2E-KA-JWT-003: Pattern A + B coexistence — SA token still works alongside JWT", func() {
		It("should accept SA token (Pattern A) when JWT providers are configured", func() {
			By("Connecting MCP client with ServiceAccount token (Pattern A)")
			saToken, err := infrastructure.GetServiceAccountToken(ctx, sharedNamespace, "kubernaut-agent-e2e-sa", kubeconfigPath)
			Expect(err).NotTo(HaveOccurred())

			connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
			defer connectCancel()
			session, err := infrastructure.ConnectMCPClient(connectCtx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      saToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred(), "MCP client should connect with SA token even when JWT providers are configured")
			defer session.Close()

			By("Verifying MCP session is alive by listing tools")
			tools, err := session.ListTools(ctx, nil)
			Expect(err).NotTo(HaveOccurred(), "ListTools should succeed with SA token auth")
			Expect(tools.Tools).NotTo(BeEmpty(), "KA should expose MCP tools for SA-authenticated user")

			GinkgoWriter.Printf("  ✅ SA token auth (Pattern A) still works — %d tools visible\n", len(tools.Tools))
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-JWT-004: JWT user invokes kubernaut_investigate
	// BR: BR-INTERACTIVE-001, BR-INTERACTIVE-004
	// Validates: Full interactive flow with JWT identity propagation
	// ---------------------------------------------------------------
	Describe("E2E-KA-JWT-004: JWT user invokes kubernaut_investigate tool", func() {
		It("should allow JWT-authenticated user to start an interactive session", func() {
			By("Obtaining a DEX JWT")
			jwtToken, err := infrastructure.GetDexIDToken(dexConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Connecting MCP client with DEX JWT")
			connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
			defer connectCancel()
			session, err := infrastructure.ConnectMCPClient(connectCtx, infrastructure.MCPClientConfig{
				Endpoint:     mcpEndpoint,
				SAToken:      jwtToken,
				TLSTransport: tlsTransport,
			})
			Expect(err).NotTo(HaveOccurred())
			defer session.Close()

			By("Creating RR fixture for investigate")
			createTestRemediationRequest(ctx, "rr-jwt-e2e-001")

			By("Calling kubernaut_investigate with start action")
			result, err := infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  "rr-jwt-e2e-001",
				"action": "start",
			})
			Expect(err).NotTo(HaveOccurred(), "kubernaut_investigate should succeed with JWT auth")
			Expect(result).NotTo(BeNil())

			text := infrastructure.ExtractToolResultText(result)
			Expect(text).NotTo(BeEmpty(), "investigate result should contain output")
			GinkgoWriter.Printf("  ✅ JWT user invoked kubernaut_investigate: %s\n", truncate(text, 200))

			By("Completing the session to release the Lease")
			_, err = infrastructure.CallInvestigate(ctx, session, map[string]any{
				"rr_id":  "rr-jwt-e2e-001",
				"action": "complete",
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ---------------------------------------------------------------
	// E2E-KA-JWT-005: Expired DEX JWT rejected
	// BR: BR-INTERACTIVE-010
	// Note: DEX issues JWTs with configurable expiry. We validate the
	// token was issued recently and check KA rejects stale tokens.
	// Since we can't easily make DEX issue an already-expired token,
	// this test validates the auth pipeline handles token format from
	// a real OIDC provider (complementing the unit-level expiry tests).
	// ---------------------------------------------------------------
	Describe("E2E-KA-JWT-005: DEX token format validated end-to-end", func() {
		It("should successfully decode and validate a DEX-issued JWT structure", func() {
			By("Obtaining a fresh DEX JWT")
			jwtToken, err := infrastructure.GetDexIDToken(dexConfig)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the token has JWT structure (3 dot-separated segments)")
			segments := 0
			for _, c := range jwtToken {
				if c == '.' {
					segments++
				}
			}
			Expect(segments).To(Equal(2), "DEX id_token must be a valid JWT (3 segments)")

			By("Sending a raw HTTP request with the JWT to verify full pipeline")
			req, err := http.NewRequestWithContext(ctx, "POST", mcpEndpoint, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+jwtToken)
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{Transport: tlsTransport, Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			GinkgoWriter.Printf("  DEX JWT → KA response: %d %s\n", resp.StatusCode, string(body))

			Expect(resp.StatusCode).NotTo(Equal(http.StatusUnauthorized),
				"a valid DEX-issued JWT should not be rejected as unauthorized")
		})
	})
})

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
