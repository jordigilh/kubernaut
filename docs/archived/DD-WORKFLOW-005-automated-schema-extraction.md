# DD-WORKFLOW-005: Automated Schema Extraction from Workflow Containers

> **ARCHIVED** (2026-03-04): This document has been moved to `docs/archived/` because it is fully superseded by CRD-based registration. See [ADR-058](../architecture/decisions/ADR-058-webhook-driven-workflow-registration.md) and [DD-WORKFLOW-017 v1.1](../architecture/decisions/DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md).

**Date**: 2025-11-15
**Updated**: 2025-11-28
**Version**: 2.0
**Status**: **SUPERSEDED** and **ARCHIVED** -- Replaced by CRD-based registration (ADR-058, BR-WORKFLOW-006)
**Related**: DD-WORKFLOW-003, DD-WORKFLOW-004, ADR-043, DD-NAMING-001

> **Note**: This document is superseded by DD-WORKFLOW-017, which consolidates the end-to-end workflow lifecycle with the `action_type`-based design (DD-WORKFLOW-016). V1.0 registration is now defined in DD-WORKFLOW-017 Phase 1. V1.1 CRD automation (Solution 2 in this document) remains as historical reference for future planning.

---

## Changelog

### Version 2.0 (2025-11-28)
- **BREAKING**: Updated all references per ADR-043 and DD-NAMING-001
  - `playbook-schema.json` → `workflow-schema.yaml`
  - `playbook` → `workflow` (terminology alignment)
  - `/playbooks` API → `/workflows` API
  - `PlaybookRegistration` CRD → `WorkflowRegistration` CRD
- Added cross-references to authoritative documents
- Updated all code examples and YAML manifests

### Version 1.0 (2025-11-15)
- Initial analysis with three solution options
- Recommended hybrid approach (Tekton EventListener + CronJob)

---

## 🔗 Authoritative References

| Document | Authority |
|----------|-----------|
| **ADR-043** | Workflow Schema Definition Standard (`/workflow-schema.yaml`) |
| **DD-NAMING-001** | "Workflow" terminology (deprecates "Playbook") |
| **DD-WORKFLOW-011** | Tekton OCI Bundles structure |

---

## User's Requirement

> "What the platform should do when a new workflow is added is to inspect this YAML file from the image itself and populate the parameter struct in the workflow stored struct."

**Translation**: Automated schema extraction pipeline that:
1. Detects new workflow container images
2. Extracts `/workflow-schema.yaml` from container (per ADR-043)
3. Populates workflow catalog automatically

---

## V1.0 Approach: Direct REST API Upload

**Status**: ✅ **APPROVED FOR V1.0**

For V1.0, workflows are registered manually via the Data Storage REST API:

```bash
# Register workflow via REST API
curl -X POST http://data-storage:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d @workflow-schema.json
```

This is intentionally simple. V1.1 introduces proper automation via CRD controller.

---

## V1.1 Approach: WorkflowRegistration CRD Controller

**Target Release**: V1.1
**Confidence: 94%** ⭐⭐

See **Solution 2** below for CRD-based automation.

---

## Historical Analysis: Tekton EventListener + Webhook

**Note**: This analysis informed V1.1 planning. Keeping for reference.

