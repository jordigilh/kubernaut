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
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Python Test Coordination", func() {
	Context("Infrastructure Lifecycle Management", func() {
		It("should keep infrastructure alive while Python tests run", func() {
			// Pattern: Go starts infrastructure, blocks until Python tests complete
			// Signal file: /tmp/hapi-integration-tests-complete
			// Created by: Makefile after pytest completes
			// Purpose: Prevents Ginkgo from tearing down infrastructure prematurely

			signalFile := "/tmp/hapi-integration-tests-complete"

			// Clean up any stale signal file from previous run
			_ = os.Remove(signalFile)

			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("🐍 Waiting for Python integration tests to complete...")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			GinkgoWriter.Println("")
			GinkgoWriter.Println("Infrastructure Status:")
			GinkgoWriter.Println("  ✅ PostgreSQL (port 15439)")
			GinkgoWriter.Println("  ✅ Redis (port 16387)")
			GinkgoWriter.Println("  ✅ Data Storage API (port 18098)")
			GinkgoWriter.Println("")
			GinkgoWriter.Println("Waiting for signal file:")
			GinkgoWriter.Printf("  📄 %s\n", signalFile)
			GinkgoWriter.Println("")
			GinkgoWriter.Println("This test will:")
			GinkgoWriter.Println("  1. Keep infrastructure alive")
			GinkgoWriter.Println("  2. Wait for Python tests to complete")
			GinkgoWriter.Println("  3. Allow AfterSuite to tear down cleanly")
			GinkgoWriter.Println("")
			GinkgoWriter.Println("Python tests will:")
			GinkgoWriter.Println("  • Use TestClient (in-process HAPI)")
			GinkgoWriter.Println("  • Connect to Data Storage at localhost:18098")
			GinkgoWriter.Println("  • Run with pytest -n 4 (4 parallel workers)")
			GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			startTime := time.Now()
			timeout := 20 * time.Minute // Match Ginkgo --timeout=20m

			Eventually(func() bool {
				elapsed := time.Since(startTime)
				if elapsed > timeout {
					Fail(fmt.Sprintf("Timeout: Python tests did not complete within %v", timeout))
				}

				// Check if signal file exists
				absPath, err := filepath.Abs(signalFile)
				if err != nil {
					GinkgoWriter.Printf("⚠️  Error resolving signal file path: %v\n", err)
					return false
				}

				_, err = os.Stat(absPath)
				if err == nil {
					// Signal file exists - Python tests complete
					GinkgoWriter.Println("")
					GinkgoWriter.Println("✅ Python integration tests completed successfully!")
					GinkgoWriter.Printf("   Duration: %v\n", elapsed.Round(time.Second))
					GinkgoWriter.Println("   Infrastructure will now be torn down by AfterSuite")
					return true
				}

				// Every 30 seconds, print a status update
				if int(elapsed.Seconds())%30 == 0 && int(elapsed.Seconds()) > 0 {
					GinkgoWriter.Printf("⏳ Still waiting for Python tests... (%v elapsed)\n", elapsed.Round(time.Second))
				}

				return false
			}, timeout, 1*time.Second).Should(BeTrue(),
				"Python integration tests should complete and create signal file")

			// Clean up signal file
			_ = os.Remove(signalFile)
		})
	})
})
