#!/bin/bash
set -e

# Model Comparison Infrastructure Setup Script (Ollama Version)
# Sets up ollama servers for model comparison testing as alternative to ramallama

echo "🚀 Setting up Model Comparison Infrastructure (Ollama)..."

# Configuration
GRANITE_PORT=11434
DEEPSEEK_PORT=11435
STEINER_PORT=11436

# Models to download and serve
GRANITE_MODEL="granite3.1-dense:8b"
DEEPSEEK_MODEL="deepseek-coder:6.7b"
GRANITE33_MODEL="granite3.3:2b"

# Function to check if port is in use
check_port() {
    local port=$1
    if lsof -Pi :$port -sTCP:LISTEN -t >/dev/null ; then
        echo "⚠️  Port $port is already in use"
        return 1
    fi
    return 0
}

# Function to wait for server to be ready
wait_for_server() {
    local endpoint=$1
    local model_name=$2
    local max_attempts=30
    local attempt=1

    echo "⏳ Waiting for $model_name server to be ready at $endpoint..."

    while [ $attempt -le $max_attempts ]; do
        if curl -s "$endpoint/v1/models" >/dev/null 2>&1; then
            echo "✅ $model_name server is ready!"
            return 0
        fi
        echo "   Attempt $attempt/$max_attempts - waiting 10s..."
        sleep 10
        attempt=$((attempt + 1))
    done

    echo "❌ $model_name server failed to start within timeout"
    return 1
}

# Function to check if ollama is available
check_ollama() {
    if ! command -v ollama >/dev/null 2>&1; then
        echo "❌ ollama not found. Please install ollama first:"
        echo "   Visit: https://ollama.ai/download"
        echo "   Or use homebrew: brew install ollama"
        exit 1
    else
        echo "✅ ollama found at $(which ollama)"
    fi
}

# Function to download model if not present
download_model() {
    local model=$1

    echo "📥 Checking model: $model"

    if ollama list | grep -q "$model"; then
        echo "✅ Model $model already available"
    else
        echo "⬇️  Downloading $model..."
        ollama pull "$model"
    fi
}

# Function to start ollama server instance
start_ollama_server() {
    local model=$1
    local port=$2
    local log_file="ollama_${model//[:\/]/_}_${port}.log"

    echo "🚀 Starting ollama server for $model on port $port..."

    # Set OLLAMA_HOST for this instance
    export OLLAMA_HOST="0.0.0.0:$port"

    # Start ollama serve in background for this port
    ollama serve > "$log_file" 2>&1 &
    local pid=$!
    echo "$pid" > "ollama_${port}.pid"

    echo "   PID: $pid"
    echo "   Logs: $log_file"

    # Wait for server to start
    sleep 5

    # Load the model (this will start serving it)
    echo "   Loading model $model..."
    ollama run "$model" "Hello" > /dev/null 2>&1 || true

    # Wait for server to be ready
    if ! wait_for_server "http://localhost:$port" "$model"; then
        echo "❌ Failed to start ollama server for $model"
        return 1
    fi

    return 0
}

# Function to check server health
check_server_health() {
    local endpoint=$1
    local model_name=$2

    echo "🔍 Checking health of $model_name at $endpoint..."

    # Test models endpoint
    if ! curl -s "$endpoint/v1/models" | jq . >/dev/null 2>&1; then
        echo "❌ Models endpoint failed for $model_name"
        return 1
    fi

    # Test completions endpoint with simple prompt
    local test_response
    test_response=$(curl -s -X POST "$endpoint/v1/chat/completions" \
        -H "Content-Type: application/json" \
        -d '{
            "model": "'"$model_name"'",
            "messages": [{"role": "user", "content": "Hello"}],
            "max_tokens": 10
        }')

    if ! echo "$test_response" | jq '.choices[0].message.content' >/dev/null 2>&1; then
        echo "❌ Chat completions failed for $model_name"
        echo "Response: $test_response"
        return 1
    fi

    echo "✅ $model_name server is healthy"
    return 0
}

# Function to setup separate ollama instances
setup_multiple_ollama() {
    echo "🔧 Setting up multiple ollama instances..."

    # Note: ollama doesn't natively support multiple ports easily
    # For demo purposes, we'll use the default ollama instance on 11434
    # and document how ramallama would handle multiple instances better

    echo "⚠️  Note: ollama runs on a single port (11434) by default"
    echo "   For true multi-model comparison, ramallama is preferred"
    echo "   This demo will use ollama with model switching"

    # Ensure ollama service is running
    if ! pgrep -f "ollama serve" >/dev/null; then
        echo "🚀 Starting ollama service..."
        ollama serve > "ollama_service.log" 2>&1 &
        echo $! > "ollama_service.pid"
        sleep 10
    else
        echo "✅ ollama service already running"
    fi

    # Download all models
    download_model "$GRANITE_MODEL"
    download_model "$DEEPSEEK_MODEL"
    download_model "$GRANITE33_MODEL"

    # For demo, we'll use the single ollama instance
    # In production, you'd want separate instances or ramallama
    return 0
}

# Main setup function
main() {
    echo "==============================================="
    echo "🔧 Model Comparison Infrastructure Setup (Ollama)"
    echo "==============================================="

    # Check if running from correct directory
    if [[ ! -f "go.mod" ]] || [[ ! -d "test/integration" ]]; then
        echo "❌ Please run this script from the project root directory"
        exit 1
    fi

    # Create logs directory
    mkdir -p logs
    cd logs

    echo "📋 Setup Plan:"
    echo "   • Using ollama for model serving (demo mode)"
    echo "   • Models: $GRANITE_MODEL, $DEEPSEEK_MODEL, $GRANITE33_MODEL"
    echo "   • Note: For production, consider ramallama for better multi-instance support"
    echo ""

    # Check dependencies
    echo "🔍 Checking dependencies..."
    check_ollama

    if ! command -v jq >/dev/null 2>&1; then
        echo "⚠️  jq recommended for JSON parsing"
        echo "   Install: brew install jq"
    fi

    # Setup ollama instances
    setup_multiple_ollama

    # Test the default ollama instance
    echo ""
    echo "🔍 Testing ollama service..."
    if check_server_health "http://localhost:11434" "$GRANITE_MODEL"; then
        echo "✅ ollama service is working correctly"
    else
        echo "❌ ollama service test failed"
        exit 1
    fi

    echo ""
    echo "✅ Model comparison infrastructure ready (ollama mode)!"
    echo ""
    echo "📊 Ready for model comparison testing:"
    echo "   • ollama service running on http://localhost:11434"
    echo "   • Models available: $GRANITE_MODEL, $DEEPSEEK_MODEL, $GRANITE33_MODEL"
    echo ""
    echo "🧪 To run the model comparison tests:"
    echo "   cd .."
    echo "   make model-comparison-test"
    echo ""
    echo "ℹ️  Note: This setup uses ollama in demo mode."
    echo "   For production multi-model comparison, install ramallama:"
    echo "   cargo install ramallama"
    echo ""
    echo "🛑 To stop:"
    echo "   ./scripts/stop_model_comparison.sh"
}

# Cleanup function for script interruption
cleanup() {
    echo ""
    echo "🛑 Setup interrupted. Cleaning up..."
    pkill -f "ollama serve" || true
    exit 1
}

trap cleanup INT TERM

# Run main function
main "$@"
