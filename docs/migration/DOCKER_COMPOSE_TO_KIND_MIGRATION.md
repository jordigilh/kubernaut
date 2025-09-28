# Migration Guide: Docker Compose to Kind Cluster

## Overview

This guide helps you migrate from the deprecated docker-compose development setup to the new Kind cluster-based integration environment.

## Why Migrate?

### Benefits of Kind Cluster
- **Production Parity**: Real Kubernetes API, networking, and RBAC
- **Better Testing**: Authentic Kubernetes behavior validation
- **Consistent Environment**: Same tools and patterns as production
- **Enhanced Features**: Network policies, resource limits, multi-node scenarios

### Docker Compose Limitations
- Simplified networking doesn't match Kubernetes
- Cannot test Kubernetes-native features (RBAC, network policies)
- Different service discovery mechanisms
- No multi-node or distributed workload testing

## Migration Steps

### Step 1: Prerequisites Check

**Before migration, ensure you have:**
```bash
# Required tools
brew install kind kubectl docker

# Verify installations
kind --version
kubectl version --client
docker --version

# Ensure Docker is running
docker info
```

### Step 2: Clean Up Existing Docker Compose Environment

```bash
# Stop and remove docker-compose services
make cleanup-dev-compose

# Or manually if needed
podman-compose -f podman-compose.yml down
podman-compose -f test/integration/docker-compose.integration.yml down
```

### Step 3: Bootstrap Kind Environment

```bash
# Bootstrap new Kind-based environment
make bootstrap-dev-kind

# This will:
# - Create Kind cluster (1 control-plane + 2 workers)
# - Deploy all services as Kubernetes resources
# - Set up monitoring stack
# - Configure networking and RBAC
```

### Step 4: Verify Migration

```bash
# Check cluster status
make kind-status

# Verify services are running
kubectl get pods -n kubernaut-integration

# Test service endpoints
curl http://localhost:30800/health     # Webhook service
curl http://localhost:30090/-/ready    # Prometheus
```

## Command Mapping

### Environment Management

| Docker Compose (OLD) | Kind Cluster (NEW) | Description |
|---------------------|-------------------|-------------|
| `make bootstrap-dev-compose` | `make bootstrap-dev-kind` | Setup development environment |
| `make cleanup-dev-compose` | `make cleanup-dev-kind` | Clean up environment |
| `podman-compose up -d` | `make kind-deploy` | Start services |
| `podman-compose down` | `make kind-undeploy` | Stop services |
| `podman-compose logs` | `make kind-logs` | View service logs |
| `docker ps` | `make kind-status` | Check service status |

### Service Access

| Docker Compose (OLD) | Kind Cluster (NEW) | Service |
|---------------------|-------------------|---------|
| `http://localhost:8080` | `http://localhost:30800` | Webhook Service |
| `http://localhost:9090` | `http://localhost:30090` | Prometheus |
| `http://localhost:9093` | `http://localhost:30093` | AlertManager |
| `localhost:5433` | `localhost:30432` | PostgreSQL |

### Testing Commands

| Docker Compose (OLD) | Kind Cluster (NEW) | Description |
|---------------------|-------------------|-------------|
| `make test-integration-dev` | `make test-integration-dev` | Run integration tests (same) |
| `make test-ai-dev` | `make test-ai-dev` | Run AI tests (same) |
| `make test-quick-dev` | `make test-quick-dev` | Run quick tests (same) |

## Configuration Changes

### Environment Variables

**Old (.env.development):**
```bash
# Docker compose configuration
POSTGRES_HOST=localhost
POSTGRES_PORT=5433
PROMETHEUS_URL=http://localhost:9090
WEBHOOK_URL=http://localhost:8080
```

**New (.env.kind-integration):**
```bash
# Kind cluster configuration
KIND_CLUSTER_NAME=kubernaut-integration
KUBECTL_CONTEXT=kind-kubernaut-integration
POSTGRES_HOST=localhost
POSTGRES_PORT=30432
PROMETHEUS_URL=http://localhost:30090
WEBHOOK_URL=http://localhost:30800
USE_KIND_CLUSTER=true
```

