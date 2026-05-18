package infrastructure

// SeverityTriageAlertRulesYAML returns a Prometheus alert rules ConfigMap YAML
// for seeding the E2E severity triage pipeline tests. Each rule is crafted to
// exercise a specific tier:
//
//   - HighCPU: for:0s → fires immediately when metric is present (tier 1)
//   - HighMemory: for:1h → stays pending when metric present (tier 1.5)
//   - DiskPressure: for:0s + metric injected → inactive then evaluates live data (tier 2)
//   - NetworkLatency: query matches no-data-ns target but no metric exists → inactive no data (tier 2.5)
//
// Note: PromQL expressions MUST include label selectors for namespace/kind/name
// because the triage pipeline's Tier 1.5 and Tier 2 use ExtractLabelMatchers(query)
// + MatchesResource to correlate rules with the target resource.
const SeverityTriageAlertRulesYAML = `
groups:
  - name: e2e-severity-triage
    interval: 5s
    rules:
      - alert: HighCPU
        expr: e2e_cpu_usage_percent{namespace="default",kind="Deployment",name="test-firing-target"} > 90
        for: 0s
        labels:
          severity: critical
          source: prometheus
        annotations:
          summary: "CPU usage is critically high"
      - alert: HighMemory
        expr: e2e_memory_usage_percent{namespace="default",kind="Deployment",name="test-pending-target"} > 85
        for: 1h
        labels:
          severity: high
          source: prometheus
        annotations:
          summary: "Memory usage is high"
      - alert: DiskPressure
        expr: e2e_disk_usage_percent{namespace="sev-tier2-ns",kind="Deployment",name="test-inactive-target"} > 90
        for: 0s
        labels:
          severity: medium
          source: prometheus
        annotations:
          summary: "Disk usage is elevated"
      - alert: NetworkLatency
        expr: e2e_network_latency_ms{namespace="no-data-ns",kind="Deployment",name="test-nodata-target"} > 100
        for: 0s
        labels:
          severity: high
          source: prometheus
        annotations:
          summary: "Network latency is high"
`
