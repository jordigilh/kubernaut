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
Session Management Package (BR-AA-HAPI-064)

Provides in-memory session management for async submit/poll/result pattern.
Sessions are stored in-process and lost on HAPI restart (by design --
AA controller handles regeneration via BR-AA-HAPI-064.5).
"""

from .session_manager import SessionManager, SessionResultNotReady
from .endpoints import get_session_or_404, session_status_response, session_result_response

__all__ = [
    "SessionManager",
    "SessionResultNotReady",
    "get_session_or_404",
    "session_status_response",
    "session_result_response",
]
