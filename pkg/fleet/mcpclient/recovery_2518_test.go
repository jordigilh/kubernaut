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

package mcpclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
)

type bindingTestSession struct{}

type bindingTestHTTPError struct {
	status     int
	retryAfter time.Duration
}

func (e bindingTestHTTPError) Error() string { return http.StatusText(e.status) }

func (e bindingTestHTTPError) HTTPStatusCode() int { return e.status }

func (e bindingTestHTTPError) RetryAfterDuration() time.Duration { return e.retryAfter }

func (bindingTestSession) CallTool(context.Context, *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return nil, errors.New("binding test session must not be called directly")
}

type bindingTestLease struct {
	binding mcpclient.ToolBinding
	result  *mcp.CallToolResult
	err     error
	calls   atomic.Int32
	release atomic.Int32
}

func (l *bindingTestLease) Binding() mcpclient.ToolBinding {
	return l.binding
}

func (l *bindingTestLease) CallTool(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	l.calls.Add(1)
	Expect(params.Name).To(Equal(l.binding.WireName),
		"a lease must overwrite the caller-visible logical name with its trusted wire binding")
	return l.result, l.err
}

func (l *bindingTestLease) Release() {
	l.release.Add(1)
}

type bindingTestResolver struct {
	initial    *bindingTestLease
	refreshed  *bindingTestLease
	refreshErr error
	resolves   atomic.Int32
	refreshes  atomic.Int32
}

func (r *bindingTestResolver) Resolve(context.Context, mcpclient.SessionHandle, mcpclient.TargetRef, string) (mcpclient.ToolBindingLease, error) {
	r.resolves.Add(1)
	return r.initial, nil
}

func (r *bindingTestResolver) Refresh(context.Context, mcpclient.SessionHandle, mcpclient.TargetRef, string, uint64) (mcpclient.ToolBindingLease, error) {
	r.refreshes.Add(1)
	if r.refreshErr != nil {
		return nil, r.refreshErr
	}
	return r.refreshed, nil
}

