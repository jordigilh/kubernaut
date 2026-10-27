# Kubernaut Secrets Management

**Authority**: [DD-AUTH-008: Secret Management Strategy (Kustomize + Helm)](../../docs/architecture/decisions/DD-AUTH-008-secret-management-kustomize-helm.md)

---

## 📋 **Overview**

This directory contains Kustomize configurations for optional, administrator-managed OAuth2-proxy cookie Secret fixtures used by DataStorage and HolmesGPT API services.

**Key Principle**: **Secret material is generated outside Kustomize and NEVER stored in Git.**

---

## 🚀 **Quick Start**

### **Deploy Secrets**

For supported deployments, first install the [Kubernaut Operator bootstrap chart](https://github.com/jordigilh/kubernaut-operator/blob/main/charts/kubernaut-operator/README.md).
It installs the operator in `kubernaut-operator-system`. Then use the
[minimal Kubernaut CR quickstart](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md),
which uses `kubernaut-system` for the application operands and CR.

```bash
# Create the application namespace before applying namespaced Secret fixtures
kubectl create namespace kubernaut-system --dry-run=client -o yaml | kubectl apply -f -

# Generate secret material outside Kustomize; these files are ignored by Git
mkdir -p deploy/secrets/local
umask 077
openssl rand -base64 32 | tr -d '\n' > deploy/secrets/local/data-storage-cookie-secret
openssl rand -base64 32 | tr -d '\n' > deploy/secrets/local/kubernaut-agent-cookie-secret

# Apply optional Secret fixtures before the CR if the CR references them
kubectl apply -k deploy/secrets/

# Verify secrets created
kubectl get secrets -n kubernaut-system | grep oauth-proxy
```

### **Expected Output**

```
data-storage-oauth-proxy-secret       Opaque   1      10s
kubernaut-agent-oauth-proxy-secret      Opaque   1      10s
```

---

## 🔍 **What Gets Created**

| Secret Name | Namespace | Key | Value | Usage |
|-------------|-----------|-----|-------|-------|
| `data-storage-oauth-proxy-secret` | `kubernaut-system` | `cookie-secret` | Base64 encoding of 32 random bytes | DataStorage oauth2-proxy sidecar |
| `kubernaut-agent-oauth-proxy-secret` | `kubernaut-system` | `cookie-secret` | Base64 encoding of 32 random bytes | HolmesGPT API oauth2-proxy sidecar |

---

## 📊 **How It Works**

### **Kustomize Secret Generation**

```yaml
# kustomization.yaml
secretGenerator:
  - name: data-storage-oauth-proxy-secret
    files:
      - cookie-secret=local/data-storage-cookie-secret
    options:
      disableNameSuffixHash: true
```

**What happens**:
1. An administrator generates the ignored local files outside Kustomize
2. `kubectl apply -k` reads each file as Secret data
3. Kustomize generates a Kubernetes Secret manifest
4. The manifest is applied to the cluster

**Result**: Secret exists in cluster, value never in Git

---

### **Operator Integration**

The application Helm chart is retired. Supported deployments install the
Kubernaut Operator Helm bootstrap chart and apply a user-owned
`kubernaut.ai/v1alpha2` `Kubernaut` resource. Use this directory only when a
current CR field explicitly references one of these administrator-managed Secret
fixtures; the operator owns application workload and Secret-reference
reconciliation.

**Deployment Order**:
1. Install the operator bootstrap chart in `kubernaut-operator-system`.
2. Create `kubernaut-system` for the application operands and `Kubernaut` CR.
3. Generate the ignored `local/*` files and apply these fixtures if the CR references them.
4. Apply the user-owned `Kubernaut` CR using the operator quickstart.

---

## 🔧 **Verification Commands**

### **Check Secret Exists**

```bash
kubectl get secret data-storage-oauth-proxy-secret -n kubernaut-system
```

### **View Secret Value (Base64 Encoded)**

```bash
kubectl get secret data-storage-oauth-proxy-secret -n kubernaut-system \
  -o jsonpath='{.data.cookie-secret}'
```

### **Decode Secret Value**

```bash
kubectl get secret data-storage-oauth-proxy-secret -n kubernaut-system \
  -o jsonpath='{.data.cookie-secret}' | base64 -d
```

### **Verify Secret Length (Should be 32 bytes)**

```bash
kubectl get secret data-storage-oauth-proxy-secret -n kubernaut-system \
  -o jsonpath='{.data.cookie-secret}' | base64 -d | wc -c
# Expected output: 32
```

### **Verify Secret References Do NOT Expose Values**

```bash
# Inspect the rendered operator-managed workload or CR status instead.
# Secret values must never be copied into a CR, manifest, or Git repository.
```

---

## 🔄 **Secret Rotation**

### **Rotate Cookie Secret**

Secrets created from the former `$(openssl rand ...)` literals must be treated as
compromised and rotated. Generate fresh local files before applying them; do not
reuse the old files.

```bash
# Generate fresh values without a trailing newline
umask 077
openssl rand -base64 32 | tr -d '\n' > deploy/secrets/local/data-storage-cookie-secret
openssl rand -base64 32 | tr -d '\n' > deploy/secrets/local/kubernaut-agent-cookie-secret

# Apply both rotated Secret values in place
kubectl apply -k deploy/secrets/
```

Restart every operator-managed workload that consumes a rotated Secret so it
reloads the new cookie value. Existing OAuth2-proxy sessions are invalidated.

---

## 🏢 **Production Deployment**

For production, keep the generated files in a secure external store instead of
the repository:

### **One-Time Setup**

```bash
# Generate secrets once, store securely
openssl rand -base64 32 > /vault/secrets/ds-cookie-secret.txt
openssl rand -base64 32 > /vault/secrets/hapi-cookie-secret.txt

# Secure the files
chmod 600 /vault/secrets/*.txt
```

### **Production Kustomization**

```yaml
# deploy/secrets/production/kustomization.yaml
secretGenerator:
  - name: data-storage-oauth-proxy-secret
    files:
      - cookie-secret=/vault/secrets/ds-cookie-secret.txt
    options:
      disableNameSuffixHash: true
```

### **Deploy Production Secrets**

```bash
kubectl apply -k deploy/secrets/production/
```

---

## 🚨 **Security Considerations**

### **What's Safe**

- ✅ `kustomization.yaml` in Git (no secret values)
- ✅ `README.md` in Git (documentation only)
- ✅ Operator/CR configuration referencing Secret names

### **What's NOT Safe**

- ❌ Hardcoded secret values in YAML files
- ❌ Secret values in Git repositories
- ❌ Secret values in Helm templates
- ❌ Secret values in `helm template` output

### **RBAC Access**

Only oauth-proxy containers should have access:

```yaml
# Deployment volume mount
volumeMounts:
  - name: oauth-proxy-cookie-secret
    mountPath: /etc/oauth-proxy
    readOnly: true  # ← Read-only mount
```

---

## 📚 **References**

- **[DD-AUTH-008: Secret Management Strategy](../../docs/architecture/decisions/DD-AUTH-008-secret-management-kustomize-helm.md)** - Authoritative design decision
- **DD-AUTH-007: OAuth2-Proxy Migration** - Migration guide (document removed)
- **[DD-AUTH-004: DataStorage OAuth](../../docs/architecture/decisions/DD-AUTH-004-openshift-oauth-proxy-legal-hold.md)** - DataStorage oauth-proxy pattern
- **[DD-AUTH-006: HAPI OAuth](../../docs/architecture/decisions/DD-AUTH-006-hapi-oauth-integration.md)** - HAPI oauth-proxy pattern

---

## 🔗 **Related**

- **Kubernaut Operator**: [installation guide](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md) - application deployment and Secret references
- **Deployments**: `deploy/data-storage/`, `deploy/kubernaut-agent/` - Service manifests
- **OAuth2-Proxy Docs**: https://oauth2-proxy.github.io/oauth2-proxy/docs/

---

**Last Updated**: January 26, 2026  
**Authority**: DD-AUTH-008
