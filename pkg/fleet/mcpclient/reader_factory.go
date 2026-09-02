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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

type mcpReaderFactory struct {
	localClient     client.Reader
	session         *mcp.ClientSession
	sessionProvider SessionProvider
	reconnect       func(context.Context) error
	prefixResolver  registry.ToolPrefixResolver
}

// NewMCPReaderFactory creates a fleet.ReaderFactory that returns local clients for
// empty ClusterID and MCP-backed readers for remote clusters.
// An optional ToolPrefixResolver enables gateway-specific tool prefix lookup;
// when nil, the EAIGW "{clusterID}__" convention is used.
func NewMCPReaderFactory(localClient client.Reader, session *mcp.ClientSession, resolver ...registry.ToolPrefixResolver) fleet.ReaderFactory {
	var pr registry.ToolPrefixResolver
	if len(resolver) > 0 {
		pr = resolver[0]
	}
	return &mcpReaderFactory{
		localClient:    localClient,
		session:        session,
		prefixResolver: pr,
	}
}

// NewMCPReaderFactoryWithProvider is like NewMCPReaderFactory but uses a
// SessionProvider for lazy session resolution. Per-cluster readers created by
// this factory automatically follow ResilientClient reconnections.
//
// reconnect (typically the owning ResilientClient's Reconnect method) is
// passed through to every per-cluster Client as WithReconnect, so a Get/List
// call that hits a dead session (SSE stream failure mid-lifetime, not just an
// initial connect failure -- issue #2317) triggers one reconnect-and-retry
// instead of failing every call forever until the pod restarts. Pass nil to
// opt out (matches pre-#2317 behavior).
func NewMCPReaderFactoryWithProvider(localClient client.Reader, provider SessionProvider, reconnect func(context.Context) error, resolver ...registry.ToolPrefixResolver) fleet.ReaderFactory {
	var pr registry.ToolPrefixResolver
	if len(resolver) > 0 {
		pr = resolver[0]
	}
	return &mcpReaderFactory{
		localClient:     localClient,
		sessionProvider: provider,
		reconnect:       reconnect,
		prefixResolver:  pr,
	}
}

// ResolveSession resolves the current MCP session, preferring provider
// (live, self-healing resolution) over static (a one-time snapshot) when a
// provider is set. Returns a clear, typed error when the result is nil --
// i.e. the gateway is currently disconnected -- so callers surface a
// transient error instead of a nil-pointer panic or a silently-stale
// session.
//
// Extracted from mcpReaderFactory.ReaderFor's pre-existing inline logic
// (dedup); also used by NewDiscovererWithProvider and workflowexecution's
// client factory so every fleet-aware consumer resolves sessions
// identically.
func ResolveSession(static *mcp.ClientSession, provider SessionProvider) (*mcp.ClientSession, error) {
	session := static
	if provider != nil {
		session = provider()
	}
	if session == nil {
		return nil, fmt.Errorf("MCP session not available (gateway disconnected)")
	}
	return session, nil
}

func (f *mcpReaderFactory) ReaderFor(ctx context.Context, clusterID string) (client.Reader, error) {
	if clusterID == "" {
		return f.localClient, nil
	}

	session, err := ResolveSession(f.session, f.sessionProvider)
	if err != nil {
		return nil, fmt.Errorf("resolve session for remote cluster %q: %w", clusterID, err)
	}

	var opts []Option
	if f.prefixResolver != nil {
		if prefix := f.prefixResolver.ToolPrefixFor(clusterID); prefix != "" {
			opts = append(opts, WithToolPrefix(prefix))
		}
	} else {
		prefix, err := f.discoverToolPrefixWithReconnect(ctx, session, clusterID)
		if err != nil {
			return nil, fmt.Errorf("resolve tool prefix for cluster %q: %w", clusterID, err)
		}
		opts = append(opts, WithToolPrefix(prefix))
	}

	if f.sessionProvider != nil {
		if f.reconnect != nil {
			opts = append(opts, WithReconnect(f.reconnect))
		}
		return NewFromSessionProvider(f.sessionProvider, clusterID, opts...), nil
	}
	return NewFromSession(session, clusterID, opts...), nil
}

// discoverToolPrefixWithReconnect calls DiscoverToolPrefix against session,
// and on a retryable session error invokes f.reconnect once and retries
// against the freshly-resolved session. Mirrors providerDiscoverer's
// retryWithReconnect (discovery.go) so this call site follows the same
// self-healing semantics as ListClusters/ToolsForCluster instead of the
// pre-#2317 "permanently nil resolver" failure mode one level deeper.
//
// Without this, a session that dies mid-lifetime (gateway itself stays
// healthy) fails ReaderFor -- and therefore remote owner-chain resolution --
// forever, until the pod restarts. Observed live on the Fleet E2E hub as
// repeated "standalone SSE stream: exceeded 5 retries without progress"
// errors reusing the same dead session ID (issue #2340, a gap #2317 missed:
// this call site invoked DiscoverToolPrefix directly on the raw session,
// bypassing the reconnect callback already available on the struct).
func (f *mcpReaderFactory) discoverToolPrefixWithReconnect(ctx context.Context, session *mcp.ClientSession, clusterID string) (string, error) {
	return discoverToolPrefixWithReconnectImpl(ctx, session, f.session, f.sessionProvider, clusterID, f.reconnect)
}

// DiscoverToolPrefixWithReconnect resolves the current session from static/
// provider (via ResolveSession) and calls DiscoverToolPrefix against it, and
// on a retryable session error invokes reconnect once (when non-nil) and
// retries against the freshly re-resolved session.
//
// Exported (issue #2346) so every fleet-aware consumer that calls
// DiscoverToolPrefix directly shares one self-healing implementation instead
// of independently reintroducing the pre-#2340 "no reconnect on a dead
// session" gap at a new call site -- as workflowexecution's
// mcpClientFactory.ClientFor (pkg/workflowexecution/executor/client_factory.go)
// had done, the third such occurrence after #2317 and #2340.
func DiscoverToolPrefixWithReconnect(ctx context.Context, static *mcp.ClientSession, provider SessionProvider, clusterID string, reconnect func(context.Context) error) (string, error) {
	session, err := ResolveSession(static, provider)
	if err != nil {
		return "", err
	}
	return discoverToolPrefixWithReconnectImpl(ctx, session, static, provider, clusterID, reconnect)
}

// discoverToolPrefixWithReconnectImpl is the shared retry body: try once
// against the already-resolved session, and on a retryable error, reconnect
// and re-resolve before retrying once more. Split out so mcpReaderFactory
// (which already has a resolved session in hand for other uses in ReaderFor)
// doesn't pay a redundant ResolveSession call.
func discoverToolPrefixWithReconnectImpl(ctx context.Context, session, static *mcp.ClientSession, provider SessionProvider, clusterID string, reconnect func(context.Context) error) (string, error) {
	prefix, err := DiscoverToolPrefix(ctx, session, clusterID)
	if err == nil || reconnect == nil || !isRetryableSessionError(err) {
		return prefix, err
	}

	if reconnErr := reconnect(ctx); reconnErr != nil {
		return "", fmt.Errorf("reconnect failed: %w (original: %w)", reconnErr, err)
	}

	session, resolveErr := ResolveSession(static, provider)
	if resolveErr != nil {
		return "", resolveErr
	}
	return DiscoverToolPrefix(ctx, session, clusterID)
}
