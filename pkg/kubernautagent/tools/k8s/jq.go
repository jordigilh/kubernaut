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

package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

var (
	jqQueryParams = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","description":"Kubernetes resource kind to query"},"jq_expr":{"type":"string","description":"jq expression to apply to the resource list"}},"required":["kind","jq_expr"]}`)
	jqCountParams = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string","description":"Kubernetes resource kind to count"},"jq_expr":{"type":"string","description":"jq expression to filter resources before counting"}},"required":["kind","jq_expr"]}`)
)

func newJQTools(resolver ResourceResolver) []tools.Tool {
	return []tools.Tool{
		&jqQueryTool{resolver: resolver},
		&jqCountTool{resolver: resolver},
	}
}

// jqQueryTool applies an arbitrary jq expression to K8s resource listings.
type jqQueryTool struct {
	resolver ResourceResolver
}

func (t *jqQueryTool) Name() string               { return "kubernetes_jq_query" }
func (t *jqQueryTool) Description() string         { return "Run a jq expression against Kubernetes resources of a given kind across all namespaces" }
func (t *jqQueryTool) Parameters() json.RawMessage { return jqQueryParams }

func (t *jqQueryTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Kind   string `json:"kind"`
		JQExpr string `json:"jq_expr"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	return runJQ(ctx, t.resolver, a.Kind, a.JQExpr, false)
}

// jqCountTool counts resources matching a jq filter expression.
type jqCountTool struct {
	resolver ResourceResolver
}

func (t *jqCountTool) Name() string               { return "kubernetes_count" }
func (t *jqCountTool) Description() string         { return "Count Kubernetes resources matching a jq filter, showing count and 10-line preview" }
func (t *jqCountTool) Parameters() json.RawMessage { return jqCountParams }

func (t *jqCountTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Kind   string `json:"kind"`
		JQExpr string `json:"jq_expr"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("parsing args: %w", err)
	}

	return runJQ(ctx, t.resolver, a.Kind, a.JQExpr, true)
}

const maxJQResults = 10000
const maxJQOutputChars = 100000

// TruncateJQOutput truncates JQ output exceeding the given character limit,
// appending a hint to guide the LLM toward more specific queries.
func TruncateJQOutput(output string, limit int) string {
	if len(output) <= limit {
		return output
	}
	return output[:limit] + fmt.Sprintf("\n... [TRUNCATED] JQ output exceeded %d character limit (%d total). Use a more specific jq expression or filter with select().", limit, len(output))
}

func runJQ(ctx context.Context, resolver ResourceResolver, kind, expr string, countMode bool) (string, error) {
	resources, err := resolver.List(ctx, kind, "")
	if err != nil {
		return "", err
	}

	jsonData, err := json.Marshal(resources)
	if err != nil {
		return "", fmt.Errorf("marshaling resources: %w", err)
	}

	var input interface{}
	if err := json.Unmarshal(jsonData, &input); err != nil {
		return "", fmt.Errorf("unmarshaling JSON for jq: %w", err)
	}

	query, err := gojq.Parse(expr)
	if err != nil {
		return "", fmt.Errorf("invalid jq expression %q: %w", expr, err)
	}

	var results []string
	iter := query.Run(input)
	for {
		if ctx.Err() != nil {
			return "", fmt.Errorf("jq execution cancelled: %w", ctx.Err())
		}
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return "", fmt.Errorf("jq execution error: %w", err)
		}
		b, _ := json.Marshal(v)
		results = append(results, string(b))
		if len(results) >= maxJQResults {
			break
		}
	}

	if countMode {
		count := len(results)
		preview := results
		if len(preview) > 10 {
			preview = preview[:10]
		}
		suffix := ""
		if count >= maxJQResults {
			suffix = fmt.Sprintf(" (capped at %d)", maxJQResults)
		}
		return fmt.Sprintf("Count: %d%s\n%s", count, suffix, strings.Join(preview, "\n")), nil
	}

	joined := strings.Join(results, "\n")
	return TruncateJQOutput(joined, maxJQOutputChars), nil
}
