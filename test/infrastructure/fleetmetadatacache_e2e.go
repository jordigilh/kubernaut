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
	"os"
)

// dexHostPortFMC is the Kind extraPortMappings host port for Dex in
// kind-fleetmetadatacache-config.yaml. Matches the NodePort (30556) directly,
// following the fullpipeline/fleet convention rather than the KA/AF one
// (which maps host 5556) -- see waitForDexReady in dex_e2e.go.
const dexHostPortFMC = 30556

// SetupFMCE2EInfrastructure deploys the dedicated Fleet Metadata Cache (FMC)
// E2E stack: DataStorage (audit trail dependency, per AGENTS.md) + DEX (OAuth2
// IdP) + the fleet-core stack (Istio + Kuadrant MCP Gateway + kube-mcp-server +
// Valkey + FMC) via DeployFleetCoreInfra.
//
// Unlike the "fleet" E2E suite (test/e2e/fleet/), this lane does NOT deploy
// Gateway, RemediationOrchestrator, or the other 8+ Kubernaut services --
// it proves FMC's own journeys in isolation (BR-INTEGRATION-065):
//   - Real OAuth2 client_credentials token acquisition from DEX
//   - Real discovery of MCPServerRegistrations via the Kuadrant MCP Gateway
//   - Real Valkey-backed scope resolution served by FMC's HTTP API
//
// Authority: Issue #54, ADR-068, BR-INTEGRATION-065.
func SetupFMCE2EInfrastructure(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) (fmcImage string, err error) {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 Fleet Metadata Cache (FMC) E2E Infrastructure")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Deploys: DataStorage + DEX + Fleet Core (Istio/Kuadrant/kube-mcp-server/Valkey/FMC)")
	_, _ = fmt.Fprintln(writer, "  Skips: Gateway, RemediationOrchestrator, and other Kubernaut services")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := "kubernaut-system"

	// ── Phase 1: Build images in parallel (before cluster creation) ─────
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building fleetmetadatacache + datastorage images (NO CLUSTER YET)...")

	type buildResult struct {
		name      string
		imageName string
		err       error
	}
	buildResults := make(chan buildResult, 2)

	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "fleetmetadatacache",
			ImageName:        "fleetmetadatacache",
			DockerfilePath:   "docker/fleetmetadatacache.Dockerfile",
			BuildContextPath: "",
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
		}
		imageName, buildErr := BuildImageForKind(cfg, writer)
		if buildErr != nil {
			buildErr = fmt.Errorf("fleetmetadatacache image build failed: %w", buildErr)
		}
		buildResults <- buildResult{name: "FleetMetadataCache", imageName: imageName, err: buildErr}
	}()

	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "datastorage",
			ImageName:        "kubernaut/datastorage",
			DockerfilePath:   "docker/data-storage.Dockerfile",
			BuildContextPath: "",
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
		}
		imageName, buildErr := BuildImageForKind(cfg, writer)
		if buildErr != nil {
			buildErr = fmt.Errorf("datastorage image build failed: %w", buildErr)
		}
		buildResults <- buildResult{name: "DataStorage", imageName: imageName, err: buildErr}
	}()

	var dsImage string
	var buildErrs []string
	for i := 0; i < 2; i++ {
		r := <-buildResults
		if r.err != nil {
			buildErrs = append(buildErrs, r.err.Error())
			continue
		}
		_, _ = fmt.Fprintf(writer, "  ✅ %s image built: %s\n", r.name, r.imageName)
		switch r.name {
		case "FleetMetadataCache":
			fmcImage = r.imageName
		case "DataStorage":
			dsImage = r.imageName
		}
	}
	if len(buildErrs) > 0 {
		return "", fmt.Errorf("image build(s) failed: %s", joinErrs(buildErrs))
	}

	// ── Phase 2: Create Kind cluster + namespace ─────────────────────────
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster + namespace...")
	opts := KindClusterOptions{
		ClusterName:             clusterName,
		KubeconfigPath:          kubeconfigPath,
		ConfigPath:              "test/infrastructure/kind-fleetmetadatacache-config.yaml",
		WaitTimeout:             "5m",
		DeleteExisting:          false,
		ReuseExisting:           false,
		UsePodman:               true,
		ProjectRootAsWorkingDir: true, // DD-TEST-007: For ./coverdata resolution
	}
	if clusterErr := CreateKindClusterWithConfig(opts, writer); clusterErr != nil {
		return "", fmt.Errorf("failed to create Kind cluster: %w", clusterErr)
	}

	if nsErr := createTestNamespace(namespace, kubeconfigPath, writer); nsErr != nil {
		return "", fmt.Errorf("failed to create namespace: %w", nsErr)
	}

	// ── Phase 3: Load images into Kind ───────────────────────────────────
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into Kind...")
	if loadErr := LoadImageToKind(fmcImage, "fleetmetadatacache", clusterName, writer); loadErr != nil {
		return "", fmt.Errorf("failed to load fleetmetadatacache image: %w", loadErr)
	}
	if loadErr := LoadImageToKind(dsImage, "datastorage", clusterName, writer); loadErr != nil {
		return "", fmt.Errorf("failed to load datastorage image: %w", loadErr)
	}

	// ── Phase 4: TLS + signing certs (must precede DEX and DataStorage) ─
	_, _ = fmt.Fprintln(writer, "\n🔐 PHASE 4: Generating inter-service TLS + audit signing certs...")
	if _, tlsErr := GenerateInterServiceTLS(ctx, kubeconfigPath, namespace, writer); tlsErr != nil {
		return "", fmt.Errorf("failed to generate inter-service TLS: %w", tlsErr)
	}
	if signErr := GenerateSigningCertSecret(ctx, kubeconfigPath, namespace, writer); signErr != nil {
		return "", fmt.Errorf("failed to generate signing certificate: %w", signErr)
	}

	// ── Phase 5: DataStorage (audit trail dependency) ────────────────────
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 5: Deploying DataStorage...")
	if dsErr := DeployDataStorageTestServices(ctx, namespace, kubeconfigPath, dsImage, writer); dsErr != nil {
		return "", fmt.Errorf("failed to deploy DataStorage: %w", dsErr)
	}

	// ── Phase 6: DEX OIDC provider (must be ready before API server OIDC patch) ─
	_, _ = fmt.Fprintln(writer, "\n🔑 PHASE 6: Deploying DEX OIDC provider...")
	if dexErr := DeployDexInfra(ctx, namespace, kubeconfigPath, dexHostPortFMC, writer); dexErr != nil {
		return "", fmt.Errorf("failed to deploy DEX: %w", dexErr)
	}

	// ── Phase 7: Patch API server for OIDC (needs DEX already running) ──
	_, _ = fmt.Fprintln(writer, "\n🔑 PHASE 7: Patching API server for OIDC...")
	if oidcErr := patchAPIServerForOIDC(ctx, clusterName, kubeconfigPath, writer); oidcErr != nil {
		return "", fmt.Errorf("API server OIDC patching failed: %w", oidcErr)
	}

	// ── Phase 8: Fleet core (Istio + Kuadrant + kube-mcp-server + Valkey + FMC) ─
	_, _ = fmt.Fprintln(writer, "\n🌐 PHASE 8: Deploying fleet-core infrastructure...")
	if coreErr := DeployFleetCoreInfra(ctx, namespace, kubeconfigPath, fmcImage, writer); coreErr != nil {
		return "", fmt.Errorf("fleet-core infra deployment failed: %w", coreErr)
	}

	if readyErr := WaitForFleetReady(writer); readyErr != nil {
		return "", fmt.Errorf("fleet readiness check failed: %w", readyErr)
	}

	// ── Phase 9: Expose FMC's own API via NodePort ───────────────────────
	// DD-TEST-001 mandates NodePort over kubectl port-forward for E2E test
	// stability. DeployFleetCoreInfra only creates a ClusterIP Service for
	// FMC (fleetmetadatacache-service, for in-cluster GW/RO callers), so this
	// is an additive Service selecting the same pods -- no shared-code change.
	_, _ = fmt.Fprintln(writer, "\n🔌 PHASE 9: Exposing FMC API via NodePort (DD-TEST-001)...")
	fmcNodePortManifest := fmt.Sprintf(`---
apiVersion: v1
kind: Service
metadata:
  name: fleetmetadatacache-e2e-nodeport
  namespace: %s
  labels:
    app: fleetmetadatacache
    component: fleet-e2e
spec:
  type: NodePort
  selector:
    app: fleetmetadatacache
  ports:
  - name: api
    port: 8080
    targetPort: 8080
    nodePort: 30150
`, namespace)
	if npErr := kubectlApplyManifest(ctx, kubeconfigPath, writer, fmcNodePortManifest); npErr != nil {
		return "", fmt.Errorf("failed to expose FMC API via NodePort: %w", npErr)
	}
	_, _ = fmt.Fprintln(writer, "    ✅ FMC API reachable at http://localhost:8150")

	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ FMC E2E Infrastructure READY")
	_, _ = fmt.Fprintln(writer, "  FMC API:      http://localhost:8150")
	_, _ = fmt.Fprintln(writer, "  MCP Gateway:  http://localhost:31975/mcp")
	_, _ = fmt.Fprintln(writer, "  DataStorage:  https://localhost:30081")
	_, _ = fmt.Fprintln(writer, "  DEX:          https://localhost:30556/dex")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return fmcImage, nil
}

func joinErrs(errs []string) string {
	out := errs[0]
	for _, e := range errs[1:] {
		out += "; " + e
	}
	return out
}
