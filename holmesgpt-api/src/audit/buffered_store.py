"""
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
"""

"""
BufferedAuditStore - Async Buffered Audit Event Storage

Business Requirement: BR-AUDIT-005 (Workflow Selection Audit Trail)
Design Decisions:
  - ADR-038: Asynchronous Buffered Audit Trace Ingestion
  - DD-AUDIT-002: Audit Shared Library Design

This is the Python equivalent of pkg/audit/store.go (Go implementation).

Key Features (per ADR-038 + DD-AUDIT-002):
1. Non-blocking: store_audit() returns immediately
2. Buffered: In-memory queue absorbs bursts
3. Batched: Multiple events per HTTP call
4. Retry: Exponential backoff on failures
5. Graceful degradation: Drops events when buffer full
6. Flush on shutdown: close() flushes remaining events

Usage:
    store = BufferedAuditStore(
        data_storage_url="http://data-storage:8080",
        config=AuditConfig(buffer_size=10000, batch_size=50)
    )

    # Non-blocking: Returns immediately
    store.store_audit({"event_id": "...", "event_type": "..."})

    # Graceful shutdown: Flushes remaining events
    store.close()
"""

# Standard library imports
import logging  # noqa: E402
import queue  # noqa: E402
import sys  # noqa: E402
import threading  # noqa: E402
import time  # noqa: E402
from dataclasses import dataclass  # noqa: E402
from typing import Any, Dict, List, Optional  # noqa: E402

# Add OpenAPI client path
sys.path.insert(0, 'src/clients')

# Data Storage OpenAPI client (Phase 2b migration)
from datastorage import ApiClient, Configuration  # noqa: E402
from datastorage.api.audit_write_api_api import AuditWriteAPIApi  # noqa: E402
from datastorage.exceptions import ApiException  # noqa: E402
from datastorage.models.audit_event_request import AuditEventRequest  # noqa: E402
from datastorage.models.audit_event_request_event_data import AuditEventRequestEventData  # noqa: E402
from datastorage.models.audit_event_request import AuditEventRequest  # noqa: E402
from datastorage_auth_session import ServiceAccountAuthPoolManager  # noqa: E402

logger = logging.getLogger(__name__)


@dataclass
class AuditConfig:
    """
    Configuration for BufferedAuditStore

    Design Decision: DD-AUDIT-002 - Configuration

    Attributes:
        buffer_size: Maximum events to buffer in memory (default: 10000)
        batch_size: Events per HTTP call (default: 50)
        flush_interval_seconds: Seconds between partial batch flushes (default: 5.0)
        max_retries: Maximum retry attempts on failure (default: 3)
        http_timeout_seconds: HTTP request timeout (default: 10)
    """
    buffer_size: int = 10000
    batch_size: int = 50
    flush_interval_seconds: float = 5.0
    max_retries: int = 3
    http_timeout_seconds: int = 10


