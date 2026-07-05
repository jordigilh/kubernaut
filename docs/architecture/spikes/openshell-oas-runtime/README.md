# Spike: OpenShell OAS Runtime — Code Removed

This directory backed the **COMPLETED** spike documented in full at
[`../SPIKE-OPENSHELL-OAS-RUNTIME.md`](../SPIKE-OPENSHELL-OAS-RUNTIME.md). All findings,
environment details, and recommendations live there.

The runnable artifacts (`setup.sh`, `kind-config.yaml`, `policy.yaml`) were removed after
the spike concluded: they stood up a one-off Kind cluster for manual validation and had no
ongoing purpose (nor CI usage) once the findings were captured. `setup.sh` also referenced
follow-up scripts (`test-sandbox.sh`, `teardown.sh`) that were never checked in, so it was
already non-runnable as-is.

The relevant OPA egress policy snippet is reproduced inline in the spike doc's
"Egress Policy" section for reference.
