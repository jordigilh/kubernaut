# ADR-028: Container Image Registry and Base Image Policy

**Status**: ✅ Accepted
**Date**: 2025-10-28
**Decision Makers**: Engineering Team
**Impact**: High (affects all services, security, compliance)
**Supersedes**: Partial update to ADR-027 (adds registry policy)

---

## Context

### Current State

ADR-027 established Red Hat UBI9 as the standard base image for Kubernaut services. However, it does not comprehensively address:

1. **Approved Container Registries**: Which registries are trusted for pulling base images?
2. **Image Versioning Strategy**: Should we use `latest`, specific versions, or version ranges?
3. **UBI Version Policy**: When should we migrate from UBI9 to UBI10?
4. **Security Scanning**: How do we ensure base images are vulnerability-free?
5. **Offline/Air-Gapped Deployments**: How do we support disconnected environments?

### Problems

1. **Security Risk**: Unrestricted registry access allows untrusted image sources
2. **Compliance Gap**: No formal policy for enterprise image sourcing
3. **Version Drift**: Inconsistent use of `latest` vs. pinned versions across Dockerfiles
4. **Future-Proofing**: No clear migration path from UBI9 → UBI10
5. **Supply Chain Security**: No verification of base image authenticity

### Real-World Impact

**Current Dockerfile Analysis** (2025-10-28):
- ✅ Most services use `registry.access.redhat.com/ubi9/*`
- ⚠️ Some services use `golang:1.24-alpine` (non-Red Hat)
- ⚠️ Some services use `gcr.io/distroless/*` (Google, not Red Hat)
- ⚠️ Inconsistent version pinning (`latest` vs. `1.24` vs. `ubi9`)

---

## Decision

### **1. Approved Container Registries**

**MANDATORY**: All base images MUST be pulled from Red Hat-approved registries only.

#### **Tier 1: Primary Registry (REQUIRED)**
```
registry.access.redhat.com
```
- **Purpose**: Official Red Hat Universal Base Images (UBI)
- **Authentication**: Public access (no credentials required)
- **Support**: Full Red Hat support and security updates
- **Use Case**: All production base images

#### **Tier 2: Red Hat Ecosystem Registry (APPROVED)**
```
registry.redhat.io
```
- **Purpose**: Red Hat Certified Container Images
- **Authentication**: Requires Red Hat account
- **Support**: Full Red Hat support
- **Use Case**: Red Hat middleware, databases, certified partners

#### **Tier 3: Internal Mirror (APPROVED)**
```
quay.io/jordigilh/*
```
- **Purpose**: Kubernaut service images and mirrored base images
- **Authentication**: Public or private (per image)
- **Support**: Self-managed
- **Use Case**: Kubernaut microservices, air-gapped deployments

#### **Prohibited Registries**
❌ **FORBIDDEN**: The following registries are NOT approved for base images:
- `docker.io` (Docker Hub) - Community images, no enterprise support
- `gcr.io` (Google Container Registry) - Not Red Hat ecosystem
- `ghcr.io` (GitHub Container Registry) - Not Red Hat ecosystem
- `quay.io` (except `jordigilh/*`) - Public Quay images lack support
- `alpine` (Docker Hub shorthand) - Not Red Hat ecosystem

---

### **Image Selection Workflow** 🔍

**MANDATORY**: Before using ANY base image, follow this discovery process:

#### **Step 1: Search Red Hat Catalog (REQUIRED)**

Search the official Red Hat Container Catalog for approved images:

**Catalog URL**: https://catalog.redhat.com/software/containers/search

**Search Process**:
1. **Search by Technology**: Enter language/runtime (e.g., "go", "python", "nodejs")
2. **Filter by UBI9**: Select "Red Hat Universal Base Image 9"
3. **Check Public Access**: Verify image is available at `registry.access.redhat.com`
4. **Verify Support**: Confirm Red Hat support and security updates

**Example Searches**:
```bash
# Search for Go toolset
https://catalog.redhat.com/software/containers/search?q=go-toolset&p=1

# Search for Python
https://catalog.redhat.com/software/containers/search?q=python&p=1

# Search for Node.js
https://catalog.redhat.com/software/containers/search?q=nodejs&p=1
```

#### **Step 2: Verify Image Availability (REQUIRED)**

