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
package mockllm_test

import (
	"encoding/json"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/response"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

var _ = Describe("BR-TESTING-001: Golden Transcript Replay — Phase 5a", func() {

	Describe("UT-REPLAY-001: LoadReplayScenarios loads golden transcripts from directory", func() {
		var goldenDir string

		BeforeEach(func() {
			var err error
			goldenDir, err = os.MkdirTemp("", "golden-*")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(goldenDir)
		})

		It("UT-REPLAY-001-001: returns nil for empty goldenDir", func() {
			replays, errs := scenarios.LoadReplayScenarios("")
			Expect(replays).To(BeNil())
			Expect(errs).To(BeNil())
		})

		It("UT-REPLAY-001-002: loads a valid golden transcript", func() {
			transcript := scenarios.GoldenTranscript{
				Scenario:   "oom-recovery",
				SignalName: "OOMKilled",
				CapturedAt: "2026-03-04T12:00:00Z",
			}
			transcript.KADialog.RawResponses = []string{
				`{"tool_calls": []}`,
				`{"root_cause_analysis": {"summary": "OOM due to traffic spike"}, "confidence": 0.95}`,
			}
			writeGoldenFile(goldenDir, "oom-recovery.json", transcript)

			replays, errs := scenarios.LoadReplayScenarios(goldenDir)
			Expect(errs).To(BeEmpty())
			Expect(replays).To(HaveLen(1))

			cfg := replays[0].Config()
			Expect(cfg.ScenarioName).To(Equal("replay:oom-recovery"))
			Expect(cfg.SignalName).To(Equal("OOMKilled"))
			Expect(cfg.ExactAnalysisText).To(ContainSubstring("OOM due to traffic spike"))
		})

		It("UT-REPLAY-001-003: skips files without signalName", func() {
			transcript := scenarios.GoldenTranscript{
				Scenario: "incomplete",
			}
			writeGoldenFile(goldenDir, "incomplete.json", transcript)

			replays, errs := scenarios.LoadReplayScenarios(goldenDir)
			Expect(replays).To(BeEmpty())
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Error()).To(ContainSubstring("missing signalName"))
		})

		It("UT-REPLAY-001-004: skips non-JSON files", func() {
			err := os.WriteFile(filepath.Join(goldenDir, "README.md"), []byte("# golden transcripts"), 0644)
			Expect(err).NotTo(HaveOccurred())

			replays, errs := scenarios.LoadReplayScenarios(goldenDir)
			Expect(replays).To(BeEmpty())
			Expect(errs).To(BeEmpty())
		})

		It("UT-REPLAY-001-005: loads multiple transcripts", func() {
			t1 := scenarios.GoldenTranscript{Scenario: "oom", SignalName: "OOMKilled"}
			t1.KADialog.RawResponses = []string{`{"rca": "oom"}`}

			t2 := scenarios.GoldenTranscript{Scenario: "crash", SignalName: "CrashLoopBackOff"}
			t2.KADialog.RawResponses = []string{`{"rca": "crash"}`}

			writeGoldenFile(goldenDir, "oom.json", t1)
			writeGoldenFile(goldenDir, "crash.json", t2)

			replays, errs := scenarios.LoadReplayScenarios(goldenDir)
			Expect(errs).To(BeEmpty())
			Expect(replays).To(HaveLen(2))
		})

		It("UT-REPLAY-001-006: uses last rawResponse as ExactAnalysisText", func() {
			transcript := scenarios.GoldenTranscript{
				Scenario:   "multi-turn",
				SignalName: "NodeNotReady",
			}
			transcript.KADialog.RawResponses = []string{
				`{"tool_calls": [{"tool": "list_available_actions"}]}`,
				`{"tool_calls": [{"tool": "get_workflow"}]}`,
				`{"root_cause_analysis": {"summary": "Node disk pressure"}, "confidence": 0.90}`,
			}
			writeGoldenFile(goldenDir, "multi-turn.json", transcript)

			replays, _ := scenarios.LoadReplayScenarios(goldenDir)
			Expect(replays).To(HaveLen(1))
			Expect(replays[0].Config().ExactAnalysisText).To(Equal(
				`{"root_cause_analysis": {"summary": "Node disk pressure"}, "confidence": 0.90}`))
		})
	})

	Describe("UT-REPLAY-002: Replay scenario matching", func() {
		It("UT-REPLAY-002-001: matches on signalName case-insensitively", func() {
			var goldenDir string
			goldenDir, _ = os.MkdirTemp("", "golden-match-*")
			defer os.RemoveAll(goldenDir)

			transcript := scenarios.GoldenTranscript{Scenario: "oom", SignalName: "OOMKilled"}
			transcript.KADialog.RawResponses = []string{`{"rca": "oom"}`}
			writeGoldenFile(goldenDir, "oom.json", transcript)

			replays, _ := scenarios.LoadReplayScenarios(goldenDir)
			Expect(replays).To(HaveLen(1))

			registry := scenarios.NewRegistry()
			for _, rs := range replays {
				registry.Register(rs)
			}

			result := registry.Detect(&scenarios.DetectionContext{
				SignalName: "oomkilled",
			})
			Expect(result).NotTo(BeNil())
			Expect(result.Confidence).To(BeNumerically("==", 1.1))
		})

		It("UT-REPLAY-002-002: replay beats keyword scenario at confidence 1.0", func() {
			var goldenDir string
			goldenDir, _ = os.MkdirTemp("", "golden-priority-*")
			defer os.RemoveAll(goldenDir)

			transcript := scenarios.GoldenTranscript{Scenario: "oom-golden", SignalName: "OOMKilled"}
			transcript.KADialog.RawResponses = []string{`{"rca": "golden oom"}`}
			writeGoldenFile(goldenDir, "oom.json", transcript)

			registry := scenarios.DefaultRegistryFull(nil, goldenDir)

			result := registry.Detect(&scenarios.DetectionContext{
				SignalName: "OOMKilled",
				Content:    "Signal Name: OOMKilled",
			})
			Expect(result).NotTo(BeNil())
			Expect(result.Confidence).To(BeNumerically("==", 1.1))

			swc, ok := result.Scenario.(scenarios.ScenarioWithConfig)
			Expect(ok).To(BeTrue())
			Expect(swc.Config().ExactAnalysisText).To(Equal(`{"rca": "golden oom"}`))
		})

		It("UT-REPLAY-002-003: does not match when signalName differs", func() {
			var goldenDir string
			goldenDir, _ = os.MkdirTemp("", "golden-nomatch-*")
			defer os.RemoveAll(goldenDir)

			transcript := scenarios.GoldenTranscript{Scenario: "oom", SignalName: "OOMKilled"}
			transcript.KADialog.RawResponses = []string{`{"rca": "oom"}`}
			writeGoldenFile(goldenDir, "oom.json", transcript)

			replays, _ := scenarios.LoadReplayScenarios(goldenDir)
			registry := scenarios.NewRegistry()
			for _, rs := range replays {
				registry.Register(rs)
			}

			result := registry.Detect(&scenarios.DetectionContext{
				SignalName: "CrashLoopBackOff",
			})
			Expect(result).To(BeNil())
		})
	})

	Describe("UT-REPLAY-003: ExactAnalysisText in response builder", func() {
		It("UT-REPLAY-003-001: returns ExactAnalysisText verbatim when set", func() {
			exactJSON := `{"root_cause_analysis": {"summary": "Golden transcript OOM analysis"}, "confidence": 0.95}`
			cfg := scenarios.MockScenarioConfig{
				ScenarioName:      "replay:oom",
				SignalName:        "OOMKilled",
				ExactAnalysisText: exactJSON,
			}

			resp := response.BuildTextResponse("mock-model", cfg)
			Expect(resp.Choices[0].Message.Content).NotTo(BeNil())
			Expect(*resp.Choices[0].Message.Content).To(Equal(exactJSON))
		})

		It("UT-REPLAY-003-002: falls back to synthesized JSON when ExactAnalysisText is empty", func() {
			cfg := scenarios.MockScenarioConfig{
				ScenarioName: "oomkilled",
				SignalName:   "OOMKilled",
				Severity:     "critical",
				Confidence:   0.95,
				RootCause:    "Container exceeded memory limits",
			}

			resp := response.BuildTextResponse("mock-model", cfg)
			text := *resp.Choices[0].Message.Content

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(text), &parsed)).To(Succeed())
			Expect(parsed).To(HaveKey("root_cause_analysis"))
		})
	})
})

func writeGoldenFile(dir, name string, t scenarios.GoldenTranscript) {
	data, err := json.MarshalIndent(t, "", "  ")
	Expect(err).NotTo(HaveOccurred())
	err = os.WriteFile(filepath.Join(dir, name), data, 0644)
	Expect(err).NotTo(HaveOccurred())
}
