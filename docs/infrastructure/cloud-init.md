# cloud-init

> **Last Updated:** 2026-06-26

## Purpose

Bootstrap VMs on first boot. cloud-init is the sole configuration management tool (Ansible is optional, not required). Terraform renders cloud-init templates and passes them to each VM at provision time.

## Templates

All cloud-init templates live in `infra/cloud-init/`:

```
infra/cloud-init/
├── gateway/        # Gateway node bootstrap
├── room-service/   # Room Service node bootstrap
└── game-server/    # Game Server node bootstrap
```

## Bootstrap Steps

Every VM runs the following sequence on first boot:

### 1. System Configuration

- Set hostname from Terraform variable (e.g., `gs-01`, `gw-02`)
- Update `/etc/hosts` with internal IP and hostname
- Configure UFW firewall rules per [networking.md](networking.md)
- Set sysctl parameters (`net.ipv4.ip_forward=1`, `net.core.somaxconn=65535`)

### 2. Docker Installation

```bash
apt-get update
apt-get install -y ca-certificates curl
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
echo "deb [arch=amd64] https://download.docker.com/linux/ubuntu noble stable" > /etc/apt/sources.list.d/docker.list
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker
```

### 3. K3s Join

**Server nodes (control plane):**

```bash
curl -sfL https://get.k3s.io | sh -s - server \
  --cluster-init \
  --node-name "<hostname>" \
  --tls-san "<load-balancer-ip>"
```

**Worker nodes:**

```bash
curl -sfL https://get.k3s.io | K3S_URL="https://<server-ip>:6443" K3S_TOKEN="<token>" sh -s - \
  --node-name "<hostname>" \
  --docker  # use Docker as container runtime
```

### 4. SSH Configuration

- Disable password authentication
- Add admin team SSH public keys (from secure store, not hardcoded)
- Configure SSH jumphost access through bastion

### 5. Firewall Setup

UFW rules per node role:

| Node Role | Allowed Inbound |
|-----------|-----------------|
| Gateway | 443 (WSS), 8080 (mgmt), 22 (admin SSH) |
| Room Service | 9000 (gRPC), 8080 (mgmt), 22 (admin SSH) |
| Game Server | 9001 (gRPC), 22 (admin SSH) |
| PostgreSQL | 5432 (PG), 22 (admin SSH) |
| Redis | 6379 (Redis), 22 (admin SSH) |
| Monitoring | 9090 (Prom), 3000 (Grafana), 3100 (Loki), 22 (admin SSH) |

Default deny on all other inbound traffic.

## Variables

Terraform injects the following variables into cloud-init templates:

| Variable | Source | Example |
|----------|--------|---------|
| `hostname` | Terraform | `gs-01` |
| `internal_ip` | Terraform | `10.73.2.10` |
| `k3s_token` | Terraform (from server) | `K10785...` |
| `k3s_server_ip` | Terraform output | `10.73.1.10` |
| `ssh_keys` | Secure store | `["ssh-ed25519 AAA..."]` |

## References

- ADR-014 — Infrastructure Platform
- [k3s.md](k3s.md)
- [terraform.md](terraform.md)
- [networking.md](networking.md)
