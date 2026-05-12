package infrastructure

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const (
	DefaultClusterName = "apifrontend-e2e"
	DefaultKindConfig  = "deploy/kustomize/overlays/e2e/kind-config.yaml"
)

type KindCluster struct {
	Name       string
	ConfigPath string
	Kubeconfig string
}

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

func (k *KindCluster) Create(ctx context.Context) error {
	args := []string{"create", "cluster", "--name", k.Name, "--config", k.ConfigPath, "--wait", "60s"}
	cmd := exec.CommandContext(ctx, "kind", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster: %w", err)
	}
	k.Kubeconfig = fmt.Sprintf("kind-%s", k.Name)
	return nil
}

func (k *KindCluster) Delete(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "kind", "delete", "cluster", "--name", k.Name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (k *KindCluster) LoadImage(ctx context.Context, image string) error {
	cmd := exec.CommandContext(ctx, "kind", "load", "docker-image", image, "--name", k.Name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind load docker-image %s: %w", image, err)
	}
	return nil
}

func (k *KindCluster) KubeconfigContext() string {
	return fmt.Sprintf("kind-%s", k.Name)
}

func (k *KindCluster) WaitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd := exec.CommandContext(ctx, "kubectl", "--context", k.KubeconfigContext(),
			"get", "nodes", "-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
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
