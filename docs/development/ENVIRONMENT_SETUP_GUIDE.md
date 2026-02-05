# Environment Setup Guide

**Date**: October 9, 2025
**Version**: 1.0
**Audience**: Developers setting up local Kubernaut development environment

---

## Overview

This guide walks you through setting up your local development environment for Kubernaut, including all required services (databases, LLM, Kubernetes, etc.).

**Quick Links**:
- [.env.example](../../.env.example) - Comprehensive environment template
- [config.app/development.yaml](../../config.app/development.yaml) - Application configuration
- [Makefile](../../Makefile) - Build and development commands

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Quick Start (5 minutes)](#quick-start-5-minutes)
3. [Detailed Setup](#detailed-setup)
4. [Service Configuration](#service-configuration)
5. [Verification](#verification)
6. [Troubleshooting](#troubleshooting)
7. [Advanced Configuration](#advanced-configuration)

---

## Prerequisites

### Required Software

| Tool | Version | Purpose | Installation |
|------|---------|---------|--------------|
| **Go** | 1.23+ | Primary language | [golang.org/dl](https://golang.org/dl/) |
| **Docker** or **Podman** | Latest | Container runtime | [docker.com](https://docker.com) or [podman.io](https://podman.io) |
| **KIND** | v0.30.0 | Local Kubernetes | `go install sigs.k8s.io/kind@v0.30.0` |
| **kubectl** | 1.28+ | Kubernetes CLI | [kubernetes.io/docs/tasks/tools](https://kubernetes.io/docs/tasks/tools/) |
| **make** | 3.81+ | Build automation | Usually pre-installed on Linux/macOS |

**Note**: KIND v0.30.0 is the tested and recommended version. E2E tests use Kubernetes v1.27.3 (kindest/node:v1.27.3) for stability with Podman.

### Optional Tools

| Tool | Purpose | Installation |
|------|---------|--------------|
| **direnv** | Auto-load environment variables | [direnv.net](https://direnv.net/) |
| **golangci-lint** | Code linting | [golangci-lint.run](https://golangci-lint.run/usage/install/) |
| **ramalama** or **ollama** | Local LLM runtime | See [LLM Setup](#llm-configuration) |

---

## Quick Start (5 minutes)

For experienced developers who want to get running quickly:

```bash
# 1. Clone repository
git clone https://github.com/jordigilh/kubernaut.git
cd kubernaut

# 2. Create your environment file
cp .env.example .env.development

# 3. Edit .env.development and set your passwords
# At minimum, change these variables:
#   - DB_PASSWORD
#   - VECTOR_DB_PASSWORD
#   - REDIS_PASSWORD
#   - LLM_ENDPOINT (if using external LLM)

# 4. Source your environment
source .env.development

# 5. Bootstrap development environment
make bootstrap-dev

# 6. Verify setup
make dev-status

# 7. Run tests
make test
```

**Done!** If all steps succeed, you're ready to develop.

If you encounter issues, see the [Detailed Setup](#detailed-setup) section below.

---

## Detailed Setup

### Step 1: Clone and Navigate

```bash
# Clone the repository
git clone https://github.com/jordigilh/kubernaut.git
cd kubernaut

# Verify you're in the right place
ls -la | grep -E "(Makefile|go.mod|cmd|pkg)"
```

You should see the main project files.

---

### Step 2: Create Environment Configuration

Kubernaut uses `.env` files for local development configuration.

#### Option A: Using .env.example (Recommended)

```bash
# Copy the comprehensive template
cp .env.example .env.development

# Edit with your preferred editor
vim .env.development  # or code, nano, etc.
```

**Required Changes**:

```bash
# At minimum, update these passwords
export DB_PASSWORD=your_secure_db_password_here
export VECTOR_DB_PASSWORD=your_vector_db_password_here
export REDIS_PASSWORD=your_redis_password_here
```

**Optional Changes** (based on your setup):

```bash
# If using external LLM service
export LLM_ENDPOINT=http://192.168.1.100:8080
export LLM_MODEL=your-model-name

# If using specific Kubernetes context
export KUBE_CONTEXT=your-k8s-context
export KUBECONFIG=/path/to/your/kubeconfig
```

#### Option B: Using direnv (Advanced)

If you prefer automatic environment loading:

```bash
# Install direnv (if not already installed)
# macOS: brew install direnv
# Linux: apt-get install direnv  or  yum install direnv

# Create .envrc file
cat > .envrc << 'EOF'
# Load environment from .env.development
dotenv .env.development

# Optional: Add custom overrides
export DEBUG_MODE=true
export LOG_LEVEL=debug
EOF

# Allow direnv to load
direnv allow

# Environment will auto-load when you cd into the directory
```

---

### Step 3: Source Environment Variables

```bash
# Load environment variables into your current shell
source .env.development

# Verify key variables are set
echo "DB_HOST: $DB_HOST"
echo "LLM_ENDPOINT: $LLM_ENDPOINT"
echo "CLUSTER_NAME: $CLUSTER_NAME"
```

**Expected Output**:
```
DB_HOST: localhost
LLM_ENDPOINT: http://localhost:8010
CLUSTER_NAME: kubernaut-dev
```

---

### Step 4: Bootstrap Development Environment

The `bootstrap-dev` target sets up all required infrastructure:

```bash
# Run the bootstrap script
make bootstrap-dev
```

**What this does**:
1. Creates KIND cluster (`kubernaut-dev`)
2. Starts PostgreSQL container (port 5433)
3. Starts Vector DB container (port 5434)
4. Starts Redis container (port 6380)
5. Applies database migrations
6. Installs Kubernetes CRDs
7. Sets up monitoring stack (optional)

**Expected Output**:
```
✅ KIND cluster created: kubernaut-dev
✅ PostgreSQL started on localhost:5433
✅ Vector DB started on localhost:5434
✅ Redis started on localhost:6380
✅ Database migrations applied
✅ CRDs installed
✅ Development environment ready!
```

**Duration**: 3-5 minutes on first run

---

### Step 5: Verify Setup

Check that all services are running correctly:

```bash
# Check development environment status
make dev-status
```

**Expected Output**:
```
Checking development environment...

✅ Go version: go1.23.1
✅ Docker/Podman: running
✅ KIND cluster: kubernaut-dev (running)
✅ PostgreSQL: localhost:5433 (connected)
✅ Vector DB: localhost:5434 (connected)
✅ Redis: localhost:6380 (connected)
✅ LLM service: http://localhost:8010 (reachable)
⚠️  HolmesGPT: http://localhost:8090 (not started - optional)

Development environment is READY! ✅
```

---

### Step 6: Run Tests

Verify everything works by running the test suite:

```bash
# Run unit tests (fast, ~30s)
make test

# Run integration tests (slower, ~2-3 minutes)
make test-integration

# Run all tests
make test-all
```

**Expected Results**:
- Unit tests: All passing (70%+ coverage target)
- Integration tests: All passing (requires real infrastructure)

---

## Service Configuration

### Database Configuration

**PostgreSQL** (Action History):
- **Host**: `localhost`
- **Port**: `5433` (non-default to avoid conflicts)
- **Database**: `action_history`
- **User**: `slm_user`
- **Password**: Set in `.env.development`

**Verify Connection**:
```bash
# Using psql
psql -h localhost -p 5433 -U slm_user -d action_history

# Using Docker/Podman
docker exec -it kubernaut-postgres psql -U slm_user -d action_history
```

**Apply Migrations**:
```bash
# Migrations are in migrations/ directory
make db-migrate
```

---

### Vector Database Configuration

**pgvector** (Embedding Storage):
- **Host**: `localhost`
- **Port**: `5434`
- **Database**: `vector_store`
- **User**: `vector_user`
- **Password**: Set in `.env.development`

**Verify Connection**:
```bash
psql -h localhost -p 5434 -U vector_user -d vector_store
```

**Check Extension**:
```sql
-- Inside psql
\dx
-- Should show 'vector' extension
```

---

### Redis Configuration

**Redis** (Caching):
- **Host**: `localhost`
- **Port**: `6380`
- **Password**: Set in `.env.development`

**Verify Connection**:
```bash
# Using redis-cli
redis-cli -p 6380 -a "$REDIS_PASSWORD" PING
# Should return: PONG

# Using Docker/Podman
docker exec -it kubernaut-redis redis-cli -a "$REDIS_PASSWORD" PING
```

---

### LLM Configuration

**Options**:

#### Option 1: Local LLM (ramalama/ollama)

```bash
# Install ramalama (macOS)
brew install ramalama

# Pull model
ramalama pull oss-gpt:20b

# Start server
ramalama serve oss-gpt:20b --port 8010

# Verify
curl http://localhost:8010/v1/models
```

#### Option 2: External LLM Service

```bash
# Update .env.development
export LLM_ENDPOINT=http://192.168.1.169:8080
export LLM_PROVIDER=ollama
export LLM_MODEL=your-model-name
```

#### Option 3: Mock LLM (Testing Only)

```bash
# Update .env.development
export USE_MOCK_LLM=true
```

**Verify LLM Access**:
```bash
# Test endpoint
curl -X POST http://localhost:8010/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "oss-gpt:20b", "messages": [{"role": "user", "content": "Hello"}]}'
```

---

### Kubernetes Configuration

**KIND Cluster** (Development):

```bash
# Create cluster (done by bootstrap-dev)
kind create cluster --name kubernaut-dev --config test/kind/kind-config.yaml

# Get kubeconfig
kind get kubeconfig --name kubernaut-dev > ~/.kube/kind-kubernaut-dev

# Set context
export KUBECONFIG=~/.kube/kind-kubernaut-dev
export KUBE_CONTEXT=kind-kubernaut-dev

# Verify
kubectl cluster-info --context kind-kubernaut-dev
```

**Using Existing Cluster**:

```bash
# Update .env.development
export KUBECONFIG=/path/to/your/kubeconfig
export KUBE_CONTEXT=your-context-name
export CLUSTER_NAME=your-cluster

# Verify access
kubectl config use-context $KUBE_CONTEXT
kubectl get nodes
```

---

## Verification

### Comprehensive Verification Checklist

```bash
# 1. Environment variables loaded
env | grep -E "DB_HOST|LLM_ENDPOINT|CLUSTER_NAME"

# 2. Database connectivity
psql -h localhost -p 5433 -U slm_user -d action_history -c "SELECT 1;"

# 3. Vector DB connectivity
psql -h localhost -p 5434 -U vector_user -d vector_store -c "SELECT 1;"

# 4. Redis connectivity
redis-cli -p 6380 -a "$REDIS_PASSWORD" PING

# 5. LLM service
curl -s http://localhost:8010/v1/models | jq .

# 6. Kubernetes cluster
kubectl cluster-info

# 7. CRDs installed
kubectl get crds | grep -E "remediation|workflow|aianalysis"

# 8. Run unit tests
make test

# 9. Run integration tests
make test-integration
```

**All checks passing?** ✅ You're ready to develop!

---

## Troubleshooting

### Common Issues and Solutions

#### Issue 1: Database Connection Refused

**Symptoms**:
```
Error: connection refused to localhost:5433
```

**Solutions**:
```bash
# Check if PostgreSQL container is running
docker ps | grep postgres

# Check if port is already in use
lsof -i :5433

# Restart PostgreSQL
docker restart kubernaut-postgres

# Check logs
docker logs kubernaut-postgres

# Try different port (update .env.development)
export DB_PORT=5435
```

---

#### Issue 2: LLM Service Unavailable

**Symptoms**:
```
Error: failed to connect to LLM at http://localhost:8010
```

**Solutions**:
```bash
# Check if LLM service is running
curl http://localhost:8010/v1/models

# Check firewall/network
telnet localhost 8010

# Use mock LLM for testing
export USE_MOCK_LLM=true

# Check if external LLM is reachable
curl http://192.168.1.169:8080/v1/models
```

---

#### Issue 3: KIND Cluster Not Found

**Symptoms**:
```
Error: context "kind-kubernaut-dev" not found
```

**Solutions**:
```bash
# List KIND clusters
kind get clusters

# Create cluster
kind create cluster --name kubernaut-dev

# Get kubeconfig
kind get kubeconfig --name kubernaut-dev

# Set context
kubectl config use-context kind-kubernaut-dev

# Update .env.development
export KUBE_CONTEXT=kind-kubernaut-dev
```

---

#### Issue 4: Environment Variables Not Set

**Symptoms**:
```
Error: DB_HOST environment variable not set
```

**Solutions**:
```bash
# Ensure you sourced the file
source .env.development

# Check current shell
echo $SHELL

# For fish/zsh, syntax may differ
# Use direnv for auto-loading (see Advanced Configuration)

# Verify variables
env | grep -E "DB_|LLM_|REDIS_"
```

---

#### Issue 5: Port Already in Use

**Symptoms**:
```
Error: bind: address already in use (port 5433)
```

**Solutions**:
```bash
# Find process using port
lsof -i :5433

# Kill process (if safe)
kill -9 <PID>

# Or use different port
# Update .env.development
export DB_PORT=5435
```

---

#### Issue 6: Permission Denied

**Symptoms**:
```
Error: permission denied while trying to connect to Docker socket
```

**Solutions**:
```bash
# Add user to docker group (Linux)
sudo usermod -aG docker $USER
newgrp docker

# macOS: Check Docker Desktop is running

# Podman: Check socket
systemctl --user status podman.socket

# Verify
docker ps
```

---

## Advanced Configuration

### Using Custom Configuration Files

You can override default configurations:

```bash
# Custom application config
export CONFIG_FILE=config.app/my-custom-config.yaml

# Custom YAML config takes precedence over .env
# See: config.app/development.yaml for structure
```

---

### Multiple Environments

Manage multiple development environments:

```bash
# Create environment-specific files
cp .env.example .env.development
cp .env.example .env.staging
cp .env.example .env.production-readonly

# Load specific environment
source .env.staging

# Or use environment variable
export ENV=staging
# (Requires custom shell script to load .env.$ENV)
```

---

### Secret Management Integration

#### Using direnv

```bash
# .envrc (gitignored)
# Load base configuration
dotenv .env.development

# Override with secrets from secure location
dotenv_if_exists ~/.kubernaut/secrets.env

# Or fetch from 1Password
export DB_PASSWORD=$(op read "op://Development/kubernaut-db/password")
```

#### Using 1Password CLI

```bash
# Install 1Password CLI
brew install --cask 1password-cli

# Sign in
eval $(op signin)

# Add to .envrc or shell script
export DB_PASSWORD=$(op read "op://Development/kubernaut-db/password")
export REDIS_PASSWORD=$(op read "op://Development/kubernaut-redis/password")
```

---

### CI/CD Environment Setup

For GitHub Actions / GitLab CI:

```yaml
# .github/workflows/test.yml
env:
  USE_MOCK_LLM: true
  USE_FAKE_K8S_CLIENT: true
  CI: true
  DB_PASSWORD: ${{ secrets.DB_PASSWORD }}
  REDIS_PASSWORD: ${{ secrets.REDIS_PASSWORD }}
```

---

### Performance Tuning

Optimize for your workload:

```bash
# Database connection pooling
export DB_MAX_OPEN_CONNS=50
export DB_MAX_IDLE_CONNS=10
export DB_CONN_MAX_LIFETIME=10m

# Redis connection pooling
export REDIS_POOL_SIZE=20
export REDIS_MAX_RETRIES=5

# LLM timeouts
export LLM_TIMEOUT=120s
export HOLMESGPT_TIMEOUT=300s

# Test timeouts
export TEST_TIMEOUT=300s
export SKIP_SLOW_TESTS=true  # Skip slow tests
```

---

## Related Documentation

### Project Documentation
- [NEXT_SESSION_GUIDE.md](../NEXT_SESSION_GUIDE.md) - Main development guide
- [Architecture Overview](../architecture/KUBERNAUT_ARCHITECTURE_OVERVIEW.md) - System architecture

### Configuration Files
- [.env.example](../../.env.example) - Environment template
- [config.app/development.yaml](../../config.app/development.yaml) - Application config
- [Makefile](../../Makefile) - Build targets

### External Resources
- [12-Factor App - Config](https://12factor.net/config) - Configuration best practices
- [direnv Documentation](https://direnv.net/) - Environment management
- [KIND Documentation](https://kind.sigs.k8s.io/) - Local Kubernetes

---

## Quick Reference

### Essential Commands

```bash
# Environment setup
cp .env.example .env.development
vim .env.development
source .env.development

# Bootstrap
make bootstrap-dev

# Development
make build           # Build binaries
make test            # Run unit tests
make test-integration # Run integration tests
make lint            # Run linters
make fmt             # Format code

# Infrastructure
make dev-status      # Check environment status
make db-migrate      # Apply database migrations
make cleanup-dev     # Clean up environment

# Kubernetes
kubectl config use-context kind-kubernaut-dev
kubectl get pods -A
kubectl logs -f <pod-name>

# Database
psql -h localhost -p 5433 -U slm_user -d action_history
redis-cli -p 6380 -a "$REDIS_PASSWORD"

# LLM
curl http://localhost:8010/v1/models
```

---

## Summary

**Setup Time**: 5-15 minutes (depending on downloads)

**Core Steps**:
1. ✅ Copy `.env.example` to `.env.development`
2. ✅ Update passwords and service endpoints
3. ✅ Source environment: `source .env.development`
4. ✅ Bootstrap infrastructure: `make bootstrap-dev`
5. ✅ Verify setup: `make dev-status`
6. ✅ Run tests: `make test`

**You're ready to develop!** 🚀

For questions or issues, see:
- [Troubleshooting](#troubleshooting) section above
- Project documentation in `docs/`
- GitHub Issues: [github.com/jordigilh/kubernaut/issues](https://github.com/jordigilh/kubernaut/issues)

---

**Last Updated**: October 9, 2025
**Version**: 1.0
**Maintainer**: Kubernaut Development Team

