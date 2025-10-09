//go:build integration
// +build integration

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

package shared

import (
	"os"
	"sync"

	"github.com/sirupsen/logrus"
)

// EnvironmentIsolationHelper manages environment variable isolation for tests
type EnvironmentIsolationHelper struct {
	logger         *logrus.Logger
	originalValues map[string]string
	originalExists map[string]bool
	mutex          sync.Mutex
}

// NewEnvironmentIsolationHelper creates a new environment isolation helper
func NewEnvironmentIsolationHelper(logger *logrus.Logger) *EnvironmentIsolationHelper {
	return &EnvironmentIsolationHelper{
		logger:         logger,
		originalValues: make(map[string]string),
		originalExists: make(map[string]bool),
	}
}

// CaptureEnvironment captures the current state of specified environment variables
func (e *EnvironmentIsolationHelper) CaptureEnvironment(envVars ...string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for _, envVar := range envVars {
		value, exists := os.LookupEnv(envVar)
		e.originalValues[envVar] = value
		e.originalExists[envVar] = exists

		e.logger.Debugf("Captured env var %s: exists=%v, value=%s", envVar, exists, value)
	}
}

// SetEnvironment sets environment variables for test isolation
func (e *EnvironmentIsolationHelper) SetEnvironment(envVars map[string]string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for key, value := range envVars {
		// Capture original state if not already captured
		if _, captured := e.originalExists[key]; !captured {
			originalValue, exists := os.LookupEnv(key)
			e.originalValues[key] = originalValue
			e.originalExists[key] = exists
		}

		os.Setenv(key, value)
		e.logger.Debugf("Set env var %s = %s", key, value)
	}
}

// UnsetEnvironment removes environment variables for test isolation
func (e *EnvironmentIsolationHelper) UnsetEnvironment(envVars ...string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for _, envVar := range envVars {
		// Capture original state if not already captured
		if _, captured := e.originalExists[envVar]; !captured {
			originalValue, exists := os.LookupEnv(envVar)
			e.originalValues[envVar] = originalValue
			e.originalExists[envVar] = exists
		}

		os.Unsetenv(envVar)
		e.logger.Debugf("Unset env var %s", envVar)
	}
}

// RestoreEnvironment restores all captured environment variables to their original state
func (e *EnvironmentIsolationHelper) RestoreEnvironment() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for envVar, originalValue := range e.originalValues {
		existed := e.originalExists[envVar]

		if existed {
			os.Setenv(envVar, originalValue)
			e.logger.Debugf("Restored env var %s = %s", envVar, originalValue)
		} else {
			os.Unsetenv(envVar)
			e.logger.Debugf("Restored env var %s (unset)", envVar)
		}
	}
}

// GetEnvironmentSnapshot returns a snapshot of current environment variables
func (e *EnvironmentIsolationHelper) GetEnvironmentSnapshot(envVars ...string) map[string]string {
	snapshot := make(map[string]string)

	for _, envVar := range envVars {
		if value, exists := os.LookupEnv(envVar); exists {
			snapshot[envVar] = value
		}
	}

	return snapshot
}

// StandardLLMEnvironmentVariables returns the list of common LLM environment variables
func StandardLLMEnvironmentVariables() []string {
	return []string{
		"LLM_ENDPOINT",
		"LLM_MODEL",
		"LLM_PROVIDER",
		"SKIP_INTEGRATION",
		"SKIP_K8S_INTEGRATION",
		"SKIP_E2E",
		"SKIP_MODEL_COMPARISON",
		"KUBEBUILDER_ASSETS",
		"LOG_LEVEL",
	}
}

// StandardIntegrationTestEnvironmentVariables returns comprehensive list for integration tests
func StandardIntegrationTestEnvironmentVariables() []string {
	base := StandardLLMEnvironmentVariables()
	additional := []string{
		"DATABASE_URL",
		"POSTGRES_HOST",
		"POSTGRES_PORT",
		"POSTGRES_USER",
		"POSTGRES_PASSWORD",
		"POSTGRES_DB",
		"TEST_TIMEOUT",
		"MAX_RETRIES",
	}

	return append(base, additional...)
}
