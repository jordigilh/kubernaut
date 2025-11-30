# DD-WORKFLOW-004: Parameter Schema Location - Analysis

**Date**: 2025-11-15
**Status**: ✅ **SUPERSEDED by ADR-043**
**Related**: DD-WORKFLOW-003, ADR-043

---

## ⚠️ SUPERSEDED

**This document has been superseded by [ADR-043: Workflow Schema Definition Standard](./ADR-043-workflow-schema-definition-standard.md).**

**Decision**: Option B (Hybrid - Catalog with Schema, Container Validates) was approved and formalized in ADR-043.

**Key Changes from ADR-043**:
- Schema file renamed: `playbook-schema.json` → `workflow-schema.yaml`
- YAML format (more readable than JSON)
- Rich validation support (enum, pattern, min/max)
- Discovery labels included in schema file

---

## Original Analysis (Historical Reference)

---

## User's Question

> "Provide a confidence assessment on having the workflow images containing a JSON file in its root directory with a predefined name that contains the parameter struct (compatible with Tekton format), so that we can just use as is in the Tekton pipeline and we don't need migration."

---

## Proposed Approach: Schema-in-Container

### Concept

Each workflow container image includes a JSON file at a predefined location:

```
/playbook-schema.json
```

**Example Container Structure**:
```
playbook-oomkill-cost:v1.0.0
├── /playbook-schema.json          # Parameter schema
├── /usr/local/bin/remediate.sh    # Workflow script
└── /etc/ansible/playbook.yml      # Or Ansible playbook
```

**Example `/playbook-schema.json`**:
```json
{
  "workflow_id": "oomkill-cost-optimized",
  "version": "1.0.0",
  "parameters": [
    {
      "name": "TARGET_RESOURCE_KIND",
      "type": "string",
      "required": true,
      "enum": ["Deployment", "StatefulSet"]
    },
    {
      "name": "TARGET_RESOURCE_NAME",
      "type": "string",
      "required": true
    },
    {
      "name": "REMEDIATION_ACTION",
      "type": "string",
      "required": true,
      "enum": ["scale_down", "increase_memory"]
    }
  ]
}
```

---

## Confidence Assessment: 75% (MODERATE-HIGH)

### ✅ Pros (Strong Points)

#### 1. **Self-Documenting Containers** (Confidence: 95%)
- Schema travels with the container
- Version-locked: schema matches implementation
- No schema drift between catalog and container
- Operators control both schema and implementation

**Example**:
```bash
# Inspect container schema without running it
docker run --rm playbook-oomkill-cost:v1.0.0 cat /playbook-schema.json
```

#### 2. **No Migration Needed** (Confidence: 90%)
- Tekton can read schema directly from container
- No separate schema management system
- No catalog sync issues

**Tekton Task Example**:
```yaml
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: playbook-executor
spec:
  steps:
    - name: extract-schema
      image: $(params.playbook-image)
      command: ["cat", "/playbook-schema.json"]

    - name: validate-parameters
      image: $(params.playbook-image)
      script: |
        #!/bin/bash
        # Validate provided params against schema
        python3 /usr/local/bin/validate-params.py

    - name: execute-playbook
      image: $(params.playbook-image)
      env:
        - name: TARGET_RESOURCE_KIND
          value: $(params.TARGET_RESOURCE_KIND)
```

#### 3. **Operator Autonomy** (Confidence: 92%)
- Operators define schema when building container
- No dependency on external catalog updates
- Schema changes = new container version

#### 4. **Immutability** (Confidence: 95%)
- Container image = immutable
- Schema cannot be changed without rebuilding
- Strong version guarantees

---

### ❌ Cons (Critical Issues)

#### 1. **LLM Cannot Discover Playbooks** (Confidence: 95%) ⚠️ **CRITICAL**

**Problem**: The LLM needs to know which playbooks exist and their parameters BEFORE execution.

**Current Flow**:
```
LLM Investigation → Search Workflow Catalog → Get Schema → Populate Parameters → Execute
```

**With Schema-in-Container**:
```
LLM Investigation → ??? How to discover playbooks ??? → Must pull ALL containers to read schemas
```

**Impact**:
- Cannot search playbooks by labels (signal_type, severity, etc.)
- Must pull every container image to read schema
- Slow, expensive, impractical

**Example Failure**:
```
User: "OOMKilled incident in production"
LLM: "I need to find relevant playbooks..."
System: "Pulling 50 container images to read schemas..."
Result: 2-3 minutes just to discover playbooks ❌
```

