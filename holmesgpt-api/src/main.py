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


def load_config() -> Dict[str, Any]:
    """
    Load service configuration from environment

    Design Decision: DD-HOLMESGPT-012 - Minimal configuration for internal service
    """
    return {
        "service_name": "holmesgpt-api",
        "version": "1.0.0",
        "environment": os.getenv("ENVIRONMENT", "development"),
        "dev_mode": os.getenv("DEV_MODE", "true").lower() == "true",

        # Authentication (K8s ServiceAccount tokens)
        "auth_enabled": os.getenv("AUTH_ENABLED", "false").lower() == "true",

        # LLM Configuration
        "llm": {
            "provider": os.getenv("LLM_PROVIDER", "ollama"),
            "model": os.getenv("LLM_MODEL", "llama2"),
            "endpoint": os.getenv("LLM_ENDPOINT", "http://localhost:11434"),
        },
    }


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


@app.on_event("startup")
async def startup_event():
    logger.info(f"Starting {config['service_name']} v{config['version']}")
    logger.info(f"Environment: {config['environment']}")
    logger.info(f"Dev mode: {config['dev_mode']}")
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
