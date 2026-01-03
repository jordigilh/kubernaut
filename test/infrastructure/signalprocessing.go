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
	"runtime"
	"strings"
	"time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// SignalProcessing E2E Infrastructure
// Per IMPLEMENTATION_PLAN_V1.30.md Day 11 and DD-TEST-001 Port Allocation
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Port Allocation (from DD-TEST-001 - AUTHORITATIVE):
//   Internal Metrics: 9090 (container)
//   Internal Health: 8081 (container)
//   Host Port: 8082 (Kind extraPortMappings)
//   NodePort (API): 30082
//   NodePort (Metrics): 30182
//   Host Metrics: 9182 (localhost:9182 → 30182)
//
// Kubeconfig Convention (per TESTING_GUIDELINES.md):
//   Pattern: ~/.kube/{service}-e2e-config
//   Path: ~/.kube/signalprocessing-e2e-config
//   Cluster Name: signalprocessing-e2e
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CreateSignalProcessingCluster creates a Kind cluster for SignalProcessing E2E testing.
// This is called ONCE in BeforeSuite.
//
// Steps:
// 1. Create Kind cluster with SignalProcessing-specific configuration
// 2. Export kubeconfig to ~/.kube/signalprocessing-e2e-config
// 3. Install SignalProcessing CRD (cluster-wide resource)
// 4. Create kubernaut-system namespace
// 5. Deploy Rego policy ConfigMaps
// 6. Build and load SignalProcessing controller image
//
// Time: ~60 seconds
func CreateSignalProcessingCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "SignalProcessing E2E Cluster Setup (ONCE)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. Create Kind cluster
	fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createSignalProcessingKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Install SignalProcessing CRD
	fmt.Fprintln(writer, "📋 Installing SignalProcessing CRD...")
	if err := installSignalProcessingCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install SignalProcessing CRD: %w", err)
	}

	// 2a. Install RemediationRequest CRD (parent of SignalProcessing)
	fmt.Fprintln(writer, "📋 Installing RemediationRequest CRD...")
	if err := installRemediationRequestCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install RemediationRequest CRD: %w", err)
	}

	// 3. Create kubernaut-system namespace
	fmt.Fprintln(writer, "📁 Creating kubernaut-system namespace...")
	if err := createSignalProcessingNamespace(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 4. Deploy Rego policy ConfigMaps
	fmt.Fprintln(writer, "📜 Deploying Rego policy ConfigMaps...")
	if err := deploySignalProcessingPolicies(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy policies: %w", err)
	}

	// 5. Build SignalProcessing controller image
	fmt.Fprintln(writer, "🔨 Building SignalProcessing controller image...")
	if err := buildSignalProcessingImage(writer); err != nil {
		return fmt.Errorf("failed to build controller image: %w", err)
	}

	// 6. Load image into Kind cluster
	fmt.Fprintln(writer, "📦 Loading SignalProcessing image into Kind...")
	if err := loadSignalProcessingImage(clusterName, writer); err != nil {
		return fmt.Errorf("failed to load controller image: %w", err)
	}

	fmt.Fprintln(writer, "✅ SignalProcessing cluster ready for E2E tests")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// DeployDataStorageForSignalProcessing deploys DataStorage infrastructure for BR-SP-090 audit testing.
// This includes PostgreSQL, Redis, migrations, and DataStorage service in kubernaut-system namespace.
//
// Prerequisites:
// - Kind cluster must be created (CreateSignalProcessingCluster)
// - kubernaut-system namespace must exist
//
// This enables the SignalProcessing controller to write audit events to DataStorage.
func DeployDataStorageForSignalProcessing(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "BR-SP-090: Deploying DataStorage for Audit Testing")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := "kubernaut-system"
	clusterName := "signalprocessing-e2e" // Matches CreateSignalProcessingCluster

	// Generate consistent DataStorage image name for build, load, and deploy
	// DD-TEST-001: Use composite tag (service-uuid) for parallel test isolation
	dataStorageImage := GenerateInfraImageName("datastorage", "signalprocessing")
	fmt.Fprintf(writer, "📦 DataStorage image: %s\n", dataStorageImage)

	// 1. Build DataStorage image with dynamic tag
	fmt.Fprintln(writer, "🔨 Building DataStorage image...")
	if err := buildDataStorageImageWithTag(dataStorageImage, writer); err != nil {
		return fmt.Errorf("failed to build DataStorage image: %w", err)
	}

	// 2. Load DataStorage image into Kind with same tag
	fmt.Fprintln(writer, "📦 Loading DataStorage image into Kind...")
	if err := loadDataStorageImageWithTag(clusterName, dataStorageImage, writer); err != nil {
		return fmt.Errorf("failed to load DataStorage image: %w", err)
	}

	// 3. Deploy shared Data Storage infrastructure with same image tag
	fmt.Fprintln(writer, "📦 Deploying Data Storage infrastructure...")
	if err := DeployDataStorageTestServices(ctx, namespace, kubeconfigPath, dataStorageImage, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage infrastructure: %w", err)
	}
	fmt.Fprintln(writer, "✅ Data Storage infrastructure deployed")

	fmt.Fprintln(writer, "✅ DataStorage infrastructure ready for BR-SP-090 audit testing")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// loadDataStorageImageForSP loads the DataStorage image into the SP Kind cluster
func loadDataStorageImageForSP(writer io.Writer) error {
	// Get cluster name - should match what's used in CreateSignalProcessingCluster
	clusterName := "signalprocessing-e2e"

	// Save image to tar (using e2e-test-datastorage tag to match deployment expectations)
	// The deployment in datastorage.go line 833 expects: localhost/kubernaut-datastorage:e2e-test-datastorage
	saveCmd := exec.Command("podman", "save", "localhost/kubernaut-datastorage:e2e-test-datastorage", "-o", "/tmp/datastorage-e2e-sp.tar")
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer

	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load image archive into Kind cluster
	cmd := exec.Command("kind", "load", "image-archive", "/tmp/datastorage-e2e-sp.tar", "--name", clusterName)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load DataStorage image into Kind: %w", err)
	}

	fmt.Fprintln(writer, "  ✅ DataStorage image loaded into Kind cluster")
	return nil
}

// waitForSPDataStorageReady waits for DataStorage service to be ready in SP E2E tests
func waitForSPDataStorageReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	timeout := 120 * time.Second
	interval := 5 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if DataStorage pod is ready
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"get", "pods", "-n", namespace, "-l", "app=datastorage",
			"-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")

		output, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(output)) == "True" {
			// Also check if service is accessible
			healthCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
				"exec", "-n", namespace, "deploy/datastorage", "--",
				"curl", "-sf", "http://127.0.0.1:8080/health")
			if healthCmd.Run() == nil {
				fmt.Fprintln(writer, "  ✅ DataStorage is ready and healthy")
				return nil
			}
		}

		fmt.Fprintln(writer, "  ⏳ DataStorage not ready yet, waiting...")
		time.Sleep(interval)
	}

	return fmt.Errorf("DataStorage not ready after %v", timeout)
}

