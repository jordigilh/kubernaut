// Command setup-fleet-infra creates the hub+spoke Kind cluster pair and
// deploys ONLY the fleet-core infrastructure (Keycloak IdP, an MCP Gateway --
// Kuadrant by default, or Envoy AI Gateway via -gateway-type=eaigw --
// kube-mcp-server, the genuinely separate remote/spoke cluster) that the
// "fleet" E2E suite (test/e2e/fleet) also uses, plus Traefik (an Ingress
// controller the Ginkgo suite doesn't need, since it never opens a browser
// against Console) -- but stops there. It does NOT build/pull any Kubernaut
// service image and does NOT `helm install charts/kubernaut`.
//
// Use this (`make setup-e2e-fleet-infra`) when you want a live fleet-core
// stack to `helm install charts/kubernaut` against yourself, once, with your
// own LLM credentials and Rego policy files -- both of which legitimately
// vary per setup and don't belong hardcoded into shared test-infra Go code.
//
// Shares its provisioning logic with the "fleet" E2E suite via
// infrastructure.SetupFleetCoreInfrastructureWithGateway
// (test/infrastructure/fleet_e2e.go) -- see that function's doc comment for
// the full design rationale.
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
	gatewayTypeFlag := flag.String("gateway-type", string(registry.GatewayKuadrant), "MCP Gateway implementation to deploy: \"kuadrant\" or \"eaigw\"")
	// Issue #2333: some E2E demo scenarios (pending-taint, pdb-deadlock,
	// autoscale, node-notready) taint/drain/pressure-test a worker node
	// distinct from the control plane, which the spoke can't provide by
	// default (control-plane-only). Only takes effect on first creation --
	// Kind can't add nodes to an already-running cluster, so requesting
	// workers against an existing single-node spoke requires deleting it
	// first (`kind delete cluster --name <cluster-name>-remote`).
	spokeWorkers := flag.Int("spoke-workers", 0, "number of extra worker nodes to add to the spoke cluster (default 0: control-plane-only, current behavior)")
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

	ctx := context.Background()
	if _, _, err := infrastructure.SetupFleetCoreInfrastructureWithGateway(ctx, *clusterName, kubeconfigPath, gatewayType, *spokeWorkers, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ setup-fleet-infra failed: %v\n", err)
		os.Exit(1)
	}
}
