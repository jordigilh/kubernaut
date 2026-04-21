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

	Describe("UT-KA-686-010: Reset() restores a fresh budget for a new phase", func() {
		It("should allow full budget after Reset()", func() {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 100,
				MaxTotalToolCalls:   5,
				MaxRepeatedFailures: 100,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)

			// Exhaust the total budget
			tools := []string{"kubectl_describe", "kubectl_logs", "kubectl_events", "kubectl_get_by_name", "prometheus_query"}
			for i, tool := range tools {
				r := detector.CheckToolCall(tool, json.RawMessage(`{}`))
				Expect(r.Allowed).To(BeTrue(), "call %d should be allowed", i+1)
			}
			r := detector.CheckToolCall("kubectl_logs_grep", json.RawMessage(`{}`))
			Expect(r.Allowed).To(BeFalse(), "should be rejected after budget exhausted")
			Expect(detector.TotalExceeded()).To(BeTrue())

			// Reset and verify full budget is available again
			detector.Reset()
			Expect(detector.TotalExceeded()).To(BeFalse(), "TotalExceeded should be false after Reset")

			for i, tool := range tools {
				r := detector.CheckToolCall(tool, json.RawMessage(`{}`))
				Expect(r.Allowed).To(BeTrue(), "post-reset call %d should be allowed", i+1)
			}
		})

		It("should reset per-tool counters", func() {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 2,
				MaxTotalToolCalls:   100,
				MaxRepeatedFailures: 100,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)

			detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
			detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
			r := detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
			Expect(r.Allowed).To(BeFalse(), "should be rejected at per-tool limit")

			detector.Reset()

			r = detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
			Expect(r.Allowed).To(BeTrue(), "post-reset per-tool counter should be fresh")
		})

		It("should reset failure tracker", func() {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 100,
				MaxTotalToolCalls:   100,
				MaxRepeatedFailures: 2,
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)
			args := json.RawMessage(`{"name":"pod-a"}`)

			detector.RecordFailure("kubectl_describe", args)
			r := detector.RecordFailure("kubectl_describe", args)
			Expect(r.Allowed).To(BeFalse(), "should abort after repeated failures")

			detector.Reset()

			r = detector.RecordFailure("kubectl_describe", args)
			Expect(r.Allowed).To(BeTrue(), "post-reset failure tracker should be fresh")
		})

		It("should preserve config and suspicious patterns after Reset()", func() {
			patterns := []*regexp.Regexp{regexp.MustCompile(`(?i)/etc/shadow`)}
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 2,
				MaxTotalToolCalls:   5,
				MaxRepeatedFailures: 2,
			}
			detector := investigator.NewAnomalyDetector(cfg, patterns)

			// Exhaust budget, then reset
			for i := 0; i < 5; i++ {
				detector.CheckToolCall("kubectl_logs", json.RawMessage(`{}`))
			}
			detector.Reset()

			// Config thresholds still enforced
			detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
			detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
			r := detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
			Expect(r.Allowed).To(BeFalse(), "per-tool limit from config should still apply")

			// Suspicious patterns still enforced
			r = detector.CheckToolCall("kubectl_logs", json.RawMessage(`{"path":"/etc/shadow"}`))
			Expect(r.Allowed).To(BeFalse(), "suspicious patterns should still be active")
			Expect(r.Reason).To(ContainSubstring("suspicious"))
		})
	})

	Describe("UT-KA-770-010: Reset at session start scopes budget to investigation", func() {
		// This test reproduces the critical bug from #770: the AnomalyDetector
		// counter persists across Investigate() calls because Reset() is only
		// called between phases (RCA→WF), not at the start of each session.
		// After 1-2 investigations the KA becomes non-functional.
		It("should allow full budget in a second session after Reset at session boundary", func() {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 100,
				MaxTotalToolCalls:   5,
				MaxRepeatedFailures: 100,
				ExemptPrefixes:      []string{"todo_"},
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)

			// --- Session 1: exhaust most of the budget (like a real investigation) ---
			session1Tools := []string{"kubectl_describe", "kubectl_events", "kubectl_logs", "kubectl_get_by_name"}
			for _, t := range session1Tools {
				r := detector.CheckToolCall(t, json.RawMessage(`{}`))
				Expect(r.Allowed).To(BeTrue(), "session 1: %s should be allowed", t)
			}
			// Inter-phase reset (RCA → workflow selection) — this exists today
			detector.Reset()
			for _, t := range session1Tools {
				r := detector.CheckToolCall(t, json.RawMessage(`{}`))
				Expect(r.Allowed).To(BeTrue(), "session 1 phase 2: %s should be allowed", t)
			}

			// --- Session boundary: simulate start of a new Investigate() call ---
			// BUG: without this Reset(), session 2 inherits session 1's counters
			detector.Reset()

			// --- Session 2: must have full budget ---
			for i, t := range session1Tools {
				r := detector.CheckToolCall(t, json.RawMessage(`{}`))
				Expect(r.Allowed).To(BeTrue(), "session 2 call %d (%s) should be allowed with fresh budget", i+1, t)
			}
			r := detector.CheckToolCall("list_available_actions", json.RawMessage(`{}`))
			Expect(r.Allowed).To(BeTrue(), "session 2: 5th call should still be within budget of 5")
		})

		It("should fail session 2 WITHOUT reset at session boundary (documents the bug)", func() {
			cfg := investigator.AnomalyConfig{
				MaxToolCallsPerTool: 100,
				MaxTotalToolCalls:   5,
				MaxRepeatedFailures: 100,
				ExemptPrefixes:      []string{"todo_"},
			}
			detector := investigator.NewAnomalyDetector(cfg, nil)

			// --- Session 1: use 4 tool calls ---
			for _, t := range []string{"kubectl_describe", "kubectl_events", "kubectl_logs", "kubectl_get_by_name"} {
				detector.CheckToolCall(t, json.RawMessage(`{}`))
			}
			// Inter-phase reset (exists today)
			detector.Reset()
			// Session 1 phase 2: use 4 more
			for _, t := range []string{"list_available_actions", "list_workflows", "get_workflow", "kubectl_describe"} {
				detector.CheckToolCall(t, json.RawMessage(`{}`))
			}

			// NO session-boundary reset — simulating the bug

			// Session 2: counter carries over from session 1 phase 2 (at 4)
			r := detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
			Expect(r.Allowed).To(BeTrue(), "5th call should still be allowed")
			r = detector.CheckToolCall("kubectl_events", json.RawMessage(`{}`))
			Expect(r.Allowed).To(BeFalse(), "6th call should be rejected — counter leaked from session 1")
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
