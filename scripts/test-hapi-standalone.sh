#!/bin/bash
# Test HolmesGPT-API Standalone (Outside Kind)
# Purpose: Isolate whether HAPI image is functional outside of Kubernetes
# Date: 2025-12-29

set -e

PROJECT_ROOT="$(git rev-parse --show-toplevel)"
cd "${PROJECT_ROOT}"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🧪 HolmesGPT-API Standalone Test"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Find the latest HAPI image
echo "📦 Finding latest holmesgpt-api image..."
HAPI_IMAGE=$(podman images --format "{{.Repository}}:{{.Tag}}" | grep "localhost/holmesgpt-api:aianalysis" | head -1)

if [ -z "$HAPI_IMAGE" ]; then
    echo "❌ No holmesgpt-api image found"
    echo "Expected format: localhost/holmesgpt-api:aianalysis-{uuid}"
    echo ""
    echo "Available images:"
    podman images | grep holmesgpt || echo "No holmesgpt images found"
    echo ""
    echo "Build image first with: make test-e2e-aianalysis"
    exit 1
fi

echo "✅ Found image: ${HAPI_IMAGE}"
echo ""

# Create config directory
CONFIG_DIR="${PROJECT_ROOT}/test/integration/aianalysis/hapi-config"
mkdir -p "${CONFIG_DIR}"

echo "📝 Creating test config..."
cat > "${CONFIG_DIR}/config.yaml" <<EOF
llm:
  provider: "mock"
  model: "mock/test-model"
  endpoint: "http://localhost:11434"
data_storage:
  url: "http://localhost:18090"
logging:
  level: "DEBUG"
EOF

echo "✅ Config created at ${CONFIG_DIR}/config.yaml"
echo ""

# Stop any existing container
echo "🧹 Cleaning up any existing test container..."
podman stop hapi-standalone-test 2>/dev/null || true
podman rm hapi-standalone-test 2>/dev/null || true
echo ""

# Test 1: Run with config file
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🧪 TEST 1: Run HAPI with --config flag"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "Command:"
echo "podman run --rm -it \\"
echo "  --name hapi-standalone-test \\"
echo "  -p 18080:8080 \\"
echo "  -v ${CONFIG_DIR}:/etc/holmesgpt:ro \\"
echo "  -e MOCK_LLM_MODE=true \\"
echo "  ${HAPI_IMAGE} \\"
echo "  --config /etc/holmesgpt/config.yaml"
echo ""

echo "Starting container (will run for 30 seconds)..."
timeout 30 podman run --rm \
  --name hapi-standalone-test \
  -p 18080:8080 \
  -v "${CONFIG_DIR}:/etc/holmesgpt:ro" \
  -e MOCK_LLM_MODE=true \
  "${HAPI_IMAGE}" \
  --config /etc/holmesgpt/config.yaml &

CONTAINER_PID=$!
sleep 5

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🔍 TEST 2: Check if HAPI is responsive"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

MAX_RETRIES=5
RETRY_COUNT=0
HAPI_READY=false

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    echo "Attempt $((RETRY_COUNT + 1))/${MAX_RETRIES}: Checking http://localhost:18080/health"

    if curl -s -f http://localhost:18080/health > /dev/null 2>&1; then
        echo "✅ HAPI health check passed!"
        HAPI_READY=true
        break
    else
        echo "⏳ Not ready yet, waiting 2s..."
        sleep 2
        RETRY_COUNT=$((RETRY_COUNT + 1))
    fi
done

echo ""

if [ "$HAPI_READY" = true ]; then
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "🧪 TEST 3: Test /health endpoint"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    curl -s http://localhost:18080/health | jq '.' || curl -s http://localhost:18080/health
    echo ""

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "🧪 TEST 4: Test /api/v1/investigate endpoint"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    curl -s -X POST http://localhost:18080/api/v1/investigate \
      -H "Content-Type: application/json" \
      -d '{
        "context": "test pod is crashing",
        "analysis_types": ["incident-analysis"]
      }' | jq '.' || echo "⚠️  Investigation endpoint failed"
    echo ""
fi

# Clean up
echo "🧹 Cleaning up..."
kill $CONTAINER_PID 2>/dev/null || true
wait $CONTAINER_PID 2>/dev/null || true
podman stop hapi-standalone-test 2>/dev/null || true
echo ""

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 Test Results"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [ "$HAPI_READY" = true ]; then
    echo "✅ HAPI is FUNCTIONAL outside of Kind"
    echo ""
    echo "🔍 Root Cause: Issue is likely in Kind/K8s deployment, not HAPI itself"
    echo ""
    echo "Possible causes:"
    echo "  1. ConfigMap not mounting correctly in Kind"
    echo "  2. Image not loaded properly into Kind"
    echo "  3. Network connectivity issue to dependencies"
    echo "  4. Resource constraints in Kind cluster"
    echo ""
    echo "Next: Run scripts/debug-hapi-e2e-failure.sh to investigate Kind deployment"
else
    echo "❌ HAPI FAILED to start even outside Kind"
    echo ""
    echo "🔍 Root Cause: Issue is with HAPI image or configuration"
    echo ""
    echo "Possible causes:"
    echo "  1. Python dependencies missing in image"
    echo "  2. Config file format incorrect"
    echo "  3. HAPI application code has bugs"
    echo "  4. Missing environment variables"
    echo ""
    echo "Next: Check HAPI container logs:"
    echo "  podman logs hapi-standalone-test"
    echo ""
    echo "Or run interactively:"
    echo "  podman run -it --rm --entrypoint /bin/bash ${HAPI_IMAGE}"
fi
echo ""