#### 2. **No Searchability** (Confidence: 90%) ⚠️ **CRITICAL**

**Problem**: Cannot search playbooks by metadata without pulling containers.

**Current Need** (from DD-WORKFLOW-001):
```python
# Search by incident characteristics
playbooks = mcp_client.search_playbooks(
    signal_type="OOMKilled",
    severity="high",
    business_category="cost-management"
)
```

**With Schema-in-Container**:
- No way to search without pulling all images
- No index of available playbooks
- No metadata-based filtering

#### 3. **Container Pull Overhead** (Confidence: 85%)

**Problem**: Must pull container just to read schema.

**Costs**:
- Network bandwidth
- Registry API calls
- Time (seconds per container)
- Storage (if caching)

**Example**:
```
50 workflow containers × 100MB each = 5GB just to read schemas
```

#### 4. **Schema Validation Timing** (Confidence: 80%)

**Problem**: Schema validation happens AFTER LLM populates parameters.

**Current Flow**:
```
1. Get schema from catalog (fast)
2. LLM populates parameters
3. Validate against schema
4. Execute if valid
```

**With Schema-in-Container**:
```
1. LLM populates parameters (no schema guidance)
2. Pull container
3. Extract schema
4. Validate (might fail)
5. Execute if valid
```

**Impact**: Wasted LLM calls if parameters don't match schema.

#### 5. **Catalog Still Needed** (Confidence: 85%)

**Reality**: You still need a catalog for discovery.

**Minimum Catalog Entry**:
```json
{
  "workflow_id": "oomkill-cost-optimized",
  "container_image": "quay.io/kubernaut/playbook-oomkill-cost:v1.0.0",
  "labels": {
    "signal_type": "OOMKilled",
    "severity": "high"
  }
}
```

**Result**: Duplicated metadata (catalog + container).

---

## Alternative Approaches

### Alternative 1: **Hybrid - Catalog with Schema, Container Validates** ⭐
**Confidence: 92%** - **RECOMMENDED**

**Concept**:
- Catalog contains parameter schema (for discovery and LLM guidance)
- Container contains same schema (for validation at execution time)

**Benefits**:
- ✅ Fast workflow discovery (catalog)
- ✅ LLM has schema before populating parameters
- ✅ Container validates at execution (defense-in-depth)
- ✅ Schema drift detection (compare catalog vs container)

**Architecture**:
```
┌─────────────────────────────────────────────────────────────┐
│ 1. Workflow Catalog (Fast Discovery)                        │
│    - Workflow metadata + labels                             │
│    - Parameter schema (for LLM)                             │
│    - Container image reference                              │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. LLM Parameter Population                                 │
│    - Uses schema from catalog                               │
│    - Populates parameters                                   │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Container Execution (Validation)                         │
│    - Reads /playbook-schema.json                            │
│    - Validates parameters against container's schema        │
│    - Detects schema drift (catalog ≠ container)             │
│    - Executes if valid                                      │
└─────────────────────────────────────────────────────────────┘
```

**Schema Drift Detection**:
```bash
# In container entrypoint
#!/bin/bash
CONTAINER_SCHEMA=$(cat /playbook-schema.json)
CATALOG_SCHEMA="${CATALOG_SCHEMA_JSON}"  # Passed as env var

if ! diff <(echo "$CONTAINER_SCHEMA") <(echo "$CATALOG_SCHEMA"); then
    echo "WARNING: Schema drift detected between catalog and container"
    echo "Container version: $(jq -r .version /playbook-schema.json)"
fi

# Validate parameters against container schema
python3 /usr/local/bin/validate-params.py /playbook-schema.json
```

**Catalog Entry**:
```json
{
  "workflow_id": "oomkill-cost-optimized",
  "version": "1.0.0",
  "container_image": "quay.io/kubernaut/playbook-oomkill-cost:v1.0.0",

  "parameters": [
    {
      "name": "TARGET_RESOURCE_KIND",
      "type": "string",
      "required": true,
      "enum": ["Deployment", "StatefulSet"]
    }
  ],

  "labels": {
    "signal_type": "OOMKilled",
    "severity": "high"
  }
}
```

**Container `/playbook-schema.json`** (same schema):
```json
{
  "workflow_id": "oomkill-cost-optimized",
  "version": "1.0.0",
  "parameters": [
    {
      "name": "TARGET_RESOURCE_KIND",
      "type": "string",
      "required": true,
      "enum": ["Deployment", "StatefulSet"]
    }
  ]
}
```

---

### Alternative 2: **Schema Extraction Pipeline**
**Confidence: 78%**

