# Fleet Demo Quick Start (Kind)

**Status**: The former automated demo helper was removed with the application Helm
deployment path. This page points to the supported operator-backed qualification and
user-managed setup flows; it does not provide a working `make setup-*` command.

## Disposable qualification environment

Run the same operator-backed Fleet journey used by CI when you need a disposable
hub-and-spoke Kind environment:

```bash
PRESERVE_E2E_CLUSTER=true make test-e2e-fleet
```

Requirements are `kind`, `kubectl`, `podman`, an LLM test configuration, and the
resources documented by the [Fleet Setup Guide](FLEET_SETUP_GUIDE.md). The command
leaves `fleet-e2e` and `fleet-e2e-remote` running after a successful run. Delete both
clusters when finished.

## User-managed deployment

1. Install the [Kubernaut Operator](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md).
2. Install the operator's dependency chart and create the PostgreSQL, Valkey, LLM,
   and fleet credential Secrets. Create the SignalProcessing and AIAnalysis policy
   ConfigMaps referenced by `spec.signalProcessing.policy.configMapName` and
   `spec.aiAnalysis.policy.configMapName`.
3. Apply a `Kubernaut` CR based on
   [`test/fixtures/operator/kubernaut-fleet.yaml`](../../../test/fixtures/operator/kubernaut-fleet.yaml),
   changing endpoints, namespaces, and Secret names for your cluster.
4. Configure an autonomous or interactive triggering path using the
   [Autonomous Mode](AUTONOMOUS_MODE_SETUP_GUIDE.md),
   [Interactive Mode](INTERACTIVE_MODE_SETUP_GUIDE.md), and
   [Fleet Setup](FLEET_SETUP_GUIDE.md) guides.

The operator is the sole application owner. This repository no longer provides the
former `setup-fleet-demo-infra`, `setup-local-demo-infra`, or
`hack/setup-demo-infra` entry points.
