#
# Copyright 2026 Jordi Gil.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

"""
Session-scoped LLM token accumulator for audit traces (#435).

Accumulates prompt and completion token counts across all LiteLLM
round-trips within a single investigation session. The accumulator
is scoped per-session via a ContextVar, which is set at the start
of each investigation and read when emitting audit events.

Architecture:
    endpoint.py sets ContextVar -> LiteLLM callback reads & accumulates
    -> endpoint.py reads totals -> audit event enriched

Business Requirements:
- DD-AUDIT-003: Service audit trace requirements
- BR-HAPI-301: LLM observability metrics
"""

from contextvars import ContextVar
from typing import Optional


class TokenAccumulator:
    """Accumulates LLM token usage across multiple calls within one session."""

    def __init__(self) -> None:
        self._prompt_tokens: int = 0
        self._completion_tokens: int = 0

    def add(self, prompt_tokens: int, completion_tokens: int) -> None:
        """Add token counts from a single LLM call."""
        self._prompt_tokens += prompt_tokens
        self._completion_tokens += completion_tokens

    @property
    def prompt_tokens(self) -> int:
        return self._prompt_tokens

    @property
    def completion_tokens(self) -> int:
        return self._completion_tokens

    def total(self) -> int:
        """Return combined prompt + completion token count."""
        return self._prompt_tokens + self._completion_tokens


_current_accumulator: ContextVar[Optional[TokenAccumulator]] = ContextVar(
    "token_accumulator", default=None
)


def set_token_accumulator(acc: Optional[TokenAccumulator]) -> None:
    """Set the session-scoped token accumulator in the current async context."""
    _current_accumulator.set(acc)


def get_token_accumulator() -> Optional[TokenAccumulator]:
    """Get the session-scoped token accumulator from the current async context."""
    return _current_accumulator.get()
