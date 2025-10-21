"""
Main entry point for HolmesGPT API Service
Minimal internal service wrapper around HolmesGPT SDK

Design Decision: DD-HOLMESGPT-012 - Minimal Internal Service Architecture
- Internal-only service (network policies handle access control)
- Kubernetes RBAC handles authorization
- Simple authentication (ServiceAccount tokens)
"""

import logging
import os
import yaml
from pathlib import Path
from typing import Dict, Any
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

# Configure structured logging
logging.basicConfig(
    level=logging.INFO,
    format='{"timestamp":"%(asctime)s","level":"%(levelname)s","logger":"%(name)s","message":"%(message)s"}'
)
logger = logging.getLogger(__name__)

# Import extensions
from src.extensions import recovery, postexec, health
from src.middleware.auth import AuthenticationMiddleware
from src.middleware.metrics import PrometheusMetricsMiddleware, metrics_endpoint


def load_config() -> Dict[str, Any]:
    """
    Load service configuration from YAML file

    Design Decision: DD-HOLMESGPT-012 - Configuration as mounted ConfigMap
    - Reads config from /etc/holmesgpt/config.yaml (mounted ConfigMap)
    - Falls back to default development configuration if file not found
    - Cleaner than environment variables - no deployment changes for config updates

    Environment variable overrides:
    - CONFIG_FILE: Override config file path (default: /etc/holmesgpt/config.yaml)
    - LLM_CREDENTIALS_PATH: Path to LLM provider credentials (generic, any provider)
    - GOOGLE_APPLICATION_CREDENTIALS: Legacy/auto-set for Google Cloud compatibility
    """
    config_file = os.getenv("CONFIG_FILE", "/etc/holmesgpt/config.yaml")
    config_path = Path(config_file)

    # Default development configuration
    default_config = {
        "service_name": "holmesgpt-api",
        "version": "1.0.0",
        "log_level": "INFO",
        "dev_mode": True,
        "auth_enabled": False,
        "api_host": "0.0.0.0",
        "api_port": 8080,
        "llm": {
            "provider": "ollama",
            "model": "llama2",
            "endpoint": "http://localhost:11434",
            "max_retries": 3,
            "timeout_seconds": 60,
            "max_tokens_per_request": 4096,
            "temperature": 0.7,
        },
        "context_api": {
            "url": "http://localhost:8091",
            "timeout_seconds": 10,
            "max_retries": 2,
        },
        "kubernetes": {
            "service_host": "kubernetes.default.svc",
            "service_port": 443,
            "token_reviewer_enabled": True,
        },
        "public_endpoints": ["/health", "/ready", "/metrics"],
        "metrics": {
            "enabled": True,
            "endpoint": "/metrics",
            "scrape_interval": "30s",
        },
    }

    # Load config from file if it exists
    if config_path.exists():
        try:
            logger.info(f"Loading configuration from {config_file}")
            with open(config_path, 'r') as f:
                file_config = yaml.safe_load(f)

            # Merge file config with defaults (file config takes precedence)
            config = {**default_config, **file_config}

            # Ensure nested dicts are properly merged
            for key in ["llm", "context_api", "kubernetes", "metrics"]:
                if key in file_config:
                    config[key] = {**default_config.get(key, {}), **file_config[key]}

            logger.info({
                "event": "config_loaded",
                "source": "file",
                "path": config_file,
                "llm_provider": config.get("llm", {}).get("provider"),
                "auth_enabled": config.get("auth_enabled"),
            })
            return config

        except Exception as e:
            logger.error({
                "event": "config_load_failed",
                "error": str(e),
                "path": config_file,
                "fallback": "using_defaults"
            })
            # Fall through to return default config
    else:
        logger.warning({
            "event": "config_file_not_found",
            "path": config_file,
            "fallback": "using_defaults"
        })

    # Add service metadata to default config
    default_config["service_name"] = "holmesgpt-api"
    default_config["version"] = "1.0.0"

    return default_config


# Load configuration
config = load_config()

# Create FastAPI application
app = FastAPI(
    title="HolmesGPT API Service",
    description="Minimal internal service wrapper around HolmesGPT SDK",
    version=config["version"],
    docs_url="/docs" if config["dev_mode"] else None,
    redoc_url="/redoc" if config["dev_mode"] else None,
)

# Add CORS middleware (minimal - internal service)
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"] if config["dev_mode"] else ["http://localhost", "http://127.0.0.1"],
    allow_credentials=True,
    allow_methods=["GET", "POST"],
    allow_headers=["*"],
)

# Add Prometheus metrics middleware
app.add_middleware(PrometheusMetricsMiddleware)
logger.info("Prometheus metrics middleware enabled")

# Add authentication middleware
if config["auth_enabled"]:
    app.add_middleware(AuthenticationMiddleware, config=config)
    logger.info("Authentication middleware enabled (K8s ServiceAccount tokens)")
else:
    logger.warning("Authentication middleware DISABLED (dev mode only)")

# Register extension routers
app.include_router(recovery.router, prefix="/api/v1", tags=["Recovery Analysis"])
app.include_router(postexec.router, prefix="/api/v1", tags=["Post-Execution Analysis"])
app.include_router(health.router, tags=["Health"])

# Pass config to extensions
recovery.router.config = config
postexec.router.config = config
health.router.config = config


# ========================================
# METRICS ENDPOINT
# ========================================

@app.get("/metrics", include_in_schema=False)
async def metrics():
    """
    Prometheus metrics endpoint

    Business Requirement: BR-HAPI-100 to 103

    Exposes metrics in Prometheus exposition format for scraping
    """
    return metrics_endpoint()


@app.on_event("startup")
async def startup_event():
    logger.info(f"Starting {config.get('service_name', 'holmesgpt-api')} v{config.get('version', '1.0.0')}")
    logger.info(f"LLM Provider: {config.get('llm', {}).get('provider', 'unknown')}")
    logger.info(f"Dev mode: {config.get('dev_mode', False)}")
    logger.info(f"Auth enabled: {config.get('auth_enabled', False)}")
    logger.info("Service started successfully")


@app.on_event("shutdown")
async def shutdown_event():
    logger.info("Service shutting down")


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        "src.main:app",
        host="0.0.0.0",
        port=8080,
        reload=config["dev_mode"],
        log_level="info"
    )
