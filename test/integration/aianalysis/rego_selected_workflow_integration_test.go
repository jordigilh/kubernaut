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

package aianalysis

import (
	"context"
	"path/filepath"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
)

// IT-AA-2714: End-to-end selected-workflow identity chain through the real
// OPA evaluator (Issue #2714, BR-AI-085 FR-AI-085-007).
//
// FedRAMP AC-6 (least privilege): proves selective autonomy can be scoped to
// a specific reviewed workflow identity (catalog-authoritative name+version)
// through a hot-reloaded policy, narrowing auto-approval blast radius.
var _ = Describe("Rego Selected Workflow Integration — #2714, BR-AI-085", Label("integration", "rego", "selected-workflow"), func() {
	var (
		evaluator *rego.Evaluator
		evalCtx   context.Context
		cancel    context.CancelFunc
	)

	BeforeEach(func() {
		policyPath := filepath.Join("testdata", "policies", "selected_workflow_approval.rego")
		evaluator = rego.NewEvaluator(rego.Config{
			PolicyPath: policyPath,
		}, logr.Discard())

		evalCtx, cancel = context.WithCancel(context.Background())
		err := evaluator.StartHotReload(evalCtx)
		Expect(err).NotTo(HaveOccurred(), "Selected-workflow policy should load successfully")
	})

	AfterEach(func() {
		if evaluator != nil {
			evaluator.Stop()
		}
		if cancel != nil {
			cancel()
		}
	})

	Describe("IT-AA-2714-001: End-to-end chain — PolicyInput -> Rego evaluator sees selected_workflow", func() {
		It("should auto-approve when PolicyInput has the trusted workflow identity", func() {
			input := &rego.PolicyInput{
				SignalContext: rego.SignalContextInput{SignalType: "alert", Severity: "high", Environment: "production"},
				KAResponse:    rego.KAResponseInput{Confidence: 0.95},
				ActionType:    "ScaleMemory",
				TargetResource: rego.TargetResourceInput{
					Kind:      "Deployment",
					Name:      "payment-api",
					Namespace: "production",
				},
				SelectedWorkflow: &rego.SelectedWorkflowInput{
					ID:      "a93e5f6a-d967-510d-8faa-9a83c5420e78",
					Name:    "increase-memory-limits-gitops-v1",
					Version: "1.4.1",
				},
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeFalse(),
				"trusted workflow identity should auto-approve via selected_workflow_approval.rego")
			Expect(result.Reason).To(ContainSubstring("trusted workflow identity"))
		})
	})

	Describe("IT-AA-2714-002: Non-trusted workflow identity requires approval", func() {
		It("should require approval when the workflow identity is not in the trusted set", func() {
			input := &rego.PolicyInput{
				SignalContext: rego.SignalContextInput{SignalType: "alert", Severity: "high", Environment: "production"},
				KAResponse:    rego.KAResponseInput{Confidence: 0.95},
				ActionType:    "ScaleMemory",
				TargetResource: rego.TargetResourceInput{
					Kind:      "Deployment",
					Name:      "payment-api",
					Namespace: "production",
				},
				SelectedWorkflow: &rego.SelectedWorkflowInput{
					ID:      "99999999-8888-7777-6666-555555555555",
					Name:    "restart-pod-v1",
					Version: "2.0.0",
				},
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue(),
				"non-trusted workflow identity should require approval")
		})
	})

	Describe("IT-AA-2714-003: Nil selected workflow through the real evaluator (backward compatibility)", func() {
		It("should evaluate without error and keep the policy default when SelectedWorkflow is nil", func() {
			input := &rego.PolicyInput{
				SignalContext: rego.SignalContextInput{SignalType: "alert", Severity: "warning", Environment: "staging"},
				KAResponse:    rego.KAResponseInput{Confidence: 0.90},
				ActionType:    "RestartPod",
				TargetResource: rego.TargetResourceInput{
					Kind:      "Deployment",
					Name:      "worker",
					Namespace: "staging",
				},
				SelectedWorkflow: nil,
			}

			result, err := evaluator.Evaluate(context.Background(), input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.ApprovalRequired).To(BeTrue(),
				"absent selected_workflow must not auto-approve via workflow-identity rules")
			Expect(result.Degraded).To(BeFalse())
		})
	})
})
