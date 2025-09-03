#!/bin/bash
set -e

# HolmesGPT REST API Development Startup Script

echo "🚀 Starting HolmesGPT REST API Development Environment"

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

# Check if we're in the right directory
if [ ! -f "app/main.py" ]; then
    print_error "app/main.py not found. Please run this script from the python-api directory."
    exit 1
fi

# Check if Python 3.11+ is available
if ! python3 --version | grep -q "Python 3.1[1-9]"; then
    print_warning "Python 3.11+ recommended. Current version: $(python3 --version)"
fi

# Create virtual environment if it doesn't exist
if [ ! -d "venv" ]; then
    print_status "Creating Python virtual environment..."
    python3 -m venv venv
    print_success "Virtual environment created"
fi

# Activate virtual environment
print_status "Activating virtual environment..."
source venv/bin/activate

# Upgrade pip
print_status "Upgrading pip..."
pip install --upgrade pip

# Install dependencies
print_status "Installing dependencies..."
pip install -r requirements.txt

# Set up environment file if it doesn't exist
if [ ! -f ".env" ]; then
    print_status "Setting up environment configuration..."
    cp env.example .env
    print_warning "Please review and customize .env file for your environment"
fi

# Check if HolmesGPT CLI is available
if command -v holmes &> /dev/null; then
    print_success "HolmesGPT CLI found: $(holmes --version)"
else
    print_warning "HolmesGPT CLI not found. API will use mock mode."
    print_status "To install HolmesGPT CLI, visit: https://github.com/robusta-dev/holmesgpt"
fi

# Check if Ollama is running
if curl -s http://localhost:11434/api/version > /dev/null 2>&1; then
    print_success "Ollama service is running"
else
    print_warning "Ollama service not detected. Starting with docker-compose..."
    if command -v docker-compose &> /dev/null; then
        docker-compose up -d ollama
        print_status "Waiting for Ollama to start..."
        sleep 10

        # Check if model needs to be pulled
        if ! docker-compose exec ollama ollama list | grep -q "gpt-oss:20b"; then
            print_status "Pulling gpt-oss:20b model (this may take a while)..."
            docker-compose exec ollama ollama pull gpt-oss:20b || {
                print_warning "Failed to pull model. You can pull it manually later:"
                echo "  docker-compose exec ollama ollama pull gpt-oss:20b"
            }
        fi
    else
        print_error "Docker Compose not found. Please install Docker and Docker Compose."
        exit 1
    fi
fi

# Run tests to verify setup
print_status "Running basic tests..."
python -m pytest tests/test_api.py::TestHealthEndpoints::test_root_endpoint -v || {
    print_warning "Some tests failed, but this is expected without full HolmesGPT setup"
}

# Start the development server
print_success "🎉 Setup complete! Starting development server..."
echo ""
echo "📋 Available Services:"
echo "  • API Server:          http://localhost:8000"
echo "  • Interactive Docs:    http://localhost:8000/docs"
echo "  • ReDoc:               http://localhost:8000/redoc"
echo "  • Health Check:        http://localhost:8000/health"
echo "  • Metrics:             http://localhost:9090/metrics"
echo ""
echo "🔧 Development Commands:"
echo "  • Run tests:           python -m pytest tests/ -v"
echo "  • Format code:         black app/ tests/ && isort app/ tests/"
echo "  • Lint code:           flake8 app/ tests/ && mypy app/"
echo "  • View logs:           docker-compose logs -f"
echo ""
echo "🚀 Starting server..."
echo "   Press Ctrl+C to stop"
echo ""

# Start the FastAPI development server
uvicorn app.main:app --host 0.0.0.0 --port 8000 --reload

