# ADR-035: Remediation Execution Engine - Tekton Pipelines

**Status**: Accepted
**Date**: 2025-11-05
**Deciders**: Architecture Team
**Related**: ADR-033 (Remediation Playbook Catalog)

---

## Context

Kubernaut requires a **remediation execution engine** to run multi-step remediation workflows in response to Kubernetes incidents. The execution engine must:

1. **Execute Remediation Playbooks**: Run predefined, multi-step remediation patterns
2. **Kubernetes-Native**: Integrate seamlessly with Kubernetes CRD-based architecture
3. **GitOps-Aligned**: Support GitOps workflows for playbook versioning and deployment
4. **Observable**: Provide execution logs, metrics, and status tracking
5. **Extensible**: Allow future integration with non-Kubernetes remediation (VMs, cloud resources)

### Industry Standards Research

Research into industry standards for remediation execution revealed:

| Execution Engine | Use Case | Market Share | K8s-Native |
|-----------------|----------|--------------|------------|
| **Ansible** | Cross-platform (VMs + K8s + cloud) | ~60% | ❌ No (external control plane) |
| **Tekton Pipelines** | Kubernetes-native CI/CD + remediation | ~30% | ✅ Yes (K8s CRDs) |
| **Argo Workflows** | Kubernetes-native workflow orchestration | ~25% | ✅ Yes (K8s CRDs) |
| **AWS Systems Manager** | AWS-specific automation | ~20% | ❌ No (AWS-only) |
| **ServiceNow Workflow** | ITSM-integrated automation | ~15% | ❌ No (proprietary) |

**Key Findings:**
- **No universal standard**: Industry uses multiple execution engines depending on platform
- **Kubernetes ecosystem**: Tekton and Argo Workflows are the **de facto standards** for K8s-native remediation
- **Ansible dominance**: Ansible is most common for **cross-platform** remediation (VMs + K8s + cloud)
- **Emerging standards**: MITRE's Remediation Tasking Language (RTL) exists but has low adoption

---

## Decision

**We will use Tekton Pipelines as the primary remediation execution engine for Kubernaut.**

### Tekton Pipelines Overview

**Tekton** is a Kubernetes-native, open-source CI/CD framework that uses Custom Resource Definitions (CRDs) to define pipelines, tasks, and runs. It is a **Cloud Native Computing Foundation (CNCF)** project with strong industry backing.

**Core Concepts:**
- **Task**: A single unit of work (e.g., "restart pod", "scale deployment")
- **Pipeline**: A sequence of Tasks executed in order
- **TaskRun**: An execution instance of a Task
- **PipelineRun**: An execution instance of a Pipeline

**Example Remediation Playbook (Tekton Task):**
```yaml
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: pod-oom-recovery
  labels:
    kubernaut.io/incident-type: pod-oom-killer
    kubernaut.io/playbook-version: v1.0
spec:
  params:
    - name: pod-name
      type: string
    - name: namespace
      type: string
    - name: memory-limit
      type: string
  steps:
    - name: increase-memory
      image: bitnami/kubectl:latest
      script: |
        kubectl set resources deployment/$(params.pod-name) \
          --limits=memory=$(params.memory-limit) \
          -n $(params.namespace)

    - name: restart-pod
      image: bitnami/kubectl:latest
      script: |
        kubectl rollout restart deployment/$(params.pod-name) \
          -n $(params.namespace)

    - name: verify-health
      image: bitnami/kubectl:latest
      script: |
        kubectl wait --for=condition=available \
          deployment/$(params.pod-name) \
          -n $(params.namespace) \
          --timeout=120s
```

---

## Rationale

### Why Tekton?

#### 1. **Kubernetes-Native** (Primary Reason)
- **CRD-Based**: Tekton uses Kubernetes CRDs, matching Kubernaut's architecture
- **No External Dependencies**: Runs entirely within Kubernetes (no external control plane)
- **Operator Pattern**: Integrates seamlessly with Kubernaut's CRD-based design
- **kubectl-Compatible**: Playbooks are standard Kubernetes YAML manifests

#### 2. **CNCF Project** (Industry Validation)
- **Open Source**: Apache 2.0 license, vendor-neutral
- **Strong Backing**: Red Hat (OpenShift Pipelines), Google (Cloud Build), IBM (Cloud Continuous Delivery)
- **Active Development**: Regular releases, large contributor base
- **Production-Ready**: Used by major enterprises for mission-critical workloads