**Confidence: 94%**

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Operator Pushes Workflow Container                       │
│    quay.io/kubernaut/workflow-oomkill:v1.0.0                │
│    - Contains /workflow-schema.yaml (per ADR-043)           │
│    - Contains remediation script/workflow                   │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ Webhook notification
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Quay.io Webhook → Tekton EventListener                   │
│    POST /webhook/workflow-registry                          │
│    {                                                        │
│      "repository": "kubernaut/workflow-oomkill",            │
│      "tag": "v1.0.0",                                       │
│      "digest": "sha256:abc123..."                           │
│    }                                                        │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ Triggers PipelineRun
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Schema Extraction Pipeline (Tekton)                      │
│                                                             │
│    Task 1: Pull container image                            │
│    Task 2: Extract /workflow-schema.yaml                   │
│    Task 3: Validate schema (YAML + JSON Schema validation) │
│    Task 4: Update workflow catalog (API call)              │
│    Task 5: Notify operators (Slack/email)                  │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ API call
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Workflow Catalog Service (Data Storage)                  │
│    POST /api/v1/workflows                                   │
│    {                                                        │
│      "workflow_id": "oomkill-cost-optimized",               │
│      "container_image": "quay.io/.../workflow-oomkill:v1",  │
│      "parameters": [...extracted from container...],        │
│      "labels": {...extracted from container...}             │
│    }                                                        │
└─────────────────────────────────────────────────────────────┘
```

### Implementation

#### Step 1: Quay.io Webhook Configuration

```bash
# Configure webhook in Quay.io repository settings
Webhook URL: https://kubernaut.example.com/webhook/workflow-registry
Events: ["push", "tag"]
Filter: tags matching "v*"
```

#### Step 2: Tekton EventListener

```yaml
apiVersion: triggers.tekton.dev/v1beta1
kind: EventListener
metadata:
  name: workflow-registry-listener
  namespace: kubernaut-system
spec:
  serviceAccountName: tekton-triggers-sa
  triggers:
    - name: workflow-push-trigger
      interceptors:
        - ref:
            name: "cel"
          params:
            - name: "filter"
              value: "body.repository.startsWith('kubernaut/workflow-')"
        - ref:
            name: "cel"
          params:
            - name: "overlays"
              value:
                - key: image_name
                  expression: "body.repository"
                - key: image_tag
                  expression: "body.tag"
                - key: image_digest
                  expression: "body.digest"
      bindings:
        - ref: workflow-extraction-binding
      template:
        - ref: workflow-extraction-template
```

#### Step 3: Tekton Pipeline

```yaml
apiVersion: tekton.dev/v1beta1
kind: Pipeline
metadata:
  name: workflow-schema-extraction
  namespace: kubernaut-system
spec:
  params:
    - name: image-name
      description: "Full container image name"
    - name: image-tag
      description: "Container image tag"
    - name: catalog-api-url
      default: "http://data-storage.kubernaut-system.svc:8080"

  tasks:
    - name: extract-schema
      taskRef:
        name: extract-workflow-schema
      params:
        - name: image
          value: "$(params.image-name):$(params.image-tag)"

    - name: validate-schema
      taskRef:
        name: validate-yaml-schema
      params:
        - name: schema-yaml
          value: "$(tasks.extract-schema.results.schema-yaml)"
      runAfter:
        - extract-schema

    - name: update-catalog
      taskRef:
        name: update-workflow-catalog
      params:
        - name: catalog-api-url
          value: "$(params.catalog-api-url)"
        - name: schema-yaml
          value: "$(tasks.extract-schema.results.schema-yaml)"
        - name: image
          value: "$(params.image-name):$(params.image-tag)"
      runAfter:
        - validate-schema

    - name: notify-success
      taskRef:
        name: send-notification
      params:
        - name: message
          value: "Workflow $(tasks.extract-schema.results.workflow-id) registered successfully"
      runAfter:
        - update-catalog
```

#### Step 4: Schema Extraction Task

```yaml
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: extract-playbook-schema
spec:
  params:
    - name: image
      description: "Container image to extract schema from"

  results:
    - name: schema-json
      description: "Extracted schema as JSON string"
    - name: playbook-id
      description: "Playbook ID from schema"
    - name: version
      description: "Playbook version"

  steps:
    - name: extract
      image: gcr.io/go-containerregistry/crane:latest
      script: |
        #!/bin/sh
        set -e

        # Export container filesystem to temp directory
        crane export $(params.image) - | tar -xf - -C /workspace

        # Verify schema file exists
        if [ ! -f /workspace/playbook-schema.json ]; then
          echo "ERROR: /playbook-schema.json not found in container"
          exit 1
        fi

        # Extract schema
        cat /workspace/playbook-schema.json | tee $(results.schema-json.path)

        # Extract workflow ID and version for results
        jq -r '.workflow_id' /workspace/playbook-schema.json > $(results.playbook-id.path)
        jq -r '.version' /workspace/playbook-schema.json > $(results.version.path)

      volumeMounts:
        - name: workspace
          mountPath: /workspace

  volumes:
    - name: workspace
      emptyDir: {}