// SetupSignalProcessingInfrastructureParallel creates the full E2E infrastructure with parallel execution.
// This optimizes setup time by running independent tasks concurrently.
//
// Parallel Execution Strategy:
//
//	Phase 1 (Sequential): Create Kind cluster + CRDs + namespace (~1 min)
//	Phase 2 (PARALLEL):   Load SP image | Build/Load DS image | Deploy PostgreSQL+Redis (~1 min)
//	Phase 3 (Sequential): Deploy DataStorage + migrations (~30s)
//	Phase 4 (Sequential): Deploy SP controller (~30s)
//
// Total time: ~3 minutes (vs ~5.5 minutes sequential)
func SetupSignalProcessingInfrastructureParallel(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "🚀 SignalProcessing E2E Infrastructure (PARALLEL MODE)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := "kubernaut-system"

	// Generate consistent DataStorage image name for build, load, and deploy
	// DD-TEST-001: Use composite tag (service-uuid) for parallel test isolation
	dataStorageImage := GenerateInfraImageName("datastorage", "signalprocessing")
	fmt.Fprintf(writer, "📦 DataStorage image: %s\n", dataStorageImage)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Create Kind cluster (Sequential - must be first)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 1: Creating Kind cluster...")

	// Create Kind cluster
	if err := createSignalProcessingKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// Install CRDs
	fmt.Fprintln(writer, "📋 Installing SignalProcessing CRD...")
	if err := installSignalProcessingCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install SignalProcessing CRD: %w", err)
	}

	fmt.Fprintln(writer, "📋 Installing RemediationRequest CRD...")
	if err := installRemediationRequestCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install RemediationRequest CRD: %w", err)
	}

	// Create namespace
	fmt.Fprintln(writer, "📁 Creating kubernaut-system namespace...")
	if err := createSignalProcessingNamespace(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Deploy Rego policies
	fmt.Fprintln(writer, "📜 Deploying Rego policy ConfigMaps...")
	if err := deploySignalProcessingPolicies(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy policies: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Parallel infrastructure setup
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n⚡ PHASE 2: Parallel infrastructure setup...")
	fmt.Fprintln(writer, "  ├── Building + Loading SP image")
	fmt.Fprintln(writer, "  ├── Building + Loading DS image")
	fmt.Fprintln(writer, "  └── Deploying PostgreSQL + Redis")

	type result struct {
		name string
		err  error
	}

	results := make(chan result, 3)

	// Goroutine 1: Build and load SP image
	go func() {
		var err error
		if buildErr := buildSignalProcessingImage(writer); buildErr != nil {
			err = fmt.Errorf("SP image build failed: %w", buildErr)
		} else if loadErr := loadSignalProcessingImage(clusterName, writer); loadErr != nil {
			err = fmt.Errorf("SP image load failed: %w", loadErr)
		}
		results <- result{name: "SP image", err: err}
	}()

	// Goroutine 2: Build and load DS image with dynamic tag
	go func() {
		var err error
		if buildErr := buildDataStorageImageWithTag(dataStorageImage, writer); buildErr != nil {
			err = fmt.Errorf("DS image build failed: %w", buildErr)
		} else if loadErr := loadDataStorageImageWithTag(clusterName, dataStorageImage, writer); loadErr != nil {
			err = fmt.Errorf("DS image load failed: %w", loadErr)
		}
		results <- result{name: "DS image", err: err}
	}()

	// Goroutine 3: Deploy PostgreSQL and Redis
	go func() {
		var err error
		if pgErr := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); pgErr != nil {
			err = fmt.Errorf("PostgreSQL deploy failed: %w", pgErr)
		} else if redisErr := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); redisErr != nil {
			err = fmt.Errorf("Redis deploy failed: %w", redisErr)
		}
		results <- result{name: "PostgreSQL+Redis", err: err}
	}()

	// Wait for all parallel tasks to complete
	var errors []string
	for i := 0; i < 3; i++ {
		r := <-results
		if r.err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", r.name, r.err))
			fmt.Fprintf(writer, "  ❌ %s failed: %v\n", r.name, r.err)
		} else {
			fmt.Fprintf(writer, "  ✅ %s completed\n", r.name)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("parallel setup failed: %v", errors)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Deploy DataStorage (requires PostgreSQL)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 3: Deploying DataStorage...")

	// Apply migrations
	fmt.Fprintln(writer, "📋 Applying audit migrations...")
	if err := ApplyAuditMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to apply audit migrations: %w", err)
	}

	// Deploy DataStorage service
	fmt.Fprintln(writer, "🚀 Deploying DataStorage service...")
	if err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, GenerateInfraImageName("datastorage", "signalprocessing"), writer); err != nil {
		return fmt.Errorf("failed to deploy DataStorage: %w", err)
	}

	// Wait for DataStorage to be ready
	fmt.Fprintln(writer, "⏳ Waiting for DataStorage to be ready...")
	if err := waitForSPDataStorageReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("DataStorage not ready: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy SignalProcessing controller (requires DataStorage)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n🎮 PHASE 4: Deploying SignalProcessing controller...")
	if err := DeploySignalProcessingController(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy controller: %w", err)
	}

	fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "✅ SignalProcessing E2E infrastructure ready (PARALLEL MODE)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// SetupSignalProcessingInfrastructureWithCoverage sets up E2E infrastructure with coverage capture.
// Per docs/development/testing/E2E_COVERAGE_COLLECTION.md
// This builds the controller with coverage instrumentation and deploys with GOCOVERDIR set.
func SetupSignalProcessingInfrastructureWithCoverage(ctx context.Context, clusterName, kubeconfigPath, coverDir string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "🚀 SignalProcessing E2E Infrastructure (COVERAGE MODE)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintf(writer, "📊 Coverage directory: %s\n", coverDir)

	namespace := "kubernaut-system"

	// Generate consistent DataStorage image name for build, load, and deploy
	// DD-TEST-001: Use composite tag (service-uuid) for parallel test isolation
	dataStorageImage := GenerateInfraImageName("datastorage", "signalprocessing")
	fmt.Fprintf(writer, "📦 DataStorage image: %s\n", dataStorageImage)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Create Kind cluster with coverage mount
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 1: Creating Kind cluster (with coverage mount)...")

	// Create Kind cluster (uses standard config, coverage mount via deployment)
	if err := createSignalProcessingKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// Install CRDs
	fmt.Fprintln(writer, "📋 Installing SignalProcessing CRD...")
	if err := installSignalProcessingCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install SignalProcessing CRD: %w", err)
	}

	fmt.Fprintln(writer, "📋 Installing RemediationRequest CRD...")
	if err := installRemediationRequestCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install RemediationRequest CRD: %w", err)
	}

	// Create namespace
	fmt.Fprintln(writer, "📁 Creating kubernaut-system namespace...")
	if err := createSignalProcessingNamespace(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Deploy Rego policies
	fmt.Fprintln(writer, "📜 Deploying Rego policy ConfigMaps...")
	if err := deploySignalProcessingPolicies(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy policies: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Parallel infrastructure setup (with coverage-enabled SP image)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n⚡ PHASE 2: Parallel infrastructure setup (COVERAGE)...")
	fmt.Fprintln(writer, "  ├── Building + Loading SP image (WITH COVERAGE)")
	fmt.Fprintln(writer, "  ├── Building + Loading DS image")
	fmt.Fprintln(writer, "  └── Deploying PostgreSQL + Redis")

	type result struct {
		name string
		err  error
	}

	results := make(chan result, 3)

	// Goroutine 1: Build and load SP image WITH COVERAGE
	go func() {
		var err error
		if buildErr := BuildSignalProcessingImageWithCoverage(writer); buildErr != nil {
			err = fmt.Errorf("SP coverage image build failed: %w", buildErr)
		} else if loadErr := LoadSignalProcessingCoverageImage(clusterName, writer); loadErr != nil {
			err = fmt.Errorf("SP coverage image load failed: %w", loadErr)
		}
		results <- result{name: "SP image (coverage)", err: err}
	}()

	// Goroutine 2: Build and load DS image with dynamic tag
	go func() {
		var err error
		if buildErr := buildDataStorageImageWithTag(dataStorageImage, writer); buildErr != nil {
			err = fmt.Errorf("DS image build failed: %w", buildErr)
		} else if loadErr := loadDataStorageImageWithTag(clusterName, dataStorageImage, writer); loadErr != nil {
			err = fmt.Errorf("DS image load failed: %w", loadErr)
		}
		results <- result{name: "DS image", err: err}
	}()

	// Goroutine 3: Deploy PostgreSQL and Redis
	go func() {
		var err error
		if pgErr := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); pgErr != nil {
			err = fmt.Errorf("PostgreSQL deploy failed: %w", pgErr)
		} else if redisErr := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); redisErr != nil {
			err = fmt.Errorf("Redis deploy failed: %w", redisErr)
		}
		results <- result{name: "PostgreSQL+Redis", err: err}
	}()

	// Wait for all parallel tasks to complete
	var errors []string
	for i := 0; i < 3; i++ {
		r := <-results
		if r.err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", r.name, r.err))
			fmt.Fprintf(writer, "  ❌ %s failed: %v\n", r.name, r.err)
		} else {
			fmt.Fprintf(writer, "  ✅ %s completed\n", r.name)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("parallel setup failed: %v", errors)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Deploy DataStorage (requires PostgreSQL)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 3: Deploying DataStorage...")

	// Apply migrations
	fmt.Fprintln(writer, "📋 Applying audit migrations...")
	if err := ApplyAuditMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to apply audit migrations: %w", err)
	}

	// Deploy DataStorage service
	fmt.Fprintln(writer, "🚀 Deploying DataStorage service...")
	if err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, GenerateInfraImageName("datastorage", "signalprocessing"), writer); err != nil {
		return fmt.Errorf("failed to deploy DataStorage: %w", err)
	}

	// Wait for DataStorage to be ready
	fmt.Fprintln(writer, "⏳ Waiting for DataStorage to be ready...")
	if err := waitForSPDataStorageReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("DataStorage not ready: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy SignalProcessing controller WITH COVERAGE
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n🎮 PHASE 4: Deploying SignalProcessing controller (WITH COVERAGE)...")
	if err := DeploySignalProcessingControllerWithCoverage(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy coverage controller: %w", err)
	}

	fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "✅ SignalProcessing E2E infrastructure ready (COVERAGE MODE)")
	fmt.Fprintf(writer, "📊 Coverage will be collected in: %s\n", coverDir)
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// DeleteSignalProcessingCluster deletes the Kind cluster and cleans up resources.
func DeleteSignalProcessingCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "🗑️  Deleting SignalProcessing E2E cluster...")

	cmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}

	// Remove kubeconfig file
	if kubeconfigPath != "" {
		_ = os.Remove(kubeconfigPath)
	}

	fmt.Fprintln(writer, "✅ SignalProcessing cluster deleted")
	return nil
}

