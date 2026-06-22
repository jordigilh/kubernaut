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

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/fleet/acm"
	"github.com/jordigilh/kubernaut/pkg/fleet/fmc"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// NewScopeChecker creates a scope.ScopeChecker appropriate for the given FleetConfig.
//
// When fleet is disabled (or no endpoint configured), returns the local checker unchanged.
// When fleet is enabled, wraps the local checker with a FederatedScopeChecker that
// routes local checks to scope.Manager and remote checks to the configured backend.
//
// Supported backends:
//   - "fmc": FMC HTTP client — queries the FMC REST API for scope checks (ADR-068)
//   - "acm": ACM Search GraphQL adapter — queries ACM Search for scope checks (ADR-068)
//
// References: ADR-068, BR-INTEGRATION-065
func NewScopeChecker(localChecker scope.ScopeChecker, cfg FleetConfig, logger logr.Logger) (scope.ScopeChecker, error) {
	if !cfg.Enabled {
		return localChecker, nil
	}

	endpoint := cfg.EffectiveEndpoint()
	if endpoint == "" {
		return localChecker, nil
	}

	backend := cfg.effectiveBackend()
	switch backend {
	case BackendFMC:
		remoteChecker := fmc.NewHTTPClient(endpoint)
		return NewFederatedScopeChecker(localChecker, remoteChecker, logger), nil
	case BackendACM:
		remoteChecker := acm.NewClient(endpoint)
		return NewFederatedScopeChecker(localChecker, remoteChecker, logger), nil
	default:
		return nil, fmt.Errorf("fleet: unsupported backend %q", backend)
	}
}
