"""
Copyright 2026 Jordi Gil.

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
Session Manager for Async Investigation Pattern (BR-AA-HAPI-064)

Manages in-memory investigation sessions for the submit/poll/result pattern.
Sessions are ephemeral -- they are lost on HAPI restart. The AA controller
handles this via session regeneration (BR-AA-HAPI-064.5).

Business Requirements:
- BR-AA-HAPI-064.1: Submit returns session ID immediately
- BR-AA-HAPI-064.2: Poll returns session status
- BR-AA-HAPI-064.3: Result retrieval after completion
- BR-AA-HAPI-064.8: TTL cleanup for memory management
"""

import uuid
import logging
from datetime import datetime, timezone
from typing import Optional, Dict, Any, Callable, Awaitable

logger = logging.getLogger(__name__)


class SessionResultNotReady(Exception):
    """Raised when get_result() is called on a session that is not yet completed."""

    def __init__(self, session_id: str, current_status: str):
        self.session_id = session_id
        self.current_status = current_status
        super().__init__(
            f"Session {session_id} is not completed (current status: {current_status})"
        )


class SessionManager:
    """
    In-memory session manager for async investigation sessions.

    BR-AA-HAPI-064: Manages the lifecycle of investigation sessions:
    - create_session: Accept request, return UUID, store as "pending"
    - get_session: Return session status for polling
    - get_result: Return investigation result when completed
    - cleanup_expired: Remove old sessions to prevent memory leaks
    - run_investigation: Execute investigation in background, updating session status

    Sessions are stored in a dict keyed by session_id (UUID string).
    Each session contains: status, created_at, completed_at, request_type,
    request_data, result, error.
    """

    def __init__(self):
        """Initialize the session manager with an empty session store."""
        self._sessions: Dict[str, Dict[str, Any]] = {}

    def create_session(self, request_type: str, request_data: dict) -> str:
        """
        Create a new investigation session.

        BR-AA-HAPI-064.1: Returns UUID immediately, stores session as "pending".

        Args:
            request_type: "incident" or "recovery"
            request_data: The original request payload

        Returns:
            str: UUID session ID
        """
        session_id = str(uuid.uuid4())
        self._sessions[session_id] = {
            "status": "pending",
            "request_type": request_type,
            "request_data": request_data,
            "result": None,
            "error": None,
            "created_at": datetime.now(timezone.utc),
            "completed_at": None,
        }
        logger.info(
            "Session created",
            extra={
                "session_id": session_id,
                "request_type": request_type,
                "status": "pending",
            },
        )
        return session_id

    def get_session(self, session_id: str) -> Optional[Dict[str, Any]]:
        """
        Get session status for polling.

        BR-AA-HAPI-064.2: Returns session dict or None if not found.

        Args:
            session_id: UUID of the session

        Returns:
            Optional dict with at least: {"status": "pending"|"investigating"|"completed"|"failed"}
            None if session_id is not found (maps to HTTP 404)
        """
        return self._sessions.get(session_id)

    def get_result(self, session_id: str) -> Dict[str, Any]:
        """
        Get the investigation result for a completed session.

        BR-AA-HAPI-064.3: Returns the full investigation result.

        Args:
            session_id: UUID of the session

        Returns:
            dict: The full IncidentResponse or RecoveryResponse

        Raises:
            SessionResultNotReady: If session is not in "completed" status
            KeyError: If session_id is not found
        """
        session = self._sessions.get(session_id)
        if session is None:
            raise KeyError(f"Session {session_id} not found")

        if session["status"] != "completed":
            raise SessionResultNotReady(session_id, session["status"])

        return session["result"]

    def cleanup_expired(self, ttl_minutes: int = 30) -> int:
        """
        Remove expired sessions (completed or failed older than TTL).

        BR-AA-HAPI-064.8: Prevents unbounded memory growth.
        Active (pending/investigating) sessions are never cleaned up.

        Args:
            ttl_minutes: Time-to-live for completed/failed sessions

        Returns:
            int: Number of sessions removed
        """
        now = datetime.now(timezone.utc)
        to_remove = []

        for session_id, session in self._sessions.items():
            # Only clean up terminal states (completed or failed)
            if session["status"] not in ("completed", "failed"):
                continue

            # Use completed_at if available, otherwise created_at
            reference_time = session.get("completed_at") or session.get("created_at")
            if reference_time is None:
                continue

            elapsed = (now - reference_time).total_seconds()
            if elapsed > ttl_minutes * 60:
                to_remove.append(session_id)

        for session_id in to_remove:
            del self._sessions[session_id]
            logger.info(
                "Session expired and removed",
                extra={"session_id": session_id, "ttl_minutes": ttl_minutes},
            )

        return len(to_remove)

    async def run_investigation(
        self,
        session_id: str,
        investigate_fn: Callable[..., Awaitable[Dict[str, Any]]],
        *args,
        **kwargs,
    ) -> None:
        """
        Run an investigation, updating session status through the lifecycle.

        BR-AA-HAPI-064.1: Transitions: pending -> investigating -> completed/failed
        Captures exceptions and stores them in the session as "failed" status.

        Args:
            session_id: UUID of the session to update
            investigate_fn: Async function to run (e.g., analyze_incident)
            *args, **kwargs: Arguments to pass to investigate_fn
        """
        session = self._sessions.get(session_id)
        if session is None:
            logger.error(
                "Session not found for investigation",
                extra={"session_id": session_id},
            )
            return

        # Transition: pending -> investigating
        session["status"] = "investigating"
        logger.info(
            "Session investigation started",
            extra={"session_id": session_id, "status": "investigating"},
        )

        try:
            result = await investigate_fn(*args, **kwargs)

            # Transition: investigating -> completed
            session["status"] = "completed"
            session["result"] = result
            session["completed_at"] = datetime.now(timezone.utc)
            logger.info(
                "Session investigation completed",
                extra={"session_id": session_id, "status": "completed"},
            )

        except Exception as exc:
            # Transition: investigating -> failed
            session["status"] = "failed"
            session["error"] = str(exc)
            session["completed_at"] = datetime.now(timezone.utc)
            logger.error(
                "Session investigation failed",
                extra={
                    "session_id": session_id,
                    "status": "failed",
                    "error": str(exc),
                },
            )


# ========================================
# Module-level singleton for endpoint access
# ========================================

_global_session_manager: Optional[SessionManager] = None


def get_session_manager() -> SessionManager:
    """Get or create the global SessionManager singleton."""
    global _global_session_manager
    if _global_session_manager is None:
        _global_session_manager = SessionManager()
    return _global_session_manager


def reset_session_manager() -> None:
    """Reset the global SessionManager (for testing)."""
    global _global_session_manager
    _global_session_manager = None
