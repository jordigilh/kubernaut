//go:build integration
// +build integration

<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package shared

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// TestIsolationManager provides comprehensive test isolation capabilities
type TestIsolationManager struct {
	logger           *logrus.Logger
	envBackups       map[string]string
	originalEnvKeys  []string
	tempDirs         []string
	cleanupFunctions []func() error
	isolationLevel   IsolationLevel
	isolationID      string
	started          bool
	mutex            sync.RWMutex
}

// IsolationLevel defines the scope of test isolation
type IsolationLevel int

const (
	// IsolationMinimal provides basic cleanup
	IsolationMinimal IsolationLevel = iota
	// IsolationStandard provides environment and temp directory isolation
	IsolationStandard
	// IsolationComprehensive provides full isolation including database transactions
	IsolationComprehensive
)

// CleanupFunc represents a cleanup function that can return an error
type CleanupFunc func() error

// NewTestIsolationManager creates a new test isolation manager
func NewTestIsolationManager(logger *logrus.Logger, level IsolationLevel) *TestIsolationManager {
	isolationID := fmt.Sprintf("test_%d_%d", time.Now().UnixNano(), os.Getpid())

	return &TestIsolationManager{
		logger:           logger,
		envBackups:       make(map[string]string),
		originalEnvKeys:  make([]string, 0),
		tempDirs:         make([]string, 0),
		cleanupFunctions: make([]func() error, 0),
		isolationLevel:   level,
		isolationID:      isolationID,
		started:          false,
	}
}

// StartIsolation begins the isolation process
func (tim *TestIsolationManager) StartIsolation() error {
	tim.mutex.Lock()
	defer tim.mutex.Unlock()

	if tim.started {
		return fmt.Errorf("isolation already started")
	}

	tim.logger.WithFields(logrus.Fields{
		"isolation_id":    tim.isolationID,
		"isolation_level": tim.isolationLevel,
	}).Debug("Starting test isolation")

	// Backup current environment state
	if err := tim.backupEnvironment(); err != nil {
		return fmt.Errorf("failed to backup environment: %w", err)
	}

	tim.started = true
	return nil
}

