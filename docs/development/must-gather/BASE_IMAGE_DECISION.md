# Base Image Decision: Red Hat UBI10 Standard

**Date**: 2026-01-04
**Decision**: Use `registry.access.redhat.com/ubi10/ubi:latest` (UBI Standard)
**Previous**: `registry.access.redhat.com/ubi10/ubi-minimal:latest`
**Status**: ✅ **Adopted**

---

## 🎯 **Decision Summary**

**Chosen**: Red Hat UBI10 Standard (`ubi10/ubi`)

**Rationale**: Aligns with OpenShift must-gather pattern and includes essential diagnostic tools pre-installed, reducing build complexity.

---

## 📊 **Comparison: UBI Variants**

### UBI Standard (ubi10/ubi) - ✅ **SELECTED**

**Base**: Red Hat Enterprise Linux 9
**Package Manager**: `dnf` (full DNF stack)
**Size**: ~200MB

**Pre-Installed Tools** (Relevant for Must-Gather):
```
✅ bash           - Shell scripting
✅ tar            - Archive creation
✅ gzip           - Compression
✅ findutils      - File search (find, xargs)
✅ util-linux     - System utilities
✅ coreutils      - Core GNU utilities (ls, cat, grep, etc.)
✅ vi/vim         - Text editing for troubleshooting
✅ less, more     - Log viewing
✅ curl           - HTTP downloads
✅ OpenSSL        - Unified crypto stack
```

**Advantages**:
- ✅ **Comprehensive toolset**: Most diagnostic tools already included
- ✅ **Debugging friendly**: Includes `vi`, `less` for interactive troubleshooting
- ✅ **OpenShift pattern**: Matches OpenShift must-gather implementation
- ✅ **Full package manager**: `dnf` for additional dependencies if needed
- ✅ **Simpler Dockerfile**: Fewer packages to install manually

**Disadvantages**:
- Larger image size (~200MB vs ~90MB for minimal)

---

### UBI Minimal (ubi10/ubi-minimal) - ❌ **REJECTED**

**Base**: Red Hat Enterprise Linux 9
**Package Manager**: `microdnf` (minimal)
**Size**: ~90MB

**Pre-Installed Tools**:
```
✅ coreutils-single  - Minimal core utilities
❌ tar               - Must install manually
❌ gzip              - Must install manually
❌ findutils         - Must install manually
❌ curl              - Must install manually
❌ vi/vim            - Not included
```

**Advantages**:
- Smaller image size
- Reduced attack surface (fewer packages)

**Disadvantages**:
- ❌ **More complex Dockerfile**: Must install many tools manually
- ❌ **Missing debugging tools**: No `vi`, `less` for interactive troubleshooting
- ❌ **Package conflicts**: `coreutils-single` vs `coreutils` issues
- ❌ **Limited package manager**: `microdnf` has fewer features than `dnf`

---

### UBI Micro (ubi10/ubi-micro) - ❌ **NOT SUITABLE**

**Size**: ~30MB
**Package Manager**: None

**Why Not Suitable**:
- No package manager to install kubectl, jq
- No shell utilities for bash scripts
- Designed for static Go binaries, not shell scripts

---

### UBI Init (ubi10/ubi-init) - ❌ **NOT SUITABLE**

**Base**: UBI Standard + systemd
**Size**: ~220MB

**Why Not Suitable**:
- Must-gather is a single-run diagnostic tool, not a service
- No need for systemd or multi-process management
- Unnecessary overhead

---

## 🏗️ **Dockerfile Optimization**

### Before (ubi-minimal)
```dockerfile
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest

# Install required tools (many needed)
RUN microdnf install -y \
    tar \
    gzip \
    findutils \
    util-linux \
    && microdnf clean all

# Install kubectl manually
RUN curl -LO "https://dl.k8s.io/release/v1.31.0/bin/linux/${TARGETARCH}/kubectl" ...

# Install jq manually
RUN curl -L "https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux-${JQ_ARCH}" ...
```

**Issues**:
- Package conflict: `coreutils-single` vs `coreutils`
- Multiple manual installations
- Complex multi-arch handling

### After (ubi standard)
```dockerfile
FROM registry.access.redhat.com/ubi10/ubi:latest

# UBI Standard already includes: tar, gzip, findutils, util-linux, coreutils, curl
# Only install additional tools we need

# Install kubectl
RUN curl -LO "https://dl.k8s.io/release/v1.31.0/bin/linux/${TARGETARCH}/kubectl" ...

# Install jq (try dnf first, fallback to direct download)
RUN dnf install -y jq || \
    (curl -L "https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux-${JQ_ARCH}" ...) && \
    dnf clean all

# Verify all tools available
RUN kubectl version --client && jq --version && tar --version
```

**Benefits**:
- ✅ Simpler: Only install kubectl and jq
- ✅ Cleaner: No package conflicts
- ✅ Verifiable: Tool verification step
- ✅ Standard pattern: Matches OpenShift

---

## 📦 **Image Size Analysis**

| Base Image | Compressed | Uncompressed | Must-Gather Final |
|------------|------------|--------------|-------------------|
| **ubi-minimal** | ~35MB | ~90MB | ~180MB |
| **ubi (standard)** | ~75MB | ~200MB | ~280MB |

**Size Difference**: +100MB uncompressed

**Trade-off Assessment**:
- ✅ **Acceptable**: 100MB increase is minimal for enterprise deployment
- ✅ **Value**: Comprehensive toolset worth the size increase
- ✅ **Standard**: Matches OpenShift pattern (proven in production)
- ✅ **Debugging**: Interactive troubleshooting capability valuable for support

**Decision**: Size increase is acceptable given the benefits.

---

## 🔍 **OpenShift Must-Gather Pattern**

