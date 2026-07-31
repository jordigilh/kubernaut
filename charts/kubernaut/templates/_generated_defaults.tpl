{{/*
DD-PLATFORM-006 Decision Area 14 (PR9): materialized schema defaults.
Auto-generated from charts/kubernaut/values.schema.json by
hack/gen-helm-defaults (`make generate-helm-defaults`). Do not edit
by hand -- changes will be overwritten on the next generation, and CI's
drift check (`make generate-helm-defaults && git diff --exit-code -- charts/kubernaut/templates/_generated_defaults.tpl`) will fail on a
stale, hand-edited copy.

Consumed by kubernaut.mergedValues (_helpers.tpl), which merges these
defaults with the user's own values.yaml/--set overrides, user values always
winning (including explicit false/0/"").
*/}}
{{- define "kubernaut.defaults" -}}
aianalysis:
    logging:
        level: INFO
    pdb:
        enabled: true
    policies:
        content: ""
        existingConfigMap: ""
    replicas: 1
apifrontend:
    autoscaling:
        cpuTarget: 75
        enabled: false
        maxReplicas: 5
        memoryTarget: 80
        minReplicas: 1
    config:
        auth:
            allowInsecureIssuers: false
            audience: ""
            issuerURL: ""
            jwksURL: ""
            oidcCaFile: ""
            replayCache:
                enabled: false
                redisDB: 1
                tls:
                    caFile: ""
                    certFile: ""
                    enabled: false
                    keyFile: ""
        interactive:
            awaitSessionTimeout: 10s
            bridgeInactivityTimeout: 15s
            enabled: true
        mcp:
            enabled: true
            sessionIdleTimeout: 5m
        rateLimit:
            ipRequestsPerSec: 10000
            maxConcurrentSessions: 50
            toolCallsPerMinute: 600
            userRequestsPerSec: 100
        rbac:
            sarCacheTTL: 30s
        server:
            healthPort: 8081
            httpPort: 8443
            metricsPort: 9090
        session:
            disconnectTTL: 10m
            retentionTTL: 720h
        severityTriage:
            cacheTTLSeconds: 30
            llmConfidence: 0.7
            llmEnabled: true
            llmProfileRef: ""
            maxQueriesPerCall: 10
            maxRulesEvaluated: 100
    enabled: true
    fleet:
        namespace: ""
        oauth2:
            credentialsSecretRef: ""
    image:
        pullPolicy: IfNotPresent
        repository: ghcr.io/jordigilh/kubernaut/apifrontend
        tag: ""
    ingress:
        annotations: {}
        className: ""
        enabled: false
        host: ""
        tls:
            secretName: ""
    llmProfileRef: ""
    logging:
        level: INFO
    pdb:
        enabled: true
    replicas: 1
    service:
        nodePort: 0
        type: ClusterIP
authwebhook:
    logging:
        level: INFO
    pdb:
        enabled: true
    replicas: 1
console:
    auth:
        secretName: ""
    enabled: false
    ingress:
        annotations: {}
        className: ""
        enabled: false
        host: ""
        tls:
            secretName: ""
    oauth2Proxy:
        image: quay.io/oauth2-proxy/oauth2-proxy:v7.15.3
    pdb:
        enabled: true
    replicas: 1
datastorage:
    autoscaling:
        cpuTarget: 75
        enabled: false
        maxReplicas: 5
        memoryTarget: 80
        minReplicas: 1
    config:
        auditHashKey:
            existingSecret: ""
            secretKey: hmacKey
        database:
            connMaxLifetime: 1h
            maxIdleConns: 20
            maxOpenConns: 100
            sslMode: disable
        redis:
            dlqMaxLen: 10000
            tls:
                caFile: ""
                certFile: ""
                insecureSkipVerify: false
                keyFile: ""
        retention:
            batchSize: 1000
            defaultDays: 2555
            enabled: false
            interval: 24h
        server:
            maxBodySize: "5242880"
            maxConcurrentRequests: 100
            rateLimit:
                burst: 100
                requestsPerSecond: 50
            readTimeout: 30s
            signerCertDir: /etc/certs
    dbExistingSecret: ""
    logging:
        level: INFO
    pdb:
        enabled: true
    replicas: 1
    service:
        nodePort: 0
        type: ClusterIP
effectivenessmonitor:
    config:
        assessment:
            stabilizationWindow: 30s
            validityWindow: 120s
    fleet:
        namespace: ""
        oauth2:
            credentialsSecretRef: ""
    logging:
        level: INFO
    pdb:
        enabled: true
    replicas: 1
fleetmetadatacache:
    enabled: false
    image:
        pullPolicy: IfNotPresent
        repository: quay.io/kubernaut-ai/fleetmetadatacache
        tag: v1.6.0
    keyTTL: 45s
    namespace: kubernaut-system
    oauth2:
        credentialsSecretRef: ""
    pdb:
        enabled: true
    replicas: 1
    syncInterval: 30s
    valkeyAddr: ""
    valkeyTLS:
        caFile: ""
        certFile: ""
        keyFile: ""
gateway:
    config:
        cors:
            allowCredentials: false
            allowedOrigins:
                - https://no-browser-clients.invalid
            maxAge: 300
        deduplication:
            cooldownPeriod: 5m
        middleware:
            trustedProxyCIDRs: []
        server:
            k8sRequestTimeout: 15s
            maxConcurrentRequests: 100
            readTimeout: 30s
            writeTimeout: 30s
    fleet:
        oauth2:
            credentialsSecretRef: ""
    ingress:
        annotations: {}
        className: ""
        enabled: false
        host: ""
        tls:
            secretName: ""
    logging:
        level: INFO
    pdb:
        enabled: true
    replicas: 1
    service:
        nodePort: 0
        type: ClusterIP
