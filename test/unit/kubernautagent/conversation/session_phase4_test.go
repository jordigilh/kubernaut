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
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/conversation"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

var _ = Describe("Session Phase 4 — #592 (todoWrite, CorrelationID, Messages)", func() {

	Describe("UT-CS-592-G2-004: Per-session todoWrite tool", func() {
		It("should create sessions with independent todoWrite tools", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)

			s1, err := mgr.Create("rar-1", "ns-1", "user:alice", "corr-1")
			Expect(err).ToNot(HaveOccurred())
			Expect(s1.TodoWrite()).ToNot(BeNil(), "session must have a todoWrite tool")
			Expect(s1.TodoWrite().Name()).To(Equal("todo_write"))

			s2, err := mgr.Create("rar-2", "ns-2", "user:bob", "corr-2")
			Expect(err).ToNot(HaveOccurred())
			Expect(s2.TodoWrite().Name()).To(Equal("todo_write"))

			Expect(s1.TodoWrite()).ToNot(BeIdenticalTo(s2.TodoWrite()),
				"each session must have its own todoWrite instance")
		})
	})

	Describe("UT-CS-592-G2-005: CorrelationID propagation", func() {
		It("should propagate correlation ID from create request", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)

			s, err := mgr.Create("rar-1", "ns-1", "user:alice", "my-correlation-123")
			Expect(err).ToNot(HaveOccurred())
			Expect(s.CorrelationID).To(Equal("my-correlation-123"))
		})

		It("should fall back to session ID when correlation ID is empty", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)

			s, err := mgr.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(s.CorrelationID).To(Equal(s.ID),
				"empty correlation ID should fall back to session ID")
		})
	})

	Describe("UT-CS-592-G2-006: Messages persistence", func() {
		It("should initialize empty and persist after AppendMessages", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)
			s, err := mgr.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			Expect(s.GetMessages()).To(BeEmpty())

			s.AppendMessages(
				llm.Message{Role: "user", Content: "What caused the OOM?"},
				llm.Message{Role: "assistant", Content: "The pod exceeded memory limits."},
			)

			msgs := s.GetMessages()
			Expect(msgs).To(HaveLen(2))
			Expect(msgs[0].Role).To(Equal("user"))
			Expect(msgs[1].Role).To(Equal("assistant"))
		})
	})

	Describe("UT-CS-592-G2-007: GetMessages returns a copy", func() {
		It("should not affect session state when returned slice is mutated", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)
			s, err := mgr.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			s.AppendMessages(llm.Message{Role: "user", Content: "Hello"})

			copy1 := s.GetMessages()
			copy1[0].Content = "MUTATED"

			original := s.GetMessages()
			Expect(original[0].Content).To(Equal("Hello"),
				"mutating the returned copy must not affect session state")
		})
	})

	Describe("UT-CS-592-G2-008: Concurrent GetMessages + AppendMessages", func() {
		It("should not race under concurrent access", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)
			s, err := mgr.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			var wg sync.WaitGroup
			for i := 0; i < 50; i++ {
				wg.Add(2)
				go func() {
					defer wg.Done()
					s.AppendMessages(llm.Message{Role: "user", Content: "msg"})
				}()
				go func() {
					defer wg.Done()
					_ = s.GetMessages()
				}()
			}
			wg.Wait()

			Expect(len(s.GetMessages())).To(BeNumerically(">=", 50))
		})
	})

	Describe("UT-CS-592-G2-009: Session.SystemPrompt with nil builder", func() {
		It("should return error when promptBuilder is nil (DD-F6)", func() {
			mgr := conversation.NewSessionManager(30*time.Minute, nil)
			s, err := mgr.Create("rar-1", "ns-1", "user:alice", "")
			Expect(err).ToNot(HaveOccurred())

			_, err = s.SystemPrompt()
			Expect(err).To(HaveOccurred(),
				"nil promptBuilder must return an error, not empty string")
			Expect(err.Error()).To(ContainSubstring("prompt builder not initialized"))
		})
	})
})
