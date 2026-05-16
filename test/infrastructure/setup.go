package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	kinfra "github.com/jordigilh/kubernaut/test/infrastructure"
)

// SetupE2EInfrastructure is the top-level orchestrator for AF E2E tests.
// It deploys the full kubernaut stack (KA+DS+PostgreSQL+Redis+mock-LLM+DEX+CRDs)
// via kinfra.SetupKubernautAgentInfrastructure, then overlays AF's own image and config.
//
// This alignment with kubernaut's canonical E2E infrastructure ensures minimal
// divergence when the repos merge (per user decision, grounded in kubernaut's
// DD-TEST-001 v2.9 test infrastructure specification).
//
//	Phase 1: Deploy full kubernaut stack via kinfra.SetupKubernautAgentInfrastructure
//	Phase 2: Build AF image
//	Phase 3: Load AF image into Kind cluster
//	Phase 4: Generate AF-specific certs + create secrets
//	Phase 5: Deploy AF via kustomize overlay
//	Phase 6: Wait for AF rollout
func SetupE2EInfrastructure(ctx context.Context, clusterName, kubeconfigPath, namespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "AF E2E Infrastructure Setup (kubernaut-aligned)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	projectRoot := getAFProjectRoot()
	certDir := filepath.Join(os.TempDir(), "apifrontend-e2e-certs")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Deploy full kubernaut stack
	// Ref: kubernaut DD-TEST-001 v2.9 — canonical infra function deploys
	// PostgreSQL, Redis, DataStorage, KA, mock-LLM, DEX, CRDs, inter-service TLS.
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 1: Deploying full kubernaut stack via kinfra.SetupKubernautAgentInfrastructure...")

	if err := kinfra.SetupKubernautAgentInfrastructure(ctx, clusterName, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("kubernaut stack setup failed: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Build AF image
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 2: Building AF image...")

	afImage, err := BuildAFImage(writer)
	if err != nil {
		return fmt.Errorf("failed to build AF image: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  apifrontend: %s\n", afImage)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load AF image into Kind cluster
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 3: Loading AF image into Kind...")

	if err := LoadImageToKind(afImage, "apifrontend", clusterName, writer); err != nil {
		return fmt.Errorf("failed to load AF image: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Generate AF-specific certs + create secrets
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 4: Generating AF TLS certificates...")

	if err := GenerateCerts(certDir, writer); err != nil {
		return fmt.Errorf("failed to generate certs: %w", err)
	}

	if err := CreateNamespace(ctx, kubeconfigPath, namespace, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}
	if err := CreateTLSSecrets(ctx, kubeconfigPath, namespace, certDir, writer); err != nil {
		return fmt.Errorf("failed to create TLS secrets: %w", err)
	}

	_ = os.Setenv("AF_E2E_CERT_DIR", certDir)
	_ = os.Setenv("CERT_DIR", certDir)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5: Deploy AF via kustomize
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 5: Deploying AF via kustomize...")

	kustomizePath := filepath.Join(projectRoot, "deploy/kustomize/overlays/e2e")
	if err := ApplyKustomize(ctx, kubeconfigPath, kustomizePath, writer); err != nil {
		return fmt.Errorf("failed to apply kustomize overlay: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 6: Wait for AF rollout
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\nPHASE 6: Waiting for AF deployment to be ready...")

	if err := WaitForDeploymentRollout(ctx, kubeconfigPath, namespace, "apifrontend", 120*time.Second, writer); err != nil {
		return fmt.Errorf("AF deployment not ready: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  apifrontend ready")

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "AF E2E Infrastructure Ready: Full kubernaut stack + AF")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// TeardownE2EInfrastructure cleans up the Kind cluster.
func TeardownE2EInfrastructure(writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "Tearing down Kind cluster: %s\n", DefaultClusterName)
	return DeleteCluster(writer)
}
