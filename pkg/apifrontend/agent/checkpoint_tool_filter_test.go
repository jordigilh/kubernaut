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

package agent

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// statefulCallbackContext extends stubCallbackContext (defined in
// history_sanitizer_test.go) with a working session.State, so
// BeforeModelCallback tests can exercise checkpoint-flag-aware logic
// (DD-AF-011, #1899).
type statefulCallbackContext struct {
	*stubCallbackContext
	state adksession.State
}

func (s statefulCallbackContext) State() adksession.State { return s.state }

func newTestReq(toolNames ...string) *model.LLMRequest {
	tools := make(map[string]any, len(toolNames))
	for _, name := range toolNames {
		tools[name] = struct{}{}
	}
	return &model.LLMRequest{Tools: tools}
}

var _ = Describe("checkpointToolFilter BeforeModelCallback (DD-AF-011, #1899)", func() {
	newCtx := func(state *mapState) *statefulCallbackContext {
		return &statefulCallbackContext{
			stubCallbackContext: &stubCallbackContext{Context: context.Background()},
			state:               state,
		}
	}

	It("IT-AF-1899-004a: removes kubernaut_discover_workflows from the tool list when af_phase2_blocked is set", func() {
		state := newMapState()
		Expect(state.Set(session.StateKeyPhase2Blocked, true)).To(Succeed())
		req := newTestReq("kubernaut_investigate", "kubernaut_discover_workflows", "kubernaut_select_workflow")

		resp, err := checkpointToolFilter(newCtx(state), req)

		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeNil(), "must return nil to let the model call proceed with the filtered tool list")
		Expect(req.Tools).NotTo(HaveKey("kubernaut_discover_workflows"),
			"the model must not even see discover_workflows as an option while phase 2 is blocked")
		Expect(req.Tools).To(HaveKey("kubernaut_investigate"), "unrelated tools must remain untouched")
	})

	It("IT-AF-1899-004b: removes kubernaut_select_workflow from the tool list when af_phase3_blocked is set", func() {
		state := newMapState()
		Expect(state.Set(session.StateKeyPhase3Blocked, true)).To(Succeed())
		req := newTestReq("kubernaut_investigate", "kubernaut_discover_workflows", "kubernaut_select_workflow")

		resp, err := checkpointToolFilter(newCtx(state), req)

		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeNil())
		Expect(req.Tools).NotTo(HaveKey("kubernaut_select_workflow"),
			"the model must not even see select_workflow as an option while phase 3 is blocked")
		Expect(req.Tools).To(HaveKey("kubernaut_discover_workflows"),
			"discover_workflows must remain available -- only phase 3 is blocked here")
	})

	It("IT-AF-1899-004c: leaves the tool list untouched when no checkpoint is blocked", func() {
		state := newMapState()
		req := newTestReq("kubernaut_investigate", "kubernaut_discover_workflows", "kubernaut_select_workflow")

		resp, err := checkpointToolFilter(newCtx(state), req)

		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeNil())
		Expect(req.Tools).To(HaveKey("kubernaut_discover_workflows"))
		Expect(req.Tools).To(HaveKey("kubernaut_select_workflow"))
	})

	It("IT-AF-1899-004d: handles a nil State without panicking (fail-safe: no filtering possible, defers to hard-reject backstop)", func() {
		req := newTestReq("kubernaut_discover_workflows")
		ctx := &statefulCallbackContext{stubCallbackContext: &stubCallbackContext{Context: context.Background()}, state: nil}

		resp, err := checkpointToolFilter(ctx, req)

		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeNil())
	})

	It("IT-AF-1899-004e: handles a nil/empty Tools map without panicking", func() {
		state := newMapState()
		Expect(state.Set(session.StateKeyPhase2Blocked, true)).To(Succeed())

		resp, err := checkpointToolFilter(newCtx(state), &model.LLMRequest{})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp).To(BeNil())
	})
})
