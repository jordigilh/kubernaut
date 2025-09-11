#!/usr/bin/env python3
"""
HolmesGPT REST API Server
Production FastAPI server wrapping HolmesGPT SDK with enterprise features

Implements business requirements BR-HAPI-001 through BR-HAPI-120+
Follows development guidelines: reuse code, align with requirements, log all errors
"""

import asyncio
import logging
import os
import sys
import time
from contextlib import asynccontextmanager

import structlog
import uvicorn
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from prometheus_client import generate_latest

# Import application modules
from config import get_settings, setup_logging
from api.routes import investigation, chat, health, config, auth, metrics
from services.holmesgpt_service import HolmesGPTService
from services.context_api_service import ContextAPIService
from services.auth_service import get_auth_service
from services.metrics_service import get_metrics_service

# Configure structured logging per development guidelines
logger = structlog.get_logger(__name__)

# Global service instances
holmesgpt_service = None
context_api_service = None
auth_service = None
metrics_service = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan manager - BR-HAPI-040"""
    global holmesgpt_service, context_api_service, auth_service, metrics_service

    settings = get_settings()
    logger.info("🚀 Starting HolmesGPT REST API Server",
                version="1.0.0",
                llm_provider=settings.llm_provider,
                port=settings.port)

    try:
        # Initialize metrics service first - BR-HAPI-016
        logger.info("📊 Initializing Metrics service")
        metrics_service = get_metrics_service()
        metrics_service.set_app_info("1.0.0", "2025-01-15", "main-branch")

        # Initialize authentication service - BR-HAPI-026
        logger.info("🔐 Initializing Authentication service")
        auth_service = get_auth_service(settings)

        # Initialize core services - BR-HAPI-030
        logger.info("🔧 Initializing HolmesGPT SDK service")
        holmesgpt_service = HolmesGPTService(settings)
        await holmesgpt_service.initialize()

        logger.info("🔗 Initializing Context API service")
        context_api_service = ContextAPIService(settings)
        await context_api_service.initialize()

        # Validate service health - BR-HAPI-019
        if not await holmesgpt_service.health_check():
            logger.warning("HolmesGPT service health check failed - continuing with degraded service")
            metrics_service.set_service_status('holmesgpt', 'unhealthy')
        else:
            metrics_service.set_service_status('holmesgpt', 'healthy')

        if not await context_api_service.health_check():
            logger.warning("Context API service health check failed - continuing with degraded service")
            metrics_service.set_service_status('context_api', 'disconnected')
        else:
            metrics_service.set_service_status('context_api', 'connected')

        logger.info("✅ All services initialized successfully")

        # Set global references for dependency injection
        app.state.holmesgpt_service = holmesgpt_service
        app.state.context_api_service = context_api_service
        app.state.auth_service = auth_service
        app.state.metrics_service = metrics_service

        yield  # Application runs here

    except Exception as e:
        logger.error("❌ Failed to initialize services", error=str(e), exc_info=True)
        if metrics_service:
            metrics_service.record_error("startup_error", "main", "critical")
        raise
    finally:
        # Graceful shutdown - BR-HAPI-040
        logger.info("🛑 Shutting down HolmesGPT REST API Server")
        if holmesgpt_service:
            await holmesgpt_service.cleanup()
        if context_api_service:
            await context_api_service.cleanup()
        logger.info("✅ Cleanup completed")


# Create FastAPI application - BR-HAPI-036, BR-HAPI-042
app = FastAPI(
    title="HolmesGPT REST API",
    description="AI-powered Kubernetes investigation and troubleshooting API",
    version="1.0.0",
    docs_url="/docs",  # BR-HAPI-025
    redoc_url="/redoc",
    openapi_url="/openapi.json",
    lifespan=lifespan
)

# Add middleware - security and monitoring
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # Configure for production
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Include API routers - modular approach per development guidelines
app.include_router(health.router)  # Health endpoints (no auth required)
app.include_router(metrics.router)  # Metrics endpoints
app.include_router(auth.router, prefix="/auth")  # Authentication endpoints
app.include_router(investigation.router, prefix="/api/v1")  # Protected endpoints
app.include_router(chat.router, prefix="/api/v1")  # Protected endpoints
app.include_router(config.router, prefix="/api/v1")  # Protected endpoints


# Metrics endpoint is now handled by metrics router


def main():
    """Main application entry point - BR-HAPI-036"""
    # Setup logging first
    setup_logging()

    settings = get_settings()

    logger.info("🚀 Starting HolmesGPT REST API Server",
               version="1.0.0",
               port=settings.port,
               llm_provider=settings.llm_provider)

    # Validate required environment - BR-HAPI-030
    if not settings.llm_provider or not settings.llm_model:
        logger.error("❌ Missing required LLM configuration")
        sys.exit(1)

    try:
        # Run server - BR-HAPI-037 (async handling)
        uvicorn.run(
            "main:app",
            host="0.0.0.0",  # BR-HAPI-054
            port=settings.port,
            workers=1,
            log_config=None,  # Use our structured logging
            access_log=False,  # We handle logging in middleware
            reload=False
        )

    except Exception as e:
        logger.error("❌ Server startup failed", error=str(e), exc_info=True)
        sys.exit(1)


if __name__ == "__main__":
    main()