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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/google/uuid"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Audit lifecycle against DataStorage contract — BR-AI-952 / GAP-T2", func() {

	Describe("IT-AUD-001: AuditEvent JSON round-trip preserves fields", func() {
		It("marshal → unmarshal keeps scalar and map data intact", func() {
			pid := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
			ev := audit.AuditEvent{
				EventType:     audit.EventTypeLLMRequest,
				EventCategory: audit.EventCategory,
				EventAction:   audit.ActionLLMRequest,
				EventOutcome:  audit.OutcomeSuccess,
				CorrelationID: "corr-aud-001",
				ParentEventID: &pid,
				ActorID:       "actor-svc-test",
				ActorType:     "User",
				Data: map[string]interface{}{
					"event_id":      "embedded-e1",
					"prompt_length": float64(42),
				},
			}

			raw, err := json.Marshal(ev)
			Expect(err).NotTo(HaveOccurred())

			var decoded audit.AuditEvent
			Expect(json.Unmarshal(raw, &decoded)).To(Succeed())

			Expect(decoded.EventType).To(Equal(ev.EventType))
			Expect(decoded.EventCategory).To(Equal(ev.EventCategory))
			Expect(decoded.EventAction).To(Equal(ev.EventAction))
			Expect(decoded.EventOutcome).To(Equal(ev.EventOutcome))
			Expect(decoded.CorrelationID).To(Equal(ev.CorrelationID))
			Expect(decoded.ActorID).To(Equal(ev.ActorID))
			Expect(decoded.ActorType).To(Equal(ev.ActorType))

			Expect(decoded.ParentEventID).NotTo(BeNil())
			Expect(decoded.ParentEventID.String()).To(Equal(pid.String()))

			Expect(decoded.Data).To(HaveKey("event_id"))
			Expect(decoded.Data["event_id"]).To(Equal("embedded-e1"))
			Expect(decoded.Data["prompt_length"]).To(Equal(float64(42)))
		})
	})

	Describe("IT-AUD-002: StoreAudit sends expected HTTP request to DS", func() {
		It("hits POST /api/v1/audit/events with decoded JSON matching the event", func() {
			var seenMethod, seenPath string
			var captured []byte

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seenMethod = r.Method
				seenPath = r.URL.Path
				body, err := io.ReadAll(r.Body)
				Expect(err).NotTo(HaveOccurred())
				captured = body

				evID := uuid.New()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = fmt.Fprintf(w, `{"event_id":%q,"event_timestamp":"2026-05-02T12:00:00Z","message":"accepted"}`,
					evID.String())
			}))
			defer srv.Close()

			cl, err := ogenclient.NewClient(srv.URL)
			Expect(err).NotTo(HaveOccurred())

			store := audit.NewDSAuditStore(cl)
			event := audit.NewEvent(audit.EventTypeAlignmentStep, "corr-ds-it")
			event.EventAction = audit.ActionAlignmentEvaluate
			event.EventOutcome = audit.OutcomePending
			event.Data["step_index"] = 0
			event.Data["step_kind"] = "tool"

			Expect(store.StoreAudit(context.Background(), event)).To(Succeed())

			Expect(seenMethod).To(Equal(http.MethodPost))
			Expect(seenPath).To(Equal("/api/v1/audit/events"))

			var envelope map[string]json.RawMessage
			Expect(json.Unmarshal(captured, &envelope)).To(Succeed())
			Expect(envelope).To(HaveKey("event_type"))

			var et string
			Expect(json.Unmarshal(envelope["event_type"], &et)).To(Succeed())
			Expect(et).To(Equal(audit.EventTypeAlignmentStep))
			Expect(envelope).To(HaveKey("correlation_id"))
			var corr string
			Expect(json.Unmarshal(envelope["correlation_id"], &corr)).To(Succeed())
			Expect(corr).To(Equal("corr-ds-it"))
			Expect(envelope).To(HaveKey("event_action"))
		})
	})

	Describe("IT-AUD-003: StoreAudit wraps errors from failed HTTP/decoding", func() {
		It("surfaces audit store wrapper on bad 201 payload", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`broken-json`))
			}))
			defer srv.Close()

			cl, err := ogenclient.NewClient(srv.URL)
			Expect(err).NotTo(HaveOccurred())

			store := audit.NewDSAuditStore(cl)
			event := audit.NewEvent(audit.EventTypeRCAComplete, "corr-fail")
			err = store.StoreAudit(context.Background(), event)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("audit store"))
		})
	})

	Describe("IT-AUD-004: ActorID/ActorType flow into DS audit request", func() {
		It("defaults are overridden when set on AuditEvent", func() {
			var envelope map[string]json.RawMessage

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, readErr := io.ReadAll(r.Body)
				Expect(readErr).NotTo(HaveOccurred())
				Expect(json.Unmarshal(body, &envelope)).To(Succeed())

				evID := uuid.New()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = fmt.Fprintf(w, `{"event_id":%q,"event_timestamp":"2026-05-02T12:00:00Z","message":"accepted"}`,
					evID.String())
			}))
			defer srv.Close()

			cl, err := ogenclient.NewClient(srv.URL)
			Expect(err).NotTo(HaveOccurred())
			store := audit.NewDSAuditStore(cl)

			event := audit.NewEvent(audit.EventTypeResponseComplete, "corr-actor")
			event.EventAction = audit.ActionResponseSent
			event.EventOutcome = audit.OutcomeSuccess
			event.ActorID = "custom-actor"
			event.ActorType = "IntegrationTest"

			Expect(store.StoreAudit(context.Background(), event)).To(Succeed())

			Expect(envelope).To(HaveKey("actor_id"))
			var actorID string
			Expect(json.Unmarshal(envelope["actor_id"], &actorID)).To(Succeed())
			Expect(actorID).To(Equal("custom-actor"))
			Expect(envelope).To(HaveKey("actor_type"))
			var actorType string
			Expect(json.Unmarshal(envelope["actor_type"], &actorType)).To(Succeed())
			Expect(actorType).To(Equal("IntegrationTest"))
		})
	})

	Describe("IT-AUD-005: Multiple StoreAudit calls preserve emission order", func() {
		It("stores events in sequence as received by the mock server", func() {
			var mu sync.Mutex
			var order []string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, readErr := io.ReadAll(r.Body)
				Expect(readErr).NotTo(HaveOccurred())
				var m map[string]json.RawMessage
				Expect(json.Unmarshal(body, &m)).To(Succeed())
				rawCorr, ok := m["correlation_id"]
				Expect(ok).To(BeTrue())
				var c string
				Expect(json.Unmarshal(rawCorr, &c)).To(Succeed())

				mu.Lock()
				order = append(order, c)
				mu.Unlock()

				evID := uuid.New()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = fmt.Fprintf(w, `{"event_id":%q,"event_timestamp":"2026-05-02T12:00:05Z","message":"accepted"}`,
					evID.String())
			}))
			defer srv.Close()

			cl, err := ogenclient.NewClient(srv.URL)
			Expect(err).NotTo(HaveOccurred())
			store := audit.NewDSAuditStore(cl)

			for _, id := range []string{"corr-order-1", "corr-order-2", "corr-order-3"} {
				ev := audit.NewEvent(audit.EventTypeLLMResponse, id)
				ev.EventOutcome = audit.OutcomeSuccess
				Expect(store.StoreAudit(context.Background(), ev)).To(Succeed())
			}

			mu.Lock()
			defer mu.Unlock()
			Expect(order).To(Equal([]string{"corr-order-1", "corr-order-2", "corr-order-3"}))
		})
	})
})
