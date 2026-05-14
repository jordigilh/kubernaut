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

// #1111 / #1113: IT audit event coverage for aiagent.response.failed.
// Uses the enrichment suite's real DS infrastructure (Podman Postgres + DSAuditStore).
//
// Business Requirements: BR-AUDIT-005 (Complete audit trail)
// Correlation ID: signal.RemediationID per investigator.go line 227
package enrichment_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kaaudit "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// errorLLMClient satisfies llm.Client and always returns an error from Chat.
type errorLLMClient struct {
	err error
}

func (e *errorLLMClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, e.err
}

func (e *errorLLMClient) StreamChat(_ context.Context, _ llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, e.err
}

func (e *errorLLMClient) Close() error { return nil }

var _ llm.Client = (*errorLLMClient)(nil)

var _ = Describe("#1111 Investigator aiagent.response.failed DS Audit", Label("integration", "kubernautagent", "audit"), func() {

	It("IT-KA-1111-001: aiagent.response.failed persisted to DS on LLM failure", func() {
		correlationID := fmt.Sprintf("test-1111-resp-failed-%d", time.Now().UnixNano())

		builder, err := prompt.NewBuilder()
		Expect(err).ToNot(HaveOccurred(), "prompt.NewBuilder should succeed")

		rp := parser.NewResultParser(suiteLogger.WithName("parser"))

		inv := investigator.New(investigator.Config{
			Client:       &errorLLMClient{err: fmt.Errorf("forced LLM failure for audit test")},
			Builder:      builder,
			ResultParser: rp,
			Enricher:     enricher,
			AuditStore:   auditStore,
			Logger:       suiteLogger.WithName("investigator-audit-test"),
			MaxTurns:     15,
			PhaseTools:   investigator.DefaultPhaseToolMap(),
		})

		testCtx, testCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer testCancel()

		signal := katypes.SignalContext{
			Name:          "test-audit-signal",
			Namespace:     "default",
			Severity:      "critical",
			Message:       "Forced LLM failure for aiagent.response.failed audit IT",
			RemediationID: correlationID,
			ResourceKind:  "Pod",
			ResourceName:  "audit-test-pod",
		}

		_, investigateErr := inv.Investigate(testCtx, signal)
		Expect(investigateErr).To(HaveOccurred(), "Investigate should fail due to LLM error")

		By("Querying DS for aiagent.response.failed audit event")
		var events []ogenclient.AuditEvent
		Eventually(func() bool {
			params := ogenclient.QueryAuditEventsParams{}
			params.CorrelationID.SetTo(correlationID)
			params.EventType.SetTo(kaaudit.EventTypeResponseFailed)
			params.Limit.SetTo(100)

			resp, qErr := ogenClient.QueryAuditEvents(testCtx, params)
			if qErr != nil {
				GinkgoWriter.Printf("  ⚠️  Audit query failed: %v\n", qErr)
				return false
			}
			events = resp.Data
			return len(events) > 0
		}, 15*time.Second, 1*time.Second).Should(BeTrue(),
			fmt.Sprintf("DS should contain %s event for correlation_id=%s", kaaudit.EventTypeResponseFailed, correlationID))

		Expect(events).To(HaveLen(1),
			"Exactly one aiagent.response.failed event expected per failed investigation")
		Expect(events[0].CorrelationID).To(Equal(correlationID))
		Expect(events[0].EventOutcome).To(Equal(ogenclient.AuditEventEventOutcomeFailure))
	})
})
