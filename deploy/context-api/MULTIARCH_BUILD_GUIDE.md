# Context API Multi-Architecture Build Guide

## Overview

This guide explains how to build the Context API for **both amd64 and arm64** architectures using OpenShift S2I builds and combine them into a single multi-arch image on quay.io.

**Architecture**: Based on ADR-027 (Multi-Architecture Build Strategy with Red Hat UBI)

## Prerequisites

1. OpenShift cluster with S2I build capability
2. quay.io account with push permissions
3. Git repository with Context API source code

## Architecture Components

```
┌─────────────────────────────────────────────────────────────────┐
│ GitHub: feature/phase2_services                                  │
│ └── docker/context-api.Dockerfile (supports GOARCH build arg)   │
└─────────────────────────────────────────────────────────────────┘
                            │
                            ├──────────────┬──────────────┐
                            ▼              ▼              ▼
                    ┌──────────────┐ ┌──────────────┐
                    │ BuildConfig  │ │ BuildConfig  │
                    │   amd64      │ │   arm64      │
                    └──────────────┘ └──────────────┘
                            │              │
                            ▼              ▼
                    ┌──────────────┐ ┌──────────────┐
                    │ImageStreamTag│ │ImageStreamTag│
                    │:latest-amd64 │ │:latest-arm64 │
                    └──────────────┘ └──────────────┘
                            │              │
                            └──────┬───────┘
                                   ▼
                            ┌──────────────┐
                            │ Manifest Job │
                            │  (buildah)   │
                            └──────────────┘
                                   │
                                   ▼
                        ┌────────────────────┐
                        │  quay.io/jordigilh │
                        │  /context-api      │
                        │  :latest (manifest)│
                        │  ├── linux/amd64   │
                        │  └── linux/arm64   │
                        └────────────────────┘
```

## Step 1: Create quay.io Authentication Secret

### Option A: Using kubectl (Recommended)

```bash
kubectl create secret docker-registry quay-io-push-secret \
  --docker-server=quay.io \
  --docker-username=YOUR_QUAY_USERNAME \
  --docker-password=YOUR_QUAY_PASSWORD \
  --namespace=kubernaut-system
```

### Option B: Using YAML Template

Edit `quay-secret-template.yaml` and replace credentials, then:

```bash
oc apply -f deploy/context-api/quay-secret-template.yaml
```

### Verify Secret

```bash
oc get secret quay-io-push-secret -n kubernaut-system -o yaml
```

## Step 2: Apply Multi-Architecture Build Configuration

```bash
# Apply the complete multi-arch build setup
oc apply -f deploy/context-api/buildconfig-multiarch.yaml
```

This creates:
- ImageStream: `context-api`
- BuildConfig: `context-api-amd64` (builds for linux/amd64)
- BuildConfig: `context-api-arm64` (builds for linux/arm64)
- ServiceAccount: `context-api-manifest-builder`
- Role/RoleBinding: For ImageStream access
- Job template: `context-api-manifest-push` (combines and pushes to quay.io)

## Step 3: Start Architecture-Specific Builds

### Build AMD64 Image

```bash
oc start-build context-api-amd64 -n kubernaut-system --follow
```

### Build ARM64 Image

```bash
oc start-build context-api-arm64 -n kubernaut-system --follow
```

### Verify Both Builds Completed

```bash
# Check build status
oc get builds -n kubernaut-system | grep context-api

# Expected output:
# context-api-amd64-1   Complete   ...
# context-api-arm64-1   Complete   ...

# Verify ImageStreamTags exist
oc get imagestreamtags -n kubernaut-system | grep context-api

# Expected output:
# context-api:latest-amd64   ...
# context-api:latest-arm64   ...
```

## Step 4: Create and Push Manifest List

Once both architecture-specific builds complete, run the manifest creation job:

