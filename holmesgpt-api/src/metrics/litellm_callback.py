#
# Copyright 2025 Jordi Gil.
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
LiteLLM Metrics Callback for HAPI (BR-HAPI-301).

Hooks into LiteLLM's callback system to automatically record
per-call LLM metrics (calls, duration, token usage) via HAMetrics.

Architecture:
    litellm.completion() -> log_success_event() -> HAMetrics.record_llm_call()

Registration: src/main.py startup_event registers this callback once.

Reference: https://docs.litellm.ai/docs/observability/custom_callback
"""

import logging
from datetime import datetime
from typing import Any, Dict

from litellm.integrations.custom_logger import CustomLogger

from .instrumentation import HAMetrics

logger = logging.getLogger(__name__)

KNOWN_PROVIDERS = {
    "vertex_ai", "openai", "anthropic", "ollama", "azure",
    "bedrock", "together_ai", "huggingface", "cohere",
}

DEFAULT_PROVIDER_MAP = {
    "gpt-": "openai",
    "claude-": "anthropic",
    "llama": "openai",
    "mistral": "openai",
    "gemini": "vertex_ai",
}


def _extract_provider_model(model_string: str) -> tuple:
    """Split a LiteLLM model string into (provider, model).

    LiteLLM uses 'provider/model' format for routed calls (e.g.
    'vertex_ai/claude-sonnet-4-20250514'). Bare model names (e.g. 'gpt-4')
    are inferred from well-known prefixes.
    """
    if "/" in model_string:
        provider, model = model_string.split("/", 1)
        if provider in KNOWN_PROVIDERS:
            return provider, model

    for prefix, provider in DEFAULT_PROVIDER_MAP.items():
        if model_string.startswith(prefix):
            return provider, model_string

    return "anthropic", model_string


class KubernautLiteLLMCallback(CustomLogger):
    """LiteLLM callback that emits Prometheus metrics via HAMetrics.

    Business Requirement: BR-HAPI-301 -- LLM Observability Metrics
    """

    def __init__(self, metrics: HAMetrics):
        self._metrics = metrics
        super().__init__()

    def log_success_event(
        self,
        kwargs: Dict[str, Any],
        response_obj: Any,
        start_time: datetime,
        end_time: datetime,
    ) -> None:
        try:
            model_string = kwargs.get("model", "") or ""
            provider, model = _extract_provider_model(model_string)
            duration = (end_time - start_time).total_seconds()

            prompt_tokens = 0
            completion_tokens = 0
            usage = getattr(response_obj, "usage", None)
            if usage is not None:
                prompt_tokens = getattr(usage, "prompt_tokens", 0) or 0
                completion_tokens = getattr(usage, "completion_tokens", 0) or 0

            self._metrics.record_llm_call(
                provider=provider,
                model=model,
                status="success",
                duration=duration,
                prompt_tokens=prompt_tokens,
                completion_tokens=completion_tokens,
            )
        except Exception:
            logger.exception("KubernautLiteLLMCallback.log_success_event failed")

    def log_failure_event(
        self,
        kwargs: Dict[str, Any],
        response_obj: Any,
        start_time: datetime,
        end_time: datetime,
    ) -> None:
        try:
            model_string = kwargs.get("model", "") or ""
            provider, model = _extract_provider_model(model_string)
            duration = (end_time - start_time).total_seconds()

            self._metrics.record_llm_call(
                provider=provider,
                model=model,
                status="error",
                duration=duration,
                prompt_tokens=0,
                completion_tokens=0,
            )
        except Exception:
            logger.exception("KubernautLiteLLMCallback.log_failure_event failed")
