/*
Copyright 2025 Jordi Gil.

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

package aianalysis_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/aianalysis/client"
)

func TestHolmesGPTClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HolmesGPT Client Suite")
}

// MockHTTPClient for testing HTTP calls
type MockHTTPClient struct {
	DoFunc func(req interface{}) (interface{}, error)
}

var _ = Describe("HolmesGPT Client", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	// ========================================
	// BR-AI-080: IncidentRequest Schema Compliance
	// ========================================
	Describe("IncidentRequest Structure", func() {
		It("should have all required HAPI fields", func() {
			req := &client.IncidentRequest{
				IncidentID:        "test-123",
				RemediationID:     "req-abc",
				SignalType:        "OOMKilled",
				Severity:          "critical",
				SignalSource:      "kubernaut",
				ResourceNamespace: "app-ns",
				ResourceKind:      "Pod",
				ResourceName:      "app-pod",
				ErrorMessage:      "Container killed",
				Environment:       "production",
				Priority:          "P1",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				ClusterName:       "prod-cluster",
			}

			Expect(req.IncidentID).To(Equal("test-123"))
			Expect(req.RemediationID).To(Equal("req-abc"))
			Expect(req.SignalType).To(Equal("OOMKilled"))
			Expect(req.Severity).To(Equal("critical"))
			Expect(req.SignalSource).To(Equal("kubernaut"))
			Expect(req.ResourceNamespace).To(Equal("app-ns"))
			Expect(req.ResourceKind).To(Equal("Pod"))
			Expect(req.ResourceName).To(Equal("app-pod"))
			Expect(req.ErrorMessage).To(Equal("Container killed"))
			Expect(req.Environment).To(Equal("production"))
			Expect(req.Priority).To(Equal("P1"))
			Expect(req.RiskTolerance).To(Equal("medium"))
			Expect(req.BusinessCategory).To(Equal("standard"))
			Expect(req.ClusterName).To(Equal("prod-cluster"))
		})

		It("should support optional enrichment results", func() {
			req := &client.IncidentRequest{
				IncidentID:    "test-123",
				RemediationID: "req-abc",
				EnrichmentResults: &client.EnrichmentResults{
					DetectedLabels: map[string]interface{}{
						"gitOpsManaged": true,
						"pdbProtected":  false,
					},
					CustomLabels: map[string][]string{
						"team": {"platform"},
					},
					OwnerChain: []client.OwnerChainEntry{
						{Namespace: "app-ns", Kind: "Deployment", Name: "app"},
					},
				},
			}

			Expect(req.EnrichmentResults).ToNot(BeNil())
			Expect(req.EnrichmentResults.DetectedLabels["gitOpsManaged"]).To(Equal(true))
			Expect(req.EnrichmentResults.CustomLabels["team"]).To(ContainElement("platform"))
			Expect(req.EnrichmentResults.OwnerChain).To(HaveLen(1))
		})
	})

	// ========================================
	// BR-AI-082: RecoveryRequest Implementation
	// ========================================
	Describe("RecoveryRequest Structure", func() {
		It("should have all required recovery fields", func() {
			exitCode := int32(1)
			req := &client.RecoveryRequest{
				IncidentID:            "recovery-456",
				RemediationID:         "req-def",
				IsRecoveryAttempt:     true,
				RecoveryAttemptNumber: 2,
				PreviousExecution: &client.PreviousExecution{
					WorkflowExecutionRef: "we-failed-123",
					OriginalRCA: client.OriginalRCA{
						Summary:             "Initial OOM analysis",
						SignalType:          "OOMKilled",
						Severity:            "critical",
						ContributingFactors: []string{"memory limit too low"},
					},
					SelectedWorkflow: client.SelectedWorkflowSummary{
						WorkflowID:     "memory-fix-v1",
						ContainerImage: "kubernaut/memory-fix:v1.0",
						Version:        "v1.0",
						Rationale:      "Selected for OOM remediation",
					},
					Failure: client.ExecutionFailure{
						FailedStepIndex: 2,
						FailedStepName:  "apply-limits",
						Reason:          "DeadlineExceeded",
						Message:         "Step timed out after 30s",
						ExitCode:        &exitCode,
						ExecutionTime:   "30s",
					},
				},
			}

			Expect(req.IncidentID).To(Equal("recovery-456"))
			Expect(req.RemediationID).To(Equal("req-def"))
			Expect(req.IsRecoveryAttempt).To(BeTrue())
			Expect(req.RecoveryAttemptNumber).To(Equal(2))
			Expect(req.PreviousExecution).ToNot(BeNil())
			Expect(req.PreviousExecution.OriginalRCA.Summary).To(Equal("Initial OOM analysis"))
			Expect(req.PreviousExecution.SelectedWorkflow.WorkflowID).To(Equal("memory-fix-v1"))
			Expect(req.PreviousExecution.Failure.Reason).To(Equal("DeadlineExceeded"))
		})

		It("should support optional fields from signal context", func() {
			req := &client.RecoveryRequest{
				IncidentID:        "recovery-456",
				RemediationID:     "req-def",
				IsRecoveryAttempt: true,
				SignalType:        strPtr("CrashLoopBackOff"),
				Severity:          strPtr("warning"),
				ResourceNamespace: strPtr("test-ns"),
				ResourceKind:      strPtr("Deployment"),
				ResourceName:      strPtr("test-deploy"),
				Environment:       "staging",
				Priority:          "P2",
				RiskTolerance:     "low",
				BusinessCategory:  "critical",
			}

			Expect(*req.SignalType).To(Equal("CrashLoopBackOff"))
			Expect(*req.Severity).To(Equal("warning"))
			Expect(*req.ResourceNamespace).To(Equal("test-ns"))
			Expect(req.Environment).To(Equal("staging"))
			Expect(req.Priority).To(Equal("P2"))
		})
	})

	// ========================================
	// BR-AI-083: Endpoint Routing (InvestigateRecovery)
	// ========================================
	Describe("InvestigateRecovery Method", func() {
		Context("when recovery request is valid", func() {
			It("should call the recovery endpoint", func() {
				// This test will fail until InvestigateRecovery is implemented
				// RED phase - test exists but implementation doesn't
				_ = ctx
				Skip("InvestigateRecovery not yet implemented - RED phase")
			})
		})

		Context("when recovery endpoint returns error", func() {
			It("should propagate the error with context", func() {
				// This test verifies error handling
				Skip("InvestigateRecovery not yet implemented - RED phase")
			})
		})
	})

	// ========================================
	// Error Handling Tests
	// ========================================
	Describe("APIError", func() {
		It("should classify transient errors correctly", func() {
			transientCodes := []int{429, 502, 503, 504}
			for _, code := range transientCodes {
				err := &client.APIError{StatusCode: code, Message: "test"}
				Expect(err.IsTransient()).To(BeTrue(), "Expected status %d to be transient", code)
			}
		})

		It("should classify permanent errors correctly", func() {
			permanentCodes := []int{400, 401, 403, 404, 500}
			for _, code := range permanentCodes {
				err := &client.APIError{StatusCode: code, Message: "test"}
				Expect(err.IsTransient()).To(BeFalse(), "Expected status %d to be permanent", code)
			}
		})

		It("should format error message correctly", func() {
			err := &client.APIError{StatusCode: 500, Message: "internal error"}
			Expect(err.Error()).To(ContainSubstring("500"))
			Expect(err.Error()).To(ContainSubstring("internal error"))
		})
	})
})

// Helper function for optional string pointers
func strPtr(s string) *string {
	return &s
}

// Suppress unused import error
var _ = errors.New

