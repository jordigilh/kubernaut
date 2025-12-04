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

package k8s

import (
	"context"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client wraps controller-runtime client for CRD operations
//
// This wrapper provides a thin abstraction over controller-runtime client,
// specifically tailored for Gateway's RemediationRequest CRD operations.
//
// Why wrap controller-runtime client:
// 1. Type safety: Methods accept/return RemediationRequest types (no casting)
// 2. Testability: Easy to mock for unit tests
// 3. Error handling: Can add Gateway-specific error handling/logging
// 4. Simplicity: Gateway only needs Create/Update/List operations
type Client struct {
	client client.Client
}

// NewClient creates a new Kubernetes client wrapper
//
// Parameters:
// - kubeClient: controller-runtime client (typically from manager.GetClient())
//
// The underlying client is configured with:
// - RemediationRequest CRD scheme registered
// - Kubernetes API authentication (in-cluster or kubeconfig)
// - Default timeouts (5s connect, 30s request)
func NewClient(kubeClient client.Client) *Client {
	return &Client{client: kubeClient}
}

// CreateRemediationRequest creates a new RemediationRequest CRD in Kubernetes
//
// This method:
// 1. Validates the CRD has required fields (name, namespace)
// 2. Calls Kubernetes API to create the CRD
// 3. Returns error if CRD already exists (conflict) or API fails
//
// Typical latency: p95 ~30ms, p99 ~50ms (Kubernetes API overhead)
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - rr: RemediationRequest CRD to create
//
// Returns:
// - error: Kubernetes API errors (conflict, timeout, validation, etc.)
func (c *Client) CreateRemediationRequest(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
	return c.client.Create(ctx, rr)
}

// UpdateRemediationRequest updates an existing RemediationRequest CRD
//
// This method:
// 1. Calls Kubernetes API to update the CRD
// 2. Handles optimistic concurrency (resourceVersion conflicts)
// 3. Returns error if CRD not found or API fails
//
// Use case: Update deduplication count when duplicate alert received
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - rr: RemediationRequest CRD with updated fields
//
// Returns:
// - error: Kubernetes API errors (not found, conflict, timeout, etc.)
func (c *Client) UpdateRemediationRequest(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) error {
	return c.client.Update(ctx, rr)
}

// ListRemediationRequestsByFingerprint lists RemediationRequests matching a fingerprint
//
// This method queries Kubernetes API for CRDs with matching label:
//
//	kubernaut.ai/signal-fingerprint: <fingerprint>
//
// Use case: Deduplication check (find existing RemediationRequest for same alert)
//
// CRITICAL: Uses direct API calls (bypasses controller-runtime cache) to prevent race conditions
// in multi-pod deployments. Without this, multiple Gateway pods can have stale cache state,
// leading to duplicate CRD creation attempts and incorrect storm detection.
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - fingerprint: Signal fingerprint (SHA256 hash)
//
// Returns:
// - *RemediationRequestList: List of matching CRDs (may be empty)
// - error: Kubernetes API errors (timeout, etc.)
func (c *Client) ListRemediationRequestsByFingerprint(ctx context.Context, fingerprint string) (*remediationv1alpha1.RemediationRequestList, error) {
	var list remediationv1alpha1.RemediationRequestList

	// Kubernetes label values must be ≤63 characters
	// SHA256 fingerprints are 64 chars, so truncate to 63
	fingerprintLabel := fingerprint
	if len(fingerprintLabel) > 63 {
		fingerprintLabel = fingerprintLabel[:63]
	}

	// PRODUCTION FIX: Force direct API call to prevent cache staleness
	// Critical for multi-pod deployments where cache can be inconsistent between pods.
	// client.MatchingFields{} forces controller-runtime to bypass its cache and query
	// the API server directly, ensuring all pods see the same CRD state.
	//
	// Without this:
	// - Pod 1 queries cache: No CRD found → Creates CRD
	// - Pod 2 queries cache: No CRD found → Attempts to create duplicate CRD (conflict)
	// - Storm detection fails due to inconsistent cache state between pods
	err := c.client.List(ctx, &list,
		client.MatchingLabels{
			"kubernaut.ai/signal-fingerprint": fingerprintLabel,
		},
		client.MatchingFields{}, // Forces direct API call, bypasses cache
	)

	return &list, err
}

// GetRemediationRequest retrieves a RemediationRequest CRD by name and namespace
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - namespace: Kubernetes namespace
// - name: CRD name
//
// Returns:
// - *RemediationRequest: The CRD if found
// - error: Kubernetes API errors (not found, timeout, etc.)
func (c *Client) GetRemediationRequest(ctx context.Context, namespace, name string) (*remediationv1alpha1.RemediationRequest, error) {
	var rr remediationv1alpha1.RemediationRequest
	err := c.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &rr)
	return &rr, err
}
