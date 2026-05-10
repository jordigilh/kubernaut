package prometheus_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	prom "github.com/jordigilh/kubernaut-apifrontend/internal/prometheus"
)

var _ = Describe("PromQL Label Extraction and Matching", func() {

	Describe("ExtractLabelMatchers", func() {
		It("UT-AF-T-013: extracts from simple selector up{job=\"foo\"}", func() {
			matchers, err := prom.ExtractLabelMatchers(`up{job="foo"}`)
			Expect(err).NotTo(HaveOccurred())
			Expect(matchers).NotTo(BeEmpty())
			found := false
			for _, m := range matchers {
				if m.Name == "job" && m.Value == "foo" {
					found = true
				}
			}
			Expect(found).To(BeTrue(), "expected matcher for job=foo")
		})

		It("UT-AF-T-014: extracts from rate with label selector", func() {
			matchers, err := prom.ExtractLabelMatchers(`rate(http_requests_total{namespace="prod"}[5m]) > 100`)
			Expect(err).NotTo(HaveOccurred())
			found := false
			for _, m := range matchers {
				if m.Name == "namespace" && m.Value == "prod" {
					found = true
				}
			}
			Expect(found).To(BeTrue(), "expected matcher for namespace=prod")
		})

		It("UT-AF-T-015: extracts from absent() inner expression", func() {
			matchers, err := prom.ExtractLabelMatchers(`absent(up{job="myapp"})`)
			Expect(err).NotTo(HaveOccurred())
			found := false
			for _, m := range matchers {
				if m.Name == "job" && m.Value == "myapp" {
					found = true
				}
			}
			Expect(found).To(BeTrue(), "expected matcher for job=myapp inside absent()")
		})

		It("UT-AF-T-016: extracts from aggregation sum by (namespace)", func() {
			matchers, err := prom.ExtractLabelMatchers(`sum by (namespace) (rate(container_cpu_usage_seconds_total{namespace="monitoring"}[5m]))`)
			Expect(err).NotTo(HaveOccurred())
			found := false
			for _, m := range matchers {
				if m.Name == "namespace" && m.Value == "monitoring" {
					found = true
				}
			}
			Expect(found).To(BeTrue(), "expected matcher for namespace=monitoring in aggregation")
		})

		It("UT-AF-T-017: extracts from subquery expression", func() {
			matchers, err := prom.ExtractLabelMatchers(`max_over_time(rate(http_requests_total{job="api"}[5m])[1h:5m])`)
			Expect(err).NotTo(HaveOccurred())
			found := false
			for _, m := range matchers {
				if m.Name == "job" && m.Value == "api" {
					found = true
				}
			}
			Expect(found).To(BeTrue(), "expected matcher for job=api in subquery")
		})

		It("UT-AF-T-018: invalid PromQL returns error (no panic)", func() {
			_, err := prom.ExtractLabelMatchers(`invalid{{{`)
			Expect(err).To(HaveOccurred())
		})

		It("UT-AF-T-081: PromQL with Unicode characters does not panic", func() {
			matchers, err := prom.ExtractLabelMatchers(`up{job="日本語"}`)
			// May succeed (Unicode is valid in label values) or error, but must not panic
			if err == nil {
				Expect(matchers).NotTo(BeNil())
			}
		})
	})

	Describe("MatchesResource", func() {
		It("UT-AF-T-019: returns true when all matchers match resource labels", func() {
			matchers, err := prom.ExtractLabelMatchers(`up{namespace="prod",job="web"}`)
			Expect(err).NotTo(HaveOccurred())

			resourceLabels := map[string]string{
				"namespace": "prod",
				"job":       "web",
				"extra":     "ignored",
			}
			Expect(prom.MatchesResource(matchers, resourceLabels)).To(BeTrue())
		})

		It("UT-AF-T-020: returns false on partial match", func() {
			matchers, err := prom.ExtractLabelMatchers(`up{namespace="prod",job="web"}`)
			Expect(err).NotTo(HaveOccurred())

			resourceLabels := map[string]string{
				"namespace": "prod",
				"job":       "different",
			}
			Expect(prom.MatchesResource(matchers, resourceLabels)).To(BeFalse())
		})

		It("UT-AF-T-021: handles regex matchers (=~)", func() {
			matchers, err := prom.ExtractLabelMatchers(`http_requests_total{namespace=~"prod|staging"}`)
			Expect(err).NotTo(HaveOccurred())

			Expect(prom.MatchesResource(matchers, map[string]string{"namespace": "prod"})).To(BeTrue())
			Expect(prom.MatchesResource(matchers, map[string]string{"namespace": "staging"})).To(BeTrue())
			Expect(prom.MatchesResource(matchers, map[string]string{"namespace": "dev"})).To(BeFalse())
		})
	})
})
