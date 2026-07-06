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
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/anthropicfamily"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
)

// TP-783: Close() behavior across every llm.Client implementation. The
// langchaingo.Adapter-specific sub-tests that lived here (UT-KA-783-CL-001/
// 003/004) were retired along with the langchaingo package itself (#1581
// completion of #1578's deprecation); anthropicfamily.Client and
// kaopenai.Client — both native, non-langchaingo implementations — replace
// its Close() coverage below.
var _ = Describe("llm.Client Close() — TP-783 (#783)", func() {

	Describe("UT-KA-783-CL-006: anthropicfamily.Client.Close is a no-op", func() {
		It("should return nil for a client constructed via the native API-key auth mode", func() {
			client, err := anthropicfamily.NewWithAPIKey("sk-ant-fake-key", "claude-sonnet-4-6")
			Expect(err).NotTo(HaveOccurred())
			Expect(client.Close()).To(Succeed())
		})
	})

	Describe("UT-KA-783-CL-007: kaopenai.Client.Close is a no-op", func() {
		It("should return nil for the shared-core-backed OpenAI-compatible wrapper", func() {
			client := kaopenai.New("gpt-4o", "https://example.invalid", "fake-key")
			Expect(client.Close()).To(Succeed())
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
