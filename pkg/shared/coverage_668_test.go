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

package shared_test

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/pkg/audit"
	cacheredis "github.com/jordigilh/kubernaut/pkg/cache/redis"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// flushBatchSpy implements audit.DataStorageClient for BufferedAuditStore Flush coverage (BR-STORAGE-001).
type flushBatchSpy struct {
	mu      sync.Mutex
	batches [][]*ogenclient.AuditEventRequest
}

func (s *flushBatchSpy) StoreBatch(_ context.Context, events []*ogenclient.AuditEventRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]*ogenclient.AuditEventRequest, len(events))
	copy(cp, events)
	s.batches = append(s.batches, cp)
	return nil
}

func (s *flushBatchSpy) lastBatch() []*ogenclient.AuditEventRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.batches) == 0 {
		return nil
	}
	return s.batches[len(s.batches)-1]
}

func newValidGatewayAuditEvent(correlationID string) *ogenclient.AuditEventRequest {
	event := audit.NewAuditEventRequest()
	event.Version = "1.0"
	audit.SetEventType(event, "gateway.crd.created")
	audit.SetEventCategory(event, "gateway")
	audit.SetEventAction(event, "created")
	audit.SetEventOutcome(event, audit.OutcomeSuccess)
	audit.SetActor(event, "service", "shared-packages-ut")
	audit.SetResource(event, "TestResource", "res-1")
	audit.SetCorrelationID(event, correlationID)
	payload := ogenclient.GatewayAuditPayload{
		EventType:   ogenclient.GatewayAuditPayloadEventTypeGatewayCrdCreated,
		SignalType:  ogenclient.GatewayAuditPayloadSignalTypeAlert,
		SignalName:  "test-alert",
		Namespace:   "default",
		Fingerprint: "fp-1",
	}
	audit.SetEventData(event, ogenclient.NewAuditEventRequestEventDataGatewayCrdCreatedAuditEventRequestEventData(payload))
	return event
}

