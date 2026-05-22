package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
)

type auditSpy struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (s *auditSpy) Emit(_ context.Context, event *audit.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *auditSpy) eventsByType(t audit.EventType) []*audit.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*audit.Event
	for _, e := range s.events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

var _ = Describe("Audit event emission – auth decorators (PR2 wiring)", func() {
	Describe("AuditingJWTDelegationTransport", func() {
		It("UT-AF-1156-060: emits jwt.delegation on outbound delegated request", func() {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			spy := &auditSpy{}
			transport := &auth.AuditingJWTDelegationTransport{
				Base:    http.DefaultTransport,
				Auditor: spy,
			}

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre-alice",
				RawToken: "test-jwt-token",
			})
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, backend.URL+"/api/v1/test", nil)
			resp, err := transport.RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			events := spy.eventsByType(audit.EventJWTDelegation)
			Expect(events).To(HaveLen(1), "expected exactly one jwt.delegation event")
			Expect(events[0].UserID).To(Equal("sre-alice"))
			Expect(events[0].Detail).To(HaveKeyWithValue("target_host", backend.URL))
		})
	})
})
