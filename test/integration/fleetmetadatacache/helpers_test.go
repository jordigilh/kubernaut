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

package fleetmetadatacache_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

func createBackend(ctx context.Context, name string) {
	GinkgoHelper()
	backend := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.envoyproxy.io/v1alpha1",
			"kind":       "Backend",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "default",
				"labels": map[string]interface{}{
					"kubernaut.ai/managed": "true",
				},
			},
		},
	}
	_, err := dynClient.Resource(registry.BackendGVR).Namespace("default").Create(ctx, backend, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "Backend %s should be created in envtest", name)
}

func deleteBackend(ctx context.Context, name string) {
	_ = dynClient.Resource(registry.BackendGVR).Namespace("default").Delete(ctx, name, metav1.DeleteOptions{})
}

// localAlwaysFalse is a stub local scope checker that always returns false,
// isolating the remote (Valkey/FMC) path under test.
type localAlwaysFalse struct{}

func (l *localAlwaysFalse) IsManagedResource(_ context.Context, _ scope.ResourceIdentity) (bool, error) {
	return false, nil
}

// Issue #1993 (ADR-068 gap closure, IA-2/AC-3): test-only helpers for
// minting per-caller ServiceAccounts/tokens and RBAC bindings against
// envtest, mirroring the SecurityTestTokens pattern in
// test/integration/gateway/security_suite_setup_test.go.
//
// test/infrastructure/serviceaccount.go's CreateServiceAccountForHTTPService
// is close but binds the SA to the *service's own* datastorage-tokenreview
// ClusterRole (for validating incoming tokens); these tests instead need
// per-caller SAs that FMC's SAR checks (the GW/RO side of the relationship),
// so a small local helper pair is simpler than adapting it.

// createServiceAccountWithToken creates a ServiceAccount in namespace and
// mints a bearer token for it via the TokenRequest API (K8s 1.24+: token
// Secrets are no longer auto-created).
func createServiceAccountWithToken(ctx context.Context, clientset kubernetes.Interface, namespace, name string) string {
	GinkgoHelper()
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
	_, err := clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "ServiceAccount %s should be created", name)

	tokenResp, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, name, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: ptr.To(int64(3600))},
	}, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "TokenRequest for %s should succeed", name)
	return tokenResp.Status.Token
}

// bindServiceAccountToFMCScopeCheckClient creates the "fmc-scope-check-client-it"
// ClusterRole (rules mirror charts/kubernaut/templates/rbac/fmc-scope-check-client-rbac.yaml's
// fmc-scope-check-client) if it doesn't already exist, then binds
// namespace/saName to it -- the same shape as production's gateway/
// remediationorchestrator-controller ClusterRoleBindings.
func bindServiceAccountToFMCScopeCheckClient(ctx context.Context, clientset kubernetes.Interface, namespace, saName string) {
	GinkgoHelper()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "fmc-scope-check-client-it"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups:     []string{""},
			Resources:     []string{"services"},
			ResourceNames: []string{"fleetmetadatacache-service"},
			Verbs:         []string{"get"},
		}},
	}
	_, err := clientset.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).ToNot(HaveOccurred(), "ClusterRole fmc-scope-check-client-it should be created")
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: saName + "-fmc-scope-check-client-it"},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "fmc-scope-check-client-it"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: saName, Namespace: namespace}},
	}
	_, err = clientset.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).ToNot(HaveOccurred(), "ClusterRoleBinding should be created")
	}
}
