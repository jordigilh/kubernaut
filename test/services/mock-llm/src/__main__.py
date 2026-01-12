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
Mock LLM Service entry point.

Runs the mock LLM server on the configured port (default: 8080).
"""

import os
import logging
import sys
from .server import MockLLMServer

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    stream=sys.stdout
)

logger = logging.getLogger(__name__)


def main():
    """Start the Mock LLM server."""
    # Read configuration from environment
    host = os.getenv("MOCK_LLM_HOST", "0.0.0.0")
    port = int(os.getenv("MOCK_LLM_PORT", "8080"))
    force_text = os.getenv("MOCK_LLM_FORCE_TEXT", "false").lower() == "true"

    logger.info(f"Starting Mock LLM Service v1.0.0")
    logger.info(f"Configuration:")
    logger.info(f"  Host: {host}")
    logger.info(f"  Port: {port}")
    logger.info(f"  Force Text Response: {force_text}")

    # Create and start server
    server = MockLLMServer(
        host=host,
        port=port,
        force_text_response=force_text
    )

    try:
        server.start()
        logger.info(f"Mock LLM server started successfully")
        logger.info(f"  URL: {server.url}")
        logger.info(f"  Health: {server.url}/health")
        logger.info(f"  Metrics: {server.url}/metrics")
        logger.info("Press Ctrl+C to stop")

        # Keep running
        import time
        while True:
            time.sleep(1)

    except KeyboardInterrupt:
        logger.info("Shutting down...")
        server.stop()
        logger.info("Mock LLM server stopped")
    except Exception as e:
        logger.error(f"Server error: {e}")
        server.stop()
        sys.exit(1)


if __name__ == "__main__":
    main()