Test image pull from `registry.access.redhat.com`:

```bash
# Verify image exists and is publicly accessible
podman pull registry.access.redhat.com/ubi9/go-toolset:1.24

# Check image metadata
skopeo inspect docker://registry.access.redhat.com/ubi9/go-toolset:1.24
```

**Success Criteria**:
- ✅ Image pulls without authentication
- ✅ Image metadata shows Red Hat as vendor
- ✅ Image has recent update timestamp (within 90 days)
- ✅ Image supports required architectures (amd64, arm64)

#### **Step 3: Decision Point**

**If Red Hat image EXISTS** → ✅ **USE IT** (proceed to implementation)

**If Red Hat image DOES NOT EXIST** → ⚠️ **STOP AND ASK FOR INPUT**

---

### **Exception Request Process** ⚠️

**When Red Hat image is NOT available**, submit an exception request:

#### **Exception Request Template**

```markdown
## Base Image Exception Request

**Requested By**: [Your Name]
**Date**: [YYYY-MM-DD]
**Service**: [Service Name]

### 1. Technology Requirement
**Language/Runtime**: [e.g., Rust, Ruby, .NET]
**Version Required**: [e.g., Rust 1.75, Ruby 3.2]

### 2. Red Hat Catalog Search Results
**Search URL**: [Link to catalog search]
**Search Terms**: [Terms used]
**Result**: ❌ No Red Hat UBI9 image found

**Evidence**:
```bash
# Catalog search performed
https://catalog.redhat.com/software/containers/search?q=rust

# Pull attempt failed
podman pull registry.access.redhat.com/ubi9/rust:latest
Error: image not found
```

### 3. Proposed Alternative Image
**Registry**: [e.g., docker.io, gcr.io]
**Image**: [Full image path]
**Version**: [Specific version or tag]

**Example**: `docker.io/rust:1.75-alpine`

### 4. Security Assessment
**Scan Results**: [Attach vulnerability scan]
**CVE Count**:
- CRITICAL: 0
- HIGH: 0
- MEDIUM: [N]
- LOW: [N]

**Scan Command**:
```bash
podman scan docker.io/rust:1.75-alpine
```

### 5. Justification
**Why is this image necessary?**
[Explain technical necessity]

**Why can't we use UBI9 base + manual installation?**
[Explain why building from UBI9 + installing runtime is not feasible]

**Alternatives Considered**:
1. [Alternative 1] - Rejected because [reason]
2. [Alternative 2] - Rejected because [reason]

### 6. Mitigation Plan
**How will we ensure security?**
- [ ] Weekly vulnerability scanning
- [ ] Automated rebuild on upstream updates
- [ ] Security advisory monitoring
- [ ] Documented update process

**How will we maintain consistency?**
- [ ] Pin to specific version (not `latest`)
- [ ] Document in service README
- [ ] Add to exception registry

### 7. Approval
**Requested Approval From**: Engineering Team
**Decision**: [PENDING/APPROVED/REJECTED]
**Approved By**: [Name]
**Date**: [YYYY-MM-DD]
```

#### **Exception Approval Criteria**

**Approved IF**:
- ✅ No Red Hat UBI9 image exists (verified via catalog search)
- ✅ Building from UBI9 + manual installation is not feasible
- ✅ Security scan shows ZERO CRITICAL/HIGH CVEs
- ✅ Mitigation plan addresses security and maintenance
- ✅ Engineering team approval documented

**Rejected IF**:
- ❌ Red Hat UBI9 image exists (use it instead)
- ❌ Can build from UBI9 + install runtime manually
- ❌ Security scan shows CRITICAL/HIGH CVEs
- ❌ No mitigation plan for security/maintenance

---

### **Exception Registry**

**Approved Exceptions** (as of 2025-10-28):

| Service | Image | Justification | Approved By | Date | Review Date |
|---------|-------|---------------|-------------|------|-------------|
| *None* | - | - | - | - | - |

**Note**: All Kubernaut services currently use Red Hat UBI9 images. No exceptions have been approved.

---

### **2. Approved Base Images**

**MANDATORY**: All base images MUST be from `registry.access.redhat.com` only.

#### **Complete Approved Image List**

