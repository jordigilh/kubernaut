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
)

// DefaultMustGatherKindNetwork is the podman network name Kind attaches
// every cluster's nodes to (kind_bridge.go's KindNodeBridgeIP relies on the
// same "kind" network name). RunMustGatherImage joins this network so the
// must-gather container can reach the cluster's API server the same way any
// Kind node does, without needing a host-forwarded port.
const DefaultMustGatherKindNetwork = "kind"

// RunMustGatherImageOptions configures a single must-gather collection run
// against a live Kind cluster.
//
// DD-TESTING-003: the must-gather image runs as a local podman container
// joined to the cluster's own "kind" podman network -- not as an in-cluster
// Pod -- because that mechanism survives the death of the process that
// triggered it (the E2E timeout/kill failure mode that motivated this
// migration, see issue #2036), where an in-cluster-Pod + kubectl-cp
// extraction mechanism does not.
type RunMustGatherImageOptions struct {
	// ClusterName is the Kind cluster name (used to derive the in-network
	// kubeconfig via "kind get kubeconfig --internal").
	ClusterName string
	// Image is the must-gather image reference to run, e.g.
	// "localhost/must-gather:e2e".
	Image string
	// OutputDir is a host directory bind-mounted as the container's
	// --dest-dir. Created if missing; the collected tarball and expanded
	// directory land here directly, with no extraction step required.
	OutputDir string
	// Network is the podman network the cluster's nodes are attached to.
	// Defaults to DefaultMustGatherKindNetwork ("kind") when empty.
	Network string
	// Namespace is the Helm release namespace (--namespace). Defaults to
	// "kubernaut-system" when empty.
	Namespace string
	// WorkflowNamespace is the Tekton/Job execution namespace
	// (--workflow-namespace). Defaults to "kubernaut-workflows" when empty.
	WorkflowNamespace string
	// Since is the log collection timeframe (--since). Defaults to "24h"
	// when empty.
	Since string
	// UsePodman selects the podman Kind provider when generating the
	// in-network kubeconfig (mirrors KindClusterOptions.UsePodman -- kind
	// has no persistent state recording which provider a cluster was
	// created under, see exportKubeconfigIfNeeded).
	UsePodman bool
	// ExtraNamespaces are additional namespaces to collect pod logs from,
	// passed through as repeated --extra-namespace flags (Issue #2036/#2194).
	// Needed by multi-cluster suites (fleet, fleetmetadatacache, eaigw) whose
	// mesh/gateway infra (Kuadrant's mcp-system/gateway-system/istio-system,
	// Envoy AI Gateway's envoy-gateway-system/envoy-ai-gateway-system) lives
	// outside RELEASE_NAMESPACE/WORKFLOW_NAMESPACE. Optional -- omitted
	// entirely when unset, matching single-cluster suites that have none.
	ExtraNamespaces []string
}

// RunMustGatherImage runs the production must-gather image as a local podman
// container against a live Kind cluster, collecting the same diagnostic
// bundle CI/support engineers get from a production install (DD-TESTING-003).
//
// The container is removed on exit (--rm); its output lands directly in
// opts.OutputDir via bind mount, so callers can upload/inspect it immediately
// with no separate tarball-extraction step. Because this is a single
// external `podman run` invocation against the still-live cluster network,
// it can be invoked independently of whatever triggered it (e.g. a suite
// teardown, or a wrapper that survives that process being killed on
// timeout) -- the failure mode that motivated replacing MustGatherPodLogs.
func RunMustGatherImage(ctx context.Context, opts RunMustGatherImageOptions, writer io.Writer) error {
	if opts.ClusterName == "" {
		return fmt.Errorf("RunMustGatherImage: ClusterName is required")
	}
	if opts.Image == "" {
		return fmt.Errorf("RunMustGatherImage: Image is required")
	}
	if opts.OutputDir == "" {
		return fmt.Errorf("RunMustGatherImage: OutputDir is required")
	}
	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create must-gather output directory: %w", err)
	}

	kubeconfigPath, cleanup, err := writeInternalKubeconfig(ctx, opts.ClusterName, opts.UsePodman)
	if err != nil {
		return fmt.Errorf("failed to generate in-network kubeconfig: %w", err)
	}
	defer cleanup()

	args := buildMustGatherPodmanArgs(opts, kubeconfigPath)

	_, _ = fmt.Fprintf(writer, "  📋 Running must-gather image %s against cluster %q...\n", opts.Image, opts.ClusterName)
	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("must-gather container run failed: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Must-gather collection complete: %s\n", opts.OutputDir)
	return nil
}

// mustGatherE2EImageName is the local image tag BuildMustGatherImageForE2E
// builds and RunMustGatherImage runs. Fixed (not per-invocation-unique, unlike
// generateInfrastructureImageTag's per-service tags) because must-gather has
// no source-controlled deployment identity to key a cache-bust off of within
// a single E2E run -- Dockerfile content changes are caught by developers
// rebuilding locally between runs, same as any other manually-tagged image.
const mustGatherE2EImageName = "localhost/kubernaut/must-gather:e2e"

