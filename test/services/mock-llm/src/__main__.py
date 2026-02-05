"""
Entrypoint for Mock LLM Service

Runs the Mock LLM HTTP server using Python's built-in HTTP server.

DD-TEST-011 v2.0: File-Based Configuration Pattern
- Reads workflow UUIDs from YAML configuration file at startup
- File can be direct path (integration tests) or ConfigMap mount (E2E)
- Simple, deterministic, environment-agnostic approach
"""

import os
import logging
from src.server import start_server, load_scenarios_from_file

# Configure logging to output to stdout
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[logging.StreamHandler()]
)

if __name__ == "__main__":
    print("🚀 Mock LLM v2.0 - File-Based Configuration (DD-TEST-011 v2.0)")
    print("=" * 70)

    # DD-TEST-011 v2.0: Load workflows from configuration file
    # In E2E: File is mounted via ConfigMap at /config/scenarios.yaml
    # In Integration: File is written directly to temp path
    # In Local Dev: File is specified via environment variable
    config_path = os.getenv("MOCK_LLM_CONFIG_PATH", "/config/scenarios.yaml")

    if os.path.exists(config_path):
        print(f"📋 Loading workflow UUIDs from file: {config_path}")
        success = load_scenarios_from_file(config_path)
        if not success:
            print("⚠️  Failed to load scenarios, using defaults")
    else:
        print(f"ℹ️  Configuration file not found: {config_path}")
        print("   Using default scenario UUIDs (development mode)")

    print("=" * 70)

    # Start HTTP server
    host = os.getenv("MOCK_LLM_HOST", "0.0.0.0")
    port = int(os.getenv("MOCK_LLM_PORT", "8080"))
    start_server(host=host, port=port)