var _ = Describe("Fleet MCP recovery contracts (Issue #2518)", func() {
	It("UT-FLEET-2518-001 [SI-10, CC7.2]: classifies the SDK and HTTP action matrix without retrying semantic tool failures", func() {
		tests := []struct {
			name       string
			err        error
			kind       mcpclient.TransportFailureKind
			action     mcpclient.RecoveryAction
			retryAfter time.Duration
		}{
			{name: "connection reset", err: mcp.ErrConnectionClosed, kind: mcpclient.FailureSessionReset, action: mcpclient.ActionReconnectAndRetryRead},
			{name: "unexpected EOF", err: io.ErrUnexpectedEOF, kind: mcpclient.FailureSessionReset, action: mcpclient.ActionReconnectAndRetryRead},
			{name: "rate limited", err: bindingTestHTTPError{status: http.StatusTooManyRequests, retryAfter: 9 * time.Second}, kind: mcpclient.FailureHTTPTransient, action: mcpclient.ActionRetryRead, retryAfter: 9 * time.Second},
			{name: "server unavailable", err: bindingTestHTTPError{status: http.StatusServiceUnavailable}, kind: mcpclient.FailureHTTPTransient, action: mcpclient.ActionRetryRead},
			{name: "unauthorized", err: bindingTestHTTPError{status: http.StatusUnauthorized}, kind: mcpclient.FailureUnauthorized, action: mcpclient.ActionReconnectAndRetryRead},
			{name: "bad request", err: bindingTestHTTPError{status: http.StatusBadRequest}, kind: mcpclient.FailurePermanent, action: mcpclient.ActionFail},
			{name: "caller canceled", err: context.Canceled, kind: mcpclient.FailureCallerCanceled, action: mcpclient.ActionFail},
			{name: "unknown", err: errors.New("unclassified transport failure"), kind: mcpclient.FailureUnknown, action: mcpclient.ActionFail},
		}

		for _, test := range tests {
			failure := mcpclient.ClassifyTransportError(test.err)
			Expect(failure.Kind).To(Equal(test.kind), test.name)
			Expect(failure.Action()).To(Equal(test.action), test.name)
			Expect(failure.RetryAfter).To(Equal(test.retryAfter), test.name)
		}

		semantic := mcpclient.ClassifyTransportError(errors.New("tool result is_error"))
		Expect(semantic.Action()).To(Equal(mcpclient.ActionFail), "CallToolResult.IsError is handled before transport classification")
	})

	It("UT-FLEET-2518-002 [SI-10]: composes a bounded read policy with deterministic jitter and caller cancellation", func() {
		var sleeps []time.Duration
		policy := mcpclient.ReadRecoveryPolicy{
			MaxAttempts:    3,
			MaxElapsed:     time.Second,
			InitialBackoff: 100 * time.Millisecond,
			MaxBackoff:     2 * time.Second,
			RetryAfterCap:  5 * time.Second,
			Jitter: func(attempt int, base time.Duration) time.Duration {
				Expect(attempt).To(BeNumerically(">=", 1))
				return base + 10*time.Millisecond
			},
			Sleep: func(ctx context.Context, delay time.Duration) error {
				sleeps = append(sleeps, delay)
				return ctx.Err()
			},
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		Expect(policy.Sleep(ctx, 25*time.Millisecond)).To(MatchError(context.Canceled))
		Expect(sleeps).To(Equal([]time.Duration{25 * time.Millisecond}))
	})

	It("UT-FLEET-2518-005 [AC-6]: derives replayability only from the trusted logical allow-list", func() {
		Expect(mcpclient.ReplayPolicyForLogicalTool(mcpclient.ToolGet)).To(Equal(mcpclient.ReplayRead))
		Expect(mcpclient.ReplayPolicyForLogicalTool(mcpclient.ToolList)).To(Equal(mcpclient.ReplayRead))
		for _, name := range []string{
			mcpclient.ToolCreateOrUpdate,
			mcpclient.ToolDelete,
			"approve_workflow",
			"patch_resource",
			"execute_workflow",
			"resources_get__attacker_wire_name",
		} {
			Expect(mcpclient.ReplayPolicyForLogicalTool(name)).To(Equal(mcpclient.ReplayNever), name)
		}
	})

	It("UT-FLEET-2518-003 [AC-4, AU-3]: re-resolves the current binding and returns one logical read result after a session reset", func() {
		first := &bindingTestLease{
			binding: mcpclient.ToolBinding{
				LogicalName:       "resources_get",
				WireName:          "old-target__resources_get",
				ReplayPolicy:      mcpclient.ReplayRead,
				SessionGeneration: 1,
				BindingGeneration: 1,
			},
			err: mcp.ErrConnectionClosed,
		}
		refreshed := &bindingTestLease{
			binding: mcpclient.ToolBinding{
				LogicalName:       "resources_get",
				WireName:          "new-target__resources_get",
				ReplayPolicy:      mcpclient.ReplayRead,
				SessionGeneration: 2,
				BindingGeneration: 2,
			},
			result: &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: `{"metadata":{"name":"nginx"}}`}},
			},
		}
		resolver := &bindingTestResolver{initial: first, refreshed: refreshed}
		var reconnects atomic.Int32
		var sessionGeneration atomic.Uint64
		sessionGeneration.Store(1)

		tool := mcpclient.NewBridgeToolWithBindingResolver(
			mcpclient.ToolDefinition{
				Name:        "resources_get",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
			"prod-east",
			resolver,
			func() mcpclient.SessionHandle {
				return mcpclient.SessionHandle{Session: bindingTestSession{}, Generation: sessionGeneration.Load()}
			},
			func(context.Context) error {
				reconnects.Add(1)
				sessionGeneration.Store(2)
				return nil
			},
		)

		result, err := tool.Execute(context.Background(), json.RawMessage(`{"name":"nginx"}`))
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(ContainSubstring(`"name":"nginx"`))
		Expect(first.calls.Load()).To(Equal(int32(1)))
		Expect(refreshed.calls.Load()).To(Equal(int32(1)))
		Expect(first.release.Load()).To(Equal(int32(1)))
		Expect(refreshed.release.Load()).To(Equal(int32(1)))
		Expect(resolver.resolves.Load()).To(Equal(int32(1)))
		Expect(resolver.refreshes.Load()).To(Equal(int32(1)))
		Expect(reconnects.Load()).To(Equal(int32(1)))
	})

	It("UT-FLEET-2518-005 [AC-6]: never replays a mutation after an ambiguous session failure", func() {
		mutation := &bindingTestLease{
			binding: mcpclient.ToolBinding{
				LogicalName:       "resources_create_or_update",
				WireName:          "prod-east__resources_create_or_update",
				ReplayPolicy:      mcpclient.ReplayNever,
				SessionGeneration: 1,
				BindingGeneration: 1,
			},
			err: mcp.ErrConnectionClosed,
		}
		resolver := &bindingTestResolver{initial: mutation}
		var reconnects atomic.Int32

		tool := mcpclient.NewBridgeToolWithBindingResolver(
			mcpclient.ToolDefinition{Name: "resources_create_or_update"},
			"prod-east",
			resolver,
			func() mcpclient.SessionHandle {
				return mcpclient.SessionHandle{Session: bindingTestSession{}, Generation: 1}
			},
			func(context.Context) error {
				reconnects.Add(1)
				return nil
			},
		)

		_, err := tool.Execute(context.Background(), json.RawMessage(`{"resource":"job"}`))
		Expect(err).To(HaveOccurred())
		Expect(mutation.calls.Load()).To(Equal(int32(1)))
		Expect(mutation.release.Load()).To(Equal(int32(1)))
		Expect(resolver.refreshes.Load()).To(Equal(int32(0)))
		Expect(reconnects.Load()).To(Equal(int32(0)),
			"ReplayNever must not reconnect and resend an ambiguous mutation")
	})

	It("UT-FLEET-2518-004 [AC-4, SI-10]: rejects stale binding refreshes instead of publishing an older generation", func() {
		stale := &bindingTestLease{
			binding: mcpclient.ToolBinding{
				LogicalName:       "resources_get",
				WireName:          "stale__resources_get",
				ReplayPolicy:      mcpclient.ReplayRead,
				SessionGeneration: 1,
				BindingGeneration: 1,
			},
			err: mcp.ErrConnectionClosed,
		}
		resolver := &bindingTestResolver{
			initial:    stale,
			refreshed:  stale,
			refreshErr: errors.New("stale binding generation"),
		}
		var sessionGeneration atomic.Uint64
		sessionGeneration.Store(1)

		tool := mcpclient.NewBridgeToolWithBindingResolver(
			mcpclient.ToolDefinition{Name: "resources_get"},
			"prod-east",
			resolver,
			func() mcpclient.SessionHandle {
				return mcpclient.SessionHandle{Session: bindingTestSession{}, Generation: sessionGeneration.Load()}
			},
			func(context.Context) error {
				sessionGeneration.Store(2)
				return nil
			},
		)

		_, err := tool.Execute(context.Background(), nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("stale binding generation"))
	})
})
