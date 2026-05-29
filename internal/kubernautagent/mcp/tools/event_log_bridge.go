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

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// LogFunc is the callback signature for sending log messages to the MCP client.
// In production this wraps ServerSession.Log(); in tests it captures calls.
// Returns an error so callers can detect delivery failures.
type LogFunc func(level, logger string, data json.RawMessage) error

// EventLogBridge reads InvestigationEvents from a channel and forwards them
// as structured JSON envelopes via a LogFunc callback. Each event is tagged
// with a monotonically increasing sequence number for ordering (SI-4).
type EventLogBridge struct {
	events    <-chan session.InvestigationEvent
	logFn     LogFunc
	logger    logr.Logger
	seq       atomic.Int64
	sessionID string
}

// NewEventLogBridge creates a bridge that reads from events and calls logFn
// for each event. The bridge runs until the context is cancelled or the
// events channel is closed.
func NewEventLogBridge(events <-chan session.InvestigationEvent, logFn LogFunc, logger logr.Logger, sessionID string) *EventLogBridge {
	return &EventLogBridge{
		events:    events,
		logFn:     logFn,
		logger:    logger,
		sessionID: sessionID,
	}
}

type logEnvelope struct {
	EventType string          `json:"type"`
	Seq       int64           `json:"seq"`
	Turn      int             `json:"turn"`
	Phase     string          `json:"phase,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// Run processes events until the context is cancelled or the channel closes.
func (b *EventLogBridge) Run(ctx context.Context) {
	b.logger.Info("EventLogBridge started",
		"investigation_session_id", b.sessionID,
		"events_chan_ptr", fmt.Sprintf("%p", b.events))
	defer b.logger.Info("EventLogBridge stopped",
		"investigation_session_id", b.sessionID,
		"events_forwarded", b.seq.Load())
	b.logger.Info("EventLogBridge entering for-select loop",
		"investigation_session_id", b.sessionID,
		"events_chan_nil", b.events == nil)
	for {
		select {
		case <-ctx.Done():
			b.logger.Info("EventLogBridge: ctx.Done fired",
				"investigation_session_id", b.sessionID)
			return
		case evt, ok := <-b.events:
			if !ok {
				b.logger.Info("EventLogBridge: channel closed",
					"investigation_session_id", b.sessionID,
					"events_forwarded_at_close", b.seq.Load())
				return
			}
			if b.seq.Load() == 0 {
				b.logger.Info("EventLogBridge: first event received!",
					"investigation_session_id", b.sessionID,
					"event_type", evt.Type)
			}
			b.forward(evt)
		}
	}
}

func (b *EventLogBridge) forward(evt session.InvestigationEvent) {
	seq := b.seq.Add(1)

	envelope := logEnvelope{
		EventType: evt.Type,
		Seq:       seq,
		Turn:      evt.Turn,
		Phase:     evt.Phase,
		Data:      evt.Data,
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		b.logger.Error(err, "failed to marshal event envelope", "event_type", evt.Type)
		return
	}

	level := "info"
	if evt.Type == session.EventTypeError {
		level = "error"
	}

	if logErr := b.logFn(level, "kubernaut-investigate", data); logErr != nil {
		b.logger.Error(logErr, "sess.Log delivery failed",
			"investigation_session_id", b.sessionID,
			"event_type", evt.Type,
			"seq", seq)
	}
}