var _ = Describe("Shared packages coverage 668", func() {

	Describe("pkg/audit OpenAPI helpers (BR-STORAGE-001)", func() {
		It("SetEventDataFromEnvelope returns nil without mutating the request today (BR-STORAGE-001)", func() {
			req := audit.NewAuditEventRequest()
			before := req.EventData
			env := audit.NewEventData("gateway", "op", "ok", map[string]interface{}{"k": 1})
			Expect(audit.SetEventDataFromEnvelope(req, env)).To(Succeed())
			Expect(req.EventData).To(Equal(before))
		})

		It("EnvelopeToMap round-trips a CommonEnvelope to map[string]interface{} (BR-STORAGE-001)", func() {
			env := audit.NewEventData("gateway", "signal_received", "success", map[string]interface{}{"id": "x"})
			m, err := audit.EnvelopeToMap(env)
			Expect(err).NotTo(HaveOccurred())
			Expect(m).To(HaveKeyWithValue("version", "1.0"))
			Expect(m).To(HaveKeyWithValue("service", "gateway"))
			Expect(m).To(HaveKeyWithValue("operation", "signal_received"))
			Expect(m).To(HaveKeyWithValue("status", "success"))
			Expect(m["payload"]).To(HaveKeyWithValue("id", "x"))
		})

		It("StructToMap converts a JSON-marshalable struct to a map (BR-STORAGE-001)", func() {
			type sample struct {
				Name string `json:"name"`
			}
			m, err := audit.StructToMap(sample{Name: "alpha"})
			Expect(err).NotTo(HaveOccurred())
			Expect(m).To(HaveKeyWithValue("name", "alpha"))
		})

		It("StructToMap returns an error when JSON marshaling fails (BR-STORAGE-001)", func() {
			_, err := audit.StructToMap(map[string]interface{}{"ch": make(chan int)})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("pkg/audit BufferedAuditStore.Flush (BR-STORAGE-001)", func() {
		It("Flush drains buffered events into StoreBatch without closing the store (BR-STORAGE-001)", func() {
			spy := &flushBatchSpy{}
			cfg := audit.Config{
				BufferSize:    64,
				BatchSize:     10,
				FlushInterval: 30 * time.Second,
				MaxRetries:    2,
			}
			store, err := audit.NewBufferedStore(spy, cfg, "shared-packages-668", logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(store.Close()).To(Succeed()) }()

			ev := newValidGatewayAuditEvent("corr-flush-668")
			Expect(store.StoreAudit(context.Background(), ev)).To(Succeed())
			Expect(store.Flush(context.Background())).To(Succeed())

			batch := spy.lastBatch()
			Expect(batch).To(HaveLen(1))
			Expect(batch[0].CorrelationID).To(Equal("corr-flush-668"))
		})

		It("Flush returns an error after the store is closed (BR-STORAGE-001)", func() {
			spy := &flushBatchSpy{}
			store, err := audit.NewBufferedStore(spy, audit.DefaultConfig(), "shared-packages-668-close", logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			Expect(store.Close()).To(Succeed())
			Expect(store.Flush(context.Background())).To(MatchError(ContainSubstring("closed")))
		})
	})

	Describe("pkg/cache/redis Options (BR-CACHE-002)", func() {
		It("DefaultOptions returns documented defaults (BR-CACHE-002)", func() {
			o := cacheredis.DefaultOptions()
			Expect(o.Addr).To(Equal("localhost:6379"))
			Expect(o.DB).To(Equal(0))
			Expect(o.Password).To(Equal(""))
			Expect(o.DialTimeout).To(Equal(5 * time.Second))
			Expect(o.ReadTimeout).To(Equal(3 * time.Second))
			Expect(o.WriteTimeout).To(Equal(3 * time.Second))
			Expect(o.PoolSize).To(Equal(10))
			Expect(o.MinIdleConns).To(Equal(5))
		})

		It("ToGoRedisOptions copies fields into go-redis Options (BR-CACHE-002)", func() {
			o := &cacheredis.Options{
				Addr:         "redis.example:6379",
				DB:           2,
				Password:     "secret",
				DialTimeout:  8 * time.Second,
				ReadTimeout:  2 * time.Second,
				WriteTimeout: 2 * time.Second,
				PoolSize:     20,
				MinIdleConns: 7,
			}
			gr := o.ToGoRedisOptions()
			Expect(gr).NotTo(BeNil())
			Expect(gr.Addr).To(Equal(o.Addr))
			Expect(gr.DB).To(Equal(o.DB))
			Expect(gr.Password).To(Equal(o.Password))
			Expect(gr.DialTimeout).To(Equal(o.DialTimeout))
			Expect(gr.ReadTimeout).To(Equal(o.ReadTimeout))
			Expect(gr.WriteTimeout).To(Equal(o.WriteTimeout))
			Expect(gr.PoolSize).To(Equal(o.PoolSize))
			Expect(gr.MinIdleConns).To(Equal(o.MinIdleConns))
		})
	})

	Describe("pkg/shared/types StructuredDescription (BR-WORKFLOW-004)", func() {
		It("String returns the What field summary (BR-WORKFLOW-004)", func() {
			d := types.StructuredDescription{What: "Restart unhealthy pods"}
			Expect(d.String()).To(Equal("Restart unhealthy pods"))
		})
	})

	Describe("pkg/shared/scope ScopeGVKs (BR-SCOPE-001)", func() {
		It("ScopeGVKs includes v1 Namespace and covers scope cache kinds (BR-SCOPE-001)", func() {
			gvks := scope.ScopeGVKs()
			Expect(gvks).NotTo(BeEmpty())
			for _, gvk := range gvks {
				Expect(gvk.Version).To(Equal("v1"))
			}
			wantNS := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
			nsSeen := 0
			for _, gvk := range gvks {
				if gvk == wantNS {
					nsSeen++
				}
			}
			Expect(nsSeen).To(BeNumerically(">=", 1))
			kinds := map[string]bool{}
			for _, gvk := range gvks {
				kinds[gvk.Kind] = true
			}
			Expect(kinds).To(HaveKey("Deployment"))
			Expect(kinds).To(HaveKey("Pod"))
		})
	})
})
