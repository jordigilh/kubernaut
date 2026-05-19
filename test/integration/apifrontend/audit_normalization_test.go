package apifrontend_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// capturingDSClient captures batches written by BufferedAuditStore for verification.
type capturingDSClient struct {
	batches [][]*ogenclient.AuditEventRequest
}

func (c *capturingDSClient) StoreBatch(_ context.Context, events []*ogenclient.AuditEventRequest) error {
	c.batches = append(c.batches, events)
	return nil
}

func (c *capturingDSClient) allEvents() []*ogenclient.AuditEventRequest {
	var all []*ogenclient.AuditEventRequest
	for _, batch := range c.batches {
		all = append(all, batch...)
	}
	return all
}

var _ = Describe("IT-AF-1156: Audit Normalization Integration", func() {
	var (
		dsClient   *capturingDSClient
		auditStore sharedaudit.AuditStore
		adapter    audit.ClosableEmitter
	)

	BeforeEach(func() {
		dsClient = &capturingDSClient{}
		var err error
		auditStore, err = sharedaudit.NewBufferedStore(dsClient, sharedaudit.Config{
			BufferSize:    100,
			BatchSize:     10,
			FlushInterval: 50 * time.Millisecond,
			MaxRetries:    1,
		}, "apifrontend", logr.Discard())
		Expect(err).NotTo(HaveOccurred())

		adapter = audit.NewStoreAdapter(auditStore, logr.Discard())
	})

	AfterEach(func() {
		if adapter != nil {
			Expect(adapter.Close(context.Background())).To(Succeed())
		}
	})

	It("IT-AF-1156-001: session.created event round-trips through BufferedAuditStore with typed payload", func() {
		adapter.Emit(context.Background(), &audit.Event{
			Type:          audit.EventSessionCreated,
			CorrelationID: "rr-sess-001",
			UserID:        "alice",
			Detail: map[string]string{
				"session_id":    "sess-it-001",
				"a2a_task_id":   "task-it-001",
				"join_mode":     "start",
				"user_identity": "alice",
			},
		})

		Expect(auditStore.Flush(context.Background())).To(Succeed())

		Eventually(func() int { return len(dsClient.allEvents()) }).
			WithTimeout(2 * time.Second).
			WithPolling(50 * time.Millisecond).
			Should(BeNumerically(">=", 1))

		events := dsClient.allEvents()
		Expect(events).To(HaveLen(1))
		evt := events[0]
		Expect(evt.EventType).To(Equal("apifrontend.session.created"))
		Expect(evt.EventCategory).To(Equal(ogenclient.AuditEventRequestEventCategoryApifrontend))
		Expect(evt.CorrelationID).To(Equal("rr-sess-001"))
		Expect(evt.ActorType.Value).To(Equal("user"))
		Expect(evt.ActorID.Value).To(Equal("alice"))
		Expect(string(evt.EventData.Type)).To(Equal("apifrontend.session.created"))
	})

	It("IT-AF-1156-002: auth.success event round-trips with typed payload", func() {
		adapter.Emit(context.Background(), &audit.Event{
			Type:   audit.EventAuthSuccess,
			UserID: "bob",
			Detail: map[string]string{
				"auth_method": "jwt",
				"issuer":      "dex.example.com",
			},
		})

		Expect(auditStore.Flush(context.Background())).To(Succeed())

		Eventually(func() int { return len(dsClient.allEvents()) }).
			WithTimeout(2 * time.Second).
			WithPolling(50 * time.Millisecond).
			Should(BeNumerically(">=", 1))

		events := dsClient.allEvents()
		Expect(events).To(HaveLen(1))
		evt := events[0]
		Expect(evt.EventType).To(Equal("apifrontend.auth.success"))
		Expect(evt.EventAction).To(Equal("authenticated"))
		Expect(evt.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess))
	})

	It("IT-AF-1156-003: Close flushes all buffered events", func() {
		for i := 0; i < 5; i++ {
			adapter.Emit(context.Background(), &audit.Event{
				Type:   audit.EventConfigReloaded,
				Detail: map[string]string{"config_version": "v1"},
			})
		}

		Expect(adapter.Close(context.Background())).To(Succeed())

		all := dsClient.allEvents()
		Expect(all).To(HaveLen(5))
		for _, evt := range all {
			Expect(evt.EventType).To(Equal("apifrontend.config.reloaded"))
		}

		adapter = nil
	})
})
