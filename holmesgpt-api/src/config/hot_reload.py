"""
ConfigMap Hot-Reload for HolmesGPT API.

BR-HAPI-199: ConfigMap Hot-Reload
DD-HAPI-004: ConfigMap Hot-Reload Design

This module implements file-based ConfigMap hot-reload using either:
1. watchdog library (preferred, event-based)
2. Polling fallback (if watchdog unavailable)

The implementation follows DD-INFRA-001 pattern for Python services.
"""

import hashlib
import logging
import os
import threading
import time
from typing import Any, Callable, Dict, Optional

import yaml

# Try to import watchdog, fall back to polling if unavailable
try:
    from watchdog.events import FileSystemEventHandler
    from watchdog.observers import Observer
    WATCHDOG_AVAILABLE = True
except ImportError:
    WATCHDOG_AVAILABLE = False


class FileWatcher:
    """
    Hot-reload file watcher for ConfigMap-mounted configuration.

    Uses watchdog library (Python equivalent of fsnotify) when available,
    falls back to polling when watchdog is not installed.

    Implements DD-INFRA-001 pattern for Python.

    Features:
    - Event-based detection (watchdog) or polling fallback
    - Debounced change detection (200ms)
    - Hash-based duplicate detection
    - Graceful degradation on callback errors
    - Metrics for reload count and errors

    Usage:
        def on_config_change(content: str):
            config = yaml.safe_load(content)
            # Apply new config

        watcher = FileWatcher("/etc/holmesgpt/config.yaml", on_config_change, logger)
        watcher.start()
        # ... service runs ...
        watcher.stop()
    """

    def __init__(
        self,
        path: str,
        callback: Callable[[str], None],
        logger: logging.Logger
    ) -> None:
        """
        Initialize FileWatcher.

        Args:
            path: Path to the config file to watch
            callback: Function to call with new content on changes
            logger: Logger for status and error messages
        """
        self._path = path
        self._callback = callback
        self._logger = logger

        # State tracking
        self._last_hash: str = ""
        self._reload_count: int = 0
        self._error_count: int = 0

        # Thread safety
        self._lock = threading.RLock()
        self._running = False

        # Debouncing
        self._debounce_timer: Optional[threading.Timer] = None
        self._debounce_seconds = 0.2  # 200ms debounce

        # Observer (watchdog or polling thread)
        self._observer: Optional[Any] = None
        self._poll_thread: Optional[threading.Thread] = None
        self._poll_interval = 1.0  # 1 second polling interval (fallback)

    def start(self) -> None:
        """
        Start watching the config file.

        Raises:
            FileNotFoundError: If config file doesn't exist
        """
        if not os.path.exists(self._path):
            raise FileNotFoundError(f"Config file not found: {self._path}")

        with self._lock:
            if self._running:
                return

            # Initial load
            self._load_and_notify()

            self._running = True

            if WATCHDOG_AVAILABLE:
                self._start_watchdog()
            else:
                self._start_polling()

            self._logger.info(
                f"📂 DD-HAPI-004: FileWatcher started - "
                f"path={self._path}, mode={'watchdog' if WATCHDOG_AVAILABLE else 'polling'}"
            )

    def stop(self) -> None:
        """Stop watching and clean up resources."""
        with self._lock:
            if not self._running:
                return

            self._running = False

            # Cancel pending debounce timer
            if self._debounce_timer:
                self._debounce_timer.cancel()
                self._debounce_timer = None

            # Stop observer
            if WATCHDOG_AVAILABLE and self._observer:
                self._observer.stop()
                self._observer.join(timeout=2.0)
                self._observer = None

            # Stop polling thread
            if self._poll_thread and self._poll_thread.is_alive():
                self._poll_thread.join(timeout=2.0)
                self._poll_thread = None

            self._logger.info(
                f"📂 DD-HAPI-004: FileWatcher stopped - "
                f"reloads={self._reload_count}, errors={self._error_count}"
            )

    @property
    def last_hash(self) -> str:
        """Hash of current active configuration."""
        with self._lock:
            return self._last_hash

    @property
    def reload_count(self) -> int:
        """Total successful reloads since start."""
        with self._lock:
            return self._reload_count

    @property
    def error_count(self) -> int:
        """Total failed reload attempts since start."""
        with self._lock:
            return self._error_count

    def _start_watchdog(self) -> None:
        """Start watchdog-based file watching."""
        handler = _FileChangeHandler(self._on_file_change)
        self._observer = Observer()

        # Watch the directory containing the file
        watch_dir = os.path.dirname(self._path) or "."
        self._observer.schedule(handler, watch_dir, recursive=False)
        self._observer.start()

    def _start_polling(self) -> None:
        """Start polling-based file watching (fallback)."""
        self._poll_thread = threading.Thread(
            target=self._poll_loop,
            daemon=True,
            name="FileWatcher-poll"
        )
        self._poll_thread.start()
        self._logger.warning(
            "⚠️ DD-HAPI-004: watchdog not available, using polling fallback"
        )

    def _poll_loop(self) -> None:
        """Polling loop for file changes."""
        last_mtime = 0.0

        while self._running:
            try:
                if os.path.exists(self._path):
                    mtime = os.path.getmtime(self._path)
                    if mtime > last_mtime:
                        last_mtime = mtime
                        self._on_file_change()
            except Exception as e:
                self._logger.error(f"Poll error: {e}")

            time.sleep(self._poll_interval)

    def _on_file_change(self) -> None:
        """Handle file change event (debounced)."""
        with self._lock:
            if not self._running:
                return

            # Cancel existing debounce timer
            if self._debounce_timer:
                self._debounce_timer.cancel()

            # Schedule debounced load
            self._debounce_timer = threading.Timer(
                self._debounce_seconds,
                self._load_and_notify
            )
            self._debounce_timer.start()

    def _load_and_notify(self) -> None:
        """Load file content, check hash, and notify callback."""
        try:
            with open(self._path, 'r') as f:
                content = f.read()

            # Compute hash
            content_hash = hashlib.sha256(content.encode()).hexdigest()

            with self._lock:
                # Skip if content hasn't changed
                if content_hash == self._last_hash:
                    return

                self._last_hash = content_hash

            # Call callback (outside lock to avoid deadlock)
            self._callback(content)

            with self._lock:
                self._reload_count += 1

            self._logger.info(
                f"✅ DD-HAPI-004: Config reloaded - "
                f"hash={content_hash[:16]}..., reloads={self._reload_count}"
            )

        except Exception as e:
            with self._lock:
                self._error_count += 1

            self._logger.error(
                f"❌ DD-HAPI-004: Config reload failed - "
                f"error={e}, errors={self._error_count}"
            )


