# DD-REGISTRY-001: Container Registry Strategy

**Date**: December 17, 2025
**Status**: ✅ **APPROVED**
**Priority**: **P0 - CRITICAL** (Deployment prerequisite)
**Supersedes**: None
**Related To**:
- ADR-028 (Container Registry Policy)
- ADR-027 (Multi-Architecture Build Strategy)

---

## 🎯 **Decision Summary**

**APPROVED**: Kubernaut uses **two Quay.io organizations** with distinct purposes:

1. **`quay.io/jordigilh/`** - Development and testing
2. **`quay.io/kubernaut/`** - Staging and production

---

## 📋 **Context & Problem**

**Problem**: Kubernaut needs a clear container registry strategy that separates development/testing images from staging/production images.

**Key Requirements**:
- **Development Isolation**: Development images should not interfere with production
- **Staging Validation**: Staging images must be tested before production
- **Production Stability**: Production images must be immutable and verified
- **Clear Ownership**: Registry organization must reflect image lifecycle

**Current State**:
- ADR-028 defines approved registries but doesn't specify development vs production strategy
- Multiple documents reference `quay.io/jordigilh/` for images
- No clear guidance on when to use which registry

---

## ✅ **Decision**

### **Registry Strategy**

| Registry | Purpose | Image Lifecycle | Ownership | Immutability |
|---|---|---|---|---|
| **`quay.io/jordigilh/`** | **Development & Testing** | Ephemeral, frequent updates | Individual developer | ⚠️ Mutable (tags can be overwritten) |
| **`quay.io/kubernaut/`** | **Staging & Production** | Stable, versioned releases | Kubernaut organization | ✅ Immutable (tags cannot be overwritten) |

---

## 📊 **Detailed Registry Specifications**

### **Development Registry: `quay.io/jordigilh/`**

**Purpose**: Development, testing, and CI/CD pipelines

**Characteristics**:
- **Ownership**: Individual developer account
- **Visibility**: Public or private (per image)
- **Tag Strategy**: Mutable tags allowed (e.g., `latest`, `dev`, `feature-xyz`)
- **Retention**: No guaranteed retention (images may be deleted)
- **Security Scanning**: Optional
- **Image Signing**: Not required

**Use Cases**:
- Local development builds
- CI/CD pipeline testing
- Integration test images
- E2E test images
- Experimental features

**Example Images**:
```
quay.io/jordigilh/gateway:dev
quay.io/jordigilh/datastorage:feature-audit-v2
quay.io/jordigilh/holmesgpt-api:test-20251217
quay.io/jordigilh/must-gather:latest
```

---

### **Production Registry: `quay.io/kubernaut/`**

**Purpose**: Staging validation and production deployments

**Characteristics**:
- **Ownership**: Kubernaut organization (team-managed)
- **Visibility**: Public (for open-source distribution)
- **Tag Strategy**: **Immutable tags** (semantic versioning only)
- **Retention**: Permanent (images never deleted)
- **Security Scanning**: **Mandatory** (ZERO CRITICAL/HIGH CVEs)
- **Image Signing**: **Mandatory** (Cosign signatures required)

**Use Cases**:
- Staging environment deployments
- Production environment deployments
- Official releases (v1.0.0, v1.1.0, etc.)
- Customer deployments
- Air-gapped environment mirrors

**Example Images**:
```
quay.io/kubernaut/gateway:v1.0.0
quay.io/kubernaut/datastorage:v1.0.0
quay.io/kubernaut/holmesgpt-api:v1.0.0
quay.io/kubernaut/must-gather:v1.0.0
```

---

## 🔄 **Image Promotion Workflow**

### **Development → Staging → Production**

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Development Build                                         │
│    - Developer builds image locally or via CI/CD             │
│    - Push to: quay.io/jordigilh/<service>:dev               │
│    - Testing: Unit tests, integration tests                  │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ Tests pass ✅
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Staging Promotion                                         │
│    - Tag image with semantic version                         │
│    - Security scan (MANDATORY)                               │
│    - Push to: quay.io/kubernaut/<service>:v1.0.0-rc1        │
│    - Deploy to staging environment                           │
│    - E2E testing in staging                                  │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ Staging validation passes ✅
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Production Promotion                                      │
│    - Sign image with Cosign (MANDATORY)                      │
│    - Push to: quay.io/kubernaut/<service>:v1.0.0            │
│    - Deploy to production environment                        │
│    - Image tag is IMMUTABLE (cannot be overwritten)          │
└─────────────────────────────────────────────────────────────┘
```

---

## 🛠️ **Implementation Guidelines**

### **For Developers**

**Development Builds**:
```bash
# Build image
podman build -t quay.io/jordigilh/gateway:dev .

# Push to development registry
podman push quay.io/jordigilh/gateway:dev
```

**Staging Promotion** (requires team approval):
```bash
# Tag for staging
podman tag quay.io/jordigilh/gateway:dev quay.io/kubernaut/gateway:v1.0.0-rc1

# Security scan (MANDATORY)
podman scan quay.io/kubernaut/gateway:v1.0.0-rc1

# Push to staging registry
podman push quay.io/kubernaut/gateway:v1.0.0-rc1
```

**Production Promotion** (requires release manager approval):
```bash
# Tag for production
podman tag quay.io/kubernaut/gateway:v1.0.0-rc1 quay.io/kubernaut/gateway:v1.0.0

# Sign image (MANDATORY)
cosign sign quay.io/kubernaut/gateway:v1.0.0

