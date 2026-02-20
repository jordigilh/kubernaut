-- +goose Up
-- +goose StatementBegin
-- Migration: Add action types for new demo scenarios (#133-#138)
-- Authority: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)
-- Purpose: Register action types for cert-manager, Helm, Linkerd, StatefulSet, and NetworkPolicy scenarios

INSERT INTO action_type_taxonomy (action_type, description) VALUES
    ('FixCertificate', '{"what": "Recreate a missing or corrupted CA Secret backing a cert-manager ClusterIssuer to restore certificate issuance.", "when_to_use": "A cert-manager Certificate is stuck in NotReady because the CA Secret has been deleted or corrupted, and the ClusterIssuer cannot sign.", "when_not_to_use": "When the Certificate failure is caused by DNS validation issues, incorrect certificate spec, or an expired root CA that needs rotation rather than recreation.", "preconditions": "cert-manager is installed, the ClusterIssuer exists, and the failure is specifically due to a missing or corrupted CA Secret."}'),
    ('HelmRollback', '{"what": "Roll back a Helm release to its previous healthy revision.", "when_to_use": "A Helm-managed workload is crashing after a helm upgrade introduced a bad configuration, and the previous Helm revision was healthy.", "when_not_to_use": "When the workload is not managed by Helm (use RollbackDeployment or GracefulRestart instead), or when the crash is caused by an external dependency failure.", "preconditions": "The Helm release has at least one previous healthy revision in its history, and the Helm tiller/controller has access to roll back."}'),
    ('FixAuthorizationPolicy', '{"what": "Remove or fix a Linkerd AuthorizationPolicy that is blocking legitimate traffic.", "when_to_use": "A Linkerd-meshed workload has high error rates (403 Forbidden) because a restrictive AuthorizationPolicy is denying all unauthenticated or unrecognized traffic.", "when_not_to_use": "When the 403 errors are intentional security policy enforcement, or when the issue is caused by something other than an AuthorizationPolicy (e.g., network partition).", "preconditions": "Linkerd is installed, the workload is meshed with a Linkerd sidecar, and an AuthorizationPolicy has been identified as the traffic blocker."}'),
    ('FixStatefulSetPVC', '{"what": "Recreate a missing PVC for a StatefulSet and restart the stuck pod.", "when_to_use": "A StatefulSet pod is stuck in Pending because its PVC has been accidentally deleted or the backing PV is no longer available.", "when_not_to_use": "When the PVC exists but the storage backend itself is down (fix the storage backend instead), or when data recovery is needed before recreating.", "preconditions": "The StatefulSet exists with a volumeClaimTemplate, the missing PVC name follows the StatefulSet naming convention, and a StorageClass is available to provision a new PV."}'),
    ('FixNetworkPolicy', '{"what": "Remove a deny-all NetworkPolicy that is blocking legitimate ingress traffic and causing health check failures.", "when_to_use": "A NetworkPolicy is blocking all ingress traffic to a workload, causing liveness and readiness probe failures that lead to pod restarts or unavailability.", "when_not_to_use": "When the NetworkPolicy is intentionally restrictive for security purposes and the outage has a different root cause.", "preconditions": "A deny-all NetworkPolicy exists in the namespace, and the workload health checks are confirmed to be failing due to blocked network traffic rather than application errors."}')
ON CONFLICT (action_type) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM action_type_taxonomy WHERE action_type IN (
    'FixCertificate',
    'HelmRollback',
    'FixAuthorizationPolicy',
    'FixStatefulSetPVC',
    'FixNetworkPolicy'
);
-- +goose StatementEnd
