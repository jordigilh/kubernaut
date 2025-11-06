package datastorage

import (
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
		os.RemoveAll(tempDir)
	})

	Context("Main Application Config Loading", func() {
		It("should load config and secrets from CONFIG_PATH and start successfully", func() {
			validYAML := `
server:
  port: 18080
  host: "127.0.0.1"
  read_timeout: 30s
  write_timeout: 30s
database:
  host: localhost
  port: 5432
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
  addr: localhost:6379
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

			// Give it a moment to start (or fail)
			time.Sleep(2 * time.Second)

			// Check if process is still running (config loaded successfully)
			err = cmd.Process.Signal(syscall.Signal(0))
			Expect(err).ToNot(HaveOccurred(), "Service should be running after loading config and secrets")

			// Cleanup: kill the process
			cmd.Process.Kill()
			cmd.Wait()
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
			validYAML := `
server:
  port: 18080
  host: "127.0.0.1"
  read_timeout: 30s
  write_timeout: 30s
database:
  host: localhost
  port: 5432
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
  addr: localhost:6379
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
  port: 5432
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