# Push to production registry
podman push quay.io/kubernaut/gateway:v1.0.0
```

---

### **For CI/CD Pipelines**

**Development Pipeline** (automated):
```yaml
# .github/workflows/dev-build.yaml
- name: Build and push development image
  run: |
    podman build -t quay.io/jordigilh/${{ matrix.service }}:dev .
    podman push quay.io/jordigilh/${{ matrix.service }}:dev
```

**Staging Pipeline** (manual approval required):
```yaml
# .github/workflows/staging-promote.yaml
- name: Promote to staging
  run: |
    podman tag quay.io/jordigilh/${{ matrix.service }}:dev \
                quay.io/kubernaut/${{ matrix.service }}:${{ github.ref_name }}
    podman scan quay.io/kubernaut/${{ matrix.service }}:${{ github.ref_name }}
    podman push quay.io/kubernaut/${{ matrix.service }}:${{ github.ref_name }}
```

**Production Pipeline** (release manager approval required):
```yaml
# .github/workflows/production-release.yaml
- name: Release to production
  run: |
    podman tag quay.io/kubernaut/${{ matrix.service }}:${{ github.ref_name }}-rc1 \
                quay.io/kubernaut/${{ matrix.service }}:${{ github.ref_name }}
    cosign sign quay.io/kubernaut/${{ matrix.service }}:${{ github.ref_name }}
    podman push quay.io/kubernaut/${{ matrix.service }}:${{ github.ref_name }}
```

---

## 🔒 **Security Requirements**

### **Development Registry (`quay.io/jordigilh/`)**

- ⚠️ Security scanning: **Optional** (recommended but not enforced)
- ⚠️ Image signing: **Not required**
- ⚠️ Vulnerability threshold: **No enforcement**

### **Production Registry (`quay.io/kubernaut/`)**

- ✅ Security scanning: **MANDATORY** (ZERO CRITICAL/HIGH CVEs)
- ✅ Image signing: **MANDATORY** (Cosign signatures required)
- ✅ Vulnerability threshold: **CRITICAL=0, HIGH=0**
- ✅ Immutable tags: **Enforced** (tags cannot be overwritten)

---

## 📋 **Tag Naming Conventions**

### **Development Registry**

| Tag Pattern | Purpose | Example |
|---|---|---|
| `latest` | Latest development build | `quay.io/jordigilh/gateway:latest` |
| `dev` | Current development branch | `quay.io/jordigilh/gateway:dev` |
| `feature-<name>` | Feature branch builds | `quay.io/jordigilh/gateway:feature-audit-v2` |
| `test-<date>` | Test builds | `quay.io/jordigilh/gateway:test-20251217` |
| `<commit-sha>` | Commit-specific builds | `quay.io/jordigilh/gateway:abc123` |

### **Production Registry**

| Tag Pattern | Purpose | Example |
|---|---|---|
| `v<major>.<minor>.<patch>` | Production release | `quay.io/kubernaut/gateway:v1.0.0` |
| `v<major>.<minor>.<patch>-rc<n>` | Release candidate | `quay.io/kubernaut/gateway:v1.0.0-rc1` |
| `v<major>.<minor>.<patch>-beta<n>` | Beta release | `quay.io/kubernaut/gateway:v1.0.0-beta1` |

**Forbidden in Production Registry**:
- ❌ `latest` (ambiguous, mutable)
- ❌ `dev` (development-specific)
- ❌ `test-*` (testing-specific)
- ❌ Commit SHAs (not human-readable)

---

## 🚨 **Critical Rules**

### **Rule 1: Production Tags are Immutable**

Once an image is pushed to `quay.io/kubernaut/` with a semantic version tag (e.g., `v1.0.0`), that tag **CANNOT** be overwritten.

**Enforcement**: Quay.io repository settings must enable "Tag Immutability" for `quay.io/kubernaut/` organization.

### **Rule 2: Production Images Must Be Signed**

All images in `quay.io/kubernaut/` with semantic version tags **MUST** have Cosign signatures.

**Verification**:
```bash
cosign verify quay.io/kubernaut/gateway:v1.0.0
```

### **Rule 3: Production Images Must Pass Security Scan**

All images in `quay.io/kubernaut/` **MUST** have ZERO CRITICAL and ZERO HIGH vulnerabilities.

**Verification**:
```bash
podman scan quay.io/kubernaut/gateway:v1.0.0 | grep -E "CRITICAL|HIGH"
# Expected: No output (ZERO CRITICAL/HIGH CVEs)
```

---

## ✅ **Approval**

**Decision**: Two-registry strategy (development vs production)
**Confidence**: **99%**
**Status**: ✅ **APPROVED** (2025-12-17)
**Priority**: **P0 - CRITICAL** (Deployment prerequisite)

**Rationale**:
1. **Clear Separation**: Development images isolated from production
2. **Security**: Production images require scanning and signing
3. **Stability**: Production tags are immutable
4. **Flexibility**: Development registry allows rapid iteration
5. **Industry Standard**: Matches Docker Hub, GHCR, ECR patterns

---

## 📚 **References**

### **Related ADRs/DDs**:
- **ADR-028**: Container Registry Policy (defines approved registries)
- **ADR-027**: Multi-Architecture Build Strategy (defines build process)

### **External Resources**:
- [Quay.io Tag Immutability](https://docs.quay.io/guides/tag-expiration.html)
- [Cosign Image Signing](https://docs.sigstore.dev/cosign/overview/)
- [Podman Security Scanning](https://docs.podman.io/en/latest/markdown/podman-scan.1.html)

---

**Last Updated**: December 17, 2025
**Next Review**: After v1.0 release (evaluate multi-registry strategy effectiveness)

