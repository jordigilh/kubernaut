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

package tools

import (
	"context"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// K8sRRExistenceChecker implements RRExistenceChecker by querying the K8s API
// for a RemediationRequest CRD by name. Follows the same pattern as
// pkg/gateway/k8s.Client.GetRemediationRequest (BR-GATEWAY-190).
//
// HARM-004: Prevents orphaned Lease creation for non-existent RRs.
type K8sRRExistenceChecker struct {
	client    client.Client
	namespace string
}

// NewK8sRRExistenceChecker creates a checker backed by a controller-runtime client.
func NewK8sRRExistenceChecker(c client.Client, namespace string) *K8sRRExistenceChecker {
	return &K8sRRExistenceChecker{client: c, namespace: namespace}
}

func (c *K8sRRExistenceChecker) RemediationRequestExists(ctx context.Context, rrID string) (bool, error) {
	var rr remediationv1.RemediationRequest
	err := c.client.Get(ctx, client.ObjectKey{Namespace: c.namespace, Name: rrID}, &rr)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

var _ RRExistenceChecker = (*K8sRRExistenceChecker)(nil)
