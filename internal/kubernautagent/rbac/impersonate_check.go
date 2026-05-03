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

// Package rbac provides startup RBAC self-checks for the Kubernaut Agent.
// BR-INTERACTIVE-002 / #891: KA verifies at startup that its ServiceAccount
// has the impersonate verb on users and groups. If denied, interactive mode
// is soft-disabled rather than failing requests at runtime.
package rbac

import (
	"context"
	"fmt"
	"strings"
	"sync"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// impersonateResources lists the K8s resources that KA must be able to
// impersonate for interactive mode. The middleware sets both
// Impersonate-User and Impersonate-Group headers, so both are required.
var impersonateResources = []string{"users", "groups"}

// ImpersonateCheckResult holds the outcome of a startup SSAR check
// for the impersonate verb.
type ImpersonateCheckResult struct {
	Allowed bool
	Reason  string
}

// CheckImpersonatePermission performs SelfSubjectAccessReviews to determine
// whether KA's ServiceAccount can impersonate both users and groups. Both
// are required for interactive mode (DD-AUTH-MCP-001): all MCP tool calls
// run under the authenticated user's identity via K8s impersonation headers
// (Impersonate-User + Impersonate-Group).
func CheckImpersonatePermission(ctx context.Context, client kubernetes.Interface) (ImpersonateCheckResult, error) {
	var denied []string
	for _, resource := range impersonateResources {
		ssar := &authorizationv1.SelfSubjectAccessReview{
			Spec: authorizationv1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Verb:     "impersonate",
					Group:    "",
					Resource: resource,
				},
			},
		}

		result, err := client.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, ssar, metav1.CreateOptions{})
		if err != nil {
			return ImpersonateCheckResult{}, fmt.Errorf("SSAR check for impersonate %s failed: %w", resource, err)
		}

		if !result.Status.Allowed {
			reason := result.Status.Reason
			if reason == "" {
				reason = fmt.Sprintf("impersonate %s denied", resource)
			}
			denied = append(denied, reason)
		}
	}

	if len(denied) > 0 {
		return ImpersonateCheckResult{
			Allowed: false,
			Reason:  strings.Join(denied, "; "),
		}, nil
	}

	return ImpersonateCheckResult{Allowed: true}, nil
}

// InteractiveReadiness tracks whether interactive mode is operational.
// Thread-safe for concurrent reads from the health endpoint.
type InteractiveReadiness struct {
	mu     sync.RWMutex
	state  readinessState
	reason string
}

type readinessState int

const (
	stateNotConfigured readinessState = iota
	stateEnabled
	stateSoftDisabled
)

// NewInteractiveReadiness returns a readiness tracker in the not_configured state.
func NewInteractiveReadiness() *InteractiveReadiness {
	return &InteractiveReadiness{state: stateNotConfigured}
}

// SetEnabled marks interactive mode as fully operational.
func (r *InteractiveReadiness) SetEnabled() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = stateEnabled
	r.reason = ""
}

// SetSoftDisabled marks interactive mode as soft-disabled with a reason.
func (r *InteractiveReadiness) SetSoftDisabled(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = stateSoftDisabled
	r.reason = reason
}

// IsEnabled returns true only when interactive mode is fully operational.
func (r *InteractiveReadiness) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state == stateEnabled
}

// StatusString returns a machine-readable status: "enabled", "soft_disabled", or "not_configured".
func (r *InteractiveReadiness) StatusString() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	switch r.state {
	case stateEnabled:
		return "enabled"
	case stateSoftDisabled:
		return "soft_disabled"
	default:
		return "not_configured"
	}
}

// Reason returns the human-readable reason for soft-disablement, or empty string.
func (r *InteractiveReadiness) Reason() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.reason
}