**Concept**: Automated pipeline extracts schemas from containers and updates catalog.

**Architecture**:
```
┌─────────────────────────────────────────────────────────────┐
│ 1. Operator Builds Workflow Container                       │
│    - Includes /playbook-schema.json                         │
│    - Pushes to registry                                     │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Schema Extraction Pipeline (Tekton/Argo)                 │
│    - Detects new container image (webhook)                  │
│    - Pulls container                                        │
│    - Extracts /playbook-schema.json                         │
│    - Updates workflow catalog                               │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Workflow Catalog (Auto-Updated)                          │
│    - Schema extracted from container                        │
│    - Searchable by LLM                                      │
└─────────────────────────────────────────────────────────────┘
```

**Pros**:
- ✅ Single source of truth (container)
- ✅ Automated catalog updates
- ✅ No manual catalog maintenance

**Cons**:
- ❌ Complex pipeline to build
- ❌ Delay between container push and catalog update
- ❌ Still need catalog for discovery

---

### Alternative 3: **OCI Annotations**
**Confidence: 82%**

**Concept**: Store schema in OCI image annotations (metadata).

**Example**:
```bash
# Build container with schema annotation
docker build -t playbook-oomkill-cost:v1.0.0 \
  --label "io.kubernaut.playbook.schema=$(cat playbook-schema.json)" \
  .
```

**Pros**:
- ✅ Schema in image metadata (no file inside container)
- ✅ Can read without pulling layers (faster)
- ✅ OCI-standard approach

**Cons**:
- ❌ Annotation size limits (varies by registry)
- ❌ Still need to query registry for each image
- ❌ Not all registries support large annotations
- ❌ Still slower than catalog

---

### Alternative 4: **Schema-Only Containers**
**Confidence: 70%**

**Concept**: Separate lightweight containers with just schemas.

**Example**:
```
playbook-oomkill-cost:v1.0.0           # 500MB (actual playbook)
playbook-oomkill-cost-schema:v1.0.0    # 1KB (just schema)
```

**Pros**:
- ✅ Fast schema discovery (tiny images)
- ✅ Schema versioned with playbook

**Cons**:
- ❌ Double the images to manage
- ❌ Version sync complexity
- ❌ Still need catalog for search

---

## Comparison Matrix

| Approach | Discovery Speed | LLM Guidance | Schema Drift | Operator Burden | Confidence |
|----------|----------------|--------------|--------------|-----------------|------------|
| **Schema-in-Container Only** | ❌ Slow (pull all) | ❌ No (after populate) | ✅ Impossible | ✅ Low | **75%** |
| **Hybrid (Catalog + Container)** ⭐ | ✅ Fast (catalog) | ✅ Yes (catalog) | ✅ Detectable | ⚠️ Medium | **92%** |
| **Schema Extraction Pipeline** | ✅ Fast (catalog) | ✅ Yes (catalog) | ✅ Auto-sync | ❌ High (pipeline) | **78%** |
| **OCI Annotations** | ⚠️ Medium (registry) | ⚠️ Partial | ⚠️ Difficult | ⚠️ Medium | **82%** |
| **Schema-Only Containers** | ⚠️ Medium (pull schemas) | ✅ Yes | ✅ Versioned | ❌ High (2x images) | **70%** |

---

## Recommended Approach: Hybrid (92% Confidence)

### Why Hybrid is Superior

**1. Solves Discovery Problem** (95% confidence)
- Catalog enables fast workflow search
- LLM gets schema before populating parameters
- No container pulls during discovery

**2. Defense-in-Depth Validation** (90% confidence)
- Catalog schema guides LLM
- Container schema validates at execution
- Detects schema drift

**3. Operator-Friendly** (88% confidence)
- Operators define schema once
- Include in both catalog and container
- Simple validation script in container

**4. Migration Path** (85% confidence)
- Start with catalog-only (v1.0)
- Add container validation (v1.1)
- Enable drift detection (v1.2)

---

## Implementation: Hybrid Approach

### Step 1: Catalog Schema (v1.0)

**Workflow Catalog Entry**:
```json
{
  "workflow_id": "oomkill-cost-optimized",
  "version": "1.0.0",
  "container_image": "quay.io/kubernaut/playbook-oomkill-cost:v1.0.0",

  "parameters": [
    {
      "name": "TARGET_RESOURCE_KIND",
      "type": "string",
      "required": true,
      "enum": ["Deployment", "StatefulSet", "DaemonSet"]
    },
    {
      "name": "TARGET_RESOURCE_NAME",
      "type": "string",
      "required": true,
      "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
    },
    {
      "name": "REMEDIATION_ACTION",
      "type": "string",
      "required": true,
      "enum": ["scale_down", "increase_memory"]
    }
  ],

  "labels": {
    "signal_type": "OOMKilled",
    "severity": "high",
    "priority": "P1"
  }
}
```

