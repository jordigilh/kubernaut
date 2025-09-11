"""
Context API Service - Integration with Kubernaut Context API
Implements dual-source integration - BR-HAPI-011, BR-HAPI-012, BR-HAPI-013
"""

import asyncio
from typing import Dict, List, Any, Optional

import httpx
import structlog
from tenacity import retry, stop_after_attempt, wait_exponential

from config import Settings

logger = structlog.get_logger(__name__)


class ContextAPIService:
    """
    Kubernaut Context API Integration Service

    Provides enrichment and intelligence overlay for HolmesGPT investigations:
    - Context validation and enrichment (BR-HAPI-013)
    - Parallel context gathering (BR-HAPI-015)
    - Context caching (BR-HAPI-014)
    - Integration with main Kubernaut app (BR-HAPI-011)
    """

    def __init__(self, settings: Settings):
        self.settings = settings
        self.base_url = settings.context_api_url
        self.timeout = settings.context_api_timeout
        self._client = None
        self._cache = {}  # Simple in-memory cache - BR-HAPI-014
        self._initialized = False

        logger.info("🔗 Context API service initialized",
                   base_url=self.base_url,
                   timeout=self.timeout)

    async def initialize(self) -> bool:
        """
        Initialize Context API service connection

        Returns:
            bool: True if initialization successful
        """
        try:
            logger.info("🚀 Initializing Context API service connection")

            # Create HTTP client with proper configuration
            self._client = httpx.AsyncClient(
                base_url=self.base_url,
                timeout=self.timeout,
                headers={
                    "User-Agent": "HolmesGPT-API/1.0.0",
                    "Content-Type": "application/json"
                }
            )

            # Test connection
            health_ok = await self.health_check()
            if not health_ok:
                logger.warning("⚠️ Context API health check failed during initialization")
                # Don't fail initialization - Context API is for enrichment only

            self._initialized = True
            logger.info("✅ Context API service initialized successfully")
            return True

        except Exception as e:
            logger.error("❌ Failed to initialize Context API service",
                        error=str(e), exc_info=True)
            return False

    async def cleanup(self):
        """Cleanup resources"""
        try:
            logger.info("🧹 Cleaning up Context API service")
            if self._client:
                await self._client.aclose()
            self._cache.clear()
            self._initialized = False
            logger.info("✅ Context API service cleanup completed")
        except Exception as e:
            logger.error("❌ Error during Context API cleanup",
                        error=str(e), exc_info=True)

    @retry(stop=stop_after_attempt(3), wait=wait_exponential(multiplier=1, min=2, max=8))
    async def health_check(self) -> bool:
        """
        Check Context API health with retry logic

        Returns:
            bool: True if Context API is healthy
        """
        try:
            if not self._client:
                return False

            # Call Context API health endpoint
            response = await self._client.get("/health", timeout=10)

            if response.status_code == 200:
                logger.debug("✅ Context API health check passed")
                return True
            else:
                logger.warning("⚠️ Context API health check failed",
                             status_code=response.status_code)
                return False

        except Exception as e:
            logger.error("❌ Context API health check failed",
                        error=str(e), exc_info=True)
            return False

    async def enrich_alert_context(
        self,
        alert_name: str,
        namespace: str,
        labels: Dict[str, str],
        annotations: Dict[str, str]
    ) -> Dict[str, Any]:
        """
        Enrich alert context with organizational intelligence - BR-HAPI-013

        Args:
            alert_name: Alert name
            namespace: Kubernetes namespace
            labels: Alert labels
            annotations: Alert annotations

        Returns:
            Dict containing enriched context data
        """
        logger.info("🔍 Enriching alert context",
                   alert_name=alert_name,
                   namespace=namespace)

        try:
            # Check cache first - BR-HAPI-014
            cache_key = f"alert:{alert_name}:{namespace}"
            if cache_key in self._cache:
                logger.debug("📋 Using cached context", cache_key=cache_key)
                return self._cache[cache_key]

            if not self._client:
                logger.warning("⚠️ Context API client not available, using basic context")
                return self._create_basic_context(alert_name, namespace, labels, annotations)

            # Parallel context gathering - BR-HAPI-015
            context_tasks = [
                self._get_namespace_context(namespace),
                self._get_alert_history(alert_name, namespace),
                self._get_resource_context(namespace),
                self._get_pattern_analysis(alert_name, labels)
            ]

            context_results = await asyncio.gather(*context_tasks, return_exceptions=True)

            # Combine all context data
            enriched_context = {
                "alert_name": alert_name,
                "namespace": namespace,
                "labels": labels,
                "annotations": annotations,
                "namespace_context": context_results[0] if not isinstance(context_results[0], Exception) else {},
                "alert_history": context_results[1] if not isinstance(context_results[1], Exception) else {},
                "resource_context": context_results[2] if not isinstance(context_results[2], Exception) else {},
                "pattern_analysis": context_results[3] if not isinstance(context_results[3], Exception) else {},
                "enrichment_timestamp": asyncio.get_event_loop().time()
            }

            # Cache the enriched context - BR-HAPI-014
            self._cache[cache_key] = enriched_context

            logger.info("✅ Alert context enriched successfully",
                       alert_name=alert_name,
                       context_keys=list(enriched_context.keys()))

            return enriched_context

        except Exception as e:
            logger.error("❌ Failed to enrich alert context",
                        alert_name=alert_name,
                        error=str(e), exc_info=True)

            # Fallback to basic context
            return self._create_basic_context(alert_name, namespace, labels, annotations)

    async def get_current_context(
        self,
        namespace: Optional[str] = None,
        include_metrics: bool = False
    ) -> Dict[str, Any]:
        """
        Get current cluster context for chat sessions

        Args:
            namespace: Optional namespace filter
            include_metrics: Include metrics data

        Returns:
            Dict containing current context
        """
        logger.info("📊 Getting current context",
                   namespace=namespace,
                   include_metrics=include_metrics)

        try:
            if not self._client:
                return {"error": "Context API not available"}

            # Build context request
            context_params = {}
            if namespace:
                context_params["namespace"] = namespace
            if include_metrics:
                context_params["include_metrics"] = "true"

            # Get current context from Context API
            response = await self._client.get("/api/v1/context", params=context_params)

            if response.status_code == 200:
                context_data = response.json()
                logger.info("✅ Current context retrieved",
                           namespace=namespace,
                           data_keys=list(context_data.keys()))
                return context_data
            else:
                logger.warning("⚠️ Failed to get current context",
                             status_code=response.status_code)
                return {"error": f"Context API returned {response.status_code}"}

        except Exception as e:
            logger.error("❌ Failed to get current context",
                        namespace=namespace,
                        error=str(e), exc_info=True)
            return {"error": str(e)}

    # Private helper methods

    async def _get_namespace_context(self, namespace: str) -> Dict[str, Any]:
        """Get namespace-specific context"""
        try:
            response = await self._client.get(f"/api/v1/namespace/{namespace}/context")
            if response.status_code == 200:
                return response.json()
            return {}
        except Exception as e:
            logger.debug("Failed to get namespace context",
                        namespace=namespace, error=str(e))
            return {}

    async def _get_alert_history(self, alert_name: str, namespace: str) -> Dict[str, Any]:
        """Get historical alert data"""
        try:
            params = {"alert": alert_name, "namespace": namespace}
            response = await self._client.get("/api/v1/alerts/history", params=params)
            if response.status_code == 200:
                return response.json()
            return {}
        except Exception as e:
            logger.debug("Failed to get alert history",
                        alert_name=alert_name, error=str(e))
            return {}

    async def _get_resource_context(self, namespace: str) -> Dict[str, Any]:
        """Get resource utilization context"""
        try:
            response = await self._client.get(f"/api/v1/namespace/{namespace}/resources")
            if response.status_code == 200:
                return response.json()
            return {}
        except Exception as e:
            logger.debug("Failed to get resource context",
                        namespace=namespace, error=str(e))
            return {}

    async def _get_pattern_analysis(self, alert_name: str, labels: Dict[str, str]) -> Dict[str, Any]:
        """Get pattern analysis for similar alerts"""
        try:
            payload = {"alert_name": alert_name, "labels": labels}
            response = await self._client.post("/api/v1/patterns/analyze", json=payload)
            if response.status_code == 200:
                return response.json()
            return {}
        except Exception as e:
            logger.debug("Failed to get pattern analysis",
                        alert_name=alert_name, error=str(e))
            return {}

    def _create_basic_context(
        self,
        alert_name: str,
        namespace: str,
        labels: Dict[str, str],
        annotations: Dict[str, str]
    ) -> Dict[str, Any]:
        """Create basic context when Context API is unavailable"""
        return {
            "alert_name": alert_name,
            "namespace": namespace,
            "labels": labels,
            "annotations": annotations,
            "enrichment_status": "basic_only",
            "context_api_available": False
        }
