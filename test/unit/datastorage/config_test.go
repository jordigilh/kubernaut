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
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/config"
)

// func TestConfig(t *testing.T) {
// 	RegisterFailHandler(Fail)
// 	RunSpecs(t, "...")
// }

// ========================================
// CONFIG LOADING UNIT TESTS (ADR-030)
// 📋 Business Requirements:
//   - BR-STORAGE-010: Structured Configuration via YAML/ConfigMaps
//   - BR-STORAGE-028: Graceful Shutdown (requires valid config)
//
// 📋 Testing Principle: Behavior + Correctness
// ========================================
var _ = Describe("Config Loading (ADR-030)", func() {
	var tempDir string

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "config-test-*")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		_ = os.RemoveAll(tempDir)
	})

	Context("LoadFromFile", func() {
		It("should load valid minimal YAML config", func() {
			configPath := filepath.Join(tempDir, "config.yaml")
			validYAML := `
server:
  port: 8080
  host: "0.0.0.0"
  readTimeout: 30s
  writeTimeout: 30s
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  sslMode: disable
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: 5m
  connMaxIdleTime: 10m
redis:
  addr: localhost:6379
  db: 0
logging:
  level: info
  format: json
`
			err := os.WriteFile(configPath, []byte(validYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			// ACT: Load config from file
			cfg, err := config.LoadFromFile(configPath)

			// CORRECTNESS: Load succeeds
			Expect(err).ToNot(HaveOccurred(), "LoadFromFile should succeed")

			// CORRECTNESS: All config fields have exact expected values
			Expect(cfg.Server.Port).To(Equal(8080), "Server port should be 8080")
			Expect(cfg.Server.Host).To(Equal("0.0.0.0"), "Server host should be 0.0.0.0")
			Expect(cfg.Database.Host).To(Equal("localhost"), "Database host should be localhost")
			Expect(cfg.Database.Port).To(Equal(5432), "Database port should be 5432")
			Expect(cfg.Database.Name).To(Equal("testdb"), "Database name should be testdb")
			Expect(cfg.Database.User).To(Equal("testuser"), "Database user should be testuser")
			Expect(cfg.Redis.Addr).To(Equal("localhost:6379"), "Redis address should be localhost:6379")
			Expect(cfg.Logging.Level).To(Equal("info"), "Logging level should be info")
		})

		It("should fail on missing config file", func() {
			cfg, err := config.LoadFromFile("/nonexistent/config.yaml")
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to read config file"))
		})

		It("should fail on invalid YAML", func() {
			configPath := filepath.Join(tempDir, "bad.yaml")
			badYAML := `
server:
  port: [this is not valid
`
			err := os.WriteFile(configPath, []byte(badYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := config.LoadFromFile(configPath)
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to parse config"))
		})
	})

	Context("LoadSecrets (ADR-030 Section 6)", func() {
		It("should load database and redis secrets from YAML files", func() {
			configPath := filepath.Join(tempDir, "config.yaml")
			dbSecretsFile := filepath.Join(tempDir, "db-secrets.yaml")
			redisSecretsFile := filepath.Join(tempDir, "redis-secrets.yaml")

			configYAML := `
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  sslMode: disable
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: 5m
  connMaxIdleTime: 10m
  secretsFile: "` + dbSecretsFile + `"
  usernameKey: "username"
  passwordKey: "password"
redis:
  addr: localhost:6379
  db: 0
  secretsFile: "` + redisSecretsFile + `"
  passwordKey: "password"
logging:
  level: info
  format: json
`
			err := os.WriteFile(configPath, []byte(configYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			dbSecretYAML := `
username: secretdbuser
password: secretdbpass123
`
			err = os.WriteFile(dbSecretsFile, []byte(dbSecretYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			redisSecretYAML := `
password: secretredispass456
`
			err = os.WriteFile(redisSecretsFile, []byte(redisSecretYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Database.Password).To(BeEmpty())
			Expect(cfg.Redis.Password).To(BeEmpty())

			err = cfg.LoadSecrets()
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Database.Password).To(Equal("secretdbpass123"))
			Expect(cfg.Database.User).To(Equal("secretdbuser"))
			Expect(cfg.Redis.Password).To(Equal("secretredispass456"))
		})

		It("should load secrets from JSON files", func() {
			configPath := filepath.Join(tempDir, "config.yaml")
			dbSecretsFile := filepath.Join(tempDir, "db-secrets.json")
			redisSecretsFile := filepath.Join(tempDir, "redis-secrets.json")

			configYAML := `
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  sslMode: disable
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: 5m
  connMaxIdleTime: 10m
  secretsFile: "` + dbSecretsFile + `"
  passwordKey: "db_password"
redis:
  addr: localhost:6379
  db: 0
  secretsFile: "` + redisSecretsFile + `"
  passwordKey: "redis_password"
logging:
  level: info
  format: json
`
			err := os.WriteFile(configPath, []byte(configYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			dbSecretJSON := `{"db_password": "json-db-secret"}`
			err = os.WriteFile(dbSecretsFile, []byte(dbSecretJSON), 0644)
			Expect(err).ToNot(HaveOccurred())

			redisSecretJSON := `{"redis_password": "json-redis-secret"}`
			err = os.WriteFile(redisSecretsFile, []byte(redisSecretJSON), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred())

			err = cfg.LoadSecrets()
			Expect(err).ToNot(HaveOccurred())
			Expect(cfg.Database.Password).To(Equal("json-db-secret"))
			Expect(cfg.Redis.Password).To(Equal("json-redis-secret"))
		})

		It("should fail if database secretsFile is not configured", func() {
			configPath := filepath.Join(tempDir, "config.yaml")
			redisSecretsFile := filepath.Join(tempDir, "redis-secrets.yaml")

			configYAML := `
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  sslMode: disable
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: 5m
  connMaxIdleTime: 10m
redis:
  addr: localhost:6379
  db: 0
  secretsFile: "` + redisSecretsFile + `"
  passwordKey: "password"
logging:
  level: info
  format: json
`
			err := os.WriteFile(configPath, []byte(configYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			redisSecretYAML := `password: redispass`
			err = os.WriteFile(redisSecretsFile, []byte(redisSecretYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred())

			err = cfg.LoadSecrets()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database secretsFile required"))
		})

		It("should fail if redis secretsFile is not configured", func() {
			configPath := filepath.Join(tempDir, "config.yaml")
			dbSecretsFile := filepath.Join(tempDir, "db-secrets.yaml")

			configYAML := `
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  sslMode: disable
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: 5m
  connMaxIdleTime: 10m
  secretsFile: "` + dbSecretsFile + `"
  passwordKey: "password"
redis:
  addr: localhost:6379
  db: 0
logging:
  level: info
  format: json
`
			err := os.WriteFile(configPath, []byte(configYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			dbSecretYAML := `password: dbpass`
			err = os.WriteFile(dbSecretsFile, []byte(dbSecretYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred())

			err = cfg.LoadSecrets()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("redis secretsFile required"))
		})
	})

	Context("Validation", func() {
		It("should pass validation for valid config", func() {
			configPath := filepath.Join(tempDir, "config.yaml")
			validYAML := `
server:
  port: 8080
  host: "0.0.0.0"
  readTimeout: 30s
  writeTimeout: 30s
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  sslMode: disable
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: 5m
  connMaxIdleTime: 10m
redis:
  addr: localhost:6379
  db: 0
logging:
  level: info
  format: json
`
			err := os.WriteFile(configPath, []byte(validYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred())

			err = cfg.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail if database host is empty", func() {
			configPath := filepath.Join(tempDir, "config.yaml")
			invalidYAML := `
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s
database:
  host: ""
  port: 5432
  name: testdb
  user: testuser
  sslMode: disable
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: 5m
  connMaxIdleTime: 10m
redis:
  addr: localhost:6379
  db: 0
logging:
  level: info
  format: json
`
			err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database host required"))
		})

		It("should fail if server port is invalid", func() {
			configPath := filepath.Join(tempDir, "config.yaml")
			invalidYAML := `
server:
  port: 100
  readTimeout: 30s
  writeTimeout: 30s
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  sslMode: disable
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: 5m
  connMaxIdleTime: 10m
redis:
  addr: localhost:6379
  db: 0
logging:
  level: info
  format: json
`
			err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("server port must be between 1024 and 65535"))
		})

		It("should fail if Redis address is empty", func() {
			configPath := filepath.Join(tempDir, "config.yaml")
			invalidYAML := `
server:
  port: 8080
  readTimeout: 30s
  writeTimeout: 30s
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  sslMode: disable
  maxOpenConns: 25
  maxIdleConns: 5
  connMaxLifetime: 5m
  connMaxIdleTime: 10m
redis:
  addr: ""
  db: 0
logging:
  level: info
  format: json
`
			err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
			Expect(err).ToNot(HaveOccurred())

			cfg, err := config.LoadFromFile(configPath)
			Expect(err).ToNot(HaveOccurred())

			err = cfg.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("redis address required"))
		})
	})
})
