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
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/fleet"
)

// ConnectConfig bundles everything Connect needs to dial the MCP Gateway on
// a fleet-aware service's behalf: the endpoint, the shared
// fleet.FleetOAuth2Config/FleetResilienceConfig DTOs, and
// CredentialsBasePath -- the one genuinely per-service-varying piece (e.g.
// "/etc/gateway/fleet-oauth2"), since every fleet-aware service mounts its
// OAuth2 client-id/client-secret Secret under a different on-disk path.
type ConnectConfig struct {
	// Endpoint is the MCP Gateway URL to dial. Callers must check this is
	// non-empty before calling Connect -- an empty Endpoint means fleet mode
	// is not configured at all, a decision Connect does not make on the
	// caller's behalf.
	Endpoint string

	// OAuth2 holds fleet OAuth2 credentials.
	OAuth2 fleet.FleetOAuth2Config

	// Resilience overrides ResilienceConfigFromFleet's backoff/timeout
	// tuning. Zero value uses DefaultResilienceConfig().
	Resilience fleet.FleetResilienceConfig

	// CredentialsBasePath is the on-disk directory this service's fleet
	// OAuth2 client-id/client-secret files are mounted at. Only consulted
	// when OAuth2.Enabled.
	CredentialsBasePath string
}

// Connect dials the MCP Gateway with backoff and returns a *ResilientClient
// that keeps retrying/reconnecting in the background for the lifetime of the
// process (ResilientClient.connectWithBackoff/doReconnect).
//
// Connect ALWAYS returns a non-nil *ResilientClient when cfg.Endpoint is
// set, mirroring NewResilient's own contract -- even when the returned error
// is non-nil (the initial connect attempt failed). Callers MUST NOT gate
// dependent resolver/reader-factory/client-factory construction on the
// returned error; gate only on cfg.Endpoint being configured at all. The
// returned client keeps retrying in the background and self-heals once the
// gateway becomes reachable -- treating a non-nil initial error as fatal (or
// as license to leave a dependent resolver nil forever) is exactly the bug
// this function exists to prevent (issue #2315).
//
// Authority: ADR-068 decision #11, BR-INTEGRATION-054.
func Connect(ctx context.Context, cfg ConnectConfig, logger logr.Logger) (*ResilientClient, error) {
	var opts []Option
	if cfg.OAuth2.Enabled {
		reloadCfg := ReloadableOAuth2Config{
			TokenURL:         cfg.OAuth2.TokenURL,
			ClientIDPath:     cfg.CredentialsBasePath + "/client-id",
			ClientSecretPath: cfg.CredentialsBasePath + "/client-secret",
			Scopes:           DefaultFleetScopes(cfg.OAuth2.Scopes),
			TokenTimeout:     10 * time.Second,
			TlsCaFile:        cfg.OAuth2.TLSCAFile,
		}
		opts = append(opts, WithReloadableOAuth2Transport(reloadCfg, logger)) //nolint:contextcheck // OAuth2 token source refresh runs as a background reload, independent of any single request
	}

	resilienceCfg := ResilienceConfigFromFleet(cfg.Resilience)
	return NewResilient(ctx, cfg.Endpoint, resilienceCfg, logger, opts...)
}
