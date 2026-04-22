package summarizer_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools/summarizer"
)

// fakeLLM records Chat calls and returns a canned summary.
type fakeLLM struct {
	calls    []llm.ChatRequest
	response string
}

func (f *fakeLLM) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	f.calls = append(f.calls, req)
	return llm.ChatResponse{
		Message: llm.Message{Role: "assistant", Content: f.response},
		Usage:   llm.TokenUsage{PromptTokens: 100, CompletionTokens: 30, TotalTokens: 130},
	}, nil
}

func (f *fakeLLM) Close() error { return nil }

var _ = Describe("Kubernaut Agent Summarizer Integration — #433", func() {

	Describe("IT-KA-433-037: Summarizer produces shortened output via secondary llm.Client", func() {
		It("should produce a shorter result through LLM summarization", func() {
			fake := &fakeLLM{response: "Container api-server OOMKilled 3 times in the last hour. Memory limit: 256Mi."}
			s := summarizer.New(fake, 100)
			Expect(s).NotTo(BeNil())

			longLogs := strings.Repeat("2026-03-01T10:00:00Z api-server container OOMKilled\n", 50)
			Expect(len(longLogs)).To(BeNumerically(">", 100), "input should exceed threshold")

			result, err := s.MaybeSummarize(context.Background(), "kubectl_logs", longLogs)
			Expect(err).NotTo(HaveOccurred())

			By("The summarized result should be shorter than the original")
			Expect(len(result)).To(BeNumerically("<", len(longLogs)))
			Expect(result).To(ContainSubstring("OOMKilled"))

			By("The LLM should have been called exactly once")
			Expect(fake.calls).To(HaveLen(1))

			By("The request should reference the tool name for context")
			Expect(fake.calls[0].Messages[0].Content).To(ContainSubstring("kubectl_logs"))
		})

		It("should not call LLM when output is within threshold", func() {
			fake := &fakeLLM{response: "should not be used"}
			s := summarizer.New(fake, 1000)
			Expect(s).NotTo(BeNil())

			shortOutput := "container running normally"
			result, err := s.MaybeSummarize(context.Background(), "kubectl_describe", shortOutput)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(shortOutput))
			Expect(fake.calls).To(BeEmpty())
		})
	})
})
