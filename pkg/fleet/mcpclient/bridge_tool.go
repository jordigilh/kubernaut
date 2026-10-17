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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// Compile-time interface compliance.
var _ tools.Tool = (*BridgeTool)(nil)

// Session abstracts an MCP client session for testing.
type Session interface {
	CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
}

// BridgeTool wraps an MCP-discovered tool as a KA tools.Tool implementation.
// Only used for remote cluster tools — local cluster tools keep using
// existing direct k8stools.
type BridgeTool struct {
	name        string
	description string
	parameters  json.RawMessage
	clusterID   string
	session     Session
	logicalName string
	resolver    ToolBindingResolver
	handle      SessionHandleProvider
	reconnect   func(context.Context) error
	policy      ReadRecoveryPolicy
	observer    RetryObserver
}

// ToolDefinition holds the metadata for an MCP tool discovered via tools/list.
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// toolDefinitionFromMCP converts an mcp.Tool to a ToolDefinition by marshaling
// the input schema from the SDK's typed representation to raw JSON.
func toolDefinitionFromMCP(t *mcp.Tool) ToolDefinition {
	var schema json.RawMessage
	if t.InputSchema != nil {
		schema, _ = json.Marshal(t.InputSchema)
	}
	return ToolDefinition{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: schema,
	}
}

// NewBridgeTool creates a bridge tool that executes a remote MCP tool via the
// given session. The clusterID is used for audit logging context.
func NewBridgeTool(def ToolDefinition, clusterID string, session Session) *BridgeTool {
	return &BridgeTool{
		name:        def.Name,
		description: def.Description,
		parameters:  def.InputSchema,
		clusterID:   clusterID,
		session:     session,
		logicalName: def.Name,
	}
}

// NewBridgeToolWithBindingResolver creates a BridgeTool whose logical name is
// resolved to a trusted, generation-fenced wire binding for every attempt.
// The provider and reconnect callback are owned by the resilient MCP client;
// the BridgeTool never closes or replaces a session itself.
func NewBridgeToolWithBindingResolver(
	def ToolDefinition,
	clusterID string,
	resolver ToolBindingResolver,
	handle SessionHandleProvider,
	reconnect func(context.Context) error,
	policies ...ReadRecoveryPolicy,
) *BridgeTool {
	return newBridgeToolWithBindingResolver(def, clusterID, resolver, handle, reconnect, nil, policies...)
}

// NewBridgeToolWithBindingResolverAndObserver is the production constructor
// for a provider-backed bridge. The observer is optional and is intentionally
// injected at construction time so recovery telemetry never relies on a
// mutable global registry or a post-construction swap.
func NewBridgeToolWithBindingResolverAndObserver(
	def ToolDefinition,
	clusterID string,
	resolver ToolBindingResolver,
	handle SessionHandleProvider,
	reconnect func(context.Context) error,
	observer RetryObserver,
	policies ...ReadRecoveryPolicy,
) *BridgeTool {
	return newBridgeToolWithBindingResolver(def, clusterID, resolver, handle, reconnect, observer, policies...)
}

func newBridgeToolWithBindingResolver(
	def ToolDefinition,
	clusterID string,
	resolver ToolBindingResolver,
	handle SessionHandleProvider,
	reconnect func(context.Context) error,
	observer RetryObserver,
	policies ...ReadRecoveryPolicy,
) *BridgeTool {
	if resolver == nil {
		panic("mcpclient.NewBridgeToolWithBindingResolver: resolver must not be nil")
	}
	if handle == nil {
		panic("mcpclient.NewBridgeToolWithBindingResolver: handle provider must not be nil")
	}
	policy := DefaultReadRecoveryPolicy()
	if len(policies) > 0 {
		policy = policies[0]
	}
	return &BridgeTool{
		name:        def.Name,
		description: def.Description,
		parameters:  def.InputSchema,
		clusterID:   clusterID,
		logicalName: def.Name,
		resolver:    resolver,
		handle:      handle,
		reconnect:   reconnect,
		policy:      policy.normalized(),
		observer:    observer,
	}
}

// NewBridgeToolFromSession creates a BridgeTool using a direct MCP session,
// auto-parsing the clusterID from the tool name's "{clusterID}__" prefix.
// Falls back to empty clusterID for tools without the prefix convention.
func NewBridgeToolFromSession(def ToolDefinition, session *mcp.ClientSession) *BridgeTool {
	clusterID := parseClusterIDFromToolName(def.Name)
	return &BridgeTool{
		name:        def.Name,
		description: def.Description,
		parameters:  def.InputSchema,
		clusterID:   clusterID,
		session:     session,
		logicalName: def.Name,
	}
}

// ClusterID returns the cluster this tool is bound to.
func (b *BridgeTool) ClusterID() string { return b.clusterID }

