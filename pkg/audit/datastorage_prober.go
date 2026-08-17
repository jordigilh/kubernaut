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

package audit

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
)

// DefaultProbeTimeout bounds how long a single Probe call may block waiting
// for DataStorage's health endpoint to respond. Mirrors
// pkg/fleet/readiness.DefaultProbeTimeout's rationale: without a bound, a
// hung DataStorage health check would stall every probe cycle of the
// periodic Gate (including the synchronous first probe on Start). Exported
// so callers building their own Client for NewReadinessGateWithClient (e.g.
// cmd/apifrontend, whose DataStorage TLS trust is driven by a config field
// rather than $TLS_CA_FILE) can reuse the same bound.
const DefaultProbeTimeout = 10 * time.Second

// DataStorageProber probes DataStorage's real readiness endpoint (verifies
// Postgres connectivity -- see pkg/datastorage/server/handlers.go's
// handleReadiness) so that every audit-writing service can gate its own
// /readyz on DataStorage's actual reachability (#1985, BR-AUDIT-005 v2.0).
// This closes the audit-loss window where a service starts serving
// traffic -- and therefore generating audit events -- before DataStorage
// is reachable: if the pod never reports Ready, Kubernetes never routes
// it traffic in the first place, so no audit event is ever lost to a
// cold-start race.
//
// DD-PLATFORM-010: HealthURL targets an unauthenticated /readyz route on
// DataStorage's main API port (8080, HTTPS) -- not the dedicated
// kubelet-only health port (8081, HTTP) -- so this reuses the exact same
// handler kubelet's own probe hits, registered a second time at the
// router's top level, outside the DD-AUTH-014 auth middleware group.
//
// Implements pkg/fleet/readiness.Prober directly; that interface has no
// Fleet-specific coupling, so DataStorageProber is aggregated into each
// service's own independent, always-on readiness.Gate (distinct from the
// existing Fleet-conditional gate).
type DataStorageProber struct {
	// HealthURL is DataStorage's cross-service readiness endpoint:
	// https://<service>:8080/readyz (DD-PLATFORM-010; see
	// kubernaut.datastorage.healthUrl in charts/kubernaut/templates/_helpers.tpl).
	HealthURL string
	// Client performs the HTTP health check. NewReadinessGate (the common
	// production constructor, used by 9 of the 10 audit-writing services)
	// always sets this to a CA-aware client via sharedtls.DefaultBaseTransport()
	// -- the same $TLS_CA_FILE-driven CAReloader transport
	// pkg/audit/openapi_client_adapter.go's real audit-write client uses --
	// since HealthURL is HTTPS and, in most deployments, signed by a
	// cluster-local (non-system-trusted) CA. Callers whose DataStorage TLS
	// trust is driven by a different mechanism (e.g. cmd/apifrontend's
	// cfg.Agent.DSTLSCaFile-based tlswiring.CAReloadableTransport, not
	// $TLS_CA_FILE) must use NewReadinessGateWithClient instead and supply
	// their own CA-aware Client. A nil Client here (e.g. direct struct
	// construction in tests) falls back to an http.Client with
	// DefaultProbeTimeout and Go's default transport, which only trusts the
	// system root CA pool.
	Client *http.Client
}

var _ readiness.Prober = (*DataStorageProber)(nil)

// Probe implements readiness.Prober: nil when DataStorage's health
// endpoint returns any 2xx status, a descriptive error otherwise
// (non-2xx status, connection failure, or timeout).
func (p *DataStorageProber) Probe(ctx context.Context) error {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: DefaultProbeTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.HealthURL, nil)
	if err != nil {
		return fmt.Errorf("building DataStorage health check request for %s: %w", p.HealthURL, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("DataStorage health endpoint %s unreachable: %w", p.HealthURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("DataStorage health endpoint %s returned %d", p.HealthURL, resp.StatusCode)
	}
	return nil
}

// DefaultReadinessProbeInterval controls how often a DataStorage
// readiness gate re-probes DataStorage's health endpoint once started.
// Shared by every audit-writing service's cmd/main.go wiring (#1985,
// BR-AUDIT-005 v2.0) to avoid redeclaring this identical constant across
// all 10 services.
const DefaultReadinessProbeInterval = 15 * time.Second

// NewReadinessGate builds and starts a readiness.Gate wrapping a
// DataStorageProber pointed at healthURL, logging its initial state.
// Every service's cmd/main.go previously duplicated this exact
// construct-start-log sequence almost verbatim (REFACTOR, #1985); this is
// the single shared implementation. Only the surrounding cfg-field
// extraction (e.g. cfg.DataStorage.HealthURL vs cfg.Agent.DSHealthURL)
// differs per service, which callers still own. Callers must Stop() the
// returned Gate on shutdown.
//
// DD-PLATFORM-010 follow-up: healthURL moved from plain-HTTP port 8081 to
// HTTPS port 8080, so the probe's http.Client must trust whatever CA
// signed DataStorage's server cert (rarely the system root pool in a
// cluster deployment). newProbeClient wires the same $TLS_CA_FILE-driven
// transport every other DataStorage HTTPS caller uses. Services whose
// DataStorage TLS trust is NOT $TLS_CA_FILE-driven must use
// NewReadinessGateWithClient instead (see DataStorageProber.Client's doc).
func NewReadinessGate(ctx context.Context, healthURL string, logger logr.Logger) *readiness.Gate {
	return NewReadinessGateWithClient(ctx, healthURL, newProbeClient(logger), logger)
}

// NewReadinessGateWithClient is NewReadinessGate with an explicit,
// caller-supplied http.Client instead of the $TLS_CA_FILE-driven default.
// cmd/apifrontend is the sole current caller (2026-08-17 CI RCA, PR #2168):
// unlike the other 9 audit-writing services, its DataStorage-facing TLS
// trust is driven by cfg.Agent.DSTLSCaFile via
// pkg/apifrontend/tlswiring.CAReloadableTransport, a distinct CA from
// whatever $TLS_CA_FILE happens to point at for that service (AF has its
// own, separate CA for its serving cert / other inter-service calls) --
// using the $TLS_CA_FILE-based default here produced a real
// "certificate signed by unknown authority" failure in E2E, permanently
// NotReady.
func NewReadinessGateWithClient(ctx context.Context, healthURL string, client *http.Client, logger logr.Logger) *readiness.Gate {
	prober := &DataStorageProber{HealthURL: healthURL, Client: client}
	gate := readiness.NewGate(DefaultReadinessProbeInterval, logger.WithName("datastorage-readiness"), prober)
	gate.Start(ctx)
	logger.Info("DataStorage readiness gate started", "ready", gate.Ready())
	return gate
}

// newProbeClient builds the http.Client used by the production
// DataStorageProber: a bounded timeout plus sharedtls.DefaultBaseTransport(),
// which honours $TLS_CA_FILE (falling back to a plain transport -- and thus
// the system root CA pool -- when that env var is unset, e.g. plaintext
// local dev). Fails open to an uncustomized http.Client on transport-build
// error (e.g. a malformed CA file) rather than blocking readiness-gate
// construction entirely; the resulting TLS verification failures still
// surface per-probe via Probe's own error return, so this is observable,
// not silent.
func newProbeClient(logger logr.Logger) *http.Client {
	transport, err := sharedtls.DefaultBaseTransport()
	if err != nil {
		logger.Error(err, "failed to build CA-aware transport for DataStorage readiness probe; falling back to the system trust store")
		return &http.Client{Timeout: DefaultProbeTimeout}
	}
	return &http.Client{Timeout: DefaultProbeTimeout, Transport: transport}
}
