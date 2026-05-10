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

package investigator_test

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// gateAuditStore captures audit events for alignment gate assertions.
type gateAuditStore struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (s *gateAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *gateAuditStore) eventsByAction(action string) []*audit.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*audit.AuditEvent
	for _, e := range s.events {
		if e.EventAction == action {
			out = append(out, e)
		}
	}
	return out
}

var _ = Describe("Target-Workflow Alignment Gate — #934", func() {

	var (
		store  *gateAuditStore
		logger logr.Logger
	)

	BeforeEach(func() {
		store = &gateAuditStore{}
		logger = logr.Discard()
	})

	Describe("UT-KA-934-008: Mismatch emits audit event with failure outcome", func() {
		It("should emit a workflow_target_alignment_gate audit event with failure outcome", func() {
			v := parser.NewValidator([]string{"wf-pod-only"})
			v.SetWorkflowMeta("wf-pod-only", parser.WorkflowMeta{
				Component: []string{"v1/Pod"},
			})

			result := &katypes.InvestigationResult{
				WorkflowID: "wf-pod-only",
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Node", Name: "worker-1", Namespace: "",
				},
			}

			investigator.CheckWorkflowTargetAlignment(context.Background(), result, v, "corr-001", store, logger)

			gateEvents := store.eventsByAction("workflow_target_alignment_gate")
			Expect(gateEvents).To(HaveLen(1), "exactly one alignment gate audit event expected")
			Expect(gateEvents[0].EventType).To(Equal(audit.EventTypeLLMRequest),
				"must use EventTypeLLMRequest for DS persistence (consistent with sameKindValidationGate)")
			Expect(gateEvents[0].EventOutcome).To(Equal(audit.OutcomeFailure))
			Expect(gateEvents[0].Data["target_kind"]).To(Equal("Node"))
			Expect(gateEvents[0].Data["workflow_id"]).To(Equal("wf-pod-only"))
			Expect(gateEvents[0].Data["aligned"]).To(BeFalse())
		})
	})

	Describe("UT-KA-934-009: Match emits audit event with success outcome", func() {
		It("should emit a workflow_target_alignment_gate audit event with success outcome", func() {
			v := parser.NewValidator([]string{"wf-deploy"})
			v.SetWorkflowMeta("wf-deploy", parser.WorkflowMeta{
				Component: []string{"apps/v1/Deployment"},
			})

			result := &katypes.InvestigationResult{
				WorkflowID: "wf-deploy",
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Deployment", Name: "api-server", Namespace: "production",
				},
			}

			investigator.CheckWorkflowTargetAlignment(context.Background(), result, v, "corr-002", store, logger)

			gateEvents := store.eventsByAction("workflow_target_alignment_gate")
			Expect(gateEvents).To(HaveLen(1))
			Expect(gateEvents[0].EventOutcome).To(Equal(audit.OutcomeSuccess))
			Expect(gateEvents[0].Data["aligned"]).To(BeTrue())
		})
	})

	Describe("UT-KA-934-010: Mismatch appends warning without HR escalation", func() {
		It("should append a warning to result.Warnings but NOT set HumanReviewNeeded", func() {
			v := parser.NewValidator([]string{"wf-pod-only"})
			v.SetWorkflowMeta("wf-pod-only", parser.WorkflowMeta{
				Component: []string{"v1/Pod"},
			})

			result := &katypes.InvestigationResult{
				WorkflowID: "wf-pod-only",
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Node", Name: "worker-1", Namespace: "",
				},
			}

			investigator.CheckWorkflowTargetAlignment(context.Background(), result, v, "corr-003", store, logger)

			Expect(result.Warnings).To(HaveLen(1))
			Expect(result.Warnings[0]).To(ContainSubstring("target kind"))
			Expect(result.Warnings[0]).To(ContainSubstring("Node"))
			Expect(result.HumanReviewNeeded).To(BeFalse(),
				"UT-KA-934-010: warning-level gate must NOT escalate to human review")
		})
	})

	Describe("UT-KA-934-011: Match does not append warning", func() {
		It("should not modify result.Warnings when target is aligned with workflow component", func() {
			v := parser.NewValidator([]string{"wf-deploy"})
			v.SetWorkflowMeta("wf-deploy", parser.WorkflowMeta{
				Component: []string{"apps/v1/Deployment"},
			})

			result := &katypes.InvestigationResult{
				WorkflowID: "wf-deploy",
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Deployment", Name: "api-server", Namespace: "production",
				},
			}

			investigator.CheckWorkflowTargetAlignment(context.Background(), result, v, "corr-004", store, logger)

			Expect(result.Warnings).To(BeEmpty(),
				"UT-KA-934-011: aligned target must NOT produce a warning")
		})
	})

	Describe("UT-KA-934-012: Gate skipped when WorkflowID empty", func() {
		It("should be a no-op when no workflow was selected", func() {
			v := parser.NewValidator([]string{"wf-deploy"})
			result := &katypes.InvestigationResult{
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Node", Name: "worker-1",
				},
			}

			investigator.CheckWorkflowTargetAlignment(context.Background(), result, v, "corr-005", store, logger)

			Expect(store.eventsByAction("workflow_target_alignment_gate")).To(BeEmpty(),
				"UT-KA-934-012: no audit event when WorkflowID is empty")
			Expect(result.Warnings).To(BeEmpty())
		})
	})

	Describe("UT-KA-934-013: Gate skipped when workflow metadata not found", func() {
		It("should be a no-op when the catalog has no metadata for the selected workflow", func() {
			v := parser.NewValidator([]string{"wf-unknown"})

			result := &katypes.InvestigationResult{
				WorkflowID: "wf-unknown",
				RemediationTarget: katypes.RemediationTarget{
					Kind: "Deployment", Name: "api-server", Namespace: "production",
				},
			}

			investigator.CheckWorkflowTargetAlignment(context.Background(), result, v, "corr-006", store, logger)

			Expect(store.eventsByAction("workflow_target_alignment_gate")).To(BeEmpty(),
				"UT-KA-934-013: no audit event when workflow metadata missing from catalog")
			Expect(result.Warnings).To(BeEmpty())
		})
	})
})