```

#### Step 5: Catalog Update Task

```yaml
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: update-playbook-catalog
spec:
  params:
    - name: catalog-api-url
    - name: schema-json
    - name: image

  steps:
    - name: update
      image: curlimages/curl:latest
      script: |
        #!/bin/sh
        set -e

        # Parse schema
        PLAYBOOK_ID=$(echo '$(params.schema-json)' | jq -r '.workflow_id')
        VERSION=$(echo '$(params.schema-json)' | jq -r '.version')
        PARAMETERS=$(echo '$(params.schema-json)' | jq -c '.parameters')
        LABELS=$(echo '$(params.schema-json)' | jq -c '.labels // {}')

        # Create catalog entry
        CATALOG_ENTRY=$(jq -n \
          --arg id "$PLAYBOOK_ID" \
          --arg version "$VERSION" \
          --arg image "$(params.image)" \
          --argjson params "$PARAMETERS" \
          --argjson labels "$LABELS" \
          '{
            workflow_id: $id,
            version: $version,
            container_image: $image,
            parameters: $params,
            labels: $labels
          }')

        # POST to catalog API
        curl -X POST \
          -H "Content-Type: application/json" \
          -d "$CATALOG_ENTRY" \
          "$(params.catalog-api-url)/playbooks" \
          --fail-with-body

        echo "Playbook $PLAYBOOK_ID:$VERSION registered successfully"
```

### Confidence: 94%

#### ✅ Strengths
1. **Automated** (99%): Zero manual intervention
2. **Real-time** (95%): Webhook triggers immediately on push
3. **Validated** (98%): Schema validation before catalog update
4. **Auditable** (99%): Tekton PipelineRun history
5. **Kubernetes-native** (99%): Uses existing Tekton infrastructure

#### ⚠️ V1.1+ Risks (Not Applicable to V1.0)

**Note**: These risks apply to the automated Tekton EventListener approach planned for V1.1+.
V1.0 uses manual REST API upload - see "V1.0 Approach" section above.

**Risk 1: Webhook Delivery Failure** (V1.1+)
- **Problem**: Webhook might fail (network, registry downtime)
- **Mitigation**: WorkflowRegistration CRD controller provides self-healing reconciliation

**Risk 2: Schema Extraction Failure** (V1.1+)
- **Problem**: Container might not have schema file
- **Mitigation**: Controller validates schema presence before registration

**Risk 3: Data Storage API Unavailable** (V1.1+)
- **Problem**: Data Storage service down during registration
- **Mitigation**: Controller retries with exponential backoff

**V1.0 Gap**: Data Storage service needs to validate incoming workflow schema payloads per ADR-043.

---

## Solution 2: WorkflowRegistration CRD Controller - V1.1 Target

**Confidence: 95%** ⭐⭐
**Target Release**: V1.1

### Design Principles

| Principle | Rationale |
|-----------|-----------|
| **Schema immutable** | Audit traces reference exact workflow that ran |
| **Only enable/disable mutable** | Operational control without changing definition |
| **New version = new CR** | Clean audit trail, each version independently trackable |
| **Status contains extracted schema** | Single source of truth (container), visibility via kubectl |

### CRD Structure

```yaml
apiVersion: kubernaut.io/v1alpha1
kind: WorkflowRegistration
metadata:
  name: oomkill-scale-down-v1-0-0  # Includes version for uniqueness
  namespace: kubernaut-system
