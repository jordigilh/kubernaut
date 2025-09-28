# Container Registry Standards

## 📦 **Registry Configuration**

### Base Registry
**Primary Registry**: `quay.io/jordigilh/`

All Kubernaut container images are hosted on Quay.io under the `jordigilh` organization.

### Image Naming Convention
```
quay.io/jordigilh/{service-name}:{version}
```

## 🏗️ **Base Images Strategy**

### Official Base Images (Used in Dockerfiles)
All services use official upstream base images for building:

| Base Image | Purpose | Use Case |
|------------|---------|----------|
| `registry.access.redhat.com/ubi9/go-toolset:1.24` | Go build environment | Enterprise services (preferred) |
| `registry.access.redhat.com/ubi9/ubi-minimal:latest` | Runtime environment | Enterprise services (preferred) |
| `golang:1.23-alpine` | Go build environment | Minimal services |
| `alpine:latest` | Minimal runtime | Lightweight services |

### Service Images - Approved Microservices Architecture
| Service | Image | Latest Version | Single Responsibility |
|---------|-------|----------------|----------------------|
| **Gateway Service** | `quay.io/jordigilh/gateway-service` | `v1.0.0` | HTTP Gateway & Security Only |
| **Alert Processor Service** | `quay.io/jordigilh/alert-service` | `v1.0.0` | Alert Processing Only |
| **AI Analysis Service** | `quay.io/jordigilh/ai-service` | `v1.0.0` | AI Analysis & Decision Making Only |
| **Workflow Orchestrator Service** | `quay.io/jordigilh/workflow-service` | `v1.0.0` | Workflow Execution Only |
| **Kubernetes Executor Service** | `quay.io/jordigilh/executor-service` | `v1.0.0` | K8s Operations Only |
| **Data Storage Service** | `quay.io/jordigilh/storage-service` | `v1.0.0` | Data Persistence Only |
| **Intelligence Service** | `quay.io/jordigilh/intelligence-service` | `v1.0.0` | Pattern Discovery Only |
| **Effectiveness Monitor Service** | `quay.io/jordigilh/monitor-service` | `v1.0.0` | Effectiveness Assessment Only |
| **Context API Service** | `quay.io/jordigilh/context-service` | `v1.0.0` | Context Orchestration Only |
| **Notification Service** | `quay.io/jordigilh/notification-service` | `v1.0.0` | Notifications Only |

## 🚀 **Build and Deployment**

### Building Images
```bash
# Build service image
podman build -t quay.io/jordigilh/{service-name}:v1.0.0 .

# Build with specific Dockerfile
podman build -t quay.io/jordigilh/webhook-service:v1.0.0 -f podman/webhook-service.Dockerfile .

# Development build
podman build -t quay.io/jordigilh/{service-name}:dev .
```

### Pushing Images
```bash
# Login to Quay.io
podman login quay.io

# Push versioned image
podman push quay.io/jordigilh/{service-name}:v1.0.0

# Push development image
podman push quay.io/jordigilh/{service-name}:dev
```

### Pulling Images
```bash
# Pull specific version
podman pull quay.io/jordigilh/{service-name}:v1.0.0

# Pull latest
podman pull quay.io/jordigilh/{service-name}:latest
```

## 📋 **Version Management**

### Versioning Strategy
- **Production**: Semantic versioning (`v1.0.0`, `v1.0.1`, `v1.1.0`)
- **Development**: Branch-based (`dev`, `main-latest`, `pr-123`)
- **Environment**: Environment-specific (`staging-v1.0.0`, `prod-v1.0.0`)

### Tag Examples
```bash
# Production releases
quay.io/jordigilh/webhook-service:v1.2.3
quay.io/jordigilh/webhook-service:v1.2
quay.io/jordigilh/webhook-service:v1
quay.io/jordigilh/webhook-service:latest

# Development builds
quay.io/jordigilh/webhook-service:dev
quay.io/jordigilh/webhook-service:main-abc123f
quay.io/jordigilh/webhook-service:pr-456

# Environment-specific
quay.io/jordigilh/webhook-service:staging-v1.2.3
quay.io/jordigilh/webhook-service:prod-v1.2.3
```

## 🔒 **Security and Access**

### Image Scanning
All images are automatically scanned for vulnerabilities:
```bash
# Manual security scan
podman scan quay.io/jordigilh/{service-name}:v1.0.0

# Trivy scan
trivy image quay.io/jordigilh/{service-name}:v1.0.0
```

### Access Control
- **Public Images**: Available for pull without authentication
- **Private Images**: Require Quay.io authentication
- **CI/CD**: Uses service account tokens

### Security Standards
- All images run as non-root user (UID 1001)
- Minimal attack surface with distroless/minimal base images
- Regular security updates and vulnerability patching
- Image signing and verification (planned)

## 🛠️ **Development Workflow**

### Local Development
```bash
# Build local image
podman build -t quay.io/jordigilh/{service-name}:dev .

# Run locally
podman run -p 8080:8080 quay.io/jordigilh/{service-name}:dev

# Test with podman-compose
podman-compose up --build
```

### CI/CD Integration
```yaml
# GitHub Actions example
- name: Build and push image
  run: |
    IMAGE_TAG="quay.io/jordigilh/${SERVICE_NAME}:${VERSION}"
    podman build -t ${IMAGE_TAG} .
    podman push ${IMAGE_TAG}
```

## 📊 **Monitoring and Metrics**

### Image Metrics
Track the following metrics for all images:
- Image size and layer count
- Build time and frequency
- Pull count and usage patterns
- Vulnerability count and severity
- Update frequency and patch status

### Registry Health
- Monitor registry availability and performance
- Track image push/pull success rates
- Alert on security vulnerabilities
- Monitor storage usage and cleanup policies

## 🔧 **Troubleshooting**

### Common Issues

**Image Pull Errors**
```bash
# Check image exists
podman manifest inspect quay.io/jordigilh/{service-name}:v1.0.0

# Verify authentication
podman login quay.io
```

**Build Failures**
```bash
# Check base image availability
podman pull quay.io/jordigilh/kubernaut-go-builder:1.24

# Verify Dockerfile syntax
podman build --no-cache -t test .
```

**Size Optimization**
```bash
# Analyze image layers
podman history quay.io/jordigilh/{service-name}:v1.0.0

# Use dive for detailed analysis
dive quay.io/jordigilh/{service-name}:v1.0.0
```

## 📚 **References**

- [Container Deployment Standards](../../.cursor/rules/10-container-deployment-standards.mdc)
- [Kubernetes Safety Guidelines](../../.cursor/rules/05-kubernetes-safety.mdc)
- [Quay.io Documentation](https://docs.quay.io/)
- [Docker Best Practices](https://docs.podman.com/develop/dev-best-practices/)

## 🔄 **Migration Guide**

### Migrating from Other Registries
If migrating from other registries (podman.io, gcr.io, etc.):

1. **Update Dockerfiles**: Change FROM statements to use `quay.io/jordigilh/` base images
2. **Update Deployments**: Change image references in Kubernetes manifests
3. **Update CI/CD**: Update build and push commands to use new registry
4. **Test Thoroughly**: Verify all services work with new images
5. **Update Documentation**: Update all references to old image names

### Rollback Plan
In case of issues with new images:
1. Keep previous images available as backup
2. Update deployments to use previous image tags
3. Investigate and fix issues with new images
4. Re-deploy with fixed images

---

**Last Updated**: September 27, 2025
**Maintained By**: Kubernaut Team
**Registry**: quay.io/jordigilh/
