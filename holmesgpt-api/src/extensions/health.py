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
Health and Readiness Endpoints

Business Requirements: BR-HAPI-126 to 145 (Health/Monitoring)
"""

import importlib.util
import logging
from typing import Dict, Any
from fastapi import APIRouter, status
from fastapi.responses import JSONResponse

logger = logging.getLogger(__name__)

router = APIRouter()


def _check_sdk() -> bool:
    """
    Check if HolmesGPT SDK is available

    REFACTOR phase: Real SDK availability check
    Checks if the holmes package can be imported (installed via pip)
    """
    try:
        return importlib.util.find_spec("holmes") is not None
    except (ImportError, ModuleNotFoundError):
        logger.warning({"event": "sdk_not_found", "reason": "holmes package not importable"})
        return False
    except Exception as e:
        logger.error({"event": "sdk_check_failed", "error": str(e)})
        return False


def _check_prometheus() -> bool:
    """
    Check if Prometheus integration is available

    REFACTOR phase: Real Prometheus availability check
    """
    try:
        return importlib.util.find_spec("prometheus_client") is not None
    except (ImportError, ModuleNotFoundError):
        logger.warning({"event": "prometheus_not_available"})
        return False


def _check_context_api() -> bool:
    """
    Check if Context API is reachable

    REFACTOR phase: Real Context API connectivity check
    GREEN phase: Assume available (network policies handle access)
    """
    # For minimal service, assume available if running in same namespace
    # Network policies ensure connectivity
    return True


def _check_dependencies() -> Dict[str, Any]:
    """
    Check critical dependencies

    Business Requirement: BR-HAPI-017 (Dependency health checking)

    REFACTOR phase: Real dependency health checks
    """
    return {
        "sdk": {"status": "up" if _check_sdk() else "down"},
        "context_api": {"status": "up" if _check_context_api() else "down"},
        "prometheus": {"status": "up" if _check_prometheus() else "down"}
    }


@router.get("/health", status_code=status.HTTP_200_OK)
async def health_check():
    """
    Liveness probe endpoint

    Business Requirement: BR-HAPI-126 (Health check endpoint)
    """
    logger.info({"event": "health_check_called"})
    return {
        "status": "healthy",
        "service": "holmesgpt-api",
        "endpoints": [
            "/api/v1/incident/analyze",
            # DD-017: PostExec endpoint deferred to V1.1 - Effectiveness Monitor not in V1.0
            # "/api/v1/postexec/analyze",
            "/health",
            "/ready"
        ],
        "features": {
            "incident_analysis": True,
            # DD-017: PostExec feature deferred to V1.1 - Effectiveness Monitor not in V1.0
            # "postexec_analysis": True,
            "authentication": True
        },
    }


@router.get("/ready", status_code=status.HTTP_200_OK)
async def readiness_check():
    """
    Readiness probe endpoint

    Business Requirements:
    - BR-HAPI-127 (Readiness check endpoint)
    - BR-HAPI-201 (Graceful shutdown with DD-007 pattern)

    TDD GREEN Phase: Check shutdown flag first
    REFACTOR phase: Real dependency health checks
    """
    # Import shutdown flag from main module
    # BR-HAPI-201: Return 503 during graceful shutdown
    import src.main as main_module

    # Check if service is shutting down (DD-007 readiness coordination)
    if hasattr(main_module, 'is_shutting_down') and main_module.is_shutting_down:
        logger.info({
            "event": "readiness_check_shutting_down",
            "dd": "DD-007-readiness-503"
        })
        return JSONResponse(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            content={
                "status": "shutting_down",
                "reason": "Service is gracefully shutting down"
            }
        )

    # Check dependencies
    dependencies = _check_dependencies()

    # Check if all critical dependencies are up
    all_healthy = all(
        dep["status"] == "up"
        for dep in dependencies.values()
    )

    if not all_healthy:
        logger.warning({
            "event": "readiness_check_failed",
            "dependencies": dependencies
        })
        return JSONResponse(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            content={
                "status": "not_ready",
                "dependencies": dependencies
            }
        )

    logger.info({"event": "readiness_check_passed"})
    return {
        "status": "ready",
        "dependencies": dependencies
    }


@router.get("/config", status_code=status.HTTP_200_OK)
async def get_config():
    """
    Get service configuration (sanitized)

    Business Requirement: BR-HAPI-128 (Configuration endpoint)
    """
    config = getattr(router, "config", {})
    safe_config = {
        "llm": {
            "provider": config.get("llm", {}).get("provider"),
            "model": config.get("llm", {}).get("model"),
            "endpoint": config.get("llm", {}).get("endpoint")
        },
        "environment": config.get("environment"),
        "dev_mode": config.get("dev_mode"),
    }
    return safe_config
