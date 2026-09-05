package agent

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

var _ = Describe("Presentation recovery guard (#2365)", func() {
	It("IT-AF-2365-001 (SI-10, AU-3): retains only present_decision in both ADK tool representations", func() {
		state := newMapState()
		Expect(state.Set(session.StateKeyPresentationRequired, true)).To(Succeed())
		req := &model.LLMRequest{
			Tools: map[string]any{
				"kubernaut_present_decision": struct{}{},
				"kubernaut_select_workflow":  struct{}{},
			},
			Config: &genai.GenerateContentConfig{Tools: []*genai.Tool{{
				FunctionDeclarations: []*genai.FunctionDeclaration{
					{Name: "kubernaut_present_decision"},
					{Name: "kubernaut_select_workflow"},
				},
			}}},
		}

		ctx := &statefulCallbackContext{stubCallbackContext: &stubCallbackContext{Context: context.Background()}, state: state}
		_, err := checkpointToolFilter(ctx, req)
		Expect(err).NotTo(HaveOccurred())
		Expect(req.Tools).To(HaveKey("kubernaut_present_decision"))
		Expect(req.Tools).NotTo(HaveKey("kubernaut_select_workflow"))
		Expect(req.Config.Tools[0].FunctionDeclarations).To(HaveLen(1))
		Expect(req.Config.Tools[0].FunctionDeclarations[0].Name).To(Equal("kubernaut_present_decision"))
		Expect(req.Config.ToolConfig.FunctionCallingConfig.AllowedFunctionNames).To(ConsistOf("kubernaut_present_decision"))
	})

	It("UT-AF-2365-005 (AC-6): clears recovery after genuine workflow selection", func() {
		state := newMapState()
		Expect(state.Set(session.StateKeyPresentationRequired, true)).To(Succeed())
		ctx := &statefulToolContext{
			fakeToolContext: fakeToolContext{Context: context.Background()},
			state:           state,
		}
		_, after := NewPhaseGuardForTest()
		_, err := after(ctx, fakeTool{name: "kubernaut_select_workflow"}, nil, map[string]any{"selected": true}, nil)
		Expect(err).NotTo(HaveOccurred())
		value, getErr := state.Get(session.StateKeyPresentationRequired)
		Expect(getErr).NotTo(HaveOccurred())
		Expect(value).To(BeFalse())
	})
})
