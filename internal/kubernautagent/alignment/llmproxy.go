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

package alignment

import (
	"context"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
)

// LLMProxy wraps an llm.Client and feeds each response to the shadow agent.
// The proxy is safe for concurrent use across investigations because the
// actual Observer is retrieved per-call from the context.
type LLMProxy struct {
	inner llm.Client
}

var _ llm.Client = (*LLMProxy)(nil)

// NewLLMProxy creates an LLMProxy that decorates the given client.
func NewLLMProxy(inner llm.Client) *LLMProxy {
	return &LLMProxy{inner: inner}
}

// Chat delegates to the inner client, then submits the response content
// for alignment evaluation via the context-scoped Observer.
func (p *LLMProxy) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	resp, err := p.inner.Chat(ctx, req)
	if err != nil {
		return resp, err
	}

	if obs := ObserverFromContext(ctx); obs != nil && resp.Message.Content != "" {
		step := Step{
			Index:   obs.NextStepIndex(),
			Kind:    StepKindLLMReasoning,
			Content: resp.Message.Content,
		}
		obs.SubmitAsync(ctx, step)
	}

	return resp, nil
}

// Close releases resources held by the inner client.
func (p *LLMProxy) Close() error {
	if p == nil || p.inner == nil {
		return nil
	}
	return p.inner.Close()
}
