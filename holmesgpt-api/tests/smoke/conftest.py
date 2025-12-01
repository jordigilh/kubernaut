"""
Smoke Test Configuration
"""

import os
import sys

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'src'))


def pytest_configure(config):
    """Register smoke marker."""
    config.addinivalue_line(
        "markers", "smoke: mark test as smoke test (requires real LLM)"
    )

