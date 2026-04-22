package prometheus_test

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
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

	Describe("UT-KA-433-190: AllToolNames includes 8 Prometheus tools", func() {
		It("should list 8 Prometheus tool names", func() {
			Expect(prometheus.AllToolNames).To(HaveLen(8))
			Expect(prometheus.AllToolNames).To(ContainElement("execute_prometheus_instant_query"))
			Expect(prometheus.AllToolNames).To(ContainElement("execute_prometheus_range_query"))
			Expect(prometheus.AllToolNames).To(ContainElement("get_metric_names"))
			Expect(prometheus.AllToolNames).To(ContainElement("get_label_values"))
			Expect(prometheus.AllToolNames).To(ContainElement("get_all_labels"))
			Expect(prometheus.AllToolNames).To(ContainElement("get_metric_metadata"))
			Expect(prometheus.AllToolNames).To(ContainElement("list_prometheus_rules"))
			Expect(prometheus.AllToolNames).To(ContainElement("get_series"))
		})
	})

	Describe("UT-KA-433-191: All Prometheus tools have non-nil parameter schemas", func() {
		It("should return non-nil schema for each tool", func() {
			cfg := prometheus.ClientConfig{URL: "http://prometheus:9090"}
			client, err := prometheus.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			allTools := prometheus.NewAllTools(client)
			Expect(allTools).To(HaveLen(8))

			for _, tool := range allTools {
				params := tool.Parameters()
				Expect(params).NotTo(BeNil(), "tool %s should have non-nil schema", tool.Name())

				var schema map[string]interface{}
				Expect(json.Unmarshal(params, &schema)).To(Succeed(),
					"tool %s schema should be valid JSON", tool.Name())
			}
		})
	})

	Describe("UT-KA-433-192: list_prometheus_rules tool exists with correct schema", func() {
		It("should have type:object schema", func() {
			cfg := prometheus.ClientConfig{URL: "http://prometheus:9090"}
			client, err := prometheus.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			allTools := prometheus.NewAllTools(client)
			var rulesTool tools.Tool
			for _, t := range allTools {
				if t.Name() == "list_prometheus_rules" {
					rulesTool = t
					break
				}
			}
			Expect(rulesTool).NotTo(BeNil(), "list_prometheus_rules tool should exist")

			var schema map[string]interface{}
			Expect(json.Unmarshal(rulesTool.Parameters(), &schema)).To(Succeed())
			Expect(schema["type"]).To(Equal("object"))
		})
	})

	Describe("UT-KA-433-193: get_series tool exists with correct schema", func() {
		It("should have required match[] parameter", func() {
			cfg := prometheus.ClientConfig{URL: "http://prometheus:9090"}
			client, err := prometheus.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			allTools := prometheus.NewAllTools(client)
			var seriesTool tools.Tool
			for _, t := range allTools {
				if t.Name() == "get_series" {
					seriesTool = t
					break
				}
			}
			Expect(seriesTool).NotTo(BeNil(), "get_series tool should exist")

			var schema map[string]interface{}
			Expect(json.Unmarshal(seriesTool.Parameters(), &schema)).To(Succeed())
			Expect(schema["type"]).To(Equal("object"))

			required, ok := schema["required"].([]interface{})
			Expect(ok).To(BeTrue(), `expected "required" in get_series schema to be a []interface{}`)
			Expect(required).To(ContainElement("match"))
		})
	})

	Describe("UT-KA-433-194: get_series rejects empty match with descriptive error", func() {
		It("should return error when match is empty string", func() {
			cfg := prometheus.ClientConfig{URL: "http://prometheus:9090"}
			client, err := prometheus.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			allTools := prometheus.NewAllTools(client)
			var seriesTool tools.Tool
			for _, t := range allTools {
				if t.Name() == "get_series" {
					seriesTool = t
					break
				}
			}
			Expect(seriesTool).NotTo(BeNil())

			_, err = seriesTool.Execute(context.Background(), json.RawMessage(`{"match":""}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("match"))
		})

		It("should return error when match is omitted", func() {
			cfg := prometheus.ClientConfig{URL: "http://prometheus:9090"}
			client, err := prometheus.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			allTools := prometheus.NewAllTools(client)
			var seriesTool tools.Tool
			for _, t := range allTools {
				if t.Name() == "get_series" {
					seriesTool = t
					break
				}
			}
			Expect(seriesTool).NotTo(BeNil())

			_, err = seriesTool.Execute(context.Background(), json.RawMessage(`{}`))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("match"))
		})
	})

	Describe("UT-KA-433-195: ClientConfig defaults MetadataLimit and MetadataTimeWindowHrs", func() {
		It("should default MetadataLimit to 100 when zero", func() {
			cfg := prometheus.ClientConfig{URL: "http://prometheus:9090"}
			client, err := prometheus.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client.Config().MetadataLimit).To(Equal(100))
		})

		It("should default MetadataTimeWindowHrs to 1 when zero", func() {
			cfg := prometheus.ClientConfig{URL: "http://prometheus:9090"}
			client, err := prometheus.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client.Config().MetadataTimeWindowHrs).To(Equal(1))
		})

		It("should preserve explicit values", func() {
			cfg := prometheus.ClientConfig{
				URL:                   "http://prometheus:9090",
				MetadataLimit:         50,
				MetadataTimeWindowHrs: 24,
			}
			client, err := prometheus.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(client.Config().MetadataLimit).To(Equal(50))
			Expect(client.Config().MetadataTimeWindowHrs).To(Equal(24))
		})
	})

	Describe("UT-KA-433-196: get_series schema declares start and end as optional", func() {
		It("should have start and end properties but not in required", func() {
			cfg := prometheus.ClientConfig{URL: "http://prometheus:9090"}
			client, err := prometheus.NewClient(cfg)
			Expect(err).NotTo(HaveOccurred())

			allTools := prometheus.NewAllTools(client)
			var seriesTool tools.Tool
			for _, t := range allTools {
				if t.Name() == "get_series" {
					seriesTool = t
					break
				}
			}
			Expect(seriesTool).NotTo(BeNil())

			var schema map[string]interface{}
			Expect(json.Unmarshal(seriesTool.Parameters(), &schema)).To(Succeed())

			props, ok := schema["properties"].(map[string]interface{})
			Expect(ok).To(BeTrue(), `expected "properties" in get_series schema to be a map[string]interface{}`)
			Expect(props).To(HaveKey("start"), "schema should declare start property")
			Expect(props).To(HaveKey("end"), "schema should declare end property")
			Expect(props).To(HaveKey("match"), "schema should declare match property")

			required, ok := schema["required"].([]interface{})
			Expect(ok).To(BeTrue(), `expected "required" in get_series schema to be a []interface{}`)
			Expect(required).To(ContainElement("match"))
			Expect(required).NotTo(ContainElement("start"), "start should be optional")
			Expect(required).NotTo(ContainElement("end"), "end should be optional")
		})
	})
})
