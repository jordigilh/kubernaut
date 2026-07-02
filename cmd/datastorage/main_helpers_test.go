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

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
)

// TestLoadDataStorageConfig_MissingFile is a characterization test for
// loadDataStorageConfig, extracted from main() in
// GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0a. Pins the guard-clause contract:
// a nonexistent config path is rejected with a wrapped error, not a panic.
// cmd/datastorage had zero test coverage before this extraction.
func TestLoadDataStorageConfig_MissingFile(t *testing.T) {
	_, err := loadDataStorageConfig("/nonexistent/config.yaml", logr.Discard())
	if err == nil {
		t.Fatal("expected an error for a nonexistent config file, got nil")
	}
}

// TestLoadDataStorageConfig_ValidConfigWithSecrets pins the full happy path:
// a valid YAML config plus on-disk database/redis secret files loads,
// resolves secrets, and validates successfully.
func TestLoadDataStorageConfig_ValidConfigWithSecrets(t *testing.T) {
	dir := t.TempDir()

	dbSecretPath := filepath.Join(dir, "db-secret.yaml")
	if err := os.WriteFile(dbSecretPath, []byte("password: db-pass\n"), 0o600); err != nil {
		t.Fatalf("failed to write db secret file: %v", err)
	}
	redisSecretPath := filepath.Join(dir, "redis-secret.yaml")
	if err := os.WriteFile(redisSecretPath, []byte("password: redis-pass\n"), 0o600); err != nil {
		t.Fatalf("failed to write redis secret file: %v", err)
	}

	configYAML := `
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
  secretsFile: ` + dbSecretPath + `
  passwordKey: password
redis:
  addr: localhost:6379
  db: 0
  secretsFile: ` + redisSecretPath + `
  passwordKey: password
logging:
  level: info
  format: json
`
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("PORT", "9999")
	t.Setenv("HEALTH_PORT", "9998")

	cfg, err := loadDataStorageConfig(configPath, logr.Discard())
	if err != nil {
		t.Fatalf("loadDataStorageConfig returned unexpected error: %v", err)
	}
	if cfg.Database.Password != "db-pass" {
		t.Errorf("expected database password to be resolved from secrets file, got %q", cfg.Database.Password)
	}
	if cfg.Redis.Password != "redis-pass" {
		t.Errorf("expected redis password to be resolved from secrets file, got %q", cfg.Redis.Password)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("expected PORT env var to override config port to 9999, got %d", cfg.Server.Port)
	}
	if cfg.Server.HealthPort != 9998 {
		t.Errorf("expected HEALTH_PORT env var to override config health port to 9998, got %d", cfg.Server.HealthPort)
	}
}

// TestLoadDataStorageConfig_MissingSecretsFile pins the fail-fast contract:
// a syntactically valid config missing the required secretsFile/passwordKey
// fields surfaces as an error from LoadSecrets (ADR-030 Section 6), not a
// zero-value password silently proceeding to startup.
func TestLoadDataStorageConfig_MissingSecretsFile(t *testing.T) {
	dir := t.TempDir()
	configYAML := `
server:
  port: 8080
  host: "0.0.0.0"
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  sslMode: disable
redis:
  addr: localhost:6379
  db: 0
logging:
  level: info
  format: json
`
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := loadDataStorageConfig(configPath, logr.Discard())
	if err == nil {
		t.Fatal("expected an error when database.secretsFile is not set, got nil")
	}
}
