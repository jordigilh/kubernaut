/*
Copyright 2025 Jordi Gil.

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

package gateway

import (
	"fmt"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// ScopeChecker is an alias for the shared scope.ScopeChecker interface.
// Both Gateway and RO use the same interface for scope validation DI.
//
// BR-SCOPE-002: Gateway Signal Filtering
// ADR-053: Resource Scope Management Architecture
type ScopeChecker = scope.ScopeChecker

// StatusRejected indicates the signal was rejected because the resource is not managed.
// BR-SCOPE-002: Gateway rejects signals from unmanaged resources.
const StatusRejected = "rejected"

// RejectionReason constants for structured rejection responses.
const (
	RejectionReasonUnmanagedResource = "unmanaged_resource"
)

// RejectionResponse contains the details of a scope rejection.
// BR-SCOPE-002: Actionable rejection response with label instructions.
type RejectionResponse struct {
	Status   string `json:"status"`   // "rejected"
	Reason   string `json:"reason"`   // "unmanaged_resource"
	Message  string `json:"message"`  // Human-readable description
	Resource string `json:"resource"` // "namespace:Kind:name"
	Action   string `json:"action"`   // Instructions to add label
}

// NewRejectedResponse creates a ProcessingResponse for rejected (unmanaged) signals.
//
// BR-SCOPE-002: Rejection response contains actionable instructions for operators.
// The response includes the label key and command needed to opt-in the resource.
func NewRejectedResponse(namespace, kind, name string) *ProcessingResponse {
	resourceRef := fmt.Sprintf("%s:%s:%s", namespace, kind, name)
	if namespace == "" {
		resourceRef = fmt.Sprintf("%s:%s", kind, name)
	}

	action := fmt.Sprintf(
		"To manage this resource, add the label: kubectl label %s %s %s=%s",
		kindToKubectlResource(kind), name, scope.ManagedLabelKey, scope.ManagedLabelValueTrue,
	)
	if namespace != "" {
		action = fmt.Sprintf(
			"To manage this resource, add the label: kubectl label -n %s %s %s %s=%s",
			namespace, kindToKubectlResource(kind), name, scope.ManagedLabelKey, scope.ManagedLabelValueTrue,
		)
	}

	return &ProcessingResponse{
		Status:  StatusRejected,
		Message: fmt.Sprintf("Signal rejected: resource %s is not managed by Kubernaut", resourceRef),
		Rejection: &RejectionResponse{
			Status:   StatusRejected,
			Reason:   RejectionReasonUnmanagedResource,
			Message:  fmt.Sprintf("Resource %s is not in Kubernaut's management scope", resourceRef),
			Resource: resourceRef,
			Action:   action,
		},
	}
}

// kindToKubectlResource converts a Kubernetes Kind to the kubectl resource name.
// e.g., "Pod" → "pod", "Deployment" → "deployment", "Node" → "node"
func kindToKubectlResource(kind string) string {
	if kind == "" {
		return "resource"
	}
	return strings.ToLower(kind)
}
