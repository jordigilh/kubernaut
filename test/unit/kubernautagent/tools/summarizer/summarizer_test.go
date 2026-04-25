package summarizer_test

import (
	"context"
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
)

// fakeLLM records Chat calls and returns a canned response.
type fakeLLM struct {
	calls    []llm.ChatRequest
	response string
}

func (f *fakeLLM) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	f.calls = append(f.calls, req)
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: f.response},
	}, nil
}

func (f *fakeLLM) StreamChat(ctx context.Context, req llm.ChatRequest, cb func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	resp, err := f.Chat(ctx, req)
	if err == nil {
		_ = cb(llm.ChatStreamEvent{Delta: resp.Message.Content, Done: true})
	}
	return resp, err
}

func (f *fakeLLM) Close() error { return nil }

type stubTool struct {
	name   string
	output string
}

func (s *stubTool) Name() string                                                    { return s.name }
func (s *stubTool) Description() string                                             { return "stub desc" }
func (s *stubTool) Parameters() json.RawMessage                                     { return json.RawMessage(`{}`) }
func (s *stubTool) Execute(_ context.Context, _ json.RawMessage) (string, error)    { return s.output, nil }

var _ = Describe("Kubernaut Agent Summarizer Unit — #433", func() {

	Describe("UT-KA-433-036: Pipeline triggers llm_summarize when output exceeds threshold", func() {
		It("should call the LLM client when output exceeds threshold", func() {
			fake := &fakeLLM{response: "summarized output"}
			s := summarizer.New(fake, 100)
			Expect(s).NotTo(BeNil(), "New should return a non-nil summarizer")

			longOutput := strings.Repeat("data ", 50) // 250 chars
			result, err := s.MaybeSummarize(context.Background(), "kubectl_logs", longOutput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("summarized output"),
				"should return the LLM's summarized response")
			Expect(fake.calls).To(HaveLen(1),
				"should have made exactly one LLM call")
		})
	})

	Describe("UT-KA-433-037: Below-threshold tool output passes through summarizer unchanged", func() {
		It("should return input unchanged when at or below threshold", func() {
			fake := &fakeLLM{response: "should not be called"}
			s := summarizer.New(fake, 1000)
			Expect(s).NotTo(BeNil())

			shortOutput := "metric_value=42"
			result, err := s.MaybeSummarize(context.Background(), "kubectl_describe", shortOutput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(shortOutput),
				"should pass through unchanged")
			Expect(fake.calls).To(BeEmpty(),
				"should not have called the LLM")
		})
	})

	Describe("UT-KA-433-038: Above-threshold tool output triggers secondary LLM summarization call", func() {
		It("should include tool name in the summarization prompt", func() {
			fake := &fakeLLM{response: "short summary"}
			s := summarizer.New(fake, 50)
			Expect(s).NotTo(BeNil())

			longOutput := strings.Repeat("log line\n", 20) // 180 chars
			result, err := s.MaybeSummarize(context.Background(), "kubectl_logs", longOutput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("short summary"))

			By("The summarization prompt should reference the tool name")
			Expect(fake.calls).To(HaveLen(1))
			prompt := fake.calls[0].Messages[0].Content
			Expect(prompt).To(ContainSubstring("kubectl_logs"),
				"summarization prompt should mention the tool that produced the output")
		})
	})

	Describe("UT-KA-433-540b: Wrap passes through short output unchanged", func() {
		It("should return original output when below threshold", func() {
			fake := &fakeLLM{response: "should not be called"}
			s := summarizer.New(fake, 1000)
			wrapped := s.Wrap(&stubTool{name: "kubectl_describe", output: "short"})
			result, err := wrapped.Execute(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("short"))
			Expect(fake.calls).To(BeEmpty())
		})
	})

	Describe("UT-KA-433-541: Wrap invokes LLM for long output", func() {
		It("should summarize output exceeding threshold", func() {
			fake := &fakeLLM{response: "condensed"}
			s := summarizer.New(fake, 50)
			longOutput := strings.Repeat("verbose ", 20)
			wrapped := s.Wrap(&stubTool{name: "kubectl_describe", output: longOutput})
			result, err := wrapped.Execute(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("condensed"))
			Expect(fake.calls).To(HaveLen(1))
		})
	})

	Describe("UT-KA-433-542: Wrap preserves tool metadata", func() {
		It("should delegate Name, Description, Parameters to inner tool", func() {
			fake := &fakeLLM{response: "x"}
			s := summarizer.New(fake, 100)
			inner := &stubTool{name: "kubectl_describe", output: "data"}
			wrapped := s.Wrap(inner)
			Expect(wrapped.Name()).To(Equal("kubectl_describe"))
			Expect(wrapped.Description()).To(Equal(inner.Description()))
			Expect(wrapped.Parameters()).To(Equal(inner.Parameters()))
		})
	})

	Describe("UT-KA-752-003: Summarizer pre-truncates before LLM call", func() {
		It("should pre-truncate input exceeding maxInputSize before sending to LLM", func() {
			fake := &fakeLLM{response: "summarized"}
			maxInput := 500
			s := summarizer.NewWithMaxInput(fake, 100, maxInput)

			longOutput := strings.Repeat("d", 1000)
			result, err := s.MaybeSummarize(context.Background(), "kubectl_get_by_kind_in_cluster", longOutput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("summarized"))

			Expect(fake.calls).To(HaveLen(1))
			prompt := fake.calls[0].Messages[0].Content
			Expect(len(prompt)).To(BeNumerically("<=", maxInput+200),
				"LLM prompt should not exceed maxInputSize plus format overhead")
		})
	})

	Describe("UT-KA-752-004: Summarizer works for moderate inputs", func() {
		It("should send full output when between threshold and maxInputSize", func() {
			fake := &fakeLLM{response: "moderate summary"}
			s := summarizer.NewWithMaxInput(fake, 100, 5000)

			moderateOutput := strings.Repeat("m", 300)
			result, err := s.MaybeSummarize(context.Background(), "kubectl_logs", moderateOutput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("moderate summary"))

			Expect(fake.calls).To(HaveLen(1))
			prompt := fake.calls[0].Messages[0].Content
			Expect(prompt).To(ContainSubstring(moderateOutput),
				"full output should be in prompt when below maxInputSize")
		})
	})

	Describe("UT-KA-752-006: Pre-truncation note in prompt", func() {
		It("should include truncation note in LLM prompt when input was pre-truncated", func() {
			fake := &fakeLLM{response: "summary with note"}
			s := summarizer.NewWithMaxInput(fake, 100, 400)

			longOutput := strings.Repeat("n", 800)
			_, err := s.MaybeSummarize(context.Background(), "kubectl_get_by_kind_in_cluster", longOutput)
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.calls).To(HaveLen(1))
			prompt := fake.calls[0].Messages[0].Content
			Expect(prompt).To(ContainSubstring("PRE-TRUNCATED"),
				"prompt should indicate pre-truncation occurred")
		})
	})
})
