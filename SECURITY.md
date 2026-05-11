# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.5.x   | :white_check_mark: |
| < 1.5   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability in kubernaut-apifrontend, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

### Contact

- Email: security@kubernaut.ai
- PGP Key: Available upon request

### What to include

- Description of the vulnerability
- Steps to reproduce
- Potential impact assessment
- Suggested fix (if any)

### Response Timeline

| Action | SLA |
| ------ | --- |
| Acknowledgment | 48 hours |
| Initial assessment | 5 business days |
| Critical fix | 7 calendar days |
| High fix | 30 calendar days |
| Medium fix | 90 calendar days |

### Disclosure Policy

We follow coordinated disclosure. We will:
1. Confirm receipt within 48 hours
2. Provide a timeline for the fix
3. Notify you when the fix is released
4. Credit you in the release notes (unless you prefer anonymity)

## Security Controls

This project implements FedRAMP Moderate-aligned security controls including:
- JWT authentication with replay protection (jti tracking)
- Per-IP and per-user rate limiting
- TLS 1.2+ with certificate hot-reload
- Audit trail with centralized storage
- RBAC enforcement via Kubernetes impersonation
- Circuit breaker patterns for all downstream services
- SBOM generation and container image signing (Cosign)
- Static analysis (gosec, govulncheck, CodeQL, gitleaks)
