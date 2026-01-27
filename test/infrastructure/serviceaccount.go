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

package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// CreateE2EServiceAccountWithDataStorageAccess creates a ServiceAccount for E2E tests
// with RBAC permissions to access DataStorage via middleware-based SAR validation.
//
// Authority: DD-AUTH-014 (Middleware-based authentication with Zero Trust)
// Authority: DD-AUTH-010 (E2E Real Authentication Mandate)
//
// This function:
//  1. Creates ServiceAccount in specified namespace
//  2. Creates RoleBinding to data-storage-client ClusterRole (CRUD permissions)
//  3. Returns immediately (tokens retrieved via TokenRequest API)
//
// Parameters:
//   - ctx: Context for cancellation
//   - namespace: Namespace to create ServiceAccount in
//   - kubeconfigPath: Path to kubeconfig file
//   - saName: Name of ServiceAccount to create
//   - writer: Output writer for logs
//
// Returns:
//   - error: Error if creation fails
//
// Usage:
//
//	err := CreateE2EServiceAccountWithDataStorageAccess(ctx, "kubernaut-system", kubeconfigPath, "datastorage-e2e-sa", GinkgoWriter)
func CreateE2EServiceAccountWithDataStorageAccess(ctx context.Context, namespace, kubeconfigPath, saName string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Creating E2E ServiceAccount with DataStorage Access\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "  Namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "  ServiceAccount: %s\n", saName)
	_, _ = fmt.Fprintf(writer, "  ClusterRole: data-storage-client (CRUD: create, get, list, update, delete - DD-AUTH-014)\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// 1. Create ServiceAccount
	_, _ = fmt.Fprintf(writer, "🔐 Creating ServiceAccount: %s\n", saName)
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":       "datastorage-e2e",
				"component": "test",
				"rbac":      "dd-auth-011",
			},
		},
	}

	_, err = clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create ServiceAccount: %w", err)
	}
	if apierrors.IsAlreadyExists(err) {
		_, _ = fmt.Fprintf(writer, "   ℹ️  ServiceAccount already exists\n")
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ ServiceAccount created\n")
	}

	// 2. Create RoleBinding to data-storage-client ClusterRole
	_, _ = fmt.Fprintf(writer, "🔐 Creating RoleBinding: %s-data-storage-client\n", saName)
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-data-storage-client", saName),
			Namespace: namespace,
			Labels: map[string]string{
				"app":       "datastorage-e2e",
				"component": "rbac",
				"test":      "e2e",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "data-storage-client", // DD-AUTH-011: verb:"create"
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: namespace,
			},
		},
	}

	_, err = clientset.RbacV1().RoleBindings(namespace).Create(ctx, rb, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create RoleBinding: %w", err)
	}
	if apierrors.IsAlreadyExists(err) {
		_, _ = fmt.Fprintf(writer, "   ℹ️  RoleBinding already exists\n")
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ RoleBinding created\n")
	}

	// Note: Kubernetes 1.24+ no longer auto-creates token Secrets
	// Tokens are retrieved via TokenRequest API (see GetServiceAccountToken function)
	_, _ = fmt.Fprintf(writer, "✅ ServiceAccount ready for TokenRequest API\n")

	_, _ = fmt.Fprintf(writer, "✅ E2E ServiceAccount ready with DataStorage access\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return nil
}

// GetServiceAccountToken retrieves the Bearer token for a ServiceAccount.
//
// Authority: DD-AUTH-010 (Real token retrieval for E2E)
//
// This function uses kubectl to extract the token from the ServiceAccount's Secret.
// The token is used with ServiceAccountTransport for OAuth2-proxy authentication.
//
// Parameters:
//   - ctx: Context for cancellation
//   - namespace: Namespace containing ServiceAccount
//   - saName: ServiceAccount name
//   - kubeconfigPath: Path to kubeconfig file
//
// Returns:
//   - string: Bearer token (without "Bearer " prefix)
//   - error: Error if retrieval fails
//
// Usage:
//
//	token, err := GetServiceAccountToken(ctx, "kubernaut-system", "datastorage-e2e-sa", kubeconfigPath)
//	transport := auth.NewServiceAccountTransport(token)
func GetServiceAccountToken(ctx context.Context, namespace, saName, kubeconfigPath string) (string, error) {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Kubernetes 1.24+: Use TokenRequest API (token Secrets no longer auto-created)
	// Authority: https://kubernetes.io/docs/reference/kubernetes-api/authentication-resources/token-request-v1/
	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			// Request long-lived token for E2E tests (default is 1 hour)
			ExpirationSeconds: func() *int64 { exp := int64(3600); return &exp }(),
		},
	}

	tokenResp, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(
		ctx,
		saName,
		treq,
		metav1.CreateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to create ServiceAccount token via TokenRequest API: %w", err)
	}

	if tokenResp.Status.Token == "" {
		return "", fmt.Errorf("TokenRequest returned empty token for ServiceAccount %s/%s", namespace, saName)
	}

	return tokenResp.Status.Token, nil
}

