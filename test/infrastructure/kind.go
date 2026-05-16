// Package infrastructure provides AF-specific E2E helpers that build on
// kubernaut's shared test/infrastructure package.
// When the repos merge, this file becomes unnecessary — callers import
// kubernaut's package directly.
package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	kinfra "github.com/jordigilh/kubernaut/test/infrastructure"
)

const (
	// DefaultClusterName is the Kind cluster name for apifrontend E2E tests.
	DefaultClusterName = "apifrontend-e2e"
	// DefaultKindConfig is the path to the Kind config file, relative to the repo root.
	DefaultKindConfig = "deploy/kustomize/overlays/e2e/kind-config.yaml"
	// DefaultNamespace is the Kubernetes namespace for AF E2E workloads.
	DefaultNamespace = "kubernaut-system"
)

// CreateAFKindCluster creates the AF E2E Kind cluster using kubernaut's
// canonical CreateKindClusterWithConfig.
func CreateAFKindCluster(kubeconfigPath string, writer io.Writer) error {
	opts := kinfra.KindClusterOptions{
		ClusterName:               DefaultClusterName,
		KubeconfigPath:            kubeconfigPath,
		ConfigPath:                DefaultKindConfig,
		WaitTimeout:               "5m",
		DeleteExisting:            true,
		CleanupOrphanedContainers: true,
		UsePodman:                 true,
		ProjectRootAsWorkingDir:   true,
	}
	return kinfra.CreateKindClusterWithConfig(opts, writer)
}

// BuildAFImage builds the apifrontend container image using kubernaut's
// canonical BuildImageForKind.
func BuildAFImage(writer io.Writer) (string, error) {
	cfg := kinfra.E2EImageConfig{
		ServiceName:      "apifrontend",
		ImageName:        "kubernaut-apifrontend",
		DockerfilePath:   "Dockerfile",
		BuildContextPath: getAFProjectRoot(),
	}
	return kinfra.BuildImageForKind(cfg, writer)
}

// BuildMockLLMImage builds the mock-LLM image from the kubernaut repo using
// kubernaut's canonical BuildImageForKind.
func BuildMockLLMImage(writer io.Writer) (string, error) {
	kubernautRepo := kubernautRepoPath()
	if _, err := os.Stat(filepath.Join(kubernautRepo, "go.mod")); err != nil {
		return "", fmt.Errorf("kubernaut repo not found at %s — cannot build mock-LLM locally", kubernautRepo)
	}
	cfg := kinfra.E2EImageConfig{
		ServiceName:      "mock-llm",
		ImageName:        "mock-llm",
		DockerfilePath:   "test/services/mock-llm/go.Dockerfile",
		BuildContextPath: kubernautRepo,
	}
	return kinfra.BuildImageForKind(cfg, writer)
}

// LoadImageToKind delegates to kubernaut's canonical implementation.
func LoadImageToKind(imageName, serviceName, clusterName string, writer io.Writer) error {
	return kinfra.LoadImageToKind(imageName, serviceName, clusterName, writer)
}

// DeleteCluster tears down the AF E2E Kind cluster.
func DeleteCluster(writer io.Writer) error {
	cmd := exec.Command("kind", "delete", "cluster", "--name", DefaultClusterName)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// ApplyKustomize applies a kustomize overlay to the cluster.
func ApplyKustomize(ctx context.Context, kubeconfigPath, kustomizePath string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-k", kustomizePath)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply -k %s failed: %w", kustomizePath, err)
	}
	return nil
}

// GenerateCerts runs the AF cert generation script.
func GenerateCerts(certDir string, writer io.Writer) error {
	projectRoot := getAFProjectRoot()
	script := projectRoot + "/deploy/kustomize/overlays/e2e/generate-certs.sh"
	cmd := exec.Command("bash", script, certDir) //nolint:gosec // G204: test infra, script path from project root
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("generate-certs.sh failed: %w", err)
	}
	return nil
}

// CreateNamespace creates a namespace (idempotent via dry-run + apply).
func CreateNamespace(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	dryRunCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
	yamlData, err := dryRunCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to generate namespace YAML: %w", err)
	}
	applyCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(string(yamlData))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer
	return applyCmd.Run()
}

// CreateTLSSecrets creates the TLS secrets required by AF from the cert directory.
func CreateTLSSecrets(ctx context.Context, kubeconfigPath, namespace, certDir string, writer io.Writer) error {
	secrets := []struct {
		name     string
		certFile string
		keyFile  string
	}{
		{"apifrontend-tls", "tls.crt", "tls.key"},
	}
	for _, s := range secrets {
		dryRunCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infra
			"create", "secret", "tls", s.name,
			"--cert="+filepath.Join(certDir, s.certFile),
			"--key="+filepath.Join(certDir, s.keyFile),
			"-n", namespace, "--dry-run=client", "-o", "yaml")
		yamlData, err := dryRunCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to generate TLS secret %s: %w", s.name, err)
		}
		applyCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
		applyCmd.Stdin = strings.NewReader(string(yamlData))
		applyCmd.Stdout = writer
		applyCmd.Stderr = writer
		if err := applyCmd.Run(); err != nil {
			return fmt.Errorf("failed to apply TLS secret %s: %w", s.name, err)
		}
	}

	dryRunCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infra
		"create", "secret", "generic", "apifrontend-ca",
		"--from-file=ca.crt="+filepath.Join(certDir, "ca.crt"),
		"-n", namespace, "--dry-run=client", "-o", "yaml")
	yamlData, err := dryRunCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to generate CA secret: %w", err)
	}
	applyCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(string(yamlData))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer
	return applyCmd.Run()
}

// WaitForDeploymentRollout waits for a deployment to become ready.
func WaitForDeploymentRollout(ctx context.Context, kubeconfigPath, namespace, name string, timeout time.Duration, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, //nolint:gosec // G204: test infra
		"rollout", "status", "deployment/"+name, "-n", namespace,
		fmt.Sprintf("--timeout=%ds", int(timeout.Seconds())))
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("deployment/%s not ready: %w", name, err)
	}
	return nil
}

func getAFProjectRoot() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
	}
	candidates := []string{".", "..", "../..", "../../.."}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			absPath, _ := filepath.Abs(dir)
			return absPath
		}
	}
	return "."
}

func kubernautRepoPath() string {
	projectRoot := getAFProjectRoot()
	candidate := filepath.Join(projectRoot, "..", "kubernaut")
	if _, err := os.Stat(filepath.Join(candidate, "go.mod")); err == nil {
		abs, _ := filepath.Abs(candidate)
		return abs
	}
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = filepath.Join(os.Getenv("HOME"), "go")
	}
	return filepath.Join(gopath, "src", "github.com", "jordigilh", "kubernaut")
}
