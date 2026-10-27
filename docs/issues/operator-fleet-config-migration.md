# Historical note: kubernaut-operator Fleet configuration migration

**Status**: Superseded by the operator-native `Kubernaut` CR contract
**Date**: 2026-06-20
**Origin**: ADR-068 Fleet Federation Architecture refactoring

## Summary

This note originally tracked copying Fleet fields from the application Helm chart into
the Kubernaut Operator's managed Helm release. That deployment model is retired.

The supported model is now:

1. Install the `kubernaut-operator` Helm bootstrap chart.
2. Apply a user/GitOps-owned `kubernaut.ai/v1alpha2` `Kubernaut` resource.
3. Configure Fleet through the CR's `spec.fleet` and
   `spec.workflowExecution.fleet` fields.
4. Let the operator render the managed service configuration and workload resources.

There is no supported application Helm release for the operator to generate or upgrade,
and there is no Helm-values compatibility contract to preserve in this repository.

## Current Fleet contract

The authoritative field names, validation, and read/write credential separation live in
the operator repository's CRD and installation documentation. In particular, the
WorkflowExecution Fleet credential reference remains separate from the shared read-only
Fleet credential reference to preserve least privilege.

See:

- [Kubernaut Operator installation guide](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md)
- [Fleet setup guide](../operations/deployment/FLEET_SETUP_GUIDE.md)
- [ADR-068: Fleet Federation Architecture](../architecture/decisions/ADR-068-fleet-federation-architecture.md)

The old chart field examples below the original version of this document are intentionally
not reproduced: they described a retired deployment API and could be mistaken for current
operator configuration.
