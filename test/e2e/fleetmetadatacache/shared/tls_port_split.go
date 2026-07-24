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
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention

	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
)

// TLSPortSplit proves the full production topology for Issue #1683's TLS +
// 3-port standard alignment, in a way no lower tier can:
//   - UT (config_test.go, http_client_test.go) only proves field defaults and
//     the WithHTTPClient option in isolation.
//   - IT (main_wiring_test.go) proves the same wiring against an
//     httptest-managed listener, not a real Kind-deployed pod reached through
//     a real Service/NodePort.
//
// This scenario proves, against the real Kind-deployed FMC pod:
//   - FMC's API port presents a cert issued by the E2E inter-service CA (the
//     harness's CA-aware client -- FMCHTTPClient, backed by
//     infrastructure.NewTLSAwareTransport -- completes a real scope-check
//     call over TLS)
//   - /readyz is unreachable on the API port (moved off it, Issue #1683
//     Section 4.3 design decision) and reachable on the dedicated health port
//   - a client that offers only TLS versions below the active TLSProfile's
//     floor is rejected by the real production listener (SC-13: cipher/
//     version restriction enforcement, not just "TLS is on")
//
// 100% gateway-agnostic: this journey exercises FMC's own server config, never
// the MCP Gateway edge, so it is shared verbatim with the EAIGW lane.
//
// Authority: Issue #1683, docs/testing/1683/TEST_PLAN.md E2E-FMC-1683-016.
func TLSPortSplit(h *Harness, v Variant) bool {
	return Describe(fmt.Sprintf("%s: FMC presents TLS on the API port with readyz split onto a dedicated health port", v.ScenarioPrefix()), func() {
		It("E2E-FMC-1683-016 [SC-8,SC-13,AC-4]: real TLS handshake + readyz port split + cipher-version restriction", func() {
			By("Confirming FMC's real API port completes a CA-verified TLS scope-check call")
			Eventually(func(g Gomega) {
				ScopeCheck(g, h, "loopback-cluster", "", "v1", "Namespace", "", "kube-system")
			}, Timeout, Interval).Should(Succeed(),
				"SC-8: a CA-trusting client must complete real scope-check calls over the TLS-protected API port")

			By("Confirming /readyz is unreachable on the API port (moved exclusively to the health port)")
			apiURL, err := url.Parse(h.FMCAPIBaseURL)
			Expect(err).ToNot(HaveOccurred())
			readyzOnAPIPort := fmt.Sprintf("%s://%s/readyz", apiURL.Scheme, apiURL.Host)
			req, err := http.NewRequestWithContext(h.Ctx, http.MethodGet, readyzOnAPIPort, http.NoBody)
			Expect(err).ToNot(HaveOccurred())
			resp, respErr := h.FMCHTTPClient.Do(req)
			if respErr == nil {
				defer func() { _ = resp.Body.Close() }()
				Expect(resp.StatusCode).ToNot(Equal(http.StatusOK),
					"AC-4: /readyz must not be served on the API port after the Issue #1683 3-port split")
			}
			// A transport-level error (connection refused/reset, TLS alert) is
			// an equally valid proof that /readyz isn't served here -- the API
			// mux never registers that handler at all post-split.

			By("Confirming /readyz IS reachable and healthy on the dedicated health port")
			Eventually(func(g Gomega) int {
				return ReadyzStatus(g, h)
			}, Timeout, Interval).Should(Equal(http.StatusOK),
				"AC-4: /readyz must be served on FMC's dedicated health port")

			By("Confirming a client offering only a below-floor TLS version is rejected (SC-13)")
			restrictedClient := &http.Client{
				Timeout: 5 * time.Second,
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						MaxVersion:         tls.VersionTLS11, //nolint:gosec // deliberate: proving the server rejects this floor
						InsecureSkipVerify: true,             //nolint:gosec // handshake version is under test, not cert trust
					},
					DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
				},
			}
			downgradedReq, err := http.NewRequestWithContext(h.Ctx, http.MethodGet, h.FMCAPIBaseURL+fmc.ClustersPath, http.NoBody)
			Expect(err).ToNot(HaveOccurred())
			downgradedResp, downgradedErr := restrictedClient.Do(downgradedReq)
			if downgradedResp != nil {
				_ = downgradedResp.Body.Close()
			}
			Expect(downgradedErr).To(HaveOccurred(),
				"SC-13: FMC's active TLSProfile must reject a handshake offering only TLS 1.1 or below")
		})
	})
}
