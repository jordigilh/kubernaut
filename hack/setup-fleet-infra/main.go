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
// fleet + Kubernaut stack: pass your own LLM credentials and (optionally)
// Rego policy files -- both legitimately vary per setup and don't belong
// hardcoded into shared test-infra Go code.
//
// Shares its provisioning logic with the "fleet" E2E suite via
// infrastructure.SetupFleetCoreInfrastructureWithGateway
// (test/infrastructure/fleet_e2e.go) -- see that function's doc comment for
// the full design rationale. The helm install itself is
// infrastructure.InstallFleetDemoHelmChart (test/infrastructure/fleet_demo_helm.go).
//
// The cluster is left running: tear it down manually with
// `kind delete cluster --name <cluster-name>` (and the "-remote" sibling)
// when done.
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
	clusterName := flag.String("cluster-name", "fleet-e2e", "Kind cluster name for the hub cluster (the spoke cluster is named <cluster-name>-remote)")
	gatewayTypeFlag := flag.String("gateway-type", string(registry.GatewayEAIGW), "MCP Gateway implementation to deploy: \"eaigw\" or \"kuadrant\" (kuadrant has a known broker-credential SPOF, issue #2309)")
	autonomous := flag.Bool("autonomous", false, "enable Gateway (gateway.enabled=true) so demo alerts auto-remediate; default false is Console-first, so you drive the first investigation yourself")
	llmProvider := flag.String("llm-provider", "", "required: global.llmProfiles.primary.provider (e.g. openai_compatible)")
	llmModel := flag.String("llm-model", "", "required: global.llmProfiles.primary.model")
	llmEndpoint := flag.String("llm-endpoint", "", "required: global.llmProfiles.primary.endpoint")
	llmAPIKeyFile := flag.String("llm-api-key-file", "", "required: path to a file containing your LLM API key (never pass the key inline)")
	spPolicyFile := flag.String("sp-policy-file", "", "optional: SignalProcessing Rego policy file (default: a permissive built-in demo policy)")
	aaPolicyFile := flag.String("aa-policy-file", "", "optional: AIAnalysis Rego policy file (default: a permissive built-in demo policy)")
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
		Autonomous:    *autonomous,
		LLMProvider:   *llmProvider,
		LLMModel:      *llmModel,
		LLMEndpoint:   *llmEndpoint,
		LLMAPIKeyFile: *llmAPIKeyFile,
		SPPolicyFile:  *spPolicyFile,
		AAPolicyFile:  *aaPolicyFile,
	}
	if err := demoOpts.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	fleetOpts, _, err := infrastructure.SetupFleetCoreInfrastructureWithGateway(ctx, *clusterName, kubeconfigPath, gatewayType, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ setup-fleet-infra failed: %v\n", err)
		os.Exit(1)
	}

	if err := infrastructure.InstallFleetDemoHelmChart(ctx, kubeconfigPath, fleetOpts, demoOpts, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ helm install failed: %v\n", err)
		os.Exit(1)
	}
}
