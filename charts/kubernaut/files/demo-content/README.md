# Demo Content (ActionTypes & RemediationWorkflows)

This directory holds the ActionType and RemediationWorkflow YAML files bundled
into the Helm chart's post-install seed job. The files are **not tracked in
git** — they are synced from the
[kubernaut-demo-scenarios](https://github.com/jordigilh/kubernaut-demo-scenarios)
repository, which is the single source of truth for demo content.

## Populating the files

```bash
make sync-demo-content
```

This copies action-types and remediation-workflows from the sibling
`../kubernaut-demo-scenarios` directory (if present) or does a shallow clone
from GitHub. The target is also run automatically in CI before Helm smoke tests.

## Directory structure

```
files/demo-content/
├── README.md            (this file — tracked in git)
├── action-types/        (*.yaml — gitignored)
│   ├── cleanup-node.yaml
│   ├── ...
│   └── scale-replicas.yaml
└── workflows/           (*.yaml — gitignored)
    ├── autoscale.yaml
    ├── ...
    └── stuck-rollout.yaml
```

## See also

- Issue [#428](https://github.com/jordigilh/kubernaut/issues/428) — CI pipeline
  change to build the Helm chart as an OCI artifact with fresh demo content.
