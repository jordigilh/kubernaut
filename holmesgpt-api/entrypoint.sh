#!/bin/bash
# HolmesGPT-API Entrypoint Script
# ADR-030: Configuration Management Standard with Exception
#
# EXCEPTION RATIONALE (see docs/architecture/decisions/ADR-030-EXCEPTION-HAPI-ENV-VAR.md):
# - Uvicorn does NOT support passing custom flags to FastAPI applications
# - Uvicorn rejects unknown command-line arguments with error
# - Environment variables are the ONLY way to pass configuration to Python/uvicorn apps
# - This script provides consistent -config flag interface (external) while using
#   CONFIG_FILE environment variable (internal implementation detail)
#
# PROOF: uvicorn --custom-flag value → Error: No such option: --custom-flag

set -e

# Default config path (matches Go services default)
CONFIG_PATH="/etc/holmesgpt/config.yaml"

# Parse -config flag (matches Go services flag parsing)
while [[ $# -gt 0 ]]; do
    case $1 in
        -config|--config)
            CONFIG_PATH="$2"
            shift 2
            ;;
        --config=*)
            CONFIG_PATH="${1#*=}"
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [-config /path/to/config.yaml]"
            exit 1
            ;;
    esac
done

# ADR-030 EXCEPTION: Export as environment variable (uvicorn limitation)
# External interface: -config flag (consistent with Go services)
# Internal implementation: CONFIG_FILE env var (Python/uvicorn constraint)
export CONFIG_FILE="$CONFIG_PATH"

# DD-AUTH-006: Port configuration for oauth-proxy integration
# Default: 8080 (backward compatible)
# With oauth-proxy: 8081 (oauth-proxy listens on 8080, proxies to 8081)
API_PORT="${API_PORT:-8080}"

echo "Starting HolmesGPT-API with config: $CONFIG_PATH"
echo "Listening on port: $API_PORT"

# Start uvicorn server (cannot pass custom flags - uses CONFIG_FILE env var)
# CRITICAL: Use python3.12 explicitly (UBI9 python-312 image defaults to python3.9)
# DD-AUTH-006: Port is configurable via API_PORT environment variable
# DD-HAPI-015: Single-worker async architecture for I/O-bound workload
# HAPI is 95% I/O-bound (HTTP calls to LLM/DataStorage/K8s API)
# FastAPI's async/await handles concurrency within single process
# Benefits: Resource efficiency (1 connection pool vs 4), singleton pattern works, 100+ concurrent requests

# DD-TEST-007: E2E Coverage Collection
# When E2E_COVERAGE=true, wrap uvicorn with coverage.py to collect Python code coverage.
# COVERAGE_FILE controls where the .coverage SQLite database is written.
# On SIGTERM (pod scale-down), uvicorn shuts down gracefully and coverage flushes data.
if [ "$E2E_COVERAGE" = "true" ]; then
    echo "📊 DD-TEST-007: E2E Coverage mode ENABLED"
    echo "   Coverage data file: ${COVERAGE_FILE:-/coverdata/.coverage}"
    echo "   Source: src/"
    export COVERAGE_FILE="${COVERAGE_FILE:-/coverdata/.coverage}"
    exec python3.12 -m coverage run --source=src -m uvicorn src.main:app --host 0.0.0.0 --port "$API_PORT" --workers 1
else
    exec python3.12 -m uvicorn src.main:app --host 0.0.0.0 --port "$API_PORT" --workers 1
fi
