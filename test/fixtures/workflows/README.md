# Test Workflow OCI Images

Minimal OCI images containing `/workflow-schema.yaml` for DataStorage
OCI-based registration (DD-WORKFLOW-017, ADR-043, BR-WORKFLOW-004).

## Structure

Each subdirectory contains a `workflow-schema.yaml` in BR-WORKFLOW-004 format.
A shared `Dockerfile` (using `FROM scratch`) builds minimal images that contain
only the schema file.

## Build and Push

```bash
# Build all workflow images (local only, current arch)
make build-test-workflows

# Build and push to quay.io (multi-arch: amd64 + arm64)
make push-test-workflows

# Override registry (e.g., for CI or ghcr.io)
make push-test-workflows WORKFLOW_REGISTRY=ghcr.io/jordigilh/kubernaut/test-workflows
```

## Images

| Directory | Image Name | ActionType | Used By |
|-----------|-----------|------------|---------|
| `oomkill-increase-memory/` | oomkill-increase-memory | IncreaseMemoryLimits | AA, HAPI |
| `crashloop-config-fix/` | crashloop-config-fix | RestartDeployment | AA, HAPI |
| `node-drain-reboot/` | node-drain-reboot | DrainNode | AA, HAPI |
| `memory-optimize/` | memory-optimize | ScaleReplicas | AA, HAPI |
| `generic-restart/` | generic-restart | RestartPod | AA, HAPI |
| `test-signal-handler/` | test-signal-handler | DeletePod | AA |
| `imagepull-fix-creds/` | imagepull-fix-creds | RollbackDeployment | HAPI |
| `hello-world/` | hello-world | RestartPod | WE |
| `failing/` | failing | RestartPod | WE |
| `oomkill-increase-memory-job/` | oomkill-increase-memory-job | IncreaseMemoryLimits | FullPipeline |
| `crashloop-config-fix-job/` | crashloop-config-fix-job | RestartDeployment | FullPipeline |
| `detected-labels-test/` | detected-labels-test | ScaleReplicas | DS E2E (ADR-043) |