// parseClusterIDFromToolName extracts the clusterID from a tool name following
// the "{clusterID}__tool_name" convention. Returns empty string if no prefix found.
func parseClusterIDFromToolName(name string) string {
	if idx := strings.Index(name, "__"); idx > 0 {
		return name[:idx]
	}
	return ""
}

func (b *BridgeTool) Name() string                { return b.name }
func (b *BridgeTool) Description() string         { return b.description }
func (b *BridgeTool) Parameters() json.RawMessage { return b.parameters }

// Execute calls the remote MCP tool via the shared session.
// Returns text content from the result, joining multiple text parts with newlines.
func (b *BridgeTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var argsMap map[string]any
	if len(args) > 0 && string(args) != "null" {
		if err := json.Unmarshal(args, &argsMap); err != nil {
			return "", fmt.Errorf("unmarshal args for tool %q: %w", b.name, err)
		}
	}

	if b.resolver != nil {
		return b.executeWithBinding(ctx, argsMap)
	}
	if b.session == nil {
		return "", fmt.Errorf("call tool %q (cluster %q): no active MCP session", b.name, b.clusterID)
	}
	result, err := b.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      b.name,
		Arguments: argsMap,
	})
	if err != nil {
		return "", fmt.Errorf("call tool %q (cluster %q): %w", b.name, b.clusterID, err)
	}

	return bridgeToolResult(b.name, b.clusterID, result)
}

func (b *BridgeTool) executeWithBinding(ctx context.Context, args map[string]any) (string, error) {
	policy := b.policy.normalized()
	if policy.MaxAttempts < 1 {
		policy.MaxAttempts = 1
	}
	logicalName := b.logicalName
	target := TargetRef{ClusterID: b.clusterID}
	budgetCtx, cancel := boundedRecoveryContext(ctx, policy.MaxElapsed)
	defer cancel()

	state := bindingAttemptState{}

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := budgetCtx.Err(); err != nil {
			return "", fmt.Errorf("call tool %q (cluster %q): %w", logicalName, b.clusterID, err)
		}

		text, failure, retry, refresh, generation, err := b.executeBindingAttempt(
			budgetCtx, args, attempt, policy.MaxAttempts, &state, target, logicalName,
		)
		if err != nil {
			return "", err
		}
		if !retry {
			return text, nil
		}
		if failure.Action() == ActionReconnectAndRetryRead {
			if err := b.reconnectForRead(budgetCtx, failure, &state.authRefreshUsed); err != nil {
				return "", fmt.Errorf("call tool %q (cluster %q): %w", logicalName, b.clusterID, err)
			}
			if refresh {
				state.refreshPending = true
				state.refreshGeneration = generation
			}
		}
		if err := waitForReadRetry(budgetCtx, policy, attempt, failure); err != nil {
			terminal := ClassifyTransportError(err)
			b.observeRecovery(budgetCtx, attempt, RetryOutcomeForFailure(terminal, attempt, policy.MaxAttempts), terminal)
			return "", fmt.Errorf("call tool %q (cluster %q): retry wait: %w", logicalName, b.clusterID, err)
		}
	}

	return "", fmt.Errorf("call tool %q (cluster %q): read recovery exhausted", logicalName, b.clusterID)
}

type bindingAttemptState struct {
	refreshPending    bool
	refreshGeneration uint64
	authRefreshUsed   bool
}

func (b *BridgeTool) executeBindingAttempt(ctx context.Context, args map[string]any, attempt, maxAttempts int, state *bindingAttemptState, target TargetRef, logicalName string) (string, *TransportFailure, bool, bool, uint64, error) {
	handle, err := b.bindingHandle(ctx, logicalName, !state.refreshPending)
	if err != nil {
		return "", nil, false, false, 0, err
	}

	var lease ToolBindingLease
	if state.refreshPending {
		lease, err = b.resolver.Refresh(ctx, handle, target, logicalName, state.refreshGeneration)
		state.refreshPending = false
	} else {
		lease, err = b.resolver.Resolve(ctx, handle, target, logicalName)
	}
	if err != nil {
		failure := ClassifyTransportError(err)
		if !shouldRetryRead(logicalName, failure, attempt, maxAttempts) {
			b.observeRecovery(ctx, attempt, RetryOutcomeForFailure(failure, attempt, maxAttempts), failure)
			return "", failure, false, false, 0, fmt.Errorf("resolve binding for tool %q (cluster %q): %w", logicalName, b.clusterID, err)
		}
		b.observeRecovery(ctx, attempt, RetryOutcomeRetry, failure)
		return "", failure, true, false, 0, nil
	}
	if lease == nil {
		return "", nil, false, false, 0, fmt.Errorf("resolve binding for tool %q (cluster %q): nil lease", logicalName, b.clusterID)
	}

	binding := lease.Binding()
	if binding.SessionGeneration != handle.Generation {
		lease.Release()
		return "", nil, false, false, 0, fmt.Errorf("call tool %q (cluster %q): %w", logicalName, b.clusterID, ErrStaleBinding)
	}
	result, callErr := lease.CallTool(ctx, &mcp.CallToolParams{Name: binding.WireName, Arguments: args})
	lease.Release()
	if callErr == nil {
		text, resultErr := bridgeToolResult(logicalName, b.clusterID, result)
		if resultErr != nil {
			b.observeRecovery(ctx, attempt, RetryOutcomeFailed, nil)
			return "", nil, false, false, 0, resultErr
		}
		if attempt > 1 {
			b.observeRecovery(ctx, attempt, RetryOutcomeSuccess, nil)
		}
		return text, nil, false, false, 0, nil
	}

	failure := ClassifyTransportError(callErr)
	if !shouldRetryBinding(binding, failure, attempt, maxAttempts) {
		b.observeRecovery(ctx, attempt, RetryOutcomeForFailure(failure, attempt, maxAttempts), failure)
		return "", failure, false, false, 0, fmt.Errorf("call tool %q (cluster %q): %w", logicalName, b.clusterID, callErr)
	}
	b.observeRecovery(ctx, attempt, RetryOutcomeRetry, failure)
	return "", failure, true, true, binding.BindingGeneration, nil
}

