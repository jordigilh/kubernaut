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

package severity

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	sharedk8s "github.com/jordigilh/kubernaut/pkg/shared/k8s"
)

// ErrGroupEnumerationUnavailable reports that the cross-group sibling check
// could not be performed at all: no mapper/client wired, a Kind that cannot be
// enumerated, or a read failure other than NotFound (#2658 R3-5). Callers
// must fail closed -- an unavailable check is never evidence of a unique
// instance.
var ErrGroupEnumerationUnavailable = errors.New("cross-group sibling enumeration unavailable")

// restMapperSiblingChecker is the production CrossGroupSiblingChecker: it
// enumerates every API group a Kind is registered in through the RESTMapper
// (one discovery cache, no extra API surface) and issues one existence GET per
// other group through the direct client.
//
// The client must NOT be the circuit-breaker-wrapped client (#2658 D1.5): a
// CB-open state would surface as a generic error and be indistinguishable from
// a genuine read failure, while the mapper itself is already
// discovery-backed and non-CB. Callers wire the raw dynamic client for exactly
// that reason.
type restMapperSiblingChecker struct {
	mapper meta.RESTMapper
	client dynamic.Interface
	logger logr.Logger
}

// NewRESTMapperCrossGroupSiblingChecker builds the RESTMapper-backed
// CrossGroupSiblingChecker (#2658).
//
// It is the FALLBACK: the mapper can only be asked about a Kind through a
// guessed plural resource name, so a Kind whose real plural differs (Gateway ->
// "gatewaies") cannot be enumerated and fails closed. Prefer
// NewDiscoveryCrossGroupSiblingChecker wherever a discovery client exists.
//
// A nil mapper or client makes every check return
// ErrGroupEnumerationUnavailable, which the triage gate turns into a
// fail-closed ambiguity.
func NewRESTMapperCrossGroupSiblingChecker(mapper meta.RESTMapper, client dynamic.Interface, logger logr.Logger) CrossGroupSiblingChecker {
	return &restMapperSiblingChecker{mapper: mapper, client: client, logger: logger}
}

// CheckCrossGroupSiblings returns the API groups (other than targetGroup) in
// which a resource with the same namespace/kind/name exists. Groups are
// returned in the mapper's enumeration order, deduplicated. A Kind registered
// in a single group returns no siblings without touching the API server: it
// cannot have a cross-group sibling (the fast path of D1.1).
func (c *restMapperSiblingChecker) CheckCrossGroupSiblings(ctx context.Context, namespace, kind, name, targetGroup string) ([]string, error) {
	if c.mapper == nil || c.client == nil {
		return nil, fmt.Errorf("%w: RESTMapper or Kubernetes client not configured", ErrGroupEnumerationUnavailable)
	}
	gvks, err := sharedk8s.KindsForKind(c.mapper, kind)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGroupEnumerationUnavailable, err)
	}
	candidates := otherGroupGVKs(gvks, targetGroup)
	if len(candidates) == 0 {
		return nil, nil
	}

	siblings := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, gvk := range candidates {
		if _, done := seen[gvk.Group]; done {
			continue // one existence check per group (multiple versions)
		}
		seen[gvk.Group] = struct{}{}
		exists, existsErr := c.exists(ctx, gvk, namespace, name)
		if existsErr != nil {
			return nil, existsErr
		}
		if exists {
			siblings = append(siblings, gvk.Group)
		}
	}
	return siblings, nil
}

// otherGroupGVKs filters the enumeration down to the groups that could hold a
// cross-group sibling, preserving the mapper's order.
func otherGroupGVKs(gvks []schema.GroupVersionKind, targetGroup string) []schema.GroupVersionKind {
	others := make([]schema.GroupVersionKind, 0, len(gvks))
	for _, gvk := range gvks {
		if gvk.Group == targetGroup {
			continue
		}
		others = append(others, gvk)
	}
	return others
}

// exists resolves gvk through the mapper and issues a single existence GET.
// NotFound is "no sibling"; any other error propagates so the caller fails
// closed. A namespaced GVK is only a sibling of a namespaced target (the
// identity is namespace-scoped), so it is skipped when the target carries no
// namespace.
func (c *restMapperSiblingChecker) exists(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (bool, error) {
	mapping, err := c.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, fmt.Errorf("%w: resolve %s: %w", ErrGroupEnumerationUnavailable, gvk.String(), err)
	}
	resource := c.client.Resource(mapping.Resource)
	var getter dynamic.ResourceInterface = resource
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if namespace == "" {
			return false, nil
		}
		getter = resource.Namespace(namespace)
	}
	if _, getErr := getter.Get(ctx, name, metav1.GetOptions{}); getErr != nil {
		if apierrors.IsNotFound(getErr) {
			return false, nil
		}
		c.logger.Info("cross-group sibling read failed",
			"kind", gvk.Kind, "group", gvk.Group, "namespace", namespace, "name", name, "error", getErr.Error())
		return false, fmt.Errorf("read candidate sibling %s: %w", gvk.String(), getErr)
	}
	return true, nil
}
