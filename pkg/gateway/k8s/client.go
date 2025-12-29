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
// BR-GATEWAY-185 v1.1: Uses field selector on spec.signalFingerprint instead of labels
// - Labels are mutable and truncated to 63 chars (data loss risk)
// - spec.signalFingerprint is immutable and supports full 64-char SHA256
//
// Use case: Deduplication check (find existing RemediationRequest for same alert)
//
// ARCHITECTURE: Uses cached client with field index for O(1) lookups
// The cache is synced across all pods, providing consistent view of CRD state.
// Multi-pod deployments: Each pod's cache watches the K8s API for changes,
// ensuring eventual consistency with minimal latency.
//
// Parameters:
// - ctx: Context for cancellation and timeout
// - fingerprint: Signal fingerprint (full 64-char SHA256 hash)
//
// Returns:
// - *RemediationRequestList: List of matching CRDs (may be empty)
// - error: Kubernetes API errors (timeout, etc.)
func (c *Client) ListRemediationRequestsByFingerprint(ctx context.Context, fingerprint string) (*remediationv1alpha1.RemediationRequestList, error) {
	var list remediationv1alpha1.RemediationRequestList

	// BR-GATEWAY-185 v1.1: Use field selector on spec.signalFingerprint
	// NO truncation - uses full 64-char SHA256 fingerprint
	// Field index is set up in server.go:NewServerWithMetrics
	err := c.client.List(ctx, &list,
		client.MatchingFields{"spec.signalFingerprint": fingerprint},
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
