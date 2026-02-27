#!/usr/bin/env bash
# Shared scenario-dir:image-name mappings for demo workflow scripts.
# Sourced by build-demo-workflows.sh and seed-workflows.sh.
#
# Format: "scenario-directory:image-name"
# The exec image is <registry>/<image-name>:<version>
# The schema image is <registry>/<image-name>-schema:<version>

WORKFLOWS=(
    "gitops-drift:git-revert-job"
    "autoscale:provision-node-job"
    "slo-burn:proactive-rollback-job"
    "memory-leak:graceful-restart-job"
    "memory-escalation:increase-memory-limits-job"
    "crashloop:crashloop-rollback-job"
    "remediation-retry:restart-pods-job"
    "hpa-maxed:patch-hpa-job"
    "pdb-deadlock:relax-pdb-job"
    "pending-taint:remove-taint-job"
    "orphaned-pvc-no-action:cleanup-pvc-job"
    "node-notready:cordon-drain-job"
    "stuck-rollout:rollback-deployment-job"
    "cert-failure:fix-certificate-job"
    "cert-failure-gitops:fix-certificate-gitops-job"
    "crashloop-helm:helm-rollback-job"
    "mesh-routing-failure:fix-authz-policy-job"
    "statefulset-pvc-failure:fix-statefulset-pvc-job"
    "network-policy-block:fix-network-policy-job"
)
