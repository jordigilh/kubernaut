// Package infrastructure provides helpers for managing Kind clusters
// and verifying deployment readiness in E2E tests.
package infrastructure

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const (
	// DefaultClusterName is the Kind cluster name used for E2E testing.
	DefaultClusterName = "apifrontend-e2e"
	// DefaultKindConfig is the path to the Kind cluster configuration.
	DefaultKindConfig = "deploy/kustomize/overlays/e2e/kind-config.yaml"
)

// KindCluster manages the lifecycle of a Kind cluster for E2E tests.
type KindCluster struct {
	Name       string
	ConfigPath string
	Kubeconfig string
}

// NewKindCluster creates a KindCluster with the given name and config path.
func NewKindCluster(name, configPath string) *KindCluster {
	if name == "" {
		name = DefaultClusterName
	}
	if configPath == "" {
		configPath = DefaultKindConfig
	}
	return &KindCluster{
		Name:       name,
		ConfigPath: configPath,
	}
}

// Create provisions the Kind cluster and waits for the control plane.
func (k *KindCluster) Create(ctx context.Context) error {
	args := []string{"create", "cluster", "--name", k.Name, "--config", k.ConfigPath, "--wait", "60s"}
	cmd := exec.CommandContext(ctx, "kind", args...) // #nosec G204 -- test infrastructure, args are controlled
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster: %w", err)
	}
	k.Kubeconfig = fmt.Sprintf("kind-%s", k.Name)
	return nil
}

// Delete tears down the Kind cluster.
func (k *KindCluster) Delete(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "kind", "delete", "cluster", "--name", k.Name) // #nosec G204 -- test infrastructure
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LoadImage loads a container image into the Kind cluster's node.
func (k *KindCluster) LoadImage(ctx context.Context, image string) error {
	cmd := exec.CommandContext(ctx, "kind", "load", "docker-image", image, "--name", k.Name) // #nosec G204 -- test infrastructure
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind load docker-image %s: %w", image, err)
	}
	return nil
}

// KubeconfigContext returns the kubectl context name for this cluster.
func (k *KindCluster) KubeconfigContext() string {
	return fmt.Sprintf("kind-%s", k.Name)
}

// WaitForReady polls the cluster until at least one node reports Ready.
func (k *KindCluster) WaitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.CommandContext(ctx, "kubectl", "--context", k.KubeconfigContext(),
			"get", "nodes", "-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}") // #nosec G204 -- test infrastructure
		out, err := cmd.Output()
		if err == nil && string(out) == "True" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("kind cluster %s not ready after %v", k.Name, timeout)
}
