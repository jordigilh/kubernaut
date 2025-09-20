# HolmesGPT Quick Start Guide

This guide provides the fastest way to get HolmesGPT running with Kubernaut for testing and development.

## Prerequisites

- **Local LLM**: LocalAI/Ramalama running at `192.168.1.169:8080`
- **Podman**: For local container deployment
- **Kubernetes cluster**: For e2e testing
- **Helm**: For Kubernetes deployment

## Quick Start Commands

### Local Development (Recommended)

```bash
# 1. Start HolmesGPT container
./scripts/run-holmesgpt-local.sh

# 2. Run integration tests
./scripts/test-holmesgpt-integration.sh

# 3. Test the direct integration
./scripts/test-holmesgpt-integration.sh

# 4. Test with Go application
# The Go application will automatically use HolmesGPT for investigations
# when ai_services.holmesgpt is enabled in configuration
```

### Kubernetes E2E Testing

```bash
# 1. Deploy HolmesGPT to cluster
./scripts/deploy-holmesgpt-e2e.sh

# 2. Run e2e test suite
./scripts/e2e-test-holmesgpt.sh

# 3. Access HolmesGPT (in another terminal)
kubectl port-forward -n kubernaut-system svc/holmesgpt-e2e 8090:8090
```

## Troubleshooting

### Local Issues
```bash
# Check container status
podman ps | grep holmesgpt

# View logs
podman logs holmesgpt-local -f

# Test LLM connectivity
curl http://192.168.1.169:8080/v1/models
```

### Kubernetes Issues
```bash
# Check pod status
kubectl get pods -n kubernaut-system -l app.kubernetes.io/name=holmesgpt

# View logs
kubectl logs -n kubernaut-system -l app.kubernetes.io/name=holmesgpt -f

# Test service
kubectl run debug --image=curlimages/curl -it --rm -- \
  curl http://holmesgpt-e2e.kubernaut-system.svc.cluster.local:8090/health
```

## Next Steps

- See [HOLMESGPT_DEPLOYMENT.md](HOLMESGPT_DEPLOYMENT.md) for detailed configuration
- Review integration with your existing Python API
- Configure monitoring and observability
- Set up CI/CD pipelines with the scripts
- Explore advanced configuration options in config files

## Useful URLs

- **Local HolmesGPT**: http://localhost:8090
# Python API removed - using direct Go integration
- **API Documentation**: http://localhost:8090/docs (after starting)
- **Local LLM**: http://192.168.1.169:8080
