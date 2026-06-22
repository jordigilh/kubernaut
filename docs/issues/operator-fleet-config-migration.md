# Follow-up: kubernaut-operator Fleet Config Migration

**Date**: 2026-06-20
**Origin**: ADR-068 Fleet Federation Architecture refactoring
**Affects**: kubernaut-operator Helm chart and OLM bundle
**Priority**: Medium (backward-compatible, no immediate breakage)

## Summary

The kubernaut Helm chart (`charts/kubernaut/`) has been updated to support the new
fleet adapter pattern (ADR-068). The `gateway.fleet` and `remediationorchestrator.fleet`
config sections now include two new fields (`backend`, `endpoint`) alongside the
deprecated `valkeyAddr` field.

The kubernaut-operator team needs to:
1. Update the operator's CRD spec to expose the new fleet config fields
2. Update the operator's reconciler to pass these fields through to the managed Helm release
3. Update OLM bundle metadata (CSV, CRD schema) if applicable

## New Fleet Config Fields

### Before (v1.4.x)

```yaml
gateway:
  fleet:
    enabled: false
    valkeyAddr: ""  # direct Valkey connection
```

### After (v1.5.x)

```yaml
gateway:
  fleet:
    enabled: false
    backend: ""     # "fmc", "acm", or "valkey" (legacy)
    endpoint: ""    # backend-specific endpoint URL
    valkeyAddr: ""  # deprecated, kept for backward compat
```

The same change applies to `remediationorchestrator.fleet`.

## Backward Compatibility

- `valkeyAddr` is still supported and will continue to work indefinitely
- If `backend` is empty but `valkeyAddr` is set, the system defaults to `backend: "valkey"`
- If `backend` and `endpoint` are both set, they take precedence over `valkeyAddr`
- No existing deployments will break — the new fields are optional

## Operator CRD Changes Needed

The operator's `Kubernaut` CRD (or equivalent) should add:

```go
type FleetConfig struct {
    Enabled    bool   `json:"enabled,omitempty"`
    Backend    string `json:"backend,omitempty"`    // NEW: "fmc", "acm", "valkey"
    Endpoint   string `json:"endpoint,omitempty"`   // NEW: backend endpoint URL
    ValkeyAddr string `json:"valkeyAddr,omitempty"` // DEPRECATED but kept
}
```

## Operator Reconciler Changes

When reconciling the Helm release, ensure the new fields are passed through:

```go
// If the operator generates values.yaml for Helm:
if fleetCfg.Backend != "" {
    values["gateway"]["fleet"]["backend"] = fleetCfg.Backend
}
if fleetCfg.Endpoint != "" {
    values["gateway"]["fleet"]["endpoint"] = fleetCfg.Endpoint
}
```

## Timeline

- **Immediate**: No action required — existing configs continue to work
- **Next operator release**: Add `backend` and `endpoint` fields to CRD
- **Future**: When `valkeyAddr` is fully deprecated, remove it from the CRD

## References

- ADR-068: Fleet Federation Architecture
- `charts/kubernaut/values.yaml` — fleet config sections
- `charts/kubernaut/values.schema.json` — JSON schema validation
- `charts/kubernaut/templates/gateway/gateway.yaml` — template rendering
- `charts/kubernaut/templates/remediationorchestrator/remediationorchestrator.yaml`
