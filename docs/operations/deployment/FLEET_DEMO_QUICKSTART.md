# Fleet Demo Quick Start (Kind)

**Status**: Self-contained, automated walkthrough — spins up a hub+spoke Kind
environment and installs Kubernaut, ready to browse, in about 15 minutes.
**Audience**: Anyone trying Kubernaut for the first time who doesn't have a cluster yet.
**Requirements**: an LLM provider API key, ~16 GB RAM, 4 CPU cores, ~50 GB disk (this
repo's own `test-e2e-fleet` CI lane runs the same two-cluster topology on that
footprint).

For a walkthrough that explains every moving part instead of running it as a black box
(or a manual, non-Kind topology), see the [Fleet Setup Guide](FLEET_SETUP_GUIDE.md).
Already have your own cluster and want full control over every Helm value instead of
the Kind automation below? Use the Helm chart's own
[Quick Start](../../../charts/kubernaut/README.md#quick-start) instead.

---

## Set up a fleet demo environment in Kind

**Prerequisites**: `kind`, `kubectl`, `helm`, `podman` (this repo's Kind clusters use
podman as the container runtime — `export KIND_EXPERIMENTAL_PROVIDER=podman`), and an
API key from an LLM provider.

The LLM credential is the only thing you provide by hand, and even that is just a file
path — everything else (Secrets, Rego policies, RBAC) is generated for you.

**1. Put your LLM credential in a file.** A plain API key for most providers, or a
service-account/ADC JSON blob for `vertex_ai`:

```bash
echo -n "<your-llm-api-key>" > /tmp/llm-credentials
```

**2. Run the setup:**

```bash
make setup-fleet-demo-infra \
  LLM_PROVIDER=openai_compatible \
  LLM_MODEL=gpt-4o \
  LLM_ENDPOINT=https://api.openai.com/v1 \
  LLM_CREDENTIALS_FILE=/tmp/llm-credentials
```

### OpenAI reasoning models

For a first-party OpenAI reasoning model, use `LLM_PROVIDER=openai`. The demo infers
conservative defaults for older `gpt-5*` and `o1`/`o3`/`o4` models. GPT model IDs at version
5.6 or newer use `enabled: true` with `effort: none`, independent of their variant suffix. This
keeps the demo's Chat Completions function-tool requests compatible with current 5.6+/GPT-6
models:

```bash
make setup-fleet-demo-infra \
  LLM_PROVIDER=openai \
  LLM_MODEL=gpt-6-luna \
  LLM_ENDPOINT=https://api.openai.com/v1 \
  LLM_CREDENTIALS_FILE=/tmp/llm-credentials
```

The equivalent user-applied CR fields are:

```yaml
spec:
  llmProfiles:
    primary:
      provider: openai
      model: gpt-6-luna
      endpoint: https://api.openai.com/v1
      credentialsSecretName: llm-credentials-primary
      reasoning:
        enabled: true
        effort: none
```

The demo writes this contract to a user-owned `Kubernaut` CR and applies it after
the operator bootstrap and dependency chart are ready. There is no live
`charts/kubernaut` application release.

```bash
kubectl apply -f kubernaut-cr.yaml
```

To override the demo decision, set `LLM_REASONING_ENABLED=true|false` and/or
`LLM_REASONING_EFFORT`. These are also rendered into the shared `primary` profile consumed
by both API Frontend and Kubernaut Agent. For `openai_compatible`, reasoning is never inferred
from a GPT-like model name; set the overrides explicitly only when the endpoint supports them.

The OpenAI Chat Completions mapping is identity-based: `none`, `minimal`, `low`, `medium`,
`high`, `xhigh`, and `max` become the same `reasoning_effort` value. The provider-supported
subset remains model-specific; for example, base `gpt-5` documents `minimal` through `high`,
while `gpt-5.6-luna` documents `none`, `low`, `medium`, `high`, `xhigh`, and `max`. After
changing Helm values, restart the API Frontend and Kubernaut Agent deployments because their
LLM clients are constructed at startup.

GPT-6 Astra does not support function calling through Chat Completions, so it is not compatible
with Kubernaut's current function-tool path; use a tool-compatible GPT-6 variant such as Sol or
Luna instead.

To deploy images from a development repository instead of the default
`quay.io/kubernaut-ai/`, add `IMAGE_REPOSITORY` and `IMAGE_TAG`, for example:

```bash
make setup-fleet-demo-infra \
  LLM_PROVIDER=openai_compatible \
  LLM_MODEL=gpt-4o \
  LLM_ENDPOINT=https://api.openai.com/v1 \
  LLM_CREDENTIALS_FILE=/tmp/llm-credentials \
  IMAGE_REPOSITORY=quay.io/jordigilh \
  IMAGE_TAG=dev
```

`IMAGE_REPOSITORY` is the common base path without the service name. The operator
demo resolves the matching prebuilt operand image for each service and passes the
complete related-image map to the bootstrap chart.
When `IMAGE_TAG` is provided, it is used unchanged as the shared image tag. Published
release tags resolve through multi-architecture manifests; architecture-suffixed tags
are only intermediate build references.

For images loaded from this repository's CI artifacts, use `IMAGE_REPOSITORY=localhost`.
`IMAGE_TAG` should be the base tag from the CI run (for example, `pr-123` or
`main-<sha>`). `download-images.sh --load` and the fleet demo use the same shared tag,
then the setup transfers those local images into the hub Kind nodes before the
operator bootstrap runs.
On ARM64, the CI artifact download omits `db-migrate`; build that one image locally with
`IMAGE_TAG=<base> IMAGE_ARCH=arm64` and tag it with the shared `<base>` tag before loading.

The EAIGW demo uses Agent Router v1.2.0 with Envoy Gateway v1.9.2 and Gateway API v1.6.1
by default. Override the two Helm versions without creating a Kubernaut release when testing
another compatible upstream matrix:

```bash
make setup-fleet-demo-infra \
  LLM_PROVIDER=openai_compatible \
  LLM_MODEL=gpt-4o \
  LLM_ENDPOINT=https://api.openai.com/v1 \
  LLM_CREDENTIALS_FILE=/tmp/llm-credentials \
  ENVOY_GATEWAY_VERSION=v1.9.2 \
  AGENT_ROUTER_VERSION=v1.2.0
```

The setup creates or reuses `mcp-session-seed` in `envoy-ai-gateway-system` so repeated
Helm/GitOps renders do not silently rotate MCP session encryption. For an existing v1.0
installation, use the documented one-time legacy-seed fallback during client reconnect.

For repeatable dependency experiments, copy
`hack/setup-demo-infra/demo-dependencies.example.yaml`, edit it, and pass it through the
Make target:

```bash
cp hack/setup-demo-infra/demo-dependencies.example.yaml /tmp/demo-dependencies.yaml
make setup-fleet-demo-infra \
  LLM_PROVIDER=openai_compatible LLM_MODEL=gpt-4o \
  LLM_ENDPOINT=https://api.openai.com/v1 \
  LLM_CREDENTIALS_FILE=/tmp/llm-credentials \
  DEPENDENCY_OVERRIDES=/tmp/demo-dependencies.yaml
```

Explicit `ENVOY_GATEWAY_VERSION` or `AGENT_ROUTER_VERSION` Make variables override values
from the YAML file.

For Claude on Vertex AI instead, point the credential file at a GCP
service-account key JSON and pass the Vertex project/location (required with
`LLM_PROVIDER=vertex_ai`; the endpoint is the regional host — the Vertex
endpoint itself is derived from project/location):

```bash
make setup-fleet-demo-infra \
  LLM_PROVIDER=vertex_ai \
  LLM_MODEL=claude-haiku-4-5@20251001 \
  LLM_CREDENTIALS_FILE=~/.config/gcloud/kubernaut-fleet-demo-vertexai-key.json \
  VERTEX_PROJECT=<GCP_PROJECT_ID> \
  VERTEX_LOCATION=us-central1
```

No `LLM_ENDPOINT`: every Vertex consumer derives the endpoint from
project/location (or honors the SDK default), so the flag is not required
with `LLM_PROVIDER=vertex_ai` (issue #2355).

This creates the hub + spoke Kind clusters, Keycloak, MCP Gateway, hub-local and
spoke `kube-mcp-server` instances, the `hub` and `remote-cluster` registrations,
and fleet-wide monitoring; once the hub cluster exists, it writes
`LLM_CREDENTIALS_FILE`'s contents into the user-owned `llm-credentials-primary` Secret
(there's no way to do this before the cluster exists to hold it); then it installs the
pinned operator bootstrap/dependency charts, applies the user-owned Kubernaut CR, and
finishes by printing a Console URL, a login, and a suggested
`/etc/hosts` line (~15 min total).

The demo keeps Kubernaut workloads in `kubernaut-system` and places the hub MCP
Gateway/server and Gateway registrations in the dedicated `mcp-system` namespace,
matching the production boundary. The remote MCP server independently defaults to
`mcp-system`. Override either namespace with `MCP_GATEWAY_NAMESPACE` or
`REMOTE_MCP_SERVER_NAMESPACE` when invoking the Make target; the generated Kubernaut
CR receives the matching `spec.fleet.mcpGateway.namespace` value.

Gateway (autonomous, alert-driven remediation) starts **disabled**. Alerts still fire
in Prometheus, but nothing auto-remediates until you ask Console to investigate — so
you see how Kubernaut reasons about a problem before opting into the fully autonomous
flow. Add `AUTONOMOUS=true` to the command above to enable Gateway from the start.

The default Rego policies are a catch-all: SignalProcessing classifies any signal
instead of rejecting unrecognized ones, and AIAnalysis always requires human approval
before a workflow executes. Pass `SP_POLICY_FILE=`/`AA_POLICY_FILE=` to use your own.

**3. Add the printed line to your own `/etc/hosts`.** Modifying a system file needs your
own privileges, so nothing here does it for you — copy the line the command above
printed and add it yourself (`sudo sh -c 'echo "..." >> /etc/hosts'` or your editor of
choice).

**4. Proceed to [Run a scenario](#run-a-scenario).**

## Run a scenario

Once Kubernaut is installed, see it in action. [kubernaut-demo-scenarios](https://github.com/jordigilh/kubernaut-demo-scenarios) provides 37 fault-injection scenarios you can run against your own cluster.

```bash
git clone https://github.com/jordigilh/kubernaut-demo-scenarios.git
cd kubernaut-demo-scenarios
```

New to Kubernaut? Start with [crashloop](https://github.com/jordigilh/kubernaut-demo-scenarios/tree/main/scenarios/crashloop) — the same demo shown at the top of the main [README](../../../README.md). It deploys a misconfigured app and lets it crash-loop:

```bash
export HUB_KUBECONFIG=~/.kube/kubernaut-hub-config
export SPOKE_KUBECONFIG=~/.kube/kubernaut-remote-cluster-config
./scenarios/crashloop/run.sh --fleet --alert-only
```

`--fleet` deploys the workload on the spoke instead of the hub, so Kubernaut
investigates and remediates it remotely across the two clusters `setup-fleet-demo-infra`
created — drop it to run single-cluster instead (the two exports alone no longer
trigger fleet mode). `--alert-only` fires the alert and stops there — exactly what you
want with Gateway disabled. Open Console at the URL `setup-fleet-demo-infra` printed,
log in with the credentials it printed alongside it, and ask it to investigate the
alert; watch it diagnose the crash loop and propose (or, once you approve, apply) a
rollback to the last working revision. If you set `AUTONOMOUS=true` above, drop
`--alert-only` instead and watch Kubernaut detect and roll back automatically, no
prompting needed.
