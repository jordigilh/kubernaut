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