```bash
# Delete previous job if exists
oc delete job context-api-manifest-push -n kubernaut-system 2>/dev/null || true

# Create new manifest push job
oc create -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: context-api-manifest-push
  namespace: kubernaut-system
  labels:
    app: context-api
    component: build
    job-type: manifest-creation
spec:
  backoffLimit: 3
  template:
    metadata:
      labels:
        app: context-api
        component: build
        job-type: manifest-creation
    spec:
      serviceAccountName: context-api-manifest-builder
      restartPolicy: OnFailure
      containers:
      - name: buildah
        image: quay.io/buildah/stable:latest
        imagePullPolicy: IfNotPresent
        securityContext:
          privileged: true
        env:
          - name: STORAGE_DRIVER
            value: vfs
          - name: BUILDAH_ISOLATION
            value: chroot
          - name: IMAGE_REGISTRY
            value: image-registry.openshift-image-registry.svc:5000
          - name: NAMESPACE
            value: kubernaut-system
          - name: QUAY_REGISTRY
            value: quay.io/jordigilh
          - name: IMAGE_NAME
            value: context-api
          - name: IMAGE_TAG
            value: latest
        volumeMounts:
          - name: quay-auth
            mountPath: /tmp/quay-auth
            readOnly: true
        command:
        - /bin/bash
        - -c
        - |
          set -e
          echo "🔐 Configuring authentication..."
          
          mkdir -p \${HOME}/.docker
          cp /tmp/quay-auth/.dockerconfigjson \${HOME}/.docker/config.json
          
          INTERNAL_TOKEN=\$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
          buildah login -u serviceaccount -p \${INTERNAL_TOKEN} \${IMAGE_REGISTRY} --tls-verify=false
          
          echo "📥 Pulling architecture-specific images from internal registry..."
          buildah pull --tls-verify=false \${IMAGE_REGISTRY}/\${NAMESPACE}/\${IMAGE_NAME}:latest-amd64
          buildah pull --tls-verify=false \${IMAGE_REGISTRY}/\${NAMESPACE}/\${IMAGE_NAME}:latest-arm64
          
          echo "📦 Creating manifest list..."
          buildah manifest create \${IMAGE_NAME}:\${IMAGE_TAG}
          buildah manifest add \${IMAGE_NAME}:\${IMAGE_TAG} \${IMAGE_REGISTRY}/\${NAMESPACE}/\${IMAGE_NAME}:latest-amd64
          buildah manifest add \${IMAGE_NAME}:\${IMAGE_TAG} \${IMAGE_REGISTRY}/\${NAMESPACE}/\${IMAGE_NAME}:latest-arm64
          
          echo "✅ Manifest list created:"
          buildah manifest inspect \${IMAGE_NAME}:\${IMAGE_TAG}
          
          echo "📤 Pushing multi-arch manifest to quay.io..."
          buildah tag \${IMAGE_NAME}:\${IMAGE_TAG} \${QUAY_REGISTRY}/\${IMAGE_NAME}:\${IMAGE_TAG}
          buildah manifest push --all \${QUAY_REGISTRY}/\${IMAGE_NAME}:\${IMAGE_TAG} docker://\${QUAY_REGISTRY}/\${IMAGE_NAME}:\${IMAGE_TAG}
          
          echo "🎉 Successfully pushed multi-arch image to \${QUAY_REGISTRY}/\${IMAGE_NAME}:\${IMAGE_TAG}"
          
          buildah manifest push --all \${QUAY_REGISTRY}/\${IMAGE_NAME}:\${IMAGE_TAG} docker://\${QUAY_REGISTRY}/\${IMAGE_NAME}:v0.1.0
          echo "🎉 Also tagged as v0.1.0"
      volumes:
        - name: quay-auth
          secret:
            secretName: quay-io-push-secret
            items:
              - key: .dockerconfigjson
                path: .dockerconfigjson
EOF
```

### Monitor Manifest Push Job

```bash
# Watch job status
oc get jobs -n kubernaut-system -w

# View job logs
oc logs job/context-api-manifest-push -n kubernaut-system -f
```

Expected output:
```
🔐 Configuring authentication...
📥 Pulling architecture-specific images from internal registry...
📦 Creating manifest list...
✅ Manifest list created:
{
   "schemaVersion": 2,
   "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
   "manifests": [
      {
         "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
         "platform": {
            "architecture": "amd64",
            "os": "linux"
         }
      },
      {
         "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
         "platform": {
            "architecture": "arm64",
            "os": "linux"
         }
      }
   ]
}
📤 Pushing multi-arch manifest to quay.io...
🎉 Successfully pushed multi-arch image to quay.io/jordigilh/context-api:latest
🎉 Also tagged as v0.1.0
```

## Step 5: Verify Multi-Arch Image on quay.io

```bash
# Verify image exists on quay.io
podman manifest inspect quay.io/jordigilh/context-api:latest

# Or using skopeo
skopeo inspect --raw docker://quay.io/jordigilh/context-api:latest | jq .
```

Expected output should show **two manifests** (one for amd64, one for arm64).