| Image | Registry Path | Version Strategy | Use Case |
|-------|---------------|------------------|----------|
| **UBI9 Go Toolset** | `registry.access.redhat.com/ubi9/go-toolset` | Pin minor (e.g., `:1.24`) | Go build stage |
| **UBI9 Minimal** | `registry.access.redhat.com/ubi9/ubi-minimal` | Use `:latest` | Go/Python runtime |
| **UBI9 Base** | `registry.access.redhat.com/ubi9/ubi` | Use `:latest` | Full package mgmt |
| **UBI9 Python 3.12** | `registry.access.redhat.com/ubi9/python-312` | Use `:latest` | Python services |
| **UBI9 Python 3.11** | `registry.access.redhat.com/ubi9/python-311` | Use `:latest` | Python 3.11 compat |
| **UBI9 Node.js 20** | `registry.access.redhat.com/ubi9/nodejs-20` | Use `:latest` | Node.js services |
| **UBI9 Node.js 18** | `registry.access.redhat.com/ubi9/nodejs-18` | Use `:latest` | Node.js 18 LTS |

---

#### **For Go Services** (Most Common)

**Standard Multi-Stage Pattern**:

```dockerfile
# Build stage - Pin to Go toolset minor version
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

# Runtime stage - Use latest for security updates
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
```

**Versioning Rules**:
- ✅ **Build Stage**: Pin to minor version (`1.24`) for reproducible builds
- ✅ **Runtime Stage**: Use `latest` for automatic security patches
- ❌ **Never**: Use `latest` in build stage (unpredictable Go versions)
- ❌ **Never**: Pin runtime to specific version (miss security updates)

**Rationale**:
- **Build Reproducibility**: Same Go version across all builds
- **Security Updates**: Runtime gets RHSA patches automatically
- **Controlled Upgrades**: Explicit Go version bump in Dockerfile

**Available Go Toolset Versions**:
- `registry.access.redhat.com/ubi9/go-toolset:1.24` (Current)
- `registry.access.redhat.com/ubi9/go-toolset:1.23` (Previous)
- `registry.access.redhat.com/ubi9/go-toolset:1.22` (Legacy)

**When to Use Full UBI9 Base**:
```dockerfile
# Runtime stage - Full UBI9 (not minimal)
FROM registry.access.redhat.com/ubi9/ubi:latest
```
- **Use Case**: Services requiring `dnf` package management
- **Trade-off**: Larger image (~200MB vs. ~100MB minimal)
- **Example**: Services needing runtime package installation

---

#### **For Python Services** (e.g., HolmesGPT API)

**Standard Single-Stage Pattern**:

```dockerfile
# Python services typically use single-stage builds
FROM registry.access.redhat.com/ubi9/python-312:latest
```

**Versioning Rules**:
- ✅ **Use `latest`**: Python images include both build tools and runtime
- ✅ **Pin Python Major.Minor**: Use `python-312` (not `python`)
- ❌ **Never**: Use generic `python` tag (ambiguous version)

**Rationale**:
- **Simplicity**: Python services don't benefit from multi-stage builds
- **Security**: `latest` provides automatic security updates
- **Consistency**: Python version controlled by image tag (`312` vs. `311`)

**Available Python Versions**:
- `registry.access.redhat.com/ubi9/python-312:latest` (Python 3.12 - Recommended)
- `registry.access.redhat.com/ubi9/python-311:latest` (Python 3.11 - Stable)
- `registry.access.redhat.com/ubi9/python-39:latest` (Python 3.9 - Legacy)

**When to Use Multi-Stage** (Optional):
```dockerfile
# Build stage - Install dependencies
FROM registry.access.redhat.com/ubi9/python-312:latest AS builder
RUN pip install --user -r requirements.txt

# Runtime stage - Copy installed packages
FROM registry.access.redhat.com/ubi9/python-312:latest
COPY --from=builder /root/.local /root/.local
```
- **Use Case**: Reduce final image size (exclude build tools)
- **Trade-off**: More complex Dockerfile
- **Benefit**: ~30-40% smaller image

---

#### **For Node.js Services** (Future)

**Standard Single-Stage Pattern**:

```dockerfile
# Node.js services typically use single-stage builds
FROM registry.access.redhat.com/ubi9/nodejs-20:latest
```

