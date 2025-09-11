package testutil

import (
	"context"

	"github.com/sirupsen/logrus"
)

// StorageTestSuiteComponents contains common test setup components for storage tests
type StorageTestSuiteComponents struct {
	Context context.Context
	Logger  *logrus.Logger
}

// StorageTestSuite creates a standardized test suite setup for storage tests
func StorageTestSuite(testName string) *StorageTestSuiteComponents {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests

	return &StorageTestSuiteComponents{
		Context: context.Background(),
		Logger:  logger,
	}
}

// VectorTestSuite creates a standardized test suite setup specifically for vector storage tests
func VectorTestSuite(testName string) *StorageTestSuiteComponents {
	return StorageTestSuite(testName)
}