// EndIsolation performs comprehensive cleanup
func (tim *TestIsolationManager) EndIsolation() error {
	tim.mutex.Lock()
	defer tim.mutex.Unlock()

	if !tim.started {
		return nil // Nothing to clean up
	}

	var errors []string

	tim.logger.WithField("isolation_id", tim.isolationID).Debug("Ending test isolation")

	// Execute cleanup functions in reverse order
	for i := len(tim.cleanupFunctions) - 1; i >= 0; i-- {
		if err := tim.cleanupFunctions[i](); err != nil {
			errors = append(errors, fmt.Sprintf("cleanup function %d failed: %v", i, err))
		}
	}

	// Restore environment variables
	if err := tim.restoreEnvironment(); err != nil {
		errors = append(errors, fmt.Sprintf("environment restoration failed: %v", err))
	}

	// Clean up temporary directories
	if err := tim.cleanupTempDirs(); err != nil {
		errors = append(errors, fmt.Sprintf("temp directory cleanup failed: %v", err))
	}

	tim.started = false

	if len(errors) > 0 {
		return fmt.Errorf("isolation cleanup encountered errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// SetEnvironmentVariable sets an environment variable with automatic cleanup
func (tim *TestIsolationManager) SetEnvironmentVariable(key, value string) error {
	tim.mutex.Lock()
	defer tim.mutex.Unlock()

	// Backup original value if not already backed up
	if _, exists := tim.envBackups[key]; !exists {
		if originalValue, wasSet := os.LookupEnv(key); wasSet {
			tim.envBackups[key] = originalValue
		} else {
			tim.envBackups[key] = "__UNSET__" // Special marker for unset variables
		}
		tim.originalEnvKeys = append(tim.originalEnvKeys, key)
	}

	if err := os.Setenv(key, value); err != nil {
		return fmt.Errorf("failed to set environment variable %s: %w", key, err)
	}

	tim.logger.WithFields(logrus.Fields{
		"key":          key,
		"value":        value,
		"isolation_id": tim.isolationID,
	}).Debug("Set isolated environment variable")

	return nil
}

// UnsetEnvironmentVariable removes an environment variable with automatic cleanup
func (tim *TestIsolationManager) UnsetEnvironmentVariable(key string) error {
	tim.mutex.Lock()
	defer tim.mutex.Unlock()

	// Backup original value if not already backed up
	if _, exists := tim.envBackups[key]; !exists {
		if originalValue, wasSet := os.LookupEnv(key); wasSet {
			tim.envBackups[key] = originalValue
		} else {
			tim.envBackups[key] = "__UNSET__"
		}
		tim.originalEnvKeys = append(tim.originalEnvKeys, key)
	}

	if err := os.Unsetenv(key); err != nil {
		return fmt.Errorf("failed to unset environment variable %s: %w", key, err)
	}

	tim.logger.WithFields(logrus.Fields{
		"key":          key,
		"isolation_id": tim.isolationID,
	}).Debug("Unset isolated environment variable")

	return nil
}

// AddCleanupFunction adds a cleanup function to be executed during isolation end
func (tim *TestIsolationManager) AddCleanupFunction(cleanupFn CleanupFunc) {
	tim.mutex.Lock()
	defer tim.mutex.Unlock()

	tim.cleanupFunctions = append(tim.cleanupFunctions, cleanupFn)

	tim.logger.WithFields(logrus.Fields{
		"cleanup_count": len(tim.cleanupFunctions),
		"isolation_id":  tim.isolationID,
	}).Debug("Added cleanup function")
}

// CreateTempDirectory creates a temporary directory that will be cleaned up
func (tim *TestIsolationManager) CreateTempDirectory(prefix string) (string, error) {
	tim.mutex.Lock()
	defer tim.mutex.Unlock()

	tempDir, err := os.MkdirTemp("", fmt.Sprintf("%s_%s_", prefix, tim.isolationID))
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	tim.tempDirs = append(tim.tempDirs, tempDir)

	tim.logger.WithFields(logrus.Fields{
		"temp_dir":     tempDir,
		"isolation_id": tim.isolationID,
	}).Debug("Created isolated temp directory")

	return tempDir, nil
}

// GetIsolationID returns the unique isolation identifier
func (tim *TestIsolationManager) GetIsolationID() string {
	tim.mutex.RLock()
	defer tim.mutex.RUnlock()
	return tim.isolationID
}

// IsStarted returns whether isolation has been started
func (tim *TestIsolationManager) IsStarted() bool {
	tim.mutex.RLock()
	defer tim.mutex.RUnlock()
	return tim.started
}

// backupEnvironment creates a backup of current environment state
func (tim *TestIsolationManager) backupEnvironment() error {
	// Environment backup is handled per-variable in SetEnvironmentVariable
	// This method is reserved for future comprehensive environment backups
	return nil
}

// restoreEnvironment restores the original environment state
func (tim *TestIsolationManager) restoreEnvironment() error {
	var errors []string

	for _, key := range tim.originalEnvKeys {
		originalValue, exists := tim.envBackups[key]
		if !exists {
			continue
		}

		if originalValue == "__UNSET__" {
			// Variable was originally unset
			if err := os.Unsetenv(key); err != nil {
				errors = append(errors, fmt.Sprintf("failed to unset %s: %v", key, err))
			}
		} else {
			// Restore original value
			if err := os.Setenv(key, originalValue); err != nil {
				errors = append(errors, fmt.Sprintf("failed to restore %s: %v", key, err))
			}
		}

		tim.logger.WithFields(logrus.Fields{
			"key":          key,
			"restored":     originalValue != "__UNSET__",
			"isolation_id": tim.isolationID,
		}).Debug("Restored environment variable")
	}

	// Clear backups
	tim.envBackups = make(map[string]string)
	tim.originalEnvKeys = make([]string, 0)

	if len(errors) > 0 {
		return fmt.Errorf("environment restoration errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// cleanupTempDirs removes all created temporary directories
func (tim *TestIsolationManager) cleanupTempDirs() error {
	var errors []string

	for _, dir := range tim.tempDirs {
		if err := os.RemoveAll(dir); err != nil {
			errors = append(errors, fmt.Sprintf("failed to remove %s: %v", dir, err))
		} else {
			tim.logger.WithFields(logrus.Fields{
				"temp_dir":     dir,
				"isolation_id": tim.isolationID,
			}).Debug("Cleaned up temp directory")
		}
	}

	// Clear temp dirs list
	tim.tempDirs = make([]string, 0)

	if len(errors) > 0 {
		return fmt.Errorf("temp directory cleanup errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// IsolatedTestSuite provides a convenient wrapper for test isolation
type IsolatedTestSuite struct {
	isolationManager *TestIsolationManager
	logger           *logrus.Logger
	suiteName        string
}

// NewEnhancedIsolatedTestSuite creates a new isolated test suite with enhanced capabilities
func NewEnhancedIsolatedTestSuite(suiteName string, logger *logrus.Logger, level IsolationLevel) *IsolatedTestSuite {
	return &IsolatedTestSuite{
		isolationManager: NewTestIsolationManager(logger, level),
		logger:           logger,
		suiteName:        suiteName,
	}
}

// BeforeEach should be called at the start of each test
func (its *IsolatedTestSuite) BeforeEach() error {
	its.logger.WithField("suite", its.suiteName).Debug("Starting test isolation")
	return its.isolationManager.StartIsolation()
}

// AfterEach should be called at the end of each test
func (its *IsolatedTestSuite) AfterEach() error {
	its.logger.WithField("suite", its.suiteName).Debug("Ending test isolation")
	return its.isolationManager.EndIsolation()
}

// GetIsolationManager returns the underlying isolation manager
func (its *IsolatedTestSuite) GetIsolationManager() *TestIsolationManager {
	return its.isolationManager
}

// Convenience methods
func (its *IsolatedTestSuite) SetEnv(key, value string) error {
	return its.isolationManager.SetEnvironmentVariable(key, value)
}

func (its *IsolatedTestSuite) UnsetEnv(key string) error {
	return its.isolationManager.UnsetEnvironmentVariable(key)
}

func (its *IsolatedTestSuite) AddCleanup(cleanupFn CleanupFunc) {
	its.isolationManager.AddCleanupFunction(cleanupFn)
}

func (its *IsolatedTestSuite) CreateTempDir(prefix string) (string, error) {
	return its.isolationManager.CreateTempDirectory(prefix)
}
