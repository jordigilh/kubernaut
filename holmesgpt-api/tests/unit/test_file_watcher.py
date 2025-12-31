"""
Unit tests for FileWatcher - ConfigMap hot-reload file watching.

BR-HAPI-199: ConfigMap Hot-Reload
DD-HAPI-004: ConfigMap Hot-Reload Design

TDD Phase: RED - These tests should FAIL until FileWatcher is implemented.
"""

import hashlib
import logging
import os
import tempfile
import threading
import time
from unittest.mock import MagicMock

import pytest
import yaml


class TestFileWatcherInitialization:
    """Test FileWatcher initialization and lifecycle."""

    def test_file_watcher_initial_load_calls_callback(self):
        """
        BR-HAPI-199: Service SHALL reload configuration from ConfigMap without pod restart.

        FileWatcher should call callback with initial content on start().
        """
        # Import here to trigger ImportError if not implemented
        from src.config.hot_reload import FileWatcher

        # Arrange
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("llm:\n  model: gpt-4\n")
            config_path = f.name

        try:
            callback = MagicMock()
            logger = logging.getLogger("test")

            # Act
            watcher = FileWatcher(config_path, callback, logger)
            watcher.start()

            # Assert
            callback.assert_called_once()
            content = callback.call_args[0][0]
            assert "model: gpt-4" in content

            watcher.stop()
        finally:
            os.unlink(config_path)

    def test_file_watcher_raises_on_missing_file(self):
        """
        BR-HAPI-199: Service SHALL gracefully degrade on invalid configuration.

        FileWatcher should raise FileNotFoundError if config file doesn't exist on start().
        """
        from src.config.hot_reload import FileWatcher

        logger = logging.getLogger("test")
        callback = MagicMock()

        watcher = FileWatcher("/nonexistent/config.yaml", callback, logger)

        with pytest.raises(FileNotFoundError):
            watcher.start()

    def test_file_watcher_stop_cleanup(self):
        """
        BR-HAPI-199: Service SHALL gracefully degrade on invalid configuration.

        FileWatcher.stop() should clean up resources without error.
        """
        from src.config.hot_reload import FileWatcher

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("test: value\n")
            config_path = f.name

        try:
            callback = MagicMock()
            logger = logging.getLogger("test")

            watcher = FileWatcher(config_path, callback, logger)
            watcher.start()
            watcher.stop()

            # Should not raise, watcher should be stopped
            # Verify watcher is no longer running
            assert not watcher._running
        finally:
            os.unlink(config_path)


class TestFileWatcherChangeDetection:
    """Test FileWatcher change detection and callback triggering."""

    def test_file_watcher_detects_change(self, wait_for):
        """
        BR-HAPI-199: Service SHALL reload configuration from ConfigMap within 90 seconds.

        FileWatcher should call callback when file content changes.
        """
        from src.config.hot_reload import FileWatcher

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("llm:\n  model: gpt-4\n")
            config_path = f.name

        try:
            callback_calls = []

            def track_callback(content):
                callback_calls.append(content)

            logger = logging.getLogger("test")
            watcher = FileWatcher(config_path, track_callback, logger)
            watcher.start()

            # Clear initial load call
            initial_calls = len(callback_calls)

            # Modify file
            with open(config_path, 'w') as f:
                f.write("llm:\n  model: claude-3-5-sonnet\n")

            # Wait for change detection (typically <100ms instead of 2s+ with sleep)
            wait_for(
                lambda: len(callback_calls) > initial_calls,
                timeout=2.0,
                error_msg="FileWatcher should detect file change"
            )

            watcher.stop()

            # Assert callback was called for change
            assert len(callback_calls) > initial_calls
            assert "claude-3-5-sonnet" in callback_calls[-1]
        finally:
            os.unlink(config_path)

    def test_file_watcher_debounces_rapid_changes(self, wait_for):
        """
        BR-HAPI-199: Service SHALL reload configuration from ConfigMap.

        Rapid file changes should be debounced to avoid excessive callback calls.
        """
        from src.config.hot_reload import FileWatcher

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("value: 0\n")
            config_path = f.name

        try:
            callback_calls = []

            def track_callback(content):
                callback_calls.append(content)

            logger = logging.getLogger("test")
            watcher = FileWatcher(config_path, track_callback, logger)
            watcher.start()

            # Wait for initial load
            wait_for(lambda: len(callback_calls) > 0, timeout=1.0)
            initial_count = len(callback_calls)

            # Rapid fire changes (simulating kubelet atomic swap)
            for i in range(5):
                with open(config_path, 'w') as f:
                    f.write(f"value: {i + 1}\n")
                time.sleep(0.05)  # 50ms between writes (simulating external system)

            # Wait for debounce to settle (typically ~200ms instead of 500ms)
            wait_for(lambda: len(callback_calls) > initial_count, timeout=1.0)
            time.sleep(0.1)  # Brief settle for debounce window

            watcher.stop()

            # Should NOT have 5 separate callbacks - should be debounced
            # Expect 1-2 callbacks, not 5
            reload_calls = len(callback_calls) - initial_count
            assert reload_calls < 5, f"Expected debouncing but got {reload_calls} callbacks"
        finally:
            os.unlink(config_path)


