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

package fleet

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/fleet/acm"
	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// ScopeCheckerOption configures optional behavior for NewScopeChecker.
type ScopeCheckerOption func(*scopeCheckerOptions)

type scopeCheckerOptions struct {
	clusterLookup ClusterLookup
}

// WithClusterRegistry adds a cluster-level precondition to scope checks using
// the provided ClusterLookup. When set, remote scope checks first verify the
// cluster is known before proceeding to resource-level checks.
func WithClusterRegistry(lookup ClusterLookup) ScopeCheckerOption {
	return func(o *scopeCheckerOptions) {
		o.clusterLookup = lookup
	}
}

// NewScopeChecker creates a scope.ScopeChecker appropriate for the given FleetConfig.
//
// When fleet is disabled (or no endpoint configured), returns the local checker unchanged.
// When fleet is enabled, wraps the local checker with a FederatedScopeChecker that
// routes local checks to scope.Manager and remote checks to the configured backend.
//
// Options:
//   - WithClusterRegistry: adds cluster-level precondition (3-level hierarchy)
//
// Supported backends:
//   - "fmc": FMC HTTP client — queries the FMC REST API for scope checks (ADR-068)
//   - "acm": ACM Search GraphQL adapter — queries ACM Search for scope checks (ADR-068)
//
// References: ADR-068, BR-INTEGRATION-065
func NewScopeChecker(localChecker scope.ScopeChecker, cfg FleetConfig, logger logr.Logger, opts ...ScopeCheckerOption) (scope.ScopeChecker, error) {
	if !cfg.Enabled {
		return localChecker, nil
	}

	endpoint := cfg.EffectiveEndpoint()
	if endpoint == "" {
		return localChecker, nil
	}

	o := &scopeCheckerOptions{}
	for _, opt := range opts {
		opt(o)
	}

	var checkerOpts []FederatedScopeCheckerOption
	if o.clusterLookup != nil {
		checkerOpts = append(checkerOpts, WithClusterLookup(o.clusterLookup))
	}

	backend := cfg.effectiveBackend()
	switch backend {
	case BackendFMC:
		remoteChecker := fmc.NewHTTPClient(endpoint)
		return NewFederatedScopeChecker(localChecker, remoteChecker, logger, checkerOpts...), nil
	case BackendACM:
		var acmOpts []acm.ClientOption
		if cfg.TLSCAFile != "" {
			reloader, err := sharedtls.NewCAReloaderFromFile(cfg.TLSCAFile)
			if err != nil {
				return nil, fmt.Errorf("fleet: failed to load ACM TLS CA from %s: %w", cfg.TLSCAFile, err)
			}
			acmOpts = append(acmOpts, acm.WithHTTPClient(&http.Client{
				Timeout:   10 * time.Second,
				Transport: reloader,
			}))
		}
		remoteChecker := acm.NewClient(endpoint, acmOpts...)
		return NewFederatedScopeChecker(localChecker, remoteChecker, logger, checkerOpts...), nil
	default:
		return nil, fmt.Errorf("fleet: unsupported backend %q", backend)
	}
}
