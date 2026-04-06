package investigator_test

import (
	"encoding/json"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
)

var _ = Describe("Kubernaut Agent I7 Anomaly Detection — #433", func() {

	Describe("UT-KA-433-054: Per-tool call limit triggers abort", func() {
		It("should reject when a single tool exceeds its per-tool limit", func() {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 3,
				MaxTotalToolCalls:   30,
				MaxRepeatedFailures: 3,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)
			args := json.RawMessage(`{"kind":"pod","name":"web","namespace":"default"}`)

			for i := 0; i < 3; i++ {
				result := detector.CheckToolCall("kubectl_describe", args)
				Expect(result.Allowed).To(BeTrue(), "call %d should be allowed", i+1)
			}

			result := detector.CheckToolCall("kubectl_describe", args)
			Expect(result.Allowed).To(BeFalse(), "4th call to same tool should be rejected")
			Expect(result.Reason).To(ContainSubstring("per-tool"))
		})
	})

	Describe("UT-KA-433-055: Total tool call limit triggers abort", func() {
		It("should reject when total calls across all tools exceed limit", func() {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 100,
				MaxTotalToolCalls:   5,
				MaxRepeatedFailures: 100,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)
			tools := []string{"kubectl_describe", "kubectl_logs", "kubectl_events", "kubectl_get_by_name", "execute_prometheus_instant_query"}

			for i, tool := range tools {
				result := detector.CheckToolCall(tool, json.RawMessage(`{}`))
				Expect(result.Allowed).To(BeTrue(), "call %d (%s) should be allowed", i+1, tool)
			}

			result := detector.CheckToolCall("kubectl_logs_grep", json.RawMessage(`{}`))
			Expect(result.Allowed).To(BeFalse(), "6th total call should be rejected")
			Expect(result.Reason).To(ContainSubstring("total"))
		})
	})

	Describe("UT-KA-433-056: Repeated identical failures trigger abort", func() {
		It("should reject after N identical tool+args failures", func() {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 100,
				MaxTotalToolCalls:   100,
				MaxRepeatedFailures: 3,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)
			args := json.RawMessage(`{"kind":"pod","name":"missing-pod","namespace":"default"}`)

			for i := 0; i < 2; i++ {
				result := detector.RecordFailure("kubectl_describe", args)
				Expect(result.Allowed).To(BeTrue(), "failure %d should still be allowed", i+1)
			}

			result := detector.RecordFailure("kubectl_describe", args)
			Expect(result.Allowed).To(BeFalse(), "3rd identical failure should trigger abort")
			Expect(result.Reason).To(ContainSubstring("repeated"))
		})

		It("should track failures independently per tool+args combination", func() {
			cfg := investigator.DefaultAnomalyConfig()
			detector := investigator.NewAnomalyDetector(cfg, nil)

			args1 := json.RawMessage(`{"name":"pod-a"}`)
			args2 := json.RawMessage(`{"name":"pod-b"}`)

			detector.RecordFailure("kubectl_describe", args1)
			detector.RecordFailure("kubectl_describe", args1)
			detector.RecordFailure("kubectl_describe", args2)

			result := detector.RecordFailure("kubectl_describe", args2)
			Expect(result.Allowed).To(BeTrue(), "different args should have independent counters")
		})
	})

	Describe("UT-KA-433-057: Suspicious argument patterns logged as anomaly", func() {
		It("should flag tool calls with suspicious arguments", func() {
			suspiciousPatterns := []*regexp.Regexp{
				regexp.MustCompile(`(?i)/etc/shadow`),
				regexp.MustCompile(`(?i);\s*rm\s+-rf`),
			}
			cfg := investigator.DefaultAnomalyConfig()
			detector := investigator.NewAnomalyDetector(cfg, suspiciousPatterns)

			args := json.RawMessage(`{"name":"pod","namespace":"default; rm -rf /"}`)
			result := detector.CheckToolCall("kubectl_logs", args)
			Expect(result.Allowed).To(BeFalse(), "suspicious arguments should be rejected")
			Expect(result.Reason).To(ContainSubstring("suspicious"))
		})
	})

	Describe("UT-KA-433-058: Below-threshold calls proceed normally", func() {
		It("should allow calls well within all thresholds", func() {
			cfg := investigator.DefaultAnomalyConfig()
			detector := investigator.NewAnomalyDetector(cfg, nil)

			result := detector.CheckToolCall("kubectl_describe", json.RawMessage(`{"kind":"pod","name":"web","namespace":"default"}`))
			Expect(result.Allowed).To(BeTrue())

			result = detector.CheckToolCall("kubectl_logs", json.RawMessage(`{"name":"web","namespace":"default"}`))
			Expect(result.Allowed).To(BeTrue())

			result = detector.CheckToolCall("execute_prometheus_instant_query", json.RawMessage(`{"query":"up"}`))
			Expect(result.Allowed).To(BeTrue())
		})

		It("should allow different tools up to per-tool limit", func() {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 3,
				MaxTotalToolCalls:   30,
				MaxRepeatedFailures: 3,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)

			for i := 0; i < 3; i++ {
				r1 := detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
				Expect(r1.Allowed).To(BeTrue())
				r2 := detector.CheckToolCall("kubectl_logs", json.RawMessage(`{}`))
				Expect(r2.Allowed).To(BeTrue())
			}
		})
	})

	Describe("UT-KA-433-059: Configurable thresholds from config", func() {
		It("should respect custom threshold values", func() {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 2,
				MaxTotalToolCalls:   10,
				MaxRepeatedFailures: 1,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)

			detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
			detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
			result := detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
			Expect(result.Allowed).To(BeFalse(), "custom per-tool limit of 2 should apply")
		})

		It("should use default config values when DefaultAnomalyConfig is used", func() {
			cfg := investigator.DefaultAnomalyConfig()
			Expect(cfg.MaxToolCallsPerTool).To(Equal(5))
			Expect(cfg.MaxTotalToolCalls).To(Equal(30))
			Expect(cfg.MaxRepeatedFailures).To(Equal(3))
		})
	})
})
