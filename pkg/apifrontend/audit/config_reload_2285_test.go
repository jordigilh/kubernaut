package audit_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

// GAP-11 (Issue #2285): the CA-cert hot-reload watcher reuses apifrontend's
// existing config.reloaded/config.rejected event types (added for the #1450
// policy-config watcher) rather than introducing new ones. ChangedKeys
// identifies which hot-reloadable component was reloaded.
var _ = Describe("StoreAdapter — CA-cert config reload parity (#2285, CM-3/AU-2/AU-12)", func() {

	var (
		store   *capturingStore
		adapter audit.ClosableEmitter
	)

	BeforeEach(func() {
		store = &capturingStore{}
		adapter = audit.NewStoreAdapter(store, logr.Discard())
	})

	Describe("UT-AF-2285-001: config.reloaded carries changed_keys=[ca_cert]", func() {
		It("populates ChangedKeys on the typed payload", func() {
			adapter.Emit(context.Background(), &audit.Event{
				Type:   audit.EventConfigReloaded,
				Detail: map[string]string{"changed_keys": "ca_cert"},
			})

			ev := store.lastEvent()
			Expect(ev).NotTo(BeNil())
			Expect(ev.EventType).To(Equal("apifrontend.config.reloaded"))
			Expect(ev.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeSuccess))

			payload, ok := ev.EventData.GetApifrontendConfigReloadedPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.ChangedKeys).To(Equal([]string{"ca_cert"}))
		})
	})

	Describe("UT-AF-2285-002: config.rejected carries a ca_cert-prefixed rejection reason", func() {
		It("stores the rejection reason on the typed payload", func() {
			adapter.Emit(context.Background(), &audit.Event{
				Type:   audit.EventConfigRejected,
				Detail: map[string]string{"rejection_reason": "ca_cert: invalid PEM content"},
			})

			ev := store.lastEvent()
			Expect(ev).NotTo(BeNil())
			Expect(ev.EventType).To(Equal("apifrontend.config.rejected"))
			Expect(ev.EventOutcome).To(Equal(ogenclient.AuditEventRequestEventOutcomeFailure))

			payload, ok := ev.EventData.GetApifrontendConfigRejectedPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.RejectionReason).To(Equal("ca_cert: invalid PEM content"))
		})
	})
})
