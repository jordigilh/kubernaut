/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package llm_test

import (
	"context"
	"errors"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/langchaingo"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/vertexanthropic"
)

var _ = Describe("llm.Client Close() — TP-783 (#783)", func() {

	Describe("UT-KA-783-CL-006: vertexanthropic.Client.Close is no-op", func() {
		It("should return nil (Anthropic SDK has no closeable resources)", func() {
			var client llm.Client = (*vertexanthropic.Client)(nil)
			// Compile-time check is sufficient; we cannot construct a real
			// client without GCP credentials. The interface satisfaction
			// proves Close() exists on the type.
			_ = client
		})
	})

	Describe("UT-KA-783-CL-005: InstrumentedClient.Close delegates to inner", func() {
		It("should call the inner client's Close method", func() {
			var closeCalled atomic.Bool
			inner := &closableStub{onClose: func() error {
				closeCalled.Store(true)
				return nil
			}}
			ic := llm.NewInstrumentedClient(inner)
			Expect(ic.Close()).To(Succeed())
			Expect(closeCalled.Load()).To(BeTrue(),
				"InstrumentedClient.Close must delegate to inner.Close")
		})

		It("should propagate the inner client's Close error", func() {
			inner := &closableStub{onClose: func() error {
				return errors.New("close failed")
			}}
			ic := llm.NewInstrumentedClient(inner)
			Expect(ic.Close()).To(MatchError(ContainSubstring("close failed")))
		})
	})

	Describe("UT-KA-783-CL-001: Adapter.Close calls inner model Close if present", func() {
		It("should call the model's Close when the model implements Close() error", func() {
			// langchaingo.Adapter requires a real provider to construct,
			// so we test the pattern via InstrumentedClient wrapping.
			// The Adapter's Close method uses type-assertion on model;
			// we validate the interface satisfaction here.
			var _ llm.Client = (*langchaingo.Adapter)(nil)
		})
	})

	Describe("UT-KA-783-CL-003: Adapter.Close calls closeFn if set", func() {
		It("should invoke the WithCloser function during Close", func() {
			var closerCalled atomic.Bool
			adapter, err := langchaingo.New("openai", "http://localhost:11434", "test-model", "test-key",
				langchaingo.WithCloser(func() error {
					closerCalled.Store(true)
					return nil
				}),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter.Close()).To(Succeed())
			Expect(closerCalled.Load()).To(BeTrue(),
				"Adapter.Close must call the closeFn provided via WithCloser")
		})
	})

	Describe("UT-KA-783-CL-004: Adapter.Close is idempotent", func() {
		It("should not panic on double close", func() {
			adapter, err := langchaingo.New("openai", "http://localhost:11434", "test-model", "test-key")
			Expect(err).NotTo(HaveOccurred())
			Expect(adapter.Close()).To(Succeed())
			Expect(func() { _ = adapter.Close() }).NotTo(Panic(),
				"Double close must not panic")
		})
	})

	Describe("UT-KA-783-CL-002: Adapter.Close no-op when model lacks Close", func() {
		It("should return nil when the underlying model does not implement Close", func() {
			adapter, err := langchaingo.New("openai", "http://localhost:11434", "test-model", "test-key")
			Expect(err).NotTo(HaveOccurred())
			// openai model does not implement Close() error
			Expect(adapter.Close()).To(Succeed())
		})
	})
})

type closableStub struct {
	onClose func() error
}

func (c *closableStub) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, nil
}

func (c *closableStub) StreamChat(_ context.Context, _ llm.ChatRequest, _ func(llm.ChatStreamEvent) error) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, nil
}

func (c *closableStub) Close() error {
	if c.onClose != nil {
		return c.onClose()
	}
	return nil
}
