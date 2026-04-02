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

package summarizer

import (
	"context"
	"encoding/json"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// SummarizedToolNames lists tool names that get llm_summarize post-processing.
var SummarizedToolNames = map[string]bool{
	"kubectl_describe":               true,
	"kubernetes_jq_query":            true,
	"kubectl_get_by_kind_in_namespace": true,
	"kubectl_get_by_kind_in_cluster":  true,
}

// Wrap decorates a tool with LLM summarization when output exceeds the threshold.
// If the summarizer is nil or the tool is not in the summarized set, the original
// tool is returned unchanged.
func Wrap(t tools.Tool, s *Summarizer) tools.Tool {
	if s == nil || !SummarizedToolNames[t.Name()] {
		return t
	}
	return &summarizingTool{inner: t, summarizer: s}
}

type summarizingTool struct {
	inner      tools.Tool
	summarizer *Summarizer
}

func (t *summarizingTool) Name() string               { return t.inner.Name() }
func (t *summarizingTool) Description() string         { return t.inner.Description() }
func (t *summarizingTool) Parameters() json.RawMessage { return t.inner.Parameters() }

func (t *summarizingTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	result, err := t.inner.Execute(ctx, args)
	if err != nil {
		return result, err
	}
	summarized, sumErr := t.summarizer.MaybeSummarize(ctx, t.inner.Name(), result)
	if sumErr != nil {
		return result, nil
	}
	return summarized, nil
}