// DeploySignalProcessingController deploys the controller to the cluster.
// This should be called after CreateSignalProcessingCluster.
func DeploySignalProcessingController(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "🚀 Deploying SignalProcessing controller...")

	// Deploy controller manager
	manifest := signalProcessingControllerManifest()
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy controller: %w", err)
	}

	// Wait for controller to be ready
	fmt.Fprintln(writer, "⏳ Waiting for controller to be ready...")
	return waitForSignalProcessingController(ctx, kubeconfigPath, writer)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Internal helper functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func createSignalProcessingKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Find Kind config file
	configPath := findSignalProcessingKindConfig()
	if configPath == "" {
		return fmt.Errorf("SignalProcessing Kind config not found")
	}

	// Check if cluster already exists
	checkCmd := exec.Command("kind", "get", "clusters")
	output, _ := checkCmd.Output()
	if strings.Contains(string(output), clusterName) {
		fmt.Fprintln(writer, "  Cluster already exists, reusing...")
		return exportSignalProcessingKubeconfig(clusterName, kubeconfigPath, writer)
	}

	// Create cluster
	cmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", configPath,
		"--kubeconfig", kubeconfigPath,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster failed: %w", err)
	}

	// Wait for cluster to be ready
	return waitForSignalProcessingClusterReady(kubeconfigPath, writer)
}

func findSignalProcessingKindConfig() string {
	// Try to find config using various relative paths and also from GOPATH
	paths := []string{
		"test/infrastructure/kind-signalprocessing-config.yaml",
		"../infrastructure/kind-signalprocessing-config.yaml",
		"../../test/infrastructure/kind-signalprocessing-config.yaml",
		"../../../test/infrastructure/kind-signalprocessing-config.yaml",
	}

	// Also try from runtime.Caller to get the file's directory
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		thisDir := filepath.Dir(thisFile)
		paths = append(paths, filepath.Join(thisDir, "kind-signalprocessing-config.yaml"))
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			absPath, _ := filepath.Abs(p)
			return absPath
		}
	}
	return ""
}

func exportSignalProcessingKubeconfig(clusterName, kubeconfigPath string, writer io.Writer) error {
	cmd := exec.Command("kind", "export", "kubeconfig",
		"--name", clusterName,
		"--kubeconfig", kubeconfigPath,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func waitForSignalProcessingClusterReady(kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "  Waiting for cluster to be ready...")
	for i := 0; i < 60; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "nodes", "-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}")
		output, err := cmd.Output()
		if err == nil && strings.Contains(string(output), "True") {
			fmt.Fprintln(writer, "  ✓ Cluster ready")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("cluster not ready after 120 seconds")
}

func installSignalProcessingCRD(kubeconfigPath string, writer io.Writer) error {
	// Find CRD file
	crdPaths := []string{
		"config/crd/bases/kubernaut.ai_signalprocessings.yaml",
		"../../../config/crd/bases/kubernaut.ai_signalprocessings.yaml",
	}

	var crdPath string
	for _, p := range crdPaths {
		if _, err := os.Stat(p); err == nil {
			crdPath, _ = filepath.Abs(p)
			break
		}
	}

	if crdPath == "" {
		return fmt.Errorf("SignalProcessing CRD not found")
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", crdPath)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install CRD: %w", err)
	}

	// Wait for CRD to be established
	fmt.Fprintln(writer, "  Waiting for CRD to be established...")
	for i := 0; i < 30; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "crd", "signalprocessings.kubernaut.ai")
		if err := cmd.Run(); err == nil {
			fmt.Fprintln(writer, "  ✓ CRD established")
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("CRD not established after 30 seconds")
}

func installRemediationRequestCRD(kubeconfigPath string, writer io.Writer) error {
	// Find CRD file
	crdPaths := []string{
		"config/crd/bases/kubernaut.ai_remediationrequests.yaml",
		"../../../config/crd/bases/kubernaut.ai_remediationrequests.yaml",
	}

	var crdPath string
	for _, p := range crdPaths {
		if _, err := os.Stat(p); err == nil {
			crdPath, _ = filepath.Abs(p)
			break
		}
	}

	if crdPath == "" {
		return fmt.Errorf("RemediationRequest CRD not found")
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", crdPath)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install RemediationRequest CRD: %w", err)
	}

	// Wait for CRD to be established
	fmt.Fprintln(writer, "  Waiting for RemediationRequest CRD to be established...")
	for i := 0; i < 30; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "crd", "remediationrequests.kubernaut.ai")
		if err := cmd.Run(); err == nil {
			fmt.Fprintln(writer, "  ✓ RemediationRequest CRD established")
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("RemediationRequest CRD not established after 30 seconds")
}

