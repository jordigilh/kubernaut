# BR-PLATFORM-005: Helm Chart Security & Network-Policy Parity with the Kubernaut Operator

**Business Requirement ID**: BR-PLATFORM-005
**Category**: Platform
**Priority**: P2
**Target Version**: V1.5
**Status**: Approved
**Date**: 2026-07-06

---

## Business Need

### Problem Statement

A follow-up triage of the Kubernaut Operator's resource-generation code
(`kubernaut-operator/internal/resources/*.go`) against the Helm chart (Issue #1589 scope)
identified five security- and network-isolation-relevant gaps where the Helm chart's rendered
resources are weaker than, or missing relative to, what the Operator produces for the same
logical deployment:

1. **Ansible/AWX custom CA trust**: the Operator supports a `caCertSecretRef` on the Ansible
   engine config, building a combined trust bundle (inter-service CA + custom CA) via a
   `build-ca-bundle` init container. The Helm chart had no equivalent — self-signed/private-CA AWX
   endpoints could not be trusted without disabling TLS verification.
2. **WorkflowExecution egress for Ansible**: the WorkflowExecution NetworkPolicy did not permit
   HTTPS (443) egress, so enabling the Ansible engine against an AWX/AAP endpoint outside the
   cluster's known peers was silently blocked by the chart's own default-deny policy.
3. **APIFrontend NetworkPolicy**: APIFrontend was the only mesh component without a NetworkPolicy
   in the Helm chart, leaving it with unrestricted ingress/egress while every other service is
   default-deny with explicit allow rules.
4. **KubernautAgent ServiceAccount token exposure**: KubernautAgent is the highest-risk component
   (LLM-driven, broadest investigative RBAC) but used the default long-lived (~1yr) automounted
   ServiceAccount token, the same as lower-risk components.
5. **KubernautAgent investigative RBAC gaps**: the Operator's `ClusterRole` for KubernautAgent
   grants read access to KubeVirt VMs/VMIs/migrations, CDI DataVolumes, and PriorityClasses to
   support VM-backed workload and scheduling/preemption investigations; the Helm chart's
   equivalent `ClusterRole` was missing these rules, silently degrading investigation quality for
   clusters running these workload types.

**Impact**: Helm-chart deployments running mixed workloads (KubeVirt, priority-based scheduling)
or private-CA AWX/AAP integrations had either broken functionality (blocked egress, TLS
verification failures) or weaker security posture (unrestricted APIFrontend network access,
long-lived unscoped SA tokens) than functionally-equivalent Operator deployments.

---

## Business Objective

Bring the Helm chart to security and functional parity with the Kubernaut Operator for these five
areas, without introducing OpenShift-specific behavior (the chart remains vanilla-Kubernetes-only
per BR-PLATFORM-003/004's OCP-removal scope).

### Success Criteria

1. `workflowexecution.config.ansible.caCertSecretRef` renders a `build-ca-bundle` init container
   that concatenates the custom CA with the inter-service CA into one `TLS_CA_FILE`, mirroring the
   Operator's `tls.go` logic. Absent by default (zero behavior change for existing installs).
2. The WorkflowExecution NetworkPolicy allows HTTPS (443) egress when, and only when,
   `workflowexecution.config.ansible` is configured.
3. A new `templates/apifrontend/networkpolicy.yaml` enforces default-deny with explicit ingress
   (same-namespace + configurable `networkPolicies.apifrontend.ingressNamespaces`) and egress
   (DataStorage, Valkey when the replay cache is enabled, OIDC/JWKS discovery when OIDC is
   configured) rules, consistent with every other mesh component.
4. The `kubernaut-agent-sa` ServiceAccount sets `automountServiceAccountToken: false`; the
   Deployment instead mounts a short-TTL (1h), audience-scoped projected token.
5. The `kubernaut-agent-investigator` ClusterRole grants `get/list/watch` on
   `kubevirt.io/{virtualmachines,virtualmachineinstances,virtualmachineinstancemigrations}`,
   `cdi.kubevirt.io/datavolumes`, and `scheduling.k8s.io/priorityclasses`.
6. Zero regression for deployments that do not use Ansible, KubeVirt, or priority-based
   scheduling — all new resources/rules are additive and off by default (items 1–3) or
   security-tightening with no functional impact (items 4–5).

---

## Functional Requirements

- **FR-1**: `workflowexecution.config.ansible.caCertSecretRef.{name,key}` in `values.yaml` /
  `values.schema.json`; init container only renders when both `ansible` and `caCertSecretRef` are
  set (guarded to avoid nil-pointer template errors when `ansible` is entirely absent).
- **FR-2**: WorkflowExecution NetworkPolicy egress gains a 443/TCP rule conditioned on
  `workflowexecution.config.ansible` being present.
- **FR-3**: `networkPolicies.apifrontend.{enabled,ingressNamespaces}` controls the new
  APIFrontend NetworkPolicy; disabling it (`enabled=false`) omits the resource entirely. The
  Valkey NetworkPolicy gains a conditional ingress rule from `apifrontend` pods when
  `apifrontend.config.auth.replayCache.enabled=true` (APIFrontend talks to Valkey directly for the
  JWT replay cache, bypassing DataStorage).
- **FR-4**: `kubernaut-agent-sa` ServiceAccount: `automountServiceAccountToken: false`. Deployment
  mounts a `projected` volume with a `serviceAccountToken` source
  (`expirationSeconds: 3600`, `audience: https://kubernetes.default.svc`), plus the standard
  `kube-root-ca.crt` ConfigMap and `downwardAPI` namespace sources, at the conventional
  `/var/run/secrets/kubernetes.sio/serviceaccount` mount path.
- **FR-5**: `kubernaut-agent-investigator` ClusterRole additive rules for KubeVirt/CDI/scheduling
  resources, read-only (`get`, `list`, `watch`).

---

## Non-Goals

- Does not add write/mutate RBAC for KubeVirt or scheduling resources — investigation only.
- Does not implement the Kagenti MCP Gateway or SPIRE/SPIFFE workload-identity resources
  (`mcpgateway.go`, `spire.go` on the Operator side) — tracked separately as lower-priority,
  CRD-gated niche features, not security parity gaps.
- Does not change OpenShift-specific TLS profile mapping (`tlsprofile.go`) — out of scope per the
  chart's vanilla-Kubernetes-only positioning.

---

## Related Decisions

- **Tracked in**: [Issue #1589](https://github.com/jordigilh/kubernaut/issues/1589) (follow-up
  triage after the initial Helm/Operator parity pass covering BR-PLATFORM-003/004).
- **Builds on**: BR-PLATFORM-003 (observability/autoscaling parity), BR-PLATFORM-004 (single-install
  guard) — same Issue #1589 initiative, same Operator-vs-Helm triage methodology.

---

**Document Status**: ✅ Approved
**Priority**: P2 — closes security/functional parity gaps for non-default configurations
