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

package hotreload

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	prodhotreload "github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHotreload(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hotreload Suite")
}

// Unit Tests: FileWatcher (DD-INFRA-001)
// Per DD-INFRA-001: ConfigMap Hot-Reload Pattern
var _ = Describe("FileWatcher", func() {
	var (
		ctx      context.Context
		cancel   context.CancelFunc
		logger   logr.Logger
		tempDir  string
		testFile string
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		logger = logr.Discard()

		// Create temp directory for test files
		var err error
		tempDir, err = os.MkdirTemp("", "hotreload-test-*")
		Expect(err).NotTo(HaveOccurred())

		testFile = filepath.Join(tempDir, "config.txt")
	})

	AfterEach(func() {
		cancel()
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	})

	// Helper to create test file
	createFile := func(content string) {
		err := os.WriteFile(testFile, []byte(content), 0644)
		Expect(err).NotTo(HaveOccurred())
	}

	// ============================================================================
	// CONSTRUCTOR TESTS
	// ============================================================================

	Context("NewFileWatcher", func() {
		It("should create watcher with valid parameters", func() {
			createFile("initial content")

			watcher, err := prodhotreload.NewFileWatcher(testFile, func(string) error { return nil }, logger)

			Expect(err).NotTo(HaveOccurred())
			Expect(watcher).NotTo(BeNil())
			// Business outcome: watcher is created successfully (internal path field is implementation detail)
		})

		It("should return error for empty path", func() {
			_, err := prodhotreload.NewFileWatcher("", func(string) error { return nil }, logger)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("path is required"))
		})

		It("should return error for nil callback", func() {
			_, err := prodhotreload.NewFileWatcher("/some/path", nil, logger)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("callback is required"))
		})
	})

	// ============================================================================
	// START/STOP LIFECYCLE TESTS
	// ============================================================================

	Context("Start", func() {
		It("should load initial content and call callback", func() {
			createFile("initial content")

			var receivedContent string
			watcher, err := prodhotreload.NewFileWatcher(testFile, func(content string) error {
				receivedContent = content
				return nil
			}, logger)
			Expect(err).NotTo(HaveOccurred())

			err = watcher.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer watcher.Stop()

			Expect(receivedContent).To(Equal("initial content"))
			Expect(watcher.GetLastHash()).NotTo(BeEmpty())
			Expect(watcher.GetReloadCount()).To(Equal(int64(1)))
		})

		It("should return error if file does not exist", func() {
			watcher, err := prodhotreload.NewFileWatcher("/nonexistent/file.txt", func(string) error { return nil }, logger)
			Expect(err).NotTo(HaveOccurred())

			err = watcher.Start(ctx)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load initial content"))
		})

		It("should return error if callback fails on initial load", func() {
			createFile("initial content")

			watcher, err := prodhotreload.NewFileWatcher(testFile, func(string) error {
				return os.ErrInvalid
			}, logger)
			Expect(err).NotTo(HaveOccurred())

			err = watcher.Start(ctx)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("callback failed"))
		})
	})

	Context("Stop", func() {
		It("should stop gracefully", func() {
			createFile("initial content")

			watcher, err := prodhotreload.NewFileWatcher(testFile, func(string) error { return nil }, logger)
			Expect(err).NotTo(HaveOccurred())

			err = watcher.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Stop should complete without hanging
			done := make(chan struct{})
			go func() {
				watcher.Stop()
				close(done)
			}()

			Eventually(done, 2*time.Second).Should(BeClosed())
		})
	})

	// ============================================================================
	// HOT-RELOAD TESTS
	// ============================================================================

	Context("Hot-Reload on File Change", func() {
		It("should detect file content change and call callback", func() {
			createFile("initial content")

			var callCount atomic.Int32
			var lastContent atomic.Value
			watcher, err := prodhotreload.NewFileWatcher(testFile, func(content string) error {
				callCount.Add(1)
				lastContent.Store(content)
				return nil
			}, logger)
			Expect(err).NotTo(HaveOccurred())

			err = watcher.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer watcher.Stop()

			Expect(callCount.Load()).To(Equal(int32(1)))

			// Update file content
			err = os.WriteFile(testFile, []byte("updated content"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Wait for reload
			Eventually(func() int32 {
				return callCount.Load()
			}, 3*time.Second, 100*time.Millisecond).Should(Equal(int32(2)))

			Expect(lastContent.Load().(string)).To(Equal("updated content"))
			Expect(watcher.GetReloadCount()).To(Equal(int64(2)))
		})

		It("should skip reload if content hash unchanged", func() {
			createFile("same content")

			var callCount atomic.Int32
			watcher, err := prodhotreload.NewFileWatcher(testFile, func(string) error {
				callCount.Add(1)
				return nil
			}, logger)
			Expect(err).NotTo(HaveOccurred())

			err = watcher.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer watcher.Stop()

			initialHash := watcher.GetLastHash()

			// Write same content
			err = os.WriteFile(testFile, []byte("same content"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Wait a bit for potential reload
			time.Sleep(500 * time.Millisecond)

			// Should NOT have reloaded (same hash)
			Expect(callCount.Load()).To(Equal(int32(1)))
			Expect(watcher.GetLastHash()).To(Equal(initialHash))
		})

		It("should keep old content if callback rejects new content", func() {
			createFile("initial content")

			var rejectNew atomic.Bool
			watcher, err := prodhotreload.NewFileWatcher(testFile, func(content string) error {
				if rejectNew.Load() {
					return os.ErrInvalid // Reject new content
				}
				return nil
			}, logger)
			Expect(err).NotTo(HaveOccurred())

			err = watcher.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer watcher.Stop()

			initialHash := watcher.GetLastHash()
			rejectNew.Store(true)

			// Update file content
			err = os.WriteFile(testFile, []byte("bad content"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Wait for reload attempt
			Eventually(func() int64 {
				return watcher.GetErrorCount()
			}, 3*time.Second, 100*time.Millisecond).Should(Equal(int64(1)))

			// Should still have old content
			Expect(watcher.GetLastHash()).To(Equal(initialHash))
			Expect(watcher.GetLastContent()).To(Equal("initial content"))
			Expect(watcher.GetReloadCount()).To(Equal(int64(1)))
		})
	})

	// ============================================================================
	// STATUS METHODS TESTS
	// ============================================================================

	Context("Status Methods", func() {
		It("should return correct reload time", func() {
			createFile("initial content")

			beforeStart := time.Now()
			watcher, err := prodhotreload.NewFileWatcher(testFile, func(string) error { return nil }, logger)
			Expect(err).NotTo(HaveOccurred())

			err = watcher.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer watcher.Stop()
			afterStart := time.Now()

			reloadTime := watcher.GetLastReloadTime()
			Expect(reloadTime).To(BeTemporally(">=", beforeStart))
			Expect(reloadTime).To(BeTemporally("<=", afterStart))
		})

		It("should return content via GetLastContent", func() {
			createFile("test content 123")

			watcher, err := prodhotreload.NewFileWatcher(testFile, func(string) error { return nil }, logger)
			Expect(err).NotTo(HaveOccurred())

			err = watcher.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer watcher.Stop()

			Expect(watcher.GetLastContent()).To(Equal("test content 123"))
		})

		It("should compute consistent hash", func() {
			createFile("content for hash")

			watcher, err := prodhotreload.NewFileWatcher(testFile, func(string) error { return nil }, logger)
			Expect(err).NotTo(HaveOccurred())

			err = watcher.Start(ctx)
			Expect(err).NotTo(HaveOccurred())
			defer watcher.Stop()

			hash1 := watcher.GetLastHash()
			Expect(hash1).NotTo(BeEmpty())
			// BR-SP-072: Full SHA256 hash (64 hex chars) for audit trail and policy version tracking
			Expect(len(hash1)).To(Equal(64)) // Full SHA256 hex = 64 chars
		})
	})

	// ============================================================================
	// CONTEXT CANCELLATION TESTS
	// ============================================================================

	Context("Context Cancellation", func() {
		It("should stop watch loop when context is cancelled", func() {
			createFile("initial content")

			watcher, err := prodhotreload.NewFileWatcher(testFile, func(string) error { return nil }, logger)
			Expect(err).NotTo(HaveOccurred())

			err = watcher.Start(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Cancel context
			cancel()

			// Business outcome: watcher stops gracefully (verified by Stop() completing without error)
			watcher.Stop()
			// If Stop() hangs, the test will timeout - that's the business outcome we care about
		})
	})
})