// installSignalProcessingCRDsBatched installs both SignalProcessing and RemediationRequest CRDs
// in a single kubectl apply command (OPTIMIZATION #2: 3-5s savings)
func installSignalProcessingCRDsBatched(kubeconfigPath string, writer io.Writer) error {
	// Find SignalProcessing CRD file
	spCRDPaths := []string{
		"config/crd/bases/kubernaut.ai_signalprocessings.yaml",
		"../../../config/crd/bases/kubernaut.ai_signalprocessings.yaml",
	}

	var spCRDPath string
	for _, p := range spCRDPaths {
		if _, err := os.Stat(p); err == nil {
			spCRDPath, _ = filepath.Abs(p)
			break
		}
	}

	if spCRDPath == "" {
		return fmt.Errorf("SignalProcessing CRD not found")
	}

	// Find RemediationRequest CRD file
	rrCRDPaths := []string{
		"config/crd/bases/kubernaut.ai_remediationrequests.yaml",
		"../../../config/crd/bases/kubernaut.ai_remediationrequests.yaml",
	}

	var rrCRDPath string
	for _, p := range rrCRDPaths {
		if _, err := os.Stat(p); err == nil {
			rrCRDPath, _ = filepath.Abs(p)
			break
		}
	}

	if rrCRDPath == "" {
		return fmt.Errorf("RemediationRequest CRD not found")
	}

	// Apply both CRDs in a single kubectl call (OPTIMIZATION #2)
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", spCRDPath, "-f", rrCRDPath)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install CRDs (batched): %w", err)
	}

	// Wait for both CRDs to be established
	fmt.Fprintln(writer, "  Waiting for CRDs to be established...")

	// Check SignalProcessing CRD
	for i := 0; i < 30; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "crd", "signalprocessings.kubernaut.ai")
		if err := cmd.Run(); err == nil {
			fmt.Fprintln(writer, "  ✓ SignalProcessing CRD established")
			break
		}
		if i == 29 {
			return fmt.Errorf("SignalProcessing CRD not established after 30 seconds")
		}
		time.Sleep(time.Second)
	}

	// Check RemediationRequest CRD
	for i := 0; i < 30; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "crd", "remediationrequests.kubernaut.ai")
		if err := cmd.Run(); err == nil {
			fmt.Fprintln(writer, "  ✓ RemediationRequest CRD established")
			return nil
		}
		if i == 29 {
			return fmt.Errorf("RemediationRequest CRD not established after 30 seconds")
		}
		time.Sleep(time.Second)
	}

	return nil
}

