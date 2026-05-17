package e2e_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// counterValue returns the latest sample value for an unlabelled Prometheus counter (e.g. af_http_panics_total).
func counterValue(metricsBody, metricName string) float64 {
	var v float64
	found := false
	for _, line := range strings.Split(metricsBody, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, metricName+" ") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			var parsed float64
			if _, err := fmt.Sscanf(fields[1], "%f", &parsed); err == nil {
				v = parsed
				found = true
			}
		}
	}
	if !found {
		return 0
	}
	return v
}


var _ = Describe("Resilience and Operational (G17/G20/G9/G10/G11)", Ordered, ContinueOnFailure, Label("e2e", "phase5-6"), func() {

	Context("TC-E2E-PANIC-01 (G17)", func() {
		It("POST /debug/panic returns 500 problem+json and increments af_http_panics_total", func() {
			before := counterValue(scrapeMetrics(), "af_http_panics_total")

			req, err := http.NewRequest(http.MethodPost, baseURL+"/debug/panic", http.NoBody)
			Expect(err).NotTo(HaveOccurred())
			resp, err := httpClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode == http.StatusNotFound {
				Skip("/debug/panic not registered — AF image likely built without -tags e2e")
			}

			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
			Expect(strings.ToLower(resp.Header.Get("Content-Type"))).To(ContainSubstring("application/problem+json"))
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.ToLower(string(body))).To(Or(
				ContainSubstring("internal server error"),
				ContainSubstring("service error"),
			))

			Eventually(func() float64 {
				return counterValue(scrapeMetrics(), "af_http_panics_total")
			}, 30*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", before+1),
				"af_http_panics_total should increase by at least 1 after handled panic")
		})
	})


})
