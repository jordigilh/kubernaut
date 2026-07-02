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

package config

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadSecrets and its per-source helpers (ADR-030 Section 6: secrets loaded
// from mounted Kubernetes Secret files, never from the YAML ConfigMap).
// Split from config.go (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3, pure code
// motion, no behavior change); see config.go for the Config struct family
// and config_validate.go for Validate.

// LoadSecrets loads secrets from mounted Kubernetes Secret files (ADR-030 Section 6)
// It supports both YAML and JSON secret files.
// This function REQUIRES secretsFile to be configured for both database and redis.
//
// Decomposed into per-source loaders (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3)
// purely for readability/complexity — behavior, order, and error messages
// are unchanged from the pre-decomposition implementation.
func (c *Config) LoadSecrets() error {
	if err := c.loadDatabaseSecrets(); err != nil {
		return err
	}
	if err := c.loadRedisSecrets(); err != nil {
		return err
	}
	return c.loadAuditHashKeySecret()
}

// loadDatabaseSecrets loads the (required) database password, and optionally
// overrides the database username, from the configured database secrets file.
func (c *Config) loadDatabaseSecrets() error {
	if c.Database.SecretsFile == "" {
		return fmt.Errorf("database secretsFile required (ADR-030 Section 6)")
	}
	if c.Database.PasswordKey == "" {
		return fmt.Errorf("database passwordKey required (ADR-030 Section 6)")
	}

	dbSecrets, err := loadSecretFile(c.Database.SecretsFile)
	if err != nil {
		return fmt.Errorf("failed to load database secrets from %s: %w",
			c.Database.SecretsFile, err)
	}

	password, ok := dbSecrets[c.Database.PasswordKey]
	if !ok {
		return fmt.Errorf("password key '%s' not found in database secret file %s",
			c.Database.PasswordKey, c.Database.SecretsFile)
	}

	passwordStr, isString := password.(string)
	if !isString {
		return fmt.Errorf("database password key '%s' in secret file is not a string",
			c.Database.PasswordKey)
	}
	c.Database.Password = passwordStr

	if c.Database.UsernameKey != "" {
		username, ok := dbSecrets[c.Database.UsernameKey]
		if !ok {
			return nil
		}
		usernameStr, usernameIsString := username.(string)
		if !usernameIsString {
			return fmt.Errorf("database username key '%s' in secret file is not a string",
				c.Database.UsernameKey)
		}
		c.Database.User = usernameStr
	}

	return nil
}

// loadRedisSecrets loads the (required) Redis password from the configured
// Redis secrets file.
func (c *Config) loadRedisSecrets() error {
	if c.Redis.SecretsFile == "" {
		return fmt.Errorf("redis secretsFile required (ADR-030 Section 6)")
	}
	if c.Redis.PasswordKey == "" {
		return fmt.Errorf("redis passwordKey required (ADR-030 Section 6)")
	}

	redisSecrets, err := loadSecretFile(c.Redis.SecretsFile)
	if err != nil {
		return fmt.Errorf("failed to load redis secrets from %s: %w",
			c.Redis.SecretsFile, err)
	}

	redisPassword, ok := redisSecrets[c.Redis.PasswordKey]
	if !ok {
		return fmt.Errorf("password key '%s' not found in redis secret file %s",
			c.Redis.PasswordKey, c.Redis.SecretsFile)
	}

	redisPasswordStr, isString := redisPassword.(string)
	if !isString {
		return fmt.Errorf("redis password key '%s' in secret file is not a string",
			c.Redis.PasswordKey)
	}
	c.Redis.Password = redisPasswordStr

	return nil
}

// loadAuditHashKeySecret loads the optional HMAC key for the keyed audit hash
// chain (GAP-05, Issue #1505). Unlike database/redis secrets, this is
// OPTIONAL — omitting it preserves the legacy unkeyed SHA256 algorithm for
// backward compatibility.
func (c *Config) loadAuditHashKeySecret() error {
	if c.Audit.HashKeySecretsFile == "" {
		return nil
	}
	if c.Audit.HashKeyKey == "" {
		return fmt.Errorf("audit hashKeyKey required when hashKeySecretsFile is set (GAP-05)")
	}

	auditSecrets, err := loadSecretFile(c.Audit.HashKeySecretsFile)
	if err != nil {
		return fmt.Errorf("failed to load audit hash key secrets from %s: %w",
			c.Audit.HashKeySecretsFile, err)
	}

	hmacKeyRaw, ok := auditSecrets[c.Audit.HashKeyKey]
	if !ok {
		return fmt.Errorf("hash key '%s' not found in audit secret file %s",
			c.Audit.HashKeyKey, c.Audit.HashKeySecretsFile)
	}

	hmacKeyStr, isString := hmacKeyRaw.(string)
	if !isString {
		return fmt.Errorf("audit hash key '%s' in secret file is not a string",
			c.Audit.HashKeyKey)
	}
	if hmacKeyStr == "" {
		return fmt.Errorf("audit hash key '%s' in secret file %s is empty",
			c.Audit.HashKeyKey, c.Audit.HashKeySecretsFile)
	}

	c.Audit.HMACKey = []byte(hmacKeyStr)
	return nil
}

// loadSecretFile unmarshals a secret file (supports YAML and JSON)
// This is a helper function for LoadSecrets()
func loadSecretFile(secretFilePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(secretFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret file %s: %w", secretFilePath, err)
	}

	var secrets map[string]interface{}

	// Try YAML first
	if err := yaml.Unmarshal(data, &secrets); err == nil {
		return secrets, nil
	}

	// Fallback to JSON
	if err := json.Unmarshal(data, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secret file as YAML or JSON: %w", err)
	}

	return secrets, nil
}