#### 3. **GitOps-Aligned** (Best Practice)
- **Declarative**: Playbooks are YAML files stored in Git
- **Version Control**: Playbook changes tracked via Git commits
- **Auditable**: Full history of playbook modifications
- **Rollback-Friendly**: Revert to previous playbook versions via Git

#### 4. **Observable** (Operational Excellence)
- **Kubernetes Events**: Execution status visible via `kubectl get pipelineruns`
- **Logs**: Task logs accessible via `kubectl logs`
- **Metrics**: Prometheus metrics for execution duration, success rate
- **Dashboards**: Tekton Dashboard UI for visual monitoring

#### 5. **Extensible** (Future-Proof)
- **Custom Tasks**: Can invoke Ansible playbooks, Terraform, or any containerized tool
- **Multi-Cloud**: Execute remediation across multiple Kubernetes clusters
- **Hybrid Workflows**: Combine K8s-native actions with external system calls

---

## Alternatives Considered

### Alternative 1: Ansible

**Pros:**
- Industry standard for cross-platform automation (60% market share)
- Mature ecosystem with thousands of pre-built playbooks
- Strong community and documentation
- Can manage VMs, cloud resources, and Kubernetes

**Cons:**
- **Not Kubernetes-Native**: Requires external Ansible Tower/AWX control plane
- **Operational Complexity**: Additional infrastructure to manage
- **Not CRD-Based**: Doesn't integrate with Kubernaut's CRD architecture
- **Overkill for K8s-Only**: Kubernaut focuses on Kubernetes remediation

**Decision**: Rejected for MVP. May integrate as a **Phase 2 enhancement** for cross-platform remediation.

---

### Alternative 2: Argo Workflows

**Pros:**
- Kubernetes-native (CRD-based)
- CNCF project (same as Tekton)
- Strong GitOps integration
- Powerful DAG-based workflow orchestration

**Cons:**
- **More Complex**: Argo Workflows is more feature-rich but harder to learn
- **Overlap with Argo CD**: May confuse users if using Argo CD for GitOps
- **Less CI/CD Focus**: Argo Workflows is more general-purpose (not CI/CD-optimized)

**Decision**: Rejected. Tekton is more focused on CI/CD-style workflows (sequential tasks), which matches remediation patterns better.

---

### Alternative 3: Custom Workflow Engine

**Pros:**
- Full control over execution logic
- Tailored to Kubernaut's specific needs
- No external dependencies

**Cons:**
- **Reinventing the Wheel**: Tekton already solves this problem
- **Maintenance Burden**: Custom engine requires ongoing development
- **No Community**: Users must learn Kubernaut-specific syntax
- **No Ecosystem**: Can't leverage existing Tekton tasks/pipelines

**Decision**: Rejected. Custom engines are only justified when existing solutions are inadequate.

---

## Implementation Strategy

### Phase 1: Tekton-Only (MVP) ✅ **Current**

**Scope**: Kubernetes-native remediation only

**Playbook Format**: Tekton `Task` and `Pipeline` CRDs

**Example Use Cases:**
- Pod OOM recovery (increase memory, restart)
- Node pressure remediation (evict pods, scale down)
- Disk pressure cleanup (delete old logs, prune images)
- Network policy fixes (update ingress rules)

**Deliverables:**
1. **Playbook Catalog Service**: Store and retrieve Tekton Task definitions
2. **RemediationExecution CRD**: Trigger Tekton PipelineRuns from Kubernaut
3. **Success Rate Tracking**: Integrate with Data Storage Service (ADR-033)
4. **AI Selection**: AI selects playbooks from catalog based on incident type

**Timeline**: Included in ADR-033 implementation (Days 12-16)

---

### Phase 2: Ansible Integration (Future Enhancement) 🔮

**Scope**: Cross-platform remediation (VMs, cloud resources, external systems)

**Integration Pattern**: Tekton Tasks invoke Ansible playbooks

**Example Hybrid Workflow:**
```yaml
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: hybrid-remediation
spec:
  steps:
    # Step 1: K8s-native action (Tekton)
    - name: scale-k8s-deployment
      image: bitnami/kubectl:latest
      script: |
        kubectl scale deployment/app --replicas=5

    # Step 2: VM remediation (Ansible via Tekton)
    - name: restart-vm-service
      image: ansible/ansible-runner:latest
      script: |
        ansible-playbook -i inventory playbooks/restart-service.yml

    # Step 3: Cloud resource update (Terraform via Tekton)
    - name: update-cloud-firewall
      image: hashicorp/terraform:latest
      script: |
        terraform apply -auto-approve
```

