// Command setup-fleet-infra creates the hub+spoke Kind cluster pair, deploys
// the fleet-core infrastructure (Keycloak IdP, an MCP Gateway -- Envoy AI
// Gateway (EAIGW) by default, or Kuadrant via -gateway-type=kuadrant --
// kube-mcp-server, the genuinely separate remote/spoke cluster, Traefik) that
// the "fleet" E2E suite (test/e2e/fleet) also uses, and then (2026-09-01,
// issue #2337) `helm upgrade --install charts/kubernaut` itself, wired up
// Console-first (BR-PLATFORM-014): Gateway starts disabled so a new user
// investigates/remediates the first demo scenario via Console chat before
// opting into the fully autonomous, Gateway-driven flow with -autonomous.
//
// EAIGW, not Kuadrant, is the default (2026-09-01): issue #2309 found
// Kuadrant's static broker credential (its own tool-discovery connection to
// kube-mcp-server) to be a structural SPOF -- no hot-reload, manual rotation
// only -- which is a bad default for a demo/QE environment meant to run
// unattended for days (BR-PLATFORM-014). EAIGW forwards the caller's own
// token instead of relying on a cached credential. Kuadrant still works and
// remains covered by its own standalone FMC E2E lane; pass
// -gateway-type=kuadrant if you specifically need it.
//
// Use this (`make setup-fleet-demo-infra`) for a one-shot, ready-to-browse
// fleet + Kubernaut stack. Pass -llm-credentials-file pointing at a file
// holding your LLM provider credentials (a plain API key, or a JSON blob
// for vertex_ai) -- the tool writes it into the llm-credentials-primary
// Secret itself once the hub cluster exists (2026-09-01: previously the
// docs asked you to pre-create this Secret before running setup at all,
// which can't work since the cluster meant to hold it doesn't exist yet).
// Everything else (PostgreSQL/Valkey/console-oauth Secrets, a catch-all
// classification policy and an "always require approval" gate for
// SignalProcessing/AIAnalysis Rego) is generated automatically unless you
// pass -sp-policy-file/-aa-policy-file to override.
//
// Shares its provisioning logic with the "fleet" E2E suite via
// infrastructure.SetupFleetCoreInfrastructureWithGateway
// (test/infrastructure/fleet_e2e.go) -- see that function's doc comment for
// the full design rationale. The helm install itself is
// infrastructure.InstallFleetDemoHelmChart (test/infrastructure/fleet_demo_helm.go).
//
// The cluster is left running: tear it down manually with
// `kind delete cluster --name <cluster-name>` (and the "-remote-cluster-name"
// sibling) when done.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

func main() {
	clusterName := flag.String("cluster-name", "kubernaut-hub", "Kind cluster name for the hub cluster")
	remoteClusterName := flag.String("remote-cluster-name", "kubernaut-remote-cluster", "Kind cluster name for the remote/spoke cluster -- matches the \"remote-cluster\" identity every fleet MCPServerRegistration/AlertManager label already uses for it")
	gatewayTypeFlag := flag.String("gateway-type", string(registry.GatewayEAIGW), "MCP Gateway implementation to deploy: \"eaigw\" or \"kuadrant\" (kuadrant has a known broker-credential SPOF, issue #2309)")
	// Issue #2333: some E2E demo scenarios (pending-taint, pdb-deadlock,
	// autoscale, node-notready) taint/drain/pressure-test a worker node
	// distinct from the control plane, which the spoke can't provide by
	// default (control-plane-only). Only takes effect on first creation --
	// Kind can't add nodes to an already-running cluster, so requesting
	// workers against an existing single-node spoke requires deleting it
	// first (`kind delete cluster --name <remote-cluster-name>`).
	spokeWorkers := flag.Int("spoke-workers", 0, "number of extra worker nodes to add to the spoke cluster (default 0: control-plane-only, current behavior)")
	autonomous := flag.Bool("autonomous", false, "enable Gateway (gateway.enabled=true) so demo alerts auto-remediate; default false is Console-first, so you drive the first investigation yourself")
	llmProvider := flag.String("llm-provider", "", "required: global.llmProfiles.primary.provider (e.g. openai_compatible)")
	llmModel := flag.String("llm-model", "", "required: global.llmProfiles.primary.model")
	llmEndpoint := flag.String("llm-endpoint", "", "required: global.llmProfiles.primary.endpoint")
	llmCredentialsFile := flag.String("llm-credentials-file", "", "required: path to a file holding your LLM provider credentials (a plain API key, or a JSON blob for vertex_ai service-account/ADC credentials) -- written verbatim into the llm-credentials-primary Secret once the hub cluster exists, replacing the mock placeholder")
	spPolicyFile := flag.String("sp-policy-file", "", "optional: SignalProcessing Rego policy file (default: a built-in catch-all demo policy)")
	aaPolicyFile := flag.String("aa-policy-file", "", "optional: AIAnalysis Rego policy file (default: a built-in policy that always requires human approval)")
	// sigs.k8s.io/controller-runtime/pkg/client/config's own init() unconditionally
	// registers a "-kubeconfig" flag on flag.CommandLine the moment anything --
	// direct or transitive -- imports it, which already happened here via the
	// infrastructure package (test/infrastructure/mcp_e2e_helpers.go) before this
	// main() body runs. A plain flag.String("kubeconfig", ...) call therefore
	// panics ("flag redefined"); mirror controller-runtime's own defensive
	// Lookup-before-register pattern (config.go's RegisterFlags) instead of
	// assuming we're the first/only registrant.
	if flag.Lookup("kubeconfig") == nil {
		flag.String("kubeconfig", "", "path to write the hub cluster's kubeconfig (default: ~/.kube/<cluster-name>-config)")
	}
	flag.Parse()
	kubeconfigPath := flag.Lookup("kubeconfig").Value.String()

	if kubeconfigPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to resolve home directory: %v\n", err)
			os.Exit(1)
		}
		kubeconfigPath = filepath.Join(homeDir, ".kube", *clusterName+"-config")
	}

	gatewayType := registry.MCPGatewayType(*gatewayTypeFlag)
	if !registry.SupportedGateways[gatewayType] {
		fmt.Fprintf(os.Stderr, "invalid -gateway-type %q; must be one of: eaigw, kuadrant\n", *gatewayTypeFlag)
		os.Exit(1)
	}

	demoOpts := infrastructure.FleetDemoHelmOptions{
		Autonomous:   *autonomous,
		LLMProvider:  *llmProvider,
		LLMModel:     *llmModel,
		LLMEndpoint:  *llmEndpoint,
		SPPolicyFile: *spPolicyFile,
		AAPolicyFile: *aaPolicyFile,
	}
	if err := demoOpts.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
	if *llmCredentialsFile == "" {
		fmt.Fprintln(os.Stderr, "❌ missing required flag: -llm-credentials-file")
		os.Exit(1)
	}

	ctx := context.Background()
	fleetOpts, remoteKubeconfigPath, err := infrastructure.SetupFleetCoreInfrastructureWithGateway(ctx, *clusterName, *remoteClusterName, kubeconfigPath, infrastructure.FleetCoreDemoOptions{
		GatewayType:        gatewayType,
		SpokeWorkers:       *spokeWorkers,
		LLMCredentialsFile: *llmCredentialsFile,
	}, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ setup-fleet-infra failed: %v\n", err)
		os.Exit(1)
	}

	if err := infrastructure.InstallFleetDemoHelmChart(ctx, kubeconfigPath, remoteKubeconfigPath, fleetOpts, demoOpts, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ helm install failed: %v\n", err)
		os.Exit(1)
	}
}
