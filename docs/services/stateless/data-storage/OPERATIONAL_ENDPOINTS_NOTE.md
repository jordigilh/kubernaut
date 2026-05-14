# Operational endpoints and deprecation notes

## Disabled or deprecated routes

Operational review (**PROD reconcile / BR consistency**): Data Storage routing does **not** disable or deprecate endpoints at middleware or route-registration level—all published OpenAPI routes remain operative subject to RBAC/oauth-proxy identity as documented.

Deprecation today is reflected in the **contract** only: **`RemediationAlertPayload`** still defines three **`deprecated`** payload properties (`gitops_sync_delay`, `operator_reconcile_delay`, `total_propagation_delay`). Clients SHOULD avoid relying on those fields for new integrations; replacements are documented in OpenAPI annotations alongside the deprecated properties.

There are **no** additional dormant toggles hiding production endpoints beyond normal configuration (TLS, RBAC-to-service, oauth-proxy ingress).
