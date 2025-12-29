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

package helpers

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/helpers"
)

var _ = Describe("Logging Helpers", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("WithMethodLogging", func() {
		It("should create logger with method context", func() {
			logger := helpers.WithMethodLogging(ctx, "TestMethod", "key", "value")
			Expect(logger).ToNot(BeNil())
			// Logger should be created with method context
			// Actual logging output is not tested (logr interface limitation)
		})

		It("should handle multiple key-value pairs", func() {
			logger := helpers.WithMethodLogging(ctx, "TestMethod",
				"key1", "value1",
				"key2", "value2",
				"key3", "value3",
			)
			Expect(logger).ToNot(BeNil())
		})

		It("should handle no additional context", func() {
			logger := helpers.WithMethodLogging(ctx, "TestMethod")
			Expect(logger).ToNot(BeNil())
		})
	})

	Describe("LogAndWrapError", func() {
		var logger logr.Logger

		BeforeEach(func() {
			logger = helpers.WithMethodLogging(ctx, "TestMethod")
		})

		It("should wrap error with message", func() {
			originalErr := fmt.Errorf("original error")
			wrappedErr := helpers.LogAndWrapError(logger, originalErr, "Failed to process")

			Expect(wrappedErr).To(HaveOccurred())
			Expect(wrappedErr.Error()).To(ContainSubstring("Failed to process"))
			Expect(wrappedErr.Error()).To(ContainSubstring("original error"))
		})

		It("should preserve error chain", func() {
			originalErr := fmt.Errorf("original error")
			wrappedErr := helpers.LogAndWrapError(logger, originalErr, "Failed to process")

			// Verify error unwrapping works
			Expect(errors.Unwrap(wrappedErr)).To(Equal(originalErr))
		})
	})

	Describe("LogAndWrapErrorf", func() {
		var logger logr.Logger

		BeforeEach(func() {
			logger = helpers.WithMethodLogging(ctx, "TestMethod")
		})

		It("should format error message with arguments", func() {
			originalErr := fmt.Errorf("original error")
			wrappedErr := helpers.LogAndWrapErrorf(logger, originalErr, "Failed to process RR %s in phase %s", "test-rr", "Processing")

			Expect(wrappedErr).To(HaveOccurred())
			Expect(wrappedErr.Error()).To(ContainSubstring("Failed to process RR test-rr in phase Processing"))
			Expect(wrappedErr.Error()).To(ContainSubstring("original error"))
		})

		It("should handle no format arguments", func() {
			originalErr := fmt.Errorf("original error")
			wrappedErr := helpers.LogAndWrapErrorf(logger, originalErr, "Failed to process")

			Expect(wrappedErr).To(HaveOccurred())
			Expect(wrappedErr.Error()).To(ContainSubstring("Failed to process"))
		})
	})

	Describe("LogInfo", func() {
		var logger logr.Logger

		BeforeEach(func() {
			logger = helpers.WithMethodLogging(ctx, "TestMethod")
		})

		It("should log info message", func() {
			// No panic should occur
			helpers.LogInfo(logger, "Test message", "key", "value")
		})

		It("should handle multiple key-value pairs", func() {
			helpers.LogInfo(logger, "Test message",
				"key1", "value1",
				"key2", "value2",
			)
		})

		It("should handle no key-value pairs", func() {
			helpers.LogInfo(logger, "Test message")
		})
	})

	Describe("LogInfoV", func() {
		var logger logr.Logger

		BeforeEach(func() {
			logger = helpers.WithMethodLogging(ctx, "TestMethod")
		})

		It("should log verbose info message", func() {
			helpers.LogInfoV(logger, 1, "Verbose message", "key", "value")
		})

		It("should handle different verbosity levels", func() {
			helpers.LogInfoV(logger, 0, "Level 0")
			helpers.LogInfoV(logger, 1, "Level 1")
			helpers.LogInfoV(logger, 2, "Level 2")
		})
	})

	Describe("LogError", func() {
		var logger logr.Logger

		BeforeEach(func() {
			logger = helpers.WithMethodLogging(ctx, "TestMethod")
		})

		It("should log error message", func() {
			err := fmt.Errorf("test error")
			helpers.LogError(logger, err, "Error occurred", "key", "value")
		})

		It("should handle nil error", func() {
			// Should not panic with nil error
			helpers.LogError(logger, nil, "No error")
		})

		It("should handle multiple key-value pairs", func() {
			err := fmt.Errorf("test error")
			helpers.LogError(logger, err, "Error occurred",
				"key1", "value1",
				"key2", "value2",
			)
		})
	})
})
