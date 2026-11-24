// Command bind-fleet-af-rbac provisions demo RBAC on a fleet-core cluster:
// the persona tool ClusterRoles and their group ClusterRoleBindings
// (infrastructure.BindAFPersonaRBAC), and the external bindings for the
// operator-rendered console roles (infrastructure.BindDemoConsoleRBAC,
// bound to the Keycloak realm's "sre" group/user,
// test/infrastructure/keycloak-realm-demo.json). The operator renders roles
// only, so both halves are applied here after operator reconciliation.
//
// Run this once, AFTER the operator bootstrap and Kubernaut CR, against the fleet-core cluster set up
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
	if flag.Lookup("namespace") == nil {
		flag.String("namespace", "kubernaut-system", "namespace of the Kubernaut CR whose operator-rendered console roles should be bound")
	}
	flag.Parse()
	kubeconfigPath := flag.Lookup("kubeconfig").Value.String()
	namespace := flag.Lookup("namespace").Value.String()

	if kubeconfigPath == "" {
		fmt.Fprintln(os.Stderr, "❌ -kubeconfig is required (e.g. ~/.kube/fleet-e2e-config)")
		os.Exit(1)
	}

	ctx := context.Background()
	if err := infrastructure.BindAFPersonaRBAC(ctx, kubeconfigPath, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ bind-fleet-af-rbac failed: %v\n", err)
		os.Exit(1)
	}
	if err := infrastructure.BindDemoConsoleRBAC(ctx, kubeconfigPath, namespace, "sre", os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ bind-fleet-af-rbac failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("\n✅ Demo persona tool roles (per-persona groups) and console-access/console-reader/console-inventory-reader (sre group) bound.")
}