func (b *BridgeTool) bindingHandle(ctx context.Context, logicalName string, allowReconnect bool) (SessionHandle, error) {
	handle := b.handle()
	if handle.Session != nil {
		return handle, nil
	}
	if !allowReconnect || b.reconnect == nil || ReplayPolicyForLogicalTool(logicalName) != ReplayRead {
		return handle, fmt.Errorf("call tool %q (cluster %q): no active MCP session", logicalName, b.clusterID)
	}
	if err := b.reconnect(ctx); err != nil {
		return handle, fmt.Errorf("reconnect for tool %q (cluster %q): %w", logicalName, b.clusterID, err)
	}
	handle = b.handle()
	if handle.Session == nil {
		return handle, fmt.Errorf("call tool %q (cluster %q): no active MCP session after reconnect", logicalName, b.clusterID)
	}
	return handle, nil
}

// RetryOutcomeForFailure returns a terminal bounded outcome for a failed
// attempt. It is exported for composition-root adapters and tests that need
// to assert the no-duplicate-terminal-observation contract.
func RetryOutcomeForFailure(failure *TransportFailure, attempt, maxAttempts int) string {
	if failure != nil && failure.Action() != ActionFail && attempt >= maxAttempts {
		return RetryOutcomeExhausted
	}
	return RetryOutcomeFailed
}

func (b *BridgeTool) observeRecovery(ctx context.Context, attempt int, outcome string, failure *TransportFailure) {
	if b.observer == nil {
		return
	}
	observation := RetryObservation{
		Source:      "bridge_tool",
		ToolClass:   "read",
		ClusterID:   b.clusterID,
		LogicalTool: b.logicalName,
		Attempt:     attempt,
		Outcome:     outcome,
	}
	if failure != nil {
		observation.FailureClass = failure.Kind
		observation.FailureCode = failure.FailureCode
	}
	b.observer.ObserveRetry(ctx, observation)
}

func shouldRetryRead(logicalName string, failure *TransportFailure, attempt, maxAttempts int) bool {
	return ReplayPolicyForLogicalTool(logicalName) == ReplayRead &&
		failure != nil && failure.Action() != ActionFail && attempt < maxAttempts
}

func shouldRetryBinding(binding ToolBinding, failure *TransportFailure, attempt, maxAttempts int) bool {
	return binding.ReplayPolicy == ReplayRead &&
		failure != nil && failure.Action() != ActionFail && attempt < maxAttempts
}

func (b *BridgeTool) reconnectForRead(ctx context.Context, failure *TransportFailure, authRefreshUsed *bool) error {
	if b.reconnect == nil {
		return failure
	}
	if failure != nil && failure.Kind == FailureUnauthorized {
		if *authRefreshUsed {
			return failure
		}
		*authRefreshUsed = true
	}
	if err := b.reconnect(ctx); err != nil {
		return fmt.Errorf("reconnect failed: %w (original: %w)", err, failure)
	}
	return nil
}

func waitForReadRetry(ctx context.Context, policy ReadRecoveryPolicy, attempt int, failure *TransportFailure) error {
	return policy.Sleep(ctx, policy.delay(attempt, failure))
}

func bridgeToolResult(name, clusterID string, result *mcp.CallToolResult) (string, error) {
	if result == nil {
		return "", fmt.Errorf("remote tool %q (cluster %q) returned nil result", name, clusterID)
	}

	if result.IsError {
		return "", fmt.Errorf("remote tool %q (cluster %q) returned error: %s", name, clusterID, ExtractText(result))
	}

	return ExtractText(result), nil
}
