#!/bin/bash
set -e

# Model Comparison Infrastructure Setup Script
# Sets up ramallama and vllm servers for model comparison testing

echo "🚀 Setting up Model Comparison Infrastructure..."

# Configuration
GRANITE_PORT=11434
DEEPSEEK_PORT=11435
STEINER_PORT=11436

# Models to download and serve
GRANITE_MODEL="granite3.1-dense:8b"
DEEPSEEK_MODEL="deepseek-coder:7b-instruct"
STEINER_MODEL="granite3.1-steiner:8b"

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

# Function to install ramallama if not present
install_ramallama() {
    if ! command -v ramallama >/dev/null 2>&1; then
        echo "📦 Installing ramallama..."
        if command -v cargo >/dev/null 2>&1; then
            cargo install ramallama
        else
            echo "❌ ramallama requires Rust/Cargo to be installed"
            echo "   Please install Rust from https://rustup.rs/ first"
            exit 1
        fi
    else
        echo "✅ ramallama already installed"
    fi
}

# Function to install vllm if not present
install_vllm() {
    if ! python -c "import vllm" >/dev/null 2>&1; then
        echo "📦 Installing vLLM..."
        pip install vllm
    else
        echo "✅ vLLM already installed"
    fi
}

# Function to download model if not present
download_model() {
    local model=$1
    local server_type=$2

    echo "📥 Checking model: $model"

    if [ "$server_type" = "ramallama" ]; then
        if ramallama list | grep -q "$model"; then
            echo "✅ Model $model already available"
        else
            echo "⬇️  Downloading $model..."
            ramallama pull "$model"
        fi
    elif [ "$server_type" = "vllm" ]; then
        # vLLM automatically downloads models on first use
        echo "✅ vLLM will download $model on first use"
    fi
}

# Function to start ramallama server
start_ramallama_server() {
    local model=$1
    local port=$2
    local log_file="ramallama_${model//[:\/]/_}_${port}.log"

    echo "🚀 Starting ramallama server for $model on port $port..."

    # Start ramallama server in background
    ramallama serve "$model" --port "$port" --host 0.0.0.0 > "$log_file" 2>&1 &
    local pid=$!
    echo "$pid" > "ramallama_${port}.pid"

    echo "   PID: $pid"
    echo "   Logs: $log_file"

    # Wait for server to be ready
    if ! wait_for_server "http://localhost:$port" "$model"; then
        echo "❌ Failed to start ramallama server for $model"
        return 1
    fi

    return 0
}

# Function to start vllm server
start_vllm_server() {
    local model=$1
    local port=$2
    local log_file="vllm_${model//[:\/]/_}_${port}.log"

    echo "🚀 Starting vLLM server for $model on port $port..."

    # Start vLLM server in background
    python -m vllm.entrypoints.openai.api_server \
        --model "$model" \
        --port "$port" \
        --host 0.0.0.0 \
        --served-model-name "$model" > "$log_file" 2>&1 &
    local pid=$!
    echo "$pid" > "vllm_${port}.pid"

    echo "   PID: $pid"
    echo "   Logs: $log_file"

    # Wait for server to be ready
    if ! wait_for_server "http://localhost:$port" "$model"; then
        echo "❌ Failed to start vLLM server for $model"
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

# Main setup function
main() {
    echo "==============================================="
    echo "🔧 Model Comparison Infrastructure Setup"
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
    echo "   • Granite 3.1 Dense:8b    → ramallama → localhost:$GRANITE_PORT"
    echo "   • DeepSeek Coder:7b       → ramallama → localhost:$DEEPSEEK_PORT"
    echo "   • Granite 3.1 Steiner:8b  → ramallama → localhost:$STEINER_PORT"
    echo ""

    # Check ports availability
    echo "🔍 Checking port availability..."
    check_port $GRANITE_PORT || { echo "❌ Port $GRANITE_PORT in use"; exit 1; }
    check_port $DEEPSEEK_PORT || { echo "❌ Port $DEEPSEEK_PORT in use"; exit 1; }
    check_port $STEINER_PORT || { echo "❌ Port $STEINER_PORT in use"; exit 1; }
    echo "✅ All ports available"

    # Install dependencies
    echo ""
    echo "📦 Installing dependencies..."
    install_ramallama

    # Download models
    echo ""
    echo "📥 Downloading models..."
    download_model "$GRANITE_MODEL" "ramallama"
    download_model "$DEEPSEEK_MODEL" "ramallama"
    download_model "$STEINER_MODEL" "ramallama"

    # Start servers
    echo ""
    echo "🚀 Starting model servers..."

    # Start Granite server
    if ! start_ramallama_server "$GRANITE_MODEL" "$GRANITE_PORT"; then
        echo "❌ Failed to start Granite server"
        exit 1
    fi

    # Start DeepSeek server
    if ! start_ramallama_server "$DEEPSEEK_MODEL" "$DEEPSEEK_PORT"; then
        echo "❌ Failed to start DeepSeek server"
        exit 1
    fi

    # Start Steiner server
    if ! start_ramallama_server "$STEINER_MODEL" "$STEINER_PORT"; then
        echo "❌ Failed to start Steiner server"
        exit 1
    fi

    echo ""
    echo "🔍 Performing health checks..."

    # Health check all servers
    check_server_health "http://localhost:$GRANITE_PORT" "$GRANITE_MODEL"
    check_server_health "http://localhost:$DEEPSEEK_PORT" "$DEEPSEEK_MODEL"
    check_server_health "http://localhost:$STEINER_PORT" "$STEINER_MODEL"

    echo ""
    echo "✅ All model servers are running and healthy!"
    echo ""
    echo "📊 Ready for model comparison testing:"
    echo "   granite3.1-dense:8b    → http://localhost:$GRANITE_PORT"
    echo "   deepseek-coder:7b      → http://localhost:$DEEPSEEK_PORT"
    echo "   granite3.1-steiner:8b  → http://localhost:$STEINER_PORT"
    echo ""
    echo "🧪 To run the model comparison tests:"
    echo "   cd .."
    echo "   go test ./test/integration/model_comparison -v"
    echo ""
    echo "🛑 To stop all servers:"
    echo "   ./scripts/stop_model_comparison.sh"
}

# Cleanup function for script interruption
cleanup() {
    echo ""
    echo "🛑 Setup interrupted. Cleaning up..."
    pkill -f "ramallama serve" || true
    pkill -f "vllm.entrypoints.openai.api_server" || true
    exit 1
}

trap cleanup INT TERM

# Run main function
main "$@"