### Step 2: Container Schema (v1.1)

**Dockerfile**:
```dockerfile
FROM bitnami/kubectl:latest

# Copy schema for validation
COPY playbook-schema.json /playbook-schema.json

# Copy validation script
COPY validate-params.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/validate-params.sh

# Copy workflow script
COPY remediate.sh /usr/local/bin/remediate.sh
RUN chmod +x /usr/local/bin/remediate.sh

# Entrypoint validates then executes
ENTRYPOINT ["/usr/local/bin/validate-params.sh"]
```

**`validate-params.sh`**:
```bash
#!/bin/bash
set -euo pipefail

SCHEMA_FILE="/playbook-schema.json"

# Validate required parameters exist
REQUIRED_PARAMS=$(jq -r '.parameters[] | select(.required==true) | .name' "$SCHEMA_FILE")

for param in $REQUIRED_PARAMS; do
    if [ -z "${!param:-}" ]; then
        echo "ERROR: Required parameter $param is missing"
        exit 1
    fi
done

# Validate enum values
# (More sophisticated validation with jq/python)

echo "Parameter validation passed"

# Execute playbook
exec /usr/local/bin/remediate.sh
```

### Step 3: Schema Drift Detection (v1.2)

**Tekton Task**:
```yaml
apiVersion: tekton.dev/v1beta1
kind: Task
metadata:
  name: playbook-executor-with-drift-detection
spec:
  params:
    - name: playbook-image
    - name: catalog-schema-json
      description: Schema from catalog (for drift detection)

  steps:
    - name: detect-schema-drift
      image: $(params.playbook-image)
      script: |
        #!/bin/bash
        CONTAINER_SCHEMA=$(cat /playbook-schema.json)
        CATALOG_SCHEMA='$(params.catalog-schema-json)'

        CONTAINER_VERSION=$(echo "$CONTAINER_SCHEMA" | jq -r .version)
        CATALOG_VERSION=$(echo "$CATALOG_SCHEMA" | jq -r .version)

        if [ "$CONTAINER_VERSION" != "$CATALOG_VERSION" ]; then
            echo "WARNING: Schema version mismatch"
            echo "Container: $CONTAINER_VERSION"
            echo "Catalog: $CATALOG_VERSION"
        fi

        # Compare parameter schemas
        if ! diff <(echo "$CONTAINER_SCHEMA" | jq -S .parameters) \
                  <(echo "$CATALOG_SCHEMA" | jq -S .parameters); then
            echo "ERROR: Schema drift detected"
            exit 1
        fi

    - name: execute-playbook
      image: $(params.playbook-image)
      env:
        - name: TARGET_RESOURCE_KIND
          value: $(params.TARGET_RESOURCE_KIND)
```

---

## Summary

### User's Proposal: Schema-in-Container Only
**Confidence: 75%** (MODERATE-HIGH)

**Critical Issue**: Cannot discover playbooks without pulling all containers.

**Recommendation**: ❌ **NOT RECOMMENDED** as sole approach

---

### Recommended: Hybrid (Catalog + Container)
**Confidence: 92%** (HIGH)

**Benefits**:
- ✅ Fast discovery (catalog)
- ✅ LLM guidance (catalog schema)
- ✅ Execution validation (container schema)
- ✅ Schema drift detection
- ✅ Defense-in-depth

**Trade-off**: Schema maintained in two places (catalog + container)

**Mitigation**:
- Operators define schema once
- Copy to both locations
- Automated drift detection

---

## Decision Needed

**Q1**: Accept schema duplication (catalog + container) for better discovery?
- **A)** Yes - Hybrid approach (92% confidence) ⭐
- **B)** No - Build schema extraction pipeline (78% confidence)
- **C)** No - Use OCI annotations (82% confidence)

**Q2**: When to implement container validation?
- **A)** v1.0 (catalog only, add container later)
- **B)** v1.1 (both from start)

**Recommendation**: Q1=A, Q2=A (start simple, add validation later)

---

**Status**: Analysis Complete - Awaiting Decision
**Confidence**: 92% for Hybrid approach
**Risk**: Low (proven pattern)
**Effort**: Low (simple implementation)
