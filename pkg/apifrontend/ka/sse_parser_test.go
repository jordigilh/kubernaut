package ka_test

import (
	"context"
	"io"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
)

// collectEvents is a test helper that runs parseSSEStream via StreamEvents-like
// mechanics by using the exported InvestigationEvent type and channel pattern.
// Since parseSSEStream is unexported, we test it indirectly through StreamEvents
// with httptest servers, or directly via this helper that mimics the goroutine.
func collectEvents(ctx context.Context, input string) []ka.InvestigationEvent {
	return collectEventsFromReader(ctx, strings.NewReader(input))
}

func collectEventsFromReader(ctx context.Context, r io.Reader) []ka.InvestigationEvent {
	ch := make(chan ka.InvestigationEvent, 64)
	go func() {
		defer close(ch)
		ka.ParseSSEStreamForTest(ctx, r, ch)
	}()
	var events []ka.InvestigationEvent
	for evt := range ch {
		events = append(events, evt)
	}
	return events
}

var _ = Describe("SSE Parser", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("UT-AF-1189-010: parses single complete event", func() {
		input := "event: complete\ndata: {\"type\":\"complete\",\"turn\":3}\n\n"
		events := collectEvents(ctx, input)
		Expect(events).To(HaveLen(1))
		Expect(events[0].Type).To(Equal(ka.EventTypeComplete))
		Expect(events[0].Turn).To(Equal(3))
	})

	It("UT-AF-1189-011: parses multiple events before terminal", func() {
		input := "event: reasoning_delta\ndata: {\"turn\":1}\n\n" +
			"event: tool_call\ndata: {\"turn\":1,\"phase\":\"investigate\"}\n\n" +
			"event: complete\ndata: {\"turn\":2}\n\n"
		events := collectEvents(ctx, input)
		Expect(events).To(HaveLen(3))
		Expect(events[0].Type).To(Equal(ka.EventTypeReasoningDelta))
		Expect(events[1].Type).To(Equal(ka.EventTypeToolCall))
		Expect(events[1].Phase).To(Equal("investigate"))
		Expect(events[2].Type).To(Equal(ka.EventTypeComplete))
	})

	It("UT-AF-1189-012: stops after terminal event (ignores trailing data)", func() {
		input := "event: complete\ndata: {}\n\n" +
			"event: reasoning_delta\ndata: {\"turn\":99}\n\n"
		events := collectEvents(ctx, input)
		Expect(events).To(HaveLen(1))
		Expect(events[0].Type).To(Equal(ka.EventTypeComplete))
	})

	It("UT-AF-1189-013: stops after error event", func() {
		input := "event: error\ndata: {\"error\":\"timeout\"}\n\n" +
			"event: reasoning_delta\ndata: {}\n\n"
		events := collectEvents(ctx, input)
		Expect(events).To(HaveLen(1))
		Expect(events[0].Type).To(Equal(ka.EventTypeError))
	})

	It("UT-AF-1189-014: stops after cancelled event", func() {
		input := "event: cancelled\ndata: {}\n\n"
		events := collectEvents(ctx, input)
		Expect(events).To(HaveLen(1))
		Expect(events[0].Type).To(Equal(ka.EventTypeCancelled))
	})

	It("UT-AF-1189-015: handles multi-line data fields", func() {
		input := "event: reasoning_delta\ndata: {\"text\":\n" +
			"data: \"hello world\"}\n\n" +
			"event: complete\ndata: {}\n\n"
		events := collectEvents(ctx, input)
		Expect(events).To(HaveLen(2))
		Expect(string(events[0].Data)).To(ContainSubstring("hello world"))
	})

	It("UT-AF-1189-016: ignores lines without event: or data: prefix", func() {
		input := "event: reasoning_delta\n: this is a comment\nid: 42\ndata: {\"turn\":1}\n\n" +
			"event: complete\ndata: {}\n\n"
		events := collectEvents(ctx, input)
		Expect(events).To(HaveLen(2))
		Expect(events[0].Type).To(Equal(ka.EventTypeReasoningDelta))
	})

	It("UT-AF-1189-017: empty stream produces no events", func() {
		events := collectEvents(ctx, "")
		Expect(events).To(BeEmpty())
	})

	It("UT-AF-1189-018: blank lines only produce no events", func() {
		events := collectEvents(ctx, "\n\n\n\n")
		Expect(events).To(BeEmpty())
	})

	It("UT-AF-1189-019: data without event type uses empty type", func() {
		input := "data: {\"turn\":1}\n\n" +
			"event: complete\ndata: {}\n\n"
		events := collectEvents(ctx, input)
		Expect(events).To(HaveLen(2))
		Expect(events[0].Type).To(Equal(""))
	})

	It("UT-AF-1189-020: context cancellation stops parsing", func() {
		cancelCtx, cancel := context.WithCancel(ctx)
		pr, pw := io.Pipe()

		go func() {
			_, _ = pw.Write([]byte("event: reasoning_delta\ndata: {\"turn\":1}\n\n"))
			time.Sleep(50 * time.Millisecond)
			cancel()
			time.Sleep(50 * time.Millisecond)
			_ = pw.Close()
		}()

		events := collectEventsFromReader(cancelCtx, pr)
		Expect(events).To(HaveLen(1))
	})

	It("UT-AF-1189-021: reader error emits error event", func() {
		pr, pw := io.Pipe()
		go func() {
			_, _ = pw.Write([]byte("event: reasoning_delta\ndata: {\"turn\":1}\n\n"))
			time.Sleep(20 * time.Millisecond)
			_ = pw.CloseWithError(io.ErrUnexpectedEOF)
		}()

		events := collectEventsFromReader(ctx, pr)
		Expect(len(events)).To(BeNumerically(">=", 1))
		last := events[len(events)-1]
		Expect(last.Type).To(Equal(ka.EventTypeError))
		Expect(string(last.Data)).To(ContainSubstring("unexpected EOF"))
	})

	It("UT-AF-1189-022: malformed JSON in data still delivers event", func() {
		input := "event: reasoning_delta\ndata: not-json\n\n" +
			"event: complete\ndata: {}\n\n"
		events := collectEvents(ctx, input)
		Expect(events).To(HaveLen(2))
		Expect(string(events[0].Data)).To(Equal("not-json"))
		Expect(events[0].Turn).To(Equal(0))
	})
})
