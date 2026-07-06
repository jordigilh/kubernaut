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

package mcp_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

var _ = Describe("Fix #1384 Bug B — Integration Tests (AU-6, SI-10, CP-10)", func() {

	Describe("IT-KA-1384-003: Reconstruction succeeds despite tool-call-only audit turns (AU-6, CP-10)", func() {
		It("should deliver ONLY non-empty messages through the full reconstruct chain", func() {
			now := time.Now()

			querier := &mockAuditQuerier{
				response: &ogenclient.AuditEventsQueryResponse{
					Data: []ogenclient.AuditEvent{
						makeLLMRequestEvent("rr-it-003", "Investigate the pod crash", now.Add(-4*time.Minute)),
						makeLLMResponseEvent("rr-it-003", "Let me check the logs.", now.Add(-3*time.Minute)),
						makeLLMRequestEvent("rr-it-003", "What do the events say?", now.Add(-2*time.Minute)),
						// Tool-call-only response: AnalysisPreview is empty
						makeLLMResponseEvent("rr-it-003", "", now.Add(-1*time.Minute)),
					},
				},
			}

			reconstructor := mcpinternal.NewDSContextReconstructor(querier, 5*time.Second, logr.Discard())
			runner := &reconSpawnRunner{}
			spawner := mcpinternal.NewReconstructionSpawner(runner, reconstructor, logr.Discard())

			err := spawner.SpawnReconstruct(context.Background(), &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-it-003",
				SessionID:     "sess-it-003",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(runner.called.Load()).To(Equal(int32(1)))

			By("verifying no empty content messages reach the LLM runner")
			for i, msg := range runner.receivedMessages {
				Expect(msg.Content).NotTo(BeEmpty(),
					"message[%d] (role=%s) has empty Content — would cause LLM 400 (SI-10 violation)", i, msg.Role)
			}

			By("verifying meaningful messages are preserved in order")
			Expect(runner.receivedMessages).To(HaveLen(3),
				"expected 3 messages (2 user + 1 non-empty assistant)")
			Expect(runner.receivedMessages[0].Role).To(Equal("user"))
			Expect(runner.receivedMessages[0].Content).To(Equal("Investigate the pod crash"))
			Expect(runner.receivedMessages[1].Role).To(Equal("assistant"))
			Expect(runner.receivedMessages[1].Content).To(Equal("Let me check the logs."))
			Expect(runner.receivedMessages[2].Role).To(Equal("user"))
			Expect(runner.receivedMessages[2].Content).To(Equal("What do the events say?"))
		})
	})

	Describe("IT-KA-1384-005: Reconstruction through production dispatch never sends empty text blocks to LLM (SI-10)", func() {
		It("should produce a clean message array even when DS returns tool-call-only audit events", func() {
			now := time.Now()

			querier := &mockAuditQuerier{
				response: &ogenclient.AuditEventsQueryResponse{
					Data: []ogenclient.AuditEvent{
						makeLLMRequestEvent("rr-it-005", "Why is the VM not booting?", now.Add(-3*time.Minute)),
						// Two consecutive empty assistant turns (tool-call-only LLM responses)
						makeLLMResponseEvent("rr-it-005", "", now.Add(-2*time.Minute)),
						makeLLMResponseEvent("rr-it-005", "", now.Add(-90*time.Second)),
						makeLLMRequestEvent("rr-it-005", "Check the CNV operator logs", now.Add(-1*time.Minute)),
						makeLLMResponseEvent("rr-it-005", "The CNV operator shows a crash loop.", now.Add(-30*time.Second)),
					},
				},
			}

			reconstructor := mcpinternal.NewDSContextReconstructor(querier, 5*time.Second, logr.Discard())

			capturedRunner := &reconSpawnRunner{}
			spawner := mcpinternal.NewReconstructionSpawner(capturedRunner, reconstructor, logr.Discard())

			err := spawner.SpawnReconstruct(context.Background(), &mcpinternal.ReconstructionContext{
				CorrelationID: "rr-it-005",
				SessionID:     "sess-it-005",
			})
			Expect(err).NotTo(HaveOccurred())

			By("asserting zero empty text blocks in the messages array")
			for i, msg := range capturedRunner.receivedMessages {
				Expect(msg.Content).NotTo(BeEmpty(),
					"message[%d] (role=%s) has empty Content — would create empty NewTextBlock in anthropicfamily (SI-10)", i, msg.Role)
			}

			By("asserting correct message count after filtering")
			Expect(capturedRunner.receivedMessages).To(HaveLen(3),
				"expected 3 messages (2 user + 1 non-empty assistant); two empty assistant turns filtered")
		})
	})
})
