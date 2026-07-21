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
	"time"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// dependentsCacheRetryBackoffs are the delays between re-checks when
// listDependentWorkflowNames's initial List() finds zero matches. mgr.GetClient()
// is informer-cache-backed: a RemediationWorkflow Create() that lands
// milliseconds before this List() runs may not yet be reflected in the local
// watch cache, producing a false "no dependents" (IT-AW-1111-009). Bounded to
// ~550ms worst case -- negligible against typical webhook admission timeouts
// (10s+) for a rare, deliberate administrative DELETE, and mirrors the same
// cache-propagation-lag remediation already applied to CRD status updates via
// RetryGetCRD (actiontype_handler.go, remediationworkflow_handler.go).
var dependentsCacheRetryBackoffs = []time.Duration{150 * time.Millisecond, 400 * time.Millisecond}

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
// the cache-backed k8sClient) reflects every write once the informer's watch
// catches up, so there is no "orphan recovery" reconciliation to perform:
// what K8s reports IS the dependents set. The watch itself lags a live
// Create() by a short, bounded window, though -- see the retry in
// listDependentWorkflowNamesWithRetry, which this delegates to.
func listDependentWorkflowNames(ctx context.Context, k8sClient client.Client, actionType, excludeName string) ([]string, error) {
	return listDependentWorkflowNamesWithRetry(ctx, k8sClient, actionType, excludeName, dependentsCacheRetryBackoffs)
}

// listDependentWorkflowNamesWithRetry is listDependentWorkflowNames's
// implementation, parameterized on the retry backoff schedule for
// determinism in unit tests (production always uses
// dependentsCacheRetryBackoffs via listDependentWorkflowNames).
//
// A zero-length result is retried against the possibility that it's a false
// negative from informer-cache propagation lag rather than a genuine "no
// dependents" -- the two are indistinguishable from inside this function, so
// every zero-result call pays up to len(backoffs) retries before returning.
// A non-empty result is always trusted immediately: cache lag can only ever
// cause a dependent to be temporarily invisible (false negative), never
// phantom-visible (false positive), so there is nothing to retry once at
// least one match is found.
func listDependentWorkflowNamesWithRetry(ctx context.Context, k8sClient client.Client, actionType, excludeName string, backoffs []time.Duration) ([]string, error) {
	if k8sClient == nil || actionType == "" {
		return nil, nil
	}

	list := func() ([]string, error) {
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

	names, err := list()
	for _, backoff := range backoffs {
		if err != nil || len(names) > 0 {
			break
		}
		select {
		case <-ctx.Done():
			return names, err
		case <-time.After(backoff):
		}
		names, err = list()
	}
	return names, err
}
