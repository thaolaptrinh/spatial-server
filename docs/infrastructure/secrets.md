# Secrets

> **Last Updated:** 2026-06-26

## Purpose

Define the strategy for managing sensitive data (credentials, keys, certificates) across all environments. Secrets are never stored in source code.

## Principles

- **Never commit secrets to source code.** `.env` files, credentials, private keys, and tokens are excluded via `.gitignore`.
- **Kubernetes Secret is the production mechanism.** Secrets are created outside of Helm charts and referenced by name.
- **Environment variables at runtime** for containerized services (DB URLs, JWT public keys).
- **Terraform state contains no secret values.** Sensitive variables are passed via secure CI/CD variables or external secret stores.

## Secret Types

### JWT Public Keys

| Secret | Source | Usage |
|--------|--------|-------|
| `jwt-public-key` | Business Backend (RS256 public key) | Gateway validates client JWT tokens |
| `jwt-private-key` | Never stored in Spatial Server | Only Business Backend signs tokens |

Stored as a Kubernetes Secret (`jwt-keys`) and mounted as a volume or injected as an environment variable.

### Database Credentials

| Secret | Rotation | Usage |
|--------|----------|-------|
| `POSTGRES_USER` | Per deployment | PostgreSQL connection |
| `POSTGRES_PASSWORD` | Per deployment (auto-generated) | PostgreSQL authentication |
| `POSTGRES_DB` | Fixed | Database name (`spatial_server`) |

Stored as a Kubernetes Secret (`db-credentials`) and injected as `DB_URL` environment variable (`postgres://user:password@host:5432/spatial_server`).

### Redis Credentials

| Secret | Rotation | Usage |
|--------|----------|-------|
| `REDIS_PASSWORD` | Per deployment | Redis `AUTH` command |

Stored as a Kubernetes Secret (`redis-credentials`) and injected as `REDIS_ADDR` environment variable.

### TLS Certificates

- **Ingress TLS:** Managed by cert-manager with Let's Encrypt (auto-renewal). No manual secret management.
- **Internal mTLS (optional, Phase 4):** Self-signed CA generated per cluster, distributed via Kubernetes Secret.

## Secret Creation (Production)

```bash
# JWT public key
kubectl create secret generic jwt-keys \
  --from-file=jwt-public-key=./keys/public.pem \
  -n spatial-server

# Database credentials
kubectl create secret generic db-credentials \
  --from-literal=POSTGRES_USER=spatial \
  --from-literal=POSTGRES_PASSWORD=$(openssl rand -base64 32) \
  --from-literal=POSTGRES_DB=spatial_server \
  -n spatial-server

# Redis credentials
kubectl create secret generic redis-credentials \
  --from-literal=REDIS_PASSWORD=$(openssl rand -base64 32) \
  -n spatial-server
```

## Local Development

Local dev uses `configs/*.yml` with default credentials (different from production). PostgreSQL and Redis in Docker Compose use environment variables from the Compose file, never from production secrets.

## References

- ADR-014 — Infrastructure Platform
- ADR-018 — Security
- [k3s.md](k3s.md)
- [helm.md](helm.md)
