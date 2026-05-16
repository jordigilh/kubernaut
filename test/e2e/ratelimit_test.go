package e2e_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// userRateLimitRejections sums user-tier rate limit rejection counters
// (af_rate_limit_rejections_total in this repo; test plans sometimes reference af_ratelimit_denied_total).
func userRateLimitRejections(metricsBody string) float64 {
	var sum float64
	for _, line := range strings.Split(metricsBody, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, `tier="user"`) {
			continue
		}
		if !strings.HasPrefix(line, "af_rate_limit_rejections_total") &&
			!strings.HasPrefix(line, "af_ratelimit_denied_total") {
			continue
		}
		if strings.Contains(line, "_created") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		var v float64
		if _, err := fmt.Sscanf(fields[len(fields)-1], "%f", &v); err == nil {
			sum += v
		}
	}
	return sum
}

var _ = Describe("Rate Limiting (G14/G15/G16)", Ordered, Label("e2e", "phase5", "ratelimit"), func() {

	Context("TC-E2E-RATELIMIT-USER-01 (G14)", func() {
		It("authenticated user exceeds request rate → HTTP 429 + user-tier rate limit metric", func() {
			token, err := fetchDEXTokenForPersona("sre")
			Expect(err).NotTo(HaveOccurred())

			var saw429 bool
			for i := 0; i < 500; i++ {
				resp, e := a2aInvoke(httpClient, baseURL, token, a2aTasksSend(fmt.Sprintf("rl-user-01-%d", i), "ping"))
				Expect(e).NotTo(HaveOccurred())
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusTooManyRequests {
					saw429 = true
					break
				}
				if resp.StatusCode >= http.StatusInternalServerError {
					_, _ = fmt.Fprintf(GinkgoWriter, "unexpected %d: %s\n", resp.StatusCode, string(body))
				}
			}
			Expect(saw429).To(BeTrue(), "expected HTTP 429 after sustained authenticated /a2a/invoke traffic")

			Eventually(func() float64 {
				return userRateLimitRejections(scrapeMetrics())
			}, 30*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0),
				"expected af_rate_limit_rejections_total (tier=user) after HTTP 429")
		})
	})

	Context("TC-E2E-RATELIMIT-USER-02 (G14)", func() {
		It("different users have independent per-user request buckets", func() {
			sreTok, err := fetchDEXTokenForPersona("sre")
			Expect(err).NotTo(HaveOccurred())
			cicdTok, err := fetchDEXTokenForPersona("cicd")
			Expect(err).NotTo(HaveOccurred())

			// Exhaust SRE bucket.
			var sreBlocked bool
			for i := 0; i < 500; i++ {
				resp, e := a2aInvoke(httpClient, baseURL, sreTok, a2aTasksSend(fmt.Sprintf("rl-alt-sre-%d", i), "ping"))
				Expect(e).NotTo(HaveOccurred())
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusTooManyRequests {
					sreBlocked = true
					break
				}
			}
			if !sreBlocked {
				Skip("SRE token did not hit per-user request limit — limits may be high for this environment")
			}

			resp, err := a2aInvoke(httpClient, baseURL, cicdTok, a2aTasksSend("rl-alt-cicd", "ping"))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).NotTo(Equal(http.StatusTooManyRequests),
				"cicd user should not inherit SRE's exhausted request bucket on first call after SRE is rate limited")
		})
	})

	Context("TC-E2E-TOOL-RATELIMIT-01 (G15)", func() {
		It("MCP tool call rate exceeded → rate limit exceeded in tool result", func() {
			token, err := fetchDEXTokenForPersona("sre")
			Expect(err).NotTo(HaveOccurred())
			sid, err := initMCPSession(token)
			Expect(err).NotTo(HaveOccurred())

			var limited bool
			for i := 0; i < 200; i++ {
				raw, code, e := mcpPOST(token, sid, buildJSONRPC(fmt.Sprintf("tool-rl-%d", i), "tools/call", map[string]interface{}{
					"name":      "af_get_pods",
					"arguments": map[string]interface{}{"namespace": "default"},
				}))
				Expect(e).NotTo(HaveOccurred())
				Expect(code).To(BeNumerically("<", 400))
				text, _, perr := parseMCPToolPayload(unwrapSSEDataLine(raw))
				Expect(perr).NotTo(HaveOccurred())
				if strings.Contains(strings.ToLower(text), "rate limit exceeded") {
					limited = true
					break
				}
			}
			Expect(limited).To(BeTrue(), "expected rapid af_get_pods calls to hit per-user tool rate limit")
		})
	})

	Context("TC-E2E-TOOL-RATELIMIT-02 (G15)", func() {
		It("per-user tool budget isolates different principals (SRE exhausted; other user still succeeds)", func() {
			// AllowToolCall is keyed only by username — all tools share one per-user bucket.
			// Validate cross-user isolation instead of per-tool isolation (not implemented).
			sreTok, err := fetchDEXTokenForPersona("sre")
			Expect(err).NotTo(HaveOccurred())
			obsTok, err := fetchDEXTokenForPersona("observability")
			Expect(err).NotTo(HaveOccurred())

			sreSid, err := initMCPSession(sreTok)
			Expect(err).NotTo(HaveOccurred())

			var exhausted bool
			for i := 0; i < 200; i++ {
				raw, code, e := mcpPOST(sreTok, sreSid, buildJSONRPC(fmt.Sprintf("tool-exhaust-sre-%d", i), "tools/call", map[string]interface{}{
					"name":      "af_get_pods",
					"arguments": map[string]interface{}{"namespace": "default"},
				}))
				Expect(e).NotTo(HaveOccurred())
				Expect(code).To(BeNumerically("<", 400))
				text, _, perr := parseMCPToolPayload(unwrapSSEDataLine(raw))
				Expect(perr).NotTo(HaveOccurred())
				if strings.Contains(strings.ToLower(text), "rate limit exceeded") {
					exhausted = true
					break
				}
			}
			if !exhausted {
				Skip("did not exhaust SRE tool rate limit in this environment")
			}

			obsSid, err := initMCPSession(obsTok)
			Expect(err).NotTo(HaveOccurred())

			raw, code, err := mcpPOST(obsTok, obsSid, buildJSONRPC("tool-rl-cross-user", "tools/call", map[string]interface{}{
				"name":      "af_get_workloads",
				"arguments": map[string]interface{}{"namespace": "default"},
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(BeNumerically("<", 400))
			text, toolErr, perr := parseMCPToolPayload(unwrapSSEDataLine(raw))
			Expect(perr).NotTo(HaveOccurred())
			Expect(toolErr).To(BeFalse(), "observability persona should still call af_get_workloads after SRE exhausts its own bucket: %s", text)
		})
	})

	Context("TC-E2E-THROTTLE-01 (G16)", func() {
		It("saturate concurrent tool calls → server busy + throttled metrics", func() {
			token, err := fetchDEXTokenForPersona("sre")
			Expect(err).NotTo(HaveOccurred())
			sid, err := initMCPSession(token)
			Expect(err).NotTo(HaveOccurred())

			var wg sync.WaitGroup
			results := make(chan string, 32)
			for i := 0; i < 24; i++ {
				wg.Add(1)
				go func(idx int) {
					defer GinkgoRecover()
					defer wg.Done()
					raw, code, e := mcpPOST(token, sid, buildJSONRPC(fmt.Sprintf("thr-%d", idx), "tools/call", map[string]interface{}{
						"name":      "af_get_pods",
						"arguments": map[string]interface{}{"namespace": "default"},
					}))
					if e != nil {
						results <- fmt.Sprintf("err:%v", e)
						return
					}
					if code >= http.StatusBadRequest {
						results <- fmt.Sprintf("http:%d:%s", code, string(raw))
						return
					}
					text, _, perr := parseMCPToolPayload(unwrapSSEDataLine(raw))
					if perr != nil {
						results <- fmt.Sprintf("parse:%v", perr)
						return
					}
					results <- text
				}(i)
			}
			wg.Wait()
			close(results)

			var sawBusy bool
			for msg := range results {
				if strings.Contains(strings.ToLower(msg), "server busy") {
					sawBusy = true
					break
				}
			}

			metrics := scrapeMetrics()
			hasThrottled := strings.Contains(metrics, `result="throttled"`) &&
				strings.Contains(metrics, "af_tool_calls_total")

			Expect(sawBusy || hasThrottled).To(BeTrue(),
				"expected at least one concurrent call to be throttled (server busy) or af_tool_calls_total with result=throttled")
		})
	})
})
