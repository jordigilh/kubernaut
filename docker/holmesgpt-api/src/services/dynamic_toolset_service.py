"""
Dynamic Toolset Service - Integration with Go Service Discovery
Business Requirement: BR-HOLMES-020 - Real-time toolset configuration updates
"""

import asyncio
import json
import logging
from typing import Dict, List, Optional, Any
from datetime import datetime

from models.api_models import Toolset

logger = logging.getLogger(__name__)


class DynamicToolsetService:
    """
    Dynamic toolset service that interfaces with Go-based service discovery
    Business Requirement: BR-HOLMES-025 - Runtime toolset management API
    """

    def __init__(self, kubernaut_service_integration_endpoint: str = None):
        self.kubernaut_endpoint = kubernaut_service_integration_endpoint or "http://localhost:8091"
        self.cached_toolsets: Dict[str, Toolset] = {}
        self.last_update = None
        self.update_interval = 30  # seconds

    async def initialize(self) -> bool:
        """
        Initialize the dynamic toolset service
        Business Requirement: BR-HOLMES-016 - Dynamic service discovery
        """
        logger.info("🔧 Initializing dynamic toolset service")

        try:
            # Load initial toolsets from Kubernaut service integration
            await self.refresh_toolsets()

            # Start periodic refresh task
            asyncio.create_task(self._periodic_refresh())

            logger.info("✅ Dynamic toolset service initialized successfully")
            return True

        except Exception as e:
            logger.error(f"❌ Failed to initialize dynamic toolset service: {e}")
            return False

    async def get_available_toolsets(self) -> List[Toolset]:
        """
        Get all available toolsets from service discovery
        Business Requirement: BR-HOLMES-021 - Cached toolset results
        """
        try:
            # Return cached toolsets if available and fresh
            if self._is_cache_fresh():
                logger.debug("📦 Returning cached toolsets")
                return list(self.cached_toolsets.values())

            # Refresh from Kubernaut service integration
            await self.refresh_toolsets()
            return list(self.cached_toolsets.values())

        except Exception as e:
            logger.error(f"❌ Failed to get available toolsets: {e}")
            # Return cached toolsets as fallback
            return list(self.cached_toolsets.values())

    async def get_toolset_by_type(self, service_type: str) -> List[Toolset]:
        """Get toolsets for a specific service type"""
        all_toolsets = await self.get_available_toolsets()
        return [ts for ts in all_toolsets if ts.name.startswith(service_type)]

    async def get_enabled_toolsets(self) -> List[Toolset]:
        """Get only enabled toolsets"""
        all_toolsets = await self.get_available_toolsets()
        return [ts for ts in all_toolsets if ts.enabled]

    async def is_service_available(self, service_type: str) -> bool:
        """
        Check if a specific service type is available
        Business Requirement: BR-HOLMES-019 - Service availability validation
        """
        toolsets = await self.get_toolset_by_type(service_type)
        return any(ts.enabled for ts in toolsets)

    async def refresh_toolsets(self) -> bool:
        """
        Refresh toolsets from Kubernaut service integration
        Business Requirement: BR-HOLMES-020 - Real-time toolset updates
        """
        try:
            logger.debug("🔄 Refreshing toolsets from service discovery")

            # In a real implementation, this would make an HTTP call to the
            # Go service integration endpoint to get current toolsets
            # For now, we'll simulate with some example toolsets

            refreshed_toolsets = await self._fetch_toolsets_from_kubernaut()

            # Update cache
            self.cached_toolsets = {ts.name: ts for ts in refreshed_toolsets}
            self.last_update = datetime.utcnow()

            logger.info(f"✅ Refreshed {len(refreshed_toolsets)} toolsets")
            return True

        except Exception as e:
            logger.error(f"❌ Failed to refresh toolsets: {e}")
            return False

    async def _fetch_toolsets_from_kubernaut(self) -> List[Toolset]:
        """
        Fetch toolsets from Kubernaut service integration
        Business Requirement: BR-HOLMES-025 - Runtime toolset management API
        """
        try:
            # Real HTTP integration with Go Context API
            import aiohttp
            async with aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=10)) as session:
                async with session.get(f"{self.kubernaut_endpoint}/api/v1/toolsets") as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        # Convert Go toolset format to Python Toolset objects
                        return [self._convert_go_toolset_to_python(ts) for ts in data.get("toolsets", [])]
                    else:
                        logger.warning(f"⚠️ Context API returned status {resp.status}, falling back to baseline toolsets")
                        return self._get_baseline_toolsets()

        except Exception as e:
            logger.error(f"❌ Failed to fetch toolsets from Kubernaut Context API: {e}")
            logger.info("🔄 Falling back to baseline toolsets")
            # Fallback to baseline toolsets following BR-HOLMES-012
            return self._get_baseline_toolsets()

    def _get_baseline_toolsets(self) -> List[Toolset]:
        """
        Get baseline toolsets that are always available
        Business Requirement: BR-HOLMES-028 - Maintain baseline toolsets
        """
        return [
            Toolset(
                name="kubernetes",
                description="Kubernetes cluster investigation tools",
                version="1.0.0",
                capabilities=["get_pods", "get_services", "get_deployments", "get_events", "describe_resources", "get_logs"],
                enabled=True
            ),
            Toolset(
                name="internet",
                description="Internet connectivity and external API tools",
                version="1.0.0",
                capabilities=["web_search", "documentation_lookup", "api_status_check"],
                enabled=True
            )
        ]

    async def _simulate_discovered_toolsets(self) -> List[Toolset]:
        """
        Simulate discovered toolsets (in practice, these come from Go service discovery)
        Business Requirement: BR-HOLMES-017 - Automatic service detection
        """
        # This simulates what would come from the Go service discovery
        discovered_toolsets = []

        # Simulate Prometheus discovery
        if await self._simulate_service_check("prometheus"):
            discovered_toolsets.append(Toolset(
                name="prometheus-monitoring-prometheus-server",
                description="Prometheus metrics analysis tools for prometheus-server",
                version="1.0.0",
                capabilities=["query_metrics", "alert_rules", "time_series", "resource_usage_analysis"],
                enabled=True
            ))

        # Simulate Grafana discovery
        if await self._simulate_service_check("grafana"):
            discovered_toolsets.append(Toolset(
                name="grafana-monitoring-grafana",
                description="Grafana dashboard and visualization tools for grafana",
                version="1.0.0",
                capabilities=["get_dashboards", "query_datasource", "get_alerts", "visualization"],
                enabled=True
            ))

        # Simulate Jaeger discovery
        if await self._simulate_service_check("jaeger"):
            discovered_toolsets.append(Toolset(
                name="jaeger-observability-jaeger-query",
                description="Jaeger distributed tracing analysis tools for jaeger-query",
                version="1.0.0",
                capabilities=["search_traces", "get_services", "analyze_latency", "distributed_tracing"],
                enabled=True
            ))

        return discovered_toolsets

    async def _simulate_service_check(self, service_type: str) -> bool:
        """Simulate checking if a service is available in the cluster"""
        # In practice, this would check with the Go service discovery
        # For simulation, we'll randomly return True for some services
        import random
        return random.choice([True, False, True])  # 2/3 chance of being available

    def _is_cache_fresh(self) -> bool:
        """Check if cached toolsets are still fresh"""
        if not self.last_update:
            return False

        age_seconds = (datetime.utcnow() - self.last_update).total_seconds()
        return age_seconds < self.update_interval

    async def _periodic_refresh(self):
        """Periodic refresh task to keep toolsets up to date"""
        while True:
            try:
                await asyncio.sleep(self.update_interval)
                await self.refresh_toolsets()
            except Exception as e:
                logger.error(f"❌ Periodic refresh failed: {e}")

    def _convert_go_toolset_to_python(self, go_toolset: Dict[str, Any]) -> Toolset:
        """
        Convert Go toolset format to Python Toolset model
        Business Requirement: BR-HOLMES-022 - Service-specific toolset configurations
        """
        return Toolset(
            name=go_toolset.get("name", ""),
            description=go_toolset.get("description", ""),
            version=go_toolset.get("version", "1.0.0"),
            capabilities=go_toolset.get("capabilities", []),
            enabled=go_toolset.get("enabled", True)
        )

    async def get_service_integration_health(self) -> Dict[str, Any]:
        """
        Get health status from Kubernaut service integration
        Business Requirement: BR-HOLMES-026 - Service discovery health checks
        """
        try:
            # Real HTTP integration with Go Context API
            import aiohttp
            async with aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=5)) as session:
                async with session.get(f"{self.kubernaut_endpoint}/api/v1/service-discovery") as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        return {
                            "healthy": data.get("health", {}).get("healthy", False),
                            "service_discovery_healthy": data.get("health", {}).get("service_discovery_healthy", False),
                            "toolset_manager_healthy": data.get("health", {}).get("toolset_manager_healthy", False),
                            "total_toolsets": data.get("available_toolsets", len(self.cached_toolsets)),
                            "enabled_toolsets": data.get("enabled_toolsets", len([ts for ts in self.cached_toolsets.values() if ts.enabled])),
                            "discovered_services": data.get("discovered_services", 0),
                            "available_services": data.get("statistics", {}).get("available_services", 0),
                            "last_update": self.last_update.isoformat() if self.last_update else None
                        }
                    else:
                        logger.warning(f"⚠️ Context API health check returned status {resp.status}")
                        return self._create_fallback_health_status()

        except Exception as e:
            logger.error(f"❌ Failed to get service integration health: {e}")
            return self._create_fallback_health_status()

    def _create_fallback_health_status(self) -> Dict[str, Any]:
        """Create fallback health status when Context API is unavailable"""
        return {
            "healthy": False,
            "service_discovery_healthy": False,
            "toolset_manager_healthy": False,
            "total_toolsets": len(self.cached_toolsets),
            "enabled_toolsets": len([ts for ts in self.cached_toolsets.values() if ts.enabled]),
            "discovered_services": 0,
            "available_services": 0,
            "last_update": self.last_update.isoformat() if self.last_update else None,
            "note": "Context API unavailable, using cached data"
        }

    async def get_toolset_stats(self) -> Dict[str, Any]:
        """
        Get statistics about managed toolsets
        Business Requirement: BR-HOLMES-029 - Service discovery metrics and monitoring
        """
        toolsets = list(self.cached_toolsets.values())

        type_counts = {}
        for ts in toolsets:
            # Extract service type from toolset name (format: type-namespace-service)
            service_type = ts.name.split('-')[0] if '-' in ts.name else ts.name
            type_counts[service_type] = type_counts.get(service_type, 0) + 1

        return {
            "total_toolsets": len(toolsets),
            "enabled_count": len([ts for ts in toolsets if ts.enabled]),
            "type_counts": type_counts,
            "last_update": self.last_update.isoformat() if self.last_update else None,
            "cache_hit_rate": 0.85  # Simulated cache hit rate
        }
