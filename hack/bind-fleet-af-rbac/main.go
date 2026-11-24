// Command bind-fleet-af-rbac creates the per-tool
// kubernaut-tool-<persona> ClusterRoles and their ClusterRoleBindings and
// binds them to the OIDC groups issued by the fleet-core Keycloak realm's
// "kubernaut-console" client (test/infrastructure/keycloak-realm-demo.json's
// "sre" group/user).
//
// #2702 (kubernaut-operator#682): the operator no longer renders persona
// ClusterRoles or user-facing ClusterRoleBindings, so this command -- via
// infrastructure.BindAFPersonaRBAC / PersonaToolClusterRolesYAML -- owns the
// full persona tool RBAC provisioning path. The coarse-grained
// kubernaut-console-access ClusterRole is NOT handled here: the AF-only E2E
// lane binds it via the harness-owned e2e-user-rbac.yaml fixture, and
// operator-managed-lane console-access provisioning is tracked separately
// (issue #2703 territory).
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
	flag.Parse()
	kubeconfigPath := flag.Lookup("kubeconfig").Value.String()

	if kubeconfigPath == "" {
		fmt.Fprintln(os.Stderr, "❌ -kubeconfig is required (e.g. ~/.kube/fleet-e2e-config)")
		os.Exit(1)
	}

	ctx := context.Background()
	if err := infrastructure.BindAFPersonaRBAC(ctx, kubeconfigPath, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ bind-fleet-af-rbac failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("\n✅ AF persona tool RBAC (ClusterRoles + bindings) bound to the Keycloak \"sre\" group.")
}
