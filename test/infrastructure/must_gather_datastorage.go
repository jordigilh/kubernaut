package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// PrepareMustGatherDataStorage provisions a short-lived, get-only test identity.
// It is confined to the explicitly named E2E cluster/namespace.
func PrepareMustGatherDataStorage(ctx context.Context, opts *RunMustGatherImageOptions, kubeconfigPath, caFile string) (func() error, error) {
	noop := func() error { return nil }
	client, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return noop, err
	}
	namespace, nodePort, bridgeIP, ca, dir, err := prepareMustGatherDataStorageTarget(ctx, client, opts, caFile)
	if err != nil {
		return noop, err
	}
	name := "must-gather-query-" + uuid.NewString()
	var saCreated, roleCreated, bindingCreated bool
	var once sync.Once
	var cleanupErr error
	cleanup := func() error { //nolint:contextcheck // bounded resource teardown must work after the collection/request context expires
		once.Do(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var errs []error
			if bindingCreated {
				errs = append(errs, client.RbacV1().RoleBindings(namespace).Delete(cleanupCtx, name, metav1.DeleteOptions{}))
			}
			if roleCreated {
				errs = append(errs, client.RbacV1().Roles(namespace).Delete(cleanupCtx, name, metav1.DeleteOptions{}))
			}
			if saCreated {
				errs = append(errs, client.CoreV1().ServiceAccounts(namespace).Delete(cleanupCtx, name, metav1.DeleteOptions{}))
			}
			// Only known files in this private directory; never broad recursive cleanup.
			for _, file := range []string{"token", "ca.crt"} {
				if err := os.Remove(filepath.Join(dir, file)); err != nil && !os.IsNotExist(err) {
					errs = append(errs, err)
				}
			}
			errs = append(errs, os.Remove(dir))
			cleanupErr = errors.Join(errs...)
		})
		return cleanupErr
	}
	fail := func(err error) (func() error, error) { return noop, errors.Join(err, cleanup()) }
	saCreated, roleCreated, bindingCreated, err = createMustGatherIdentity(ctx, client, namespace, name)
	if err != nil {
		return fail(err)
	}
	expiration := int64(600)
	token, err := client.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, name, &authnv1.TokenRequest{Spec: authnv1.TokenRequestSpec{ExpirationSeconds: &expiration}}, metav1.CreateOptions{})
	if err != nil {
		return fail(fmt.Errorf("request short-lived collector token: %w", err))
	}
	if token.Status.Token == "" {
		return fail(fmt.Errorf("collector token request returned no credential"))
	}
	caPath, tokenPath := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "token")
	if err := os.WriteFile(caPath, ca, 0600); err != nil {
		return fail(fmt.Errorf("write collector ca: %w", err))
	}
	if err := os.WriteFile(tokenPath, []byte(token.Status.Token), 0600); err != nil {
		return fail(fmt.Errorf("write collector token: %w", err))
	}
	authority := "data-storage-service." + namespace + ".svc"
	opts.DataStorage = &MustGatherDataStorageOptions{URL: "https://" + authority + ":8080", CAFile: caPath, TokenFile: tokenPath,
		ConnectTo: authority + ":8080:" + bridgeIP + ":" + strconv.Itoa(int(nodePort))}
	return cleanup, nil
}

func createMustGatherIdentity(ctx context.Context, client kubernetes.Interface, namespace, name string) (bool, bool, bool, error) {
	_, err := client.CoreV1().ServiceAccounts(namespace).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}, metav1.CreateOptions{})
	if err != nil {
		return false, false, false, fmt.Errorf("create collector serviceaccount: %w", err)
	}
	_, err = client.RbacV1().Roles(namespace).Create(ctx, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"services"}, ResourceNames: []string{"data-storage-service"}, Verbs: []string{"get"}}}}, metav1.CreateOptions{})
	if err != nil {
		return true, false, false, fmt.Errorf("create get-only collector role: %w", err)
	}
	_, err = client.RbacV1().RoleBindings(namespace).Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}, RoleRef: rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: name}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: name, Namespace: namespace}}}, metav1.CreateOptions{})
	if err != nil {
		return true, true, false, fmt.Errorf("bind get-only collector role: %w", err)
	}
	return true, true, true, nil
}

func prepareMustGatherDataStorageTarget(ctx context.Context, client kubernetes.Interface, opts *RunMustGatherImageOptions, caFile string) (string, int32, string, []byte, string, error) {
	namespace := opts.Namespace
	if namespace == "" {
		namespace = kubernautSystem
	}
	service, err := client.CoreV1().Services(namespace).Get(ctx, "data-storage-service", metav1.GetOptions{})
	if err != nil {
		return "", 0, "", nil, "", fmt.Errorf("resolve collector DataStorage service: %w", err)
	}
	var nodePort int32
	for _, port := range service.Spec.Ports {
		if port.Port == 8080 {
			nodePort = port.NodePort
			break
		}
	}
	if nodePort == 0 {
		return "", 0, "", nil, "", fmt.Errorf("collector DataStorage service has no external nodeport")
	}
	bridgeIP, err := KindNodeBridgeIP(ctx, opts.ClusterName+"-control-plane")
	if err != nil {
		return "", 0, "", nil, "", err
	}
	if net.ParseIP(bridgeIP) == nil {
		return "", 0, "", nil, "", fmt.Errorf("collector node bridge address is invalid")
	}
	ca, err := os.ReadFile(caFile)
	if err != nil {
		return "", 0, "", nil, "", fmt.Errorf("read collector ca: %w", err)
	}
	dir, err := os.MkdirTemp("", "must-gather-query-credentials-*")
	if err != nil {
		return "", 0, "", nil, "", fmt.Errorf("create collector credential directory: %w", err)
	}
	return namespace, nodePort, bridgeIP, ca, dir, nil
}
