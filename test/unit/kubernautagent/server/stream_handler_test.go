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

package server_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
)

type stubInvestigator struct {
	fn func(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error)
}

func (s *stubInvestigator) Investigate(ctx context.Context, signal katypes.SignalContext) (*katypes.InvestigationResult, error) {
	return s.fn(ctx, signal)
}

var _ = Describe("SSE Stream Handler — #823 PR7", func() {

	var (
		store *session.Store
		mgr   *session.Manager
		h     *server.Handler
	)

	BeforeEach(func() {
		store = session.NewStore(30 * time.Minute)
		mgr = session.NewManager(store, slog.Default(), audit.NopAuditStore{})
		h = server.NewHandler(mgr, &stubInvestigator{
			fn: func(ctx context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return &katypes.InvestigationResult{RCASummary: "cancelled"}, nil
			},
		}, slog.Default())
	})

	Describe("UT-KA-823-D01: SSE stream delivers investigation events to HTTP client", func() {
		It("returns SSE-framed events via io.Reader", func() {
			// The investigation waits for `subscribed` before emitting events,
			// ensuring the LazySink channel is active when events are sent.
			subscribed := make(chan struct{})
			proceed := make(chan struct{})
			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				<-subscribed
				sink := session.EventSinkFromContext(ctx)
				if sink != nil {
					sink <- session.InvestigationEvent{
						Type:  session.EventTypeReasoningDelta,
						Turn:  0,
						Phase: "rca",
						Data:  json.RawMessage(`{"content_preview":"test"}`),
					}
				}
				<-proceed
				return map[string]string{"rca_summary": "test"}, nil
			}, map[string]string{"remediation_id": "rr-sse-test"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := mgr.GetSession(id)
				if s == nil {
					return session.StatusPending
				}
				return s.Status
			}, 5*time.Second).Should(Equal(session.StatusRunning))

			resp, handlerErr := h.SessionStreamAPIV1IncidentSessionSessionIDStreamGet(
				context.Background(),
				agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetParams{SessionID: id},
			)
			Expect(handlerErr).NotTo(HaveOccurred())

			okResp, ok := resp.(*agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetOK)
			Expect(ok).To(BeTrue(), "response should be OK type with SSE data")
			Expect(okResp.Data).NotTo(BeNil())

			close(subscribed)
			close(proceed)
			data, readErr := io.ReadAll(okResp.Data)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("event: reasoning_delta"))
			Expect(string(data)).To(ContainSubstring("data: "))
		})
	})

	Describe("UT-KA-823-D02: Stream ends when investigation completes", func() {
		It("returns 404 for a session that has already completed", func() {
			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return map[string]string{"rca_summary": "done"}, nil
			}, map[string]string{"remediation_id": "rr-sse-eof"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(id)
				if sess == nil {
					return session.StatusPending
				}
				return sess.Status
			}, 5*time.Second).Should(Equal(session.StatusCompleted))

			resp, handlerErr := h.SessionStreamAPIV1IncidentSessionSessionIDStreamGet(
				context.Background(),
				agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetParams{SessionID: id},
			)
			Expect(handlerErr).NotTo(HaveOccurred())

			httpErr, ok := resp.(*agentclient.HTTPError)
			Expect(ok).To(BeTrue(), "already-completed session should return 404 (terminal)")
			Expect(httpErr.Status).To(Equal(404))
		})
	})

	Describe("UT-KA-823-D03: Stream for unknown session returns 404", func() {
		It("returns HTTPError for nonexistent session", func() {
			resp, handlerErr := h.SessionStreamAPIV1IncidentSessionSessionIDStreamGet(
				context.Background(),
				agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetParams{SessionID: "nonexistent-id"},
			)
			Expect(handlerErr).NotTo(HaveOccurred())

			httpErr, ok := resp.(*agentclient.HTTPError)
			Expect(ok).To(BeTrue(), "response should be HTTPError for not-found")
			Expect(httpErr.Status).To(Equal(404))
			Expect(httpErr.Detail).To(ContainSubstring("not found"))
		})
	})

	Describe("UT-KA-823-D04: Stream for terminal session returns 404", func() {
		It("returns HTTPError for completed session with no active stream", func() {
			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
				return map[string]string{"rca_summary": "done"}, nil
			}, map[string]string{"remediation_id": "rr-terminal"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(id)
				if sess == nil {
					return session.StatusPending
				}
				return sess.Status
			}, 5*time.Second).Should(Equal(session.StatusCompleted))

			resp, handlerErr := h.SessionStreamAPIV1IncidentSessionSessionIDStreamGet(
				context.Background(),
				agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetParams{SessionID: id},
			)
			Expect(handlerErr).NotTo(HaveOccurred())

			httpErr, ok := resp.(*agentclient.HTTPError)
			Expect(ok).To(BeTrue(), "response should be HTTPError for terminal session")
			Expect(httpErr.Status).To(Equal(404))
		})
	})
})
