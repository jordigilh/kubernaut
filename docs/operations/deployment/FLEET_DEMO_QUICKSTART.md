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

This creates the hub + spoke Kind clusters, Keycloak, MCP Gateway, kube-mcp-server,
and fleet-wide monitoring; once the hub cluster exists, it writes
`LLM_CREDENTIALS_FILE`'s contents into the `llm-credentials-primary` Secret (there's no
way to do this before the cluster exists to hold it); then it runs
`helm install charts/kubernaut` itself — remaining Secrets, default Rego policies, and
RBAC included — and finishes by printing a Console URL, a login, and a suggested
`/etc/hosts` line (~15 min total).

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
