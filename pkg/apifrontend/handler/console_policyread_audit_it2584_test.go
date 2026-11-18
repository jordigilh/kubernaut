package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/requestid"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// typedAuditCapture records the typed events that reach the DataStorage
// boundary, so a handler-level read can be asserted through the real adapter
// rather than through an in-process recorder.
type typedAuditCapture struct {
	mu     sync.Mutex
	events []*ogenclient.AuditEventRequest
}

func (s *typedAuditCapture) StoreAudit(_ context.Context, event *ogenclient.AuditEventRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *typedAuditCapture) Flush(context.Context) error { return nil }

func (s *typedAuditCapture) Close() error { return nil }

func (s *typedAuditCapture) snapshot() []*ogenclient.AuditEventRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*ogenclient.AuditEventRequest(nil), s.events...)
}

var _ = Describe("IT-AF-2584-010: typed Console read audit attribution", func() {
	It("IT-AF-2584-010 [BR-AUDIT-005, FedRAMP AU-2/AU-3, SOC2 CC7.2]: records one attributed typed event per authenticated read", func() {
		const requestID = "req-2584-attribution"
		capture := &typedAuditCapture{}
		service, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory:  &consoleInventoryStub{page: successfulPage()},
			CurrentPolicies: &consoleCurrentPolicyStub{},
			Authorizer:      &consoleAuthorizer{allow: true},
			Auditor:         audit.NewStoreAdapter(capture, logr.Discard()),
		})
		Expect(err).NotTo(HaveOccurred())

		restHandler := requestid.Middleware(handler.NewConsolePolicyReadHandler(service, logr.Discard()))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/console/inventory/managed-resources?cluster_id=local", http.NoBody)
		req.Header.Set("X-Request-ID", requestID)
		req = req.WithContext(auth.WithUserIdentity(req.Context(), &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}))
		recorder := httptest.NewRecorder()

		restHandler.ServeHTTP(recorder, req)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		events := capture.snapshot()
		Expect(events).To(HaveLen(1))

		event := events[0]
		Expect(event.EventType).To(Equal("apifrontend.console.managed_resource_list_read"))
		Expect(event.EventAction).To(Equal("read"))
		Expect(string(event.EventOutcome)).To(Equal("success"))
		Expect(event.EventCategory).To(Equal(ogenclient.AuditEventRequestEventCategoryApifrontend))
		Expect(event.ActorType.Value).To(Equal("user"))
		Expect(event.ActorID.Value).To(Equal("alice"))
		Expect(event.CorrelationID).To(Equal(requestID))
		// The typed payload carries the bounded outcome, never the records.
		payload, ok := event.EventData.GetApifrontendConsoleManagedResourceListReadPayload()
		Expect(ok).To(BeTrue(), "the event must use the typed Console read payload")
		Expect(string(payload.ResultState)).To(Equal("success"))
		Expect(payload.RequestID.Value).To(Equal(requestID))
		Expect(payload.ClusterID.Value).To(Equal("local"))
		Expect(payload.FilterFingerprint.Value).NotTo(BeEmpty())

		encoded, err := json.Marshal(event)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring("checkout"))
	})
})
