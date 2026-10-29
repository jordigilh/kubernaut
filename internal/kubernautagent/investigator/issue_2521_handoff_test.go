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

package investigator

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("Issue #2521 target-scoped Phase 3 handoff", func() {
	It("UT-KA-2521-007: excludes raw PhaseRCA observations while retaining structured RCA and target context", func() {
		inv := New(Config{Logger: logr.Discard()})
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Signal:     katypes.SignalContext{Namespace: "prod", ResourceKind: "Deployment", ResourceName: "api"},
			RCASummary: "the target deployment is OOMKilled",
			EnrichData: &prompt.EnrichmentData{OwnerChain: []string{"Deployment/api"}},
		}, "rr-2521")
		Expect(err).NotTo(HaveOccurred())

		var packet map[string]json.RawMessage
		Expect(json.Unmarshal([]byte(frozen.serialized), &packet)).To(Succeed())
		Expect(packet).NotTo(HaveKey("current_observations"))
		Expect(string(packet["rca"])).To(ContainSubstring("OOMKilled"))
		Expect(string(packet["context"])).To(ContainSubstring("Deployment/api"))
		Expect(frozen.serialized).NotTo(ContainSubstring("raw PhaseRCA tool output"))
	})

	It("UT-KA-2521-008: an oversized ordinary PhaseRCA observation does not make structured handoff unavailable", func() {
		inv := New(Config{Logger: logr.Discard()})
		inv.finishToolResult(context.Background(), "kubectl_get_by_kind_in_cluster", strings.Repeat("large observation ", 20), "rr-2521", katypes.PhaseRCA)

		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Signal:     katypes.SignalContext{Namespace: "prod", ResourceKind: "Deployment", ResourceName: "api"},
			RCASummary: "structured RCA remains available",
		}, "rr-2521")
		Expect(err).NotTo(HaveOccurred())
		Expect(frozen.serialized).To(ContainSubstring("structured RCA remains available"))
	})

	It("UT-KA-2521-008: structured budget covers the serialized provider message context", func() {
		messages := []llm.Message{
			{Role: "system", Content: strings.Repeat("system-context ", 20)},
			{Role: "user", Content: "Select the appropriate remediation workflow."},
		}
		err := validateWorkflowSelectionContextBudget(messages, 32)
		Expect(err).To(MatchError("workflow selection prompt exceeds configured byte budget"))
	})

	It("UT-KA-2521-008: parse-retry provider dispatch also enforces the structured budget", func() {
		inv := New(Config{Logger: logr.Discard(), Pipeline: Pipeline{StructuredPhase3ContextBudget: 32}})
		result, content := inv.retryWorkflowSubmit(context.Background(), "last response", []llm.Message{{
			Role: "system", Content: strings.Repeat("previous workflow context ", 20),
		}}, "RCA", LLMInvocationContext{CorrelationID: "rr-budget-retry-2521"})
		Expect(content).To(BeEmpty())
		Expect(result).NotTo(BeNil())
		Expect(result.TerminalOutcome).To(Equal(sharedtypes.TerminalOutcomeRCAIncomplete))
	})
})
