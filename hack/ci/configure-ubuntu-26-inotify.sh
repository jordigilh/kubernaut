#!/usr/bin/env bash
# Apply the Ubuntu 26.04 inotify instance-quota workaround for heavy E2E lanes.

set -euo pipefail

sudo sysctl -w fs.inotify.max_user_instances=1280

{
  echo "Ubuntu 26 inotify instance quota after workaround:"
  sysctl fs.inotify.max_user_instances
} >> /tmp/e2e-runner-resources-after.txt
