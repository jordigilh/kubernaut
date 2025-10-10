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

package testutil

import (
	"context"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// InternalTestSuiteComponents contains common test setup components for internal tests
type InternalTestSuiteComponents struct {
	Context     context.Context
	Logger      *logrus.Logger
	TempDir     string
	ConfigFile  string
	OriginalEnv map[string]string
}

// InternalTestSuite creates a standardized test suite setup for internal tests
func InternalTestSuite(testName string) *InternalTestSuiteComponents {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests

	return &InternalTestSuiteComponents{
		Context:     context.Background(),
		Logger:      logger,
		OriginalEnv: make(map[string]string),
	}
}

// ConfigTestSuite creates a standardized test suite setup for configuration tests
func ConfigTestSuite(testName string) *InternalTestSuiteComponents {
	return InternalTestSuite(testName)
}

// DatabaseTestSuite creates a standardized test suite setup for database tests
func DatabaseTestSuite(testName string) *InternalTestSuiteComponents {
	return InternalTestSuite(testName)
}

// ValidationTestSuite creates a standardized test suite setup for validation tests
func ValidationTestSuite(testName string) *InternalTestSuiteComponents {
	return InternalTestSuite(testName)
}

// ErrorTestSuite creates a standardized test suite setup for error tests
func ErrorTestSuite(testName string) *InternalTestSuiteComponents {
	return InternalTestSuite(testName)
}

// CreateTempDir creates a temporary directory for testing
func (c *InternalTestSuiteComponents) CreateTempDir() error {
	var err error
	c.TempDir, err = os.MkdirTemp("", "internal-test")
	if err != nil {
		return err
	}
	c.ConfigFile = filepath.Join(c.TempDir, "config.yaml")
	return nil
}

// CleanupTempDir removes the temporary directory
func (c *InternalTestSuiteComponents) CleanupTempDir() {
	if c.TempDir != "" {
		if err := os.RemoveAll(c.TempDir); err != nil {
			c.Logger.WithError(err).Error("Failed to remove temporary directory")
		}
		c.TempDir = ""
		c.ConfigFile = ""
	}
}

// SaveEnvVar saves an environment variable for later restoration
func (c *InternalTestSuiteComponents) SaveEnvVar(key string) {
	c.OriginalEnv[key] = os.Getenv(key)
}

// SaveEnvVars saves multiple environment variables for later restoration
func (c *InternalTestSuiteComponents) SaveEnvVars(keys []string) {
	for _, key := range keys {
		c.SaveEnvVar(key)
	}
}

// RestoreEnvVars restores all saved environment variables
func (c *InternalTestSuiteComponents) RestoreEnvVars() {
	for key, value := range c.OriginalEnv {
		if value == "" {
			if err := os.Unsetenv(key); err != nil {
				c.Logger.WithError(err).WithField("key", key).Error("Failed to unset environment variable")
			}
		} else {
			if err := os.Setenv(key, value); err != nil {
				c.Logger.WithError(err).WithField("key", key).Error("Failed to set environment variable")
			}
		}
	}
	c.OriginalEnv = make(map[string]string)
}

// SetEnvVar sets an environment variable
func (c *InternalTestSuiteComponents) SetEnvVar(key, value string) {
	if err := os.Setenv(key, value); err != nil {
		c.Logger.WithError(err).WithField("key", key).Error("Failed to set environment variable")
	}
}

// SetEnvVars sets multiple environment variables
func (c *InternalTestSuiteComponents) SetEnvVars(vars map[string]string) {
	for key, value := range vars {
		if err := os.Setenv(key, value); err != nil {
			c.Logger.WithError(err).WithField("key", key).Error("Failed to set environment variable")
		}
	}
}

// WriteConfigFile writes content to the config file
func (c *InternalTestSuiteComponents) WriteConfigFile(content string) error {
	return os.WriteFile(c.ConfigFile, []byte(content), 0644)
}