class BufferedAuditStore:
    """
    Async buffered audit event storage

    Business Requirement: BR-AUDIT-005 (Workflow Selection Audit Trail)
    Design Decisions:
      - ADR-038: Asynchronous Buffered Audit Trace Ingestion
      - DD-AUDIT-002: Audit Shared Library Design

    This class provides non-blocking audit event storage with:
    - In-memory buffer (queue.Queue)
    - Background worker thread for batch writes
    - Exponential backoff retry on failures
    - Graceful degradation (drops events when buffer full)

    Thread Safety:
    - store_audit() is thread-safe (uses queue.Queue)
    - close() is idempotent (uses threading.Event)
    """

    def __init__(
        self,
        data_storage_url: str,
        config: Optional[AuditConfig] = None
    ):
        """
        Initialize BufferedAuditStore

        Args:
            data_storage_url: Data Storage Service URL (e.g., "http://data-storage:8080")
            config: Configuration (uses defaults if not provided)
        """
        self._url = data_storage_url
        self._config = config or AuditConfig()

        # In-memory buffer (thread-safe queue)
        self._queue: queue.Queue = queue.Queue(maxsize=self._config.buffer_size)

        # Shutdown coordination
        self._shutdown = threading.Event()
        self._closed = threading.Event()

        # Flush coordination (for explicit flush() calls)
        self._flush_queue: queue.Queue = queue.Queue()

        # Metrics (thread-safe via atomic operations)
        self._buffered_count = 0
        self._dropped_count = 0
        self._written_count = 0
        self._failed_batch_count = 0
        self._lock = threading.Lock()

        # ========================================
        # DD-AUTH-005: Inject ServiceAccount authentication session
        # This change affects holmesgpt-api service automatically:
        # - holmesgpt-api reads ServiceAccount token from /var/run/secrets/kubernetes.io/serviceaccount/token
        # - Session caches token for 5 minutes (reduces filesystem I/O)
        # - Session injects Authorization: Bearer <token> header on every request
        # - Gracefully degrades if token file doesn't exist (local dev)
        #
        # See: docs/architecture/decisions/DD-AUTH-005-datastorage-client-authentication-pattern.md
        # ========================================

        # Initialize Data Storage OpenAPI client with auth session (Phase 2b + DD-AUTH-005)
        # Replaces manual requests.post() for type safety and contract validation
        auth_pool = ServiceAccountAuthPoolManager()
        api_config = Configuration(host=data_storage_url)
        self._api_client = ApiClient(configuration=api_config)
        self._api_client.rest_client.pool_manager = auth_pool  # ← ServiceAccount token injection
        self._audit_api = AuditWriteAPIApi(self._api_client)

        # Start background worker
        self._worker = threading.Thread(
            target=self._background_writer,
            name="audit-buffer-worker",
            daemon=True
        )
        self._worker.start()

        logger.info(
            f"📊 DD-AUDIT-002: BufferedAuditStore initialized with OpenAPI client - "
            f"url={data_storage_url}, "
            f"buffer_size={self._config.buffer_size}, "
            f"batch_size={self._config.batch_size}, "
            f"flush_interval={self._config.flush_interval_seconds}s"
        )

    def store_audit(self, event: AuditEventRequest) -> bool:
        """
        Add event to buffer (non-blocking)

        V3.0: OGEN MIGRATION - Accepts OpenAPI-generated Pydantic model instead of dict.

        ADR-038: "Fire-and-forget with local buffering"
        DD-AUDIT-002: "StoreAudit returns immediately, does not wait for write"

        Args:
            event: AuditEventRequest (OpenAPI-generated Pydantic model)

        Returns:
            True if event was buffered, False if dropped (buffer full)

        Note: This method NEVER blocks business logic. If buffer is full,
        the event is dropped and False is returned.
        """
        if self._shutdown.is_set():
            logger.warning("⚠️ DD-AUDIT-002: Audit store is shutting down, dropping event")
            return False

        try:
            # Non-blocking put (raises queue.Full if buffer is full)
            self._queue.put_nowait(event)

            with self._lock:
                self._buffered_count += 1

            return True

        except queue.Full:
            # Graceful degradation: Drop event, don't block
            with self._lock:
                self._dropped_count += 1

            logger.warning(
                f"⚠️ DD-AUDIT-002: Audit buffer full, dropping event - "
                f"event_type={event.get('event_type', 'unknown')}, "
                f"dropped_count={self._dropped_count}"
            )
            return False

    def flush(self, timeout: float = 5.0) -> bool:
        """
        Explicitly flush all buffered events without closing the store

        This method forces the background worker to immediately write all
        buffered events to Data Storage. Blocks until flush completes or timeout.

        Args:
            timeout: Maximum seconds to wait for flush (default: 5.0)

        Returns:
            True if flush completed successfully, False if timeout

        Note: This is useful for testing to ensure audit events are persisted
        before querying Data Storage.
        """
        if self._shutdown.is_set():
            logger.warning("⚠️ Audit store is shutting down, flush ignored")
            return False

        # Create completion event
        done_event = threading.Event()

        # Send flush request to background worker
        try:
            self._flush_queue.put_nowait(done_event)
        except queue.Full:
            logger.error("❌ Flush queue full, cannot request flush")
            return False

        # Wait for completion
        if done_event.wait(timeout=timeout):
            logger.debug(f"✅ Audit flush completed - queue_size={self._queue.qsize()}")
            return True
        else:
            logger.warning(f"⚠️ Audit flush timeout after {timeout}s")
            return False

    def close(self) -> None:
        """
        Flush remaining events and stop background worker

        DD-AUDIT-002: "Blocks until all buffered events are written"

        This method:
        1. Signals shutdown to background worker
        2. Waits for worker to flush remaining events
        3. Returns when all events are written (or timeout)

        Safe to call multiple times (idempotent).
        """
        # Idempotent: Check if already closed
        if self._closed.is_set():
            logger.debug("DD-AUDIT-002: Audit store already closed, skipping")
            return

        logger.info(
            f"📊 DD-AUDIT-002: Closing audit store, flushing remaining events - "
            f"queue_size={self._queue.qsize()}"
        )

        # Signal shutdown
        self._shutdown.set()

        # Wait for worker to finish (with timeout)
        self._worker.join(timeout=30.0)

        if self._worker.is_alive():
            logger.error("❌ DD-AUDIT-002: Timeout waiting for audit store to close")
        else:
            with self._lock:
                logger.info(
                    f"✅ DD-AUDIT-002: Audit store closed - "
                    f"buffered={self._buffered_count}, "
                    f"written={self._written_count}, "
                    f"dropped={self._dropped_count}, "
                    f"failed_batches={self._failed_batch_count}"
                )

        self._closed.set()

    def _background_writer(self) -> None:
        """
        Background worker: Batch and write events

        DD-AUDIT-002: "Background worker handles batching and writing"

        This worker:
        - Batches events for efficient writes (up to batch_size)
        - Flushes partial batches periodically (every flush_interval)
        - Handles explicit flush requests from flush() method
        - Retries failed writes with exponential backoff
        - Stops when shutdown is signaled
        """
        batch: List[AuditEventRequest] = []
        last_flush = time.time()

        while True:
            try:
                # Check for explicit flush requests (non-blocking)
                try:
                    done_event = self._flush_queue.get_nowait()

                    # Drain ALL remaining events from queue into batch before flushing
                    # This ensures flush() writes ALL buffered events, not just current batch
                    while not self._queue.empty():
                        try:
                            event = self._queue.get_nowait()
                            batch.append(event)
                        except queue.Empty:
                            break

                    # Flush complete batch (includes all queued events)
                    if batch:
                        self._write_batch_with_retry(batch)
                        batch = []
                        last_flush = time.time()
                    # Signal completion
                    done_event.set()
                except queue.Empty:
                    pass

                # Check for shutdown
                if self._shutdown.is_set() and self._queue.empty():
                    # Flush remaining batch before exit
                    if batch:
                        self._write_batch_with_retry(batch)
                    return

                # Try to get event with timeout
                try:
                    event = self._queue.get(timeout=0.1)
                    batch.append(event)
                except queue.Empty:
                    pass

                # Check if batch is full
                if len(batch) >= self._config.batch_size:
                    self._write_batch_with_retry(batch)
                    batch = []
                    last_flush = time.time()

                # Check if flush interval reached
                elif batch and (time.time() - last_flush) >= self._config.flush_interval_seconds:
                    self._write_batch_with_retry(batch)
                    batch = []
                    last_flush = time.time()

            except Exception as e:
                logger.error(f"💥 DD-AUDIT-002: Background writer error - {e}")

    def _write_batch_with_retry(self, batch: List[AuditEventRequest]) -> None:
        """
        Write batch by iterating through events and POSTing each individually

        ADR-034: Data Storage expects single events at POST /api/v1/audit/events
        ADR-038: Client-side batching with fire-and-forget pattern
        DD-AUDIT-002: "Retries failed writes with exponential backoff"

        Retry strategy (per event):
        - Attempt 1: Immediate
        - Attempt 2: 1 second delay
        - Attempt 3: 4 seconds delay
        - After max_retries: Drop event and continue
        """
        if not batch:
            return

        written = 0
        failed = 0

        for event in batch:
            if self._write_single_event_with_retry(event):
                written += 1
            else:
                failed += 1

        with self._lock:
            self._written_count += written
            if failed > 0:
                self._failed_batch_count += 1

        if written > 0:
            logger.debug(
                f"✅ DD-AUDIT-002: Wrote audit events - "
                f"written={written}, failed={failed}"
            )

        if failed > 0:
            logger.warning(
                f"⚠️ DD-AUDIT-002: Some events failed - "
                f"written={written}, failed={failed}"
            )

    def _write_single_event_with_retry(self, event: AuditEventRequest) -> bool:
        """
        Write single event with exponential backoff retry using OpenAPI client

        ADR-034: POST /api/v1/audit/events expects single event JSON
        Phase 2b: Uses typed AuditEventRequest model for type safety

        Args:
            event: ADR-034 compliant audit event

        Returns:
            True if written successfully, False if dropped
        """
        for attempt in range(1, self._config.max_retries + 1):
            try:
                # V3.0: OGEN MIGRATION - No conversion needed, already AuditEventRequest!
                # Event is already an OpenAPI-generated Pydantic model from events.py

                # Call OpenAPI endpoint (type-safe, contract-validated)
                response = self._audit_api.create_audit_event(
                    audit_event_request=event
                )

                logger.debug(
                    f"✅ Audit event written via OpenAPI - "
                    f"event_id={response.event_id}, "
                    f"event_type={event.event_type}, "
                    f"correlation_id={event.correlation_id}"
                )
                return True

            except ApiException as e:
                # OpenAPI client exception (HTTP errors, timeouts, etc.)
                event_type = event.get("event_type", "unknown")
                correlation_id = event.get("correlation_id", "")

                logger.warning(
                    f"⚠️ DD-AUDIT-002: OpenAPI audit write failed - "
                    f"attempt={attempt}/{self._config.max_retries}, "
                    f"event_type={event_type}, correlation_id={correlation_id}, "
                    f"status={e.status}, error={e.reason}, body={e.body}"
                )

                if attempt < self._config.max_retries:
                    # Exponential backoff: 1s, 4s, 9s
                    backoff = attempt * attempt
                    time.sleep(backoff)

            except Exception as e:
                # Validation errors (Pydantic), unexpected errors
                event_type = event.get("event_type", "unknown")
                correlation_id = event.get("correlation_id", "")

                logger.error(
                    f"❌ DD-AUDIT-002: Unexpected error in audit write - "
                    f"event_type={event_type}, correlation_id={correlation_id}, "
                    f"error_type={type(e).__name__}, error={e}"
                )
                # Don't retry on validation errors (they won't succeed)
                return False

        # Final failure: Drop event
        event_type = event.get("event_type", "unknown")
        correlation_id = event.get("correlation_id", "")

        logger.error(
            f"❌ DD-AUDIT-002: Dropping audit event after max retries - "
            f"event_type={event_type}, correlation_id={correlation_id}, "
            f"max_retries={self._config.max_retries}"
        )
        return False