// waitForServiceAccountToken waits for ServiceAccount token Secret to be created.
// Kubernetes 1.24+ creates token Secrets automatically, but there's a delay.
func waitForServiceAccountToken(ctx context.Context, clientset *kubernetes.Clientset, namespace, saName string, writer io.Writer) error {
	timeout := 30 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if token Secret exists
		secrets, err := clientset.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("type=kubernetes.io/service-account-token"),
		})
		if err != nil {
			return fmt.Errorf("failed to list Secrets: %w", err)
		}

		// Find Secret for this ServiceAccount
		for i := range secrets.Items {
			secret := &secrets.Items[i]
			if secret.Annotations["kubernetes.io/service-account.name"] == saName {
				// Check if token exists and is non-empty
				if token, ok := secret.Data["token"]; ok && len(token) > 0 {
					_, _ = fmt.Fprintf(writer, "   ✅ Token Secret ready: %s\n", secret.Name)
					return nil
				}
			}
		}

		// Wait before retry
		_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for token Secret creation...\n")
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for ServiceAccount token Secret (waited %v)", timeout)
}

// CreateDataStorageAccessRoleBinding creates a RoleBinding for a ServiceAccount
// to access DataStorage in a specific namespace. This is used for E2E tests where
// DataStorage is deployed in the same namespace as the test controller.
//
// Authority: DD-AUTH-011-E2E-RBAC-ISSUE.md (Dynamic RoleBinding creation)
//
// Parameters:
//   - ctx: Context for cancellation
//   - namespace: Namespace for RoleBinding (same as DataStorage)
//   - kubeconfigPath: Path to kubeconfig file
//   - saName: ServiceAccount name
//   - writer: Output writer for logs
//
// Returns:
//   - error: Error if creation fails
//
// Usage:
//
//	// For Notification E2E - DataStorage in same namespace
//	err := CreateDataStorageAccessRoleBinding(ctx, "notification-e2e", kubeconfigPath, "notification-controller", GinkgoWriter)
func CreateDataStorageAccessRoleBinding(ctx context.Context, namespace, kubeconfigPath, saName string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🔐 Creating DataStorage access RoleBinding for %s...\n", saName)

	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-datastorage-access", saName),
			Namespace: namespace,
			Labels: map[string]string{
				"app":       saName,
				"component": "rbac",
				"test":      "e2e",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "data-storage-client", // DD-AUTH-011: verb:"create"
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: namespace,
			},
		},
	}

	_, err = clientset.RbacV1().RoleBindings(namespace).Create(ctx, rb, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create RoleBinding: %w", err)
	}

	if apierrors.IsAlreadyExists(err) {
		_, _ = fmt.Fprintf(writer, "   ℹ️  RoleBinding already exists: %s-datastorage-access\n", saName)
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ RoleBinding created: %s-datastorage-access\n", saName)
	}

	_, _ = fmt.Fprintf(writer, "   Namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "   ServiceAccount: %s\n", saName)
	_, _ = fmt.Fprintf(writer, "   ClusterRole: data-storage-client (CRUD: create, get, list, update, delete)\n")

	return nil
}

