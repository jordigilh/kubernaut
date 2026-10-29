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
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	internalerrors "github.com/jordigilh/kubernaut/internal/errors"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

var _ = Describe("Issue #2533 frozen historical decision context", func() {
	It("UT-KA-2533-004: carries bounded decision context without changing repeat identity", func() {
		inv := New(Config{Logger: logr.Discard()})
		history := &enrichment.RemediationHistoryResult{
			TargetResource: "prod/Deployment/api",
			Tier1: []enrichment.Tier1Entry{{
				RemediationUID:   "rr-2533",
				WorkflowID:       "catalog-workflow",
				WorkflowName:     "catalog-name",
				AssessmentReason: "Full",
				EffectivenessScore: func() *float64 {
					value := 0.0
					return &value
				}(),
				WorkflowRationale: "The current target evidence supports the catalog workflow.",
				RepeatJustification: &sharedtypes.RepeatJustification{
					EvidenceBasis:         "Current evidence basis",
					PriorOutcomeRelevance: "The prior outcome was ineffective",
					ExpectedMechanism:     "The workflow addresses the observed mechanism",
				},
			}},
		}
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Signal:     katypes.SignalContext{Namespace: "prod", ResourceKind: "Deployment", ResourceName: "api"},
			EnrichData: &prompt.EnrichmentData{HistoryResult: history},
		}, "rr-current-2533")
		Expect(err).NotTo(HaveOccurred())

		var packet evidenceHandoff
		Expect(json.Unmarshal([]byte(frozen.serialized), &packet)).To(Succeed())
		Expect(packet.PriorAttempts).To(HaveLen(1))
		Expect(packet.PriorAttempts[0].WorkflowRationale).To(HaveValue(Equal("The current target evidence supports the catalog workflow.")))
		Expect(packet.PriorAttempts[0].RepeatJustification).NotTo(BeNil())
		Expect(frozen.repeat.CompletedPriorWorkflows).To(HaveKeyWithValue("catalog-workflow", []string{"rr-2533"}))
	})

	It("UT-KA-2533-005: historical reasoning cannot establish repeat identity by itself", func() {
		packet, err := buildEvidenceHandoff(handoffInput{EnrichData: &prompt.EnrichmentData{
			HistoryResult: &enrichment.RemediationHistoryResult{Tier1: []enrichment.Tier1Entry{{
				RemediationUID:    "rr-unknown-2533",
				WorkflowRationale: "A model-authored rationale without catalog identity",
			}}},
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(repeatContextFromHandoff(packet).CompletedPriorWorkflows).To(BeEmpty())
	})

	It("UT-KA-2533-005: complete frozen-packet budget remains fail-closed for historical context", func() {
		inv := New(Config{Logger: logr.Discard(), Pipeline: Pipeline{StructuredPhase3ContextBudget: 512}})
		history := &enrichment.RemediationHistoryResult{TargetResource: "prod/Deployment/api"}
		for i := 0; i < 2; i++ {
			history.Tier1 = append(history.Tier1, enrichment.Tier1Entry{
				RemediationUID:    fmt.Sprintf("rr-budget-2533-%d", i),
				WorkflowRationale: strings.Repeat("r", 2000),
			})
		}
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Signal:     katypes.SignalContext{Namespace: "prod", ResourceKind: "Deployment", ResourceName: "api"},
			EnrichData: &prompt.EnrichmentData{HistoryResult: history},
		}, "rr-budget-current-2533")
		Expect(err).To(MatchError(ContainSubstring("byte budget")))
		Expect(frozen).To(BeNil())
	})

	It("UT-KA-2533-006: the rendered Phase 3 request remains bounded after frozen history validation", func() {
		inv := New(Config{Logger: logr.Discard()})
		maxText := strings.Repeat("x", sharedtypes.HistoricalDecisionTextLimit)
		history := &enrichment.RemediationHistoryResult{TargetResource: "prod/Deployment/api", Tier1: []enrichment.Tier1Entry{{
			RemediationUID:    "rr-prompt-budget-2533",
			WorkflowRationale: maxText,
			RepeatJustification: &sharedtypes.RepeatJustification{
				EvidenceBasis:         maxText,
				PriorOutcomeRelevance: maxText,
				ExpectedMechanism:     maxText,
			},
		}}}
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Signal:     katypes.SignalContext{Namespace: "prod", ResourceKind: "Deployment", ResourceName: "api"},
			EnrichData: &prompt.EnrichmentData{HistoryResult: history},
		}, "rr-prompt-budget-current-2533")
		Expect(err).NotTo(HaveOccurred())
		builder, err := prompt.NewBuilder()
		Expect(err).NotTo(HaveOccurred())
		rendered, err := builder.RenderWorkflowSelection(prompt.WorkflowSelectionInput{
			EnrichData:      frozen.source.EnrichData,
			EvidenceHandoff: frozen.serialized,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(rendered)).To(BeNumerically("<=", katypes.DefaultStructuredPhase3ContextBudget))
		// The decision-context values are rendered in both the human-readable
		// history and frozen JSON handoff; the complete prompt budget check keeps
		// that intentional duplication bounded before provider dispatch.
		Expect(strings.Count(rendered, maxText)).To(Equal(8))
		Expect(validateWorkflowSelectionPromptBudget(rendered, len(rendered))).NotTo(HaveOccurred())
		budgetErr := validateWorkflowSelectionPromptBudget(rendered, len(rendered)-1)
		Expect(budgetErr).To(MatchError("workflow selection prompt exceeds configured byte budget"))
		var typedBudgetErr *internalerrors.PromptBudgetError
		Expect(errors.As(budgetErr, &typedBudgetErr)).To(BeTrue())
		Expect(typedBudgetErr.PromptBytes).To(Equal(len(rendered)))
		Expect(typedBudgetErr.BudgetBytes).To(Equal(len(rendered) - 1))
		Expect(rendered).To(ContainSubstring("Historical decision context"))
	})
})
