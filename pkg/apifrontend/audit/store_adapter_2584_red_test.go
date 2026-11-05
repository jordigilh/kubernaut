package audit_test

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// RED contract for Issue #2584 / BR-AUDIT-005. Console reads must remain in
// the typed AF/DataStorage audit path. The current adapter intentionally drops
// unknown event types, so these assertions fail until the union and registry
// are extended together.
var _ = Describe("IT-AF-2584-012: typed Console read audit events", func() {
	var (
		store   *capturingStore
		adapter audit.ClosableEmitter
	)

	BeforeEach(func() {
		store = &capturingStore{}
		adapter = audit.NewStoreAdapter(store, logr.Discard())
	})

	DescribeTable("persists one sanitized typed event per authenticated read",
		func(eventType audit.EventType, resultState, expectedType, expectedOutcome string) {
			adapter.Emit(context.Background(), &audit.Event{
				Type:          eventType,
				CorrelationID: "request-2584-correlation",
				RequestID:     "request-2584",
				UserID:        "alice",
				ClusterID:     "prod-east",
				Detail: map[string]string{
					"result_state":         resultState,
					"filter_fingerprint":   "sha256:filter-fingerprint",
					"resource_fingerprint": "sha256:resource-fingerprint",
					// These values model data the handler must never copy into
					// an audit payload.
					"returned_records": "deployment/payments/checkout",
					"policy_source":    "package main; secret-token",
					"arbitrary_filter": "namespace=payments&token=secret-token",
				},
			})

			event := store.lastEvent()
			Expect(event).NotTo(BeNil(), "authenticated Console reads must not use the logged-only path")
			Expect(event.EventType).To(Equal(expectedType))
			Expect(event.EventAction).To(Equal("read"))
			Expect(string(event.EventOutcome)).To(Equal(expectedOutcome))
			Expect(event.CorrelationID).To(Equal("request-2584-correlation"))
			Expect(event.ActorType.Value).To(Equal("user"))
			Expect(event.ActorID.Value).To(Equal("alice"))

			payload, err := json.Marshal(event.EventData)
			Expect(err).NotTo(HaveOccurred())
			payloadText := string(payload)
			Expect(payloadText).To(ContainSubstring(`"result_state":"` + resultState + `"`))
			Expect(payloadText).NotTo(ContainSubstring("returned_records"))
			Expect(payloadText).NotTo(ContainSubstring("policy_source"))
			Expect(payloadText).NotTo(ContainSubstring("secret-token"))
			Expect(payloadText).NotTo(ContainSubstring("arbitrary_filter"))
		},
		Entry("policy scope success", audit.EventType("console.policy_scope_read"), "success", "apifrontend.console.policy_scope_read", "success"),
		Entry("managed resource list empty", audit.EventType("console.managed_resource_list_read"), "empty", "apifrontend.console.managed_resource_list_read", "success"),
		Entry("policy scope partial", audit.EventType("console.policy_scope_read"), "partial", "apifrontend.console.policy_scope_read", "failure"),
		Entry("managed resource list stale", audit.EventType("console.managed_resource_list_read"), "stale", "apifrontend.console.managed_resource_list_read", "failure"),
		Entry("managed resource exact forbidden", audit.EventType("console.managed_resource_get_read"), "forbidden", "apifrontend.console.managed_resource_get_read", "failure"),
	)

	It("UT-AF-2584-011: the generated event union validates every Console read payload", func() {
		adapter.Emit(context.Background(), &audit.Event{
			Type:          audit.EventType("console.managed_resource_get_read"),
			CorrelationID: "request-2584-union",
			UserID:        "alice",
			ClusterID:     "local",
			Detail: map[string]string{
				"result_state":         "not_found",
				"resource_fingerprint": "sha256:resource-fingerprint",
			},
		})

		event := store.lastEvent()
		Expect(event).NotTo(BeNil())
		Expect(event.EventData.Validate()).To(Succeed())
		Expect(event.EventData.Type).To(Equal(ogenclient.AuditEventRequestEventDataType("apifrontend.console.managed_resource_get_read")))
	})
})
