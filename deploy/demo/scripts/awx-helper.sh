#!/usr/bin/env bash
# Deploy AWX via the official AWX Operator for Ansible engine demo scenarios.
# Reuses Kubernaut's shared PostgreSQL (creates an 'awx' database/role).
# The AWX Operator manages its own Redis sidecar.
#
# Ported from test/infrastructure/awx_e2e.go for use in demo environments.
#
# Usage:
#   ./deploy/demo/scripts/awx-helper.sh                     # Full AWX setup
#   ./deploy/demo/scripts/awx-helper.sh --skip-operator      # Skip operator install (already present)
#   ./deploy/demo/scripts/awx-helper.sh --configure-only     # Only configure AWX (project, templates, token)
#
# Issue #312: First Ansible engine demo scenario
set -euo pipefail

AWX_OPERATOR_VERSION="2.19.1"
AWX_IMAGE_VERSION="24.6.1"
AWX_INSTANCE_NAME="awx-demo"
AWX_SERVICE_NAME="${AWX_INSTANCE_NAME}-service"
AWX_SERVICE_PORT=80
AWX_NODEPORT=30095
AWX_NAMESPACE="${AWX_NAMESPACE:-kubernaut-system}"

AWX_DB_NAME="awx"
AWX_DB_USER="awx"
AWX_DB_PASS="awx_demo_password"
AWX_ADMIN_USER="admin"
AWX_ADMIN_PASS="admin_demo_password"
AWX_SECRET_KEY="kubernaut-demo-awx-secret-key"
AWX_TOKEN_SECRET_NAME="awx-api-token"

AWX_PLAYBOOKS_REPO="https://github.com/jordigilh/kubernaut-test-playbooks.git"

SKIP_OPERATOR=false
CONFIGURE_ONLY=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --skip-operator)   SKIP_OPERATOR=true; shift ;;
        --configure-only)  CONFIGURE_ONLY=true; shift ;;
        --namespace)       AWX_NAMESPACE="$2"; shift 2 ;;
        --help|-h)
            echo "Usage: $0 [--skip-operator] [--configure-only] [--namespace NS]"
            exit 0 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

TOTAL_START=$(date +%s)

echo "============================================="
echo " AWX Setup for Ansible Engine Demos"
echo " Namespace: ${AWX_NAMESPACE}"
echo "============================================="
echo ""

# ── 1. Install AWX Operator ─────────────────────────────────────────────────

install_awx_operator() {
    echo "==> Step 1: Installing AWX Operator ${AWX_OPERATOR_VERSION}..."

    TMPDIR_KUSTOMIZE=$(mktemp -d)
    trap "rm -rf ${TMPDIR_KUSTOMIZE}" RETURN

    cat > "${TMPDIR_KUSTOMIZE}/kustomization.yaml" <<EOF
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - github.com/ansible/awx-operator/config/default?ref=${AWX_OPERATOR_VERSION}
images:
  - name: quay.io/ansible/awx-operator
    newTag: ${AWX_OPERATOR_VERSION}
  - name: gcr.io/kubebuilder/kube-rbac-proxy
    newName: registry.k8s.io/kubebuilder/kube-rbac-proxy
    newTag: v0.15.0
namespace: ${AWX_NAMESPACE}
EOF

    kubectl apply -k "${TMPDIR_KUSTOMIZE}"

    echo "  Waiting for AWX Operator controller..."
    kubectl rollout status deployment/awx-operator-controller-manager \
        -n "${AWX_NAMESPACE}" --timeout=180s
    echo "  AWX Operator controller ready."
    echo ""
}

# ── 2. Create AWX database in shared PostgreSQL ─────────────────────────────