**Versioning Rules**:
- ✅ **Use `latest`**: Node.js images include both npm and runtime
- ✅ **Pin Node.js Major**: Use `nodejs-20` (LTS) or `nodejs-18`
- ❌ **Never**: Use generic `nodejs` tag (ambiguous version)

**Available Node.js Versions**:
- `registry.access.redhat.com/ubi9/nodejs-20:latest` (Node.js 20 LTS - Recommended)
- `registry.access.redhat.com/ubi9/nodejs-18:latest` (Node.js 18 LTS - Stable)
- `registry.access.redhat.com/ubi9/nodejs-16:latest` (Node.js 16 - Legacy)

---

#### **Runtime-Only Images**

**UBI9 Minimal** (Recommended for Go):
```dockerfile
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
```
- **Size**: ~100MB
- **Packages**: `microdnf` (minimal package manager)
- **Use Case**: Go static binaries, minimal runtime dependencies
- **Benefits**: Smallest UBI9 variant, reduced attack surface

**UBI9 Base** (Full Package Management):
```dockerfile
FROM registry.access.redhat.com/ubi9/ubi:latest
```
- **Size**: ~200MB
- **Packages**: `dnf` (full package manager), `systemd`
- **Use Case**: Services requiring runtime package installation
- **Trade-off**: Larger image, more attack surface

---

### **3. UBI Version Policy**

#### **Current Standard: UBI9**
- **Status**: ✅ **Production Standard** (2025-10-28)
- **RHEL Base**: Red Hat Enterprise Linux 9
- **Support**: Full support until 2032 (RHEL 9 lifecycle)
- **Migration**: No immediate migration needed

#### **Future Standard: UBI10**
- **Status**: ⏸️ **Evaluation** (when available)
- **RHEL Base**: Red Hat Enterprise Linux 10
- **Migration Trigger**: UBI10 GA release + 6-month stability period
- **Migration Plan**:
  1. **Month 1-2**: Evaluate UBI10 compatibility with Kubernaut services
  2. **Month 3-4**: Pilot migration (1-2 non-critical services)
  3. **Month 5-6**: Gradual rollout (all services)
  4. **Month 7+**: UBI10 becomes new standard

#### **UBI8 (Legacy)**
- **Status**: ❌ **Not Approved** for new services
- **Rationale**: RHEL 8 maintenance support ends 2029 (shorter than UBI9)
- **Exception**: Existing services may continue using UBI8 until migration

---

### **4. Image Versioning Strategy**

#### **Build Stage Images** (Go Toolset, Python, Node.js)
**Recommendation**: **Pin to minor version**

```dockerfile
# ✅ CORRECT: Pin to Go 1.24
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

# ❌ WRONG: Use latest (unpredictable builds)
FROM registry.access.redhat.com/ubi9/go-toolset:latest AS builder

# ❌ WRONG: Pin to patch version (too restrictive)
FROM registry.access.redhat.com/ubi9/go-toolset:1.24.1 AS builder
```

**Rationale**:
- ✅ Reproducible builds (same Go version across rebuilds)
- ✅ Controlled upgrades (explicit Go version bump)
- ✅ Security updates (patch versions auto-applied)

#### **Runtime Stage Images** (UBI Minimal, UBI Base)
**Recommendation**: **Use `latest`**

```dockerfile
# ✅ CORRECT: Use latest for security updates
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# ❌ WRONG: Pin to specific version (miss security updates)
FROM registry.access.redhat.com/ubi9/ubi-minimal:9.3-1552
```

**Rationale**:
- ✅ Automatic security updates (RHSA patches applied)
- ✅ CVE remediation (no manual intervention)
- ✅ Minimal breaking changes (UBI minimal is stable)

---

### **5. Security Scanning Requirements**

#### **Base Image Scanning**
**MANDATORY**: All base images MUST pass security scanning before use.

**Tools**:
1. **Red Hat Security Advisories (RHSA)**: Monitor for CVEs
2. **Podman/Skopeo**: `skopeo inspect` for image metadata
3. **OpenShift Image Streams**: Automatic CVE tracking

**Acceptance Criteria**:
- ✅ **ZERO** CRITICAL vulnerabilities
- ✅ **ZERO** HIGH vulnerabilities (or documented mitigation)
- ⚠️ **MEDIUM** vulnerabilities: Acceptable with plan
- ✅ **LOW** vulnerabilities: Acceptable

