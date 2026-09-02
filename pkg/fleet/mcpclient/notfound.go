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
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// asRemoteNotFound recognizes the Kubernetes API's standard NotFound message
// shape (apierrors.NewNotFound's `"<resource>[.<group>] %q not found"`)
// inside a remote MCP tool's error text and converts it to a typed
// *apierrors.StatusError, or returns nil when the text doesn't match.
//
// The MCP protocol only carries tool errors as plain text (result.IsError
// renders as a formatted string, not a Go error type), so the remote
// kube-mcp-server's own apierrors.NewNotFound arrives here as a string.
// Every caller relying on apierrors.IsNotFound / client.IgnoreNotFound
// against a resource fetched through this client -- e.g.
// JobExecutor.Cleanup's ownership-check Get and idempotent-delete Delete
// (pkg/workflowexecution/executor/job.go) -- silently never matched,
// permanently blocking WorkflowExecution's cleanup finalizer whenever the
// target Job never existed (issue #2349).
//
// This mirrors the identical, independently-discovered fix already applied
// to kubernaut-agent's own overlay client
// (internal/kubernautagent/tools/custom/fleet_resource_context.go, issue
// #2344) -- this is the foundational occurrence: that fix only covered
// kubernaut-agent's investigator tools, not this shared client used by
// every other fleet consumer (Gateway, WorkflowExecution, SignalProcessing,
// EffectivenessMonitor, RemediationOrchestrator, APIFrontend).
//
// Scoped to the exact requested name (`%q not found`), not a blind "not
// found" substring match, so an unrelated error mentioning a different name
// isn't misclassified.
func asRemoteNotFound(errText string, gvk schema.GroupVersionKind, name string) error {
	if !strings.Contains(errText, fmt.Sprintf("%q not found", name)) {
		return nil
	}
	return apierrors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: strings.ToLower(gvk.Kind)}, name)
}
