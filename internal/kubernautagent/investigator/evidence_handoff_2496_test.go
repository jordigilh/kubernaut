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
	"math"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/sanitization"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

type restrictedStructuredContextPolicy2496 struct{}

func (restrictedStructuredContextPolicy2496) Name() string {
	return "restricted-structured-context-policy"
}

func (restrictedStructuredContextPolicy2496) Sanitize(_ context.Context, input string) (string, error) {
	if strings.Contains(input, "restricted-context-2496") {
		return "", &structuredContextPolicyError2496{}
	}
	return input, nil
}

type structuredContextPolicyError2496 struct{}

func (*structuredContextPolicyError2496) Error() string {
	return "operator policy rejected restricted context"
}

var _ = Describe("#2496 structured Phase 3 handoff after Issue #2521", func() {
	It("UT-KA-2496-001: target-scoped history remains the only repeat-selection source", func() {
		inv := New(Config{Logger: logr.Discard()})
		score := .5
		input := handoffInput{
			Signal: katypes.SignalContext{ResourceKind: "Deployment", ResourceName: "api", Namespace: "prod", ResourceAPIVersion: "apps/v1"},
			EnrichData: &prompt.EnrichmentData{HistoryResult: &enrichment.RemediationHistoryResult{
				TargetResource: "prod/Deployment/api",
				Tier1:          []enrichment.Tier1Entry{{RemediationUID: "prior-2496", WorkflowID: "same-workflow", WorkflowName: "restart-api", AssessmentReason: "Full", EffectivenessScore: &score}},
			}},
		}
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), input, "rr-current-2496")
		Expect(err).NotTo(HaveOccurred())
		Expect(frozen.repeat.CompletedPriorWorkflows).To(HaveKeyWithValue("same-workflow", []string{"prior-2496"}))

		input.EnrichData.HistoryResult.TargetResource = "prod/Deployment/other"
		other, err := inv.freezeEvidenceHandoff(context.Background(), input, "rr-other-2496")
		Expect(err).NotTo(HaveOccurred())
		Expect(other.repeat.CompletedPriorWorkflows).NotTo(HaveKey("same-workflow"))
	})

	DescribeTable("UT-KA-2521-010: cluster-scoped history uses the canonical target identity", func(rawTarget string) {
		inv := New(Config{Logger: logr.Discard()})
		score := 0.5
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Signal: katypes.SignalContext{ResourceKind: "Node", ResourceName: "worker-1"},
			EnrichData: &prompt.EnrichmentData{HistoryResult: &enrichment.RemediationHistoryResult{
				TargetResource: rawTarget,
				Tier1: []enrichment.Tier1Entry{{
					RemediationUID: "prior-cluster-2521", WorkflowID: "workflow-cluster", WorkflowName: "repair-node",
					AssessmentReason: "Full", EffectivenessScore: &score,
				}},
			}},
		}, "rr-cluster-2521")
		Expect(err).NotTo(HaveOccurred())
		Expect(frozen.repeat.CompletedPriorWorkflows).To(HaveKeyWithValue("workflow-cluster", []string{"prior-cluster-2521"}))
	},
		Entry("canonical", "Node/worker-1"),
		Entry("leading slash", "/Node/worker-1"),
		Entry("legacy cluster prefix", "cluster/Node/worker-1"),
	)

	It("UT-KA-2521-010: known cluster history survives a stale namespace after scope lookup failure", func() {
		inv := New(Config{Logger: logr.Discard()})
		score := 0.5
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Signal: katypes.SignalContext{ResourceKind: "Node", ResourceName: "worker-1", Namespace: "stale-namespace"},
			EnrichData: &prompt.EnrichmentData{HistoryResult: &enrichment.RemediationHistoryResult{
				TargetResource: "Node/worker-1",
				Tier1: []enrichment.Tier1Entry{{
					RemediationUID: "prior-stale-scope", WorkflowID: "workflow-stale-scope", WorkflowName: "repair-node",
					AssessmentReason: "Full", EffectivenessScore: &score,
				}},
			}},
		}, "rr-stale-scope-2521")
		Expect(err).NotTo(HaveOccurred())
		Expect(frozen.repeat.CompletedPriorWorkflows).To(HaveKey("workflow-stale-scope"))
	})

	It("UT-KA-2521-121: normalizes custom cluster-scoped history before matching", func() {
		mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Group: "example.com", Version: "v1"}})
		mapper.Add(schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}, meta.RESTScopeRoot)
		inv := New(Config{Logger: logr.Discard(), ScopeResolver: NewMapperScopeResolver(mapper)})
		score := 0.5
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Signal: katypes.SignalContext{ResourceKind: "Widget", ResourceName: "api", Namespace: "prod"},
			EnrichData: &prompt.EnrichmentData{HistoryResult: &enrichment.RemediationHistoryResult{
				TargetResource: "Widget/api",
				Tier1: []enrichment.Tier1Entry{{
					RemediationUID: "prior-custom-cluster", WorkflowID: "workflow-custom-cluster",
					WorkflowName: "repair-widget", AssessmentReason: "Full", EffectivenessScore: &score,
				}},
			}},
		}, "rr-custom-cluster-2521")
		Expect(err).NotTo(HaveOccurred())
		Expect(frozen.repeat.CompletedPriorWorkflows).To(HaveKeyWithValue("workflow-custom-cluster", []string{"prior-custom-cluster"}))
	})

	It("UT-KA-2496-008: source values are sanitized without retaining raw PhaseRCA observations", func() {
		inv := New(Config{Logger: logr.Discard()})
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			RCASummary: "password=rca-private-value",
			EnrichData: &prompt.EnrichmentData{DetectedLabels: map[string]string{"safe-label": "password=label-private-value"}},
		}, "rr-privacy-2496")
		Expect(err).NotTo(HaveOccurred())
		Expect(frozen.serialized).NotTo(ContainSubstring("rca-private-value"))
		Expect(frozen.serialized).NotTo(ContainSubstring("label-private-value"))
		Expect(frozen.serialized).To(ContainSubstring("REDACTED"))
		Expect(frozen.serialized).NotTo(ContainSubstring("current_observations"))
	})

	It("UT-KA-2496-010: structured context policy failure blocks the complete handoff", func() {
		inv := New(Config{Logger: logr.Discard(), Pipeline: Pipeline{
			Sanitizer: sanitization.NewPipeline(restrictedStructuredContextPolicy2496{}),
		}})
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{RCASummary: "restricted-context-2496"}, "rr-policy-2496")
		Expect(err).To(MatchError(ContainSubstring("operator policy rejected restricted context")))
		Expect(frozen).To(BeNil())
	})

	It("UT-KA-2496-010: fixed structured-context budget remains fail-closed", func() {
		inv := New(Config{Logger: logr.Discard(), Pipeline: Pipeline{StructuredPhase3ContextBudget: 2048}})
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			RCASummary: strings.Repeat("structured RCA detail ", 200),
		}, "rr-budget-2496")
		Expect(err).To(MatchError(ContainSubstring("byte budget")))
		Expect(frozen).To(BeNil())
	})

	It("UT-KA-2496-010: non-finite RCA values cannot produce a frozen packet", func() {
		score := math.Inf(1)
		inv := New(Config{Logger: logr.Discard()})
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Phase1: &prompt.Phase1Data{RCAConfidence: &score},
		}, "rr-nonfinite-2496")
		Expect(err).To(MatchError(ContainSubstring("serialize handoff source")))
		Expect(frozen).To(BeNil())
	})

	It("UT-KA-2496-010: malformed target API versions remain unknown without rewriting source input", func() {
		inv := New(Config{Logger: logr.Discard()})
		frozen, err := inv.freezeEvidenceHandoff(context.Background(), handoffInput{
			Signal: katypes.SignalContext{ResourceKind: "Subscription", ResourceName: "etcd", Namespace: "demo", ResourceAPIVersion: "../../etc/passwd"},
		}, "rr-target-2496")
		Expect(err).NotTo(HaveOccurred())
		Expect(frozen.source.Signal.ResourceAPIVersion).To(Equal("../../etc/passwd"))
		var packet evidenceHandoff
		Expect(json.Unmarshal([]byte(frozen.serialized), &packet)).To(Succeed())
		Expect(packet.Target.APIGroup).To(BeNil())
		Expect(packet.Target.APIVersion).To(BeNil())
		Expect(packet.Unknowns).To(ContainElement("target API group/version could not be parsed"))
	})

	It("UT-KA-2496-010: digest excludes runtime metadata and changes with structured RCA", func() {
		packet, err := buildEvidenceHandoff(handoffInput{RCASummary: "Memory pressure"})
		Expect(err).NotTo(HaveOccurred())
		at := time.Date(2026, time.October, 10, 15, 0, 0, 0, time.UTC)
		wire, digest, err := packet.serializeFrozen(at, katypes.DefaultStructuredPhase3ContextBudget)
		Expect(err).NotTo(HaveOccurred())
		later, laterDigest, err := packet.serializeFrozen(at.Add(time.Second), katypes.DefaultStructuredPhase3ContextBudget)
		Expect(err).NotTo(HaveOccurred())
		Expect(later).NotTo(Equal(wire))
		Expect(laterDigest).To(Equal(digest))
		packet.RCA.Summary = "New structured RCA"
		_, changedDigest, err := packet.serializeFrozen(at, katypes.DefaultStructuredPhase3ContextBudget)
		Expect(err).NotTo(HaveOccurred())
		Expect(changedDigest).NotTo(Equal(digest))
	})

	It("UT-KA-2511-011: repeat context requires catalog identity and a valid assessment", func() {
		packet, err := buildEvidenceHandoff(handoffInput{EnrichData: &prompt.EnrichmentData{
			HistoryResult: &enrichment.RemediationHistoryResult{Tier1: []enrichment.Tier1Entry{{
				RemediationUID: "rr-unknown-2496", WorkflowRationale: "model rationale only",
			}}},
		}})
		Expect(err).NotTo(HaveOccurred())
		Expect(repeatContextFromHandoff(packet).CompletedPriorWorkflows).To(BeEmpty())
	})
})
