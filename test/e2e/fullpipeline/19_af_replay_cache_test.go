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

package fullpipeline

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2E-FP-AF-1999: source-bound jti replay-cache reuse against a real deployed
// chart (real Valkey, real AF pod), reproducing a confirmed production
// incident: with the distributed replay cache enabled and the old
// Seen(jti)-only semantics, ANY second presentation of the same JWT (the
// normal way an OAuth2 Bearer token is used across multiple requests) was
// wrongly rejected as a replay. The fix (#1999, DD-PLATFORM-006 DA18) makes
// replay detection source-bound: Seen(jti, sourceKey) only flags a replay
// when the *same* jti arrives from a *different* source.
//
// Deliberately targets GET /a2a/access (#1919, ConsoleAccessHandler) rather
// than /mcp: both routes pass through the identical AuthMiddleware chain
// (pkg/apifrontend/handler/router.go's a2aChain/mcpChain both wrap
// cfg.AuthMiddleware), so this exercises the exact same JWTValidator/
// ReplayCacheStore code path #1999 fixed -- but /a2a/access is a lightweight
// SAR-backed check with no MCP session semantics, avoiding a collision with
// 06_af_audit_trace_test.go's E2E-FP-AF-001, which depends on being the
// first-ever session-less request to AF's real /mcp endpoint in the whole
// suite run to observe a one-time apifrontend.mcp.session_init audit event
// (pkg/apifrontend/handler/mcp.go's seenSessions sentinel is keyed on the
// absence of an Mcp-Session-Id header, so it can only ever fire once per AF
// process lifetime -- a separate, pre-existing gap, not this PR's concern).
//
// Prerequisite: AF deployed in the FP cluster with
// apifrontend.config.auth.replayCache.enabled=true against the chart's own
// Valkey (test/infrastructure/fullpipeline_e2e_helm.go). Every other AF test
// in this suite that calls getAFToken() (suite_test.go, cached per-process)
// already exercises this same-source-reuse path implicitly; this test makes
// the assertion explicit and named so a regression fails clearly instead of
// diffusely across unrelated specs.
//
// BR: BR-SECURITY-1505 (SOC2 CC6.1/CC7.2, FedRAMP AC-6/SI-10)
var _ = Describe("E2E-FP-AF-1999: source-bound jti replay-cache reuse (#1999, BR-SECURITY-1505)",
	Label("e2e", "fullpipeline", "apifrontend", "security", "replay-cache"), func() {

		BeforeEach(func() {
			resp, err := afHTTPClient.Get(afBaseURL + "/healthz")
			if err != nil || resp.StatusCode != http.StatusOK {
				Skip("AF not deployed in FP cluster (Issue #1189 prerequisite)")
			}
			_ = resp.Body.Close()
		})

		accessCheck := func(token string) int {
			req, err := http.NewRequest(http.MethodGet, afBaseURL+"/a2a/access", http.NoBody)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := afHTTPClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
		}

		It("E2E-FP-AF-1999-001: the same client reusing its Bearer token across independent authenticated requests is never rejected as a replay", func() {
			token := getAFToken()
			Expect(token).NotTo(BeEmpty())

			By("First authenticated request with this token is not rejected as unauthorized")
			code1 := accessCheck(token)
			Expect(code1).NotTo(Equal(http.StatusUnauthorized), "first use of the token must not be rejected as unauthorized")

			By("Reusing the exact same token for a second, independent request")
			code2 := accessCheck(token)
			Expect(code2).NotTo(Equal(http.StatusUnauthorized),
				"reusing the same Bearer token from the same client must NOT be rejected as a replay "+
					"(#1999) -- this exact scenario was a confirmed production incident before the fix")

			By("A third reuse also succeeds, ruling out a one-shot fluke")
			code3 := accessCheck(token)
			Expect(code3).NotTo(Equal(http.StatusUnauthorized), "third reuse of the token must also not be rejected as a replay")
		})
	})
