package launcher

import (
	"context"
	"errors"
	"iter"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/model"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
)

// hangingLLM is a model.LLM fake whose GenerateContent blocks until the
// ctx it receives ends, then yields the resulting error — used to prove
// wrapWithTimeout's decorator actually bounds the call (#1955), not just
// that it resolves the right duration.
type hangingLLM struct {
	name string
}

func (m *hangingLLM) Name() string { return m.name }

func (m *hangingLLM) GenerateContent(ctx context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		<-ctx.Done()
		yield(nil, ctx.Err())
	}
}

// awaitGenerateContent ranges over seq in a goroutine and fails the spec
// fast if it doesn't terminate within bound, instead of letting a
// regression (no timeout enforced) hang the whole suite.
func awaitGenerateContent(bound time.Duration, seq iter.Seq2[*model.LLMResponse, error]) error {
	done := make(chan error, 1)
	go func() {
		var lastErr error
		for _, err := range seq {
			lastErr = err
		}
		done <- lastErr
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(bound):
		Fail("GenerateContent did not return within the safety bound — #1955 timeout not enforced")
		return nil
	}
}

var _ = Describe("timeoutModel decorator (#1955, BR-AI-1955)", func() {
	It("UT-AF-1955-001: resolves and enforces the DefaultLLMTimeoutSeconds fallback when cfg.TimeoutSeconds is unset", func() {
		inner := &hangingLLM{name: "claude-test"}
		wrapped := wrapWithTimeout(inner, config.LLMConfig{})

		tm, ok := wrapped.(*timeoutModel)
		Expect(ok).To(BeTrue())
		Expect(tm.timeout).To(Equal(time.Duration(config.DefaultLLMTimeoutSeconds) * time.Second))

		// Shrink the resolved timeout directly (white-box, same package) so
		// this spec doesn't have to wait out the real 120s default in order
		// to prove enforcement — the resolution branch above already
		// confirmed 120s is what production code would have picked.
		tm.timeout = 10 * time.Millisecond

		err := awaitGenerateContent(2*time.Second, wrapped.GenerateContent(context.Background(), &model.LLMRequest{}, false))
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
	})

	It("UT-AF-1955-002: resolves and enforces an explicit cfg.TimeoutSeconds instead of the default", func() {
		inner := &hangingLLM{name: "claude-test"}
		wrapped := wrapWithTimeout(inner, config.LLMConfig{TimeoutSeconds: 45})

		tm, ok := wrapped.(*timeoutModel)
		Expect(ok).To(BeTrue())
		Expect(tm.timeout).To(Equal(45 * time.Second))

		tm.timeout = 10 * time.Millisecond

		err := awaitGenerateContent(2*time.Second, wrapped.GenerateContent(context.Background(), &model.LLMRequest{}, false))
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
	})

	It("streaming calls (stream=true) are bounded the same way as non-streaming", func() {
		inner := &hangingLLM{name: "claude-test"}
		wrapped := &timeoutModel{inner: inner, timeout: 10 * time.Millisecond}

		err := awaitGenerateContent(2*time.Second, wrapped.GenerateContent(context.Background(), &model.LLMRequest{}, true))
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, context.DeadlineExceeded)).To(BeTrue())
	})

	It("Name() passes through to the wrapped inner model", func() {
		inner := &hangingLLM{name: "claude-passthrough"}
		wrapped := wrapWithTimeout(inner, config.LLMConfig{})
		Expect(wrapped.Name()).To(Equal("claude-passthrough"))
	})
})
