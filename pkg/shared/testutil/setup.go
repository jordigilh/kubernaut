package testutil

import (
	"context"

	"github.com/sirupsen/logrus"
)

// SharedTestSuiteComponents contains common test setup components for shared package tests
type SharedTestSuiteComponents struct {
	Context context.Context
	Logger  *logrus.Logger
}

// SharedTestSuite creates a standardized test suite setup for shared package tests
func SharedTestSuite(testName string) *SharedTestSuiteComponents {
	logger := logrus.New()
	logger.SetLevel(logrus.FatalLevel) // Suppress logs during tests

	return &SharedTestSuiteComponents{
		Context: context.Background(),
		Logger:  logger,
	}
}

// BasicTestSuite creates a minimal test suite setup for simple utility tests
func BasicTestSuite(testName string) *SharedTestSuiteComponents {
	return SharedTestSuite(testName)
}
