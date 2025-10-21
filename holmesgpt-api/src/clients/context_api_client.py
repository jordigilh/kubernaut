"""
Context API Client for HolmesGPT API

Provides historical context from Context API service for AI investigations.

Business Requirements: BR-HAPI-070 (Historical Context Integration)
"""

import logging
import os
import aiohttp
from typing import Dict, Any, List, Optional

logger = logging.getLogger(__name__)


class ContextAPIClient:
    """
    Client for Context API service integration

    Provides historical context for AI investigations:
    - Past remediation success rates
    - Similar incident patterns
    - Environment-specific context
    """

    def __init__(self, base_url: Optional[str] = None):
        """
        Initialize Context API client

        Args:
            base_url: Context API base URL (defaults to CONTEXT_API_URL env var)
        """
        self.base_url = base_url or os.getenv(
            "CONTEXT_API_URL",
            "http://context-api.kubernaut-system.svc.cluster.local:8091"
        )
        self.timeout = aiohttp.ClientTimeout(total=10)  # 10s timeout
        logger.info({
            "event": "context_api_client_initialized",
            "base_url": self.base_url
        })

    async def get_historical_context(
        self,
        namespace: str,
        target_type: str,
        target_name: str,
        signal_type: Optional[str] = None,
        time_range: str = "30d"
    ) -> Dict[str, Any]:
        """
        Get historical context for investigation

        Args:
            namespace: Kubernetes namespace
            target_type: Resource type (e.g., "deployment", "statefulset")
            target_name: Resource name
            signal_type: Signal type (e.g., "prometheus", "kubernetes-event")
            time_range: Time range for historical data (e.g., "7d", "30d", "90d")

        Returns:
            Dict containing historical context:
            - success_rates: Past remediation success rates
            - similar_incidents: Similar past incidents with similarity scores
            - environment_patterns: Environment-specific patterns
        """
        endpoint = f"{self.base_url}/api/v1/context/historical"

        params = {
            "namespace": namespace,
            "targetType": target_type,
            "targetName": target_name,
            "timeRange": time_range,
            "includeEmbeddings": "true"  # Include semantic search results
        }

        if signal_type:
            params["signalType"] = signal_type

        try:
            async with aiohttp.ClientSession(timeout=self.timeout) as session:
                async with session.get(endpoint, params=params) as response:
                    if response.status == 200:
                        data = await response.json()
                        logger.info({
                            "event": "context_api_success",
                            "namespace": namespace,
                            "target": f"{target_type}/{target_name}",
                            "similar_incidents": len(data.get("similar_incidents", []))
                        })
                        return data
                    elif response.status == 404:
                        logger.warning({
                            "event": "context_api_no_data",
                            "namespace": namespace,
                            "target": f"{target_type}/{target_name}"
                        })
                        return self._empty_context()
                    else:
                        logger.error({
                            "event": "context_api_error",
                            "status": response.status,
                            "namespace": namespace
                        })
                        return self._empty_context()

        except aiohttp.ClientError as e:
            logger.error({
                "event": "context_api_connection_error",
                "error": str(e),
                "base_url": self.base_url
            })
            return self._empty_context()
        except Exception as e:
            logger.error({
                "event": "context_api_unexpected_error",
                "error": str(e)
            })
            return self._empty_context()

    def _empty_context(self) -> Dict[str, Any]:
        """
        Return empty context when Context API is unavailable

        Allows graceful degradation - investigation proceeds without historical context
        """
        return {
            "success_rates": {},
            "similar_incidents": [],
            "environment_patterns": {},
            "available": False
        }

    async def health_check(self) -> bool:
        """
        Check if Context API is healthy

        Returns:
            True if Context API is healthy, False otherwise
        """
        try:
            async with aiohttp.ClientSession(timeout=self.timeout) as session:
                async with session.get(f"{self.base_url}/health") as response:
                    is_healthy = response.status == 200
                    logger.debug({
                        "event": "context_api_health_check",
                        "healthy": is_healthy
                    })
                    return is_healthy
        except Exception as e:
            logger.warning({
                "event": "context_api_health_check_failed",
                "error": str(e)
            })
            return False



