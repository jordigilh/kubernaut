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

package mcpclient

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// GatewayDiscoverer abstracts two-phase tool discovery for different MCP Gateway
// implementations. KA calls this itself, server-side, to pre-scope the LLM's
// tool context to the investigation's one target cluster (DD-FLEET-005);
// the LLM never calls ListClusters/ToolsForCluster directly or chooses a
// cluster. ListClusters remains useful as a KA-internal capability and is
// also called programmatically by other services (e.g. KuadrantDiscoverer's
// own ToolsForCluster) and exercised directly by the fleet E2E suite.
//
// Authority: ADR-068 decision #11, BR-FLEET-054, DD-FLEET-005
type GatewayDiscoverer interface {
	// ListClusters returns metadata for all clusters visible through the gateway.
	// The result does not include tool schemas to minimize context usage.
	// An optional category filter narrows results (gateway-dependent semantics).
	ListClusters(ctx context.Context, category string) ([]ClusterInfo, error)

	// ToolsForCluster returns the full tool schemas for a specific cluster.
	// For Kuadrant, this calls select_tools then ListTools to scope the session.
	// For EAIGW, this filters the full tools/list by the cluster's prefix.
	ToolsForCluster(ctx context.Context, clusterID string) ([]ToolDefinition, error)
}

// ClusterInfo holds metadata about a cluster discovered through the gateway.
// Tool names are included for select_tools but full schemas are omitted
// to keep the LLM context lean.
type ClusterInfo struct {
	Name       string   `json:"name"`
	Categories []string `json:"categories,omitempty"`
	Hint       string   `json:"hint,omitempty"`
	Prefix     string   `json:"prefix,omitempty"`
	Tools      []string `json:"tools,omitempty"`
}

// NewDiscoverer creates a GatewayDiscoverer for the given gateway type and session.
// Returns an error for unsupported or empty gateway types, or if session is nil.
//
// Authority: ADR-068 decision #11 (CM-6: Configuration Settings)
func NewDiscoverer(gatewayType registry.MCPGatewayType, session *mcp.ClientSession) (GatewayDiscoverer, error) {
	if session == nil {
		return nil, fmt.Errorf("MCP session must not be nil for gateway discovery (gatewayType=%q)", gatewayType)
	}
	switch gatewayType {
	case registry.GatewayKuadrant:
		return &KuadrantDiscoverer{session: session}, nil
	case registry.GatewayEAIGW:
		return &EAIGWDiscoverer{session: session}, nil
	default:
		return nil, fmt.Errorf("unsupported gateway type %q for tool discovery; must be one of: eaigw, kuadrant", gatewayType)
	}
}

// providerDiscoverer implements GatewayDiscoverer by resolving the live MCP
// session via a SessionProvider on every call and delegating to a freshly
// constructed concrete discoverer (KuadrantDiscoverer/EAIGWDiscoverer),
// instead of binding to a single session snapshot at construction time.
// This is what lets a caller build its GatewayDiscoverer once at startup and
// have it keep working transparently across ResilientClient reconnects --
// including recovering from an initial connect failure once the gateway
// becomes reachable (issue #2315).
type providerDiscoverer struct {
	gatewayType registry.MCPGatewayType
	provider    SessionProvider
	reconnect   func(context.Context) error
}

// Compile-time interface compliance.
var _ GatewayDiscoverer = (*providerDiscoverer)(nil)

// NewDiscovererWithProvider creates a GatewayDiscoverer that resolves the
// live session via ResolveSession on every ListClusters/ToolsForCluster
// call instead of a fixed snapshot, mirroring NewMCPReaderFactoryWithProvider's
// pattern. Unlike NewDiscoverer, this never fails at construction time on a
// nil/disconnected session -- session availability is only checked (and
// reported as a clear per-call error) when a method is actually invoked,
// since the underlying gateway connection may not exist yet at startup and
// is expected to self-heal.
//
// reconnect (typically the owning ResilientClient's Reconnect method) is
// invoked once, with a single retry of the underlying call, when a call
// fails with a retryable session error. Without it, a session that goes bad
// mid-lifetime (issue #2317: EAIGWDiscoverer/KuadrantDiscoverer call
// session.ListTools/CallTool directly on the raw *mcp.ClientSession,
// bypassing Client.callTool's own retry-on-reconnect logic) stays broken
// forever -- SessionProvider() alone only re-reads whatever session
// ResilientClient currently holds, it never repairs it -- exactly mirroring
// the pre-#2315 "permanently nil resolver" failure mode this package was
// built to fix, just one level deeper. Observed live on the Fleet E2E hub as
// repeated "standalone SSE stream: exceeded 5 retries without progress"
// investigation failures reusing the same dead session ID. Pass nil to opt
// out.
func NewDiscovererWithProvider(gatewayType registry.MCPGatewayType, provider SessionProvider, reconnect func(context.Context) error) GatewayDiscoverer {
	return &providerDiscoverer{gatewayType: gatewayType, provider: provider, reconnect: reconnect}
}

func (d *providerDiscoverer) resolve() (GatewayDiscoverer, error) {
	session, err := ResolveSession(nil, d.provider)
	if err != nil {
		return nil, fmt.Errorf("resolve gateway discoverer session: %w", err)
	}
	return NewDiscoverer(d.gatewayType, session)
}

// retryWithReconnect runs call() against a freshly resolved discoverer, and
// on a retryable session error invokes d.reconnect once and retries call() a
// single time against the now-refreshed session. Shared by
// ListClusters/ToolsForCluster so both follow identical self-healing
// semantics (mirrors ResilientClient.Get/List and Client.callTool's own
// retry-once-after-reconnect pattern).
func retryWithReconnect[T any](ctx context.Context, d *providerDiscoverer, call func(GatewayDiscoverer) (T, error)) (T, error) {
	disc, err := d.resolve()
	if err != nil {
		var zero T
		return zero, err
	}
	result, err := call(disc)
	if err == nil || d.reconnect == nil || !isRetryableSessionError(err) {
		return result, err
	}

	if reconnErr := d.reconnect(ctx); reconnErr != nil {
		var zero T
		return zero, fmt.Errorf("reconnect failed: %w (original: %w)", reconnErr, err)
	}
	disc, resolveErr := d.resolve()
	if resolveErr != nil {
		var zero T
		return zero, resolveErr
	}
	return call(disc)
}

func (d *providerDiscoverer) ListClusters(ctx context.Context, category string) ([]ClusterInfo, error) {
	return retryWithReconnect(ctx, d, func(disc GatewayDiscoverer) ([]ClusterInfo, error) {
		return disc.ListClusters(ctx, category)
	})
}

func (d *providerDiscoverer) ToolsForCluster(ctx context.Context, clusterID string) ([]ToolDefinition, error) {
	return retryWithReconnect(ctx, d, func(disc GatewayDiscoverer) ([]ToolDefinition, error) {
		return disc.ToolsForCluster(ctx, clusterID)
	})
}