### Service Discovery

**Docker Compose:**
- Services communicate via container names
- Simple bridge networking
- No service mesh or DNS policies

**Kind Cluster:**
- Services communicate via Kubernetes DNS
- Native service discovery (`service-name.namespace.svc.cluster.local`)
- Network policies and service mesh capabilities

## Troubleshooting Migration Issues

### Common Issues and Solutions

#### 1. Port Conflicts
**Problem:** Ports 30800, 30090, 30093, 30432 are in use
**Solution:**
```bash
# Check what's using the ports
lsof -i :30800
lsof -i :30090

# Stop conflicting services or modify Kind configuration
```

#### 2. Docker/Podman Issues
**Problem:** Kind cannot create cluster
**Solution:**
```bash
# Ensure Docker is running
docker info

# For Podman users
export KIND_EXPERIMENTAL_PROVIDER=podman
podman machine start
```

#### 3. kubectl Context Issues
**Problem:** kubectl commands fail
**Solution:**
```bash
# Verify kubectl context
kubectl config current-context

# Switch to Kind context
kubectl config use-context kind-kubernaut-integration

# List available contexts
kubectl config get-contexts
```

#### 4. Service Startup Issues
**Problem:** Services not starting in Kind cluster
**Solution:**
```bash
# Check pod status
kubectl get pods -n kubernaut-integration

# Check pod logs
kubectl logs -f deployment/webhook-service -n kubernaut-integration

# Check events
kubectl get events -n kubernaut-integration --sort-by='.lastTimestamp'
```

### Performance Considerations

#### Resource Requirements

**Docker Compose:**
- Memory: ~2-3GB
- CPU: 1-2 cores
- Startup: 30-45 seconds

**Kind Cluster:**
- Memory: ~4-6GB
- CPU: 2-4 cores
- Startup: 60-90 seconds

#### Optimization Tips

```bash
# Reduce resource usage
kubectl patch deployment webhook-service -n kubernaut-integration -p '{"spec":{"template":{"spec":{"containers":[{"name":"webhook-service","resources":{"requests":{"memory":"128Mi","cpu":"100m"}}}]}}}}'

# Use local registry for faster image loading
kind create cluster --config=test/kind/kind-config.yaml

# Pre-pull images
docker pull postgres:16
docker pull redis:7-alpine
kind load docker-image postgres:16 --name kubernaut-integration
```

## Rollback Plan

If you need to rollback to docker-compose temporarily:

```bash
# Clean up Kind environment
make cleanup-dev-kind --delete-cluster

# Restore docker-compose environment
make bootstrap-dev-compose

# Verify legacy environment
curl http://localhost:8080/health
```

## Migration Checklist

- [ ] Install required tools (kind, kubectl, docker)
- [ ] Clean up existing docker-compose environment
- [ ] Bootstrap Kind cluster environment
- [ ] Verify all services are running
- [ ] Update development scripts and documentation
- [ ] Test integration test suite
- [ ] Update CI/CD pipelines (if applicable)
- [ ] Train team on new commands and workflows

## Support and Resources

### Documentation
- [ADR-003: Kind Integration Environment](../architecture/decisions/ADR-003-KIND-INTEGRATION-ENVIRONMENT.md)
- [Kind Cluster Configuration](../../test/kind/)
- [Kubernetes Manifests](../../deploy/integration/)

### Commands Reference
```bash
# Quick reference
make dev-help                    # Show all available commands
make kind-status                 # Check cluster and service status
make kind-logs                   # View service logs
kubectl get pods -A              # List all pods
kubectl config get-contexts      # List kubectl contexts
```

### Getting Help
- Check logs: `make kind-logs`
- Cluster status: `make kind-status`
- Reset environment: `make cleanup-dev-kind --all && make bootstrap-dev-kind`

---

**Migration Timeline:** Allow 1-2 hours for complete migration including testing and verification.

**Rollback Time:** < 30 minutes if needed to return to docker-compose setup.
