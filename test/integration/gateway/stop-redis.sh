#!/bin/bash
set -euo pipefail

echo "🛑 Stopping local Redis..."
podman stop redis-gateway-test 2>/dev/null || true
podman rm -f redis-gateway-test 2>/dev/null || true
echo "✅ Redis stopped"


