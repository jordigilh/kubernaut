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
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ReplayPolicy is the trusted action policy attached to a logical MCP tool.
// It is assigned by the application/gateway registration layer; callers must
// never infer it from a wire name, MCP annotation, or server-provided schema.
type ReplayPolicy string

const (
	// ReplayRead permits one bounded replay after a retryable session failure.
	ReplayRead ReplayPolicy = "replay_read"
	// ReplayNever forbids automatic replay. This is the safe default for
	// mutations and unknown tools because a transport failure can be ambiguous.
	ReplayNever ReplayPolicy = "replay_never"
)

// SessionHandle is an immutable view of the provider-owned active session.
// Generation changes whenever the owning resilient client publishes a fresh
// session. A binding lease must not publish or call through a stale handle.
type SessionHandle struct {
	Session    Session
	Generation uint64
}

// SessionHandleProvider resolves the current session and its generation.
// Implementations must be safe for concurrent use and must not transfer
// session ownership to the caller.
type SessionHandleProvider func() SessionHandle

// TargetRef identifies the validated target scope for a binding.
type TargetRef struct {
	ClusterID string
}

// ToolBinding is the immutable logical-to-wire mapping used for one attempt.
// WireName and both generations are provider/resolver-owned values.
type ToolBinding struct {
	LogicalName       string
	WireName          string
	InputSchema       json.RawMessage
	ReplayPolicy      ReplayPolicy
	SessionGeneration uint64
	BindingGeneration uint64
}

// ToolBindingLease owns the binding lifetime from resolution through the
// matching CallTool. Release is idempotent and must be called on every path.
type ToolBindingLease interface {
	Binding() ToolBinding
	CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error)
	Release()
}

// ToolBindingResolver resolves a trusted logical tool for a target and can
// refresh it after a provider reconnects. Gateway-specific selection/session
// behavior belongs behind this interface rather than in BridgeTool.
type ToolBindingResolver interface {
	Resolve(context.Context, SessionHandle, TargetRef, string) (ToolBindingLease, error)
	Refresh(context.Context, SessionHandle, TargetRef, string, uint64) (ToolBindingLease, error)
}

// ErrStaleBinding indicates that a refresh attempted to publish behind a
// newer binding generation. The caller must discard the stale lease/result.
var ErrStaleBinding = errors.New("stale binding generation")

// ReplayPolicyForLogicalTool returns the only MCP resource tools approved for
// automatic read recovery. All other names fail closed to ReplayNever.
func ReplayPolicyForLogicalTool(logicalName string) ReplayPolicy {
	switch logicalName {
	case ToolGet, ToolList:
		return ReplayRead
	default:
		return ReplayNever
	}
}
