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

package adapters

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// K8sSignalContextResolver reads the RemediationRequest CR from the K8s API
// and maps its spec fields to a SignalContext for Phase 3 workflow discovery.
type K8sSignalContextResolver struct {
	client    client.Reader
	namespace string
}

// NewK8sSignalContextResolver creates a resolver that reads RR CRs from the
// given namespace.
func NewK8sSignalContextResolver(c client.Reader, namespace string) *K8sSignalContextResolver {
	return &K8sSignalContextResolver{client: c, namespace: namespace}
}

// ResolveSignalContext fetches the RR CR and maps spec fields to SignalContext.
func (r *K8sSignalContextResolver) ResolveSignalContext(ctx context.Context, rrID string) (*katypes.SignalContext, error) {
	var rr remediationv1.RemediationRequest
	key := client.ObjectKey{Namespace: r.namespace, Name: rrID}
	if err := r.client.Get(ctx, key, &rr); err != nil {
		return nil, fmt.Errorf("resolve signal context for %s: %w", rrID, err)
	}

	return &katypes.SignalContext{
		Name:           rr.Spec.SignalName,
		Severity:       rr.Spec.Severity,
		ResourceKind:   rr.Spec.TargetResource.Kind,
		ResourceName:   rr.Spec.TargetResource.Name,
		Namespace:      rr.Spec.TargetResource.Namespace,
		RemediationID:  rr.Name,
	}, nil
}

// ResolveEnrichmentData returns empty enrichment data. Full enrichment is
// handled by the investigator's enrichment pipeline, not the signal resolver.
func (r *K8sSignalContextResolver) ResolveEnrichmentData(_ context.Context, _ string) (*prompt.EnrichmentData, error) {
	return &prompt.EnrichmentData{}, nil
}
