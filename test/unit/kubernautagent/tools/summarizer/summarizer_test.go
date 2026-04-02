package summarizer_test

import (
	"context"
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
})
