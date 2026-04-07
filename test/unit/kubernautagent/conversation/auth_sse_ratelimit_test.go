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

package conversation_test

import (
	"context"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/conversation"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

var _ = Describe("Auth + SSE + Rate Limit — #592", func() {

	Describe("UT-CS-592-010: TokenReview validates bearer token", func() {
		It("should authenticate valid token and return user identity", func() {
			mockAuthn := &auth.MockAuthenticator{
				ValidUsers: map[string]string{
					"valid-token-123": "user:alice@example.com",
				},
			}
			mockAuthz := &auth.MockAuthorizer{AllowedUsers: map[string]bool{"user:alice@example.com": true}}
			convAuth := conversation.NewConversationAuth(mockAuthn, mockAuthz)

			userID, err := convAuth.Authenticate(context.Background(), "valid-token-123")
			Expect(err).NotTo(HaveOccurred())
			Expect(userID).To(Equal("user:alice@example.com"),
				"valid token must return the authenticated user identity")
		})
	})

	Describe("UT-CS-592-011: SAR: user can UPDATE RAR -> authorized", func() {
		It("should authorize user with UPDATE permission on target RAR", func() {
			mockAuthn := &auth.MockAuthenticator{
				ValidUsers: map[string]string{"token": "user:bob"},
			}
			mockAuthz := &auth.MockAuthorizer{AllowedUsers: map[string]bool{"user:bob": true}}
			convAuth := conversation.NewConversationAuth(mockAuthn, mockAuthz)

			allowed, err := convAuth.AuthorizeRAR(context.Background(),
				"user:bob", "production", "payment-svc-oomkill")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue(),
				"user with UPDATE permission on RAR must be authorized")
		})
	})

	Describe("UT-CS-592-012: SAR denied -> 403", func() {
		It("should deny user without UPDATE permission on target RAR", func() {
			mockAuthn := &auth.MockAuthenticator{
				ValidUsers: map[string]string{"token": "user:eve"},
			}
			mockAuthz := &auth.MockAuthorizer{}
			convAuth := conversation.NewConversationAuth(mockAuthn, mockAuthz)

			allowed, err := convAuth.AuthorizeRAR(context.Background(),
				"user:eve", "production", "payment-svc-oomkill")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse(),
				"user without UPDATE permission on RAR must be denied")
		})
	})

	Describe("UT-CS-592-C1-001: AuthorizeRAR uses CheckAccessWithGroup with kubernaut.ai group", func() {
		It("should call CheckAccessWithGroup with kubernaut.ai API group", func() {
			mockAuthn := &auth.MockAuthenticator{
				ValidUsers: map[string]string{"token": "user:operator"},
			}
			mockAuthz := &auth.MockAuthorizer{
				PerGroupResourceDecisions: map[string]map[string]bool{
					"kubernaut.ai/production/remediationapprovalrequests/oom-fix/update": {
						"user:operator": true,
					},
				},
			}
			convAuth := conversation.NewConversationAuth(mockAuthn, mockAuthz)

			allowed, err := convAuth.AuthorizeRAR(context.Background(),
				"user:operator", "production", "oom-fix")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue(),
				"AuthorizeRAR must use CheckAccessWithGroup with kubernaut.ai API group")
		})
	})

	Describe("UT-CS-592-013: SSE events include incrementing ID", func() {
		It("should assign incrementing IDs to SSE events", func() {
			writer := conversation.NewSSEWriter(60 * time.Second)

			evt1 := writer.WriteEvent("message", "Hello")
			evt2 := writer.WriteEvent("message", "World")
			evt3 := writer.WriteEvent("message", "!")

			Expect(evt1).NotTo(BeNil(), "WriteEvent must return an event")
			Expect(evt2).NotTo(BeNil())
			Expect(evt3).NotTo(BeNil())

			id1, _ := strconv.Atoi(evt1.ID)
			id2, _ := strconv.Atoi(evt2.ID)
			id3, _ := strconv.Atoi(evt3.ID)
			Expect(id1).To(BeNumerically("<", id2))
			Expect(id2).To(BeNumerically("<", id3))
		})
	})

	Describe("UT-CS-592-014: SSE reconnection via Last-Event-ID", func() {
		It("should replay events from the given Last-Event-ID", func() {
			writer := conversation.NewSSEWriter(60 * time.Second)

			evt1 := writer.WriteEvent("message", "Hello")
			_ = writer.WriteEvent("message", "World")
			_ = writer.WriteEvent("message", "!")

			Expect(evt1).NotTo(BeNil())
			replayed := writer.ReplayFrom(evt1.ID)
			Expect(replayed).NotTo(BeEmpty(),
				"ReplayFrom must return events after the given ID")
			Expect(len(replayed)).To(BeNumerically(">=", 2),
				"should replay events 2 and 3 after event 1")
		})
	})

	Describe("UT-CS-592-015: SSE 60s response buffer", func() {
		It("should buffer events for reconnection within TTL", func() {
			writer := conversation.NewSSEWriter(60 * time.Second)

			_ = writer.WriteEvent("message", "First")
			evt2 := writer.WriteEvent("message", "Second")

			Expect(evt2).NotTo(BeNil())
			replayed := writer.ReplayFrom("0")
			Expect(len(replayed)).To(BeNumerically(">=", 2),
				"buffer should retain events within TTL window")
		})
	})

	Describe("UT-CS-592-016: Per-user rate limit", func() {
		It("should reject the 11th request within 1 minute", func() {
			limiter := conversation.NewRateLimiter(10, 30)

			for i := 0; i < 10; i++ {
				Expect(limiter.AllowUser("user:alice")).To(BeTrue(),
					"first 10 requests should be allowed")
			}
			Expect(limiter.AllowUser("user:alice")).To(BeFalse(),
				"11th request within 1 minute must be rate-limited (429)")
		})
	})

	Describe("UT-CS-592-017: Per-session rate limit", func() {
		It("should reject the 31st turn in a session", func() {
			limiter := conversation.NewRateLimiter(10, 30)

			Expect(limiter.AllowSession("session-1", 30)).To(BeTrue(),
				"30th turn should be allowed (at limit)")
			Expect(limiter.AllowSession("session-1", 31)).To(BeFalse(),
				"31st turn must be rate-limited (429)")
		})
	})

	Describe("UT-CS-592-018: Conversation turn emits audit event with identity", func() {
		It("should emit an audit event with user identity and correlation_id", func() {
			store := &capturingConvAuditStore{}
			emitter := conversation.NewTurnAuditor(store)

			emitter.EmitTurn(context.Background(), "session-1", "user:alice", "rem-001",
				"What caused the OOM?", "The pod exceeded memory limits due to...")

			Expect(store.events).NotTo(BeEmpty(),
				"EmitTurn must store an audit event")
			evt := store.events[0]
			Expect(evt.Data).To(HaveKeyWithValue("user_id", "user:alice"))
			Expect(evt.CorrelationID).To(Equal("rem-001"))
			Expect(evt.Data).To(HaveKey("session_id"))
		})
	})
})

type capturingConvAuditStore struct {
	events []*audit.AuditEvent
}

func (s *capturingConvAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}
