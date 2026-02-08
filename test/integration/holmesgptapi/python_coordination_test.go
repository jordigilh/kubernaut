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

package holmesgptapi

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Python Test Coordination", func() {
	Context("Infrastructure Lifecycle Management", func() {
		It("should run Python integration tests against Go infrastructure", func() {
			// Pattern: Go infrastructure (Ginkgo) + Python tests (pytest in UBI9 container)
			// Architecture:
			// - Go: Sets up envtest, PostgreSQL, Redis, DataStorage (with DD-AUTH-014)
			// - Python: Runs in UBI9 container with --network=host, installs deps at runtime
			// - Same as E2E: No custom Docker image, uses registry.access.redhat.com/ubi9/python-312
			// - Coordination: Python creates signal file when complete

			signalFile := "/tmp/hapi-integration-tests-complete"

			// Clean up any stale signal file from previous run
			_ = os.Remove(signalFile)

			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("🐍 Running Python integration tests in UBI9 container...")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("")
			GinkgoWriter.Println("Infrastructure Status (provided by Go):")
			GinkgoWriter.Println("  ✅ envtest (Kubernetes API with auth)")
			GinkgoWriter.Println("  ✅ PostgreSQL (port 15439)")
			GinkgoWriter.Println("  ✅ Redis (port 16387)")
			GinkgoWriter.Println("  ✅ Data Storage API (port 18098, DD-AUTH-014)")
			GinkgoWriter.Println("")
			GinkgoWriter.Println("Python Test Container:")
			GinkgoWriter.Println("  • Image: registry.access.redhat.com/ubi9/python-312:latest")
			GinkgoWriter.Println("  • Network: host (direct access to Go infrastructure)")
			GinkgoWriter.Println("  • Tests: pytest tests/integration/ (business logic)")
			GinkgoWriter.Println("  • Pattern: Same as E2E - runtime dependency install (no custom image)")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// Get workspace root for volume mount
			workspaceRoot, err := filepath.Abs("../../..")
			Expect(err).NotTo(HaveOccurred())

			// Write token to temporary file for mounting into container
			// DD-AUTH-014: Python ServiceAccountAuthPoolManager expects token at standard K8s path
			// NOTE: Use workspace path (not /tmp) for podman VM compatibility on macOS
			tokenFile := filepath.Join(workspaceRoot, ".hapi-integration-sa-token")
			GinkgoWriter.Printf("🔐 Writing ServiceAccount token to %s...\n", tokenFile)
			GinkgoWriter.Printf("   Token length: %d chars\n", len(serviceAccountToken))
			err = os.WriteFile(tokenFile, []byte(serviceAccountToken), 0644)
			Expect(err).NotTo(HaveOccurred(), "Failed to write ServiceAccount token file")

			// Verify file exists
			if _, err := os.Stat(tokenFile); err != nil {
				Fail(fmt.Sprintf("Token file verification failed: %v", err))
			}
			GinkgoWriter.Printf("✅ Token file written and verified: %s\n", tokenFile)
			defer func() { _ = os.Remove(tokenFile) }() // Explicitly ignore - test cleanup

			// ========================================
			// Run pytest in UBI9 Python container (NO custom image build)
			// Pattern: Same as E2E tests - install deps at runtime
			// Benefits: Simpler, no custom Dockerfile, consistent with E2E
			// ========================================
			GinkgoWriter.Println("🐍 Running Python tests in UBI9 container (runtime deps)...")

			// Build pytest command with dependency installation and Python coverage
			// NOTE: Must install holmesgpt first to avoid httpx version conflicts
			// Same pattern as custom Dockerfile (holmesgpt-api-integration-test.Dockerfile)
			// Coverage: COVERAGE_FILE=/tmp/.coverage to avoid permission errors on bind mount
			//           Output redirected from outside container to avoid permission issues
			pytestCmd := fmt.Sprintf(
				"cd /workspace && " +
					"pip install -q --break-system-packages dependencies/holmesgpt && " +
					"cd holmesgpt-api && " +
					"grep -v '../dependencies/holmesgpt' requirements.txt > /tmp/requirements-filtered.txt && " +
					"pip install -q --break-system-packages -r /tmp/requirements-filtered.txt -r requirements-test.txt && " +
					"COVERAGE_FILE=/tmp/.coverage PYTHONUNBUFFERED=1 HAPI_URL=http://127.0.0.1:18120 DATA_STORAGE_URL=http://127.0.0.1:18098 MOCK_LLM_MODE=true " +
					"pytest tests/integration/ -v --tb=short --cov=src --cov-report=term-missing -o addopts='' && " +
					"python -m coverage report --precision=2 --show-missing",
			)

			// Run Python tests in container (same pattern as E2E)
			// DD-AUTH-014: Mount ServiceAccount token at standard Kubernetes path
			// Output redirected from outside container to workspace coverage file
			GinkgoWriter.Printf("   Token: %s → /var/run/secrets/kubernetes.io/serviceaccount/token\n", tokenFile)

			coverageFile := filepath.Join(workspaceRoot, "coverage_integration_holmesgpt-api_python.txt")
			runCmd := exec.Command("bash", "-c",
				fmt.Sprintf("podman run --rm --network=host "+
					"-v %s:/workspace:z "+
					"-v %s:/var/run/secrets/kubernetes.io/serviceaccount/token:ro "+
					"-e COVERAGE_FILE=/tmp/.coverage "+
					"-e PYTHONUNBUFFERED=1 "+
					"registry.access.redhat.com/ubi9/python-312:latest "+
					"sh -c %q 2>&1 | tee %s",
					workspaceRoot,
					tokenFile,
					pytestCmd,
					coverageFile),
			)
			runCmd.Stdout = GinkgoWriter
			runCmd.Stderr = GinkgoWriter

			startTime := time.Now()
			err = runCmd.Run()
			duration := time.Since(startTime)

			GinkgoWriter.Println("")
			GinkgoWriter.Printf("⏱️  Python tests completed in %v\n", duration.Round(time.Second))

			// Check test results
			if err != nil {
				Fail(fmt.Sprintf("Python integration tests failed: %v", err))
			}

			GinkgoWriter.Println("✅ All Python integration tests passed")

			// Create signal file for coordination (if Makefile needs it)
			_ = os.WriteFile(signalFile, []byte("complete"), 0644)
		})
	})
})
