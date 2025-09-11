"""
API Routes Package
FastAPI route handlers organized by functionality
"""

from . import health, investigation, chat, config, auth, metrics

__all__ = [
    "health",
    "investigation",
    "chat",
    "config",
    "auth",
    "metrics"
]