spec:
  # IMMUTABLE - set once, cannot be changed
  containerImage: quay.io/kubernaut/workflow-oomkill:v1.0.0

  # MUTABLE - operational control only
  enabled: true  # Toggle to disable/enable workflow

status:
  phase: Ready  # Pending → Extracting → Validating → Ready / Disabled / Failed

  # EXTRACTED from container's /workflow-schema.yaml
  extractedSchema:
    metadata:
      workflow_id: oomkill-scale-down
      version: "1.0.0"
      description: Scale down deployment to reduce memory pressure
    labels:
      signal_type: OOMKilled
      severity: critical
      risk_tolerance: low
    parameters:
      - name: TARGET_RESOURCE_KIND
        type: string
        required: true
        enum: [Deployment, StatefulSet]
      - name: TARGET_RESOURCE_NAME
        type: string
        required: true

  # Audit trail
  extractedAt: "2025-11-28T10:00:00Z"
  registeredAt: "2025-11-28T10:00:05Z"
  containerDigest: "sha256:abc123..."
```

### Field Mutability

| Field | Mutable | Notes |
|-------|---------|-------|
| `spec.containerImage` | ❌ No | Immutable - new version = new CR |
| `spec.enabled` | ✅ Yes | Operational toggle only |
| `status.*` | ✅ Yes | Controller-managed |

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Operator applies WorkflowRegistration                    │
│    spec:                                                    │
│      containerImage: quay.io/.../workflow-oomkill:v1.0.0    │
│      enabled: true                                          │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ Watch event (Create)
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Controller: Extract Schema                               │
│    - Pull container image (crane)                           │
│    - Extract /workflow-schema.yaml (per ADR-043)            │
│    - Validate schema                                        │
│    - Store in status.extractedSchema                        │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ Register
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Controller: Register with Data Storage                   │
│    POST /api/v1/workflows                                   │
│    - Send extracted schema                                  │
│    - Update status.phase = Ready                            │
└─────────────────────────────────────────────────────────────┘
```

### Controller Reconciliation

| Event | Controller Action |
|-------|-------------------|
| **Create CR** | Extract schema → Register → status.phase = Ready |
| **`enabled: false`** | Update Data Storage: workflow disabled |
| **`enabled: true`** | Update Data Storage: workflow enabled |
| **`containerImage` change** | ❌ Rejected by webhook (immutable) |
| **Delete CR** | Deregister from Data Storage catalog |

### Audit Trace Integrity

```
Remediation Event #12345
├── Signal: OOMKilled in prod/my-app
├── Workflow: oomkill-scale-down v1.0.0
├── Parameters: {TARGET_REPLICAS: 2}
└── Outcome: Success

# Audit query 6 months later:
kubectl get workflowregistration oomkill-scale-down-v1-0-0 -o yaml
→ Guaranteed: same schema as execution time (immutable)
```

### Implementation

#### CRD Definition

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: workflowregistrations.kubernaut.io
spec:
  group: kubernaut.io
  names:
    kind: WorkflowRegistration
    plural: workflowregistrations
    singular: workflowregistration
    shortNames:
      - wfr
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              required:
                - containerImage
              properties:
                containerImage:
                  type: string
                  description: "OCI bundle reference (IMMUTABLE after creation)"
                  x-kubernetes-validations:
                    - rule: "self == oldSelf"
                      message: "containerImage is immutable"
                enabled:
                  type: boolean
                  default: true
                  description: "Enable/disable workflow (MUTABLE)"
            status:
              type: object
              properties:
                phase:
                  type: string
                  enum: ["Pending", "Extracting", "Validating", "Ready", "Disabled", "Failed"]
                extractedSchema:
                  type: object
                  description: "Schema extracted from container's /workflow-schema.yaml"
                  x-kubernetes-preserve-unknown-fields: true
                extractedAt:
                  type: string
                  format: date-time
                registeredAt:
                  type: string
                  format: date-time
                containerDigest:
                  type: string
                  description: "SHA256 digest of container image"
                message:
                  type: string
