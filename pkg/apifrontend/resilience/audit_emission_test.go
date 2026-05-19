package resilience

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
)

type auditSpyEmitter struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (s *auditSpyEmitter) Emit(_ context.Context, event *audit.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *auditSpyEmitter) eventsByType(t audit.EventType) []*audit.Event {
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

// UT-AF-1156-062: CircuitBreakerAuditFunc wires AuditFunc to emit circuitbreaker.trip
func TestCircuitBreakerAuditFunc_EmitsEvent(t *testing.T) {
	spy := &auditSpyEmitter{}

	auditFn := CircuitBreakerAuditFunc(spy)

	base := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("fail")
	})

	cbt := NewCircuitBreakerTransport(base, &CircuitBreakerConfig{
		Name:             "test-cb-audit-emit",
		MaxRequests:      1,
		Interval:         10 * time.Second,
		Timeout:          50 * time.Millisecond,
		FailureThreshold: 2,
		DependencyName:   "ka",
		AuditFunc:        auditFn,
	})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/test", http.NoBody)
	_, _ = cbt.RoundTrip(req)
	_, _ = cbt.RoundTrip(req)

	events := spy.eventsByType(audit.EventCircuitBreakerTrip)
	if len(events) != 1 {
		t.Fatalf("expected 1 circuitbreaker.trip event, got %d", len(events))
	}
	if events[0].Detail["dependency"] != "ka" {
		t.Errorf("dependency = %q, want %q", events[0].Detail["dependency"], "ka")
	}
}
