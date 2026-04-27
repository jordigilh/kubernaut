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
	"log/slog"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

func userCtx(user string) context.Context {
	return context.WithValue(context.Background(), auth.UserContextKey, user)
}

var _ = Describe("Session Object-Level Authorization — #823 PR7.5", func() {

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

	createSessionAs := func(user string) string {
		ctx := userCtx(user)
		proceed := make(chan struct{})
		id, err := mgr.StartInvestigation(ctx, func(bgCtx context.Context) (interface{}, error) {
			<-proceed
			return &katypes.InvestigationResult{RCASummary: "done"}, nil
		}, map[string]string{"remediation_id": "rr-authz-test"})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { close(proceed) })
		return id
	}

	Describe("UT-KA-823-A01: Session created_by stored in metadata", func() {
		It("session metadata contains the creating user identity", func() {
			id := createSessionAs("system:serviceaccount:kubernaut:operator-a")

			sess, err := mgr.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Metadata).To(HaveKeyWithValue("created_by", "system:serviceaccount:kubernaut:operator-a"))
		})
	})

	Describe("UT-KA-823-A02: Owner can access their session status", func() {
		It("returns 200 when owner queries session status", func() {
			id := createSessionAs("user-a")

			resp, err := h.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(
				userCtx("user-a"),
				agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams{SessionID: id},
			)
			Expect(err).NotTo(HaveOccurred())

			_, ok := resp.(*agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetOKApplicationJSON)
			Expect(ok).To(BeTrue(), "owner should receive 200 OK for session status")
		})
	})

	Describe("UT-KA-823-A03: Non-owner gets 404 on session status", func() {
		It("returns 404 when non-owner queries session status", func() {
			id := createSessionAs("user-a")

			resp, err := h.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(
				userCtx("user-b"),
				agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams{SessionID: id},
			)
			Expect(err).NotTo(HaveOccurred())

			httpErr, ok := resp.(*agentclient.HTTPError)
			Expect(ok).To(BeTrue(), "non-owner should receive HTTPError (404)")
			Expect(httpErr.Status).To(Equal(404))
		})
	})

	Describe("UT-KA-823-A04: Non-owner gets 404 on cancel", func() {
		It("returns 404 when non-owner tries to cancel session", func() {
			id := createSessionAs("user-a")

			resp, err := h.CancelSessionAPIV1IncidentSessionSessionIDCancelPost(
				userCtx("user-b"),
				agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostParams{SessionID: id},
			)
			Expect(err).NotTo(HaveOccurred())

			_, ok := resp.(*agentclient.CancelSessionAPIV1IncidentSessionSessionIDCancelPostNotFound)
			Expect(ok).To(BeTrue(), "non-owner should receive 404 on cancel")
		})
	})

	Describe("UT-KA-823-A05: Non-owner gets 404 on snapshot", func() {
		It("returns 404 when non-owner tries to read snapshot", func() {
			id := createSessionAs("user-a")

			resp, err := h.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGet(
				userCtx("user-b"),
				agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetParams{SessionID: id},
			)
			Expect(err).NotTo(HaveOccurred())

			httpErr, ok := resp.(*agentclient.SessionSnapshotAPIV1IncidentSessionSessionIDSnapshotGetNotFound)
			Expect(ok).To(BeTrue(), "non-owner should receive 404 on snapshot")
			Expect(httpErr.Status).To(Equal(404))
		})
	})

	Describe("UT-KA-823-A06: Non-owner gets 404 on stream", func() {
		It("returns 404 when non-owner tries to subscribe to stream", func() {
			id := createSessionAs("user-a")

			resp, err := h.SessionStreamAPIV1IncidentSessionSessionIDStreamGet(
				userCtx("user-b"),
				agentclient.SessionStreamAPIV1IncidentSessionSessionIDStreamGetParams{SessionID: id},
			)
			Expect(err).NotTo(HaveOccurred())

			httpErr, ok := resp.(*agentclient.HTTPError)
			Expect(ok).To(BeTrue(), "non-owner should receive HTTPError (404) on stream")
			Expect(httpErr.Status).To(Equal(404))
		})
	})

	Describe("UT-KA-823-A07: Non-owner gets 404 on result", func() {
		It("returns 404 when non-owner tries to read result", func() {
			completedStore := session.NewStore(30 * time.Minute)
			completedMgr := session.NewManager(completedStore, slog.Default(), audit.NopAuditStore{})
			completedH := server.NewHandler(completedMgr, nil, slog.Default())

			id, sErr := completedMgr.StartInvestigation(userCtx("user-a"), func(ctx context.Context) (interface{}, error) {
				return &katypes.InvestigationResult{RCASummary: "done", Confidence: 0.9}, nil
			}, map[string]string{"remediation_id": "rr-authz-result"})
			Expect(sErr).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := completedMgr.GetSession(id)
				if sess == nil {
					return session.StatusPending
				}
				return sess.Status
			}, 5*time.Second).Should(Equal(session.StatusCompleted))

			resp, err := completedH.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGet(
				userCtx("user-b"),
				agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetParams{SessionID: id},
			)
			Expect(err).NotTo(HaveOccurred())

			_, ok := resp.(*agentclient.IncidentSessionResultEndpointAPIV1IncidentSessionSessionIDResultGetNotFound)
			Expect(ok).To(BeTrue(), "non-owner should receive 404 on result")
		})
	})

	Describe("UT-KA-823-A08: Empty user (no auth) bypasses authz", func() {
		It("allows access when auth middleware is disabled (no user in context)", func() {
			id := createSessionAs("")

			resp, err := h.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(
				context.Background(),
				agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams{SessionID: id},
			)
			Expect(err).NotTo(HaveOccurred())

			_, ok := resp.(*agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetOKApplicationJSON)
			Expect(ok).To(BeTrue(), "unauthenticated context should still access sessions (dev mode)")
		})
	})

	Describe("UT-KA-823-A10: Denied access emits audit event", func() {
		It("emits aiagent.session.access_denied when non-owner attempts access", func() {
			recorder := &syncAuditRecorder{}
			authzStore := session.NewStore(30 * time.Minute)
			authzMgr := session.NewManager(authzStore, slog.Default(), recorder)
			authzH := server.NewHandler(authzMgr, nil, slog.Default())

			proceed := make(chan struct{})
			id, err := authzMgr.StartInvestigation(userCtx("user-a"), func(ctx context.Context) (interface{}, error) {
				<-proceed
				return nil, nil
			}, map[string]string{"remediation_id": "rr-denied-audit"})
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { close(proceed) })

			resp, err := authzH.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(
				userCtx("user-b"),
				agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams{SessionID: id},
			)
			Expect(err).NotTo(HaveOccurred())

			httpErr, ok := resp.(*agentclient.HTTPError)
			Expect(ok).To(BeTrue())
			Expect(httpErr.Status).To(Equal(404))

		Eventually(func() bool {
			for _, evt := range recorder.Events() {
				if evt.EventType == audit.EventTypeSessionAccessDenied {
					user, _ := evt.Data["requesting_user"].(string)
					sid, _ := evt.Data["session_id"].(string)
					ep, _ := evt.Data["endpoint"].(string)
					return user == "user-b" && sid == id && ep != ""
				}
			}
			return false
		}, 5*time.Second).Should(BeTrue(),
			"access_denied audit event must include requesting_user, session_id, and endpoint")

			By("verifying session_owner is included (GAP-T5 / SEC-2)")
			var found *audit.AuditEvent
			for _, evt := range recorder.Events() {
				if evt.EventType == audit.EventTypeSessionAccessDenied {
					found = evt
					break
				}
			}
			Expect(found).NotTo(BeNil())
			owner, _ := found.Data["session_owner"].(string)
			Expect(owner).To(Equal("user-a"), "session_owner must identify the session creator")
			Expect(found.CorrelationID).To(Equal("rr-denied-audit"),
				"correlationID must be the remediation_id from session metadata")
		})
	})

	Describe("UT-KA-823-A09: session.observed includes observer identity", func() {
		It("audit event includes the subscribing user extracted from context", func() {
			recorder := &syncAuditRecorder{}
			authzStore := session.NewStore(30 * time.Minute)
			authzMgr := session.NewManager(authzStore, slog.Default(), recorder)

			proceed := make(chan struct{})
			id, err := authzMgr.StartInvestigation(userCtx("user-a"), func(ctx context.Context) (interface{}, error) {
				<-proceed
				return nil, nil
			}, map[string]string{"remediation_id": "rr-observer-audit"})
			Expect(err).NotTo(HaveOccurred())

			_, subErr := authzMgr.Subscribe(userCtx("user-a"), id)
			Expect(subErr).NotTo(HaveOccurred())

			close(proceed)

		Eventually(func() bool {
			for _, evt := range recorder.Events() {
				if evt.EventType == audit.EventTypeSessionObserved {
					if observer, ok := evt.Data["observer_user"].(string); ok {
						return observer == "user-a"
					}
				}
			}
			return false
		}, 5*time.Second).Should(BeTrue(),
			"session.observed audit event must include observer_user identity")

			By("verifying session_owner is included (GAP-T6 / SEC-4)")
			var observedEvt *audit.AuditEvent
			for _, evt := range recorder.Events() {
				if evt.EventType == audit.EventTypeSessionObserved {
					observedEvt = evt
					break
				}
			}
			Expect(observedEvt).NotTo(BeNil())
			owner, _ := observedEvt.Data["session_owner"].(string)
			Expect(owner).To(Equal("user-a"), "session_owner must identify the session creator in observed events")
		})
	})
})

type syncAuditRecorder struct {
	mu     sync.Mutex
	events []*audit.AuditEvent
}

func (r *syncAuditRecorder) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *syncAuditRecorder) Events() []*audit.AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]*audit.AuditEvent, len(r.events))
	copy(cp, r.events)
	return cp
}
