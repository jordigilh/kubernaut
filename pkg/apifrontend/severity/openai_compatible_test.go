package severity_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
)

// fakeChatClient is a test double for severity.ChatClient, mirroring the
// existing promptCaptureLLM/mockLLM pattern used for GenAITriager in
// llm_test.go.
type fakeChatClient struct {
	resp        *openaicompat.Response
	err         error
	capturedReq openaicompat.Request
}

func (f *fakeChatClient) Chat(_ context.Context, req openaicompat.Request) (*openaicompat.Response, error) {
	f.capturedReq = req
	return f.resp, f.err
}

var _ = Describe("OpenAICompatibleTriager", func() {
	var defaultInput severity.TriageInput

	BeforeEach(func() {
		defaultInput = severity.TriageInput{
			Namespace:   "prod",
			Kind:        "Deployment",
			Name:        "web-api",
			Description: "High error rate",
		}
	})

	Describe("UT-AF-1618-001: Construction", func() {
		It("panics with nil ChatClient", func() {
			Expect(func() {
				severity.NewOpenAICompatibleTriager(severity.OpenAICompatibleTriagerConfig{})
			}).To(Panic())
		})
	})

	Describe("UT-AF-1618-002: TriagePure", func() {
		It("sends the triage prompt as a single user message tagged with the configured model", func() {
			fake := &fakeChatClient{
				resp: &openaicompat.Response{Message: openaicompat.Message{Content: "critical"}},
			}
			triager := severity.NewOpenAICompatibleTriager(severity.OpenAICompatibleTriagerConfig{
				ChatClient: fake,
				Model:      "openai/gpt-oss-120b",
			})

			result, err := triager.TriagePure(context.Background(), defaultInput)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("critical"))
			Expect(result.Confidence).To(Equal(1.0))
			Expect(fake.capturedReq.Model).To(Equal("openai/gpt-oss-120b"))
			Expect(fake.capturedReq.Messages).To(HaveLen(1))
			Expect(fake.capturedReq.Messages[0].Role).To(Equal("user"))
			Expect(fake.capturedReq.Messages[0].Content).To(ContainSubstring("web-api"))
		})
	})

	Describe("UT-AF-1618-003: TriageWithRules", func() {
		It("returns the classified severity from the response", func() {
			fake := &fakeChatClient{
				resp: &openaicompat.Response{Message: openaicompat.Message{Content: "warning"}},
			}
			triager := severity.NewOpenAICompatibleTriager(severity.OpenAICompatibleTriagerConfig{
				ChatClient: fake,
				Model:      "openai/gpt-oss-120b",
			})

			result, err := triager.TriageWithRules(context.Background(), nil, defaultInput)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("warning"))
		})
	})

	Describe("UT-AF-1618-004: Response validation", func() {
		It("returns an error when the chat call fails", func() {
			fake := &fakeChatClient{err: errors.New("connection refused")}
			triager := severity.NewOpenAICompatibleTriager(severity.OpenAICompatibleTriagerConfig{
				ChatClient: fake,
				Model:      "openai/gpt-oss-120b",
			})

			_, err := triager.TriagePure(context.Background(), defaultInput)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("connection refused"))
		})

		It("returns an error when the response is nil", func() {
			fake := &fakeChatClient{resp: nil}
			triager := severity.NewOpenAICompatibleTriager(severity.OpenAICompatibleTriagerConfig{
				ChatClient: fake,
				Model:      "openai/gpt-oss-120b",
			})

			_, err := triager.TriagePure(context.Background(), defaultInput)

			Expect(err).To(HaveOccurred())
		})

		It("returns an error when the response content is empty", func() {
			fake := &fakeChatClient{resp: &openaicompat.Response{Message: openaicompat.Message{Content: ""}}}
			triager := severity.NewOpenAICompatibleTriager(severity.OpenAICompatibleTriagerConfig{
				ChatClient: fake,
				Model:      "openai/gpt-oss-120b",
			})

			_, err := triager.TriagePure(context.Background(), defaultInput)

			Expect(err).To(HaveOccurred())
		})

		It("degrades confidence to 0.5 when the model's text isn't a recognized severity", func() {
			fake := &fakeChatClient{resp: &openaicompat.Response{Message: openaicompat.Message{Content: "not sure, maybe bad?"}}}
			triager := severity.NewOpenAICompatibleTriager(severity.OpenAICompatibleTriagerConfig{
				ChatClient: fake,
				Model:      "openai/gpt-oss-120b",
			})

			result, err := triager.TriagePure(context.Background(), defaultInput)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Severity).To(Equal("warning")) // NormalizeSeverity default
			Expect(result.Confidence).To(Equal(0.5))
		})
	})
})
