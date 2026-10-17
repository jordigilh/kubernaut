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
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// GatewayBindingResolver is the adapter-owned implementation of the
// gateway-neutral ToolBindingResolver contract. EAIGW bindings are resolved
// from the prefixed tools/list view. Kuadrant bindings hold a session lease
// across select_tools, ListTools, and the matching CallTool so concurrent
// target selections cannot overwrite one another.
type GatewayBindingResolver struct {
	discoverer  GatewayDiscoverer
	gatewayType registry.MCPGatewayType

	// Kuadrant selection is session-scoped. A single gate is deliberately
	// conservative: the shared provider session must not be selected for one
	// target while another target is between ListTools and CallTool.
	selectionGate chan struct{}

	mu          sync.Mutex
	generations map[string]uint64
}

// NewGatewayBindingResolver constructs an adapter-owned resolver for a
// discoverer. Gateway-specific behavior remains here; BridgeTool only knows
// the ToolBindingResolver interface.
func NewGatewayBindingResolver(gatewayType registry.MCPGatewayType, discoverer GatewayDiscoverer) (*GatewayBindingResolver, error) {
	if discoverer == nil {
		return nil, fmt.Errorf("gateway binding resolver requires a discoverer")
	}
	if !registry.SupportedGateways[gatewayType] {
		return nil, fmt.Errorf("unsupported gateway type %q for binding resolver", gatewayType)
	}
	resolver := &GatewayBindingResolver{
		discoverer:  discoverer,
		gatewayType: gatewayType,
		generations: make(map[string]uint64),
	}
	if gatewayType == registry.GatewayKuadrant {
		resolver.selectionGate = make(chan struct{}, 1)
	}
	return resolver, nil
}

var _ ToolBindingResolver = (*GatewayBindingResolver)(nil)

func (r *GatewayBindingResolver) Resolve(ctx context.Context, session SessionHandle, target TargetRef, logicalName string) (ToolBindingLease, error) {
	if err := validateBindingRequest(session, target, logicalName); err != nil {
		return nil, err
	}
	release, err := r.acquireSelection(ctx)
	if err != nil {
		return nil, err
	}
	def, err := r.discoverDefinition(ctx, target.ClusterID, logicalName)
	if err != nil {
		release()
		return nil, err
	}

	key := bindingKey(target, logicalName)
	r.mu.Lock()
	generation := r.generations[key] + 1
	r.generations[key] = generation
	r.mu.Unlock()

	return &gatewayBindingLease{
		binding: ToolBinding{
			LogicalName:       logicalName,
			WireName:          def.Name,
			InputSchema:       def.InputSchema,
			ReplayPolicy:      ReplayPolicyForLogicalTool(logicalName),
			SessionGeneration: session.Generation,
			BindingGeneration: generation,
		},
		session: session.Session,
		release: release,
	}, nil
}

func (r *GatewayBindingResolver) Refresh(ctx context.Context, session SessionHandle, target TargetRef, logicalName string, priorGeneration uint64) (ToolBindingLease, error) {
	if err := validateBindingRequest(session, target, logicalName); err != nil {
		return nil, err
	}
	key := bindingKey(target, logicalName)
	r.mu.Lock()
	current := r.generations[key]
	r.mu.Unlock()
	if current != priorGeneration {
		return nil, ErrStaleBinding
	}

	release, err := r.acquireSelection(ctx)
	if err != nil {
		return nil, err
	}
	def, err := r.discoverDefinition(ctx, target.ClusterID, logicalName)
	if err != nil {
		release()
		return nil, err
	}

	r.mu.Lock()
	if r.generations[key] != priorGeneration {
		r.mu.Unlock()
		release()
		return nil, ErrStaleBinding
	}
	nextGeneration := priorGeneration + 1
	r.generations[key] = nextGeneration
	r.mu.Unlock()

	return &gatewayBindingLease{
		binding: ToolBinding{
			LogicalName:       logicalName,
			WireName:          def.Name,
			InputSchema:       def.InputSchema,
			ReplayPolicy:      ReplayPolicyForLogicalTool(logicalName),
			SessionGeneration: session.Generation,
			BindingGeneration: nextGeneration,
		},
		session: session.Session,
		release: release,
	}, nil
}

func validateBindingRequest(session SessionHandle, target TargetRef, logicalName string) error {
	if session.Session == nil {
		return fmt.Errorf("resolve binding for cluster %q: MCP session unavailable", target.ClusterID)
	}
	if strings.TrimSpace(target.ClusterID) == "" {
		return fmt.Errorf("resolve binding: target cluster ID must not be empty")
	}
	if strings.TrimSpace(logicalName) == "" {
		return fmt.Errorf("resolve binding for cluster %q: logical tool name must not be empty", target.ClusterID)
	}
	return nil
}

func (r *GatewayBindingResolver) acquireSelection(ctx context.Context) (func(), error) {
	if r.selectionGate == nil {
		return func() {}, nil
	}
	select {
	case r.selectionGate <- struct{}{}:
		return func() { <-r.selectionGate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *GatewayBindingResolver) discoverDefinition(ctx context.Context, clusterID, logicalName string) (ToolDefinition, error) {
	defs, err := r.discoverer.ToolsForCluster(ctx, clusterID)
	if err != nil {
		return ToolDefinition{}, fmt.Errorf("discover binding for cluster %q: %w", clusterID, err)
	}
	for _, def := range defs {
		if def.Name == logicalName {
			return def, nil
		}
	}

	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	prefix, prefixErr := PrefixFromToolNames(clusterID, names)
	if prefixErr != nil {
		return ToolDefinition{}, fmt.Errorf("resolve binding prefix for cluster %q: %w", clusterID, prefixErr)
	}
	for _, def := range defs {
		if strings.TrimPrefix(def.Name, prefix) == logicalName {
			return def, nil
		}
	}
	return ToolDefinition{}, fmt.Errorf("logical tool %q not found for cluster %q", logicalName, clusterID)
}

func bindingKey(target TargetRef, logicalName string) string {
	return target.ClusterID + "\x00" + logicalName
}

type gatewayBindingLease struct {
	binding ToolBinding
	session Session
	release func()
	one     sync.Once
}

func (l *gatewayBindingLease) Binding() ToolBinding { return l.binding }

func (l *gatewayBindingLease) CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	if params == nil {
		return nil, fmt.Errorf("call binding %q: parameters must not be nil", l.binding.LogicalName)
	}
	if l.session == nil {
		return nil, fmt.Errorf("call binding %q: MCP session unavailable", l.binding.LogicalName)
	}
	callParams := *params
	callParams.Name = l.binding.WireName
	return l.session.CallTool(ctx, &callParams)
}

func (l *gatewayBindingLease) Release() {
	l.one.Do(l.release)
}