create_awx_database() {
    echo "==> Step 2: Creating AWX database in shared PostgreSQL..."

    PG_USER=$(kubectl get secret kubernaut-pg-credentials -n "${AWX_NAMESPACE}" \
        -o jsonpath='{.data.POSTGRES_USER}' 2>/dev/null | base64 -d 2>/dev/null || echo "slm_user")
    PG_PASS=$(kubectl get secret kubernaut-pg-credentials -n "${AWX_NAMESPACE}" \
        -o jsonpath='{.data.POSTGRES_PASSWORD}' 2>/dev/null | base64 -d 2>/dev/null || echo "test_password")
    PG_DB=$(kubectl get secret kubernaut-pg-credentials -n "${AWX_NAMESPACE}" \
        -o jsonpath='{.data.POSTGRES_DB}' 2>/dev/null | base64 -d 2>/dev/null || echo "action_history")

    kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: awx-db-init
  namespace: ${AWX_NAMESPACE}
spec:
  backoffLimit: 10
  ttlSecondsAfterFinished: 300
  template:
    spec:
      restartPolicy: OnFailure
      containers:
      - name: init
        image: postgres:16-alpine
        command: ["sh", "-c"]
        args:
        - |
          until pg_isready -h postgresql -p 5432 -U ${PG_USER}; do
            echo "Waiting for PostgreSQL..."
            sleep 2
          done
          echo "Creating AWX database and user..."
          export PGPASSWORD='${PG_PASS}'
          psql -h postgresql -p 5432 -U ${PG_USER} -d ${PG_DB} -c "CREATE ROLE ${AWX_DB_USER} WITH LOGIN PASSWORD '${AWX_DB_PASS}';" 2>&1 || true
          set -e
          psql -h postgresql -p 5432 -U ${PG_USER} -d ${PG_DB} -tc "SELECT 1 FROM pg_database WHERE datname = '${AWX_DB_NAME}'" | grep -q 1 || \
            psql -h postgresql -p 5432 -U ${PG_USER} -d ${PG_DB} -c "CREATE DATABASE ${AWX_DB_NAME} OWNER ${AWX_DB_USER};"
          psql -h postgresql -p 5432 -U ${PG_USER} -d ${PG_DB} -c "GRANT ALL PRIVILEGES ON DATABASE ${AWX_DB_NAME} TO ${AWX_DB_USER};"
          echo "AWX database ready."
EOF

    echo "  Waiting for DB init job..."
    kubectl wait --for=condition=Complete job/awx-db-init -n "${AWX_NAMESPACE}" --timeout=120s
    echo "  AWX database created."
    echo ""
}

# ── 3. Create AWX prerequisite Secrets ───────────────────────────────────────

create_awx_secrets() {
    echo "==> Step 3: Creating AWX prerequisite Secrets..."

    kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: awx-postgres-configuration
  namespace: ${AWX_NAMESPACE}
stringData:
  host: postgresql
  port: "5432"
  database: "${AWX_DB_NAME}"
  username: "${AWX_DB_USER}"
  password: "${AWX_DB_PASS}"
  sslmode: prefer
  type: unmanaged
---
apiVersion: v1
kind: Secret
metadata:
  name: ${AWX_INSTANCE_NAME}-admin-password
  namespace: ${AWX_NAMESPACE}
stringData:
  password: "${AWX_ADMIN_PASS}"
---
apiVersion: v1
kind: Secret
metadata:
  name: ${AWX_INSTANCE_NAME}-secret-key
  namespace: ${AWX_NAMESPACE}
stringData:
  secret_key: "${AWX_SECRET_KEY}"
EOF

    echo "  Secrets created (PG config, admin password, secret key)."
    echo ""
}

# ── 4. Apply AWX Custom Resource ────────────────────────────────────────────

apply_awx_cr() {
    echo "==> Step 4: Applying AWX Custom Resource (${AWX_INSTANCE_NAME})..."

    kubectl apply -f - <<EOF
apiVersion: awx.ansible.com/v1beta1
kind: AWX
metadata:
  name: ${AWX_INSTANCE_NAME}
  namespace: ${AWX_NAMESPACE}
spec:
  service_type: nodeport
  nodeport_port: ${AWX_NODEPORT}
  admin_user: ${AWX_ADMIN_USER}
  admin_password_secret: ${AWX_INSTANCE_NAME}-admin-password
  secret_key_secret: ${AWX_INSTANCE_NAME}-secret-key
  postgres_configuration_secret: awx-postgres-configuration
  image: quay.io/ansible/awx
  image_version: "${AWX_IMAGE_VERSION}"
  ee_images:
    - name: "AWX EE (latest)"
      image: "quay.io/ansible/awx-ee:${AWX_IMAGE_VERSION}"
  web_resource_requirements:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      cpu: "1"
      memory: 1Gi
  task_resource_requirements:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      cpu: "1"
      memory: 1Gi
  ee_resource_requirements:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 500m
      memory: 256Mi
  redis_resource_requirements:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 500m
      memory: 128Mi
  init_container_resource_requirements:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 256Mi
EOF

    echo "  AWX CR applied. Operator will reconcile."
    echo ""
}

