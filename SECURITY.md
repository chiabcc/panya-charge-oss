# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in panya-charge-oss, please report it
responsibly using one of these channels:

1. **GitHub Security Advisories** (preferred) —
   [Report a vulnerability](https://github.com/chiabcc/panya-charge-oss/security/advisories/new)
2. **Email** — security@chiabcc.com

Do not open a public issue or pull request for security vulnerabilities.

## What to report

- **Code injection** — command injection, SQL injection, or template injection in any code path
- **Denial of service** — resource exhaustion in OCPP message handling or MQTT processing
- **OCPP protocol exploitation** — crafted OCPP messages that cause crashes, panics, or data corruption
- **MQTT injection** — crafted MQTT payloads that trigger unsafe behavior

## What NOT to report

The following are **by design** and are not security issues:

- **No authentication** — this is a self-hosted OSS protocol bridge with no auth layer. Deploy behind a firewall or VPN.
- **No TLS by default** — the OCPP WebSocket server listens without TLS. Users who need encrypted transport should place the server behind a reverse proxy (nginx, caddy) that terminates TLS.
- **No RBAC** — single-user, self-hosted deployment. No multi-tenancy.
- **No input validation on MQTT topics** — MQTT topics are configuration, not user input.
- **No rate limiting on OCPP connections** — this is a local CSMS meant for trusted chargers. Deploy behind a firewall.

## Security principles

1. **Self-hosted threat model** — this software runs in a local environment. The assumption is that the host machine and network are trusted.
2. **Defense in depth** — recommend TLS termination via reverse proxy for production deployments.
3. **Safe defaults** — minimum charging current (6A) is enforced. Contactors have an 180s cooldown to prevent hardware damage.
4. **No secrets in config** — sensitive values (MQTT passwords) should be provided via environment variables, not committed to source control.

## Response timeline

- Vulnerability reports are acknowledged within 7 days
- Security fixes are prioritized over feature work
- A patched release is published within 30 days of confirmation