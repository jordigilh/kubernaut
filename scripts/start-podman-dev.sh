#!/bin/bash
set -e

# HolmesGPT Podman Development Environment Startup Script

echo "🚀 Starting HolmesGPT Podman Development Environment"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if podman is installed
if ! command -v podman &> /dev/null; then
    print_error "Podman is not installed. Please install podman first."
    echo "  macOS: brew install podman"
    echo "  Linux: sudo apt-get install podman (Ubuntu/Debian) or sudo dnf install podman (RHEL/Fedora)"
    exit 1
fi

# Check if podman-compose is installed
if ! command -v podman-compose &> /dev/null; then
    print_warning "podman-compose is not installed. Installing via pip..."
    pip3 install podman-compose || {
        print_error "Failed to install podman-compose. Please install it manually:"
        echo "  pip3 install podman-compose"
        exit 1
    }
fi

# Check if we're in the right directory
if [ ! -f "podman-compose.yml" ]; then
    print_error "podman-compose.yml not found. Please run this script from the project root directory."
    exit 1
fi

print_status "Checking Podman status..."
podman --version

# Start Podman machine if on macOS
if [[ "$OSTYPE" == "darwin"* ]]; then
    print_status "Checking Podman machine status (macOS)..."
    if ! podman machine list | grep -q "Currently running"; then
        print_status "Starting Podman machine..."
        podman machine start || {
            print_warning "Podman machine not initialized. Initializing..."
            podman machine init
            podman machine start
        }
    fi
    print_success "Podman machine is running"
fi

# Function to wait for service to be healthy
wait_for_service() {
    local service_name=$1
    local max_attempts=30
    local attempt=1

    print_status "Waiting for $service_name to be healthy..."

    while [ $attempt -le $max_attempts ]; do
        if podman-compose ps | grep -q "$service_name.*healthy"; then
            print_success "$service_name is healthy"
            return 0
        fi

        echo -n "."
        sleep 2
        ((attempt++))
    done

    print_warning "$service_name did not become healthy within expected time"
    return 1
}

# Stop any existing containers
print_status "Stopping any existing containers..."
podman-compose down 2>/dev/null || true

# Start the development environment
print_status "Starting development environment..."
podman-compose up -d

# Wait for services to be healthy
wait_for_service "ollama"

# Pull the required model if not already present
print_status "Checking if gpt-oss:20b model is available..."
if ! podman exec dev-ollama ollama list | grep -q "gpt-oss:20b"; then
    print_status "Pulling gpt-oss:20b model (this may take a while)..."
    podman exec dev-ollama ollama pull gpt-oss:20b || {
        print_warning "Failed to pull gpt-oss:20b model. You can pull it manually later:"
        echo "  podman exec dev-ollama ollama pull gpt-oss:20b"
    }
else
    print_success "gpt-oss:20b model is available"
fi

wait_for_service "holmesgpt"

# Test HolmesGPT
print_status "Testing HolmesGPT connectivity..."
if podman exec dev-holmesgpt holmes --version > /dev/null 2>&1; then
    print_success "HolmesGPT is working"

    # Run a simple test
    print_status "Running a simple test query..."
    podman exec dev-holmesgpt holmes ask "Hello, are you working?" --max-tokens 50 || {
        print_warning "Test query failed (this may be expected if the model is still loading)"
    }
else
    print_warning "HolmesGPT test failed"
fi

# Display service status
echo ""
print_status "Development environment status:"
podman-compose ps

echo ""
print_success "🎉 HolmesGPT Podman Development Environment is ready!"
echo ""
echo "📋 Available Services:"
echo "  • Ollama (LLM):        http://localhost:11434"
echo "  • HolmesGPT:           Container: dev-holmesgpt"
echo "  • Prometheus:          http://localhost:9090"
echo "  • AlertManager:        http://localhost:9093"
echo ""
echo "🔧 Development Commands:"
echo "  • View logs:           podman-compose logs -f [service]"
echo "  • Execute in container: podman exec -it dev-holmesgpt bash"
echo "  • Test HolmesGPT:      podman exec dev-holmesgpt holmes ask 'test query'"
echo "  • Stop environment:    podman-compose down"
echo "  • Restart service:     podman-compose restart [service]"
echo ""
echo "🚀 Start your application:"
echo "  go run ./cmd/server --config config/development.yaml"
echo ""
echo "📊 Monitor with:"
echo "  podman-compose logs -f"

