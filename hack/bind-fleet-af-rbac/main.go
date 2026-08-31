// Command bind-fleet-af-rbac binds the per-tool kubernaut-tool-<persona>
// ClusterRoles that `helm install charts/kubernaut` creates for API Frontend
// to the OIDC groups issued by the fleet-core Keycloak realm's
// "kubernaut-console" client (test/infrastructure/keycloak-realm-fleet.json's
// "sre" group/user).
//
// The chart deliberately creates only these ClusterRoles, never the
// bindings (it can't presume to know a real deployer's IdP group names) --
// see infrastructure.bindAFPersonaToolClusterRoles's doc comment (Issue
// #1737). The coarse-grained kubernaut-console-access ClusterRole is NOT
// handled here: the chart already binds it directly from
// apifrontend.config.rbac.consoleAccessGroups (default includes "sre"), no
// manual step needed for that one.
// Run this once, AFTER `helm install`, against the fleet-core cluster set up
// by `make setup-e2e-fleet-infra`:
//
//	make bind-fleet-af-rbac KUBECONFIG=~/.kube/fleet-e2e-config
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

func main() {
	// sigs.k8s.io/controller-runtime/pkg/client/config's own init() unconditionally
	// registers a "-kubeconfig" flag on flag.CommandLine the moment anything --
	// direct or transitive -- imports it, which already happened here via the
	// infrastructure package (test/infrastructure/mcp_e2e_helpers.go) before this
	// main() body runs. A plain flag.String("kubeconfig", ...) call therefore
	// panics ("flag redefined"); mirror controller-runtime's own defensive
	// Lookup-before-register pattern (config.go's RegisterFlags) instead of
	// assuming we're the first/only registrant.
	if flag.Lookup("kubeconfig") == nil {
		flag.String("kubeconfig", "", "path to the fleet-core cluster's kubeconfig (required)")
	}
	flag.Parse()
	kubeconfigPath := flag.Lookup("kubeconfig").Value.String()

	if kubeconfigPath == "" {
		fmt.Fprintln(os.Stderr, "❌ -kubeconfig is required (e.g. ~/.kube/fleet-e2e-config)")
		os.Exit(1)
	}

	ctx := context.Background()
	if err := infrastructure.BindFleetAFPersonaRBAC(ctx, kubeconfigPath, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ bind-fleet-af-rbac failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("\n✅ AF persona + console-access RBAC bound to the Keycloak \"sre\" group.")
}