**Triggers for Phase 2:**
- User requests for non-Kubernetes remediation
- Multi-cloud deployment scenarios
- Legacy VM infrastructure integration

**Estimated Timeline**: 6-12 months post-MVP

---

## Consequences

### Positive

1. **✅ Kubernetes-Native**: Seamless integration with Kubernaut's CRD-based architecture
2. **✅ Industry Standard**: Tekton is the de facto standard for K8s-native CI/CD and automation
3. **✅ GitOps-Aligned**: Playbooks stored in Git, version-controlled, auditable
4. **✅ Observable**: Full visibility into execution via Kubernetes events and logs
5. **✅ Extensible**: Can invoke Ansible, Terraform, or any containerized tool in Phase 2
6. **✅ No Vendor Lock-In**: Open-source CNCF project with multi-vendor support
7. **✅ Reduced Complexity**: No external control plane (Ansible Tower/AWX) required

### Negative

1. **⚠️ Kubernetes-Only (MVP)**: Phase 1 does not support VM or cloud resource remediation
2. **⚠️ Learning Curve**: Users must learn Tekton syntax (mitigated by templates and examples)
3. **⚠️ YAML Complexity**: Complex playbooks can become verbose (mitigated by task reuse)
4. **⚠️ Limited Ansible Ecosystem**: Cannot directly use Ansible Galaxy playbooks (Phase 2 addresses this)

### Neutral

1. **🔄 Tekton vs Argo Workflows**: Both are valid choices; Tekton chosen for CI/CD focus
2. **🔄 Future Flexibility**: Phase 2 Ansible integration provides cross-platform escape hatch

---

## Validation

### Success Criteria

1. **Playbook Execution**: Tekton Tasks execute successfully for common incident types
2. **AI Integration**: AI can select and trigger Tekton playbooks from catalog
3. **Success Rate Tracking**: Execution results stored in Data Storage Service (ADR-033)
4. **GitOps Workflow**: Playbooks stored in Git, version-controlled, auditable
5. **User Adoption**: Users can create custom playbooks using Tekton syntax

### Monitoring

- **Metric**: Playbook execution success rate (target: >90%)
- **Metric**: Average playbook execution time (target: <2 minutes)
- **Metric**: Playbook catalog size (target: 20+ playbooks by end of Q1 2026)
- **Feedback**: User surveys on Tekton usability (target: >4/5 satisfaction)

---

## References

- **Tekton Documentation**: https://tekton.dev/docs/
- **CNCF Tekton Project**: https://www.cncf.io/projects/tekton/
- **ADR-033**: Remediation Playbook Catalog (Multi-Dimensional Success Tracking)
- **Industry Research**: Remediation execution standards (November 2025)
- **MITRE RTL**: Remediation Tasking Language (https://www.mitre.org/sites/default/files/pdf/11_3822.pdf)

---

## Approval

**Approved By**: Architecture Team
**Date**: 2025-11-05
**Next Review**: 2026-05-05 (6 months post-MVP)

---

## Appendix: Playbook Catalog Schema

The **Remediation Playbook Catalog** (ADR-033) will store Tekton Task definitions with metadata:

```yaml
apiVersion: kubernaut.io/v1alpha1
kind: RemediationPlaybook
metadata:
  name: pod-oom-recovery
  labels:
    kubernaut.io/incident-type: pod-oom-killer
    kubernaut.io/version: v1.0
spec:
  description: "Increase pod memory and restart on OOM"
  incidentType: pod-oom-killer
  successRate: 0.85  # Tracked via ADR-033
  executionCount: 120  # Tracked via ADR-033
  tektonTask:
    apiVersion: tekton.dev/v1
    kind: Task
    metadata:
      name: pod-oom-recovery
    spec:
      # ... (Tekton Task definition)
```

This schema allows:
- **AI Selection**: Query playbooks by `incidentType` and `successRate`
- **Version Control**: Track playbook versions via Git
- **Success Tracking**: Store execution results in Data Storage Service (ADR-033)
- **Extensibility**: Add `ansiblePlaybook` field in Phase 2 for hybrid workflows

