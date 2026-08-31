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

package investigator

import (
	"context"
	"fmt"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/tools"
)

// FleetOverlayResolver resolves the set of remote-cluster tools KA should
// expose to the LLM for one investigation's target cluster, under the exact
// same generic names the equivalent local K8s tools already use (DD-FLEET-005
// full name transparency). Implemented by cmd/kubernautagent (wrapping a
// fleetclient.GatewayDiscoverer) and injected via Config.FleetOverlayResolver.
//
// nil is the expected value for KA deployments with fleet disabled: Investigate
// never calls Overlay in that case, and every investigation behaves exactly as
// it did before DD-FLEET-005 (zero regression for hub-local investigations).
//
// Authority: DD-FLEET-005, ADR-068 decision #11, BR-INTEGRATION-054/065.
type FleetOverlayResolver interface {
	// Overlay returns a map from generic tool name (e.g. "kubectl_get_by_name")
	// to the tools.Tool implementation that should serve that name for the
	// given target cluster. Called once per investigation, at Investigate()
	// start, from signal.ClusterID — never by the LLM directly (AC-4: the LLM
	// never chooses which cluster's tools it gets; AC-6: it only ever sees the
	// one cluster's tools its own investigation was launched for).
	Overlay(ctx context.Context, clusterID string) (map[string]tools.Tool, error)
}

// fleetOverlayContextKey is an unexported key type so this package's context
// value can never collide with a context key defined elsewhere (100 Go
// Mistakes anti-pattern: never use a built-in type like string as a context key).
type fleetOverlayContextKey struct{}

// WithFleetOverlay returns a derived context carrying the resolved
// per-investigation fleet tool overlay. Mirrors the existing
// audit.WithClusterID context-carrier pattern
// (internal/kubernautagent/audit/emitter.go).
func WithFleetOverlay(ctx context.Context, overlay map[string]tools.Tool) context.Context {
	return context.WithValue(ctx, fleetOverlayContextKey{}, overlay)
}

// FleetOverlayFromContext retrieves the fleet tool overlay from ctx, if any.
// Returns (nil, false) for hub-local investigations (no overlay was ever
// stored, or Overlay returned an empty map) so callers fall back to the
// shared local tool registry unchanged — zero behavior change for non-fleet
// deployments and for investigations whose target cluster published no tools.
func FleetOverlayFromContext(ctx context.Context) (map[string]tools.Tool, bool) {
	overlay, ok := ctx.Value(fleetOverlayContextKey{}).(map[string]tools.Tool)
	if !ok || len(overlay) == 0 {
		return nil, false
	}
	return overlay, true
}

