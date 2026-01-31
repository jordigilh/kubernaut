"""
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
"""

"""
Main entry point for HolmesGPT API Service
Minimal internal service wrapper around HolmesGPT SDK

Design Decision: DD-HOLMESGPT-012 - Minimal Internal Service Architecture
- Internal-only service (network policies handle access control)
- Kubernetes RBAC handles authorization
- Simple authentication (ServiceAccount tokens)

Business Requirements:
- BR-HAPI-200: RFC 7807 Error Response Standard
- BR-HAPI-201: Graceful Shutdown with DD-007 Pattern
"""

import logging
import os
import signal
import threading
import yaml
from pathlib import Path
from typing import Dict, Any, Optional
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

# Import centralized logging configuration
from src.config import setup_logging

# Import hot-reload ConfigManager (BR-HAPI-199, DD-HAPI-004)
from src.config.hot_reload import ConfigManager

# Import config models
from src.models.config_models import AppConfig

logger = logging.getLogger(__name__)

# Import extensions
from src.extensions import recovery, incident, health
# DD-017: PostExec extension deferred to V1.1 - Effectiveness Monitor not in V1.0
# from src.extensions import postexec
from src.middleware.auth import AuthenticationMiddleware
from src.middleware.metrics import PrometheusMetricsMiddleware, metrics_endpoint
from src.middleware.rfc7807 import add_rfc7807_exception_handlers

# Import auth components for dependency injection (DD-AUTH-014)
from src.auth import K8sAuthenticator, K8sAuthorizer, MockAuthenticator, MockAuthorizer


# ========================================
# GRACEFUL SHUTDOWN STATE (GREEN Phase)
# ========================================

# Global shutdown flag for readiness probe coordination
# BR-HAPI-201: Graceful shutdown with DD-007 pattern
# TDD GREEN Phase: Minimal implementation to make tests pass
is_shutting_down = False


def handle_shutdown_signal(signum, frame):
    """
    Handle SIGTERM/SIGINT for graceful shutdown

    Business Requirement: BR-HAPI-201
    Design Decision: DD-007 - Kubernetes-Aware Graceful Shutdown

    TDD GREEN Phase: Minimal implementation
    Sets shutdown flag to signal readiness probe to return 503.
    uvicorn handles the actual graceful shutdown automatically.
    """
    global is_shutting_down

    signal_name = "SIGTERM" if signum == signal.SIGTERM else "SIGINT"

    logger.info({
        "event": "shutdown_signal_received",
        "signal": signal_name,
        "dd": "DD-007-step1-signal-received"
    })

    # Set shutdown flag - readiness probe will now return 503
    is_shutting_down = True

    logger.info({
        "event": "shutdown_flag_set",
        "readiness_probe": "will_return_503",
        "dd": "DD-007-step2-readiness-coordination"
    })


# Register signal handlers (only in main thread)
# BR-HAPI-TESTING: Signal handlers only work in main thread
# When pytest runs with -n (parallel workers), worker threads cannot register signal handlers
# Solution: Only register in main thread to allow parallel test execution
if threading.current_thread() is threading.main_thread():
    signal.signal(signal.SIGTERM, handle_shutdown_signal)
    signal.signal(signal.SIGINT, handle_shutdown_signal)
    logger.info("Signal handlers registered for graceful shutdown (SIGTERM, SIGINT)")
else:
    logger.info("Skipping signal handler registration (not main thread)")