// CreateServiceAccount creates a ServiceAccount without any RBAC permissions.
// This is used for testing unauthorized access scenarios.
//
// Authority: DD-AUTH-011 (Access control testing)
//
// Parameters:
//   - ctx: Context for cancellation
//   - namespace: Namespace to create ServiceAccount in
//   - kubeconfigPath: Path to kubeconfig file
//   - saName: Name of ServiceAccount to create
//   - writer: Output writer for logs
//
// Returns:
//   - error: Error if creation fails
func CreateServiceAccount(ctx context.Context, namespace, kubeconfigPath, saName string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":       "datastorage-e2e",
				"component": "test",
				"rbac":      "none",
			},
		},
	}

	_, err = clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create ServiceAccount: %w", err)
	}

	// Note: Kubernetes 1.24+ no longer auto-creates token Secrets
	// Tokens are retrieved via TokenRequest API when needed
	_, _ = fmt.Fprintf(writer, "✅ ServiceAccount created without RBAC: %s\n", saName)
	return nil
}

// CreateServiceAccountWithReadOnlyAccess creates a ServiceAccount with read-only access.
// This ServiceAccount has "get" permission but not "create", which should fail SAR
// when ose-oauth-proxy checks for verb:"create".
//
// Authority: DD-AUTH-011 (Testing insufficient permissions)
//
// Parameters:
//   - ctx: Context for cancellation
//   - namespace: Namespace to create ServiceAccount in
//   - kubeconfigPath: Path to kubeconfig file
//   - saName: Name of ServiceAccount to create
//   - writer: Output writer for logs
//
// Returns:
//   - error: Error if creation fails
func CreateServiceAccountWithReadOnlyAccess(ctx context.Context, namespace, kubeconfigPath, saName string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// 1. Create ServiceAccount
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":       "datastorage-e2e",
				"component": "test",
				"rbac":      "read-only",
			},
		},
	}

	_, err = clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create ServiceAccount: %w", err)
	}

	// 2. Create ClusterRole with "get" permission only (insufficient for audit writes)
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "data-storage-read-only-test",
			Labels: map[string]string{
				"app":       "datastorage-e2e",
				"component": "rbac",
				"test":      "sar-validation",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"services"},
				ResourceNames: []string{"data-storage-service"},
				Verbs:         []string{"get"}, // Only "get", not "create"
			},
		},
	}

	_, err = clientset.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create ClusterRole: %w", err)
	}

	// 3. Create RoleBinding
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-read-only", saName),
			Namespace: namespace,
			Labels: map[string]string{
				"app":       "datastorage-e2e",
				"component": "rbac",
				"test":      "e2e",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "data-storage-read-only-test",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: namespace,
			},
		},
	}

	_, err = clientset.RbacV1().RoleBindings(namespace).Create(ctx, rb, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create RoleBinding: %w", err)
	}

	// Note: Kubernetes 1.24+ no longer auto-creates token Secrets
	// Tokens are retrieved via TokenRequest API when needed
	_, _ = fmt.Fprintf(writer, "✅ ServiceAccount created with read-only access (verb:get): %s\n", saName)
	return nil
}

// VerifyRBACPermission verifies if a ServiceAccount has a specific RBAC permission
// using kubectl auth can-i. Returns true if permission is granted, false otherwise.
//
// Authority: DD-AUTH-011 (RBAC verification)
//
// Parameters:
//   - ctx: Context for cancellation
//   - namespace: Namespace containing ServiceAccount
//   - saName: ServiceAccount name
//   - kubeconfigPath: Path to kubeconfig file
//   - verb: RBAC verb to check (create, get, list, update, delete)
//   - resource: K8s resource type (e.g., "services")
//   - resourceName: Specific resource name (e.g., "data-storage-service")
//   - writer: Output writer for logs
//
// Returns:
//   - bool: true if permission granted, false otherwise
//   - error: Error if command fails
//
// Usage:
//
//	canCreate, err := VerifyRBACPermission(ctx, "kubernaut-system", "gateway-sa", kubeconfigPath, "create", "services", "data-storage-service", GinkgoWriter)
func VerifyRBACPermission(ctx context.Context, namespace, saName, kubeconfigPath, verb, resource, resourceName string, writer io.Writer) (bool, error) {
	resourcePath := resource
	if resourceName != "" {
		resourcePath = fmt.Sprintf("%s/%s", resource, resourceName)
	}

	cmd := exec.CommandContext(ctx, "kubectl", "auth", "can-i", verb,
		resourcePath,
		fmt.Sprintf("--as=system:serviceaccount:%s:%s", namespace, saName),
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
	)

	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	output, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(output))

	// kubectl auth can-i returns:
	//   - exit 0 + "yes" when permission granted
	//   - exit 1 + "no" when permission denied (NOT an error!)
	//   - exit 1 + error message when actual error occurred
	hasPermission := outputStr == "yes"
	hasNoPermission := outputStr == "no"

	if err != nil && !hasNoPermission {
		// Real error (not just "no" permission)
		_, _ = fmt.Fprintf(writer, "   ⚠️  kubectl auth can-i command failed: %s\n", outputStr)
		return false, fmt.Errorf("kubectl auth can-i command failed: %w", err)
	}

	if hasPermission {
		_, _ = fmt.Fprintf(writer, "   ✅ ServiceAccount %s/%s has '%s' permission on %s\n", namespace, saName, verb, resourcePath)
	} else {
		_, _ = fmt.Fprintf(writer, "   ❌ ServiceAccount %s/%s does NOT have '%s' permission on %s\n", namespace, saName, verb, resourcePath)
	}

	return hasPermission, nil
}