// prescopeFleetOverlay resolves the fleet tool overlay for clusterID and
// returns a derived context carrying it, for a fleet-target investigation
// (clusterID != ""). For a hub-local investigation (clusterID == ""), ctx is
// returned unchanged with no observable side effect at all — pre-scoping is
// skipped entirely, not attempted and discarded, so hub-local investigations
// never pay the resolver's cost and this remains a true zero-regression
// no-op for the overwhelming majority of (non-fleet) deployments.
//
// FAIL-CLOSED for fleet-target investigations (amendment 2026-08-30,
// supersedes ADR-068 decision #11's original fail-open language; see
// DD-FLEET-005 "Amendment: fail-closed tool-overlay resolution"). Required
// by OWASP ASVS 4.0.3 V4.1.5 ("access controls fail securely including when
// an exception occurs") — the tool overlay IS the access-control boundary
// that scopes which cluster's resources an investigation can read (AC-4), so
// a resolver exception must deny/abort, not silently fall through to a
// different (wrong) cluster's access. A
// fleet-target investigation (clusterID != "") whose overlay cannot be
// resolved — either because no FleetOverlayResolver is configured on this KA
// instance at all (EventTypeFleetOverlayUnavailable), or because a
// configured resolver's Overlay() call itself fails, e.g. the MCP
// gateway/kube-mcp-server is unreachable (EventTypeFleetOverlayFailed) —
// now returns a non-nil error instead of silently continuing with the local
// tool registry. Falling back to local tools is never correct here: the
// local/hub cluster is never the resource the operator or the firing signal
// actually targeted, so any tool call the LLM makes without the overlay
// queries a namespace/pod/deployment on the WRONG cluster. Confirmed live
// 2026-08-30 (Issue #2312): a real investigation of a remote-cluster pod
// fell back to hub-only tools after an EAIGW SSE tools/list failure, got
// clean "not found" responses from the hub (which correctly has no such
// namespace), and the LLM confidently concluded the incident was
// "resolved/stale" — a fabricated verdict with no indication anywhere in the
// response that the investigation never actually reached the target
// cluster. Had the hub coincidentally had a similarly-named resource, the
// same fail-open path could just as easily have produced a confident wrong
// RCA pointing at real (but wrong-cluster) evidence, risking an automated
// remediation action against the wrong cluster entirely. Both failure modes
// are still logged and audited exactly as before (AU-3) — the only change
// is that the investigation now stops instead of proceeding on tools that
// cannot answer for the cluster it was asked about.
//
// On success, ctx also gains audit.WithClusterID(ctx, clusterID) — this is
// the same context.WithClusterID session.Manager's attachInvestigationContext
// already sets for callers that go through it (see the package-level
// doc/ADR-068 decision #11), applied here too so cluster attribution for
// every audit event downstream of this call (e.g. alignment.SubmitToolStep's
// attributionClusterID) is guaranteed correct even for callers that invoke
// Investigate() directly without going through session.Manager (as KA's own
// integration tests do).
func (inv *Investigator) prescopeFleetOverlay(ctx context.Context, clusterID, correlationID string) (context.Context, error) {
	if clusterID == "" {
		return ctx, nil
	}
	if inv.fleetOverlayResolver == nil {
		inv.logger.Error(nil, "fleet-target investigation reached prescopeFleetOverlay but no FleetOverlayResolver "+
			"is configured on this KA instance; failing closed rather than falling back to local/hub tools",
			"cluster_id", clusterID,
		)
		inv.emitFleetOverlayUnavailableAudit(ctx, clusterID, correlationID)
		return ctx, fmt.Errorf("fleet tool overlay unavailable for cluster %q: no FleetOverlayResolver configured "+
			"on this kubernaut-agent instance", clusterID)
	}
	overlay, err := inv.fleetOverlayResolver.Overlay(ctx, clusterID)
	if err != nil {
		inv.logger.Error(err, "fleet tool overlay resolution failed; failing closed rather than falling back to local/hub tools",
			"cluster_id", clusterID,
		)
		inv.emitFleetOverlayFailedAudit(ctx, clusterID, correlationID, err)
		return ctx, fmt.Errorf("fleet tool overlay unavailable for cluster %q: %w", clusterID, err)
	}
	ctx = audit.WithClusterID(ctx, clusterID)
	return WithFleetOverlay(ctx, overlay), nil
}

// emitFleetOverlayFailedAudit records the AU-3 audit event for a failed fleet
// tool overlay resolution (see prescopeFleetOverlay). Best-effort: an audit
// store failure must never turn an already-fail-open degradation into a
// investigation-aborting error.
func (inv *Investigator) emitFleetOverlayFailedAudit(ctx context.Context, clusterID, correlationID string, resolveErr error) {
	event := audit.NewEvent(audit.EventTypeFleetOverlayFailed, correlationID)
	event.EventAction = audit.ActionFleetOverlayFailed
	event.EventOutcome = audit.OutcomeFailure
	event.ClusterID = clusterID
	event.Data["cluster_id"] = clusterID
	event.Data["error_message"] = resolveErr.Error()
	audit.StoreBestEffort(ctx, inv.auditStore, event, inv.auditLog())
}

// emitFleetOverlayUnavailableAudit records the AU-3/AC-4 audit event for a
// fleet-target investigation that reached prescopeFleetOverlay with no
// FleetOverlayResolver configured at all (see prescopeFleetOverlay).
// Best-effort, same rationale as emitFleetOverlayFailedAudit.
func (inv *Investigator) emitFleetOverlayUnavailableAudit(ctx context.Context, clusterID, correlationID string) {
	event := audit.NewEvent(audit.EventTypeFleetOverlayUnavailable, correlationID)
	event.EventAction = audit.ActionFleetOverlayUnavailable
	event.EventOutcome = audit.OutcomeFailure
	event.ClusterID = clusterID
	event.Data["cluster_id"] = clusterID
	event.Data["reason"] = "no FleetOverlayResolver configured on this kubernaut-agent instance"
	audit.StoreBestEffort(ctx, inv.auditStore, event, inv.auditLog())
}

// resolveTool looks up name in the fleet tool overlay. A hit means the LLM's
// generic tool name resolves to a remote cluster's BridgeTool for this
// investigation; a miss tells the caller to fall back to the local tool
// registry unchanged. The same generic name resolves to a different backing
// implementation depending on the investigation's target cluster, never to a
// different schema or a different name — the LLM's tool-calling behavior
// never depends on fleet topology (AC-6, DD-FLEET-005 Alternative 1).
func resolveTool(overlay map[string]tools.Tool, name string) (tools.Tool, bool) {
	if overlay == nil {
		return nil, false
	}
	t, ok := overlay[name]
	return t, ok
}