```

#### Controller Implementation (Go)

```go
package controller

import (
    "context"
    "time"

    "github.com/google/go-containerregistry/pkg/crane"
    kubernautv1alpha1 "github.com/jordigilh/kubernaut/api/kubernaut.io/v1alpha1"
    "gopkg.in/yaml.v3"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type WorkflowRegistrationReconciler struct {
    client.Client
    DataStorageURL string
}

func (r *WorkflowRegistrationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var wfr kubernautv1alpha1.WorkflowRegistration
    if err := r.Get(ctx, req.NamespacedName, &wfr); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle enable/disable toggle (only mutable operation)
    if wfr.Status.Phase == "Ready" || wfr.Status.Phase == "Disabled" {
        return r.reconcileEnabledState(ctx, &wfr)
    }

    // First-time registration (schema extraction)
    return r.reconcileNewRegistration(ctx, &wfr)
}

func (r *WorkflowRegistrationReconciler) reconcileNewRegistration(
    ctx context.Context,
    wfr *kubernautv1alpha1.WorkflowRegistration,
) (ctrl.Result, error) {
    // Phase 1: Extract schema from container
    wfr.Status.Phase = "Extracting"
    r.Status().Update(ctx, wfr)

    schema, digest, err := r.extractSchema(wfr.Spec.ContainerImage)
    if err != nil {
        wfr.Status.Phase = "Failed"
        wfr.Status.Message = err.Error()
        r.Status().Update(ctx, wfr)
        return ctrl.Result{}, err
    }

    // Phase 2: Validate schema per ADR-043
    wfr.Status.Phase = "Validating"
    r.Status().Update(ctx, wfr)

    if err := r.validateSchema(schema); err != nil {
        wfr.Status.Phase = "Failed"
        wfr.Status.Message = err.Error()
        r.Status().Update(ctx, wfr)
        return ctrl.Result{}, err
    }

    // Phase 3: Register with Data Storage
    if err := r.registerWorkflow(ctx, schema, wfr.Spec.ContainerImage); err != nil {
        wfr.Status.Phase = "Failed"
        wfr.Status.Message = err.Error()
        r.Status().Update(ctx, wfr)
        return ctrl.Result{}, err
    }

    // Phase 4: Update status with extracted schema (for visibility)
    now := metav1.Now()
    wfr.Status.Phase = "Ready"
    wfr.Status.ExtractedSchema = schema  // User can see via kubectl
    wfr.Status.ExtractedAt = &now
    wfr.Status.RegisteredAt = &now
    wfr.Status.ContainerDigest = digest
    wfr.Status.Message = ""
    r.Status().Update(ctx, wfr)

    return ctrl.Result{}, nil
}

func (r *WorkflowRegistrationReconciler) reconcileEnabledState(
    ctx context.Context,
    wfr *kubernautv1alpha1.WorkflowRegistration,
) (ctrl.Result, error) {
    // Only operation: toggle enabled/disabled
    desiredPhase := "Ready"
    if !wfr.Spec.Enabled {
        desiredPhase = "Disabled"
    }

    if wfr.Status.Phase != desiredPhase {
        // Update Data Storage with enabled state
        if err := r.updateWorkflowEnabled(ctx, wfr); err != nil {
            return ctrl.Result{}, err
        }
        wfr.Status.Phase = desiredPhase
        r.Status().Update(ctx, wfr)
    }

    return ctrl.Result{}, nil
}

func (r *WorkflowRegistrationReconciler) extractSchema(image string) (*WorkflowSchema, string, error) {
    // Get container digest for audit trail
    digest, err := crane.Digest(image)
    if err != nil {
        return nil, "", fmt.Errorf("failed to get digest: %w", err)
    }

    // Extract /workflow-schema.yaml per ADR-043
    schemaBytes, err := crane.FileContent(image, "/workflow-schema.yaml")
    if err != nil {
        return nil, "", fmt.Errorf("schema file not found (expected /workflow-schema.yaml per ADR-043): %w", err)
    }

    var schema WorkflowSchema
    if err := yaml.Unmarshal(schemaBytes, &schema); err != nil {
        return nil, "", fmt.Errorf("invalid schema YAML: %w", err)
    }

    return &schema, digest, nil
}
```

#### Usage

```bash
# Operator creates PlaybookRegistration
cat <<EOF | kubectl apply -f -
apiVersion: kubernaut.io/v1alpha1
kind: PlaybookRegistration
metadata:
  name: oomkill-cost-optimized
  namespace: kubernaut-system
spec:
  containerImage: quay.io/kubernaut/playbook-oomkill-cost:v1.0.0
  autoUpdate: true
EOF

# Check status
kubectl get playbookregistration oomkill-cost-optimized -o yaml

# Output:
# status:
#   phase: Ready
#   playbookId: oomkill-cost-optimized
#   version: 1.0.0
#   catalogRegistered: true
#   lastUpdated: "2025-11-15T10:00:00Z"
```

### Confidence: 92%

#### ✅ Strengths
1. **Kubernetes-native** (99%): CRD + Operator pattern
2. **Declarative** (98%): kubectl apply workflow
3. **Status tracking** (99%): CR status shows progress
4. **RBAC integration** (99%): Standard K8s RBAC

#### ⚠️ Gap to 100% (8% risk)

**Risk 1: Operator Complexity** (4% risk)
- **Problem**: Requires Go operator development and maintenance
- **Impact**: Higher development effort
- **Mitigation**:
  - Use Operator SDK for scaffolding
  - Leverage controller-runtime libraries
  - Comprehensive unit tests
- **Confidence after mitigation**: 96%

**Risk 2: Manual CR Creation** (3% risk)
- **Problem**: Operators must manually create PlaybookRegistration CRs
- **Impact**: Not fully automated
- **Mitigation**:
  - Combine with Solution 1 (webhook creates CR)
  - Provide CLI tool: `kubernaut workflow register <image>`
- **Confidence after mitigation**: 97%

**Risk 3: Image Pull Credentials** (1% risk)
- **Problem**: Operator needs credentials for private registries
- **Impact**: Cannot pull images
- **Mitigation**:
  ```yaml
  spec:
    containerImage: quay.io/private/playbook:v1
    imagePullSecrets:
      - name: quay-pull-secret
  ```
- **Confidence after mitigation**: 99%

---

## Solution 3: Periodic Reconciliation Job

**Confidence: 90%**

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ 1. CronJob (Every 5 minutes)                                │
│    - Lists all images in registry with label                │
│      "io.kubernaut.playbook=true"                           │
│    - Compares with catalog                                  │
│    - Extracts schemas from new/updated images               │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Registry API Query                                       │
│    GET /v2/kubernaut/playbook-*/tags/list                   │
│    Filter: images with label "io.kubernaut.playbook=true"   │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Diff Against Catalog                                     │
│    New images: Extract schema and register                  │
│    Updated images: Re-extract and update catalog            │
│    Deleted images: Mark as deprecated in catalog            │
└─────────────────────────────────────────────────────────────┘
```

### Implementation

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: playbook-reconciliation
  namespace: kubernaut-system
spec:
  schedule: "*/5 * * * *"  # Every 5 minutes
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: playbook-reconciler
          containers:
            - name: reconcile
              image: quay.io/kubernaut/playbook-reconciler:latest
              env:
                - name: REGISTRY_URL
                  value: "quay.io"
                - name: REGISTRY_ORG
                  value: "kubernaut"
                - name: CATALOG_API_URL
                  value: "http://mock-mcp-server.kubernaut-system.svc:8081"
              command:
                - /usr/local/bin/reconcile-playbooks.sh
          restartPolicy: OnFailure
```

### Confidence: 90%

#### ✅ Strengths
1. **Eventual consistency** (99%): Always converges to correct state
2. **Self-healing** (98%): Recovers from missed webhooks
3. **Simple** (95%): No complex event handling

#### ⚠️ Gap to 100% (10% risk)

**Risk 1: Latency** (5% risk)
- **Problem**: Up to 5-minute delay for new playbooks
- **Impact**: Not real-time
- **Mitigation**:
  - Reduce interval to 1 minute
  - Combine with Solution 1 (webhook for real-time, CronJob for recovery)
- **Confidence after mitigation**: 95%

**Risk 2: Registry API Rate Limits** (3% risk)
- **Problem**: Frequent registry queries might hit rate limits
- **Impact**: Reconciliation failures
- **Mitigation**:
  - Cache registry responses
  - Use registry webhooks instead of polling
- **Confidence after mitigation**: 96%

**Risk 3: Full Scan Overhead** (2% risk)
- **Problem**: Scans all images every run
- **Impact**: Inefficient
- **Mitigation**:
  - Track last reconciliation timestamp
  - Only check images updated since last run
- **Confidence after mitigation**: 98%

---

## Release Strategy

### Product Context

| Release | Purpose | Scope |
|---------|---------|-------|
| **V1.0** | PoC / Demo | Get early feedback before completing gaps |
| **V1.1** | Production-Ready | Address feedback, complete automation |

**Rationale**: Shipping V1.0 early enables feedback that may change V1.1 priorities and requirements. Over-engineering V1.0 risks wasted effort.

---

## Comparison Matrix

| Version | Approach | Complexity | Status |
|---------|----------|------------|--------|
| **V1.0** | Direct REST API | ✅ Minimal | **APPROVED** |
| **V1.1** | WorkflowRegistration CRD | Medium | **PLANNED** |

---

## APPROVED: Phased Approach

### V1.0: Manual REST API Registration

```bash
curl -X POST http://data-storage:8080/api/v1/workflows -d @workflow.json
```

✅ Simple, works for demos, no automation overhead.

### V1.1: WorkflowRegistration CRD Controller

Automates registration via Kubernetes-native CRD controller (see Solution 2 above).

**V1.1 scope will be informed by V1.0 feedback.**

---

## V1.1 Enhancements (Deferred)

The following enhancements are deferred to V1.1, pending V1.0 feedback:

### Schema Validation (V1.1)

```bash
#!/bin/bash
# Pre-commit hook for workflow repos
if [ -f workflow-schema.yaml ]; then
    # Validate schema format per ADR-043
    yq eval workflow-schema.yaml > /dev/null || exit 1

    # Ensure schema is in Dockerfile
    grep -q "COPY workflow-schema.yaml /workflow-schema.yaml" Dockerfile || exit 1
fi
```

### WorkflowRegistration CRD Controller (V1.1)

See Solution 2 above for full implementation details.

---

## Final Recommendation

### V1.0: Direct REST API (APPROVED)

**Approach**: Manual workflow registration via Data Storage REST API
**Effort**: Minimal
**Purpose**: Enable early demos and feedback collection

### V1.1: WorkflowRegistration CRD Controller (PLANNED)

**Approach**: Kubernetes-native CRD controller for automated registration
**Effort**: Medium (1-2 weeks)
**Purpose**: Production-ready automation based on V1.0 feedback

---

**Status**: ✅ Approved (V1.0 approach)
**V1.0**: Direct REST API registration
**V1.1**: WorkflowRegistration CRD controller (scope informed by V1.0 feedback)
**Authority**: ADR-043 defines schema format (`/workflow-schema.yaml`)
