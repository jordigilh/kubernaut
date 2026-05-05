package streaming_test

import (
	"encoding/json"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut-apifrontend/internal/streaming"
)

func TestStreamingSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Streaming Suite")
}

var _ = Describe("SSE Event Formatting", func() {
	textEvent := func(author, text string) *adksession.Event {
		evt := adksession.NewEvent("inv-1")
		evt.Author = author
		evt.Content = genai.NewContentFromText(text, genai.RoleModel)
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

	It("UT-AF-240-001: formats ADK event as SSE frame with id field", func() {
		evt := textEvent("agent", "Hello from agent")
		frame, err := streaming.FormatSSEFrame(evt, 42)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(frame)).To(ContainSubstring("id: 42"))
		Expect(string(frame)).To(ContainSubstring("event:"))
		Expect(string(frame)).To(ContainSubstring("data:"))
		Expect(string(frame)).To(HaveSuffix("\n\n"))
	})

	It("UT-AF-240-002: maps author to SSE event type", func() {
		Expect(streaming.EventTypeFromEvent(textEvent("agent", "x"))).To(Equal("agent"))
		Expect(streaming.EventTypeFromEvent(textEvent("user", "x"))).To(Equal("user"))
		Expect(streaming.EventTypeFromEvent(toolCallEvent())).To(Equal("tool-call"))
	})

	It("UT-AF-240-003: heartbeat comment frame", func() {
		frame := streaming.HeartbeatFrame()
		Expect(string(frame)).To(HavePrefix(":"))
		Expect(string(frame)).To(HaveSuffix("\n\n"))
	})

	It("UT-AF-240-004: marks terminal event", func() {
		evt := textEvent("agent", "Investigation complete")
		evt.Actions.StateDelta = map[string]any{
			streaming.StateKeyTerminal: true,
		}
		frame, err := streaming.FormatSSEFrame(evt, 5)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(frame)).To(ContainSubstring("event: done"))
	})

	It("UT-AF-240-005: skips partial events", func() {
		evt := textEvent("agent", "partial...")
		evt.Partial = true
		frame, err := streaming.FormatSSEFrame(evt, 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(frame).To(BeNil())
	})

	It("UT-AF-240-006: tool-call event for FunctionCall parts", func() {
		evt := toolCallEvent()
		eventType := streaming.EventTypeFromEvent(evt)
		Expect(eventType).To(Equal("tool-call"))
	})

	It("UT-AF-240-007: data field is valid JSON", func() {
		evt := textEvent("agent", "structured output")
		frame, err := streaming.FormatSSEFrame(evt, 1)
		Expect(err).NotTo(HaveOccurred())

		lines := strings.Split(string(frame), "\n")
		var dataLine string
		for _, line := range lines {
			if strings.HasPrefix(line, "data: ") {
				dataLine = strings.TrimPrefix(line, "data: ")
				break
			}
		}
		Expect(dataLine).NotTo(BeEmpty())
		Expect(json.Valid([]byte(dataLine))).To(BeTrue())
	})

	It("UT-AF-240-008: event type has no newlines", func() {
		evt := textEvent("agent\ninjected", "test")
		eventType := streaming.EventTypeFromEvent(evt)
		Expect(eventType).NotTo(ContainSubstring("\n"))
	})
})