func createSignalProcessingNamespace(kubeconfigPath string, writer io.Writer) error {
	manifest := `
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-system
  labels:
    app.kubernetes.io/name: kubernaut
    app.kubernetes.io/component: signalprocessing
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func deploySignalProcessingPolicies(kubeconfigPath string, writer io.Writer) error {
	// OPTIMIZATION #1: Batch all 4 Rego ConfigMaps into single kubectl apply
	// Eliminates 3 kubectl invocations + API server round trips (15-30s savings)
	// Per SP_E2E_OPTIMIZATION_TRIAGE_DEC_25_2025.md

	// Combine all 4 policy ConfigMaps into a single YAML manifest
	// NOTE: Using OPA v1.0 syntax with 'if' keyword before rule bodies
	combinedPolicies := `---
# 1. Environment Classification Policy (BR-SP-051)
# Input: {"namespace": {"name": string, "labels": map}, "signal": {"labels": map}}
# Output: {"environment": string, "source": string}
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-environment-policy
  namespace: kubernaut-system
data:
  environment.rego: |
    package signalprocessing.environment

    # Default result: unknown environment
    # OPA v1.0 syntax: requires 'if' keyword before rule body
    default result := {"environment": "unknown", "source": "default"}

    # Primary: Check namespace label kubernaut.ai/environment (BR-SP-051)
    result := {"environment": env, "source": "namespace-labels"} if {
      env := input.namespace.labels["kubernaut.ai/environment"]
      env != ""
    }

    # Fallback: Check namespace name patterns
    result := {"environment": "production", "source": "namespace-name"} if {
      not input.namespace.labels["kubernaut.ai/environment"]
      input.namespace.name == "production"
    }
    result := {"environment": "production", "source": "namespace-name"} if {
      not input.namespace.labels["kubernaut.ai/environment"]
      input.namespace.name == "prod"
    }
    result := {"environment": "staging", "source": "namespace-name"} if {
      not input.namespace.labels["kubernaut.ai/environment"]
      input.namespace.name == "staging"
    }
    result := {"environment": "development", "source": "namespace-name"} if {
      not input.namespace.labels["kubernaut.ai/environment"]
      input.namespace.name == "development"
    }
    result := {"environment": "development", "source": "namespace-name"} if {
      not input.namespace.labels["kubernaut.ai/environment"]
      input.namespace.name == "dev"
    }
---
# 2. Priority Assignment Policy (BR-SP-070)
# Input: {"environment": string, "signal": {"severity": string}}
# Output: {"priority": "P0-P3", "confidence": 0.9}
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-priority-policy
  namespace: kubernaut-system
data:
  priority.rego: |
    package signalprocessing.priority

    # Priority assignment based on environment and severity
    # OPA v1.0 syntax: requires 'if' keyword before rule body
    default result := {"priority": "P3", "confidence": 0.6}

    # Production + critical = P0 (highest urgency)
    result := {"priority": "P0", "confidence": 0.9} if {
      input.environment == "production"
      input.signal.severity == "critical"
    }
    # Production + warning = P1 (high urgency)
    result := {"priority": "P1", "confidence": 0.9} if {
      input.environment == "production"
      input.signal.severity == "warning"
    }
    # Staging + critical = P2 (medium urgency per BR-SP-070)
    result := {"priority": "P2", "confidence": 0.9} if {
      input.environment == "staging"
      input.signal.severity == "critical"
    }
    # Staging + warning = P2 (medium urgency)
    result := {"priority": "P2", "confidence": 0.9} if {
      input.environment == "staging"
      input.signal.severity == "warning"
    }
    # Development = P3 (lowest urgency, regardless of severity)
    result := {"priority": "P3", "confidence": 0.9} if {
      input.environment == "development"
    }
---
# 3. Business Classification Policy (BR-SP-071)
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-business-policy
  namespace: kubernaut-system
data:
  business.rego: |
    package signalprocessing.business

    import rego.v1

    # Default: Return "unknown" with low confidence when no specific rule matches
    # Operators MUST define their own default rules.
    default result := {"business_unit": "unknown", "confidence": 0.0, "policy_name": "operator-default"}

    # Example business unit mappings based on namespace labels
    result := {"business_unit": input.namespace.labels["kubernaut.io/business-unit"], "confidence": 0.95, "policy_name": "namespace-label"} if {
      input.namespace.labels["kubernaut.io/business-unit"]
    }
---
# 4. Custom Labels Extraction Policy (BR-SP-071)
apiVersion: v1
kind: ConfigMap
metadata:
  name: signalprocessing-customlabels-policy
  namespace: kubernaut-system
data:
  customlabels.rego: |
    package signalprocessing.customlabels

    import rego.v1

    # Default: Return empty labels when no specific rule matches
    # Operators define their own label extraction rules.
    default result := {"labels": {}, "policy_name": "operator-default"}

    # Example: Extract labels from namespace annotations
    result := {"labels": extracted, "policy_name": "namespace-annotations"} if {
      input.namespace.annotations
      extracted := {k: v | some k, v in input.namespace.annotations; startswith(k, "kubernaut.io/label-")}
      count(extracted) > 0
    }
`

	// Single kubectl apply for all 4 ConfigMaps
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(combinedPolicies)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy Rego policies (batched): %w", err)
	}

	fmt.Fprintln(writer, "  ✓ Rego policies deployed (batched: environment, priority, business, customlabels)")
	return nil
}

func buildSignalProcessingImage(writer io.Writer) error {
	// Find project root
	projectRoot := findSignalProcessingProjectRoot()
	if projectRoot == "" {
		return fmt.Errorf("project root not found")
	}

	// Use the Dockerfile in docker/ directory
	dockerfilePath := filepath.Join(projectRoot, "docker", "signalprocessing-controller.Dockerfile")

	// Check if Dockerfile exists
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		return fmt.Errorf("SignalProcessing Dockerfile not found at %s", dockerfilePath)
	}

	// Build controller image using podman (preferred) or docker
	containerCmd := "podman"
	if _, err := exec.LookPath("podman"); err != nil {
		containerCmd = "docker"
	}

	// DD-TEST-001 v1.1: Use unique image tag
	imageTag := GetSignalProcessingImageTag()
	imageName := fmt.Sprintf("localhost/signalprocessing-controller:%s", imageTag)
	fmt.Fprintf(writer, "  📦 Image tag: %s (DD-TEST-001 compliant)\n", imageTag)

	cmd := exec.Command(containerCmd, "build",
		"-t", imageName,
		"-f", dockerfilePath,
		projectRoot,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	cmd.Dir = projectRoot

	return cmd.Run()
}

func loadSignalProcessingImage(clusterName string, writer io.Writer) error {
	// When using podman with Kind, we need to save the image to a tar file
	// and load it using kind load image-archive
	// DD-TEST-001 v1.1: Include unique tag in temp file name to avoid collisions
	imageTag := GetSignalProcessingImageTag()
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("signalprocessing-controller-%s.tar", imageTag))
	imageName := fmt.Sprintf("localhost/signalprocessing-controller:%s", imageTag)

	// Save image to tar file
	fmt.Fprintf(writer, "  Saving image to tar file: %s...\n", tmpFile)
	saveCmd := exec.Command("podman", "save",
		"-o", tmpFile,
		imageName,
	)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load image into Kind using image-archive
	fmt.Fprintln(writer, "  Loading image into Kind...")
	loadCmd := exec.Command("kind", "load", "image-archive",
		tmpFile,
		"--name", clusterName,
	)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer
	if err := loadCmd.Run(); err != nil {
		// Cleanup tmp file
		os.Remove(tmpFile)
		return fmt.Errorf("failed to load image: %w", err)
	}

	// DD-TEST-001 v1.1: Cleanup tmp file immediately after load
	os.Remove(tmpFile)
	fmt.Fprintf(writer, "  ✅ Temp tar file cleaned: %s\n", tmpFile)
	return nil
}

func findSignalProcessingProjectRoot() string {
	// Try to find go.mod to determine project root
	paths := []string{
		".",
		"..",
		"../..",
		"../../..",
	}
	for _, p := range paths {
		goMod := filepath.Join(p, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			absPath, _ := filepath.Abs(p)
			return absPath
		}
	}
	return ""
}

// GetProjectRoot returns the project root directory (exported for test suites).
// Per E2E_COVERAGE_COLLECTION.md: needed to locate coverdata directory.
func GetProjectRoot() (string, error) {
	root := findSignalProcessingProjectRoot()
	if root == "" {
		return "", fmt.Errorf("project root not found (go.mod not found)")
	}
	return root, nil
}

func signalProcessingControllerManifest() string {
	// DD-TEST-001 v1.1: Use unique image tag
	imageTag := GetSignalProcessingImageTag()
	imageName := fmt.Sprintf("localhost/signalprocessing-controller:%s", imageTag)

	return fmt.Sprintf(`
apiVersion: v1
kind: ServiceAccount
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: signalprocessing-controller
rules:
# DD-CRD-001: Single API group kubernaut.ai for all CRDs
- apiGroups: ["kubernaut.ai"]
  resources: ["signalprocessings"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["kubernaut.ai"]
  resources: ["signalprocessings/status"]
  verbs: ["get", "update", "patch"]
# BR-SP-003: RemediationRequest access for recovery context
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationrequests"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["pods", "services", "configmaps", "namespaces", "nodes"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["policy"]
  resources: ["poddisruptionbudgets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["networking.k8s.io"]
  resources: ["networkpolicies"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: signalprocessing-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: signalprocessing-controller
subjects:
- kind: ServiceAccount
  name: signalprocessing-controller
  namespace: kubernaut-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
  labels:
    app: signalprocessing-controller
spec:
  replicas: 1
  selector:
    matchLabels:
      app: signalprocessing-controller
  template:
    metadata:
      labels:
        app: signalprocessing-controller
    spec:
      serviceAccountName: signalprocessing-controller
      containers:
      - name: controller
        image: %s
        imagePullPolicy: Never
        ports:
        - containerPort: 9090
          name: metrics
        - containerPort: 8081
          name: health
        env:
        - name: NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        # BR-SP-090: Point to DataStorage service in kubernaut-system namespace
        - name: DATA_STORAGE_URL
          value: "http://datastorage.kubernaut-system.svc.cluster.local:8080"
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          limits:
            cpu: 500m
            memory: 256Mi
          requests:
            cpu: 100m
            memory: 64Mi
        volumeMounts:
        - name: policies
          mountPath: /etc/signalprocessing/policies
          readOnly: true
      volumes:
      - name: policies
        projected:
          sources:
          - configMap:
              name: signalprocessing-environment-policy
          - configMap:
              name: signalprocessing-priority-policy
          - configMap:
              name: signalprocessing-business-policy
          - configMap:
              name: signalprocessing-customlabels-policy
---
apiVersion: v1
kind: Service
metadata:
  name: signalprocessing-controller-metrics
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: signalprocessing-controller
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30182
`, imageName)
}

func waitForSignalProcessingController(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	timeout := 120 * time.Second
	interval := 5 * time.Second
	deadline := time.Now().Add(timeout)
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"rollout", "status", "deployment/signalprocessing-controller",
			"-n", "kubernaut-system", "--timeout=5s")
		if err := cmd.Run(); err == nil {
			fmt.Fprintln(writer, "  ✓ Controller ready")
			return nil
		}

		// Print diagnostic info every 5 attempts (25 seconds)
		if attempt%5 == 0 {
			fmt.Fprintf(writer, "  ⏳ Controller not ready yet (attempt %d)...\n", attempt)
			// Get pod status
			podCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
				"get", "pods", "-n", "kubernaut-system", "-l", "app=signalprocessing-controller", "-o", "wide")
			podCmd.Stdout = writer
			podCmd.Stderr = writer
			_ = podCmd.Run()

			// Get pod logs (last 10 lines)
			logsCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
				"logs", "-n", "kubernaut-system", "-l", "app=signalprocessing-controller", "--tail=10")
			logsCmd.Stdout = writer
			logsCmd.Stderr = writer
			_ = logsCmd.Run()
		}
		time.Sleep(interval)
	}

	// Final diagnostic dump before failure
	fmt.Fprintln(writer, "  ❌ Controller not ready after timeout. Final diagnostics:")
	describeCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"describe", "pod", "-n", "kubernaut-system", "-l", "app=signalprocessing-controller")
	describeCmd.Stdout = writer
	describeCmd.Stderr = writer
	_ = describeCmd.Run()

	logsCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"logs", "-n", "kubernaut-system", "-l", "app=signalprocessing-controller", "--tail=50")
	logsCmd.Stdout = writer
	logsCmd.Stderr = writer
	_ = logsCmd.Run()

	return fmt.Errorf("controller not ready after %v", timeout)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// SignalProcessing Integration Test Infrastructure (Podman Compose)
// Per DD-TEST-001: Port Allocation Strategy
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const (
	// SignalProcessing integration test ports (per DD-TEST-001)
	// NOTE: RO uses 15435/16381, so SP gets the next available
	SignalProcessingIntegrationPostgresPort    = 15436 // PostgreSQL for audit storage
	SignalProcessingIntegrationRedisPort       = 16382 // Redis for DataStorage DLQ
	SignalProcessingIntegrationDataStoragePort = 18094 // DataStorage API for audit events
	SignalProcessingIntegrationMetricsPort     = 19094 // DataStorage metrics port

	// Container names (DD-TEST-002: Programmatic Podman setup)
	SignalProcessingIntegrationPostgresContainer    = "signalprocessing_postgres_test"
	SignalProcessingIntegrationRedisContainer       = "signalprocessing_redis_test"
	SignalProcessingIntegrationDataStorageContainer = "signalprocessing_datastorage_test"
	SignalProcessingIntegrationMigrationsContainer  = "signalprocessing_migrations"
	SignalProcessingIntegrationNetwork              = "signalprocessing_test_network"

	// Database configuration
	SignalProcessingIntegrationDBName     = "kubernaut"
	SignalProcessingIntegrationDBUser     = "kubernaut"
	SignalProcessingIntegrationDBPassword = "kubernaut-test-password"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// DD-TEST-001 v1.1: Unique Image Tags for E2E Testing
// Format: {service}-{user}-{git-hash}-{timestamp}
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// signalProcessingImageTag holds the unique tag for this test run (set once, reused)
var signalProcessingImageTag string

// GetSignalProcessingImageTag returns the unique image tag for this E2E test run.
// Per DD-TEST-001: Format is {service}-{user}-{git-hash}-{timestamp}
// This tag is used for both building and cleanup.
func GetSignalProcessingImageTag() string {
	// Check if already set (avoid regenerating)
	if signalProcessingImageTag != "" {
		return signalProcessingImageTag
	}

	// Check if IMAGE_TAG env var is set (from Makefile or CI)
	if tag := os.Getenv("IMAGE_TAG"); tag != "" {
		signalProcessingImageTag = tag
		return tag
	}

	// Generate unique tag per DD-TEST-001: {service}-{user}-{git-hash}-{timestamp}
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}

	gitHash := getSignalProcessingGitHash()
	timestamp := time.Now().Unix()

	signalProcessingImageTag = fmt.Sprintf("signalprocessing-%s-%s-%d", user, gitHash, timestamp)

	// Export for cleanup in AfterSuite
	os.Setenv("IMAGE_TAG", signalProcessingImageTag)

	return signalProcessingImageTag
}

// getSignalProcessingGitHash returns the short git commit hash
func getSignalProcessingGitHash() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}

// GetDataStorageImageTagForSP returns the DataStorage image tag used for SP E2E tests
// DD-TEST-001 v1.3: Use infrastructure image format for parallel test isolation
// Format: localhost/{infrastructure}:{consumer}-{uuid}
// Example: localhost/datastorage:signalprocessing-1884d074
func GetDataStorageImageTagForSP() string {
	return GenerateInfraImageName("datastorage", "signalprocessing")
}

// GetSignalProcessingFullImageName returns the full image name with unique tag
func GetSignalProcessingFullImageName() string {
	return fmt.Sprintf("localhost/signalprocessing-controller:%s", GetSignalProcessingImageTag())
}

// StartSignalProcessingIntegrationInfrastructure starts the full podman-compose stack for SignalProcessing integration tests
// This includes: PostgreSQL, Redis, and DataStorage API (for BR-SP-090 audit trail)
//
// Port Allocation (per DD-TEST-001):
//   - PostgreSQL:     15435
//   - Redis:          16381
//   - DataStorage:    18094
//
// This function is designed to be called from SynchronizedBeforeSuite (Process 1 only)
func StartSignalProcessingIntegrationInfrastructure(writer io.Writer) error {
	projectRoot := getProjectRoot()

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Starting SignalProcessing Integration Test Infrastructure\n")
	fmt.Fprintf(writer, "Per DD-TEST-002: Sequential Startup Pattern (Programmatic Podman)\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d (RO:15435, SP:15436)\n", SignalProcessingIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:          localhost:%d (RO:16381, SP:16382)\n", SignalProcessingIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", SignalProcessingIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  Pattern:        DD-TEST-002 Sequential Startup (Programmatic Go)\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// ============================================================================
	// STEP 1: Cleanup existing containers (using shared utility)
	// ============================================================================
	fmt.Fprintf(writer, "🧹 Cleaning up existing containers...\n")
	CleanupContainers([]string{
		SignalProcessingIntegrationPostgresContainer,
		SignalProcessingIntegrationRedisContainer,
		SignalProcessingIntegrationDataStorageContainer,
		SignalProcessingIntegrationMigrationsContainer,
	}, writer)
	// Note: No custom network to remove (using host network pattern)
	fmt.Fprintf(writer, "   ✅ Cleanup complete\n\n")

	// ============================================================================
	// STEP 2: Network setup (SKIPPED - using host network for localhost connectivity)
	// ============================================================================
	// Note: Using host network instead of custom podman network to avoid DNS resolution issues
	// All services connect via localhost:PORT (same pattern as Gateway/other successful services)
	fmt.Fprintf(writer, "🌐 Network: Using host network for localhost connectivity\n\n")

	// ============================================================================
	// STEP 3: Start PostgreSQL FIRST (using shared utility)
	// ============================================================================
	fmt.Fprintf(writer, "🐘 Starting PostgreSQL...\n")
	if err := StartPostgreSQL(PostgreSQLConfig{
		ContainerName: SignalProcessingIntegrationPostgresContainer,
		Port:          SignalProcessingIntegrationPostgresPort,
		DBName:        SignalProcessingIntegrationDBName,
		DBUser:        SignalProcessingIntegrationDBUser,
		DBPassword:    SignalProcessingIntegrationDBPassword,
		// No Network field - use host network (default)
	}, writer); err != nil {
		return fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	// CRITICAL: Wait for PostgreSQL to be ready before proceeding (using shared utility)
	fmt.Fprintf(writer, "⏳ Waiting for PostgreSQL to be ready...\n")
	if err := WaitForPostgreSQLReady(SignalProcessingIntegrationPostgresContainer, SignalProcessingIntegrationDBUser, SignalProcessingIntegrationDBName, writer); err != nil {
		return fmt.Errorf("PostgreSQL failed to become ready: %w", err)
	}
	fmt.Fprintf(writer, "\n")

	// ============================================================================
	// STEP 4: Run migrations (SP-specific)
	// ============================================================================
	fmt.Fprintf(writer, "🔄 Running database migrations...\n")
	if err := runSPMigrations(projectRoot, writer); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ Migrations applied successfully\n\n")

	// ============================================================================
	// STEP 5: Start Redis SECOND (using shared utility)
	// ============================================================================
	fmt.Fprintf(writer, "🔴 Starting Redis...\n")
	if err := StartRedis(RedisConfig{
		ContainerName: SignalProcessingIntegrationRedisContainer,
		Port:          SignalProcessingIntegrationRedisPort,
	}, writer); err != nil {
		return fmt.Errorf("failed to start Redis: %w", err)
	}

	// Wait for Redis to be ready (using shared utility)
	fmt.Fprintf(writer, "⏳ Waiting for Redis to be ready...\n")
	if err := WaitForRedisReady(SignalProcessingIntegrationRedisContainer, writer); err != nil {
		return fmt.Errorf("Redis failed to become ready: %w", err)
	}
	fmt.Fprintf(writer, "\n")

	// ============================================================================
	// STEP 6: Start DataStorage LAST (SP-specific)
	// ============================================================================
	fmt.Fprintf(writer, "📦 Starting DataStorage service...\n")
	if err := startSPDataStorage(projectRoot, writer); err != nil {
		return fmt.Errorf("failed to start DataStorage: %w", err)
	}

	// CRITICAL: Wait for DataStorage HTTP endpoint to be ready (using shared utility)
	fmt.Fprintf(writer, "⏳ Waiting for DataStorage HTTP endpoint to be ready...\n")
	if err := WaitForHTTPHealth(
		fmt.Sprintf("http://127.0.0.1:%d/health", SignalProcessingIntegrationDataStoragePort),
		60*time.Second,
		writer,
	); err != nil{
		// Print container logs for debugging
		fmt.Fprintf(writer, "\n⚠️  DataStorage failed to become healthy. Container logs:\n")
		logsCmd := exec.Command("podman", "logs", SignalProcessingIntegrationDataStorageContainer)
		logsCmd.Stdout = writer
		logsCmd.Stderr = writer
		_ = logsCmd.Run()
		return fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	fmt.Fprintf(writer, "\n")

	// ============================================================================
	// SUCCESS
	// ============================================================================
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "✅ SignalProcessing Integration Infrastructure Ready\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:        localhost:%d\n", SignalProcessingIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:             localhost:%d\n", SignalProcessingIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage HTTP:  http://localhost:%d\n", SignalProcessingIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  DataStorage Metrics: http://localhost:%d\n", SignalProcessingIntegrationMetricsPort)
	fmt.Fprintf(writer, "  Database:          %s\n", SignalProcessingIntegrationDBName)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return nil
}

// StopSignalProcessingIntegrationInfrastructure stops and cleans up the SignalProcessing integration test infrastructure
func StopSignalProcessingIntegrationInfrastructure(writer io.Writer) error {
	fmt.Fprintf(writer, "🛑 Stopping SignalProcessing Integration Infrastructure (DD-TEST-002)...\n")

	// Stop and remove containers (using shared utility)
	CleanupContainers([]string{
		SignalProcessingIntegrationDataStorageContainer,
		SignalProcessingIntegrationRedisContainer,
		SignalProcessingIntegrationPostgresContainer,
		SignalProcessingIntegrationMigrationsContainer,
	}, writer)

	// Note: No custom network to remove (using host network pattern)

	// DD-INTEGRATION-001 v2.0: Clean up composite-tagged DataStorage image
	// This ensures images with UUID tags don't accumulate between test runs
	fmt.Fprintf(writer, "   Cleaning up DataStorage image (composite tag)...\n")
	pruneCmd := exec.Command("podman", "image", "prune", "-f", "--all")
	if err := pruneCmd.Run(); err != nil {
		fmt.Fprintf(writer, "   ⚠️  Warning: Failed to prune images: %v\n", err)
	} else {
		fmt.Fprintf(writer, "   ✅ Images pruned\n")
	}

	fmt.Fprintf(writer, "✅ SignalProcessing Integration Infrastructure stopped and cleaned up\n")
	return nil
}

// Service-specific helper functions (common functions moved to shared_integration_utils.go)

// runSPMigrations applies database migrations for SignalProcessing
func runSPMigrations(projectRoot string, writer io.Writer) error {
	migrationsDir := filepath.Join(projectRoot, "migrations")

	// Apply migrations: extract only "Up" sections (stop at "-- +goose Down")
	migrationScript := `
		set -e
		echo "Applying migrations (Up sections only)..."
		find /migrations -maxdepth 1 -name '*.sql' -type f | sort | while read f; do
			echo "Applying $f..."
			sed -n '1,/^-- +goose Down/p' "$f" | grep -v '^-- +goose Down' | psql
		done
		echo "Migrations complete!"
	`

	// Use host.containers.internal for macOS compatibility (Podman VM can reach host)
	// Connect to PostgreSQL on host port (not internal container port)
	cmd := exec.Command("podman", "run", "--rm",
		"--name", SignalProcessingIntegrationMigrationsContainer,
		"-v", fmt.Sprintf("%s:/migrations:ro", migrationsDir),
		"-e", "PGHOST=host.containers.internal",
		"-e", fmt.Sprintf("PGPORT=%d", SignalProcessingIntegrationPostgresPort),
		"-e", "PGUSER="+SignalProcessingIntegrationDBUser,
		"-e", "PGPASSWORD="+SignalProcessingIntegrationDBPassword,
		"-e", "PGDATABASE="+SignalProcessingIntegrationDBName,
		"postgres:16-alpine",
		"bash", "-c", migrationScript,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// startSPDataStorage starts the DataStorage service for SignalProcessing
func startSPDataStorage(projectRoot string, writer io.Writer) error {
	// Per DD-INTEGRATION-001 v2.0: Use composite image tag for collision avoidance
	dsImage := GetDataStorageImageTagForSP()

	// Always build with unique tag (no image reuse to ensure fresh builds)
	fmt.Fprintf(writer, "   Building DataStorage image: %s\n", dsImage)
	buildCmd := exec.Command("podman", "build",
		"-t", dsImage,
		"-f", filepath.Join(projectRoot, "docker", "data-storage.Dockerfile"),
		projectRoot,
	)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build DataStorage image: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ DataStorage image built: %s\n", dsImage)

	// Mount config directory
	configDir := filepath.Join(projectRoot, "test", "integration", "signalprocessing", "config")

	// Use host network pattern (same as Gateway) - no --network flag
	// Connect to PostgreSQL and Redis via localhost (not container names)
	cmd := exec.Command("podman", "run", "-d",
		"--name", SignalProcessingIntegrationDataStorageContainer,
		"-p", fmt.Sprintf("%d:8080", SignalProcessingIntegrationDataStoragePort),
		"-v", fmt.Sprintf("%s:/etc/datastorage:ro", configDir),
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		dsImage,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// E2E COVERAGE CAPTURE (per E2E_COVERAGE_COLLECTION.md)
// Go 1.20+ binary profiling for E2E coverage measurement
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CoverageDir is the directory where E2E coverage data is written
const CoverageDir = "coverdata"

// BuildSignalProcessingImageWithCoverage builds the controller image with coverage instrumentation.
// Per E2E_COVERAGE_COLLECTION.md: Uses GOFLAGS=-cover to enable binary profiling.
// Uses the standard Dockerfile with --build-arg GOFLAGS=-cover.
func BuildSignalProcessingImageWithCoverage(writer io.Writer) error {
	projectRoot := findSignalProcessingProjectRoot()
	if projectRoot == "" {
		return fmt.Errorf("project root not found")
	}

	dockerfilePath := filepath.Join(projectRoot, "docker", "signalprocessing-controller.Dockerfile")
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		return fmt.Errorf("SignalProcessing Dockerfile not found at %s", dockerfilePath)
	}

	containerCmd := "podman"
	if _, err := exec.LookPath("podman"); err != nil {
		containerCmd = "docker"
	}

	// Use unique image tag with coverage suffix
	imageTag := GetSignalProcessingImageTag() + "-coverage"
	imageName := fmt.Sprintf("localhost/signalprocessing-controller:%s", imageTag)
	fmt.Fprintf(writer, "  📦 Building with coverage: %s\n", imageName)

	// Build with GOFLAGS=-cover passed as build arg
	cmd := exec.Command(containerCmd, "build",
		"-t", imageName,
		"-f", dockerfilePath,
		"--build-arg", "GOFLAGS=-cover",
		projectRoot,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	cmd.Dir = projectRoot

	if err := cmd.Run(); err != nil {
		return err
	}

	// PROFILING: Get image size for optimization analysis
	sizeCmd := exec.Command(containerCmd, "images", "--format", "{{.Size}}", imageName)
	sizeOutput, err := sizeCmd.Output()
	if err == nil {
		fmt.Fprintf(writer, "  📊 Image size: %s\n", string(sizeOutput))
	}

	return nil
}

// GetSignalProcessingCoverageImageTag returns the coverage-enabled image tag
func GetSignalProcessingCoverageImageTag() string {
	return GetSignalProcessingImageTag() + "-coverage"
}

// GetSignalProcessingCoverageFullImageName returns the full image name with coverage tag
func GetSignalProcessingCoverageFullImageName() string {
	return fmt.Sprintf("localhost/signalprocessing-controller:%s", GetSignalProcessingCoverageImageTag())
}

// LoadSignalProcessingCoverageImage loads the coverage-enabled image into Kind
func LoadSignalProcessingCoverageImage(clusterName string, writer io.Writer) error {
	imageTag := GetSignalProcessingCoverageImageTag()
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("signalprocessing-controller-%s.tar", imageTag))
	imageName := GetSignalProcessingCoverageFullImageName()

	fmt.Fprintf(writer, "  Saving coverage image to tar file: %s...\n", tmpFile)
	saveCmd := exec.Command("podman", "save",
		"-o", tmpFile,
		imageName,
	)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	fmt.Fprintln(writer, "  Loading coverage image into Kind...")
	loadCmd := exec.Command("kind", "load", "image-archive",
		tmpFile,
		"--name", clusterName,
	)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer
	if err := loadCmd.Run(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to load image: %w", err)
	}

	os.Remove(tmpFile)
	fmt.Fprintf(writer, "  ✅ Coverage image loaded and temp file cleaned\n")
	return nil
}

// signalProcessingControllerCoverageManifest returns the controller manifest with GOCOVERDIR set
func signalProcessingControllerCoverageManifest() string {
	imageTag := GetSignalProcessingCoverageImageTag()
	imageName := fmt.Sprintf("localhost/signalprocessing-controller:%s", imageTag)

	return fmt.Sprintf(`
apiVersion: v1
kind: ServiceAccount
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: signalprocessing-controller
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["signalprocessings", "remediationrequests"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["kubernaut.ai"]
  resources: ["signalprocessings/status", "signalprocessings/finalizers"]
  verbs: ["get", "update", "patch"]
- apiGroups: [""]
  resources: ["pods", "services", "namespaces", "nodes", "configmaps", "secrets", "events"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["policy"]
  resources: ["poddisruptionbudgets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["networking.k8s.io"]
  resources: ["networkpolicies"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: signalprocessing-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: signalprocessing-controller
subjects:
- kind: ServiceAccount
  name: signalprocessing-controller
  namespace: kubernaut-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: signalprocessing-controller
  namespace: kubernaut-system
  labels:
    app: signalprocessing-controller
spec:
  replicas: 1
  selector:
    matchLabels:
      app: signalprocessing-controller
  template:
    metadata:
      labels:
        app: signalprocessing-controller
    spec:
      serviceAccountName: signalprocessing-controller
      terminationGracePeriodSeconds: 30
      # E2E Coverage: Run as root to write to hostPath volume (acceptable for E2E tests)
      securityContext:
        runAsUser: 0
        runAsGroup: 0
      containers:
      - name: controller
        image: %s
        imagePullPolicy: Never
        env:
        - name: NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        # BR-SP-090: Point to DataStorage service in kubernaut-system namespace
        - name: DATA_STORAGE_URL
          value: "http://datastorage.kubernaut-system.svc.cluster.local:8080"
        # E2E Coverage: Set GOCOVERDIR to enable coverage capture
        - name: GOCOVERDIR
          value: /coverdata
        ports:
        - containerPort: 9090
          name: metrics
        - containerPort: 8081
          name: health
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 256Mi
        volumeMounts:
        # Mount policies at /etc/signalprocessing/policies (same as standard manifest)
        - name: policies
          mountPath: /etc/signalprocessing/policies
          readOnly: true
        # E2E Coverage: Mount coverage directory
        - name: coverdata
          mountPath: /coverdata
      volumes:
      # Projected volume for all policies (same as standard manifest)
      - name: policies
        projected:
          sources:
          - configMap:
              name: signalprocessing-environment-policy
          - configMap:
              name: signalprocessing-priority-policy
          - configMap:
              name: signalprocessing-business-policy
          - configMap:
              name: signalprocessing-customlabels-policy
      # E2E Coverage: hostPath volume for coverage data
      - name: coverdata
        hostPath:
          path: /coverdata
          type: DirectoryOrCreate
---
apiVersion: v1
kind: Service
metadata:
  name: signalprocessing-controller-metrics
  namespace: kubernaut-system
  labels:
    app: signalprocessing-controller
spec:
  type: NodePort
  ports:
  - port: 9090
    targetPort: 9090
    nodePort: 30182
    name: metrics
  selector:
    app: signalprocessing-controller
`, imageName)
}

// applyManifest applies a Kubernetes manifest using kubectl
func applyManifest(kubeconfigPath, manifest string, writer io.Writer) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// DeploySignalProcessingControllerWithCoverage deploys the coverage-enabled controller
func DeploySignalProcessingControllerWithCoverage(kubeconfigPath string, writer io.Writer) error {
	manifest := signalProcessingControllerCoverageManifest()

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply coverage controller manifest: %w", err)
	}

	// Wait for controller to be ready
	fmt.Fprintln(writer, "⏳ Waiting for coverage-enabled controller to be ready...")
	ctx := context.Background()
	return waitForSignalProcessingController(ctx, kubeconfigPath, writer)
}

// ScaleDownControllerForCoverage scales the controller to 0 to trigger graceful shutdown
// and flush coverage data to /coverdata
func ScaleDownControllerForCoverage(kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "📊 Scaling down controller for coverage flush...")

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"scale", "deployment", "signalprocessing-controller",
		"-n", "kubernaut-system", "--replicas=0")
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to scale down controller: %w", err)
	}

	// Wait for pod to terminate using kubectl wait (blocks until pod is deleted)
	fmt.Fprintln(writer, "⏳ Waiting for controller pod to terminate...")
	waitCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"wait", "--for=delete", "pod",
		"-l", "app=signalprocessing-controller",
		"-n", "kubernaut-system",
		"--timeout=60s")
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	_ = waitCmd.Run() // Ignore error if no pods exist

	// Coverage data is written on SIGTERM before pod exits, no additional wait needed
	// The kubectl wait --for=delete already blocks until pod is fully terminated

	fmt.Fprintln(writer, "✅ Controller scaled down, coverage data should be flushed")
	return nil
}

// ExtractCoverageFromKind copies coverage data from Kind node to host
func ExtractCoverageFromKind(clusterName, coverDir string, writer io.Writer) error {
	fmt.Fprintln(writer, "📦 Extracting coverage data from Kind node...")

	// Get the worker node container name
	workerNode := clusterName + "-worker"

	// Create local coverage directory if not exists
	if err := os.MkdirAll(coverDir, 0755); err != nil {
		return fmt.Errorf("failed to create coverage directory: %w", err)
	}

	// Copy coverage files from Kind node to host
	cmd := exec.Command("docker", "cp",
		workerNode+":/coverdata/.",
		coverDir)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		// Try with podman if docker fails
		cmd = exec.Command("podman", "cp",
			workerNode+":/coverdata/.",
			coverDir)
		cmd.Stdout = writer
		cmd.Stderr = writer
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to copy coverage data: %w", err)
		}
	}

	// List extracted files
	files, _ := os.ReadDir(coverDir)
	if len(files) == 0 {
		fmt.Fprintln(writer, "⚠️  No coverage files found (controller may not have processed any requests)")
	} else {
		fmt.Fprintf(writer, "✅ Extracted %d coverage files\n", len(files))
	}

	return nil
}

// GenerateCoverageReport generates coverage report from extracted data
func GenerateCoverageReport(coverDir string, writer io.Writer) error {
	fmt.Fprintln(writer, "📊 Generating E2E coverage report...")

	// Check if coverage data exists
	files, err := os.ReadDir(coverDir)
	if err != nil || len(files) == 0 {
		fmt.Fprintln(writer, "⚠️  No coverage data to report")
		return nil
	}

	// Generate percent summary
	cmd := exec.Command("go", "tool", "covdata", "percent", "-i="+coverDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate coverage percent: %w\n%s", err, output)
	}
	fmt.Fprintf(writer, "\n%s\n", output)

	// Generate text format for HTML conversion
	textFile := filepath.Join(coverDir, "e2e-coverage.txt")
	cmd = exec.Command("go", "tool", "covdata", "textfmt",
		"-i="+coverDir,
		"-o="+textFile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate text format: %w", err)
	}

	// Generate HTML report
	htmlFile := filepath.Join(coverDir, "e2e-coverage.html")
	cmd = exec.Command("go", "tool", "cover",
		"-html="+textFile,
		"-o="+htmlFile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate HTML report: %w", err)
	}

	fmt.Fprintf(writer, "📄 Text report: %s\n", textFile)
	fmt.Fprintf(writer, "📄 HTML report: %s\n", htmlFile)
	fmt.Fprintln(writer, "✅ E2E coverage report generated")

	return nil
}