Red Hat OpenShift's must-gather implementations use **UBI Standard** as the base:

**Example**: OpenShift Must-Gather
- **Base**: `registry.redhat.io/openshift4/ose-cli:latest` (derived from UBI Standard)
- **Tools**: Full CLI toolset (`oc`, `kubectl`, standard Unix utilities)
- **Size**: ~300MB (larger than our implementation)
- **Pattern**: Comprehensive tooling over minimal footprint

**Our Alignment**:
```
OpenShift Pattern:     UBI Standard + OpenShift CLI + debugging tools
Kubernaut Pattern:     UBI Standard + kubectl + jq + debugging tools
```

**Result**: We follow OpenShift best practices while keeping image smaller.

---

## 🛡️ **Security Considerations**

### UBI Standard Security Profile

**✅ Advantages**:
- Regular security updates from Red Hat
- CVE patching within 24-48 hours
- Subscription-free redistribution
- 10-year support lifecycle (RHEL 9)
- Maintained by Red Hat security team

**⚠️ Considerations**:
- Larger attack surface (more packages)
- More dependencies to scan

**Mitigation**:
```bash
# Security scanning in CI pipeline
podman scan quay.io/kubernaut/must-gather:latest

# Update base image regularly
FROM registry.access.redhat.com/ubi10/ubi:latest  # Always pulls latest security patches
```

**Decision**: UBI Standard's security posture is acceptable for enterprise must-gather tool.

---

## 🚀 **Build Performance**

### Build Time Comparison

| Base Image | Build Time | Reason |
|------------|------------|--------|
| **ubi-minimal** | ~3-4 min | More packages to install via microdnf |
| **ubi (standard)** | ~2-3 min | Fewer packages to install, faster dnf |

**Result**: UBI Standard builds **faster** despite being larger base.

---

## 📚 **References**

### Red Hat Documentation
- [Red Hat Universal Base Images](https://developers.redhat.com/products/rhel/ubi)
- [UBI10 Standard Image](https://catalog.redhat.com/software/containers/ubi10/ubi/615bcf606feffc5384e8452e)
- [Building Must-Gather Images](https://docs.openshift.com/container-platform/4.14/support/gathering-cluster-data.html#gathering-data-specific-features_gathering-cluster-data)

### OpenShift Must-Gather Examples
- [OpenShift Must-Gather Repository](https://github.com/openshift/must-gather)
- [OCP Must-Gather Base Image](https://catalog.redhat.com/software/containers/openshift4/ose-cli/5cd9ba3f5a13467289f4d51d)

### Kubernaut Documentation
- [BR-PLATFORM-001: Must-Gather Diagnostic Collection](../../docs/requirements/BR-PLATFORM-001-must-gather-diagnostic-collection.md)
- [Must-Gather Test Plan](../../docs/development/must-gather/TEST_PLAN_MUST_GATHER_V1_0.md)

---

## ✅ **Decision Checklist**

### Requirements Met

- [x] **Red Hat certified**: UBI10 from official Red Hat registry
- [x] **OpenShift compatible**: Follows OpenShift must-gather pattern
- [x] **Security maintained**: Regular updates from Red Hat
- [x] **Tool completeness**: All required diagnostic tools included
- [x] **Multi-arch support**: amd64 + arm64 compatible
- [x] **Freely redistributable**: No licensing restrictions
- [x] **Long-term support**: 10-year RHEL 9 lifecycle

### Trade-offs Accepted

- [x] **Size increase**: +100MB acceptable for enterprise tooling
- [x] **Attack surface**: Larger surface acceptable with regular updates
- [x] **Complexity**: Simpler Dockerfile outweighs minimal size benefit

---

## 🔄 **Migration Path**

### From ubi-minimal to ubi (standard)

**Step 1**: Update Dockerfile base image
```dockerfile
# Before
FROM registry.access.redhat.com/ubi10/ubi-minimal:latest

# After
FROM registry.access.redhat.com/ubi10/ubi:latest
```

**Step 2**: Remove redundant package installations
```dockerfile
# Remove (already in ubi standard)
RUN microdnf install -y tar gzip findutils util-linux

# Keep (still need these)
RUN curl -LO kubectl...
RUN dnf install -y jq || curl -L jq...
```

**Step 3**: Rebuild and test
```bash
make build
make test-unit
make test-e2e
```

**Step 4**: Verify no regressions
- ✅ All scripts execute correctly
- ✅ All tools available (kubectl, jq, tar, gzip)
- ✅ Multi-arch build works (amd64 + arm64)
- ✅ No package conflicts

---

## 🎯 **Success Criteria**

### Validation Checklist

- [x] Dockerfile builds successfully on amd64 and arm64
- [x] Image size within acceptable range (< 350MB)
- [x] All required tools present and functional
- [x] No package installation conflicts
- [x] Security scan passes with no critical CVEs
- [x] Unit tests pass
- [ ] E2E tests pass (pending cluster deployment)
- [ ] Production validation complete (pending release)

---

## 💡 **Lessons Learned**

### What Worked

1. **Follow OpenShift patterns**: Proven in enterprise production
2. **Prioritize tooling over size**: Comprehensive tools > minimal footprint
3. **UBI Standard is the sweet spot**: Not too minimal, not too bloated
4. **Verification steps**: Add tool checks to catch missing dependencies early

### Future Considerations

1. **Consider EPEL repository**: May enable `dnf install jq` directly
2. **Monitor image size**: Alert if final image exceeds 400MB
3. **Track CVEs**: Automated security scanning in CI
4. **Document debugging**: Leverage `vi`, `less` for interactive debugging

---

**Decision Maker**: Kubernaut Platform Team
**Approved**: 2026-01-04
**Next Review**: V1.1 planning (Q2 2026)