**Scan Frequency**:
- **Build Time**: Every container build
- **Runtime**: Weekly scans of deployed images
- **RHSA Alerts**: Immediate response to Red Hat advisories

#### **Service Image Scanning**
**MANDATORY**: All Kubernaut service images MUST pass scanning.

**Pipeline Integration**:
```bash
# CI/CD Pipeline
podman build -t quay.io/jordigilh/gateway:v1.0.0 .
podman scan quay.io/jordigilh/gateway:v1.0.0
# Fail build if CRITICAL/HIGH CVEs found
```

---

### **6. Air-Gapped Deployment Support**

#### **Image Mirroring Strategy**

**For Disconnected Environments**:
1. **Mirror Base Images**: Copy UBI images to internal registry
2. **Update Dockerfiles**: Point to mirrored registry
3. **Periodic Sync**: Update mirrors weekly for security patches

**Example Mirroring**:
```bash
# Mirror UBI9 images to internal registry
skopeo copy \
  docker://registry.access.redhat.com/ubi9/go-toolset:1.24 \
  docker://internal-registry.example.com/ubi9/go-toolset:1.24

skopeo copy \
  docker://registry.access.redhat.com/ubi9/ubi-minimal:latest \
  docker://internal-registry.example.com/ubi9/ubi-minimal:latest
```

**Dockerfile for Air-Gapped**:
```dockerfile
# Use internal mirror
FROM internal-registry.example.com/ubi9/go-toolset:1.24 AS builder
# ...
FROM internal-registry.example.com/ubi9/ubi-minimal:latest
```

---

## Implementation

### **Dockerfile Template (Go Service)**

```dockerfile
# Service Name - Multi-Architecture Dockerfile using Red Hat UBI9
# Supports: linux/amd64, linux/arm64
# Based on: ADR-027 (Multi-Architecture), ADR-028 (Registry Policy)

# Build stage - Red Hat UBI9 Go 1.24 toolset
# ADR-028: Pin to minor version (1.24) for reproducible builds
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder

# Build arguments for multi-architecture support
ARG GOOS=linux
ARG GOARCH=amd64

# Switch to root for package installation
USER root

# Install build dependencies
RUN dnf update -y && \
	dnf install -y git ca-certificates tzdata && \
	dnf clean all

# Switch back to default user for security
USER 1001

# Set working directory
WORKDIR /opt/app-root/src

# Copy go mod files
COPY --chown=1001:0 go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY --chown=1001:0 . .

# Build the service binary
RUN CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
	-ldflags='-w -s -extldflags "-static"' \
	-a -installsuffix cgo \
	-o service-binary \
	./cmd/service

# Runtime stage - Red Hat UBI9 minimal runtime image
# ADR-028: Use latest for automatic security updates
FROM --platform=linux/${TARGETARCH} registry.access.redhat.com/ubi9/ubi-minimal:latest

# Install runtime dependencies
RUN microdnf update -y && \
	microdnf install -y ca-certificates tzdata && \
	microdnf clean all

# Create non-root user for security
RUN useradd -r -u 1001 -g root service-user

# Copy the binary from builder stage
COPY --from=builder /opt/app-root/src/service-binary /usr/local/bin/service

# Set proper permissions
RUN chmod +x /usr/local/bin/service

# Switch to non-root user for security
USER service-user

# Expose ports
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
	CMD ["/usr/bin/curl", "-f", "http://localhost:8080/health"] || exit 1

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/service"]

# Red Hat UBI9 compatible metadata labels
LABEL name="kubernaut-service" \
	vendor="Kubernaut" \
	version="1.0.0" \
	release="1" \
	summary="Kubernaut Service" \
	description="A microservice component of Kubernaut" \
	maintainer="jgil@redhat.com" \
	io.k8s.description="Kubernaut Service" \
	io.k8s.display-name="Kubernaut Service" \
	io.openshift.tags="kubernaut,microservice"
```

---

## Consequences

### **Positive**

