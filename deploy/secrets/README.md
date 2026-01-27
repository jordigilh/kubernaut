# Kubernaut Secrets Management

**Authority**: [DD-AUTH-008: Secret Management Strategy (Kustomize + Helm)](../../docs/architecture/decisions/DD-AUTH-008-secret-management-kustomize-helm.md)

---

## 📋 **Overview**

This directory contains Kustomize configurations for generating OAuth2-proxy cookie secrets used by DataStorage and HolmesGPT API services.

**Key Principle**: **Secrets are generated at deployment time, NEVER stored in Git.**

---

## 🚀 **Quick Start**

### **Deploy Secrets**

```bash
# Generate and apply secrets
kubectl apply -k deploy/secrets/

# Verify secrets created
kubectl get secrets -n kubernaut-system | grep oauth-proxy
```

### **Expected Output**

```
data-storage-oauth-proxy-secret       Opaque   1      10s
holmesgpt-api-oauth-proxy-secret      Opaque   1      10s
```

---

## 🔍 **What Gets Created**

| Secret Name | Namespace | Key | Value | Usage |
|-------------|-----------|-----|-------|-------|
| `data-storage-oauth-proxy-secret` | `kubernaut-system` | `cookie-secret` | Random 32-byte string | DataStorage oauth2-proxy sidecar |
| `holmesgpt-api-oauth-proxy-secret` | `kubernaut-system` | `cookie-secret` | Random 32-byte string | HolmesGPT API oauth2-proxy sidecar |

---

## 📊 **How It Works**

### **Kustomize Secret Generation**

```yaml
# kustomization.yaml
secretGenerator:
  - name: data-storage-oauth-proxy-secret
    literals:
      - cookie-secret=$(openssl rand -base64 32)  # ← Executed at deploy time
    options:
      disableNameSuffixHash: true
```

**What happens**:
1. `kubectl apply -k` reads `kustomization.yaml`
2. Executes `$(openssl rand -base64 32)` on your machine
3. Generates a Kubernetes Secret manifest with the random value
4. Applies the Secret to the cluster

**Result**: Secret exists in cluster, value never in Git

---

### **Helm Integration**

Helm charts **reference** these secrets by name (do NOT create them):

```yaml
# helm/kubernaut/values.yaml
dataStorage:
  oauth:
    secretName: data-storage-oauth-proxy-secret  # ← References Kustomize-generated secret
```

```yaml
# helm/kubernaut/templates/data-storage-deployment.yaml
volumes:
  - name: oauth-proxy-cookie-secret
    secret:
      secretName: {{ .Values.dataStorage.oauth.secretName }}
```

**Deployment Order**:
```bash
kubectl apply -k deploy/secrets/           # Step 1: Create secrets
helm upgrade --install kubernaut ./helm/kubernaut  # Step 2: Deploy app
```

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

### **Verify Helm Template Does NOT Expose Secrets**

```bash
helm template kubernaut ./helm/kubernaut | grep -i "cookie-secret"
# Expected: Only references to secretName, NO actual secret values
```

---

## 🔄 **Secret Rotation**

### **Rotate Cookie Secret**

```bash
# 1. Delete existing secret
kubectl delete secret data-storage-oauth-proxy-secret -n kubernaut-system

# 2. Regenerate secret
kubectl apply -k deploy/secrets/

# 3. Restart deployment to pick up new secret
kubectl rollout restart deployment/data-storage-service -n kubernaut-system
```

**Note**: Pod will automatically restart after secret deletion.

---

## 🏢 **Production Deployment**

For production, use **file-based secrets** instead of dynamic generation:

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
- ✅ Helm values referencing secret names

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
- **[DD-AUTH-007: OAuth2-Proxy Migration](../../docs/development/SOC2/DD-AUTH-007_OAUTH_PROXY_MIGRATION.md)** - Migration guide
- **[DD-AUTH-004: DataStorage OAuth](../../docs/architecture/decisions/DD-AUTH-004-openshift-oauth-proxy-legal-hold.md)** - DataStorage oauth-proxy pattern
- **[DD-AUTH-006: HAPI OAuth](../../docs/architecture/decisions/DD-AUTH-006-hapi-oauth-integration.md)** - HAPI oauth-proxy pattern

---

## 🔗 **Related**

- **Helm Charts**: `helm/kubernaut/` - Application deployment (references secrets)
- **Deployments**: `deploy/data-storage/`, `deploy/holmesgpt-api/` - Service manifests
- **OAuth2-Proxy Docs**: https://oauth2-proxy.github.io/oauth2-proxy/docs/

---

**Last Updated**: January 26, 2026  
**Authority**: DD-AUTH-008