// Note: getKubernetesClient() is defined in datastorage.go (shared helper)

// VerifyServiceAccountAccess verifies that a ServiceAccount can access DataStorage
// using kubectl auth can-i. This is useful for debugging RBAC issues in E2E tests.
//
// Authority: DD-AUTH-011 (RBAC verification)
//
// Parameters:
//   - namespace: Namespace containing DataStorage
//   - saName: ServiceAccount name
//   - kubeconfigPath: Path to kubeconfig file
//   - writer: Output writer for logs
//
// Returns:
//   - error: Error if verification fails
//
// Usage:
//
//	err := VerifyServiceAccountAccess("kubernaut-system", "datastorage-e2e-sa", kubeconfigPath, GinkgoWriter)
func VerifyServiceAccountAccess(namespace, saName, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🔍 Verifying ServiceAccount RBAC permissions...\n")

	cmd := exec.Command("kubectl", "auth", "can-i", "create",
		"services/data-storage-service",
		fmt.Sprintf("--as=system:serviceaccount:%s:%s", namespace, saName),
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
	)

	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	output, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(output))

	if err != nil {
		_, _ = fmt.Fprintf(writer, "   ❌ RBAC verification failed: %s\n", outputStr)
		return fmt.Errorf("ServiceAccount %s/%s cannot access DataStorage: %w", namespace, saName, err)
	}

	if outputStr != "yes" {
		_, _ = fmt.Fprintf(writer, "   ❌ RBAC verification failed: got '%s', expected 'yes'\n", outputStr)
		return fmt.Errorf("ServiceAccount %s/%s denied access to DataStorage", namespace, saName)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ ServiceAccount %s/%s can access DataStorage (verb:create)\n", namespace, saName)
	return nil
}

// IntegrationAuthConfig contains the authentication configuration for integration tests
// with envtest. It provides both the ServiceAccount token and kubeconfig file that
// DataStorage containers can use to validate requests via TokenReview/SAR APIs.
type IntegrationAuthConfig struct {
	// Token is the ServiceAccount Bearer token (from TokenRequest API)
	Token string

	// KubeconfigPath is the path to the kubeconfig file for envtest API server
	KubeconfigPath string

	// ServiceAccountName is the name of the created ServiceAccount
	ServiceAccountName string

	// Namespace is the namespace containing the ServiceAccount
	Namespace string
}

