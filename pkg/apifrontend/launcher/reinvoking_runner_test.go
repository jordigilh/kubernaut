package launcher_test

import (
	"context"
	"iter"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/adk/agent"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// scriptedRunnerCall captures the arguments of one Run() invocation against
// scriptedRunner, so tests can assert exactly what reinvokingRunner passed
// through on each turn (in particular, the synthetic continuation message).
type scriptedRunnerCall struct {
	userID    string
	sessionID string
	msg       *genai.Content
}

// scriptedRunner is a fake underlying Runner (the seam reinvokingRunner
// wraps). Each call to Run() appends the next scripted response event to the
// shared session (mirroring how the real *runner.Runner commits non-partial
// events) and yields it, then returns. It has no reinvocation awareness of
// its own — that decision belongs entirely to reinvokingRunner.
type scriptedRunner struct {
	sessionSvc adksession.Service
	responses  []*adksession.Event
	calls      []scriptedRunnerCall
}

func (r *scriptedRunner) Run(ctx context.Context, userID, sessionID string, msg *genai.Content, _ agent.RunConfig) iter.Seq2[*adksession.Event, error] {
	callIdx := len(r.calls)
	r.calls = append(r.calls, scriptedRunnerCall{userID: userID, sessionID: sessionID, msg: msg})
	return func(yield func(*adksession.Event, error) bool) {
		if callIdx >= len(r.responses) {
			return
		}
		resp, err := r.sessionSvc.Get(ctx, &adksession.GetRequest{AppName: "test-app", UserID: userID, SessionID: sessionID})
		if err != nil {
			yield(nil, err)
			return
		}
		event := r.responses[callIdx]
		if appendErr := r.sessionSvc.AppendEvent(ctx, resp.Session, event); appendErr != nil {
			yield(nil, appendErr)
			return
		}
		yield(event, nil)
	}
}

func textOnlyModelEvent(invocationID, text string) *adksession.Event {
	event := adksession.NewEvent(invocationID)
	event.Author = "model"
	event.Content = genai.NewContentFromText(text, genai.RoleModel)
	return event
}

func toolCallModelEvent(invocationID string) *adksession.Event {
	event := adksession.NewEvent(invocationID)
	event.Author = "model"
	event.Content = &genai.Content{
		Role: "model",
		Parts: []*genai.Part{
			{FunctionCall: &genai.FunctionCall{Name: "kubernaut_investigate", Args: map[string]any{}}},
		},
	}
	return event
}

var _ = Describe("reinvokingRunner (BR-SESS-013, issue #1776)", func() {
	It("UT-AF-REINV-002: Run() re-invokes when last event has no tool call (session Active, count < Max)", func() {
		sessionSvc := adksession.InMemoryService()
		_, err := sessionSvc.Create(context.Background(), &adksession.CreateRequest{
			AppName: "test-app", UserID: "user-1", SessionID: "sess-1",
		})
		Expect(err).NotTo(HaveOccurred())

		fake := &scriptedRunner{
			sessionSvc: sessionSvc,
			responses: []*adksession.Event{
				textOnlyModelEvent("inv-1", "I need more information."),
				toolCallModelEvent("inv-2"),
			},
		}
		rr := launcher.NewReinvokingRunnerForTest(fake, sessionSvc, "test-app", logr.Discard())

		var events []*adksession.Event
		for event, runErr := range rr.Run(context.Background(), "user-1", "sess-1", genai.NewContentFromText("investigate", genai.RoleUser), agent.RunConfig{}) {
			Expect(runErr).NotTo(HaveOccurred())
			events = append(events, event)
		}

		Expect(fake.calls).To(HaveLen(2),
			"Run() must re-invoke the inner runner exactly once when the last event has no tool call")
		Expect(fake.calls[1].msg).To(Equal(session.SyntheticMessage()),
			"the reinvocation call must pass the synthetic continuation message, not the original message")
		Expect(events).To(HaveLen(2), "both the original and reinvoked turn's events must be yielded")
	})

	It("UT-AF-REINV-003: Run() does NOT re-invoke when last event has a tool call", func() {
		sessionSvc := adksession.InMemoryService()
		_, err := sessionSvc.Create(context.Background(), &adksession.CreateRequest{
			AppName: "test-app", UserID: "user-1", SessionID: "sess-2",
		})
		Expect(err).NotTo(HaveOccurred())

		fake := &scriptedRunner{
			sessionSvc: sessionSvc,
			responses:  []*adksession.Event{toolCallModelEvent("inv-1")},
		}
		rr := launcher.NewReinvokingRunnerForTest(fake, sessionSvc, "test-app", logr.Discard())

		var events []*adksession.Event
		for event, runErr := range rr.Run(context.Background(), "user-1", "sess-2", genai.NewContentFromText("investigate", genai.RoleUser), agent.RunConfig{}) {
			Expect(runErr).NotTo(HaveOccurred())
			events = append(events, event)
		}

		Expect(fake.calls).To(HaveLen(1),
			"Run() must NOT re-invoke the inner runner when the last event already contains a tool call")
		Expect(events).To(HaveLen(1))
	})
})
