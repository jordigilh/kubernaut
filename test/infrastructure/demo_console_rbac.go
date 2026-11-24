/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package infrastructure

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// demoConsoleRoleBaseNames lists the operator-rendered, deliberately-unbound
// ClusterRoles the demo must bind externally (kubernaut-operator#682
// roles-only model):
//
//   - console-access: the coarse-grained kubernaut.ai/console verb=use gate
//     (#1919). Without an external binding, the console-access authorization
//     check is deny-by-default for every user.
//   - console-reader: the console REST read projection capabilities
//     (kubernaut_read_policy_scope / kubernaut_list_managed_resources /
//     kubernaut_get_managed_resource, #636/DD-662).
//   - console-inventory-reader: the user-side local-target get/list grants for
//     the console inventory projection (#663).
//
// The operator renders these with the CR namespace prefixed
// (<namespace>-console-access etc.); it never renders a user-facing
// ClusterRoleBinding. Binding them to the deployment's IdP groups is the demo
// operator's SOP (BR-PLATFORM-014; FedRAMP AC-6 least privilege -- capability
// SARs stay deny-by-default unless a role is explicitly bound).
var demoConsoleRoleBaseNames = []string{"console-access", "console-reader", "console-inventory-reader"}

// BuildDemoConsoleRBACManifest returns the demo-owned external
// ClusterRoleBindings that bind the operator-rendered console roles in the
// given namespace to the given IdP group. It renders bindings only -- never
// ClusterRoles and never inline rules -- so the demo cannot widen any
// capability beyond what the operator rendered.
func BuildDemoConsoleRBACManifest(namespace, group string) string {
	var b strings.Builder
	for _, base := range demoConsoleRoleBaseNames {
		fmt.Fprintf(&b, `---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: demo-%s-%s-binding
  labels:
    app.kubernetes.io/part-of: kubernaut-e2e
    kubernaut.ai/demo-rbac: "2703"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: %s-%s
subjects:
  - kind: Group
    name: %s
    apiGroup: rbac.authorization.k8s.io
`, base, namespace, namespace, base, group)
	}
	return b.String()
}

// BindDemoConsoleRBAC applies the demo-owned console ClusterRoleBindings and
// waits until the coarse-grained console-access SAR allows the demo user.
// Must run after operator reconciliation -- the ClusterRoles it binds are
// rendered by the operator and do not exist before it. Mirrors
// BindAFPersonaRBAC (which owns the persona tool roles) for the console
// surface; the two are separate because the persona seam is shared with the
// E2E harness lanes (#2702) while the console bindings are demo SOP (#2703).
func BindDemoConsoleRBAC(ctx context.Context, kubeconfigPath, namespace, group string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "  🔑 Binding operator-rendered console roles to demo IdP group %q...\n", group)
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, BuildDemoConsoleRBACManifest(namespace, group)); err != nil {
		return fmt.Errorf("failed to bind demo console RBAC: %w", err)
	}
	return waitForDemoConsoleRBAC(ctx, kubeconfigPath, namespace, group, writer)
}

// demoConsoleSARUser is the demo Keycloak realm's sole human user
// (keycloak-realm-demo.json), a member of the group the console roles are
// bound to. The SAR probes run as this user to prove the bindings actually
// authorize a real demo identity.
const demoConsoleSARUser = "sre-user"

// waitForDemoConsoleRBAC polls SubjectAccessReviews as the demo user until
// both the coarse-grained console-access gate and the console-reader
// capability SAR allow, proving the external bindings took effect (both
// checks are deny-by-default without them). The reader probe also fails fast
// if the operator's rendered role names drift from this harness's contract.
// The 60s deadline is a child context, so a stalled SAR request (not just a
// slow retry loop) is bounded too.
func waitForDemoConsoleRBAC(ctx context.Context, kubeconfigPath, namespace, group string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("create Kubernetes client for demo console RBAC verification: %w", err)
	}
	probes := []authorizationv1.ResourceAttributes{
		{Verb: "use", Group: "kubernaut.ai", Resource: "console"},
		{Verb: "use", Group: "kubernaut.ai", Resource: "tools", Name: "kubernaut_read_policy_scope"},
	}
	pollCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for {
		allowed := true
		for _, probe := range probes {
			result, sarErr := clientset.AuthorizationV1().SubjectAccessReviews().Create(pollCtx, &authorizationv1.SubjectAccessReview{
				Spec: authorizationv1.SubjectAccessReviewSpec{
					User:   demoConsoleSARUser,
					Groups: []string{group},
					ResourceAttributes: &authorizationv1.ResourceAttributes{
						Verb:     probe.Verb,
						Group:    probe.Group,
						Resource: probe.Resource,
						Name:     probe.Name,
					},
				},
			}, metav1.CreateOptions{})
			if sarErr != nil {
				err = sarErr
				allowed = false
				break
			}
			if !result.Status.Allowed {
				err = fmt.Errorf("SubjectAccessReview denied %s/%s%s for group %s: %s",
					probe.Group, probe.Resource, probe.Name, group, result.Status.Reason)
				allowed = false
				break
			}
		}
		if allowed {
			_, _ = fmt.Fprintf(writer, "  ✅ console-access and console-reader SARs verified for %s/%s\n", group, namespace)
			return nil
		}
		if sleepErr := sleepOrDone(pollCtx, 2*time.Second); sleepErr != nil {
			return fmt.Errorf("demo console RBAC SARs did not allow console access within 60s: %w", err)
		}
	}
}