## Step 6: Update Deployment to Use quay.io Image

Update `deploy/context-api/deployment.yaml`:

```yaml
spec:
  template:
    spec:
      containers:
      - name: context-api
        image: quay.io/jordigilh/context-api:latest  # Multi-arch manifest
        imagePullPolicy: Always
```

Apply and rollout:

```bash
oc apply -f deploy/context-api/deployment.yaml
oc rollout restart deployment/context-api -n kubernaut-system
```

## Automation: Complete Build Pipeline

To rebuild everything:

```bash
#!/bin/bash
set -e

echo "🔨 Starting multi-arch build pipeline..."

# Step 1: Start architecture-specific builds
echo "📦 Building amd64 image..."
oc start-build context-api-amd64 -n kubernaut-system --wait

echo "📦 Building arm64 image..."
oc start-build context-api-arm64 -n kubernaut-system --wait

# Step 2: Create and push manifest
echo "📦 Creating manifest list and pushing to quay.io..."
oc delete job context-api-manifest-push -n kubernaut-system 2>/dev/null || true
oc create -f deploy/context-api/buildconfig-multiarch.yaml --selector=job-type=manifest-creation

# Step 3: Wait for manifest push to complete
oc wait --for=condition=complete job/context-api-manifest-push -n kubernaut-system --timeout=10m

# Step 4: Rollout deployment
echo "🚀 Deploying updated image..."
oc rollout restart deployment/context-api -n kubernaut-system
oc rollout status deployment/context-api -n kubernaut-system --timeout=5m

echo "✅ Multi-arch build and deployment complete!"
echo "🔍 Verify: podman manifest inspect quay.io/jordigilh/context-api:latest"
```

Save as `build-and-deploy-multiarch.sh` and run:

```bash
chmod +x build-and-deploy-multiarch.sh
./build-and-deploy-multiarch.sh
```

## Troubleshooting

### Build Fails with "Exec format error"

**Problem**: Binary compiled for wrong architecture.

**Solution**: 
- Check GOARCH build arg in BuildConfig
- Verify node architecture: `oc get nodes -o wide`

### Manifest Push Job Fails with Authentication Error

**Problem**: quay.io credentials incorrect or missing.

**Solution**:
```bash
# Recreate secret with correct credentials
oc delete secret quay-io-push-secret -n kubernaut-system
kubectl create secret docker-registry quay-io-push-secret \
  --docker-server=quay.io \
  --docker-username=YOUR_QUAY_USERNAME \
  --docker-password=YOUR_QUAY_PASSWORD \
  --namespace=kubernaut-system
```

### "Error pulling image from internal registry"

**Problem**: ImageStreamTag not found.

**Solution**:
```bash
# Verify both builds completed
oc get imagestreamtags -n kubernaut-system | grep context-api

# If missing, rebuild:
oc start-build context-api-amd64 -n kubernaut-system --wait
oc start-build context-api-arm64 -n kubernaut-system --wait
```

### Buildah Job Requires Privileged Access

**Problem**: `privileged: true` security requirement.

**Solution**: In OpenShift, the buildah container needs privileged access to create containers. This is expected for build operations. Ensure the ServiceAccount has proper SCCs:

```bash
oc adm policy add-scc-to-user privileged -z context-api-manifest-builder -n kubernaut-system
```

## Architecture Decision

This multi-arch build strategy follows **ADR-027: Multi-Architecture Container Build Strategy with Red Hat UBI Base Images**.

**Key Benefits**:
- ✅ **True Multi-Architecture Support**: Single image tag works on both amd64 and arm64
- ✅ **Enterprise Compliance**: Uses Red Hat UBI9 base images
- ✅ **OpenShift-Native**: Leverages S2I and internal registry
- ✅ **Automated Pipeline**: Can be fully automated with OpenShift Pipelines/Tekton
- ✅ **OCI Standards**: Manifest lists follow OCI specification

## Related Documentation

- [ADR-027: Multi-Architecture Build Strategy](../../docs/architecture/decisions/ADR-027-multi-architecture-build-strategy.md)
- [Context API Implementation Plan](../../docs/services/stateless/context-api/implementation/IMPLEMENTATION_PLAN_V2.0.md)
- [Context API Build Guide](../../docs/services/stateless/context-api/BUILD.md)
- [Gap Analysis: Context vs Notification](../../docs/services/stateless/context-api/implementation/CONTEXT_VS_NOTIFICATION_GAP_ANALYSIS.md)


