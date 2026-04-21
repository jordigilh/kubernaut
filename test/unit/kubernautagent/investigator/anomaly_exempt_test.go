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

// Package investigator_test contains unit tests for the anomaly detector's
// exempt prefix feature.
//
// Issue #770: todo_write consumes investigation budget, causing Node scenarios
// to hit the 30-call ceiling before completing RCA.
package investigator_test

import (
	"encoding/json"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
)

var _ = Describe("Anomaly Detector Exempt Prefixes (#770)", func() {

	It("UT-KA-770-001: todo_write calls do NOT count against total budget", func() {
		cfg := investigator.AnomalyConfig{
			MaxToolCallsPerTool: 100,
			MaxTotalToolCalls:   3,
			MaxRepeatedFailures: 100,
			ExemptPrefixes:      []string{"todo_"},
		}
		detector := investigator.NewAnomalyDetector(cfg, nil)

		for i := 0; i < 10; i++ {
			r := detector.CheckToolCall("todo_write", json.RawMessage(`{"todos":[{"id":"1","content":"test","status":"pending"}]}`))
			Expect(r.Allowed).To(BeTrue(), "todo_write call %d should be allowed (exempt from total budget)", i+1)
		}

		r := detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
		Expect(r.Allowed).To(BeTrue(), "first investigation tool call should still be allowed")
		r = detector.CheckToolCall("kubectl_events", json.RawMessage(`{}`))
		Expect(r.Allowed).To(BeTrue(), "second investigation tool call should still be allowed")
		r = detector.CheckToolCall("kubectl_logs", json.RawMessage(`{}`))
		Expect(r.Allowed).To(BeTrue(), "third investigation tool call should still be allowed")

		r = detector.CheckToolCall("kubectl_get_by_name", json.RawMessage(`{}`))
		Expect(r.Allowed).To(BeFalse(), "fourth investigation call should exceed total budget of 3")
		Expect(r.Reason).To(ContainSubstring("total"))
	})

	It("UT-KA-770-002: todo_write calls do NOT count against per-tool budget", func() {
		cfg := investigator.AnomalyConfig{
			MaxToolCallsPerTool: 2,
			MaxTotalToolCalls:   100,
			MaxRepeatedFailures: 100,
			ExemptPrefixes:      []string{"todo_"},
		}
		detector := investigator.NewAnomalyDetector(cfg, nil)

		for i := 0; i < 20; i++ {
			r := detector.CheckToolCall("todo_write", json.RawMessage(`{}`))
			Expect(r.Allowed).To(BeTrue(), "todo_write call %d should be allowed (exempt from per-tool budget)", i+1)
		}
	})

	It("UT-KA-770-003: todo_write calls ARE checked for suspicious arguments", func() {
		patterns := []*regexp.Regexp{regexp.MustCompile(`(?i)/etc/shadow`)}
		cfg := investigator.AnomalyConfig{
			MaxToolCallsPerTool: 100,
			MaxTotalToolCalls:   100,
			MaxRepeatedFailures: 100,
			ExemptPrefixes:      []string{"todo_"},
		}
		detector := investigator.NewAnomalyDetector(cfg, patterns)

		r := detector.CheckToolCall("todo_write", json.RawMessage(`{"todos":[{"content":"/etc/shadow"}]}`))
		Expect(r.Allowed).To(BeFalse(), "suspicious args in todo_write should still be rejected")
		Expect(r.Reason).To(ContainSubstring("suspicious"))
	})

	It("UT-KA-770-004: investigation tools still count normally with exempt tools present", func() {
		cfg := investigator.AnomalyConfig{
			MaxToolCallsPerTool: 2,
			MaxTotalToolCalls:   5,
			MaxRepeatedFailures: 100,
			ExemptPrefixes:      []string{"todo_"},
		}
		detector := investigator.NewAnomalyDetector(cfg, nil)

		detector.CheckToolCall("todo_write", json.RawMessage(`{}`))
		detector.CheckToolCall("todo_write", json.RawMessage(`{}`))
		detector.CheckToolCall("todo_write", json.RawMessage(`{}`))

		r := detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
		Expect(r.Allowed).To(BeTrue(), "investigation tools should count from 0 regardless of exempt calls")

		r = detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
		Expect(r.Allowed).To(BeTrue(), "second kubectl_describe should be within per-tool limit")

		r = detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
		Expect(r.Allowed).To(BeFalse(), "third kubectl_describe should exceed per-tool limit of 2")
	})

	It("UT-KA-770-005: custom exempt prefixes from config are respected", func() {
		cfg := investigator.AnomalyConfig{
			MaxToolCallsPerTool: 100,
			MaxTotalToolCalls:   2,
			MaxRepeatedFailures: 100,
			ExemptPrefixes:      []string{"todo_", "internal_"},
		}
		detector := investigator.NewAnomalyDetector(cfg, nil)

		for i := 0; i < 5; i++ {
			r := detector.CheckToolCall("internal_planning", json.RawMessage(`{}`))
			Expect(r.Allowed).To(BeTrue(), "internal_planning should be exempt")
		}

		r := detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
		Expect(r.Allowed).To(BeTrue())
		r = detector.CheckToolCall("kubectl_events", json.RawMessage(`{}`))
		Expect(r.Allowed).To(BeTrue())
		r = detector.CheckToolCall("kubectl_logs", json.RawMessage(`{}`))
		Expect(r.Allowed).To(BeFalse(), "non-exempt tools should still hit total budget")
	})

	It("UT-KA-770-006: DefaultAnomalyConfig includes todo_ in ExemptPrefixes", func() {
		cfg := investigator.DefaultAnomalyConfig()
		Expect(cfg.ExemptPrefixes).To(ContainElement("todo_"),
			"DefaultAnomalyConfig must include todo_ as an exempt prefix")
	})

	It("UT-KA-770-007: TotalExceeded returns false even after many exempt tool calls", func() {
		cfg := investigator.AnomalyConfig{
			MaxToolCallsPerTool: 100,
			MaxTotalToolCalls:   3,
			MaxRepeatedFailures: 100,
			ExemptPrefixes:      []string{"todo_"},
		}
		detector := investigator.NewAnomalyDetector(cfg, nil)

		for i := 0; i < 50; i++ {
			detector.CheckToolCall("todo_write", json.RawMessage(`{}`))
		}

		Expect(detector.TotalExceeded()).To(BeFalse(),
			"TotalExceeded should be false — exempt calls don't count")

		detector.CheckToolCall("kubectl_describe", json.RawMessage(`{}`))
		detector.CheckToolCall("kubectl_events", json.RawMessage(`{}`))
		detector.CheckToolCall("kubectl_logs", json.RawMessage(`{}`))
		Expect(detector.TotalExceeded()).To(BeFalse(), "at limit, not exceeded")

		detector.CheckToolCall("kubectl_get_by_name", json.RawMessage(`{}`))
		Expect(detector.TotalExceeded()).To(BeTrue(), "now exceeded after 4th investigation call")
	})
})
