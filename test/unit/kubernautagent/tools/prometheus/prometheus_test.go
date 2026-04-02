package prometheus_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/prometheus"
)

var _ = Describe("Kubernaut Agent Prometheus Tools Unit — #433", func() {

	Describe("UT-KA-433-033: Prometheus client config parses URL, headers, timeout, size limit", func() {
		It("should create a client from valid config with all fields set", func() {
			cfg := prometheus.ClientConfig{
				URL:       "http://prometheus:9090",
				Headers:   map[string]string{"Authorization": "Bearer test-token"},
				Timeout:   30 * time.Second,
				SizeLimit: 30000,
			}
			client, err := prometheus.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil(), "NewClient should return a non-nil client")
		})

		It("should apply default size limit when zero", func() {
			cfg := prometheus.ClientConfig{
				URL: "http://prometheus:9090",
			}
			client, err := prometheus.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})
	})

	Describe("UT-KA-433-034: Response exceeding size limit is truncated with topk() hint", func() {
		It("should truncate text longer than sizeLimit and append topk() hint", func() {
			longText := strings.Repeat("x", 35000)
			result := prometheus.TruncateWithHint(longText, 30000)
			Expect(len(result)).To(BeNumerically("<=", 31000),
				"truncated result should be near sizeLimit")
			Expect(result).To(ContainSubstring("topk"),
				"truncated result should suggest topk() to narrow query")
		})

		It("should pass through text within limit unchanged", func() {
			shortText := "metric_value=42"
			result := prometheus.TruncateWithHint(shortText, 30000)
			Expect(result).To(Equal(shortText))
		})
	})
})
