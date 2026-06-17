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

package tools_test

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

type logCapture struct {
	mu       sync.Mutex
	messages []json.RawMessage
}

func (c *logCapture) logFn(level, logger string, data json.RawMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, data)
	return nil
}

func (c *logCapture) latest() json.RawMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.messages) == 0 {
		return nil
	}
	return c.messages[len(c.messages)-1]
}

func (c *logCapture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.messages)
}

var _ = Describe("IT #1438 — EmitSessionEndedByRR → EventLogBridge wiring", func() {

	It("IT-KA-1438-001 (SI-4): Timeout handler path — session_ended propagates through EventLogBridge", func() {
		store := session.NewStore(30 * time.Minute)
		mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

		rrID := "rr-it-1438-001"

		pendingID, err := mgr.StartInteractiveSession(context.Background(),
			func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					InteractiveHold: true,
					RCASummary:      "pod crash loop",
				}, nil
			},
			map[string]string{"remediation_id": rrID},
		)
		Expect(err).NotTo(HaveOccurred())

		err = mgr.LaunchDeferredInvestigation(pendingID)
		Expect(err).NotTo(HaveOccurred())

		By("Subscribe to activate LazySink and event channel")
		eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
		Expect(subErr).NotTo(HaveOccurred())

		By("Wait for UserDriving state")
		Eventually(func() session.Status {
			s, _ := mgr.GetSession(pendingID)
			if s == nil {
				return ""
			}
			return s.Status
		}, 5*time.Second).Should(Equal(session.StatusUserDriving))

		By("Start EventLogBridge (production wiring)")
		capture := &logCapture{}
		bridge := mcptools.NewEventLogBridge(eventCh, capture.logFn, logr.Discard(), pendingID)
		bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
		defer bridgeCancel()
		go bridge.Run(bridgeCtx)

		By("Drain EventTypeComplete emitted by goroutine exit")
		Eventually(func() int { return capture.count() }, 10*time.Second).Should(BeNumerically(">=", 1))

		By("Emit session_ended as timeout handler would")
		mgr.EmitSessionEndedByRR(rrID, "inactivity_timeout")

		By("Verify EventLogBridge forwarded session_ended")
		Eventually(func() string {
			raw := capture.latest()
			if raw == nil {
				return ""
			}
			var env struct {
				EventType string `json:"type"`
				Phase     string `json:"phase"`
			}
			_ = json.Unmarshal(raw, &env)
			return env.EventType
		}, 3*time.Second).Should(Equal(session.EventTypeSessionEnded))

		var env struct {
			EventType string `json:"type"`
			Phase     string `json:"phase"`
			Seq       int64  `json:"seq"`
		}
		Expect(json.Unmarshal(capture.latest(), &env)).To(Succeed())
		Expect(env.Phase).To(Equal("inactivity_timeout"),
			"AU-3: terminal event must carry the release reason")
		Expect(env.Seq).To(BeNumerically(">", 0),
			"SI-4: event must have a positive sequence number for ordering")
	})

	It("IT-KA-1438-003 (SI-4): onSessionExpired callback path — ttl_expired emits session_ended before CompleteHTTPSession", func() {
		store := session.NewStore(30 * time.Minute)
		mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

		rrID := "rr-it-1438-003"

		pendingID, err := mgr.StartInteractiveSession(context.Background(),
			func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					InteractiveHold: true,
					RCASummary:      "ttl expiry test",
				}, nil
			},
			map[string]string{"remediation_id": rrID},
		)
		Expect(err).NotTo(HaveOccurred())

		err = mgr.LaunchDeferredInvestigation(pendingID)
		Expect(err).NotTo(HaveOccurred())

		eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
		Expect(subErr).NotTo(HaveOccurred())

		Eventually(func() session.Status {
			s, _ := mgr.GetSession(pendingID)
			if s == nil {
				return ""
			}
			return s.Status
		}, 5*time.Second).Should(Equal(session.StatusUserDriving))

		capture := &logCapture{}
		bridge := mcptools.NewEventLogBridge(eventCh, capture.logFn, logr.Discard(), pendingID)
		bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
		defer bridgeCancel()
		go bridge.Run(bridgeCtx)

		Eventually(func() int { return capture.count() }, 3*time.Second).Should(BeNumerically(">=", 1))

		By("Simulate the expanded onSessionExpired callback (main.go B3 fix)")
		mgr.EmitSessionEndedByRR(rrID, "ttl_expired")
		mcptools.CompleteHTTPSession(mgr, rrID, nil, logr.Discard(), "ttl_expired")

		By("Verify session_ended reached EventLogBridge before channel close")
		Eventually(func() string {
			raw := capture.latest()
			if raw == nil {
				return ""
			}
			var env struct {
				EventType string `json:"type"`
				Phase     string `json:"phase"`
			}
			_ = json.Unmarshal(raw, &env)
			return env.EventType
		}, 3*time.Second).Should(Equal(session.EventTypeSessionEnded))

		var env struct {
			EventType string `json:"type"`
			Phase     string `json:"phase"`
			Seq       int64  `json:"seq"`
		}
		Expect(json.Unmarshal(capture.latest(), &env)).To(Succeed())
		Expect(env.Phase).To(Equal("ttl_expired"),
			"AU-3: terminal event must carry 'ttl_expired' as release reason")
		Expect(env.Seq).To(BeNumerically(">", 0),
			"SI-4: event must have a positive sequence number")
	})

	It("IT-KA-1438-002 (SI-4): Disconnect handler path — session_ended propagates through EventLogBridge", func() {
		store := session.NewStore(30 * time.Minute)
		mgr := session.NewManager(store, logr.Discard(), audit.NopAuditStore{}, nil)

		rrID := "rr-it-1438-002"

		pendingID, err := mgr.StartInteractiveSession(context.Background(),
			func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					InteractiveHold: true,
					RCASummary:      "node not ready",
				}, nil
			},
			map[string]string{"remediation_id": rrID},
		)
		Expect(err).NotTo(HaveOccurred())

		err = mgr.LaunchDeferredInvestigation(pendingID)
		Expect(err).NotTo(HaveOccurred())

		eventCh, subErr := mgr.Subscribe(context.Background(), pendingID)
		Expect(subErr).NotTo(HaveOccurred())

		Eventually(func() session.Status {
			s, _ := mgr.GetSession(pendingID)
			if s == nil {
				return ""
			}
			return s.Status
		}, 5*time.Second).Should(Equal(session.StatusUserDriving))

		capture := &logCapture{}
		bridge := mcptools.NewEventLogBridge(eventCh, capture.logFn, logr.Discard(), pendingID)
		bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
		defer bridgeCancel()
		go bridge.Run(bridgeCtx)

		Eventually(func() int { return capture.count() }, 3*time.Second).Should(BeNumerically(">=", 1))

		By("Emit session_ended as disconnect handler would")
		mgr.EmitSessionEndedByRR(rrID, "disconnect")

		By("Verify EventLogBridge forwarded session_ended with disconnect reason")
		Eventually(func() string {
			raw := capture.latest()
			if raw == nil {
				return ""
			}
			var env struct {
				EventType string `json:"type"`
			}
			_ = json.Unmarshal(raw, &env)
			return env.EventType
		}, 3*time.Second).Should(Equal(session.EventTypeSessionEnded))

		var env struct {
			EventType string `json:"type"`
			Phase     string `json:"phase"`
		}
		Expect(json.Unmarshal(capture.latest(), &env)).To(Succeed())
		Expect(env.Phase).To(Equal("disconnect"),
			"AU-3: terminal event must carry 'disconnect' as release reason")
	})
})
