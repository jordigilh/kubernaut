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
	"log/slog"
)

// StubProvider is the v1.3 MCP tool provider that logs a warning and
// returns an empty tool list. Config parsing and registry wiring are
// tested; real SSE/stdio transport deferred to v1.5.
type StubProvider struct {
	logger *slog.Logger
}

// NewStubProvider creates a stub MCP provider.
func NewStubProvider(logger *slog.Logger) *StubProvider {
	return &StubProvider{logger: logger}
}

// DiscoverTools returns an empty tool list and logs a warning.
func (p *StubProvider) DiscoverTools(_ context.Context) ([]Tool, error) {
	p.logger.Warn("MCP tool discovery is stubbed in v1.3; returning empty tool list")
	return []Tool{}, nil
}

// Close is a no-op for the stub provider.
func (p *StubProvider) Close() error {
	return nil
}
