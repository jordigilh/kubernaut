"""
Entrypoint for Mock LLM Service

Runs the Mock LLM HTTP server using Python's built-in HTTP server.
"""

from src.server import start_server

if __name__ == "__main__":
    # Start the Mock LLM server on port 8080
    start_server(port=8080)
