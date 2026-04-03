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
	"strconv"
	"time"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// AllToolNames lists the 8 Prometheus tool names matching HAPI parity.
var AllToolNames = []string{
	"execute_prometheus_instant_query",
	"execute_prometheus_range_query",
	"get_metric_names",
	"get_label_values",
	"get_all_labels",
	"get_metric_metadata",
	"list_prometheus_rules",
	"get_series",
}

var (
	instantQuerySchema = json.RawMessage(`{
		"type": "object",
		"properties": {"query": {"type": "string", "description": "PromQL instant query expression"}},
		"required": ["query"]
	}`)
	rangeQuerySchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "PromQL range query expression"},
			"start": {"type": "string", "description": "Range start time (RFC3339 or relative)"},
			"end":   {"type": "string", "description": "Range end time (RFC3339 or relative)"},
			"step":  {"type": "string", "description": "Query resolution step (e.g. 15s, 1m)"}
		},
		"required": ["query", "start", "end", "step"]
	}`)
	metricNamesSchema = json.RawMessage(`{
		"type": "object",
		"properties": {},
		"additionalProperties": false
	}`)
	labelValuesSchema = json.RawMessage(`{
		"type": "object",
		"properties": {"label": {"type": "string", "description": "Label name to get values for"}},
		"required": ["label"]
	}`)
	allLabelsSchema = json.RawMessage(`{
		"type": "object",
		"properties": {},
		"additionalProperties": false
	}`)
	metricMetadataSchema = json.RawMessage(`{
		"type": "object",
		"properties": {},
		"additionalProperties": false
	}`)
	rulesSchema = json.RawMessage(`{
		"type": "object",
		"properties": {},
		"additionalProperties": false
	}`)
	seriesSchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"match": {"type": "string", "description": "PromQL series selector (e.g. up, {job=\"prometheus\"})"},
			"start": {"type": "string", "description": "Start timestamp (RFC3339 or Unix). Default: 1 hour ago"},
			"end":   {"type": "string", "description": "End timestamp (RFC3339 or Unix). Default: now"}
		},
		"required": ["match"]
	}`)
)

// NewAllTools creates the 8 Prometheus tools backed by the given client.
func NewAllTools(client *Client) []tools.Tool {
	return []tools.Tool{
		&promTool{client: client, toolName: "execute_prometheus_instant_query", desc: "Execute a PromQL instant query", schema: instantQuerySchema,
			exec: func(ctx context.Context, c *Client, args json.RawMessage) (string, error) {
				var a struct{ Query string `json:"query"` }
				if err := json.Unmarshal(args, &a); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				return c.doGet(ctx, "/api/v1/query", url.Values{"query": {a.Query}})
			},
		},
		&promTool{client: client, toolName: "execute_prometheus_range_query", desc: "Execute a PromQL range query", schema: rangeQuerySchema,
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
		&promTool{client: client, toolName: "get_metric_names", desc: "Get available metric names", schema: metricNamesSchema,
			exec: func(ctx context.Context, c *Client, _ json.RawMessage) (string, error) {
				return c.doGet(ctx, "/api/v1/label/__name__/values", nil)
			},
		},
		&promTool{client: client, toolName: "get_label_values", desc: "Get values for a label", schema: labelValuesSchema,
			exec: func(ctx context.Context, c *Client, args json.RawMessage) (string, error) {
				var a struct{ Label string `json:"label"` }
				if err := json.Unmarshal(args, &a); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				return c.doGet(ctx, fmt.Sprintf("/api/v1/label/%s/values", a.Label), nil)
			},
		},
		&promTool{client: client, toolName: "get_all_labels", desc: "Get all label names", schema: allLabelsSchema,
			exec: func(ctx context.Context, c *Client, _ json.RawMessage) (string, error) {
				return c.doGet(ctx, "/api/v1/labels", nil)
			},
		},
		&promTool{client: client, toolName: "get_metric_metadata", desc: "Get metric metadata (help, type)", schema: metricMetadataSchema,
			exec: func(ctx context.Context, c *Client, _ json.RawMessage) (string, error) {
				return c.doGet(ctx, "/api/v1/metadata", nil)
			},
		},
		&promTool{client: client, toolName: "list_prometheus_rules", desc: "List Prometheus alerting and recording rules", schema: rulesSchema,
			exec: func(ctx context.Context, c *Client, _ json.RawMessage) (string, error) {
				return c.doGet(ctx, "/api/v1/rules", nil)
			},
		},
		&promTool{client: client, toolName: "get_series",
			desc: "Get time series label sets matching a selector via /api/v1/series. " +
				"SLOWER than other discovery methods — use only when you need full label sets. " +
				fmt.Sprintf("Returns up to %d series. If %d results returned, more may exist — use a more specific selector. ",
					client.config.MetadataLimit, client.config.MetadataLimit) +
				"By default returns series active in the last 1 hour.",
			schema: seriesSchema,
			exec: func(ctx context.Context, c *Client, args json.RawMessage) (string, error) {
				var a struct {
					Match string `json:"match"`
					Start string `json:"start"`
					End   string `json:"end"`
				}
				if err := json.Unmarshal(args, &a); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				if a.Match == "" {
					return "", fmt.Errorf("match parameter is required")
				}

				params := url.Values{
					"match[]": {a.Match},
					"limit":   {strconv.Itoa(c.config.MetadataLimit)},
				}

				now := time.Now().Unix()
				if a.End != "" {
					params.Set("end", a.End)
				} else {
					params.Set("end", strconv.FormatInt(now, 10))
				}
				if a.Start != "" {
					params.Set("start", a.Start)
				} else {
					params.Set("start", strconv.FormatInt(now-int64(c.config.MetadataTimeWindowHrs)*3600, 10))
				}

				result, err := c.doGet(ctx, "/api/v1/series", params)
				if err != nil {
					return result, err
				}
				return applyMetadataTruncationHint(result, c.config.MetadataLimit), nil
			},
		},
	}
}

// promTool wraps a Client + function-pointer to implement the Tool interface.
type promTool struct {
	client   *Client
	toolName string
	desc     string
	schema   json.RawMessage
	exec     func(ctx context.Context, c *Client, args json.RawMessage) (string, error)
}

func (t *promTool) Name() string               { return t.toolName }
func (t *promTool) Description() string         { return t.desc }
func (t *promTool) Parameters() json.RawMessage { return t.schema }

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

// applyMetadataTruncationHint detects when a Prometheus metadata API response
// contains exactly `limit` items in its "data" array, indicating the result set
// was capped. It injects "_truncated" and "_message" fields to guide the LLM
// toward using a more specific selector.
func applyMetadataTruncationHint(body string, limit int) string {
	var resp map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return body
	}
	dataRaw, ok := resp["data"]
	if !ok {
		return body
	}
	var items []json.RawMessage
	if err := json.Unmarshal(dataRaw, &items); err != nil {
		return body
	}
	if len(items) != limit {
		return body
	}

	resp["_truncated"] = json.RawMessage(`true`)
	resp["_message"] = json.RawMessage(
		fmt.Sprintf(`"Results truncated at limit=%d. Use a more specific match selector to see additional series."`, limit),
	)
	out, err := json.Marshal(resp)
	if err != nil {
		return body
	}
	return string(out)
}
