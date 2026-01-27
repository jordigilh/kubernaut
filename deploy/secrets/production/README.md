# Production Secrets Management

**Authority**: [DD-AUTH-008: Secret Management Strategy](../../../docs/architecture/decisions/DD-AUTH-008-secret-management-kustomize-helm.md)

---

## 📋 **Overview**

Production secret management using **file-based** secret generation for stability and persistence.

**Key Difference from Dev**:
- **Dev**: Secrets generated dynamically on every `kubectl apply` (`$(openssl rand)`)
- **Production**: Secrets read from secure files (generated once, stored encrypted)

---

## 🔧 **One-Time Setup**

### **1. Create Secure Directory**

```bash
# Create secrets directory (encrypted filesystem recommended)
sudo mkdir -p /vault/secrets

# Secure permissions
sudo chmod 700 /vault/secrets
```

### **2. Generate Secrets**

```bash
# DataStorage OAuth2-Proxy Cookie Secret
openssl rand -base64 32 | sudo tee /vault/secrets/ds-cookie-secret.txt

# HolmesGPT API OAuth2-Proxy Cookie Secret
openssl rand -base64 32 | sudo tee /vault/secrets/hapi-cookie-secret.txt

# Verify secrets are 32 bytes
cat /vault/secrets/ds-cookie-secret.txt | base64 -d | wc -c  # Should output: 32
cat /vault/secrets/hapi-cookie-secret.txt | base64 -d | wc -c  # Should output: 32
```

### **3. Secure Files**

```bash
# Restrict permissions (owner read-only)
sudo chmod 600 /vault/secrets/*.txt

# Set ownership
sudo chown root:root /vault/secrets/*.txt

# Verify permissions
ls -la /vault/secrets/
# Expected: -rw------- 1 root root ... ds-cookie-secret.txt
```

---

## 🚀 **Deployment**

### **Deploy Production Secrets**

```bash
# Apply secrets from files
kubectl apply -k deploy/secrets/production/

# Verify secrets created
kubectl get secrets -n kubernaut-system | grep oauth-proxy
```

### **Expected Output**

```
data-storage-oauth-proxy-secret       Opaque   1      10s
holmesgpt-api-oauth-proxy-secret      Opaque   1      10s
```

---

## 🔍 **Verification**

### **Check Secret Values Match Files**

```bash
# Get secret from Kubernetes
kubectl get secret data-storage-oauth-proxy-secret -n kubernaut-system \
  -o jsonpath='{.data.cookie-secret}' | base64 -d > /tmp/k8s-secret.txt

# Compare with source file
diff /tmp/k8s-secret.txt /vault/secrets/ds-cookie-secret.txt
# Expected: No output (files match)

# Cleanup
rm /tmp/k8s-secret.txt
```

---

## 🔄 **Secret Rotation**

### **Rotate Cookie Secrets**

```bash
# 1. Generate new secrets
openssl rand -base64 32 | sudo tee /vault/secrets/ds-cookie-secret.txt
openssl rand -base64 32 | sudo tee /vault/secrets/hapi-cookie-secret.txt

# 2. Delete existing secrets from Kubernetes
kubectl delete secret data-storage-oauth-proxy-secret -n kubernaut-system
kubectl delete secret holmesgpt-api-oauth-proxy-secret -n kubernaut-system

# 3. Redeploy secrets
kubectl apply -k deploy/secrets/production/

# 4. Restart deployments to pick up new secrets
kubectl rollout restart deployment/data-storage-service -n kubernaut-system
kubectl rollout restart deployment/holmesgpt-api -n kubernaut-system
```

---

## 💾 **Backup & Recovery**

### **Backup Secrets**

```bash
# Backup to encrypted location
sudo tar czf /backup/kubernaut-secrets-$(date +%Y%m%d).tar.gz /vault/secrets/

# Verify backup
tar tzf /backup/kubernaut-secrets-$(date +%Y%m%d).tar.gz
```

### **Restore Secrets**

```bash
# Extract from backup
sudo tar xzf /backup/kubernaut-secrets-YYYYMMDD.tar.gz -C /

# Redeploy to Kubernetes
kubectl apply -k deploy/secrets/production/
```

---

## 🏢 **Multi-Environment Strategy**

### **Directory Structure**

```
/vault/
└── secrets/
    ├── dev/
    │   ├── ds-cookie-secret.txt
    │   └── hapi-cookie-secret.txt
    ├── staging/
    │   ├── ds-cookie-secret.txt
    │   └── hapi-cookie-secret.txt
    └── production/
        ├── ds-cookie-secret.txt
        └── hapi-cookie-secret.txt
```

### **Environment-Specific Deployment**

```yaml
# deploy/secrets/production/kustomization.yaml
secretGenerator:
  - name: data-storage-oauth-proxy-secret
    files:
      - cookie-secret=/vault/secrets/production/ds-cookie-secret.txt
```

---

## 🔒 **Security Best Practices**

### **1. Encrypted Filesystem**

```bash
# Example: LUKS-encrypted partition
sudo cryptsetup luksFormat /dev/sdb1
sudo cryptsetup open /dev/sdb1 vault
sudo mkfs.ext4 /dev/mapper/vault
sudo mount /dev/mapper/vault /vault
```

### **2. Access Control**

```bash
# Only root and deployment user should access
sudo groupadd kubernaut-deploy
sudo usermod -a -G kubernaut-deploy deploy-user
sudo chgrp kubernaut-deploy /vault/secrets/*.txt
sudo chmod 640 /vault/secrets/*.txt
```

### **3. Audit Logging**

```bash
# Enable audit on secrets directory (Linux auditd)
sudo auditctl -w /vault/secrets/ -p rwa -k kubernaut-secrets
```

---

## 🚨 **Troubleshooting**

### **Error: File Not Found**

```
Error: couldn't execute literal substitution for ...
```

**Solution**: Verify file paths exist:
```bash
ls -la /vault/secrets/ds-cookie-secret.txt
```

### **Error: Permission Denied**

```
Error: open /vault/secrets/ds-cookie-secret.txt: permission denied
```

**Solution**: Fix permissions:
```bash
sudo chmod 644 /vault/secrets/*.txt  # Allow kubectl to read
```

### **Secret Not Updating**

If secret doesn't change after updating file:

```bash
# Force delete and recreate
kubectl delete secret data-storage-oauth-proxy-secret -n kubernaut-system
kubectl apply -k deploy/secrets/production/
```

---

## 📚 **References**

- **[DD-AUTH-008: Secret Management Strategy](../../../docs/architecture/decisions/DD-AUTH-008-secret-management-kustomize-helm.md)**
- **[LUKS Encryption Guide](https://wiki.archlinux.org/title/Dm-crypt/Device_encryption)**
- **[Kubernetes Secrets Best Practices](https://kubernetes.io/docs/concepts/security/secrets-good-practices/)**
- **[OAuth2-Proxy Documentation](https://oauth2-proxy.github.io/oauth2-proxy/docs/)**

---

**Last Updated**: January 26, 2026  
**Environment**: Production  
**Authority**: DD-AUTH-008