global:
    fleet:
        backend: ""
        enabled: false
        endpoint: ""
        mcpGatewayEndpoint: ""
        mcpGatewayType: ""
        oauth2:
            credentialsSecretRef: ""
            enabled: false
            scopes: []
            tlsCAFile: ""
            tokenURL: ""
        tlsCAFile: ""
        tokenSecretRef: ""
    image:
        digest: ""
        namespace: kubernaut-ai
        pullPolicy: IfNotPresent
        registry: quay.io
        separator: /
        tag: ""
    imagePullSecrets: []
    llmProfiles: {}
    podDefaults:
        pdb:
            enabled: true
    tolerations: []
hooks:
    tlsCerts:
        extraSANs: []
        image: docker.io/bitnami/kubectl:latest@sha256:6e2cdb22d6ab7264ea198c717f555e30536b54029d26c8781b9f25f78951b564
kubernautAgent:
    alignmentCheck:
        enabled: false
        llmProfileRef: ""
        maxStepTokens: 500
        timeout: 10s
    interactive:
        enabled: false
        inactivityTimeout: 10m
        maxAnalyzingTimeout: 45m
        maxConcurrentSessions: 50
        rateLimitPerUser: 20
        sessionTTL: 30m
    llmProfileRef: primary
    logging:
        level: INFO
    pdb:
        enabled: true
    phaseModels: {}
    replicas: 1
    service:
        nodePort: 0
        type: ClusterIP
monitoring:
    alertManager:
        enabled: false
        tlsCaFile: ""
        url: ""
    prometheus:
        enabled: false
        tlsCaFile: ""
        url: ""
    prometheusRule:
        additionalLabels: {}
        enabled: false
        thresholds:
            activeSessionsFor: 5m
            activeSessionsMax: 10
            commandDurationFor: 10m
            commandDurationP99Max: 30
            leaseContentionFor: 5m
            leaseContentionRateMax: 0.1
            takeoverRateFor: 15m
            takeoverRateMax: 0.05
    serviceMonitor:
        enabled: false
networkPolicies:
    apiServerCIDR: ""
    apiServerCIDRs: []
    apiServerPort: 0
    apifrontend:
        ingressCIDRs: []
        ingressNamespaceSelectors: []
        ingressNamespaces: []
    console:
        ingressCIDRs: []
        ingressNamespaceSelectors: []
        ingressNamespaces: []
    datastorage:
        ingressCIDRs: []
        ingressNamespaceSelectors: []
    externalRegistry:
        cidr: 0.0.0.0/0
        port: 443
    externalWebhooks:
        cidr: 0.0.0.0/0
        port: 443
    gateway:
        ingressCIDRs: []
        ingressNamespaceSelectors: []
        ingressNamespaces: []
    idp:
        cidr: 0.0.0.0/0
        extraPorts: []
        port: 443
    kubernautAgent:
        ingressCIDRs: []
        ingressNamespaceSelectors: []
    llm:
        cidr: 0.0.0.0/0
        port: 443
    mcpGateway:
        cidr: 0.0.0.0/0
        port: 8080
    monitoring:
        alertManagerPort: 9093
        namespace: ""
        prometheusPort: 9090
notification:
    logging:
        level: INFO
    pdb:
        enabled: true
    replicas: 1
    routing:
        content: ""
        existingConfigMap: ""
    slack:
        channel: '#kubernaut-alerts'
        secretKey: webhook-url
        secretName: ""
postgresql:
    auth:
        database: action_history
        existingSecret: ""
        username: slm_user
    enabled: true
    host: ""
    image: postgres:16-alpine
    port: 5432
    replicas: 1
    storage:
        size: 10Gi
        storageClassName: ""
remediationorchestrator:
    config:
        asyncPropagation:
            gitOpsSyncDelay: 3m
            operatorReconcileDelay: 1m
            proactiveAlertDelay: 5m
        effectivenessAssessment:
            stabilizationWindow: 5m
        notifications:
            notifySelfResolved: false
        retention:
            period: 24h
        routing:
            consecutiveFailureCooldown: 1h
            consecutiveFailureThreshold: 3
            ineffectiveChainThreshold: 3
            ineffectiveTimeWindow: 4h
            recentlyRemediatedCooldown: 5m
            recurrenceCountThreshold: 5
        timeouts:
            analyzing: 10m
            executing: 30m
            global: 1h
            processing: 5m
            verifying: 30m
    fleet:
        oauth2:
            credentialsSecretRef: ""
    logging:
        level: INFO
    pdb:
        enabled: true
    replicas: 1
signalprocessing:
    fleet:
        namespace: ""
        oauth2:
            credentialsSecretRef: ""
    logging:
        level: INFO
    pdb:
        enabled: true
    policies:
        content: ""
        existingConfigMap: ""
    proactiveSignalMappings:
        content: ""
        existingConfigMap: ""
    replicas: 1
tls:
    certManager:
        issuerRef:
            group: cert-manager.io
            kind: ClusterIssuer
            name: ""
    interService:
        caFile: /etc/tls-ca/ca.crt
        certDir: /etc/tls
    mode: hook
valkey:
    enabled: true
    existingSecret: ""
    host: ""
    image: valkey/valkey:8-alpine
    port: 6379
    replicas: 1
    storage:
        size: 512Mi
        storageClassName: ""
workflowexecution:
    config:
        ansible:
            caCertSecretRef:
                key: ca.crt
            organizationID: 1
            tokenSecretRef:
                key: token
                namespace: ""
        execution:
            cooldownPeriod: 1m
    fleet:
        oauth2:
            credentialsSecretRef: ""
    logging:
        level: INFO
    pdb:
        enabled: true
    replicas: 1
    workflowNamespace: kubernaut-workflows
{{- end -}}
