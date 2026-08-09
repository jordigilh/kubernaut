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

package infrastructure

// ============================================================================
// AF Resource-Scope E2E Fixtures (#2022, ADR-053)
//
// AF's tool-layer scope check (pkg/shared/scope) fail-closes to unmanaged
// for any target resource/namespace lacking the kubernaut.ai/managed label.
// E2E fixture namespaces created before that check existed were never
// labeled, since nothing previously required it. EnsureManagedNamespace lets
// E2E suites create (or re-label an already-existing) fixture namespace so
// it reflects what a real Kubernaut deployment would already have
// configured, keeping fixtures in sync with the enforced scope contract.
// ============================================================================

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ManagedNamespaceLabelKey/Value mirror pkg/shared/scope's
// ManagedLabelKey/ManagedLabelValueTrue. Duplicated here (rather than
// importing pkg/shared/scope) to keep test/infrastructure free of a
// dependency on apifrontend's production scope package.
const (
	ManagedNamespaceLabelKey   = "kubernaut.ai/managed"
	ManagedNamespaceLabelValue = "true"
)

// EnsureManagedNamespace creates the named namespace labeled
// kubernaut.ai/managed=true, or -- if it already exists -- patches the label
// onto it when missing. Safe to call concurrently from multiple Ginkgo
// parallel processes: a create race resolves via IsAlreadyExists, and a
// label race is idempotent (both writers converge on the same value).
func EnsureManagedNamespace(ctx context.Context, c client.Client, name string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
		Name:   name,
		Labels: map[string]string{ManagedNamespaceLabelKey: ManagedNamespaceLabelValue},
	}}
	if err := c.Create(ctx, ns); err == nil {
		return nil
	} else if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %s: %w", name, err)
	}

	var existing corev1.Namespace
	if err := c.Get(ctx, client.ObjectKey{Name: name}, &existing); err != nil {
		return fmt.Errorf("get existing namespace %s: %w", name, err)
	}
	if existing.Labels[ManagedNamespaceLabelKey] == ManagedNamespaceLabelValue {
		return nil
	}
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	existing.Labels[ManagedNamespaceLabelKey] = ManagedNamespaceLabelValue
	if err := c.Update(ctx, &existing); err != nil {
		return fmt.Errorf("label existing namespace %s: %w", name, err)
	}
	return nil
}
