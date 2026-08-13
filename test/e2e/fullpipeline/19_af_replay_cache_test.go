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
	"fmt"
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

		It("E2E-FP-AF-1999-001: the same client reusing its Bearer token across independent MCP sessions is never rejected as a replay", func() {
			token := getAFToken()
			Expect(token).NotTo(BeEmpty())

			By("First MCP handshake with this token succeeds")
			sessionID1, err := fpInitMCPSessionExplicit(afHTTPClient, afBaseURL, token)
			Expect(err).NotTo(HaveOccurred(), "first use of the token must succeed")
			Expect(sessionID1).NotTo(BeEmpty())

			By("Reusing the exact same token for a second, independent MCP handshake")
			sessionID2, err := fpInitMCPSessionExplicit(afHTTPClient, afBaseURL, token)
			Expect(err).NotTo(HaveOccurred(),
				"reusing the same Bearer token from the same client must NOT be rejected as a replay "+
					"(#1999) -- this exact scenario was a confirmed production incident before the fix")
			Expect(sessionID2).NotTo(BeEmpty())
			Expect(sessionID2).NotTo(Equal(sessionID1),
				"each handshake mints a fresh MCP session even though the underlying JWT is reused")

			By("A third reuse also succeeds, ruling out a one-shot fluke")
			sessionID3, err := fpInitMCPSessionExplicit(afHTTPClient, afBaseURL, token)
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("third reuse of the token must also succeed: %v", err))
			Expect(sessionID3).NotTo(BeEmpty())
		})
	})
