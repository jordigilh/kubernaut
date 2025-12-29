#!/usr/bin/env python3
"""
PoC: ConfigMap Hot-Reload Pattern for Python

Evaluates DD-INFRA-001 implementation complexity for Python services.

Requirements tested:
1. File-based ConfigMap watching (using watchdog library)
2. Callback on content change
3. Graceful degradation (keep old config on error)
4. Hash-based version tracking
5. Thread-safe config access

Usage:
    # Terminal 1: Run the watcher
    python hot_reload_poc.py /path/to/config.yaml

    # Terminal 2: Modify the file
    echo "new_key: new_value" >> /path/to/config.yaml

Dependencies:
    pip install watchdog pyyaml
"""

import hashlib
import logging
import os
import sys
import threading
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Callable, Dict, Optional

# External dependencies
try:
    from watchdog.observers import Observer
    from watchdog.events import FileSystemEventHandler, FileModifiedEvent, FileCreatedEvent
    import yaml
except ImportError as e:
    print(f"Missing dependency: {e}")
    print("Install with: pip install watchdog pyyaml")
    sys.exit(1)


# ============================================================================
# CORE HOT-RELOAD IMPLEMENTATION
# ============================================================================

class FileWatcher:
    """
    Hot-reload file watcher for ConfigMap-mounted configuration files.

    Python equivalent of DD-INFRA-001 Go implementation.
    Uses watchdog library (similar to fsnotify in Go).

    Key Features:
    - Watches file for changes (handles ConfigMap symlink updates)
    - Calls callback with new content
    - Graceful degradation: keeps old config if callback fails
    - Thread-safe config access
    - Hash-based change detection
    """

    def __init__(
        self,
        path: str,
        callback: Callable[[str], None],
        logger: Optional[logging.Logger] = None
    ):
        """
        Initialize file watcher.

        Args:
            path: Path to config file (e.g., /etc/kubernaut/config/settings.yaml)
            callback: Function called with new content when file changes
                      Should raise exception to reject invalid config
            logger: Optional logger instance
        """
        self.path = Path(path).resolve()
        self.callback = callback
        self.logger = logger or logging.getLogger(__name__)

        # State tracking
        self._lock = threading.RLock()
        self._last_content: str = ""
        self._last_hash: str = ""
        self._last_reload: Optional[datetime] = None
        self._reload_count: int = 0
        self._error_count: int = 0

        # Watchdog observer
        self._observer: Optional[Observer] = None
        self._running = False

    def start(self) -> None:
        """
        Start watching the file.

        Loads initial content and starts background watcher.
        Raises exception if initial load fails.
        """
        self.logger.info(f"Starting file hot-reloader for {self.path}")

        # Load initial content (fail if invalid)
        self._load_initial()

        # Create event handler
        handler = _FileChangeHandler(self)

        # Watch the parent directory (ConfigMap uses symlinks)
        watch_dir = str(self.path.parent)

        # Start observer
        self._observer = Observer()
        self._observer.schedule(handler, watch_dir, recursive=False)
        self._observer.start()
        self._running = True

        self.logger.info(
            f"File hot-reloader started successfully",
            extra={"initial_hash": self._last_hash[:8]}
        )

    def stop(self) -> None:
        """Stop the file watcher gracefully."""
        self.logger.info("Stopping file hot-reloader")
        self._running = False
        if self._observer:
            self._observer.stop()
            self._observer.join(timeout=5.0)

    def _load_initial(self) -> None:
        """Load initial file content. Raises exception on failure."""
        if not self.path.exists():
            raise FileNotFoundError(f"Config file not found: {self.path}")

        content = self.path.read_text()

        # Validate via callback (will raise if invalid)
        self.callback(content)

        # Update state
        with self._lock:
            self._last_content = content
            self._last_hash = self._compute_hash(content)
            self._last_reload = datetime.now(timezone.utc)

    def _handle_file_change(self) -> None:
        """
        Process file content changes.

        Called by watchdog event handler.
        Implements graceful degradation.
        """
        # Small delay for symlink updates to complete
        time.sleep(0.1)

        try:
            content = self.path.read_text()
        except Exception as e:
            self.logger.error(f"Failed to read updated file: {e}")
            return

        # Check if content actually changed
        new_hash = self._compute_hash(content)

        with self._lock:
            if new_hash == self._last_hash:
                return  # No change (spurious event)
            old_hash = self._last_hash

        self.logger.debug(f"File change detected: {old_hash[:8]} -> {new_hash[:8]}")

        # Try to apply new config
        try:
            self.callback(content)
        except Exception as e:
            with self._lock:
                self._error_count += 1
            self.logger.error(
                f"Failed to apply new configuration - keeping previous: {e}",
                extra={"new_hash": new_hash[:8], "error_count": self._error_count}
            )
            return  # Graceful degradation

        # Success - update state
        with self._lock:
            self._last_content = content
            self._last_hash = new_hash
            self._last_reload = datetime.now(timezone.utc)
            self._reload_count += 1

        self.logger.info(
            f"Configuration hot-reloaded successfully",
            extra={"hash": new_hash[:8], "reload_count": self._reload_count}
        )

    @staticmethod
    def _compute_hash(content: str) -> str:
        """Compute SHA-256 hash of content."""
        return hashlib.sha256(content.encode()).hexdigest()

    # Status methods
    @property
    def last_hash(self) -> str:
        with self._lock:
            return self._last_hash

    @property
    def last_reload(self) -> Optional[datetime]:
        with self._lock:
            return self._last_reload

    @property
    def reload_count(self) -> int:
        with self._lock:
            return self._reload_count

    @property
    def error_count(self) -> int:
        with self._lock:
            return self._error_count


