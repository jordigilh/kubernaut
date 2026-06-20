//go:build ignore

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
	"encoding/json"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// MockProvider is the interface expected by DiscoverAndBridgeFromMock for testing.
type MockProvider interface {
	GetConfig() ServerConfig
	DiscoverTools(ctx context.Context) ([]Tool, error)
	NewSession(ctx context.Context) (Session, error)
}

// DiscoverAndBridgeFromMock is a test-friendly version of DiscoverAndBridge
// that accepts MockProvider instead of concrete *StreamableProvider.
func DiscoverAndBridgeFromMock(ctx context.Context, providers []MockProvider) ([]tools.Tool, error) {
	var bridged []tools.Tool
	seen := make(map[string]bool)
	logger := logr.Discard()

	for _, p := range providers {
		discovered, err := p.DiscoverTools(ctx)
		if err != nil {
			return nil, err
		}

		cfg := p.GetConfig()
		for _, t := range discovered {
			if seen[t.ToolName] {
				logger.Info("skipping duplicate tool", "tool", t.ToolName, "server", cfg.Name)
				continue
			}
			seen[t.ToolName] = true
			bridged = append(bridged, NewBridgeTool(t, cfg.Name, p))
		}
	}

	return bridged, nil
}

// MarshalInputSchemaPublic exposes marshalInputSchema for testing.
func MarshalInputSchemaPublic(schema any) json.RawMessage {
	result, _ := marshalInputSchema(schema)
	return result
}
