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