class _FileChangeHandler(FileSystemEventHandler):
    """Watchdog event handler for file changes."""

    def __init__(self, watcher: FileWatcher):
        self.watcher = watcher
        self._debounce_timer: Optional[threading.Timer] = None
        self._debounce_lock = threading.Lock()

    def on_modified(self, event):
        if event.is_directory:
            return
        self._trigger_reload(event.src_path)

    def on_created(self, event):
        # ConfigMap updates create new symlink targets
        if event.is_directory:
            return
        self._trigger_reload(event.src_path)

    def _trigger_reload(self, src_path: str):
        """Debounced reload trigger."""
        # Check if this event is for our file
        if Path(src_path).name != self.watcher.path.name:
            # Could be symlink update - check if our file changed
            pass

        # Debounce rapid events (ConfigMap updates can trigger multiple)
        with self._debounce_lock:
            if self._debounce_timer:
                self._debounce_timer.cancel()
            self._debounce_timer = threading.Timer(
                0.2,  # 200ms debounce
                self.watcher._handle_file_change
            )
            self._debounce_timer.start()


# ============================================================================
# EXAMPLE USAGE: YAML CONFIG HOT-RELOAD
# ============================================================================

class ConfigManager:
    """
    Example: Hot-reloadable YAML configuration manager.

    Shows how to use FileWatcher for application config.
    """

    def __init__(self, config_path: str, logger: logging.Logger):
        self._config: Dict[str, Any] = {}
        self._lock = threading.RLock()
        self.logger = logger

        # Create file watcher with validation callback
        self._watcher = FileWatcher(
            path=config_path,
            callback=self._apply_config,
            logger=logger
        )

    def start(self) -> None:
        """Start config manager with hot-reload."""
        self._watcher.start()

    def stop(self) -> None:
        """Stop config manager."""
        self._watcher.stop()

    def _apply_config(self, content: str) -> None:
        """
        Validate and apply new configuration.

        Raises exception to reject invalid config (graceful degradation).
        """
        # Parse YAML
        new_config = yaml.safe_load(content)

        if not isinstance(new_config, dict):
            raise ValueError("Configuration must be a YAML dictionary")

        # Add any validation here
        # e.g., required keys, value ranges, etc.

        # Apply atomically
        with self._lock:
            self._config = new_config

        self.logger.info(f"Applied config with {len(new_config)} keys")

    def get(self, key: str, default: Any = None) -> Any:
        """Thread-safe config access."""
        with self._lock:
            return self._config.get(key, default)

    def get_all(self) -> Dict[str, Any]:
        """Get copy of all config."""
        with self._lock:
            return dict(self._config)

    @property
    def stats(self) -> Dict[str, Any]:
        """Get reload statistics."""
        return {
            "last_hash": self._watcher.last_hash[:8] if self._watcher.last_hash else None,
            "last_reload": self._watcher.last_reload.isoformat() if self._watcher.last_reload else None,
            "reload_count": self._watcher.reload_count,
            "error_count": self._watcher.error_count,
        }


# ============================================================================
# POC TEST
# ============================================================================

def main():
    """
    PoC: Test hot-reload functionality.

    Usage:
        python hot_reload_poc.py [config_path]

    If no path provided, creates a temporary test file.
    """
    import tempfile

    # Setup logging
    logging.basicConfig(
        level=logging.DEBUG,
        format="%(asctime)s - %(levelname)s - %(message)s"
    )
    logger = logging.getLogger("hot-reload-poc")

    # Get or create config file
    if len(sys.argv) > 1:
        config_path = sys.argv[1]
    else:
        # Create temp file for testing
        temp_dir = tempfile.mkdtemp()
        config_path = os.path.join(temp_dir, "config.yaml")

        # Write initial config
        initial_config = """
# Test configuration
llm:
  model: gpt-4
  temperature: 0.7
  max_tokens: 4096

features:
  validation_enabled: true
  retry_count: 3
"""
        with open(config_path, "w") as f:
            f.write(initial_config)

        logger.info(f"Created test config at: {config_path}")

    # Create config manager
    config_manager = ConfigManager(config_path, logger)

    try:
        # Start hot-reload
        config_manager.start()

        print("\n" + "="*60)
        print("🔥 HOT-RELOAD POC RUNNING")
        print("="*60)
        print(f"\nWatching: {config_path}")
        print(f"Initial config: {config_manager.get_all()}")
        print(f"Stats: {config_manager.stats}")
        print("\n📝 Modify the config file to test hot-reload...")
        print("   (Press Ctrl+C to exit)\n")

        # Monitor loop
        last_reload_count = config_manager.stats["reload_count"]
        while True:
            time.sleep(1)

            # Check if reloaded
            current_count = config_manager.stats["reload_count"]
            if current_count > last_reload_count:
                print(f"\n✅ Config reloaded! (count: {current_count})")
                print(f"   New config: {config_manager.get_all()}")
                print(f"   Stats: {config_manager.stats}\n")
                last_reload_count = current_count

    except KeyboardInterrupt:
        print("\n\n⏹️  Stopping...")
    finally:
        config_manager.stop()
        print("Done.")


if __name__ == "__main__":
    main()


