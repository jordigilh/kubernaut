package session_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
	"github.com/jordigilh/kubernaut-apifrontend/internal/session"
)

var _ = Describe("Re-invocation Fallback", func() {
	ctx := context.Background()
	inmem := adksession.InMemoryService()

	getEvents := func(events ...*adksession.Event) adksession.Events {
		resp, err := inmem.Create(ctx, &adksession.CreateRequest{
			AppName: "test",
			UserID:  "test",
		})
		Expect(err).NotTo(HaveOccurred())
		for _, evt := range events {
			err := inmem.AppendEvent(ctx, resp.Session, evt)
			Expect(err).NotTo(HaveOccurred())
		}
		getResp, err := inmem.Get(ctx, &adksession.GetRequest{
			AppName:   "test",
			UserID:    "test",
			SessionID: resp.Session.ID(),
		})
		Expect(err).NotTo(HaveOccurred())
		return getResp.Session.Events()
	}

	textEvent := func() *adksession.Event {
		evt := adksession.NewEvent("inv-1")
		evt.Author = "agent"
		evt.Content = genai.NewContentFromText("analysis complete", genai.RoleModel)
		return evt
	}

	toolCallEvent := func() *adksession.Event {
		evt := adksession.NewEvent("inv-1")
		evt.Author = "agent"
		evt.Content = &genai.Content{
			Role: string(genai.RoleModel),
			Parts: []*genai.Part{
				{
					FunctionCall: &genai.FunctionCall{
						Name: "af_get_pods",
						Args: map[string]any{"namespace": "default"},
					},
				},
			},
		}
		return evt
	}

	It("UT-AF-230-001: detects text-only turn end during active investigation", func() {
		events := getEvents(textEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, 0)
		Expect(result).To(BeTrue())
	})

	It("UT-AF-230-002: does not trigger with tool calls", func() {
		events := getEvents(toolCallEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, 0)
		Expect(result).To(BeFalse())
	})

	It("UT-AF-230-003: does not trigger when terminal", func() {
		events := getEvents(textEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseCompleted, events, 0)
		Expect(result).To(BeFalse())
	})

	It("UT-AF-230-004: generates correct synthetic message", func() {
		msg := session.SyntheticMessage()
		Expect(msg).NotTo(BeNil())
		Expect(msg.Role).To(Equal(string(genai.RoleUser)))
		Expect(msg.Parts).To(HaveLen(1))
		Expect(msg.Parts[0].Text).NotTo(BeEmpty())
	})

	It("UT-AF-230-005: tracks reinvocation count", func() {
		events := getEvents(textEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, 1)
		Expect(result).To(BeTrue())
	})

	It("UT-AF-230-006: stops after max reinvocations", func() {
		events := getEvents(textEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, session.MaxReinvocations)
		Expect(result).To(BeFalse())
	})

	It("UT-AF-230-007: does not trigger when Disconnected", func() {
		events := getEvents(textEvent())
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseDisconnected, events, 0)
		Expect(result).To(BeFalse())
	})

	It("UT-AF-230-008: does not trigger with empty events", func() {
		events := getEvents()
		result := session.NeedsReinvocation(v1alpha1.SessionPhaseActive, events, 0)
		Expect(result).To(BeFalse())
	})
})
