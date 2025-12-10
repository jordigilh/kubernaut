/*
Copyright 2025 Jordi Gil.

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

package helpers

import (
	"context"
	"fmt"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ServiceAccountHelper provides utilities for managing test ServiceAccounts
type ServiceAccountHelper struct {
	k8sClient  *kubernetes.Clientset
	ctrlClient client.Client
	namespace  string
}

// NewServiceAccountHelper creates a new ServiceAccount helper
func NewServiceAccountHelper(k8sClient *kubernetes.Clientset, ctrlClient client.Client, namespace string) *ServiceAccountHelper {
	return &ServiceAccountHelper{
		k8sClient:  k8sClient,
		ctrlClient: ctrlClient,
		namespace:  namespace,
	}
}

// CreateServiceAccount creates a ServiceAccount for testing
func (h *ServiceAccountHelper) CreateServiceAccount(ctx context.Context, name string) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: h.namespace,
		},
	}

	_, err := h.k8sClient.CoreV1().ServiceAccounts(h.namespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create ServiceAccount %s: %w", name, err)
	}

	return nil
}

// CreateServiceAccountWithRBAC creates a ServiceAccount and binds it to a ClusterRole
func (h *ServiceAccountHelper) CreateServiceAccountWithRBAC(ctx context.Context, name, clusterRoleName string) error {
	// Create ServiceAccount
	if err := h.CreateServiceAccount(ctx, name); err != nil {
		return err
	}

	// Create ClusterRoleBinding
	bindingName := fmt.Sprintf("%s-%s-binding", h.namespace, name)
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: bindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      name,
				Namespace: h.namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
	}

	_, err := h.k8sClient.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create ClusterRoleBinding %s: %w", bindingName, err)
	}

	return nil
}

// GetServiceAccountToken extracts the token for a ServiceAccount
// In Kubernetes 1.24+, tokens are no longer automatically created
// This function creates a token request and returns the token
func (h *ServiceAccountHelper) GetServiceAccountToken(ctx context.Context, name string) (string, error) {
	// Wait for ServiceAccount to be ready
	time.Sleep(2 * time.Second)

	// For K8s 1.24+, use TokenRequest API
	// Empty audience = token valid for any audience (required for Kind clusters)
	// Kind clusters use localhost API server URLs (e.g., https://127.0.0.1:PORT)
	// which don't match the standard "https://kubernetes.default.svc" audience
	expirationSeconds := int64(3600) // 1 hour
	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         []string{}, // Empty = valid for any audience
			ExpirationSeconds: &expirationSeconds,
		},
	}

	result, err := h.k8sClient.CoreV1().ServiceAccounts(h.namespace).CreateToken(
		ctx,
		name,
		treq,
		metav1.CreateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to create token for ServiceAccount %s: %w", name, err)
	}

	if result.Status.Token == "" {
		return "", fmt.Errorf("token is empty for ServiceAccount %s", name)
	}

	return result.Status.Token, nil
}

// DeleteServiceAccount deletes a ServiceAccount
func (h *ServiceAccountHelper) DeleteServiceAccount(ctx context.Context, name string) error {
	err := h.k8sClient.CoreV1().ServiceAccounts(h.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ServiceAccount %s: %w", name, err)
	}

	return nil
}

// DeleteClusterRoleBinding deletes a ClusterRoleBinding
func (h *ServiceAccountHelper) DeleteClusterRoleBinding(ctx context.Context, name string) error {
	bindingName := fmt.Sprintf("%s-%s-binding", h.namespace, name)
	err := h.k8sClient.RbacV1().ClusterRoleBindings().Delete(ctx, bindingName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ClusterRoleBinding %s: %w", bindingName, err)
	}

	return nil
}

// Cleanup removes all test ServiceAccounts and bindings
func (h *ServiceAccountHelper) Cleanup(ctx context.Context, names []string) error {
	for _, name := range names {
		if err := h.DeleteClusterRoleBinding(ctx, name); err != nil {
			return err
		}
		if err := h.DeleteServiceAccount(ctx, name); err != nil {
			return err
		}
	}
	return nil
}

// CreateClusterRoleForTests creates a ClusterRole for Gateway integration tests
// This is needed for Kind clusters where the ClusterRole doesn't exist
// For OCP clusters, the ClusterRole already exists from Gateway deployment
func (h *ServiceAccountHelper) CreateClusterRoleForTests(ctx context.Context, name string) error {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "kubernaut",
				"app.kubernetes.io/component": "gateway",
				"test":                        "integration",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"remediation.kubernaut.ai"},
				Resources: []string{"remediationrequests"},
				Verbs:     []string{"create", "get", "list", "watch", "update", "patch", "delete"},
			},
		},
	}

	_, err := h.k8sClient.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create ClusterRole %s: %w", name, err)
	}

	return nil
}

// DeleteClusterRole deletes a ClusterRole
func (h *ServiceAccountHelper) DeleteClusterRole(ctx context.Context, name string) error {
	err := h.k8sClient.RbacV1().ClusterRoles().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ClusterRole %s: %w", name, err)
	}

	return nil
}
