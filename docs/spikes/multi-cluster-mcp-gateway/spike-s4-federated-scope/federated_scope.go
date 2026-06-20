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

package mcpclient

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

const managedLabel = "kubernaut.ai/managed"

// FederatedScopeChecker implements scope.ScopeChecker by routing scope checks
// to the local ScopeChecker (informer-backed) or remote MCPResourceClient
// depending on the cluster context.
//
// This serves both GW (signal filtering) and RO (remediation scope blocking).
//
// Authority: Issue #54, WS7, ADR-053 (Resource Scope Management)
type FederatedScopeChecker struct {
	local         scope.ScopeChecker
	remote        MCPResourceClient
	clusterPrefix string
	logger        logr.Logger
}

// NewFederatedScopeChecker creates a scope checker that routes local/remote.
// If clusterPrefix is empty, all checks go to the local checker.
func NewFederatedScopeChecker(local scope.ScopeChecker, remote MCPResourceClient, clusterPrefix string, logger logr.Logger) *FederatedScopeChecker {
	return &FederatedScopeChecker{
		local:         local,
		remote:        remote,
		clusterPrefix: clusterPrefix,
		logger:        logger.WithName("federated-scope"),
	}
}

// IsManaged implements scope.ScopeChecker using the 2-level hierarchy (ADR-053):
//  1. Resource label: kubernaut.ai/managed=true -> managed
//  2. Namespace label: kubernaut.ai/managed=true -> managed
//  3. Default: unmanaged (safe default)
//
// For remote clusters (clusterPrefix != ""), it uses MCPResourceClient to fetch labels.
// For local cluster (clusterPrefix == ""), it delegates to the local ScopeChecker.
func (f *FederatedScopeChecker) IsManaged(ctx context.Context, namespace, kind, name string) (bool, error) {
	if f.clusterPrefix == "" {
		return f.local.IsManaged(ctx, namespace, kind, name)
	}

	resourceLabels, err := f.remote.GetLabels(ctx, f.clusterPrefix, kind, namespace, name)
	if err != nil {
		f.logger.Info("resource label check failed, falling through to namespace",
			"kind", kind, "name", name, "namespace", namespace, "error", err)
	} else if resourceLabels[managedLabel] == "true" {
		return true, nil
	}

	nsLabels, err := f.remote.GetLabels(ctx, f.clusterPrefix, "Namespace", "", namespace)
	if err != nil {
		return false, fmt.Errorf("checking namespace %q scope on cluster %s: %w",
			namespace, f.clusterPrefix, err)
	}
	if nsLabels[managedLabel] == "true" {
		return true, nil
	}

	return false, nil
}

// Compile-time interface compliance.
var _ scope.ScopeChecker = (*FederatedScopeChecker)(nil)
