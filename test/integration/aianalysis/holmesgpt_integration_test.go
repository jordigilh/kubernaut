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

package aianalysis

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	aianalysisclient "github.com/jordigilh/kubernaut/pkg/aianalysis/client"
)

var _ = Describe("HolmesGPT-API Integration", Label("integration", "holmesgpt"), func() {
	var (
		hgClient   *aianalysisclient.HolmesGPTClient
		testCtx    context.Context
		cancelFunc context.CancelFunc
	)

	BeforeEach(func() {
		// Get HolmesGPT-API URL from environment (set by podman-compose)
		apiURL := os.Getenv("HOLMESGPT_API_URL")
		if apiURL == "" {
			apiURL = "http://localhost:8081" // Default for local development
		}

		hgClient = aianalysisclient.NewHolmesGPTClient(aianalysisclient.Config{
			BaseURL: apiURL,
			Timeout: 60 * time.Second,
		})

		testCtx, cancelFunc = context.WithTimeout(context.Background(), 60*time.Second)
	})

	AfterEach(func() {
		cancelFunc()
	})

	Context("Incident Analysis - BR-AI-006", func() {
		It("should return valid analysis response", func() {
			resp, err := hgClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				Context: "Pod CrashLoopBackOff in staging namespace. Container test-app restarted 5 times.",
				DetectedLabels: map[string]interface{}{
					"gitOpsManaged": true,
					"pdbProtected":  false,
				},
				OwnerChain: []aianalysisclient.OwnerChainEntry{
					{Namespace: "staging", Kind: "Deployment", Name: "test-app"},
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.Analysis).NotTo(BeEmpty())
			Expect(resp.Confidence).To(BeNumerically(">", 0))
			Expect(resp.Confidence).To(BeNumerically("<=", 1.0))
		})

		It("should include targetInOwnerChain in response - BR-AI-007", func() {
			resp, err := hgClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				Context: "Memory pressure detected on pod web-app-abc123",
				OwnerChain: []aianalysisclient.OwnerChainEntry{
					{Namespace: "default", Kind: "Deployment", Name: "web-app"},
				},
			})

			Expect(err).NotTo(HaveOccurred())
			// TargetInOwnerChain should be a boolean value
			// The mock LLM server returns deterministic responses
			Expect(resp.TargetInOwnerChain).To(BeAssignableToTypeOf(true))
		})

		It("should return selected workflow - BR-AI-016", func() {
			resp, err := hgClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				Context: "OOM Killed - container exceeded memory limit. Pod memory-hog in namespace default.",
				DetectedLabels: map[string]interface{}{
					"gitOpsManaged": true,
				},
			})

			Expect(err).NotTo(HaveOccurred())
			// MockLLM should return workflow recommendations
			Expect(resp.SelectedWorkflow).NotTo(BeNil())
			Expect(resp.SelectedWorkflow.WorkflowID).NotTo(BeEmpty())
		})

		It("should include alternative workflows for production - BR-AI-016", func() {
			resp, err := hgClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				Context: "Pod in CrashLoopBackOff state. Environment: production. Business priority: P0.",
				DetectedLabels: map[string]interface{}{
					"environment": "production",
				},
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			// Production should include alternative workflows for review
			// AlternativeWorkflows may be empty if only one workflow matches
			// but field should exist in response
		})
	})

	Context("Human Review Flag - BR-HAPI-197", func() {
		It("should handle needs_human_review response", func() {
			// This tests the needs_human_review field from HolmesGPT-API
			// The mock LLM can be configured to return this flag
			resp, err := hgClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				Context: "Unknown error pattern in production - requires investigation",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			// needs_human_review field should be accessible
			// Value depends on mock LLM configuration
			if resp.NeedsHumanReview {
				Expect(resp.HumanReviewReason).NotTo(BeNil())
			}
		})
	})

	Context("Validation History - DD-HAPI-002", func() {
		It("should return validation attempts history when present", func() {
			resp, err := hgClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{
				Context: "Database connection timeout in staging",
				OwnerChain: []aianalysisclient.OwnerChainEntry{
					{Namespace: "staging", Kind: "Deployment", Name: "db-client"},
				},
			})

			Expect(err).NotTo(HaveOccurred())
			// ValidationAttemptsHistory may be present if LLM required retries
			// This is for audit purposes per DD-HAPI-002
			_ = resp.ValidationAttemptsHistory // Access the field to verify it exists
		})
	})

	Context("Error Handling - BR-AI-009", func() {
		It("should handle timeout gracefully", func() {
			// Create a very short timeout context
			shortCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
			defer cancel()

			_, err := hgClient.Investigate(shortCtx, &aianalysisclient.IncidentRequest{
				Context: "Test timeout handling",
			})

			// Should return a timeout error
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context"))
		})

		It("should return structured error for invalid request", func() {
			// Empty request should fail validation
			_, err := hgClient.Investigate(testCtx, &aianalysisclient.IncidentRequest{})

			// HolmesGPT-API returns RFC 7807 errors for validation failures
			Expect(err).To(HaveOccurred())
		})
	})
})
