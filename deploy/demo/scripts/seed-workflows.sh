#!/usr/bin/env bash
# Seed workflow catalog in DataStorage via REST API
# Registers OCI-based remediation workflows that the LLM can discover
#
# Usage:
#   ./deploy/demo/scripts/seed-workflows.sh [DATASTORAGE_URL]
#
# Default DATASTORAGE_URL: http://localhost:30081

set -euo pipefail

DATASTORAGE_URL="${1:-http://localhost:30081}"
SA_TOKEN=""

echo "==> Seeding workflow catalog at ${DATASTORAGE_URL}"

# Get a ServiceAccount token for authentication (DD-AUTH-014)
get_sa_token() {
    SA_TOKEN=$(kubectl create token holmesgpt-api-sa -n kubernaut-system --duration=10m 2>/dev/null || echo "")
    if [ -z "$SA_TOKEN" ]; then
        echo "WARNING: Could not create SA token, proceeding without auth"
    fi
}

# POST a workflow to DataStorage
register_workflow() {
    local payload="$1"
    local name="$2"

    local auth_header=""
    if [ -n "$SA_TOKEN" ]; then
        auth_header="-H \"Authorization: Bearer ${SA_TOKEN}\""
    fi

    echo "  Registering workflow: ${name}"
    eval curl -s -X POST "${DATASTORAGE_URL}/api/v1/workflows" \
        -H "Content-Type: application/json" \
        ${auth_header} \
        -d "'${payload}'" \
        -o /dev/null -w "    HTTP %{http_code}\n"
}

get_sa_token

# Workflow 1: OOMKill Memory Increase (remediation for OOMKilled containers)
# DD-WORKFLOW-017: OCI-based registration -- DataStorage pulls the image,
# extracts /workflow-schema.yaml, and populates all catalog fields from it.
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/oomkill-increase-memory-job:latest"
}' "oomkill-increase-memory-job"

# Workflow 2: CrashLoop Config Fix (remediation for CrashLoopBackOff)
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/crashloop-config-fix-job:latest"
}' "crashloop-config-fix-job"

# ============================================
# Demo Scenario Workflows (#114, #119-#130)
# Built by: deploy/demo/scripts/build-demo-workflows.sh
# ============================================

# Workflow 3: GitOps Revert (#125 -- GitOps drift remediation)
# actionType: GitRevertCommit | detectedLabels: gitOpsTool: "*"
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/git-revert-job:v1.0.0"
}' "git-revert-job"

# Workflow 4: Node Provisioning (#126 -- Cluster autoscaling)
# actionType: ProvisionNode
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/provision-node-job:v1.0.0"
}' "provision-node-job"

# Workflow 5: Proactive Rollback (#128 -- SLO error budget burn)
# actionType: ProactiveRollback
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/proactive-rollback-job:v1.0.0"
}' "proactive-rollback-job"

# Workflow 6: Graceful Restart (#129 -- Predictive memory exhaustion)
# actionType: GracefulRestart
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/graceful-restart-job:v1.0.0"
}' "graceful-restart-job"

# Workflow 7: CrashLoop Rollback (#120 -- CrashLoopBackOff remediation)
# actionType: GracefulRestart
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/crashloop-rollback-job:v1.0.0"
}' "crashloop-rollback-job"

# Workflow 8: Patch HPA (#123 -- HPA maxed out)
# actionType: PatchHPA | detectedLabels: hpaEnabled: "true"
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/patch-hpa-job:v1.0.0"
}' "patch-hpa-job"

# Workflow 9: Relax PDB (#124 -- PDB deadlock)
# actionType: RelaxPDB | detectedLabels: pdbProtected: "true"
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/relax-pdb-job:v1.0.0"
}' "relax-pdb-job"

# Workflow 10: Remove Taint (#122 -- Pending pods due to node taint)
# actionType: RemoveTaint
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/remove-taint-job:v1.0.0"
}' "remove-taint-job"

# Workflow 11: Cleanup PVC (#121 -- Orphaned PVC disk pressure)
# actionType: CleanupPVC
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/cleanup-pvc-job:v1.0.0"
}' "cleanup-pvc-job"

# Workflow 12: Cordon + Drain (#127 -- Node NotReady)
# actionType: CordonDrainNode
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/cordon-drain-job:v1.0.0"
}' "cordon-drain-job"

# Workflow 13: Rollback Deployment (#130 -- Stuck rollout)
# actionType: GracefulRestart
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/rollback-deployment-job:v1.0.0"
}' "rollback-deployment-job"

# ============================================
# New Demo Scenario Workflows (#133-#138)
# ============================================

# Workflow 14: Fix Certificate (#133 -- cert-manager CRD failure)
# actionType: FixCertificate
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/fix-certificate-job:v1.0.0"
}' "fix-certificate-job"

# Workflow 15: Fix Certificate GitOps (#134 -- cert-manager + ArgoCD)
# actionType: GitRevertCommit | detectedLabels: gitOpsTool: "*"
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/fix-certificate-gitops-job:v1.0.0"
}' "fix-certificate-gitops-job"

# Workflow 16: Helm Rollback (#135 -- Helm-managed CrashLoopBackOff)
# actionType: HelmRollback | detectedLabels: helmManaged: "true"
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/helm-rollback-job:v1.0.0"
}' "helm-rollback-job"

# Workflow 17: Fix AuthorizationPolicy (#136 -- Linkerd mesh routing)
# actionType: FixAuthorizationPolicy | detectedLabels: serviceMesh: "*"
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/fix-authz-policy-job:v1.0.0"
}' "fix-authz-policy-job"

# Workflow 18: Fix StatefulSet PVC (#137 -- StatefulSet PVC failure)
# actionType: FixStatefulSetPVC | detectedLabels: stateful: "true"
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/fix-statefulset-pvc-job:v1.0.0"
}' "fix-statefulset-pvc-job"

# Workflow 19: Fix NetworkPolicy (#138 -- NetworkPolicy traffic block)
# actionType: FixNetworkPolicy | detectedLabels: networkIsolated: "true"
register_workflow '{
  "container_image": "quay.io/kubernaut-cicd/test-workflows/fix-network-policy-job:v1.0.0"
}' "fix-network-policy-job"

echo ""
echo "==> Workflow seeding complete (19 workflows)"
echo "==> Verify: curl -s ${DATASTORAGE_URL}/api/v1/workflows | jq '.'"
