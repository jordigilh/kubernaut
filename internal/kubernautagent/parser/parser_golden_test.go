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

package parser_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
)

// goldenTranscript mirrors the schema from scenarios/replay.go for test loading.
type goldenTranscript struct {
	Scenario   string `json:"scenario"`
	SignalName string `json:"signalName"`
	Analysis   struct {
		RootCauseAnalysis struct {
			Summary string `json:"summary"`
		} `json:"rootCauseAnalysis"`
		SelectedWorkflow struct {
			WorkflowID string `json:"workflowId"`
		} `json:"selectedWorkflow"`
	} `json:"analysis"`
	KADialog struct {
		RawResponses []string `json:"rawResponses"`
	} `json:"kaDialog"`
}

// embeddedGoldenDir returns the path to the testdata/golden_transcripts
// directory co-located with this test file.
func embeddedGoldenDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "testdata", "golden_transcripts")
}

var _ = Describe("BR-TESTING-001 Phase 6: Parser Golden Tests", func() {
	goldenDir := os.Getenv("KA_GOLDEN_TRANSCRIPTS_DIR")
	if goldenDir == "" {
		goldenDir = embeddedGoldenDir()
	}

	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		Fail("golden transcripts directory unreadable (" + goldenDir + "): " + err.Error())
	}

	p := parser.NewResultParser()

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		fileName := entry.Name()

		It("parses golden transcript: "+fileName, func() {
			data, err := os.ReadFile(filepath.Join(goldenDir, fileName))
			Expect(err).NotTo(HaveOccurred())

			var t goldenTranscript
			Expect(json.Unmarshal(data, &t)).To(Succeed())
			Expect(t.KADialog.RawResponses).NotTo(BeEmpty(),
				"golden transcript must have at least one rawResponse")

			lastResponse := t.KADialog.RawResponses[len(t.KADialog.RawResponses)-1]
			result, err := p.Parse(lastResponse)
			Expect(err).NotTo(HaveOccurred(), "parser should handle real Claude output")

			if t.Analysis.RootCauseAnalysis.Summary != "" {
				Expect(result.RCASummary).NotTo(BeEmpty(),
					"parser must extract RCA summary from real Claude response")
			}

			if t.Analysis.SelectedWorkflow.WorkflowID != "" {
				Expect(result.WorkflowID).To(Equal(t.Analysis.SelectedWorkflow.WorkflowID),
					"parser must extract correct workflow_id from real Claude response")
			}

			Expect(result.SignalName).NotTo(BeEmpty(),
				"parser must extract signal_name from real Claude response")
		})
	}
})
