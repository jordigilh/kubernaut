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

package alertmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// Tool is an alias for the shared tool interface.
type Tool = tools.Tool

// AllToolNames lists the Alertmanager tool names for phase mapping.
var AllToolNames = []string{
	"get_alerts",
	"get_silences",
}

var (
	getAlertsSchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"active":    {"type": "boolean", "description": "Filter by active alerts (default: true)"},
			"silenced":  {"type": "boolean", "description": "Filter by silenced alerts (default: true)"},
			"inhibited": {"type": "boolean", "description": "Filter by inhibited alerts (default: true)"},
			"receiver":  {"type": "string", "description": "Filter by receiver name"},
			"filter":    {"type": "array", "items": {"type": "string"}, "description": "Label matchers (e.g. alertname=~KubePod.*)"}
		},
		"additionalProperties": false
	}`)
	getSilencesSchema = json.RawMessage(`{
		"type": "object",
		"properties": {
			"filter": {"type": "array", "items": {"type": "string"}, "description": "Label matchers to filter silences (e.g. alertname=KubePodCrashLooping)"}
		},
		"additionalProperties": false
	}`)
)

// NewAllTools creates the Alertmanager tools backed by the given client.
func NewAllTools(client *Client) []Tool {
	return []Tool{
		&amTool{
			client:   client,
			toolName: "get_alerts",
			desc:     "Query Alertmanager for active, silenced, or inhibited alerts. Returns raw JSON alert list.",
			schema:   getAlertsSchema,
			exec:     executeGetAlerts,
		},
		&amTool{
			client:   client,
			toolName: "get_silences",
			desc:     "Query Alertmanager for configured silences. Returns raw JSON silence list.",
			schema:   getSilencesSchema,
			exec:     executeGetSilences,
		},
	}
}

type amTool struct {
	client   *Client
	toolName string
	desc     string
	schema   json.RawMessage
	exec     func(ctx context.Context, c *Client, args json.RawMessage) (string, error)
}

func (t *amTool) Name() string               { return t.toolName }
func (t *amTool) Description() string         { return t.desc }
func (t *amTool) Parameters() json.RawMessage { return t.schema }

func (t *amTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.exec(ctx, t.client, args)
}

func executeGetAlerts(ctx context.Context, c *Client, args json.RawMessage) (string, error) {
	var a struct {
		Active   *bool    `json:"active"`
		Silenced *bool    `json:"silenced"`
		Inhibited *bool   `json:"inhibited"`
		Receiver string   `json:"receiver"`
		Filter   []string `json:"filter"`
	}

	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("parsing args: %w", err)
		}
	}

	params := make(map[string][]string)
	if a.Active != nil {
		params["active"] = []string{strconv.FormatBool(*a.Active)}
	}
	if a.Silenced != nil {
		params["silenced"] = []string{strconv.FormatBool(*a.Silenced)}
	}
	if a.Inhibited != nil {
		params["inhibited"] = []string{strconv.FormatBool(*a.Inhibited)}
	}
	if a.Receiver != "" {
		params["receiver"] = []string{a.Receiver}
	}
	if len(a.Filter) > 0 {
		params["filter"] = append(params["filter"], a.Filter...)
	}

	if len(params) == 0 {
		params = nil
	}

	return c.DoGet(ctx, "/api/v2/alerts", params)
}

func executeGetSilences(ctx context.Context, c *Client, args json.RawMessage) (string, error) {
	var a struct {
		Filter []string `json:"filter"`
	}

	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("parsing args: %w", err)
		}
	}

	var params map[string][]string
	if len(a.Filter) > 0 {
		params = map[string][]string{
			"filter": a.Filter,
		}
	}

	return c.DoGet(ctx, "/api/v2/silences", params)
}