def load_config() -> AppConfig:
    """
    Load service configuration from YAML file

    ADR-030: Configuration Management Standard with Exception
    - External Interface: -config flag (consistent with Go services)
    - Internal Implementation: CONFIG_FILE env var (uvicorn limitation)

    EXCEPTION RATIONALE:
    Uvicorn does NOT support custom command-line flags. Test proof:
      $ uvicorn src.main:app --custom-flag value
      Error: No such option: --custom-flag

    Solution: entrypoint.sh parses -config flag and exports CONFIG_FILE env var.
    See: docs/architecture/decisions/ADR-030-EXCEPTION-HAPI-ENV-VAR.md

    User Experience (identical to Go services):
    - Gateway:  args: ["-config", "/etc/gateway/config.yaml"]
    - HAPI:     args: ["-config", "/etc/holmesgpt/config.yaml"]
    """
    import os

    # ADR-030 EXCEPTION: Read from env var (set by entrypoint.sh from -config flag)
    config_file = os.getenv("CONFIG_FILE", "/etc/holmesgpt/config.yaml")
    config_path = Path(config_file)

    # ADR-030: Config file is MANDATORY - fail fast if not found
    if not config_path.exists():
        raise FileNotFoundError(
            f"Configuration file not found: {config_path}\n"
            f"ADR-030 requires YAML ConfigMap to be mounted.\n"
            f"Ensure ConfigMap is mounted at /etc/holmesgpt/ or use -config flag."
        )

    # Load configuration from YAML file
    try:
        logger.info(f"Loading configuration from {config_file}")
        with open(config_path, 'r') as f:
            config = yaml.safe_load(f)

        if not config:
            raise ValueError("Configuration file is empty")

        # ADR-030: Business-critical settings from YAML, everything else hardcoded

        # Hardcoded defaults (no business value in configuration)
        defaults = {
            "service_name": "holmesgpt-api",
            "version": "1.0.0",
            "dev_mode": False,
            "auth_enabled": True,  # DD-AUTH-014: Auth always enabled via middleware
            "api_host": "0.0.0.0",
            "api_port": 8080,
            "llm": {
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
            "audit": {
                "flush_interval_seconds": 5.0,  # Default: 5 seconds (production)
                "buffer_size": 10000,           # BR-AUDIT-005, ADR-038
                "batch_size": 50,               # Events per HTTP batch
            },
        }

        # Merge YAML config with defaults (YAML takes precedence)
        for key in ["llm", "logging", "data_storage", "audit"]:
            if key in config:
                if key in defaults and isinstance(defaults[key], dict):
                    defaults[key].update(config[key])
                else:
                    defaults[key] = config[key]

        config = {**defaults, **config}

        # Map legacy log_level to nested logging.level (backward compat)
        if "log_level" not in config and "logging" in config:
            config["log_level"] = config["logging"].get("level", "INFO")
        elif "log_level" in config:
            config.setdefault("logging", {})["level"] = config["log_level"]

        logger.info({
            "event": "config_loaded",
            "source": "yaml_file",
            "path": config_file,
            "llm_provider": config.get("llm", {}).get("provider"),
        })

        return config

    except Exception as e:
        logger.error({
            "event": "config_load_failed",
            "error": str(e),
            "path": config_file,
        })
        raise


# Load configuration
config = load_config()

# Setup logging based on configuration
# This must be called after config is loaded but before any logging occurs
setup_logging(config)
logger.info(f"holmesgpt-api starting with log level: {config.get('log_level', 'INFO')}")

# ========================================
# HOT-RELOAD CONFIG MANAGER (BR-HAPI-199)
# ========================================
# ConfigManager provides hot-reload capability for config changes
# without pod restart. Falls back to static config if file not found.

config_manager: Optional[ConfigManager] = None
HOT_RELOAD_ENABLED = os.getenv("HOT_RELOAD_ENABLED", "true").lower() == "true"

def init_config_manager() -> Optional[ConfigManager]:
    """
    Initialize ConfigManager for hot-reload support.

    BR-HAPI-199: ConfigMap Hot-Reload
    DD-HAPI-004: ConfigMap Hot-Reload Design

    Returns None if config file doesn't exist or hot-reload is disabled.
    """
    if not HOT_RELOAD_ENABLED:
        logger.info("Hot-reload disabled via HOT_RELOAD_ENABLED=false")
        return None

    config_file = os.getenv("CONFIG_FILE", "/etc/holmesgpt/config.yaml")
    config_path = Path(config_file)

    if not config_path.exists():
        logger.warning(f"Config file not found: {config_file}, hot-reload disabled")
        return None

    try:
        manager = ConfigManager(config_file, logger, enable_hot_reload=True)
        manager.start()
        logger.info({
            "event": "config_manager_started",
            "br": "BR-HAPI-199",
            "dd": "DD-HAPI-004",
            "path": config_file,
            "hot_reload": True,
        })
        return manager
    except Exception as e:
        logger.error(f"Failed to start ConfigManager: {e}, using static config")
        return None

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

# Add authentication middleware with dependency injection (DD-AUTH-014)
# Production: Real K8s TokenReview + SAR APIs
# Integration/E2E: Mock implementations (configurable via ENV_MODE)
ENV_MODE = os.getenv("ENV_MODE", "production").lower()

if ENV_MODE == "production":
    # Production: Real Kubernetes TokenReview + SAR APIs
    # Authority: DD-AUTH-014 (Middleware-based SAR authentication)
    try:
        # DD-AUTH-014: Support file-based kubeconfig for integration tests
        # If KUBECONFIG env var is set, load from file (integration tests with envtest)
        # Otherwise, load in-cluster config (production)
        import os
        from kubernetes import client as k8s_client, config as k8s_config
        
        if os.getenv("KUBECONFIG"):
            logger.info({
                "event": "loading_kubeconfig",
                "path": os.getenv("KUBECONFIG"),
                "mode": "file-based (integration tests)"
            })
            k8s_config.load_kube_config()
            api_client = k8s_client.ApiClient()
            authenticator = K8sAuthenticator(api_client)
            authorizer = K8sAuthorizer(api_client)
        else:
            logger.info({"event": "loading_incluster_config", "mode": "production"})
            authenticator = K8sAuthenticator()
            authorizer = K8sAuthorizer()
        
        logger.info({
            "event": "auth_initialized",
            "mode": "production" if not os.getenv("KUBECONFIG") else "integration",
            "authenticator": "K8sAuthenticator",
            "authorizer": "K8sAuthorizer"
        })
    except Exception as e:
        logger.error({
            "event": "auth_init_failed",
            "mode": "production",
            "error": str(e)
        })
        # In production, fail fast if K8s auth cannot be initialized
        raise RuntimeError(f"Failed to initialize K8s auth components: {e}")
else:
    # Integration/E2E tests: Mock implementations
    # Authority: DD-AUTH-014 (Testable auth via dependency injection)
    authenticator = MockAuthenticator(
        valid_users={
            "test-token-authorized": "system:serviceaccount:test:authorized-sa",
            "test-token-readonly": "system:serviceaccount:test:readonly-sa",
        }
    )
    authorizer = MockAuthorizer(
        default_allow=True  # Permissive for integration tests
    )
    logger.info({
        "event": "auth_initialized",
        "mode": ENV_MODE,
        "authenticator": "MockAuthenticator",
        "authorizer": "MockAuthorizer",
        "note": "Using mock auth for testing"
    })

# Get pod namespace dynamically (for SAR checks)
# In production, read from ServiceAccount namespace file
# In integration/E2E, use environment variable or default
POD_NAMESPACE = os.getenv("POD_NAMESPACE")
if not POD_NAMESPACE:
    namespace_path = Path("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
    if namespace_path.exists():
        POD_NAMESPACE = namespace_path.read_text().strip()
    else:
        POD_NAMESPACE = "kubernaut-system"  # Default fallback

logger.info({"event": "sar_namespace_configured", "namespace": POD_NAMESPACE})

# Apply auth middleware with dependency injection
# Authority: DD-AUTH-014 - SAR authorization enforced on all endpoints
# Read auth config from YAML (allows override of resource_name for E2E)
auth_config = config.get("auth", {})
app.add_middleware(
    AuthenticationMiddleware,
    authenticator=authenticator,
    authorizer=authorizer,
    config={
        "namespace": POD_NAMESPACE,
        "resource": "services",
        "resource_name": auth_config.get("resource_name", "holmesgpt-api"),  # Match actual Service name
        "verb": "create",  # Default verb, will be mapped per HTTP method
    }
)
logger.info({
    "event": "auth_middleware_enabled",
    "authority": "DD-AUTH-014",
    "mode": ENV_MODE,
    "namespace": POD_NAMESPACE
})

# Add RFC 7807 exception handlers
add_rfc7807_exception_handlers(app)
logger.info("RFC 7807 exception handlers enabled (BR-HAPI-200)")

# Register extension routers
# All configuration is now via environment variables (LLM_ENDPOINT, LLM_MODEL, LLM_PROVIDER)
# No router.config anti-pattern - tests use mock LLM server instead
app.include_router(recovery.router, prefix="/api/v1", tags=["Recovery Analysis"])
app.include_router(incident.router, prefix="/api/v1", tags=["Incident Analysis"])
# DD-017: PostExec endpoint deferred to V1.1 - Effectiveness Monitor not available in V1.0
# Logic preserved in src/extensions/postexec.py for V1.1
# app.include_router(postexec.router, prefix="/api/v1", tags=["Post-Execution Analysis"])
app.include_router(health.router, tags=["Health"])


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
    global config_manager

    logger.info(f"Starting {config.get('service_name', 'holmesgpt-api')} v{config.get('version', '1.0.0')}")
    logger.info(f"LLM Provider: {config.get('llm', {}).get('provider', 'unknown')}")
    logger.info(f"Dev mode: {config.get('dev_mode', False)}")
    logger.info(f"Auth mode: {ENV_MODE} (DD-AUTH-014)")

    # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
    # MANDATORY: Validate audit initialization (ADR-032 §2)
    # Per ADR-032 §3: HAPI is P0 service - audit is MANDATORY for LLM interactions
    # Service MUST crash if audit cannot be initialized (ADR-032 §2)
    # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
    from src.audit.factory import get_audit_store
    try:
        # Get audit config from loaded YAML config (ADR-030)
        audit_config = config.get("audit", {})
        data_storage_url = config.get("data_storage", {}).get("url", "http://data-storage:8080")

        audit_store = get_audit_store(
            data_storage_url=data_storage_url,
            flush_interval_seconds=audit_config.get("flush_interval_seconds"),
            buffer_size=audit_config.get("buffer_size"),
            batch_size=audit_config.get("batch_size")
        )  # Will crash (sys.exit(1)) if init fails
        logger.info({
            "event": "audit_store_initialized",
            "status": "mandatory_per_adr_032",
            "classification": "P0",
            "adr": "ADR-032 §2",
            "audit_config": {
                "flush_interval_seconds": audit_config.get("flush_interval_seconds", 5.0),
                "buffer_size": audit_config.get("buffer_size", 10000),
                "batch_size": audit_config.get("batch_size", 50),
            }
        })
    except Exception as e:
        # This should never be reached (get_audit_store calls sys.exit(1))
        # But included for completeness
        logger.error(
            f"FATAL: Audit initialization failed - service cannot start per ADR-032 §2: {e}",
            extra={"adr": "ADR-032 §2"}
        )
        import sys
        sys.exit(1)  # Crash immediately - Kubernetes will restart pod

    # Initialize ConfigManager for hot-reload (BR-HAPI-199)
    config_manager = init_config_manager()
    if config_manager:
        app.state.config_manager = config_manager
        logger.info({
            "event": "hot_reload_enabled",
            "br": "BR-HAPI-199",
            "fields": ["llm.model", "llm.provider", "llm.endpoint", "toolsets", "log_level"],
        })
    else:
        app.state.config_manager = None
        logger.info("Hot-reload not available, using static configuration")

    logger.info("Service started successfully")


@app.on_event("shutdown")
async def shutdown_event():
    global config_manager

    logger.info("Service shutting down")

    # Stop ConfigManager (BR-HAPI-199)
    if config_manager:
        config_manager.stop()
        logger.info({
            "event": "config_manager_stopped",
            "br": "BR-HAPI-199",
            "reload_count": config_manager.reload_count,
            "error_count": config_manager.error_count,
        })


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        "src.main:app",
        host="0.0.0.0",
        port=8080,
        reload=config["dev_mode"],
        log_level="info"
    )
