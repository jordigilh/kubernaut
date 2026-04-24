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
	"log/slog"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/agentclient"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

var _ = Describe("TP-823-OAS: Cancel, Snapshot, Stream Endpoints (#823 PR2)", func() {

	var (
		store   *session.Store
		manager *session.Manager
		handler *server.Handler
		logger  *slog.Logger
	)

	BeforeEach(func() {
		store = session.NewStore(5 * time.Minute)
		logger = slog.Default()
		manager = session.NewManager(store, logger, nil)
		handler = server.NewHandler(manager, nil, logger)
	})

	// --- Cancel Endpoint ---

	Describe("UT-KA-823-OAS-001: Cancel running session returns 200 with cancelled status", func() {
		It("should cancel a running investigation and return the cancelled session state", func() {
			sessionID, err := manager.StartInvestigation(
				context.Background(),
				func(bgCtx context.Context) (interface{}, error) {
					<-bgCtx.Done()
					return nil, bgCtx.Err()
				},
				map[string]string{"incident_id": "cancel-test-001", "remediation_id": "rem-001"},
			)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(sessionID)
				return s.Status
			}).Should(Equal(session.StatusRunning))

			params := agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostParams{
				SessionID: sessionID,
			}
			resp, err := handler.CancelSessionAPIV1IncidentSessionSessionIDCancelPost(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			cancelResp, ok := resp.(*agentclient.CancelSessionResponse)
			Expect(ok).To(BeTrue(), "response should be *CancelSessionResponse")
			Expect(cancelResp.SessionID).To(Equal(sessionID))
			Expect(cancelResp.Status).To(Equal("cancelled"))
		})
	})

	Describe("UT-KA-823-OAS-002: Cancel nonexistent session returns 404 RFC 7807", func() {
		It("should return 404 problem+json with session ID in detail", func() {
			params := agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostParams{
				SessionID: "nonexistent-uuid",
			}
			resp, err := handler.CancelSessionAPIV1IncidentSessionSessionIDCancelPost(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			errResp, ok := resp.(*agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostNotFound)
			Expect(ok).To(BeTrue(), "response should be 404 problem+json type")
			Expect(errResp.Type).To(Equal("https://kubernaut.ai/problems/not-found"))
			Expect(errResp.Title).To(Equal("Session Not Found"))
			Expect(errResp.Detail).To(ContainSubstring("nonexistent-uuid"))
			Expect(errResp.Status).To(Equal(404))
			Expect(errResp.Instance).To(ContainSubstring("/cancel"))
		})
	})

	Describe("UT-KA-823-OAS-003: Cancel already-terminal session returns 409 RFC 7807", func() {
		It("should return 409 when session is already completed", func() {
			sessionID, err := manager.StartInvestigation(
				context.Background(),
				func(_ context.Context) (interface{}, error) {
					return &katypes.InvestigationResult{RCASummary: "done"}, nil
				},
				map[string]string{"incident_id": "completed-test"},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() session.Status {
				s, _ := manager.GetSession(sessionID)
				return s.Status
			}).Should(Equal(session.StatusCompleted))

			params := agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostParams{
				SessionID: sessionID,
			}
			resp, err := handler.CancelSessionAPIV1IncidentSessionSessionIDCancelPost(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			errResp, ok := resp.(*agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostConflict)
			Expect(ok).To(BeTrue(), "response should be 409 problem+json type")
			Expect(errResp.Type).To(Equal("https://kubernaut.ai/problems/session-already-terminal"))
			Expect(errResp.Status).To(Equal(409))
			Expect(errResp.Detail).To(ContainSubstring(sessionID))
		})
	})

	// --- Snapshot Endpoint ---

	Describe("UT-KA-823-OAS-004: Snapshot of cancelled session returns 200 with state", func() {
		It("should return session state including metadata and created_at", func() {
			sessionID, err := manager.StartInvestigation(
				context.Background(),
				func(bgCtx context.Context) (interface{}, error) {
					<-bgCtx.Done()
					return nil, bgCtx.Err()
				},
				map[string]string{"incident_id": "snap-test-001", "remediation_id": "rem-snap"},
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() session.Status {
				s, _ := manager.GetSession(sessionID)
				return s.Status
			}).Should(Equal(session.StatusRunning))

			Expect(manager.CancelInvestigation(sessionID)).To(Succeed())

			params := agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetParams{
				SessionID: sessionID,
			}
			resp, err := handler.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			snap, ok := resp.(*agentclient.SessionSnapshot)
			Expect(ok).To(BeTrue(), "response should be *SessionSnapshot")
			Expect(snap.SessionID).To(Equal(sessionID))
			Expect(snap.Status).To(Equal("cancelled"))
			Expect(snap.CreatedAt).NotTo(BeEmpty())
			_, parseErr := time.Parse(time.RFC3339, snap.CreatedAt)
			Expect(parseErr).NotTo(HaveOccurred(), "created_at should be valid RFC3339")

			md, hasMD := snap.Metadata.Get()
			Expect(hasMD).To(BeTrue())
			Expect(md["incident_id"]).To(Equal("snap-test-001"))
		})
	})

	Describe("UT-KA-823-OAS-005: Snapshot of running session returns 409", func() {
		It("should return 409 indicating session is in progress", func() {
			sessionID, err := manager.StartInvestigation(
				context.Background(),
				func(bgCtx context.Context) (interface{}, error) {
					<-bgCtx.Done()
					return nil, bgCtx.Err()
				},
				nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() session.Status {
				s, _ := manager.GetSession(sessionID)
				return s.Status
			}).Should(Equal(session.StatusRunning))

			params := agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetParams{
				SessionID: sessionID,
			}
			resp, err := handler.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			_, ok := resp.(*agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetConflict)
			Expect(ok).To(BeTrue(), "running session should return 409")
		})
	})

	Describe("UT-KA-823-OAS-006: Snapshot of nonexistent session returns 404", func() {
		It("should return 404 with RFC 7807 fields", func() {
			params := agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetParams{
				SessionID: "ghost-session",
			}
			resp, err := handler.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			errResp, ok := resp.(*agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetNotFound)
			Expect(ok).To(BeTrue(), "response should be 404 problem+json type")
			Expect(errResp.Type).To(Equal("https://kubernaut.ai/problems/not-found"))
			Expect(errResp.Detail).To(ContainSubstring("ghost-session"))
			Expect(errResp.Status).To(Equal(404))
		})
	})

	// --- Status Mapping Fix ---

	Describe("UT-KA-823-OAS-007: Status endpoint shows cancelled for cancelled session", func() {
		It("should report cancelled instead of unknown", func() {
			sessionID, err := manager.StartInvestigation(
				context.Background(),
				func(bgCtx context.Context) (interface{}, error) {
					<-bgCtx.Done()
					return nil, bgCtx.Err()
				},
				nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() session.Status {
				s, _ := manager.GetSession(sessionID)
				return s.Status
			}).Should(Equal(session.StatusRunning))

			Expect(manager.CancelInvestigation(sessionID)).To(Succeed())

			params := agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams{
				SessionID: sessionID,
			}
			resp, err := handler.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			raw, ok := resp.(*agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetOKApplicationJSON)
			Expect(ok).To(BeTrue(), "response should be 200 OK")

			var body map[string]string
			Expect(json.Unmarshal([]byte(*raw), &body)).To(Succeed())
			Expect(body["status"]).To(Equal("cancelled"),
				"mapSessionStatusToAPI must map StatusCancelled to 'cancelled', not 'unknown'")
		})
	})

	// --- Stream Stub ---

	Describe("UT-KA-823-OAS-008: Stream endpoint returns 501 Not Implemented", func() {
		It("should return ErrNotImplemented since SSE is deferred to PR4", func() {
			params := agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetParams{
				SessionID: "any-session",
			}
			_, err := handler.SessionStreamAPIV1IncidentSessionSessionIDStreamGet(context.Background(), params)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not implemented"))
		})
	})
})
