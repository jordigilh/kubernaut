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

package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// AllToolNames lists the 6 baseline Prometheus tool names.
var AllToolNames = []string{
	"execute_prometheus_instant_query",
	"execute_prometheus_range_query",
	"get_metric_names",
	"get_label_values",
	"get_all_labels",
	"get_metric_metadata",
}

// NewAllTools creates the 6 baseline Prometheus tools backed by the given client.
func NewAllTools(client *Client) []tools.Tool {
	return []tools.Tool{
		&promTool{client: client, toolName: "execute_prometheus_instant_query", desc: "Execute a PromQL instant query",
			exec: func(ctx context.Context, c *Client, args json.RawMessage) (string, error) {
				var a struct{ Query string `json:"query"` }
				if err := json.Unmarshal(args, &a); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				return c.doGet(ctx, "/api/v1/query", url.Values{"query": {a.Query}})
			},
		},
		&promTool{client: client, toolName: "execute_prometheus_range_query", desc: "Execute a PromQL range query",
			exec: func(ctx context.Context, c *Client, args json.RawMessage) (string, error) {
				var a struct {
					Query string `json:"query"`
					Start string `json:"start"`
					End   string `json:"end"`
					Step  string `json:"step"`
				}
				if err := json.Unmarshal(args, &a); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				params := url.Values{"query": {a.Query}, "start": {a.Start}, "end": {a.End}, "step": {a.Step}}
				return c.doGet(ctx, "/api/v1/query_range", params)
			},
		},
		&promTool{client: client, toolName: "get_metric_names", desc: "Get available metric names",
			exec: func(ctx context.Context, c *Client, _ json.RawMessage) (string, error) {
				return c.doGet(ctx, "/api/v1/label/__name__/values", nil)
			},
		},
		&promTool{client: client, toolName: "get_label_values", desc: "Get values for a label",
			exec: func(ctx context.Context, c *Client, args json.RawMessage) (string, error) {
				var a struct{ Label string `json:"label"` }
				if err := json.Unmarshal(args, &a); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				return c.doGet(ctx, fmt.Sprintf("/api/v1/label/%s/values", a.Label), nil)
			},
		},
		&promTool{client: client, toolName: "get_all_labels", desc: "Get all label names",
			exec: func(ctx context.Context, c *Client, _ json.RawMessage) (string, error) {
				return c.doGet(ctx, "/api/v1/labels", nil)
			},
		},
		&promTool{client: client, toolName: "get_metric_metadata", desc: "Get metric metadata (help, type)",
			exec: func(ctx context.Context, c *Client, _ json.RawMessage) (string, error) {
				return c.doGet(ctx, "/api/v1/metadata", nil)
			},
		},
	}
}

// promTool wraps a Client + function-pointer to implement the Tool interface.
type promTool struct {
	client   *Client
	toolName string
	desc     string
	exec     func(ctx context.Context, c *Client, args json.RawMessage) (string, error)
}

func (t *promTool) Name() string               { return t.toolName }
func (t *promTool) Description() string         { return t.desc }
func (t *promTool) Parameters() json.RawMessage { return nil }

func (t *promTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.exec(ctx, t.client, args)
}

// TruncateWithHint truncates response text exceeding sizeLimit and appends
// a topk() hint for the LLM to narrow its query.
func TruncateWithHint(text string, sizeLimit int) string {
	if len(text) <= sizeLimit {
		return text
	}
	return text[:sizeLimit] + "\n... [TRUNCATED] Response exceeded limit. Use topk() or add label filters to narrow your query."
}
