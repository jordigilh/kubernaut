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

package authwebhook

import (
	"context"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// listDependentWorkflowNames returns the names of all live RemediationWorkflow
// CRDs (cluster-wide, mirroring the namespace-agnostic scope DS's retired
// Postgres catalog used) whose spec.actionType matches actionType, excluding
// excludeName if non-empty.
//
// excludeName exists because both the finalizer reconciler
// (RemediationWorkflowReconciler.reconcileDelete) and the admission webhook's
// best-effort cross-update goroutine (handleDelete) run their dependents
// count while the RW being deleted is still present in etcd -- K8s only
// physically removes an object once its finalizers are cleared, and this
// list runs *before* that removal to compute the count the finalizer removal
// is gated on. Without excluding it, the count would be permanently
// off-by-one-high after every deletion, with no later trigger to correct it.
// Callers with no such self-reference (e.g. the ActionType DELETE dependents
// gate) pass "".
//
// #1661 Change 8d: this is now the SOLE dependents check for ActionType
// deletion and the sole source for status.activeWorkflowCount. DS's Postgres
// catalog stopped learning about RemediationWorkflow CRDs the moment Change
// 8c removed AW's CreateWorkflowInline call, so any DS-backed dependents
// check is permanently blind to workflows created after that point -- the
// safety gap this change closes (IT-AW-1111-009). The live etcd list (via
// the cache-backed k8sClient) is always current, so there is no longer any
// "orphan recovery" reconciliation to perform: what K8s reports IS the
// dependents set.
func listDependentWorkflowNames(ctx context.Context, k8sClient client.Client, actionType, excludeName string) ([]string, error) {
	if k8sClient == nil || actionType == "" {
		return nil, nil
	}

	rwList := &rwv1alpha1.RemediationWorkflowList{}
	if err := k8sClient.List(ctx, rwList); err != nil {
		return nil, err
	}

	var names []string
	for i := range rwList.Items {
		if rwList.Items[i].Spec.ActionType != actionType {
			continue
		}
		if excludeName != "" && rwList.Items[i].Name == excludeName {
			continue
		}
		names = append(names, rwList.Items[i].Name)
	}
	return names, nil
}
