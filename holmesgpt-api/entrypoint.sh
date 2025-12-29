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

echo "Starting HolmesGPT-API with config: $CONFIG_PATH"

# Start uvicorn server (cannot pass custom flags - uses CONFIG_FILE env var)
exec uvicorn src.main:app --host 0.0.0.0 --port 8080 --workers 4
