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

package datastorage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config Integration Tests (ADR-030)", func() {
	var (
		tempDir          string
		configPath       string
		dbSecretsPath    string
		redisSecretsPath string
		binaryPath       string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "config-integration-*")
		Expect(err).ToNot(HaveOccurred())

		configPath = filepath.Join(tempDir, "config.yaml")
		dbSecretsPath = filepath.Join(tempDir, "db-secrets.yaml")
		redisSecretsPath = filepath.Join(tempDir, "redis-secrets.yaml")

		// Build the data storage binary
		binaryPath = filepath.Join(tempDir, "datastorage")
		buildCmd := exec.Command("go", "build", "-o", binaryPath, "../../../cmd/datastorage")
		output, err := buildCmd.CombinedOutput()
		if err != nil {
			Fail("Failed to build datastorage binary: " + string(output))
		}
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Context("Main Application Config Loading", func() {
		It("should load config and secrets from CONFIG_PATH and start successfully", func() {
			// Use unique port per parallel process to avoid conflicts
			serverPort := 18090 + GinkgoParallelProcess()
			validYAML := fmt.Sprintf(`
server:
  port: %d # DD-TEST-001 + process offset
  host: "127.0.0.1"
  read_timeout: 30s
  write_timeout: 30s`, serverPort) + `
database:
  host: localhost
  port: 15433 # DD-TEST-001
  name: testdb
  user: testuser
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 10m
  secretsFile: "` + dbSecretsPath + `"
  passwordKey: "password"
redis:
  addr: localhost:16379 # DD-TEST-001
  db: 0
  secretsFile: "` + redisSecretsPath + `"
  passwordKey: "password"
logging:
  level: info
  format: json
`
			err := os.WriteFile(configPath, []byte(validYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Create secret files
			dbSecretYAML := `password: test-db-password`
			err = os.WriteFile(dbSecretsPath, []byte(dbSecretYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			redisSecretYAML := `password: test-redis-password`
			err = os.WriteFile(redisSecretsPath, []byte(redisSecretYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			cmd := exec.Command(binaryPath)
			cmd.Env = append(os.Environ(), "CONFIG_PATH="+configPath)

			// Start the service
			err = cmd.Start()
			Expect(err).ToNot(HaveOccurred())

			// Per TESTING_GUIDELINES.md: Use Eventually() to verify process started
			Eventually(func() bool {
				// Check if process is still running (config loaded successfully)
				err := cmd.Process.Signal(syscall.Signal(0))
				return err == nil // nil means process is running
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(), "Process should start successfully")

			// Final check
			err = cmd.Process.Signal(syscall.Signal(0))
			Expect(err).ToNot(HaveOccurred(), "Service should be running after loading config and secrets")

			// Cleanup: kill the process
			Expect(cmd.Process.Kill()).ToNot(HaveOccurred())
			_ = cmd.Wait()
		})

		It("should fail to start if CONFIG_PATH is not set", func() {
			cmd := exec.Command(binaryPath)
			// Explicitly don't set CONFIG_PATH

			output, err := cmd.CombinedOutput()
			Expect(err).To(HaveOccurred(), "Service should fail without CONFIG_PATH")
			Expect(string(output)).To(ContainSubstring("CONFIG_PATH environment variable required"))
		})

		It("should fail to start if config file doesn't exist", func() {
			cmd := exec.Command(binaryPath)
			cmd.Env = append(os.Environ(), "CONFIG_PATH=/nonexistent/config.yaml")

			output, err := cmd.CombinedOutput()
			Expect(err).To(HaveOccurred(), "Service should fail with missing config file")
			Expect(string(output)).To(ContainSubstring("Failed to load configuration file"))
		})

		It("should fail to start if database secret file is missing", func() {
			// Use unique port per parallel process to avoid conflicts
			serverPort := 18090 + GinkgoParallelProcess()
			validYAML := fmt.Sprintf(`
server:
  port: %d # DD-TEST-001 + process offset
  host: "127.0.0.1"
  read_timeout: 30s
  write_timeout: 30s`, serverPort) + `
database:
  host: localhost
  port: 15433 # DD-TEST-001
  name: testdb
  user: testuser
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 10m
  secretsFile: "/nonexistent/db-secrets.yaml"
  passwordKey: "password"
redis:
  addr: localhost:16379 # DD-TEST-001
  db: 0
  secretsFile: "` + redisSecretsPath + `"
  passwordKey: "password"
logging:
  level: info
  format: json
`
			err := os.WriteFile(configPath, []byte(validYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Create only Redis secret file
			redisSecretYAML := `password: test-redis-password`
			err = os.WriteFile(redisSecretsPath, []byte(redisSecretYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			cmd := exec.Command(binaryPath)
			cmd.Env = append(os.Environ(), "CONFIG_PATH="+configPath)

			output, err := cmd.CombinedOutput()
			Expect(err).To(HaveOccurred(), "Service should fail with missing database secret file")
			Expect(string(output)).To(ContainSubstring("Failed to load secrets"))
		})

		It("should fail to start if config is invalid", func() {
			invalidYAML := `
server:
  port: 100
database:
  host: ""
  port: 15433 # DD-TEST-001
  name: testdb
  user: testuser
  ssl_mode: disable
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m
  conn_max_idle_time: 10m
  secretsFile: "` + dbSecretsPath + `"
  passwordKey: "password"
redis:
  addr: ""
  db: 0
  secretsFile: "` + redisSecretsPath + `"
  passwordKey: "password"
logging:
  level: info
  format: json
`
			err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			// Create secret files so LoadSecrets() succeeds and Validate() runs
			dbSecretYAML := `password: test-password`
			err = os.WriteFile(dbSecretsPath, []byte(dbSecretYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			redisSecretYAML := `password: test-redis-password`
			err = os.WriteFile(redisSecretsPath, []byte(redisSecretYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			cmd := exec.Command(binaryPath)
			cmd.Env = append(os.Environ(), "CONFIG_PATH="+configPath)

			output, err := cmd.CombinedOutput()
			Expect(err).To(HaveOccurred(), "Service should fail with invalid config")
			Expect(string(output)).To(ContainSubstring("Invalid configuration"))
		})
	})
})
