"""
HolmesGPT service layer.

This module provides a unified interface to HolmesGPT using direct
Python API integration for maximum reliability and fail-fast behavior.
"""

import asyncio
import json
import logging
import os
import time
from datetime import datetime, timezone
from typing import Dict, List, Optional, Any, Union

import aiohttp
import psutil

from app.config import Settings
from app.models.requests import HolmesOptions, ContextData, AlertData, InvestigationContext
from app.models.responses import (
    AskResponse, InvestigateResponse, HealthCheckResponse,
    Recommendation, AnalysisResult, InvestigationResult, HealthStatus
)
# Cache functionality removed - direct processing only
from app.utils.metrics import track_operation
from app.services.holmesgpt_wrapper import HolmesGPTWrapper
from app.services.kubernetes_context_provider import KubernetesContextProvider
from app.services.action_history_context_provider import ActionHistoryContextProvider


class HolmesGPTService:
    """Service for interacting with HolmesGPT via Python API."""

    def __init__(self, settings: Settings):
        self.settings = settings
        self.logger = logging.getLogger(__name__)
        # Cache functionality removed - direct processing only

        # HolmesGPT integration state
        self._holmes_wrapper = None
        self._api_available = False
        self._initialization_time = None
        self._session_aiohttp = None

        # Context providers (replaces deprecated MCP Bridge functionality)
        self._k8s_context_provider = KubernetesContextProvider(settings)
        self._action_history_provider = ActionHistoryContextProvider(settings)

        # Performance tracking
        self._operation_count = 0
        self._total_processing_time = 0.0
        self._last_health_check = None

    async def initialize(self) -> None:
        """Initialize the HolmesGPT service."""
        start_time = time.time()
        self.logger.info("Initializing HolmesGPT service...")

        try:
            # Create aiohttp session
            timeout = aiohttp.ClientTimeout(total=self.settings.request_timeout)
            self._session_aiohttp = aiohttp.ClientSession(timeout=timeout)

            # Initialize HolmesGPT Python API
            await self._initialize_api()

            # Validate API is available
            if not self._api_available:
                raise RuntimeError("HolmesGPT Python API is not available")

            self._initialization_time = time.time() - start_time
            self.logger.info(
                f"HolmesGPT service initialized in {self._initialization_time:.2f}s "
                f"(api_available={self._api_available})"
            )

        except Exception as e:
            self.logger.error(f"Failed to initialize HolmesGPT service: {e}")
            raise

    async def _initialize_api(self) -> None:
        """Initialize HolmesGPT Python API."""
        try:
            self.logger.debug("Initializing HolmesGPT Python API...")

            # Initialize HolmesGPT wrapper
            self._holmes_wrapper = HolmesGPTWrapper(self.settings)
            self._api_available = await self._holmes_wrapper.initialize()

            if self._api_available:
                self.logger.info("HolmesGPT Python API initialized successfully")
            else:
                raise RuntimeError("HolmesGPT Python API initialization failed")

        except ImportError as e:
            self.logger.error(f"HolmesGPT Python library not available: {e}")
            raise RuntimeError(f"HolmesGPT import failed: {e}")
        except Exception as e:
            self.logger.error(f"Error initializing HolmesGPT Python API: {e}")
            raise

    @track_operation("ask")
    async def ask(self, prompt: str, context: Optional[ContextData] = None, options: Optional[HolmesOptions] = None) -> AskResponse:
        """Ask HolmesGPT a question."""
        start_time = time.time()

        # Cache functionality removed - direct processing only

        try:
            self.logger.info(f"Processing ask request (prompt length: {len(prompt)})")

            # Prepare options with defaults
            merged_options = self._merge_options(options)

            # Use HolmesGPT Python API
            if self._api_available and self._holmes_wrapper:
                result = await self._holmes_wrapper.ask(prompt, context, options)
            else:
                raise RuntimeError("HolmesGPT API not available")

            # Track performance
            processing_time = time.time() - start_time
            self._operation_count += 1
            self._total_processing_time += processing_time

            # Cache functionality removed - direct processing only

            self.logger.info(f"Ask request completed in {processing_time:.2f}s")
            return result

        except Exception as e:
            processing_time = time.time() - start_time
            self.logger.error(f"Ask request failed after {processing_time:.2f}s: {e}")
            raise

    async def _ask_direct(self, prompt: str, context: Optional[ContextData], options: Dict[str, Any]) -> AskResponse:
        """Ask using direct Python import."""
        try:
            # This method is now handled by the HolmesGPTWrapper
            # Kept for interface compatibility but delegates to wrapper
            self.logger.debug("Using direct HolmesGPT API for ask")

            if self._holmes_wrapper:
                return await self._holmes_wrapper.ask(prompt, context, HolmesOptions(**options))
            else:
                raise RuntimeError("HolmesGPT wrapper not available")

        except Exception as e:
            self.logger.error(f"Direct ask failed: {e}")
            raise

    @track_operation("investigate")
    async def investigate(
        self,
        alert: AlertData,
        context: Optional[ContextData] = None,
        options: Optional[HolmesOptions] = None,
        investigation_context: Optional[InvestigationContext] = None
    ) -> InvestigateResponse:
        """Investigate an alert."""
        start_time = time.time()

        # Cache functionality removed - direct processing only

        try:
            self.logger.info(f"Processing investigation for alert: {alert.name}")

            # Enhance context with Kubernetes and action history data (replaces MCP Bridge)
            enhanced_context = await self._enrich_investigation_context(alert, context)

            # Prepare options with defaults
            merged_options = self._merge_options(options)

            # Use HolmesGPT Python API with enhanced context
            if self._api_available and self._holmes_wrapper:
                result = await self._holmes_wrapper.investigate(alert, enhanced_context, options, investigation_context)
            else:
                raise RuntimeError("HolmesGPT API not available")

            # Track performance
            processing_time = time.time() - start_time
            self._operation_count += 1
            self._total_processing_time += processing_time

            # Cache functionality removed - direct processing only

            self.logger.info(f"Investigation completed in {processing_time:.2f}s")
            return result

        except Exception as e:
            processing_time = time.time() - start_time
            self.logger.error(f"Investigation failed after {processing_time:.2f}s: {e}")
            raise

    async def _investigate_direct(
        self,
        alert: AlertData,
        context: Optional[ContextData],
        options: Dict[str, Any],
        investigation_context: Optional[InvestigationContext]
    ) -> InvestigateResponse:
        """Investigate using direct Python import."""
        try:
            # This method is now handled by the HolmesGPTWrapper
            # Kept for interface compatibility but delegates to wrapper
            self.logger.debug("Using direct HolmesGPT API for investigation")

            if self._holmes_wrapper:
                return await self._holmes_wrapper.investigate(
                    alert, context, HolmesOptions(**options), investigation_context
                )
            else:
                raise RuntimeError("HolmesGPT wrapper not available")

        except Exception as e:
            self.logger.error(f"Direct investigation failed: {e}")
            raise

    async def health_check(self) -> HealthCheckResponse:
        """Perform comprehensive health check."""
        checks = {}
        overall_healthy = True
        messages = []

        try:
            # Check HolmesGPT API availability
            if self._api_available and self._holmes_wrapper:
                # Test HolmesGPT wrapper
                wrapper_health = await self._holmes_wrapper.health_check()
                checks["holmesgpt"] = wrapper_health
                if wrapper_health.status != "healthy":
                    overall_healthy = False
                    messages.append(f"HolmesGPT API: {wrapper_health.message}")
            else:
                overall_healthy = False
                checks["holmesgpt"] = HealthStatus(
                    component="holmesgpt_api",
                    status="unavailable",
                    message="HolmesGPT Python API not available",
                    last_check=datetime.now()
                )
                messages.append("HolmesGPT API not available")

            # Check Ollama if configured
            if self.settings.ollama_url:
                try:
                    test_start = time.time()
                    async with self._session_aiohttp.get(f"{self.settings.ollama_url}/api/tags") as response:
                        if response.status == 200:
                            response_time = time.time() - test_start
                            checks["ollama"] = HealthStatus(
                                component="ollama",
                                status="healthy",
                                message="Ollama API is available",
                                last_check=datetime.now(),
                                response_time=response_time
                            )
                        else:
                            overall_healthy = False
                            checks["ollama"] = HealthStatus(
                                component="ollama",
                                status="unhealthy",
                                message=f"Ollama API returned status {response.status}",
                                last_check=datetime.now()
                            )
                            messages.append(f"Ollama API unhealthy: status {response.status}")
                except Exception as e:
                    overall_healthy = False
                    checks["ollama"] = HealthStatus(
                        component="ollama",
                        status="unhealthy",
                        message=f"Ollama API error: {str(e)}",
                        last_check=datetime.now()
                    )
                    messages.append(f"Ollama API unhealthy: {str(e)}")

            # Cache functionality removed - no cache checks needed

            # Check system resources
            try:
                memory_info = psutil.virtual_memory()
                cpu_percent = psutil.cpu_percent(interval=1)

                memory_healthy = memory_info.percent < 90
                cpu_healthy = cpu_percent < 95

                if memory_healthy and cpu_healthy:
                    checks["system"] = HealthStatus(
                        component="system",
                        status="healthy",
                        message=f"Memory: {memory_info.percent:.1f}%, CPU: {cpu_percent:.1f}%",
                        last_check=datetime.now()
                    )
                else:
                    overall_healthy = False
                    checks["system"] = HealthStatus(
                        component="system",
                        status="unhealthy",
                        message=f"High resource usage - Memory: {memory_info.percent:.1f}%, CPU: {cpu_percent:.1f}%",
                        last_check=datetime.now()
                    )
                    messages.append(f"High resource usage: Memory {memory_info.percent:.1f}%, CPU {cpu_percent:.1f}%")
            except Exception as e:
                self.logger.warning(f"Could not check system resources: {e}")

            self._last_health_check = datetime.now()

            return HealthCheckResponse(
                healthy=overall_healthy,
                status="healthy" if overall_healthy else "unhealthy",
                message="All systems operational" if overall_healthy else "; ".join(messages),
                timestamp=self._last_health_check.timestamp(),
                checks=checks,
                system_info={
                    "operations_count": self._operation_count,
                    "average_processing_time": (
                        self._total_processing_time / self._operation_count
                        if self._operation_count > 0 else 0.0
                    ),
                    "initialization_time": self._initialization_time,
                    "api_available": self._api_available
                }
            )

        except Exception as e:
            self.logger.error(f"Health check failed: {e}")
            return HealthCheckResponse(
                healthy=False,
                status="error",
                message=f"Health check error: {str(e)}",
                timestamp=datetime.now().timestamp(),
                checks={}
            )

    async def get_service_info(self) -> Dict[str, Any]:
        """Get detailed service information."""
        return {
            "service": "HolmesGPT Python API Service",
            "version": "1.0.0",
            "api_available": self._api_available,
            "initialization_time": self._initialization_time,
            "operations_count": self._operation_count,
            "total_processing_time": self._total_processing_time,
            "average_processing_time": (
                self._total_processing_time / self._operation_count
                if self._operation_count > 0 else 0.0
            ),
            "last_health_check": self._last_health_check.isoformat() if self._last_health_check else None,
            "settings": {
                # Cache functionality removed
                "request_timeout": self.settings.request_timeout,
                "holmes_llm_provider": self.settings.holmes_llm_provider,
                "holmes_default_model": self.settings.holmes_default_model,
            }
        }

    async def _enrich_investigation_context(self, alert: AlertData, base_context: Optional[ContextData]) -> ContextData:
        """
        Enrich investigation context with Kubernetes and action history data.

        This replaces the deprecated MCP Bridge by directly providing context
        to HolmesGPT instead of using multi-turn conversations with LocalAI.
        """
        try:
            # Start with base context or create new
            if base_context:
                enriched_context = base_context.model_dump() if hasattr(base_context, 'model_dump') else base_context.__dict__.copy()
            else:
                enriched_context = {}

            # Gather context from both providers in parallel
            k8s_task = self._k8s_context_provider.get_investigation_context(alert)
            history_task = self._action_history_provider.get_action_history_context(alert)

            k8s_context, history_context = await asyncio.gather(k8s_task, history_task, return_exceptions=True)

            # Merge Kubernetes context
            if isinstance(k8s_context, dict) and "error" not in k8s_context:
                enriched_context.update(k8s_context)
                self.logger.debug("Added Kubernetes context to investigation")
            elif isinstance(k8s_context, Exception):
                self.logger.warning(f"Failed to get Kubernetes context: {k8s_context}")

            # Merge action history context
            if isinstance(history_context, dict) and "error" not in history_context:
                enriched_context.update(history_context)
                self.logger.debug("Added action history context to investigation")
            elif isinstance(history_context, Exception):
                self.logger.warning(f"Failed to get action history context: {history_context}")

            # Add enrichment metadata
            enriched_context["context_enrichment"] = {
                "enriched_at": datetime.now(timezone.utc).isoformat(),
                "sources": ["kubernetes_context_provider", "action_history_context_provider"],
                "replaces": "deprecated_mcp_bridge"
            }

            # Convert back to ContextData if needed
            if hasattr(base_context, '__class__'):
                try:
                    return base_context.__class__(**enriched_context)
                except:
                    # Fallback to dict if conversion fails
                    pass

            return enriched_context

        except Exception as e:
            self.logger.error(f"Failed to enrich investigation context: {e}")
            # Return original context on failure
            return base_context or {}

    def _merge_options(self, options: Optional[HolmesOptions]) -> Dict[str, Any]:
        """Merge user options with defaults."""
        defaults = {
            "max_tokens": self.settings.holmes_default_max_tokens,
            "temperature": self.settings.holmes_default_temperature,
            "timeout": self.settings.holmes_default_timeout,
            "model": self.settings.holmes_default_model,
            "debug": False
        }

        if options:
            # Convert Pydantic model to dict and merge
            user_options = options.dict(exclude_unset=True)
            defaults.update(user_options)

        return defaults

    async def reload(self) -> None:
        """Reload service configuration."""
        self.logger.info("Reloading HolmesGPT service...")
        await self.cleanup()
        await self.initialize()

    async def cleanup(self) -> None:
        """Clean up resources."""
        self.logger.info("Cleaning up HolmesGPT service...")

        if self._holmes_wrapper:
            await self._holmes_wrapper.cleanup()
            self._holmes_wrapper = None

        self._api_available = False

        if self._session_aiohttp:
            await self._session_aiohttp.close()
            self._session_aiohttp = None

        self.logger.info("HolmesGPT service cleanup completed")