if WATCHDOG_AVAILABLE:
    class _FileChangeHandler(FileSystemEventHandler):
        """Watchdog event handler for config file changes."""

        def __init__(self, callback: Callable[[], None]) -> None:
            super().__init__()
            self._callback = callback

        def on_modified(self, event) -> None:
            if not event.is_directory:
                self._callback()

        def on_created(self, event) -> None:
            # Handle ConfigMap symlink swap
            if not event.is_directory:
                self._callback()


class ConfigManager:
    """
    Thread-safe configuration manager with hot-reload support.

    Provides typed getters for configuration values that are
    automatically updated when the underlying ConfigMap changes.

    Usage:
        config = ConfigManager("/etc/holmesgpt/config.yaml", logger)
        config.start()

        # Access config (thread-safe)
        model = config.get_llm_model()
        toolsets = config.get_toolsets()

        # On shutdown
        config.stop()
    """

    # Default configuration values
    DEFAULTS = {
        "llm": {
            "model": "gpt-4",
            "provider": "openai",
            "endpoint": None,
            "max_retries": 3,
            "timeout_seconds": 60,
            "temperature": 0.7,
            "max_tokens_per_request": 4096,
        },
        "toolsets": {},
        "log_level": "INFO",
        "service_name": "holmesgpt-api",
        "version": "1.0.0",
    }

    def __init__(
        self,
        path: str,
        logger: logging.Logger,
        enable_hot_reload: bool = True
    ) -> None:
        """
        Initialize ConfigManager.

        Args:
            path: Path to the config file
            logger: Logger for status and error messages
            enable_hot_reload: Whether to enable hot-reload (default: True)
        """
        self._path = path
        self._logger = logger
        self._enable_hot_reload = enable_hot_reload

        # Current config (protected by lock)
        self._config: Dict[str, Any] = dict(self.DEFAULTS)
        self._lock = threading.RLock()

        # File watcher
        self._watcher: Optional[FileWatcher] = None

    def start(self) -> None:
        """Start the config manager and file watcher."""
        if self._enable_hot_reload:
            self._watcher = FileWatcher(
                self._path,
                self._on_config_change,
                self._logger
            )
            self._watcher.start()
        else:
            # Just load once
            if os.path.exists(self._path):
                with open(self._path, 'r') as f:
                    self._on_config_change(f.read())

    def stop(self) -> None:
        """Stop the config manager and file watcher."""
        if self._watcher:
            self._watcher.stop()
            self._watcher = None

    def _on_config_change(self, content: str) -> None:
        """Handle configuration change."""
        try:
            new_config = yaml.safe_load(content) or {}

            with self._lock:
                # Merge with defaults (new config takes precedence)
                self._config = self._merge_config(self.DEFAULTS, new_config)

            self._logger.info(
                f"📝 DD-HAPI-004: Config applied - "
                f"llm.model={self.get_llm_model()}, "
                f"toolsets={list(self.get_toolsets().keys())}"
            )

        except yaml.YAMLError as e:
            self._logger.error(
                f"❌ DD-HAPI-004: Invalid YAML, keeping previous config - error={e}"
            )
            raise  # Re-raise to increment error count in FileWatcher

    def _merge_config(
        self,
        defaults: Dict[str, Any],
        override: Dict[str, Any]
    ) -> Dict[str, Any]:
        """Deep merge configuration with defaults."""
        result = dict(defaults)

        for key, value in override.items():
            if key in result and isinstance(result[key], dict) and isinstance(value, dict):
                result[key] = self._merge_config(result[key], value)
            else:
                result[key] = value

        return result

    # =========================================================================
    # Typed Getters (Thread-Safe)
    # =========================================================================

    def get_llm_model(self) -> str:
        """Get current LLM model name."""
        with self._lock:
            return self._config.get("llm", {}).get("model", self.DEFAULTS["llm"]["model"])

    def get_llm_provider(self) -> str:
        """Get current LLM provider."""
        with self._lock:
            return self._config.get("llm", {}).get("provider", self.DEFAULTS["llm"]["provider"])

    def get_llm_endpoint(self) -> Optional[str]:
        """Get custom LLM endpoint (None for default)."""
        with self._lock:
            return self._config.get("llm", {}).get("endpoint")

    def get_llm_max_retries(self) -> int:
        """Get LLM max retries."""
        with self._lock:
            return self._config.get("llm", {}).get("max_retries", self.DEFAULTS["llm"]["max_retries"])

    def get_llm_timeout(self) -> int:
        """Get LLM timeout in seconds."""
        with self._lock:
            return self._config.get("llm", {}).get("timeout_seconds", self.DEFAULTS["llm"]["timeout_seconds"])

    def get_llm_temperature(self) -> float:
        """Get LLM temperature."""
        with self._lock:
            return self._config.get("llm", {}).get("temperature", self.DEFAULTS["llm"]["temperature"])

    def get_toolsets(self) -> Dict[str, Any]:
        """Get toolsets configuration."""
        with self._lock:
            return dict(self._config.get("toolsets", {}))

    def get_log_level(self) -> str:
        """Get log level."""
        with self._lock:
            return self._config.get("log_level", self.DEFAULTS["log_level"])

    def get_raw_config(self) -> Dict[str, Any]:
        """Get full raw config (for backwards compatibility)."""
        with self._lock:
            return dict(self._config)

    # =========================================================================
    # Metrics
    # =========================================================================

    @property
    def reload_count(self) -> int:
        """Total successful config reloads."""
        if self._watcher:
            return self._watcher.reload_count
        return 0

    @property
    def error_count(self) -> int:
        """Total config reload errors."""
        if self._watcher:
            return self._watcher.error_count
        return 0

    @property
    def last_hash(self) -> str:
        """Hash of current config."""
        if self._watcher:
            return self._watcher.last_hash
        return ""