// BuildMustGatherImageForE2E resolves the must-gather image RunMustGatherImage
// runs, building it locally from cmd/must-gather/Dockerfile if not already
// cached.
//
// Deliberately always-local-build: unlike BuildImageForKind's per-service
// registry-mode fast path (IMAGE_REGISTRY/IMAGE_TAG), CI does not yet publish
// a per-commit must-gather image the way it does for gateway/datastorage/etc,
// so a registry-mode branch here would silently return a reference nothing
// has pushed, and the later `podman run` would fail to pull it. Wiring an
// equivalent registry-mode fast path is a separate, explicit follow-up once
// CI's build matrix actually publishes must-gather per-commit (tracked under
// issue #2036) -- until then, building locally (~20-40s, matches the
// DD-TESTING-003 spike measurement) on every E2E teardown that needs it is
// simpler and cannot silently reference a nonexistent image.
func BuildMustGatherImageForE2E(ctx context.Context, writer io.Writer) (string, error) {
	checkCmd := exec.CommandContext(ctx, "podman", "image", "exists", mustGatherE2EImageName)
	if checkCmd.Run() == nil {
		_, _ = fmt.Fprintf(writer, "   ✅ must-gather image already exists (using cache): %s\n", mustGatherE2EImageName)
		return mustGatherE2EImageName, nil
	}

	projectRoot := getProjectRoot()
	mustGatherDir := filepath.Join(projectRoot, "cmd", "must-gather")

	_, _ = fmt.Fprintf(writer, "🔨 Building must-gather image: %s\n", mustGatherE2EImageName)
	buildCmd := exec.CommandContext(ctx, "podman", "build",
		"-t", mustGatherE2EImageName,
		"-f", filepath.Join(mustGatherDir, "Dockerfile"),
		mustGatherDir,
	)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to build must-gather image: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ must-gather image built: %s\n", mustGatherE2EImageName)
	return mustGatherE2EImageName, nil
}

// buildMustGatherPodmanArgs builds the "podman run" argument list for a
// must-gather collection run. Factored out as a pure function (no exec) so
// its shape can be unit-tested without shelling out, matching this
// package's convention of separating arg-building/validation logic from the
// exec.Command wrapper (see checkKindVersionOutput vs. validateKindVersion
// in kind_cluster_helpers.go).
func buildMustGatherPodmanArgs(opts RunMustGatherImageOptions, kubeconfigPath string) []string {
	network := opts.Network
	if network == "" {
		network = DefaultMustGatherKindNetwork
	}
	namespace := opts.Namespace
	if namespace == "" {
		namespace = "kubernaut-system"
	}
	workflowNamespace := opts.WorkflowNamespace
	if workflowNamespace == "" {
		workflowNamespace = "kubernaut-workflows"
	}
	since := opts.Since
	if since == "" {
		since = "24h"
	}

	args := []string{
		"run", "--rm",
		"--network", network,
		"-e", "KUBECONFIG=/kubeconfig/kubeconfig-internal.yaml",
		"-v", fmt.Sprintf("%s:/kubeconfig/kubeconfig-internal.yaml:ro,Z", kubeconfigPath),
		"-v", fmt.Sprintf("%s:/must-gather:Z", opts.OutputDir),
		opts.Image,
		"--dest-dir=/must-gather",
		"--namespace=" + namespace,
		"--workflow-namespace=" + workflowNamespace,
		"--since=" + since,
	}
	for _, ns := range opts.ExtraNamespaces {
		if ns == "" {
			continue
		}
		args = append(args, "--extra-namespace="+ns)
	}
	return args
}

// writeInternalKubeconfig generates a kubeconfig that resolves the cluster's
// API server via its in-podman-network DNS name (e.g.
// "https://<cluster>-control-plane:6443") rather than the host-routable
// 127.0.0.1 form kind_cluster_helpers.go's exportKubeconfigIfNeeded produces:
// the must-gather container joins the "kind" network directly (DD-TESTING-003),
// so it must reach the API server the same way any other node on that
// network does, not via a host-forwarded port it has no route to.
//
// Returns the temp file path and a cleanup func the caller must invoke.
func writeInternalKubeconfig(ctx context.Context, clusterName string, usePodman bool) (string, func(), error) {
	noopCleanup := func() {}

	cmd := exec.CommandContext(ctx, "kind", "get", "kubeconfig", "--name", clusterName, "--internal")
	if usePodman {
		cmd.Env = append(os.Environ(), "KIND_EXPERIMENTAL_PROVIDER=podman")
	}
	out, err := cmd.Output()
	if err != nil {
		return "", noopCleanup, fmt.Errorf("kind get kubeconfig --internal failed: %w", err)
	}

	cleaned, err := stripKindProviderBanner(out)
	if err != nil {
		return "", noopCleanup, err
	}

	f, err := os.CreateTemp("", "must-gather-kubeconfig-internal-*.yaml")
	if err != nil {
		return "", noopCleanup, fmt.Errorf("failed to create temp kubeconfig file: %w", err)
	}
	tmpPath := f.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := f.Write(cleaned); err != nil {
		_ = f.Close()
		cleanup()
		return "", noopCleanup, fmt.Errorf("failed to write temp kubeconfig file: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", noopCleanup, fmt.Errorf("failed to close temp kubeconfig file: %w", err)
	}

	return tmpPath, cleanup, nil
}

// stripKindProviderBanner removes kind's "using podman due to
// KIND_EXPERIMENTAL_PROVIDER" / "enabling experimental podman provider"
// banner lines that some kind CLI versions print ahead of the actual
// kubeconfig YAML on the same stream "kind get kubeconfig" writes to.
// Left in place, they corrupt the YAML ("line 3: mapping values are not
// allowed in this context") wherever a downstream kubectl call parses the
// file (confirmed during the DD-TESTING-003 spike). A pure byte-slice
// transform, factored out so it's unit-testable without shelling out.
func stripKindProviderBanner(raw []byte) ([]byte, error) {
	idx := strings.Index(string(raw), "apiVersion:")
	if idx < 0 {
		return nil, fmt.Errorf("kubeconfig output has no apiVersion: field (got: %q)", string(raw))
	}
	return raw[idx:], nil
}
