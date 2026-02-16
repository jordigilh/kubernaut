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
Shared Session Endpoint Helpers (BR-AA-HAPI-064)

Provides reusable FastAPI endpoint logic for session status polling and result retrieval.
Used by both incident and recovery routers to avoid code duplication.

REFACTOR: Extracted from incident/endpoint.py and recovery/endpoint.py where
the session status and result GET handlers were 100% identical.
"""

from typing import Any, Callable, Awaitable, Dict, Optional
from fastapi import HTTPException
from .session_manager import SessionManager, SessionResultNotReady, get_session_manager


def get_session_or_404(session_id: str) -> Dict[str, Any]:
    """
    Look up a session by ID, raising HTTP 404 if not found.

    BR-AA-HAPI-064.5: A 404 signals to the AA controller that the session
    was lost (e.g., HAPI restarted) and regeneration should be attempted.

    Args:
        session_id: UUID of the session

    Returns:
        Session dict

    Raises:
        HTTPException(404): If session not found
    """
    sm = get_session_manager()
    session = sm.get_session(session_id)
    if session is None:
        raise HTTPException(status_code=404, detail=f"Session {session_id} not found")
    return session


def session_status_response(session_id: str) -> dict:
    """
    Build the session status polling response.

    BR-AA-HAPI-064.2: Returns current status and creation timestamp.

    Args:
        session_id: UUID of the session

    Returns:
        dict with status and created_at
    """
    session = get_session_or_404(session_id)
    result = {
        "status": session["status"],
        "created_at": session.get("created_at", "").isoformat() if session.get("created_at") else None,
    }
    # Expose internal error message when session failed — critical for debugging
    # Without this, callers see "HAPI session failed: " with no detail
    if session.get("status") == "failed" and session.get("error"):
        result["error"] = session["error"]
    return result


def session_result_response(session_id: str) -> Any:
    """
    Retrieve the investigation result for a completed session.

    BR-AA-HAPI-064.3: Returns the full result dict.
    Returns HTTP 409 if the session is not yet completed.

    Args:
        session_id: UUID of the session

    Returns:
        The full investigation result (dict)

    Raises:
        HTTPException(404): If session not found
        HTTPException(409): If session not yet completed
    """
    session = get_session_or_404(session_id)
    sm = get_session_manager()
    try:
        return sm.get_result(session_id)
    except SessionResultNotReady:
        raise HTTPException(
            status_code=409,
            detail=f"Session {session_id} is not yet completed (status: {session['status']})"
        )
