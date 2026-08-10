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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

// fakeAutoCloseTombstone implements mcptools.AutoCloseTombstone for unit
// tests, decoupled from the real mcp.AutoCloseTombstone's TTL/goroutine
// machinery (UT-KA-2075-001 covers that in isolation).
type fakeAutoCloseTombstone struct {
	hits map[string]bool
}

func (f *fakeAutoCloseTombstone) Mark(_ string) {}

func (f *fakeAutoCloseTombstone) WasRecentlyAutoClosed(rrID string) bool {
	return f.hits[rrID]
}

// UT-KA-2075-002: complete_no_action.Handle's tombstone fallback (#2075) --
// isolated from the real no_matching_workflows race (IT-KA-2075-001 proves
// the two production call sites are actually wired together).
var _ = Describe("kubernaut_complete_no_action tool — #2075 tombstone fallback", func() {
	It("UT-KA-2075-002a (AC-6, AU-3): returns already_resolved instead of erroring on a tombstone hit", func() {
		sessions := &mockSessionManager{isActive: false}
		completer := &mockHTTPCompleter{}
		tombstone := &fakeAutoCloseTombstone{hits: map[string]bool{"rr-2075-a": true}}

		tool := mcptools.NewCompleteNoActionTool(sessions,
			mcptools.WithCompleteNoActionHTTPCompleter(completer),
			mcptools.WithCompleteNoActionAutoCloseTombstone(tombstone),
		)

		output, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{RRID: "rr-2075-a"},
			mcpinternal.UserInfo{Username: "alice"})
		Expect(err).NotTo(HaveOccurred(), "#2075: a tombstone hit must resolve the race, not error")
		Expect(output.Status).To(Equal("already_resolved"))

		_, completedResult := completer.getCompleted()
		Expect(completedResult).To(BeNil(),
			"the original auto-close already completed the HTTP session exactly once -- a tombstone hit must not re-invoke it (no duplicate audit event)")

		releasedID, _ := sessions.getReleased()
		Expect(releasedID).To(BeEmpty(),
			"the original auto-close already released the lease exactly once -- a tombstone hit must not re-invoke Release")
	})

	It("UT-KA-2075-002b: still returns the original error on a genuine miss (no active session, no tombstone hit) -- regression check", func() {
		sessions := &mockSessionManager{isActive: false}
		tombstone := &fakeAutoCloseTombstone{hits: map[string]bool{}}

		tool := mcptools.NewCompleteNoActionTool(sessions,
			mcptools.WithCompleteNoActionAutoCloseTombstone(tombstone),
		)

		_, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{RRID: "rr-2075-b"},
			mcpinternal.UserInfo{Username: "alice"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no active interactive session"))
	})

	It("UT-KA-2075-002c: does not consult the tombstone when a driver session is genuinely active (normal path unaffected)", func() {
		sessions := &mockSessionManager{
			isActive:        true,
			getDriverResult: &mcpinternal.InteractiveSession{SessionID: "sess-2075-c", ActingUser: mcpinternal.UserInfo{Username: "alice"}},
		}
		completer := &mockHTTPCompleter{}
		tombstone := &fakeAutoCloseTombstone{hits: map[string]bool{}}

		tool := mcptools.NewCompleteNoActionTool(sessions,
			mcptools.WithCompleteNoActionHTTPCompleter(completer),
			mcptools.WithCompleteNoActionAutoCloseTombstone(tombstone),
		)

		output, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{RRID: "rr-2075-c"},
			mcpinternal.UserInfo{Username: "alice"})
		Expect(err).NotTo(HaveOccurred())
		Expect(output.Status).To(Equal("completed_no_action"))

		_, completedResult := completer.getCompleted()
		Expect(completedResult).NotTo(BeNil(), "the normal, non-raced path must still complete the HTTP session exactly once")
	})

	It("UT-KA-2075-002d: an inactive session still errors exactly as before when no tombstone is configured (optional dependency, no regression)", func() {
		sessions := &mockSessionManager{isActive: false}
		tool := mcptools.NewCompleteNoActionTool(sessions)

		_, err := tool.Handle(context.Background(), mcptools.CompleteNoActionInput{RRID: "rr-2075-d"},
			mcpinternal.UserInfo{Username: "alice"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no active interactive session"))
	})
})
