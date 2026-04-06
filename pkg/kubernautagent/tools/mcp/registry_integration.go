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

package mcp

import (
	"context"
	"fmt"
)

// RegisterToolsFromProviders iterates configured MCP providers, discovers
// tools, and returns the total count of tools discovered. In v1.3 this
// validates the wiring; tool registration into the agent's registry will
// be completed when the Registry type is introduced in Phase 3.
func RegisterToolsFromProviders(ctx context.Context, providers []MCPToolProvider) (int, error) {
	total := 0
	for _, p := range providers {
		tools, err := p.DiscoverTools(ctx)
		if err != nil {
			return total, fmt.Errorf("discovering MCP tools: %w", err)
		}
		total += len(tools)
	}
	return total, nil
}
