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

package audit_test

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// batchSpy implements pkg/audit.DataStorageClient for buffered store tests (BR-AI-056 audit trail).
type batchSpy struct {
	mu      sync.Mutex
	batches [][]*ogenclient.AuditEventRequest
}

func (b *batchSpy) StoreBatch(_ context.Context, events []*ogenclient.AuditEventRequest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.batches = append(b.batches, events)
	return nil
}

func (b *batchSpy) lastBatch() []*ogenclient.AuditEventRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.batches) == 0 {
		return nil
	}
	return b.batches[len(b.batches)-1]
}

var _ = Describe("Kubernaut Agent audit coverage 668 (BR-HAPI-197 DD-AUDIT-002)", func() {

	Describe("toIncidentResponseData human_review_reason mapping (BR-HAPI-197)", func() {
		It("maps recognised LLM human_review_reason strings onto ogen enum values", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			rd, err := json.Marshal(map[string]interface{}{
				"rca_summary":           "incomplete RCA",
				"severity":              "high",
				"confidence":            0.4,
				"needs_human_review":    true,
				"human_review_reason":   "rca_incomplete",
			})
			Expect(err).NotTo(HaveOccurred())

			event := audit.NewEvent(audit.EventTypeResponseComplete, "corr-hr-valid")
			event.EventAction = audit.ActionResponseSent
			event.EventOutcome = audit.OutcomeSuccess
			event.Data["response_data"] = string(rd)
			event.Data["total_prompt_tokens"] = 10
			event.Data["total_completion_tokens"] = 5

			Expect(store.StoreAudit(context.Background(), event)).To(Succeed())
			req := recorder.calls[0]
			payload, ok := req.EventData.GetAIAgentResponsePayload()
			Expect(ok).To(BeTrue())
			Expect(payload.ResponseData.NeedsHumanReview.Set).To(BeTrue())
			Expect(payload.ResponseData.NeedsHumanReview.Value).To(BeTrue())
			Expect(payload.ResponseData.HumanReviewReason.Set).To(BeTrue())
			Expect(payload.ResponseData.HumanReviewReason.Value).To(Equal(ogenclient.IncidentResponseDataHumanReviewReasonRcaIncomplete))
		})

		It("drops unrecognised human_review_reason strings so OpenAPI validation keeps the event", func() {
			recorder := &fakeOgenClient{}
			store := audit.NewDSAuditStore(recorder)

			rd, err := json.Marshal(map[string]interface{}{
				"rca_summary":          "x",
				"severity":           "low",
				"confidence":         0.1,
				"needs_human_review": true,
				"human_review_reason":  "free_text_from_model",
			})
			Expect(err).NotTo(HaveOccurred())

			event := audit.NewEvent(audit.EventTypeResponseComplete, "corr-hr-invalid")
			event.EventAction = audit.ActionResponseSent
			event.EventOutcome = audit.OutcomeSuccess
			event.Data["response_data"] = string(rd)

			Expect(store.StoreAudit(context.Background(), event)).To(Succeed())
			req := recorder.calls[0]
			payload, ok := req.EventData.GetAIAgentResponsePayload()
			Expect(ok).To(BeTrue())
			Expect(payload.ResponseData.HumanReviewReason.Set).To(BeFalse())
		})
	})

	Describe("BufferedDSAuditStore options and lifecycle (DD-AUDIT-002)", func() {
		It("WithFlushInterval, WithBufferSize, and WithBatchSize construct a store that flushes via Close", func() {
			spy := &batchSpy{}
			store, err := audit.NewBufferedDSAuditStore(
				spy,
				logr.Discard(),
				audit.WithFlushInterval(300*time.Millisecond),
				audit.WithBufferSize(64),
				audit.WithBatchSize(1),
			)
			Expect(err).NotTo(HaveOccurred())

			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-buf-668")
			event.EventAction = audit.ActionLLMRequest
			event.EventOutcome = audit.OutcomeSuccess
			event.Data["model"] = "test-model"
			event.Data["prompt_length"] = 4
			event.Data["prompt_preview"] = "ping"

			Expect(store.StoreAudit(context.Background(), event)).To(Succeed())
			Expect(store.Close()).To(Succeed())

			batch := spy.lastBatch()
			Expect(batch).To(HaveLen(1))
			Expect(batch[0].EventType).To(Equal(audit.EventTypeLLMRequest))
			Expect(batch[0].CorrelationID).To(Equal("corr-buf-668"))
		})

		It("Flush pushes buffered events to DataStorage without closing the worker", func() {
			spy := &batchSpy{}
			store, err := audit.NewBufferedDSAuditStore(spy, logr.Discard(), audit.WithBatchSize(5))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = store.Close() }()

			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-flush-668")
			event.EventAction = audit.ActionLLMRequest
			event.EventOutcome = audit.OutcomeSuccess
			event.Data["model"] = "m"
			event.Data["prompt_length"] = 2
			event.Data["prompt_preview"] = "ab"

			Expect(store.StoreAudit(context.Background(), event)).To(Succeed())
			Expect(store.Flush(context.Background())).To(Succeed())

			batch := spy.lastBatch()
			Expect(batch).To(HaveLen(1))
			Expect(batch[0].CorrelationID).To(Equal("corr-flush-668"))
		})
	})

	Describe("NopAuditStore", func() {
		It("StoreAudit succeeds without persisting (audit disabled path)", func() {
			var nop audit.NopAuditStore
			event := audit.NewEvent(audit.EventTypeLLMRequest, "corr-nop")
			event.EventAction = audit.ActionLLMRequest
			event.EventOutcome = audit.OutcomeSuccess
			Expect(nop.StoreAudit(context.Background(), event)).To(Succeed())
		})
	})
})