class TestFileWatcherHashTracking:
    """Test FileWatcher hash tracking for duplicate detection."""

    def test_file_watcher_tracks_hash(self):
        """
        BR-HAPI-199: Service SHALL log configuration hash on reload for audit trail.

        FileWatcher should track content hash and expose it.
        """
        from src.config.hot_reload import FileWatcher

        config_content = "llm:\n  model: gpt-4\n"
        expected_hash = hashlib.sha256(config_content.encode()).hexdigest()

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(config_content)
            config_path = f.name

        try:
            callback = MagicMock()
            logger = logging.getLogger("test")

            watcher = FileWatcher(config_path, callback, logger)
            watcher.start()

            assert watcher.last_hash == expected_hash

            watcher.stop()
        finally:
            os.unlink(config_path)

    def test_file_watcher_skips_unchanged_content(self):
        """
        BR-HAPI-199: Service SHALL reload configuration from ConfigMap.

        FileWatcher should skip callback if content hash hasn't changed.
        """
        from src.config.hot_reload import FileWatcher

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("llm:\n  model: gpt-4\n")
            config_path = f.name

        try:
            callback_calls = []

            def track_callback(content):
                callback_calls.append(content)

            logger = logging.getLogger("test")
            watcher = FileWatcher(config_path, track_callback, logger)  # Fixed: was 'callback'
            watcher.start()

            initial_count = len(callback_calls)

            # "Touch" file without changing content
            time.sleep(0.3)
            with open(config_path, 'w') as f:
                f.write("llm:\n  model: gpt-4\n")  # Same content

            time.sleep(0.5)
            watcher.stop()

            # Callback should NOT be called again (same hash)
            assert len(callback_calls) == initial_count
        finally:
            os.unlink(config_path)


class TestFileWatcherMetrics:
    """Test FileWatcher metrics and counters."""

    def test_file_watcher_reload_count(self, wait_for):
        """
        BR-HAPI-199: Metrics exposed for reload count and errors.

        FileWatcher should track successful reload count.
        """
        from src.config.hot_reload import FileWatcher

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("value: 1\n")
            config_path = f.name

        try:
            callback = MagicMock()
            logger = logging.getLogger("test")

            watcher = FileWatcher(config_path, callback, logger)
            watcher.start()

            assert watcher.reload_count == 1  # Initial load

            # Trigger reload
            with open(config_path, 'w') as f:
                f.write("value: 2\n")

            # Wait for reload (typically <100ms instead of 3s with 2× sleep(1.5))
            wait_for(lambda: watcher.reload_count >= 2, timeout=2.0, error_msg="Expected reload_count to increment")

            watcher.stop()
        finally:
            os.unlink(config_path)

    def test_file_watcher_error_count(self):
        """
        BR-HAPI-199: Metrics exposed for reload count and errors.

        FileWatcher should track error count when callback raises.
        """
        from src.config.hot_reload import FileWatcher

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("value: 1\n")
            config_path = f.name

        try:
            def failing_callback(content):
                if "value: 2" in content:
                    raise ValueError("Simulated validation error")

            logger = logging.getLogger("test")
            watcher = FileWatcher(config_path, failing_callback, logger)
            watcher.start()

            # Initial load should succeed
            assert watcher.error_count == 0

            # Trigger reload that will fail in callback - wait for poll interval
            time.sleep(1.5)
            with open(config_path, 'w') as f:
                f.write("value: 2\n")
            time.sleep(1.5)  # Wait for poll + debounce

            assert watcher.error_count >= 1

            watcher.stop()
        finally:
            os.unlink(config_path)


class TestFileWatcherGracefulDegradation:
    """Test FileWatcher graceful degradation on errors."""

    def test_file_watcher_graceful_on_callback_error(self):
        """
        BR-HAPI-199: Service SHALL gracefully degrade on invalid configuration.

        FileWatcher should continue watching even if callback raises.
        """
        from src.config.hot_reload import FileWatcher

        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("value: 1\n")
            config_path = f.name

        try:
            call_count = [0]

            def sometimes_failing_callback(content):
                call_count[0] += 1
                if call_count[0] == 2:
                    raise ValueError("Simulated error")

            logger = logging.getLogger("test")
            watcher = FileWatcher(config_path, sometimes_failing_callback, logger)
            watcher.start()

            # First reload - wait for poll interval
            time.sleep(1.5)
            with open(config_path, 'w') as f:
                f.write("value: 2\n")
            time.sleep(1.5)

            # Second reload (will fail, but watcher should continue)
            # This is the callback that raises

            # Third reload (should still work after error)
            with open(config_path, 'w') as f:
                f.write("value: 3\n")
            time.sleep(1.5)

            watcher.stop()

            # Watcher should have continued after error
            assert call_count[0] >= 3, f"Watcher should continue after callback error, got {call_count[0]} calls"
        finally:
            os.unlink(config_path)

