"""
HolmesGPT REST API Package.

A FastAPI-based REST API service for interacting with HolmesGPT,
providing alert investigation and remediation capabilities.
"""

__version__ = "1.0.0"
__author__ = "HolmesGPT API Team"
__description__ = "REST API service for HolmesGPT alert investigation and remediation"

from .main import app

__all__ = ["app"]

