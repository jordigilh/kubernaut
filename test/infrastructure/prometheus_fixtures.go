package infrastructure

// SeverityTriageAlertRulesYAML returns a Prometheus alert rules ConfigMap YAML
// for seeding the E2E severity triage pipeline tests. Each rule is crafted to
// exercise a specific tier:
//
//   - HighCPU: for:0s → fires immediately when metric is present (tier 1)
//   - HighMemory: for:1h → stays pending when metric present (tier 2)
//   - DiskPressure: for:0s → no metric injected → inactive with data (tier 3)
//   - NetworkLatency: for:0s → no rule data at all → inactive no data (tier 4)
const SeverityTriageAlertRulesYAML = `
groups:
  - name: e2e-severity-triage
    interval: 5s
    rules:
      - alert: HighCPU
        expr: e2e_cpu_usage_percent > 90
        for: 0s
        labels:
          severity: critical
          source: prometheus
        annotations:
          summary: "CPU usage is critically high"
      - alert: HighMemory
        expr: e2e_memory_usage_percent > 85
        for: 1h
        labels:
          severity: high
          source: prometheus
        annotations:
          summary: "Memory usage is high"
      - alert: DiskPressure
        expr: e2e_disk_usage_percent > 90
        for: 0s
        labels:
          severity: medium
          source: prometheus
        annotations:
          summary: "Disk usage is elevated"
`
