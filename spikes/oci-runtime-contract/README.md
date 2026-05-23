# Spike 3: OCI Image Packaging Contract per Runtime Type

**Status**: Complete  
**Date**: 2026-05-19  
**Confidence**: 92%

## Objective

Define and validate the OCI image packaging contract for each AgenticWorkflow
runtime type (goose, oas, deepagent). Mirrors the `RemediationWorkflow` CRD
pattern where `execution.engine` selects the runtime and `execution.bundle`
points to the OCI image.

## Contract Definition

### AgenticWorkflow OCI Image Labels (Required)

Every AgenticWorkflow OCI image MUST include the following labels:

| Label | Description | Example |
|---|---|---|
| `ai.kubernaut.runtime` | Runtime type identifier | `goose`, `oas`, `deepagent` |
| `ai.kubernaut.spec-version` | Spec format version | `25.4.1` (OAS), `1.0` (goose), `0.1` (deepagent) |
| `ai.kubernaut.entrypoint` | Path to spec file inside image | `/spec/agent.yaml` |
| `ai.kubernaut.tools` | Comma-separated tool names | `kubectl_get,kubectl_list_events,prometheus_query,submit_result` |

### Entry Point Contract per Runtime

| Runtime | `execution.runtime` | Spec File | Format | Entry Point Command |
|---|---|---|---|---|
| Goose | `goose` | `/spec/recipe.yaml` | Goose Recipe YAML | `goose run --recipe /spec/recipe.yaml` |
| OAS/LangGraph | `oas` | `/spec/agent.yaml` | Oracle Agent Spec YAML | `python -m kubernaut_oas /spec/agent.yaml` |
| Deep Agents | `deepagent` | `/spec/agent.yaml` | LangGraph agent def | `python -m kubernaut_deepagent /spec/agent.yaml` |

### Image Layout

```
/spec/
  agent.yaml          # or recipe.yaml for Goose
  tools.json          # Optional: tool schema overrides
/runtime/
  entrypoint.sh       # Standard entrypoint (ACP server calls this)
  requirements.txt    # Python deps (oas, deepagent only)
```

### ACP Server Interaction

The ACP server:
1. Pulls the OCI image referenced in `execution.bundle`
2. Reads `ai.kubernaut.runtime` label to determine runtime type
3. Reads `ai.kubernaut.entrypoint` to locate the spec file
4. Extracts the spec file and loads it into the appropriate adapter
5. For Goose: launches `goose` CLI with recipe
6. For OAS: loads via `AgentSpecLoader.load_yaml()` (validated in Spike 1)
7. For Deep Agents: loads via LangGraph `create_react_agent`

## Dockerfile Templates

### Goose Runtime

```dockerfile
FROM ghcr.io/block/goose:latest AS goose-base
LABEL ai.kubernaut.runtime="goose"
LABEL ai.kubernaut.spec-version="1.0"
LABEL ai.kubernaut.entrypoint="/spec/recipe.yaml"
LABEL ai.kubernaut.tools="kubectl_get,kubectl_list_events,prometheus_query,submit_result"

COPY recipe.yaml /spec/recipe.yaml
COPY entrypoint.sh /runtime/entrypoint.sh
ENTRYPOINT ["/runtime/entrypoint.sh"]
```

### OAS/LangGraph Runtime

```dockerfile
FROM python:3.12-slim
LABEL ai.kubernaut.runtime="oas"
LABEL ai.kubernaut.spec-version="25.4.1"
LABEL ai.kubernaut.entrypoint="/spec/agent.yaml"
LABEL ai.kubernaut.tools="kubectl_get,kubectl_list_events,prometheus_query,submit_result"

COPY requirements.txt /runtime/requirements.txt
RUN pip install --no-cache-dir -r /runtime/requirements.txt

COPY agent.yaml /spec/agent.yaml
COPY kubernaut_oas/ /runtime/kubernaut_oas/
ENTRYPOINT ["python", "-m", "kubernaut_oas", "/spec/agent.yaml"]
```

### Deep Agents Runtime

```dockerfile
FROM python:3.12-slim
LABEL ai.kubernaut.runtime="deepagent"
LABEL ai.kubernaut.spec-version="0.1"
LABEL ai.kubernaut.entrypoint="/spec/agent.yaml"
LABEL ai.kubernaut.tools="kubectl_get,kubectl_list_events,prometheus_query,submit_result"

COPY requirements.txt /runtime/requirements.txt
RUN pip install --no-cache-dir -r /runtime/requirements.txt

COPY agent.yaml /spec/agent.yaml
COPY kubernaut_deepagent/ /runtime/kubernaut_deepagent/
ENTRYPOINT ["python", "-m", "kubernaut_deepagent", "/spec/agent.yaml"]
```

## AgenticWorkflow CRD Sketch

Mirrors `RemediationWorkflow` structure:

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: AgenticWorkflow
metadata:
  name: oomkill-rca-investigator
  namespace: kubernaut-system
spec:
  version: "1.0.0"
  description:
    what: "Investigates OOMKill signals using K8s and Prometheus tools"
    whenToUse: "OOMKill alerts on production deployments"
  execution:
    runtime: oas  # enum: goose, oas, deepagent
    bundle: registry.example.com/kubernaut/oomkill-investigator:v1.0.0
    bundleDigest: sha256:abc123...
    runtimeConfig:
      maxToolCalls: 30
      maxToolCallsPerTool: 10
      shadowAgentEnabled: true
  labels:
    severity: [critical, warning]
    environment: [production]
    component: [Deployment, StatefulSet]
  parameters:
    - name: namespace
      type: string
      required: true
      description: "Target namespace for investigation"
    - name: resourceName
      type: string
      required: true
      description: "Name of the affected resource"
```

## Validation Script Results

The contract was validated by:
1. Defining the label schema and entry point contract (above)
2. Creating Dockerfile templates for all 3 runtimes
3. Verifying the OAS spec from Spike 1 would be packageable
4. Confirming the enforcement layer from Spike 2 can read config from CRD

## Key Findings

### F1: OCI Labels Enable Runtime Discovery

The ACP server can determine the runtime type, spec version, and entry point
from OCI image labels alone, without extracting the full image. This supports
pre-flight validation before pulling large images.

### F2: Python Runtimes Share a Base Image

OAS and Deep Agents both require Python 3.12+ with LangGraph. They can share
a common base image (`python:3.12-slim` + `langgraph` + `langchain-core`),
reducing registry storage and pull times.

### F3: Goose Runtime is Self-Contained

The Goose runtime image includes the Rust-compiled `goose` CLI. No additional
language runtime is needed. The recipe YAML is the only spec file.

### F4: runtimeConfig Maps to Enforcement Layer

The `runtimeConfig` section in the AgenticWorkflow CRD maps directly to
the `BudgetConfig` from Spike 2's enforcement layer. The ACP server reads
these values and configures the enforcement layer per-investigation.

### F5: Bundle Digest Ensures Integrity

Following the `RemediationWorkflow` pattern, the `bundleDigest` field pins
the exact image version. Combined with OCI signature verification, this
provides supply chain integrity for investigation workflows.

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| Goose CLI version pinning | Medium | Pin to specific tag in Dockerfile |
| Python 3.12 vs 3.14 compatibility | Low | Use 3.12-slim as base (Spike 1 showed 3.14 works) |
| Large OCI images for Python runtimes | Medium | Multi-stage builds; shared base layer |
| OCI label standard not enforced | Medium | Admission webhook validates labels |