// CreateIntegrationServiceAccountWithDataStorageAccess creates a ServiceAccount in envtest
// with RBAC permissions to access DataStorage, retrieves a token via TokenRequest API,
// and writes a kubeconfig file that DataStorage containers can use.
//
// Authority: DD-AUTH-014 (Middleware-based authentication for integration tests)
//
// This function eliminates the need for:
//   - ENV_MODE security risk (no conditional auth in production binary)
//   - MockUserTransport (no manual header injection)
//   - X-Auth-Request-User header fallback in handlers
//
// Usage Pattern (in service integration test SynchronizedBeforeSuite Phase 1):
//
//	authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
//	    cfg,                                    // from testEnv.Start()
//	    "remediationorchestrator-ds-client",   // ServiceAccount name
//	    "default",                              // Namespace
//	    GinkgoWriter,                           // Log writer
//	)
//	Expect(err).ToNot(HaveOccurred())
//
//	// Pass to DataStorage container:
//	dsInfra, err := infrastructure.StartDSBootstrap(infrastructure.DSBootstrapConfig{
//	    // ... other config ...
//	    EnvtestKubeconfig: authConfig.KubeconfigPath, // ← DataStorage uses envtest API
//	}, GinkgoWriter)
//
// Parameters:
//   - cfg: envtest REST config (from testEnv.Start())
//   - saName: ServiceAccount name to create
//   - namespace: Namespace for ServiceAccount
//   - writer: Output writer for logs
//
// Returns:
//   - IntegrationAuthConfig: Token + kubeconfig for DataStorage container
//   - error: Error if creation fails
//
// Benefits:
//   - ✅ Real TokenReview/SAR validation (envtest provides real K8s APIs)
//   - ✅ No ENV_MODE security risk (production binary unchanged)
//   - ✅ No header injection (context-only user attribution)
//   - ✅ Realistic test coverage (actual middleware code path)
func CreateIntegrationServiceAccountWithDataStorageAccess(
	cfg *rest.Config,
	saName string,
	namespace string,
	writer io.Writer,
) (*IntegrationAuthConfig, error) {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Creating Integration ServiceAccount with DataStorage Access (envtest)\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "  Namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "  ServiceAccount: %s\n", saName)
	_, _ = fmt.Fprintf(writer, "  ClusterRole: data-storage-client (CRUD: create, get, list, update, delete)\n")
	_, _ = fmt.Fprintf(writer, "  Authority: DD-AUTH-014 (Real K8s auth via envtest)\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	ctx := context.Background()

	// Create Kubernetes clientset from envtest config
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// ════════════════════════════════════════════════════════════════════════
	// STEP 1: Create Namespace (if it doesn't exist)
	// ════════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintf(writer, "📦 Ensuring namespace exists: %s\n", namespace)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	_, err = clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create namespace: %w", err)
	}
	if apierrors.IsAlreadyExists(err) {
		_, _ = fmt.Fprintf(writer, "   ℹ️  Namespace already exists\n")
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ Namespace created\n")
	}

	// ════════════════════════════════════════════════════════════════════════
	// STEP 2: Create ServiceAccount
	// ════════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintf(writer, "🔐 Creating ServiceAccount: %s\n", saName)
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":       "datastorage-integration-client",
				"component": "test",
				"rbac":      "dd-auth-014",
			},
		},
	}

	_, err = clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create ServiceAccount: %w", err)
	}
	if apierrors.IsAlreadyExists(err) {
		_, _ = fmt.Fprintf(writer, "   ℹ️  ServiceAccount already exists\n")
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ ServiceAccount created\n")
	}

	// ════════════════════════════════════════════════════════════════════════
	// STEP 3: Create ClusterRole for DataStorage client access
	// ════════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintf(writer, "🔐 Creating ClusterRole: data-storage-client\n")
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "data-storage-client",
			Labels: map[string]string{
				"app":       "datastorage",
				"component": "rbac",
				"test":      "integration",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"services"},
				ResourceNames: []string{"data-storage-service"},
				Verbs:         []string{"create", "get", "list", "update", "delete"},
			},
			// DD-AUTH-014: WORKAROUND for envtest TokenReview behavior
			// envtest/fake K8s API checks if the token BEING validated has TokenReview permission
			// (not if the caller has it). This is different from real K8s API behavior.
			// Grant TokenReview to client SA so validation succeeds.
			{
				APIGroups: []string{"authentication.k8s.io"},
				Resources: []string{"tokenreviews"},
				Verbs:     []string{"create"},
			},
		},
	}

	_, err = clientset.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create ClusterRole: %w", err)
	}
	if apierrors.IsAlreadyExists(err) {
		_, _ = fmt.Fprintf(writer, "   ℹ️  ClusterRole already exists\n")
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ ClusterRole created\n")
	}

	// ════════════════════════════════════════════════════════════════════════
	// STEP 4: Create ClusterRoleBinding
	// ════════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintf(writer, "🔐 Creating ClusterRoleBinding: %s-data-storage-client\n", saName)
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-data-storage-client", saName),
			Labels: map[string]string{
				"app":       "datastorage",
				"component": "rbac",
				"test":      "integration",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "data-storage-client",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: namespace,
			},
		},
	}

	_, err = clientset.RbacV1().ClusterRoleBindings().Create(ctx, clusterRoleBinding, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create ClusterRoleBinding: %w", err)
	}
	if apierrors.IsAlreadyExists(err) {
		_, _ = fmt.Fprintf(writer, "   ℹ️  ClusterRoleBinding already exists\n")
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ ClusterRoleBinding created\n")
	}

	// ════════════════════════════════════════════════════════════════════════
	// STEP 4.5: Create DataStorage Service ServiceAccount + RBAC (for TokenReview)
	// ════════════════════════════════════════════════════════════════════════
	// DD-AUTH-014: DataStorage needs its own ServiceAccount to call TokenReview API
	// The client ServiceAccount (created above) is what gets validated
	// The DataStorage service ServiceAccount does the validating
	datastorageSAName := "datastorage-service"
	_, _ = fmt.Fprintf(writer, "🔐 Creating DataStorage Service ServiceAccount: %s\n", datastorageSAName)
	
	datastorageSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      datastorageSAName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":       "datastorage",
				"component": "service",
				"rbac":      "dd-auth-014",
			},
		},
	}

	_, err = clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, datastorageSA, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create DataStorage service ServiceAccount: %w", err)
	}
	if apierrors.IsAlreadyExists(err) {
		_, _ = fmt.Fprintf(writer, "   ℹ️  DataStorage service ServiceAccount already exists\n")
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ DataStorage service ServiceAccount created\n")
	}

	// Create ClusterRole for DataStorage service (TokenReview permission)
	_, _ = fmt.Fprintf(writer, "🔐 Creating ClusterRole: datastorage-tokenreview\n")
	datastorageClusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "datastorage-tokenreview",
			Labels: map[string]string{
				"app":       "datastorage",
				"component": "rbac",
				"test":      "integration",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"authentication.k8s.io"},
				Resources: []string{"tokenreviews"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"authorization.k8s.io"},
				Resources: []string{"subjectaccessreviews"},
				Verbs:     []string{"create"},
			},
		},
	}

	_, err = clientset.RbacV1().ClusterRoles().Create(ctx, datastorageClusterRole, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create DataStorage TokenReview ClusterRole: %w", err)
	}
	if apierrors.IsAlreadyExists(err) {
		_, _ = fmt.Fprintf(writer, "   ℹ️  DataStorage TokenReview ClusterRole already exists\n")
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ DataStorage TokenReview ClusterRole created\n")
	}

	// Create ClusterRoleBinding for DataStorage service
	_, _ = fmt.Fprintf(writer, "🔐 Creating ClusterRoleBinding: datastorage-service-tokenreview\n")
	datastorageClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "datastorage-service-tokenreview",
			Labels: map[string]string{
				"app":       "datastorage",
				"component": "rbac",
				"test":      "integration",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "datastorage-tokenreview",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      datastorageSAName,
				Namespace: namespace,
			},
		},
	}

	_, err = clientset.RbacV1().ClusterRoleBindings().Create(ctx, datastorageClusterRoleBinding, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("failed to create DataStorage service ClusterRoleBinding: %w", err)
	}
	if apierrors.IsAlreadyExists(err) {
		_, _ = fmt.Fprintf(writer, "   ℹ️  DataStorage service ClusterRoleBinding already exists\n")
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ DataStorage service ClusterRoleBinding created\n")
	}

	// Get token for DataStorage service (for kubeconfig)
	_, _ = fmt.Fprintf(writer, "🎫 Requesting DataStorage service token...\n")
	datastorageTokenRequest := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: int64Ptr(3600), // 1 hour
		},
	}

	datastorageTokenResp, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(
		ctx,
		datastorageSAName,
		datastorageTokenRequest,
		metav1.CreateOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create DataStorage service token: %w", err)
	}
	datastorageToken := datastorageTokenResp.Status.Token
	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage service token retrieved\n")

	// ════════════════════════════════════════════════════════════════════════
	// STEP 5: Get Token via TokenRequest API (for client)
	// ════════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintf(writer, "🎫 Requesting client ServiceAccount token...\n")
	tokenRequest := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: int64Ptr(3600), // 1 hour
		},
	}

	tokenResp, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(
		ctx,
		saName,
		tokenRequest,
		metav1.CreateOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	token := tokenResp.Status.Token
	_, _ = fmt.Fprintf(writer, "   ✅ Token retrieved (expires: %v)\n", tokenResp.Status.ExpirationTimestamp.Time)

	// ════════════════════════════════════════════════════════════════════════
	// STEP 6: Write kubeconfig file for DataStorage container
	// ════════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintf(writer, "📝 Writing kubeconfig for envtest...\n")

	// DD-AUTH-014: Rewrite 127.0.0.1 → host.containers.internal for Podman bridge networking
	// envtest now binds to 127.0.0.1 (via /etc/hosts localhost fix)
	// Container uses host.containers.internal to reach host's 127.0.0.1
	// TLS: Skip verification (cert is for "localhost", not "host.containers.internal")
	// Reference: docs/handoff/DD_AUTH_014_MACOS_PODMAN_LIMITATION.md
	containerAPIServer := strings.Replace(cfg.Host, "127.0.0.1", "host.containers.internal", 1)
	
	_, _ = fmt.Fprintf(writer, "   📍 envtest URL: %s\n", cfg.Host)
	_, _ = fmt.Fprintf(writer, "   🔄 Container URL: %s (TLS verification skipped)\n", containerAPIServer)
	_, _ = fmt.Fprintf(writer, "   ℹ️  host.containers.internal routes to host's 127.0.0.1\n")

	// DD-AUTH-014: Kubeconfig uses DataStorage service token (for TokenReview API calls)
	// Client token is returned separately for audit client authentication
	kubeconfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"envtest": {
				Server:                containerAPIServer,
				InsecureSkipTLSVerify: true, // Required: TLS cert is for "localhost", not "host.containers.internal"
				// Note: CertificateAuthorityData omitted (incompatible with InsecureSkipTLSVerify)
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			datastorageSAName: {
				Token: datastorageToken, // DataStorage service token (can create TokenReviews)
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"envtest": {
				Cluster:  "envtest",
				AuthInfo: datastorageSAName,
			},
		},
		CurrentContext: "envtest",
	}

	// Write to temporary directory (DD-AUTH-014)
	// NOTE: Podman rootless cannot mount files from /tmp/ (statfs fails)
	// Must use a directory under $HOME which Podman rootless can access
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	
	kubeconfigDir := filepath.Join(homeDir, "tmp", "kubernaut-envtest")
	err = os.MkdirAll(kubeconfigDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	kubeconfigPath := filepath.Join(kubeconfigDir, fmt.Sprintf("kubeconfig-%s.yaml", saName))
	err = clientcmd.WriteToFile(kubeconfig, kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	// Fix file permissions for Podman rootless access (DD-AUTH-014)
	// clientcmd.WriteToFile creates files with mode 0600 (owner-only)
	// Podman rootless needs 0644 (readable by group/others) to mount the file
	err = os.Chmod(kubeconfigPath, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to set kubeconfig permissions: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Kubeconfig written: %s (mode: 0644, Podman-compatible location)\n", kubeconfigPath)
	_, _ = fmt.Fprintf(writer, "   📍 API Server: %s\n", cfg.Host)

	// ════════════════════════════════════════════════════════════════════════
	// STEP 7: Return configuration
	// ════════════════════════════════════════════════════════════════════════
	authConfig := &IntegrationAuthConfig{
		Token:              token,
		KubeconfigPath:     kubeconfigPath,
		ServiceAccountName: saName,
		Namespace:          namespace,
	}

	_, _ = fmt.Fprintf(writer, "✅ Integration auth configured with real K8s TokenReview/SAR\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return authConfig, nil
}

// int64Ptr is a helper to get a pointer to an int64 value
func int64Ptr(i int64) *int64 {
	return &i
}