# ── 5. Wait for AWX to be ready ─────────────────────────────────────────────

wait_for_awx() {
    echo "==> Step 5: Waiting for AWX to be ready (up to 12 min)..."

    local deadline=$(($(date +%s) + 720))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        local ready_count=0
        local total_count=0
        while IFS= read -r line; do
            total_count=$((total_count + 1))
            local phase status
            phase=$(echo "$line" | awk '{print $3}')
            status=$(echo "$line" | awk '{print $2}')
            if [ "$phase" = "Running" ]; then
                local ready total
                ready=$(echo "$status" | cut -d/ -f1)
                total=$(echo "$status" | cut -d/ -f2)
                if [ "$ready" = "$total" ] && [ "$ready" != "0" ]; then
                    ready_count=$((ready_count + 1))
                fi
            fi
        done < <(kubectl get pods -n "${AWX_NAMESPACE}" \
            -l "app.kubernetes.io/managed-by=awx-operator,app.kubernetes.io/part-of=${AWX_INSTANCE_NAME}" \
            --no-headers 2>/dev/null || true)

        if [ "$ready_count" -ge 2 ] && [ "$ready_count" -eq "$total_count" ]; then
            echo "  AWX is ready (${ready_count} pods running)."
            echo ""
            return 0
        fi
        echo "  AWX pods: ${ready_count}/${total_count} ready..."
        sleep 15
    done

    echo "ERROR: AWX did not become ready within 12 minutes."
    kubectl get pods -n "${AWX_NAMESPACE}" -l "app.kubernetes.io/part-of=${AWX_INSTANCE_NAME}"
    return 1
}

# ── 6. Configure AWX (project, inventory, templates, token) ─────────────────

