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
// A fleet-target investigation (clusterID != "") arriving at a KA instance
// with no FleetOverlayResolver configured at all (inv.fleetOverlayResolver
// == nil, i.e. fleet mode isn't wired on this instance) is a DIFFERENT,
// degraded condition — distinct from the hub-local no-op above — and is
// logged plus recorded as an EventTypeFleetOverlayUnavailable audit event
// (issue #1768 follow-up). Before this, the two cases were indistinguishable
// from the outside: no log, no audit event, no way to tell "a fleet-scoped
// investigation silently ran against local/hub tools" apart from "this
// investigation never had a target cluster" or even "prescopeFleetOverlay
// was never reached" (e.g. a regression removing the call).
//
// On resolver error (resolver present but Overlay itself fails), the
// investigation fails open: it proceeds with ctx unchanged (no overlay),
// behaving like a hub-local investigation minus remote-cluster tool access,
// rather than aborting the investigation over a degraded fleet dependency
// (GA Readiness Dimension 12: no silent failures). The failure is both
// logged and recorded as an EventTypeFleetOverlayFailed audit event (AU-3)
// carrying clusterID and correlationID, so a degraded fleet investigation is
// independently queryable, not just grep-able from logs.
//
// On success, ctx also gains audit.WithClusterID(ctx, clusterID) — this is
// the same context.WithClusterID session.Manager's attachInvestigationContext
// already sets for callers that go through it (see the package-level
// doc/ADR-068 decision #11), applied here too so cluster attribution for
// every audit event downstream of this call (e.g. alignment.SubmitToolStep's
// attributionClusterID) is guaranteed correct even for callers that invoke
// Investigate() directly without going through session.Manager (as KA's own
// integration tests do).
func (inv *Investigator) prescopeFleetOverlay(ctx context.Context, clusterID, correlationID string) context.Context {
	if clusterID == "" {
		return ctx
	}
	if inv.fleetOverlayResolver == nil {
		inv.logger.Info("fleet-target investigation reached prescopeFleetOverlay but no FleetOverlayResolver "+
			"is configured on this KA instance; proceeding without remote-cluster tools",
			"cluster_id", clusterID,
		)
		inv.emitFleetOverlayUnavailableAudit(ctx, clusterID, correlationID)
		return ctx
	}
	overlay, err := inv.fleetOverlayResolver.Overlay(ctx, clusterID)
	if err != nil {
		inv.logger.Error(err, "fleet tool overlay resolution failed; investigation proceeds without remote-cluster tools",
			"cluster_id", clusterID,
		)
		inv.emitFleetOverlayFailedAudit(ctx, clusterID, correlationID, err)
		return ctx
	}
	ctx = audit.WithClusterID(ctx, clusterID)
	return WithFleetOverlay(ctx, overlay)
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
