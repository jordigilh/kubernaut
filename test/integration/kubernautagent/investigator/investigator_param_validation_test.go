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

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("BR-HAPI-191: Parameter Validation Self-Correction Integration (#1170)", func() {

	var (
		invLogger  logr.Logger
		auditStore *recordingAuditStore
		builder    *prompt.Builder
		rp         *parser.ResultParser
		enricher   *enrichment.Enricher
		phaseTools katypes.PhaseToolMap
	)

	BeforeEach(func() {
		invLogger = logr.Discard()
		auditStore = &recordingAuditStore{}
		builder, _ = prompt.NewBuilder()
		rp = parser.NewResultParser()
		k8sClient := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{}}
		dsClient := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
		enricher = enrichment.NewEnricher(k8sClient, dsClient, auditStore, invLogger)
		phaseTools = investigator.DefaultPhaseToolMap()
	})

	Describe("IT-KA-1170-001: Invalid params trigger self-correction with schema hint", func() {
		It("should self-correct from bad params to valid params", func() {
			min := float64(1)
			max := float64(10)
			validator := parser.NewValidator([]string{"scale-deployment"})
			validator.SetWorkflowMeta("scale-deployment", parser.WorkflowMeta{
				ExecutionEngine: "tekton",
				ExecutionBundle: "quay.io/test/scale:v1",
				Parameters: []models.WorkflowParameter{
					{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Number of replicas", Minimum: &min, Maximum: &max},
					{Name: "NAMESPACE", Type: "string", Required: true, Description: "Target namespace"},
				},
			})

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"OOM kills detected","confidence":0.9}`}},
					wfToolResp(`{"workflow_id":"scale-deployment","confidence":0.85,"parameters":{"REPLICA_COUNT":"not-a-number","EXTRA_PARAM":"hallucinated"}}`),
					wfToolResp(`{"workflow_id":"scale-deployment","confidence":0.85,"parameters":{"REPLICA_COUNT":3,"NAMESPACE":"production"}}`),
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
				Pipeline: investigator.Pipeline{
					CatalogFetcher:  &staticCatalogFetcher{validator: validator},
					AnomalyDetector: investigator.NewAnomalyDetector(investigator.DefaultAnomalyConfig(), nil),
				},
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "oom-alert", Namespace: "production", Severity: "critical", Message: "OOM Kill",
				Environment: "Production", Priority: "P0",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.WorkflowID).To(Equal("scale-deployment"),
				"Self-correction should produce a valid workflow")
			Expect(result.HumanReviewNeeded).To(BeFalse())

			By("recording validation attempts in history")
			Expect(result.ValidationAttemptsHistory).To(HaveLen(2))

			firstAttempt := result.ValidationAttemptsHistory[0]
			Expect(firstAttempt.IsValid).To(BeFalse(),
				"First attempt should fail due to bad parameters")
			Expect(len(firstAttempt.Errors)).To(BeNumerically(">=", 1),
				"First attempt should record parameter validation errors")

			secondAttempt := result.ValidationAttemptsHistory[1]
			Expect(secondAttempt.IsValid).To(BeTrue(),
				"Second attempt should pass with corrected parameters")

			By("sending structured feedback with schema hint to LLM")
			Expect(len(mockClient.calls)).To(BeNumerically(">=", 3),
				"Should have RCA + bad workflow + correction calls")
			correctionCall := mockClient.calls[len(mockClient.calls)-1]
			lastUserMsg := ""
			for i := len(correctionCall.Messages) - 1; i >= 0; i-- {
				if correctionCall.Messages[i].Role == "user" {
					lastUserMsg = correctionCall.Messages[i].Content
					break
				}
			}
			Expect(lastUserMsg).To(ContainSubstring("REPLICA_COUNT"),
				"Correction message should mention the failing parameter")
			Expect(lastUserMsg).To(ContainSubstring("Expected parameters"),
				"Correction message should include schema hint")
		})
	})

	Describe("IT-KA-1170-002: Corrected params pass validation on retry", func() {
		It("should accept valid params on first try without self-correction", func() {
			validator := parser.NewValidator([]string{"restart-pod"})
			validator.SetWorkflowMeta("restart-pod", parser.WorkflowMeta{
				ExecutionEngine: "tekton",
				ExecutionBundle: "quay.io/test/restart:v1",
				Parameters: []models.WorkflowParameter{
					{Name: "POD_NAME", Type: "string", Required: true, Description: "Pod to restart"},
				},
			})

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"CrashLoopBackOff","confidence":0.9}`}},
					wfToolResp(`{"workflow_id":"restart-pod","confidence":0.8,"parameters":{"POD_NAME":"my-pod-abc"}}`),
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
				Pipeline: investigator.Pipeline{
					CatalogFetcher:  &staticCatalogFetcher{validator: validator},
					AnomalyDetector: investigator.NewAnomalyDetector(investigator.DefaultAnomalyConfig(), nil),
				},
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "crash-alert", Namespace: "default", Severity: "high", Message: "CrashLoopBackOff",
				Environment: "Development", Priority: "P2",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.WorkflowID).To(Equal("restart-pod"))
			Expect(result.HumanReviewNeeded).To(BeFalse())
			Expect(result.ValidationAttemptsHistory).To(HaveLen(1))
			Expect(result.ValidationAttemptsHistory[0].IsValid).To(BeTrue(),
				"Valid params should pass on first attempt")
		})
	})

	Describe("IT-KA-1170-003: Undeclared params stripped in final result", func() {
		It("should remove hallucinated parameters from the final result", func() {
			validator := parser.NewValidator([]string{"drain-node"})
			validator.SetWorkflowMeta("drain-node", parser.WorkflowMeta{
				ExecutionEngine: "tekton",
				Parameters: []models.WorkflowParameter{
					{Name: "NODE_NAME", Type: "string", Required: true, Description: "Node to drain"},
				},
			})

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"Node pressure","confidence":0.9}`}},
					wfToolResp(`{"workflow_id":"drain-node","confidence":0.85,"parameters":{"NODE_NAME":"worker-1","HALLUCINATED":"evil","TARGET_RESOURCE_NAME":"worker-1","TARGET_RESOURCE_KIND":"Node","TARGET_RESOURCE_NAMESPACE":"","TARGET_RESOURCE_API_VERSION":"v1"}}`),
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
				Pipeline: investigator.Pipeline{
					CatalogFetcher:  &staticCatalogFetcher{validator: validator},
					AnomalyDetector: investigator.NewAnomalyDetector(investigator.DefaultAnomalyConfig(), nil),
				},
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "node-pressure", Namespace: "default", Severity: "high", Message: "NodePressure",
				Environment: "Development", Priority: "P2",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.WorkflowID).To(Equal("drain-node"))
			Expect(result.Parameters).To(HaveKey("NODE_NAME"),
				"Declared params should be preserved")
			Expect(result.Parameters).NotTo(HaveKey("HALLUCINATED"),
				"Undeclared params should be stripped")
			Expect(result.Parameters).To(HaveKey("TARGET_RESOURCE_NAME"),
				"KA-managed params should be preserved")
			Expect(result.Parameters).To(HaveKey("TARGET_RESOURCE_KIND"),
				"KA-managed params should be preserved")
		})
	})

	Describe("IT-KA-1170-004: validation_attempts_history shows errors and schema hint in audit", func() {
		It("should emit validation_attempt audit events with error details", func() {
			validator := parser.NewValidator([]string{"scale-deployment"})
			validator.SetWorkflowMeta("scale-deployment", parser.WorkflowMeta{
				ExecutionEngine: "tekton",
				Parameters: []models.WorkflowParameter{
					{Name: "REPLICA_COUNT", Type: "integer", Required: true, Description: "Replicas"},
				},
			})

			mockClient := &mockLLMClient{
				responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", Content: `{"rca_summary":"scaling needed","confidence":0.9}`}},
					wfToolResp(`{"workflow_id":"scale-deployment","confidence":0.85,"parameters":{"REPLICA_COUNT":"bad"}}`),
					wfToolResp(`{"workflow_id":"scale-deployment","confidence":0.85,"parameters":{"REPLICA_COUNT":5}}`),
				},
			}

			inv := investigator.New(investigator.Config{
				Client: mockClient, Builder: builder, ResultParser: rp,
				Enricher: enricher, AuditStore: auditStore, Logger: invLogger,
				MaxTurns: 15, PhaseTools: phaseTools,
				Pipeline: investigator.Pipeline{
					CatalogFetcher:  &staticCatalogFetcher{validator: validator},
					AnomalyDetector: investigator.NewAnomalyDetector(investigator.DefaultAnomalyConfig(), nil),
				},
			})

			result, err := inv.Investigate(context.Background(), katypes.SignalContext{
				Name: "scale-alert", Namespace: "prod", Severity: "warning", Message: "High CPU",
				Environment: "Production", Priority: "P1",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.WorkflowID).To(Equal("scale-deployment"))

			validationEvents := filterEvents(auditStore.events, audit.EventTypeValidationAttempt)
			Expect(len(validationEvents)).To(BeNumerically(">=", 2),
				"Should emit at least 2 validation_attempt audit events (fail + pass)")

			failEvent := validationEvents[0]
			Expect(failEvent.Data["is_valid"]).To(BeFalse())
			errList, ok := failEvent.Data["errors"].([]string)
			Expect(ok).To(BeTrue(), "errors should be []string")
			Expect(len(errList)).To(BeNumerically(">=", 1))
			Expect(errList[0]).To(ContainSubstring("REPLICA_COUNT"),
				"Audit event should contain parameter-level error detail")
		})
	})
})
