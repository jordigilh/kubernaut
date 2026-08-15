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
)

// defaultProbeTimeout bounds how long a single Probe call may block waiting
// for DataStorage's health endpoint to respond. Mirrors
// pkg/fleet/readiness.DefaultProbeTimeout's rationale: without a bound, a
// hung DataStorage health check would stall every probe cycle of the
// periodic Gate (including the synchronous first probe on Start).
const defaultProbeTimeout = 10 * time.Second

// DataStorageProber probes DataStorage's real health endpoint (port 8081
// readyz, verifies Postgres connectivity -- see
// pkg/datastorage/server/handlers.go's handleReadiness) so that every
// audit-writing service can gate its own /readyz on DataStorage's actual
// reachability (#1985, BR-AUDIT-005 v2.0). This closes the audit-loss
// window where a service starts serving traffic -- and therefore
// generating audit events -- before DataStorage is reachable: if the pod
// never reports Ready, Kubernetes never routes it traffic in the first
// place, so no audit event is ever lost to a cold-start race.
//
// Implements pkg/fleet/readiness.Prober directly; that interface has no
// Fleet-specific coupling, so DataStorageProber is aggregated into each
// service's own independent, always-on readiness.Gate (distinct from the
// existing Fleet-conditional gate).
type DataStorageProber struct {
	// HealthURL is DataStorage's health-check endpoint (its readyz port,
	// 8081 -- distinct from the main API port 8080 used for audit writes).
	HealthURL string
	// Client performs the HTTP health check. Defaults to an http.Client
	// with defaultProbeTimeout when nil.
	Client *http.Client
}

var _ readiness.Prober = (*DataStorageProber)(nil)

// Probe implements readiness.Prober: nil when DataStorage's health
// endpoint returns any 2xx status, a descriptive error otherwise
// (non-2xx status, connection failure, or timeout).
func (p *DataStorageProber) Probe(ctx context.Context) error {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: defaultProbeTimeout}
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
func NewReadinessGate(ctx context.Context, healthURL string, logger logr.Logger) *readiness.Gate {
	prober := &DataStorageProber{HealthURL: healthURL}
	gate := readiness.NewGate(DefaultReadinessProbeInterval, logger.WithName("datastorage-readiness"), prober)
	gate.Start(ctx)
	logger.Info("DataStorage readiness gate started", "ready", gate.Ready())
	return gate
}
