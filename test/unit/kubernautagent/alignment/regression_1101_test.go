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

package alignment_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/alignment"
	alignprompt "github.com/jordigilh/kubernaut/internal/kubernautagent/alignment/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

func testdataPath(filename string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "testdata", filename)
}

var _ = Describe("Regression #1101: Shadow agent evaluates raw tool output — BR-AI-601", func() {

	Describe("UT-SA-1101-001: Raw kubectl output is classified CLEAN", func() {
		It("should not flag raw K8s Deployment/ReplicaSet JSON from kubectl_get_by_kind_in_namespace", func() {
			rawData, err := os.ReadFile(testdataPath("alert_misdirection_raw_tool_result.json"))
			Expect(err).NotTo(HaveOccurred())

			client := &mockLLMClient{responses: []llm.ChatResponse{cleanResponse()}}
			evaluator := alignment.NewEvaluator(client, alignment.EvaluatorConfig{
				Timeout:       10 * time.Second,
				MaxStepTokens: 4000,
				MaxRetries:    1,
			}, alignprompt.SystemPrompt())

			step := alignment.Step{
				Index:   4,
				Kind:    alignment.StepKindToolResult,
				Tool:    "kubectl_get_by_kind_in_namespace",
				Content: string(rawData),
			}

			obs := evaluator.EvaluateStep(context.Background(), step)
			Expect(obs.Suspicious).To(BeFalse(),
				"raw K8s JSON must not be flagged — this is the pre-summarizer data that #1101 fixes")
		})
	})

	Describe("UT-SA-1101-002: Summarized analysis with directive language was the false positive source", func() {
		It("should demonstrate that the summarized content contains directive language the shadow previously flagged", func() {
			summarizedData, err := os.ReadFile(testdataPath("alert_misdirection_summarized_tool_result.txt"))
			Expect(err).NotTo(HaveOccurred())

			Expect(string(summarizedData)).To(ContainSubstring("Immediately roll back the deployment"),
				"the summarized content must contain the directive language that caused #1101")
			Expect(string(summarizedData)).To(ContainSubstring("Recommended Next Steps"),
				"the summarized content must contain the analysis section header")
		})
	})

	Describe("UT-SA-1101-003: Pre-summarizer submission ensures shadow sees raw data", func() {
		It("should verify the raw tool output does not contain LLM analysis headers", func() {
			rawData, err := os.ReadFile(testdataPath("alert_misdirection_raw_tool_result.json"))
			Expect(err).NotTo(HaveOccurred())

			content := string(rawData)
			Expect(content).NotTo(ContainSubstring("Recommended Next Steps"),
				"raw tool output must not contain LLM analysis — confirming the shadow sees external data only")
			Expect(content).NotTo(ContainSubstring("Key Incident Findings"),
				"raw tool output must not contain LLM analysis headers")
			Expect(content).NotTo(ContainSubstring("Immediately roll back"),
				"raw tool output must not contain directive language from the summarizer")
		})
	})
})
