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

package readiness

import (
	"context"
	"fmt"
)

// MCPClient is the subset of mcpclient.ResilientClient needed by
// MCPClientProber. Defined locally (not imported) to avoid a readiness ->
// mcpclient dependency edge; *mcpclient.ResilientClient satisfies this
// structurally.
type MCPClient interface {
	Ready() bool
	Reconnect(ctx context.Context) error
}

// MCPClientProber probes an MCP Gateway connection. When the client
// reports not-ready, Probe attempts a reconnect so the connection
// self-heals as soon as the Gateway becomes reachable again, instead of
// waiting for the next lazy reconnect-on-error inside Get/List.
type MCPClientProber struct {
	Client MCPClient
}

var _ Prober = (*MCPClientProber)(nil)

// Probe implements Prober.
func (p *MCPClientProber) Probe(ctx context.Context) error {
	if p.Client.Ready() {
		return nil
	}
	if err := p.Client.Reconnect(ctx); err != nil {
		return fmt.Errorf("MCP Gateway unreachable: %w", err)
	}
	return nil
}

// ClusterRegistry is the subset of registry.ClusterQuerier needed by
// ClusterRegistryProber. Defined locally to avoid a readiness -> registry
// dependency edge; registry.ClusterRegistry implementations satisfy this
// structurally.
type ClusterRegistry interface {
	Ready() bool
}

// ClusterRegistryProber probes a Fleet cluster registry's watch health.
// Note: today's ClusterRegistry implementations report Ready() as a
// one-shot flag that becomes true after the initial informer sync and
// never resets on subsequent watch failures (tracked separately); this
// Prober faithfully reflects whatever the registry reports.
type ClusterRegistryProber struct {
	Registry ClusterRegistry
}

var _ Prober = (*ClusterRegistryProber)(nil)

// Probe implements Prober.
func (p *ClusterRegistryProber) Probe(_ context.Context) error {
	if !p.Registry.Ready() {
		return fmt.Errorf("fleet cluster registry not ready")
	}
	return nil
}

// ScopeCheckerProber probes a federated scope-checker backend (FMC or ACM)
// via its Pinger.
type ScopeCheckerProber struct {
	Pinger Pinger
}

var _ Prober = (*ScopeCheckerProber)(nil)

// Probe implements Prober.
func (p *ScopeCheckerProber) Probe(ctx context.Context) error {
	if err := p.Pinger.Ping(ctx); err != nil {
		return fmt.Errorf("scope-checker backend unreachable: %w", err)
	}
	return nil
}
