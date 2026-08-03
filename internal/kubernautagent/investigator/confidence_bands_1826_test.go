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

// Package investigator_test: BR-KA-213 / Issue #1826 — proves
// investigator.Config's ResolvedConfidenceThreshold/InconclusiveConfidenceThreshold
// flow through the real production entry point (RunWorkflowDiscoveryFromRCA ->
// runWorkflowSelection -> prompt.Builder.RenderWorkflowSelection) into the
// actual system prompt sent to the LLM, not just reachable at the
// prompt.Builder unit level (see confidence_bands_1826_test.go in the
// prompt package for that).
package investigator_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("IT-KA-1826-001: confidence bands wired from investigator.Config into the LLM system prompt", func() {
	It("should render the operator-configured bands, not the 0.7/0.5 hardcoded defaults", func() {
		logger := logr.Discard()
		store := &gateRecordingAuditStore{}
		client := &gateMockLLMClient{}
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		rp := parser.NewResultParser()
		enricher := enrichment.NewEnricher(&rcaK8sClient{}, &rcaDSClient{}, store, logger)

		inv := newTestInvestigator(investigator.Config{
			Client: client, Builder: builder, ResultParser: rp,
			Enricher: enricher, AuditStore: store, Logger: logger,
			MaxTurns: 15, PhaseTools: investigator.DefaultPhaseToolMap(),
			ResolvedConfidenceThreshold:     0.85,
			InconclusiveConfidenceThreshold: 0.3,
		})

		client.responses = []llm.ChatResponse{
			gateWfToolResp(`{"workflow_id":"restart-pod","confidence":0.9}`),
		}

		signal := katypes.SignalContext{Name: "sig", Namespace: "default", ResourceKind: "Pod", ResourceName: "web-pod"}
		rcaResult := &katypes.InvestigationResult{
			RCASummary: "root cause identified",
			RemediationTarget: katypes.RemediationTarget{
				Kind: "Pod", Name: "web-pod", Namespace: "default",
			},
		}

		_, err = inv.RunWorkflowDiscoveryFromRCA(context.Background(), signal, rcaResult, nil, "corr-1826")
		Expect(err).NotTo(HaveOccurred())

		Expect(client.calls).NotTo(BeEmpty(), "IT-KA-1826-001: the LLM must have been called for workflow selection")
		systemPrompt := client.calls[0].Messages[0].Content
		Expect(systemPrompt).To(ContainSubstring(`"resolved" (confidence >= 0.85)`),
			"IT-KA-1826-001: investigator.Config.ResolvedConfidenceThreshold must reach the rendered system prompt")
		Expect(systemPrompt).To(ContainSubstring(`"inconclusive" (confidence < 0.3)`),
			"IT-KA-1826-001: investigator.Config.InconclusiveConfidenceThreshold must reach the rendered system prompt")
		Expect(systemPrompt).ToNot(ContainSubstring(`>= 0.7)`),
			"the hardcoded 0.7 default must not leak through when a custom threshold is configured")
	})
})