awx_api() {
    local method="$1" url="$2" token="${3:-}"
    shift 2; [ $# -gt 0 ] && shift

    local auth_args=()
    if [ -n "$token" ]; then
        auth_args=(-H "Authorization: Bearer ${token}")
    else
        auth_args=(-u "${AWX_ADMIN_USER}:${AWX_ADMIN_PASS}")
    fi

    local data_args=()
    if [ -n "${AWX_API_BODY:-}" ]; then
        data_args=(-d "${AWX_API_BODY}")
    fi

    curl -sf -X "${method}" "${url}" \
        -H "Content-Type: application/json" \
        "${auth_args[@]}" \
        "${data_args[@]}" 2>/dev/null || true
}

configure_awx() {
    echo "==> Step 6: Configuring AWX..."

    local awx_url="http://localhost:${AWX_NODEPORT}"

    # 6a. Create organization
    echo "  Creating organization..."
    AWX_API_BODY='{"name":"Kubernaut Demo","description":"Demo organization"}'
    local org_result
    org_result=$(awx_api POST "${awx_url}/api/v2/organizations/" "")
    local org_id
    org_id=$(echo "$org_result" | jq -r '.id // empty' 2>/dev/null || echo "")
    if [ -z "$org_id" ]; then
        org_id=1
    fi
    echo "    Organization ID: ${org_id}"

    # 6b. Create project (Git SCM -> kubernaut-test-playbooks)
    echo "  Creating project..."
    AWX_API_BODY=$(jq -n \
        --arg name "kubernaut-demo-playbooks" \
        --arg desc "Ansible playbooks for Kubernaut demo scenarios" \
        --argjson org "$org_id" \
        --arg repo "${AWX_PLAYBOOKS_REPO}" \
        '{name:$name, description:$desc, organization:$org, scm_type:"git", scm_url:$repo, scm_branch:"main", scm_update_on_launch:true}')
    local proj_result
    proj_result=$(awx_api POST "${awx_url}/api/v2/projects/" "")
    local proj_id
    proj_id=$(echo "$proj_result" | jq -r '.id // empty' 2>/dev/null || echo "")

    if [ -z "$proj_id" ]; then
        echo "    Project may already exist, looking up..."
        proj_id=$(AWX_API_BODY="" awx_api GET "${awx_url}/api/v2/projects/?name=kubernaut-demo-playbooks" "" | \
            jq -r '.results[0].id // empty' 2>/dev/null || echo "")
    fi
    echo "    Project ID: ${proj_id}"

    # Wait for project sync
    echo "  Waiting for project sync..."
    local sync_deadline=$(($(date +%s) + 300))
    while [ "$(date +%s)" -lt "$sync_deadline" ]; do
        local proj_status
        proj_status=$(AWX_API_BODY="" awx_api GET "${awx_url}/api/v2/projects/${proj_id}/" "" | \
            jq -r '.status // empty' 2>/dev/null || echo "")
        if [ "$proj_status" = "successful" ]; then
            echo "    Project synced."
            break
        elif [ "$proj_status" = "failed" ] || [ "$proj_status" = "error" ]; then
            echo "ERROR: Project sync failed (status: ${proj_status})"
            return 1
        fi
        sleep 5
    done

    # 6c. Create inventory
    echo "  Creating inventory..."
    AWX_API_BODY=$(jq -n --argjson org "$org_id" \
        '{name:"localhost", description:"In-cluster execution", organization:$org}')
    local inv_result
    inv_result=$(awx_api POST "${awx_url}/api/v2/inventories/" "")
    local inv_id
    inv_id=$(echo "$inv_result" | jq -r '.id // empty' 2>/dev/null || echo "")
    echo "    Inventory ID: ${inv_id}"

    # Add localhost host
    AWX_API_BODY='{"name":"localhost","variables":"ansible_connection: local\nansible_python_interpreter: /usr/bin/python3"}'
    awx_api POST "${awx_url}/api/v2/inventories/${inv_id}/hosts/" "" >/dev/null

    # 6d. Create job template for GitOps memory update
    echo "  Creating job template (kubernaut-gitops-update-memory)..."
    AWX_API_BODY=$(jq -n --argjson proj "$proj_id" --argjson inv "$inv_id" \
        '{name:"kubernaut-gitops-update-memory", description:"GitOps: update memory limits via git commit", project:$proj, playbook:"playbooks/gitops-update-memory-limits.yml", inventory:$inv, ask_variables_on_launch:true}')
    local tmpl_result
    tmpl_result=$(awx_api POST "${awx_url}/api/v2/job_templates/" "")
    local tmpl_id
    tmpl_id=$(echo "$tmpl_result" | jq -r '.id // empty' 2>/dev/null || echo "")
    echo "    Job Template ID: ${tmpl_id}"

    # 6e. Create K8s Bearer Token credential for AWX EE
    # The playbook uses kubernetes.core.k8s_info to read Deployments, ArgoCD Applications,
    # and WorkflowExecutions. This SA provides read access from within the AWX EE.
    echo "  Creating K8s ServiceAccount for AWX EE..."
    kubectl apply -f - <<EOFK8S
apiVersion: v1
kind: ServiceAccount
metadata:
  name: awx-ee-reader
  namespace: ${AWX_NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: awx-ee-reader
rules:
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list"]
  - apiGroups: ["argoproj.io"]
    resources: ["applications"]
    verbs: ["get", "list"]
  - apiGroups: ["kubernaut.ai"]
    resources: ["workflowexecutions"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: awx-ee-reader
subjects:
  - kind: ServiceAccount
    name: awx-ee-reader
    namespace: ${AWX_NAMESPACE}
roleRef:
  kind: ClusterRole
  name: awx-ee-reader
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: v1
kind: Secret
metadata:
  name: awx-ee-reader-token
  namespace: ${AWX_NAMESPACE}
  annotations:
    kubernetes.io/service-account.name: awx-ee-reader
type: kubernetes.io/service-account-token
EOFK8S

    echo "  Waiting for SA token..."
    local sa_deadline=$(($(date +%s) + 30))
    local sa_token=""
    while [ "$(date +%s)" -lt "$sa_deadline" ]; do
        sa_token=$(kubectl get secret awx-ee-reader-token -n "${AWX_NAMESPACE}" \
            -o jsonpath='{.data.token}' 2>/dev/null | base64 -d 2>/dev/null || echo "")
        if [ -n "$sa_token" ]; then break; fi
        sleep 2
    done
    if [ -z "$sa_token" ]; then
        echo "WARNING: Could not obtain SA token for AWX EE K8s credential"
    fi

    local k8s_host="https://kubernetes.default.svc"

    echo "  Registering K8s credential in AWX..."
    AWX_API_BODY=$(jq -n \
        --argjson org "$org_id" \
        --arg token "$sa_token" \
        --arg host "$k8s_host" \
        '{name:"kubernaut-k8s-reader", description:"In-cluster K8s read access for AWX EE", organization:$org, credential_type:17, inputs:{host:$host, bearer_token:$token, verify_ssl:false}}')
    local k8s_cred_result
    k8s_cred_result=$(awx_api POST "${awx_url}/api/v2/credentials/" "")
    local k8s_cred_id
    k8s_cred_id=$(echo "$k8s_cred_result" | jq -r '.id // empty' 2>/dev/null || echo "")
    echo "    K8s credential ID: ${k8s_cred_id}"

    if [ -n "$tmpl_id" ] && [ -n "$k8s_cred_id" ]; then
        echo "  Attaching K8s credential to job template..."
        AWX_API_BODY=$(jq -n --argjson id "$k8s_cred_id" '{id:$id}')
        awx_api POST "${awx_url}/api/v2/job_templates/${tmpl_id}/credentials/" "" >/dev/null
        echo "    K8s credential attached to job template."
    fi
    echo ""

    # 6f. Create API token
    echo "  Creating API token..."
    AWX_API_BODY='{"description":"Kubernaut WE controller token","scope":"write"}'
    local token_result
    token_result=$(awx_api POST "${awx_url}/api/v2/users/1/personal_tokens/" "")
    local api_token
    api_token=$(echo "$token_result" | jq -r '.token // empty' 2>/dev/null || echo "")

    if [ -z "$api_token" ]; then
        echo "ERROR: Failed to create API token"
        return 1
    fi
    echo "    API token created."

    # 6f. Store token in K8s Secret
    echo "  Creating AWX token Secret..."
    kubectl create secret generic "${AWX_TOKEN_SECRET_NAME}" \
        -n "${AWX_NAMESPACE}" \
        --from-literal=token="${api_token}" \
        --dry-run=client -o yaml | kubectl apply -f -
    echo "    Token Secret created (${AWX_TOKEN_SECRET_NAME})."

    echo ""
    echo "  AWX configuration complete."
    echo "    AWX API: ${awx_url}"
    echo "    Job Template: kubernaut-gitops-update-memory (ID: ${tmpl_id})"
    echo ""
}

# ── 7. Patch WE controller with Ansible config ──────────────────────────────

patch_we_controller() {
    echo "==> Step 7: Patching WE controller with Ansible config..."

    local current_config
    current_config=$(kubectl get configmap workflowexecution-config \
        -n "${AWX_NAMESPACE}" -o jsonpath='{.data.workflowexecution\.yaml}' 2>/dev/null || echo "")

    if echo "$current_config" | grep -q "ansible:" 2>/dev/null; then
        echo "  Ansible config already present in WE controller ConfigMap."
    else
        local awx_internal_url="http://${AWX_SERVICE_NAME}.${AWX_NAMESPACE}:${AWX_SERVICE_PORT}"
        local ansible_section
        ansible_section=$(cat <<EOF

ansible:
  apiURL: "${awx_internal_url}"
  tokenSecretRef:
    name: "${AWX_TOKEN_SECRET_NAME}"
    namespace: "${AWX_NAMESPACE}"
    key: "token"
  insecure: true
  organizationID: ${org_id:-1}
EOF
)
        local new_config="${current_config}${ansible_section}"
        kubectl patch configmap workflowexecution-config -n "${AWX_NAMESPACE}" \
            --type merge -p "{\"data\":{\"workflowexecution.yaml\":$(echo "$new_config" | jq -Rs .)}}"
        echo "  ConfigMap updated with ansible config."
    fi

    echo "  Restarting WE controller..."
    kubectl rollout restart deployment/workflowexecution-controller -n "${AWX_NAMESPACE}"
    kubectl rollout status deployment/workflowexecution-controller \
        -n "${AWX_NAMESPACE}" --timeout=120s
    echo "  WE controller restarted with Ansible executor enabled."
    echo ""
}

# ── Main ─────────────────────────────────────────────────────────────────────

if [ "$CONFIGURE_ONLY" = true ]; then
    configure_awx
    patch_we_controller
else
    if [ "$SKIP_OPERATOR" = false ]; then
        install_awx_operator
    fi
    create_awx_database
    create_awx_secrets
    apply_awx_cr
    wait_for_awx
    configure_awx
    patch_we_controller
fi

TOTAL_END=$(date +%s)
TOTAL_DURATION=$((TOTAL_END - TOTAL_START))
total_mins=$((TOTAL_DURATION / 60))
total_secs=$((TOTAL_DURATION % 60))

echo "============================================="
echo " AWX setup complete (${total_mins}m ${total_secs}s)"
echo "============================================="