1. ✅ **Security Compliance**: All images from trusted Red Hat sources
2. ✅ **Enterprise Support**: Full Red Hat support for base images
3. ✅ **Consistency**: Standardized registry and versioning across all services
4. ✅ **Supply Chain Security**: Reduced risk of supply chain attacks
5. ✅ **Air-Gapped Support**: Clear mirroring strategy for disconnected environments
6. ✅ **Future-Proofing**: Clear migration path to UBI10

### **Negative**

1. ⚠️ **Registry Dependency**: Requires `registry.access.redhat.com` availability
   - **Mitigation**: Mirror images to `quay.io/jordigilh/*` for redundancy
2. ⚠️ **Image Size**: UBI images larger than Alpine (~200MB vs. ~5MB)
   - **Mitigation**: Use `ubi-minimal` (100MB) instead of full UBI
3. ⚠️ **Build Performance**: DNF package management slower than apk
   - **Mitigation**: Acceptable trade-off for enterprise support

### **Neutral**

1. 🔄 **Migration Effort**: Existing services using Alpine/Distroless need migration
   - **Plan**: Gradual migration over 2-3 months
2. 🔄 **Version Management**: Requires monitoring for Go/Python version updates
   - **Plan**: Quarterly review of toolset versions

---

## Compliance Checklist

Before merging any Dockerfile:

- [ ] **Registry**: Uses `registry.access.redhat.com` for base images
- [ ] **Base Image**: Uses approved UBI9 image (go-toolset, ubi-minimal, python-312)
- [ ] **Build Stage**: Pins to minor version (e.g., `go-toolset:1.24`)
- [ ] **Runtime Stage**: Uses `latest` for security updates
- [ ] **Security Scan**: Passes vulnerability scan (no CRITICAL/HIGH CVEs)
- [ ] **Multi-Arch**: Supports `linux/amd64` and `linux/arm64`
- [ ] **Labels**: Includes Red Hat UBI9 compatible metadata labels
- [ ] **Comments**: References ADR-027 and ADR-028 in Dockerfile header

---

## Migration Plan

### **Phase 1: Audit** (Week 1)
- [ ] Identify all services using non-Red Hat base images
- [ ] Document exceptions and justifications
- [ ] Create migration priority list

### **Phase 2: Pilot** (Week 2-3)
- [ ] Migrate 2-3 low-risk services to UBI9
- [ ] Validate build times, image sizes, runtime performance
- [ ] Document lessons learned

### **Phase 3: Rollout** (Week 4-8)
- [ ] Migrate remaining services (2-3 per week)
- [ ] Update CI/CD pipelines for security scanning
- [ ] Update documentation and Dockerfile templates

### **Phase 4: Enforcement** (Week 9+)
- [ ] Enable automated Dockerfile linting (check for approved registries)
- [ ] Add pre-commit hooks to validate registry compliance
- [ ] Quarterly review of base image versions

---

## References

### **Related ADRs**
- [ADR-027: Multi-Architecture Build Strategy](./ADR-027-multi-architecture-build-strategy.md)

### **Red Hat Resources**
- [Red Hat Universal Base Images (UBI)](https://developers.redhat.com/products/rhel/ubi)
- [UBI9 Container Images Catalog](https://catalog.redhat.com/software/containers/search?q=ubi9)
- [Red Hat Container Security Guide](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/9/html/building_running_and_managing_containers/index)
- [Red Hat Security Advisories (RHSA)](https://access.redhat.com/security/security-updates/)

### **Security Resources**
- [Skopeo Image Inspection](https://github.com/containers/skopeo)
- [Podman Security Scanning](https://docs.podman.io/en/latest/markdown/podman-scan.1.html)
- [OpenShift Image Security](https://docs.openshift.com/container-platform/latest/security/container_security/security-container-content.html)

---

**Decision Rationale**:

Mandating `registry.access.redhat.com` and UBI9 base images provides:
1. **Enterprise Support**: Full Red Hat backing for production workloads
2. **Security Compliance**: Regular RHSA updates and CVE tracking
3. **OpenShift Optimization**: Native integration with Red Hat OpenShift
4. **Supply Chain Security**: Trusted image sources reduce attack surface
5. **Consistency**: Standardized base across all Kubernaut services

The benefits of enterprise support, security compliance, and OpenShift optimization far outweigh the trade-offs of larger image sizes and slower build times.

**Approved**:
- Engineering Team, 2025-10-28

