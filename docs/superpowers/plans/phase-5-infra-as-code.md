# Phase 5 — Infra-as-Code Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One reproducible path from source to production cluster — `terraform apply` provisions cloud VMs/net/DB, `cloud-init` bootstraps K3s nodes, `helm install` deploys every service plus full monitoring stack, and CI builds images + packages charts.

**Architecture:** `infra/` holds Terraform modules (VPC, K3s server/agent, RDS, ElastiCache, DNS), cloud-init (Docker + K3s install), Helm charts (one per deployable + `monitoring` umbrella), and K3s cluster manifests (namespace, ingress, network policies, RBAC). A staging docker-compose overlay brings Prometheus/Grafana/Loki/Promtail/Alertmanager to local dev. OTel `otelgrpc` stats handlers propagate W3C `traceparent` across gRPC; every log line carries `trace_id` + `request_id`.

**Tech Stack:** Terraform 1.7+, K3s, Helm 3, cloud-init, Prometheus/Grafana/Loki/Promtail, OpenTelemetry Go SDK (`otelgrpc`), GitHub Actions. Module path: `github.com/thaolaptrinh/spatial-server`.

**Pre-existing files:** `deploy/docker-compose/docker-compose.yml` (postgres:16-alpine, redis:7-alpine, env `SPATIAL_REDIS__ADDR`/`SPATIAL_GATEWAY__JWT_SECRET`); `build/docker/gateway.Dockerfile` (EXPOSE 8080 9000), `build/docker/room-service.Dockerfile`, `build/docker/game-server.Dockerfile` (Go 1.25, alpine:3.21); `.github/workflows/ci.yml` (Go lint/test/build only); `Makefile` (`build`/`test`/`dev-up`/`docker-build`/`demo`); `apps/*/main.go` use `grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))`. **`infra/` does not exist yet.**

**Validation note:** Infra tasks use `helm lint` / `helm template`, `terraform fmt/validate`, `kubectl apply --dry-run=client`, `docker compose config`. Only Task 10 (OTel) uses Go TDD. Inline `# === path ===` lines inside a code block denote separate files to create.

---

### Task 1: Create `infra/` directory structure + update AGENTS.md + Makefile targets

**Files:** Create `infra/.gitkeep`; Modify `AGENTS.md`, `docs/architecture/repository-structure.md`, `Makefile`.

- [ ] **Step 1: Create skeleton**

```bash
mkdir -p infra/{terraform/{providers,modules/{vpc,k3s,node_pool,rds,elasticache,dns},environments/staging},cloud-init,helm/{gateway,room-service,game-server,postgres,redis,monitoring},k3s} && touch infra/.gitkeep
```

- [ ] **Step 2: Update AGENTS.md "Project Structure"** — after `deploy/` line add:
```
infra/              Terraform, Helm, cloud-init, K3s manifests
├── terraform/      Cloud provisioning (VMs, net, DB) — never apps (ADR-014)
├── cloud-init/     K3s node bootstrap (Docker + K3s install)
├── helm/           Charts: gateway, room-service, game-server, postgres, redis, monitoring
└── k3s/            Cluster manifests (namespace, ingress, RBAC, network policies)
deploy/             Docker Compose (base + staging monitoring overlay)
```

- [ ] **Step 3: Add Makefile targets** (before `.PHONY: docker-build demo`)

```makefile
helm-lint:; @for c in infra/helm/*/; do echo "==> $$c"; helm lint $$c || exit 1; done
helm-template:; @for c in infra/helm/*/; do helm template $$(basename $$c) $$c || exit 1; done
terraform-fmt:; terraform -chdir=infra/terraform/environments/staging fmt -check -recursive
terraform-validate:; terraform -chdir=infra/terraform/environments/staging init -backend=false && terraform -chdir=infra/terraform/environments/staging validate
k3s-dry-run:; @for f in infra/k3s/*.yaml; do kubectl apply --dry-run=client -f "$$f" || exit 1; done
```

- [ ] **Step 4: Verify + commit** — `make -n helm-lint | head -2` parses; then `git add infra/.gitkeep AGENTS.md docs/architecture/repository-structure.md Makefile && git commit -m "infra: create infra/ structure and make targets (ADR-014)"`

---

### Task 2: Terraform providers + VPC module

**Files:** Create `infra/terraform/providers/{versions,aws}.tf`, `infra/terraform/modules/vpc/{main,variables,outputs}.tf`.

- [ ] **Step 1: Write providers + VPC**

```hcl
# === providers/versions.tf ===
terraform { required_version = ">= 1.7.0"; required_providers { aws = { source = "hashicorp/aws", version = "~> 5.40" } } }
# === providers/aws.tf ===
provider "aws" { region = var.aws_region; default_tags { tags = { Project = "spatial-server", ManagedBy = "terraform" } } }
variable "aws_region" { type = string; default = "us-east-1" }
# === modules/vpc/main.tf ===
data "aws_region" "current" {}
module "vpc" {
  source = "terraform-aws-modules/vpc/aws"; version = "5.8.1"; name = var.name; cidr = var.cidr
  azs = ["${data.aws_region.current.name}a", "${data.aws_region.current.name}b"]
  public_subnets = var.public_subnets; private_subnets = var.private_subnets
  enable_nat_gateway = true; single_nat_gateway = true; enable_dns_hostnames = true
}
resource "aws_security_group" "k3s" {
  name = "${var.name}-k3s"; vpc_id = module.vpc.vpc_id
  ingress { from_port = 0; to_port = 0; protocol = "-1"; self = true }
  ingress { from_port = 443; to_port = 443; protocol = "tcp"; cidr_blocks = ["0.0.0.0/0"] }
  ingress { from_port = 6443; to_port = 6443; protocol = "tcp"; cidr_blocks = var.private_subnets }
  egress { from_port = 0; to_port = 0; protocol = "-1"; cidr_blocks = ["0.0.0.0/0"] }
}
# === modules/vpc/variables.tf ===
variable "cidr" { type = string; default = "10.0.0.0/16" }; variable "name" { type = string; default = "spatial" }
variable "public_subnets" { type = list(string); default = ["10.0.1.0/24", "10.0.2.0/24"] }
variable "private_subnets" { type = list(string); default = ["10.0.10.0/24", "10.0.11.0/24"] }
# === modules/vpc/outputs.tf ===
output "vpc_id" { value = module.vpc.vpc_id }; output "private_subnets" { value = module.vpc.private_subnets }
output "public_subnets" { value = module.vpc.public_subnets }; output "k3s_sg_id" { value = aws_security_group.k3s.id }
```

- [ ] **Step 2: Validate + commit** — `terraform -chdir=infra/terraform fmt -check`; then `git add infra/terraform/providers infra/terraform/modules/vpc && git commit -m "infra: terraform providers and vpc module"`

---

### Task 3: Terraform K3s + node_pool + cloud-init bootstrap

**Files:** Create `infra/cloud-init/{common,k3s-server,k3s-agent}.yaml`, `infra/terraform/modules/k3s/{main,outputs}.tf`, `infra/terraform/modules/node_pool/main.tf`.

- [ ] **Step 1: Write cloud-init** (common shipped inlined into server+agent; `${...}` templated by Terraform `templatefile()`)

```yaml
# === cloud-init/common.yaml === (merged into k3s-server.yaml + k3s-agent.yaml)
#cloud-config
users: [{ name: spatial, sudo: "ALL=(ALL) NOPASSWD:ALL", shell: /bin/bash, ssh_authorized_keys: ["${ssh_pub_key}"] }]
ssh_pwauth: false; disable_root: true
packages: [docker.io, fail2ban, curl, jq]
runcmd: [sysctl -w fs.file-max=2097152, 'printf "spatial soft/hard nofile 1048576\n">/etc/security/limits.d/99-spatial.conf', systemctl enable --now docker fail2ban]
# === cloud-init/k3s-server.yaml === (merged with common)
#cloud-config
write_files: [{ path: /etc/rancher/k3s/config.yaml, content: "tls-san: [${server_private_ip}]\ndisable: [traefik]" }]
runcmd: ['curl -sfL https://get.k3s.io|INSTALL_K3S_EXEC="server --cluster-init" sh -', chmod 644 /etc/rancher/k3s/k3s.yaml, 'echo K3S_TOKEN=$(cat /var/lib/rancher/k3s/server/node-token)>/home/spatial/.k3s-join.env']
# === cloud-init/k3s-agent.yaml === (merged with common)
#cloud-config
runcmd: ['curl -sfL https://get.k3s.io|INSTALL_K3S_EXEC=agent K3S_URL=https://${server_private_ip}:6443 K3S_TOKEN=${k3s_token} sh -']
```

- [ ] **Step 2: Write K3s + node_pool modules**

```hcl
# === modules/k3s/main.tf ===
variable "subnet_id" { type = string }; variable "sg_id" { type = string }; variable "ssh_pub_key" { type = string }; variable "instance_type" { type = string; default = "t3.medium" }
data "aws_ami" "ubuntu" { most_recent = true; owners = ["099720109477"]; filter { name = "name"; values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"] } }
resource "aws_network_interface" "this" { subnet_id = var.subnet_id; security_groups = [var.sg_id] }
resource "aws_instance" "k3s_server" {
  ami = data.aws_ami.ubuntu.id; instance_type = var.instance_type; vpc_security_group_ids = [var.sg_id]
  user_data = templatefile("${path.module}/../../cloud-init/k3s-server.yaml", { server_private_ip = aws_network_interface.this.private_ip; ssh_pub_key = var.ssh_pub_key })
  tags = { Name = "spatial-k3s-server", Role = "control-plane" }
}
# === modules/k3s/outputs.tf ===
output "server_private_ip" { value = aws_instance.k3s_server.private_ip }; output "server_id" { value = aws_instance.k3s_server.id }
# === modules/node_pool/main.tf ===
variable "name" { type = string }; variable "instance_type" { type = string; default = "t3.large" }; variable "desired" { type = number; default = 2 }; variable "min_size" { type = number; default = 1 }; variable "max_size" { type = number; default = 6 }
variable "subnet_ids" { type = list(string) }; variable "sg_id" { type = string }; variable "server_private_ip" { type = string }; variable "k3s_token" { type = string }; variable "ssh_pub_key" { type = string }
data "aws_ami" "ubuntu" { most_recent = true; owners = ["099720109477"]; filter { name = "name"; values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"] } }
resource "aws_key_pair" "this" { key_name = "${var.name}-key"; public_key = var.ssh_pub_key }
resource "aws_launch_template" "agent" {
  name_prefix = "${var.name}-"; image_id = data.aws_ami.ubuntu.id; instance_type = var.instance_type; key_name = aws_key_pair.this.key_name
  user_data = base64encode(templatefile("${path.module}/../../cloud-init/k3s-agent.yaml", { server_private_ip = var.server_private_ip; k3s_token = var.k3s_token; ssh_pub_key = var.ssh_pub_key }))
}
resource "aws_autoscaling_group" "agents" {
  name = var.name; desired_capacity = var.desired; min_size = var.min_size; max_size = var.max_size; vpc_zone_identifier = var.subnet_ids
  launch_template { id = aws_launch_template.agent.id; version = "$Latest" }
}
```

- [ ] **Step 3: Validate + commit** — `terraform -chdir=infra/terraform fmt -check`; then `git add infra/cloud-init infra/terraform/modules/k3s infra/terraform/modules/node_pool && git commit -m "infra: k3s/node_pool modules and cloud-init bootstrap"`

---

### Task 4: Terraform RDS + ElastiCache + DNS + staging environment

**Files:** Create `infra/terraform/modules/{rds,elasticache,dns}/{main,variables,outputs}.tf`, `infra/terraform/environments/staging/{main,backend,variables}.tf`, `terraform.tfvars.example`.

- [ ] **Step 1: Write RDS module**

```hcl
# === modules/rds/main.tf ===
variable "subnet_ids" { type = list(string) }; variable "vpc_id" { type = string }; variable "allowed_sg_ids" { type = list(string) }; variable "instance_class" { type = string; default = "db.t4g.medium" }; variable "db_password" { type = string; sensitive = true }
resource "aws_db_subnet_group" "this" { name = "spatial"; subnet_ids = var.subnet_ids }
resource "aws_security_group" "rds" { name = "spatial-rds"; vpc_id = var.vpc_id; ingress { from_port = 5432; to_port = 5432; protocol = "tcp"; security_groups = var.allowed_sg_ids }; egress { from_port = 0; to_port = 0; protocol = "-1"; cidr_blocks = ["0.0.0.0/0"] } }
resource "aws_db_instance" "postgres" {
  identifier = "spatial-postgres"; engine = "postgres"; engine_version = "16"; instance_class = var.instance_class; allocated_storage = 50
  db_name = "spatial"; username = "spatial"; password = var.db_password; db_subnet_group_name = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.rds.id]; storage_encrypted = true; skip_final_snapshot = false
}
output "endpoint" { value = aws_db_instance.postgres.endpoint }; output "sg_id" { value = aws_security_group.rds.id }
```

- [ ] **Step 2: Write ElastiCache module** (vars: same shape as RDS + `node_type = "cache.t4g.small"`)

```hcl
# === modules/elasticache/main.tf ===
resource "aws_elasticache_subnet_group" "this" { name = "spatial-redis"; subnet_ids = var.subnet_ids }
resource "aws_security_group" "redis" { name = "spatial-redis"; vpc_id = var.vpc_id; ingress { from_port = 6379; to_port = 6379; protocol = "tcp"; security_groups = var.allowed_sg_ids }; egress { from_port = 0; to_port = 0; protocol = "-1"; cidr_blocks = ["0.0.0.0/0"] } }
resource "aws_elasticache_replication_group" "redis" {
  replication_group_id = "spatial-redis"; description = "spatial Redis HA (ADR-022)"; node_type = var.node_type; port = 6379
  parameter_group_name = "default.redis7"; engine_version = "7"; number_cache_clusters = 2; subnet_group_name = aws_elasticache_subnet_group.this.name
  security_group_ids = [aws_security_group.redis.id]; automatic_failover = true; transit_encryption_enabled = true
}
output "primary_endpoint" { value = aws_elasticache_replication_group.redis.primary_endpoint_address }
```

- [ ] **Step 3: Write DNS module** (vars: `zone_name`, `hostname`, `lb_dns_name`, `lb_zone_id`)

```hcl
# === modules/dns/main.tf ===
data "aws_route53_zone" "this" { name = var.zone_name; private_zone = false }
resource "aws_route53_record" "gateway" { zone_id = data.aws_route53_zone.this.zone_id; name = var.hostname; type = "A"; alias { name = var.lb_dns_name; zone_id = var.lb_zone_id; evaluate_target_health = true } }
```

- [ ] **Step 4: Write staging environment**

```hcl
# === environments/staging/backend.tf ===
terraform { backend "s3" { bucket = "spatial-tfstate"; key = "staging/terraform.tfstate"; region = "us-east-1"; dynamodb_table = "spatial-tf-locks"; encrypt = true } }
# === environments/staging/main.tf ===
terraform { required_version = ">= 1.7.0" }
module "providers" { source = "../../providers" }
module "vpc" { source = "../../modules/vpc"; name = "spatial-staging" }
module "k3s" { source = "../../modules/k3s"; subnet_id = module.vpc.private_subnets[0]; sg_id = module.vpc.k3s_sg_id; ssh_pub_key = var.ssh_pub_key }
module "agents" { source = "../../modules/node_pool"; name = "spatial-staging-agents"; subnet_ids = module.vpc.private_subnets; sg_id = module.vpc.k3s_sg_id; server_private_ip = module.k3s.server_private_ip; k3s_token = "WIRE_FROM_TF_OUTPUT"; ssh_pub_key = var.ssh_pub_key }
module "rds" { source = "../../modules/rds"; subnet_ids = module.vpc.private_subnets; vpc_id = module.vpc.vpc_id; allowed_sg_ids = [module.vpc.k3s_sg_id]; db_password = var.db_password }
module "redis" { source = "../../modules/elasticache"; subnet_ids = module.vpc.private_subnets; vpc_id = module.vpc.vpc_id; allowed_sg_ids = [module.vpc.k3s_sg_id] }
module "dns" { source = "../../modules/dns"; zone_name = var.dns_zone; hostname = "gateway.${var.dns_zone}"; lb_dns_name = var.lb_dns_name; lb_zone_id = var.lb_zone_id }
# === environments/staging/variables.tf ===
variable "ssh_pub_key" { type = string }; variable "db_password" { type = string; sensitive = true }; variable "dns_zone" { type = string }; variable "lb_dns_name" { type = string }; variable "lb_zone_id" { type = string }
# === environments/staging/terraform.tfvars.example === (do NOT commit real values)
# ssh_pub_key = "ssh-ed25519 AAA..."; db_password = "change-me"; dns_zone = "spatial.example.com"; lb_dns_name = "tf-output-xxx.elb.amazonaws.com"; lb_zone_id = "Z..."
```

- [ ] **Step 5: Validate + commit** — `make terraform-fmt && make terraform-validate` prints `Success! The configuration is valid.`; then `git add infra/terraform/modules/rds infra/terraform/modules/elasticache infra/terraform/modules/dns infra/terraform/environments/staging && git commit -m "infra: rds/elasticache/dns modules and staging environment"`

---

### Task 5: Helm chart — gateway

**Files:** Create `infra/helm/gateway/{Chart.yaml,values.yaml,templates/{deployment,service,hpa,ingress,configmap,secret,pdb}.yaml,templates/NOTES.txt}`.

- [ ] **Step 1: Write Chart.yaml + values.yaml**

```yaml
# === Chart.yaml ===
apiVersion: v2; name: gateway; type: application; version: 0.1.0; appVersion: "0.1.0"
# === values.yaml ===
replicaCount: 2
image: { repository: ghcr.io/thaolaptrinh/spatial-gateway, tag: dev-latest, pullPolicy: IfNotPresent }
service: { type: LoadBalancer, wsPort: 8080, grpcPort: 9000 }
resources: { requests: { cpu: 250m, memory: 512Mi }, limits: { cpu: "1", memory: 1Gi } }
hpa: { enabled: true, minReplicas: 2, maxReplicas: 10, cpuTarget: 70 }
ingress: { enabled: true, className: traefik, host: gateway.example.com }
secretRef: gateway-secret
env: { grpc__host: "0.0.0.0", grpc__port: "9000", gateway__ws_port: "8080", room_service__addr: "room-service:9000" }
```

- [ ] **Step 2: Write templates**

```yaml
# === templates/deployment.yaml ===
apiVersion: apps/v1; kind: Deployment; metadata: { name: {{ .Release.Name }}-gateway, labels: { app: gateway } }
spec:
  replicas: {{ .Values.replicaCount }}
  selector: { matchLabels: { app: gateway } }
  template:
    metadata: { labels: { app: gateway }, annotations: { prometheus.io/scrape: "true", prometheus.io/port: "9000" } }
    spec:
      containers:
        - name: gateway
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          ports: [{ name: ws, containerPort: 8080 }, { name: grpc, containerPort: 9000 }]
          envFrom: [{ configMapRef: { name: {{ .Release.Name }}-gateway-config } }, { secretRef: { name: {{ .Values.secretRef }} } }]
          resources: {{ toYaml .Values.resources | nindent 12 }}
          readinessProbe: { httpGet: { path: /health, port: ws }, initialDelaySeconds: 5 }
          livenessProbe: { httpGet: { path: /health, port: ws }, initialDelaySeconds: 15 }
# === templates/service.yaml ===
apiVersion: v1; kind: Service; metadata: { name: {{ .Release.Name }}-gateway, labels: { app: gateway } }
spec: { type: {{ .Values.service.type }}, ports: [{ name: ws, port: {{ .Values.service.wsPort }}, targetPort: {{ .Values.service.wsPort }} }, { name: grpc, port: {{ .Values.service.grpcPort }}, targetPort: {{ .Values.service.grpcPort }} }], selector: { app: gateway } }
# === templates/configmap.yaml === (iterates .Values.env, renders env var name as SPATIAL_<UPPER>__<KEY>)
# === templates/hpa.yaml === (gated {{- if .Values.hpa.enabled }}, CPU averageUtilization: .Values.hpa.cpuTarget)
# === templates/pdb.yaml === (minAvailable: 1, selector app: gateway)
# === templates/ingress.yaml === (Traefik, /ws → wsPort, TLS placeholder for Phase 6)
# === templates/secret.yaml === (gated {{- if .Values.secret.create }}; ref .Values.secretRef for external secrets)
# === templates/NOTES.txt === (instructions: verify with `kubectl get pods -l app=gateway`)
```

- [ ] **Step 3: Validate + commit** — `make helm-lint` prints `1 chart(s) linted, 0 failed`; then `git add infra/helm/gateway && git commit -m "infra: helm chart for gateway (ADR-014, ADR-017)"`

---

### Task 6: Helm charts — room-service + game-server

**Files:** Create `infra/helm/room-service/{Chart.yaml,values.yaml,templates/{deployment,service,configmap,pdb}.yaml}`, `infra/helm/game-server/{...,templates/{deployment,service,hpa,configmap,pdb}.yaml}`.

- [ ] **Step 1: room-service chart** — `Chart.yaml` name: `room-service`. `values.yaml`:

```yaml
replicaCount: 2  # Lease leader pair (ADR-011)
image: { repository: ghcr.io/thaolaptrinh/spatial-room-service, tag: dev-latest, pullPolicy: IfNotPresent }
service: { type: ClusterIP, grpcPort: 9000, mgmtPort: 9001 }
resources: { requests: { cpu: 200m, memory: 256Mi }, limits: { cpu: 500m, memory: 512Mi } }
secretRef: room-service-secret
env: { grpc__host: "0.0.0.0", grpc__port: "9000" }
```

`templates/deployment.yaml` — same shape as gateway but `app: room-service`, ports `grpc:9000` + `mgmt:9001`, `readinessProbe: { tcpSocket: { port: 9000 } }`, `serviceAccountName: {{ .Release.Name }}-room-service` (RBAC from Task 9). No HPA (fixed 2 via Lease). `service.yaml`: ClusterIP :9000. `configmap.yaml` + `pdb.yaml` per gateway pattern.

- [ ] **Step 2: game-server chart** — `Chart.yaml` name: `game-server`. `values.yaml`:

```yaml
replicaCount: 3  # stateful, registers with Room Service on start
image: { repository: ghcr.io/thaolaptrinh/spatial-game-server, tag: dev-latest, pullPolicy: IfNotPresent }
service: { type: ClusterIP, grpcPort: 9000, headless: true }
resources: { requests: { cpu: 500m, memory: 1Gi }, limits: { cpu: "2", memory: 2Gi } }  # 5K entities ADR-017
hpa: { enabled: true, minReplicas: 3, maxReplicas: 20, cpuTarget: 70 }
secretRef: game-server-secret
env: { grpc__host: "0.0.0.0", grpc__port: "9000", room_service__addr: "room-service:9000", register_on_start: "true" }
```

`templates/service.yaml` sets `clusterIP: None` when `.Values.service.headless` true. `hpa.yaml` uses CPU target. `deployment.yaml` uses `app: game-server`, readiness `tcpSocket:9000`.

- [ ] **Step 3: Validate + commit** — `make helm-lint` (both clean); then `git add infra/helm/room-service infra/helm/game-server && git commit -m "infra: helm charts for room-service and game-server (ADR-011, ADR-017)"`

---

### Task 7: Helm charts — postgres + redis (managed toggle)

**Files:** Create `infra/helm/{postgres,redis}/{Chart.yaml,values.yaml,templates/{statefulset|deployment,service}.yaml}`.

- [ ] **Step 1: postgres chart** — `values.yaml`: `managed: false`, `image: {repository: postgres, tag: "16-alpine"}`, `persistence: {enabled: true, size: 10Gi, storageClass: ""}`, `credentials: {user: spatial, db: spatial}`. `templates/statefulset.yaml` (gated `{{- if not .Values.managed }}`):

```yaml
apiVersion: apps/v1; kind: StatefulSet; metadata: { name: {{ .Release.Name }}-postgres }
spec:
  serviceName: {{ .Release.Name }}-postgres; replicas: 1; selector: { matchLabels: { app: postgres } }
  template:
    metadata: { labels: { app: postgres } }
    spec:
      containers:
        - name: postgres; image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          env:
            - { name: POSTGRES_USER, valueFrom: { secretKeyRef: { name: {{ .Release.Name }}-pg, key: user } } }
            - { name: POSTGRES_PASSWORD, valueFrom: { secretKeyRef: { name: {{ .Release.Name }}-pg, key: password } } }
            - { name: POSTGRES_DB, value: {{ .Values.credentials.db }} }
          ports: [{ containerPort: 5432 }]; volumeMounts: [{ name: data, mountPath: /var/lib/postgresql/data }]
  volumeClaimTemplates:
    - metadata: { name: data }; spec: { accessModes: [ReadWriteOnce], resources: { requests: { storage: {{ .Values.persistence.size }} } } }
{{- end }}
```

`templates/service.yaml`: ClusterIP :5432, same `if not .Values.managed` gate. `templates/secret.yaml`: creates Secret `<release>-pg` with `user` + `password`.

- [ ] **Step 2: redis chart** — same `managed` gate. `values.yaml`: `managed: false`, `image: {repository: redis, tag: "7-alpine"}`, `persistence: {enabled: true, size: 5Gi}`. `templates/deployment.yaml`: `redis:7-alpine`, no PVC (dev parity with compose). `templates/service.yaml`: ClusterIP :6379. Both gated `{{- if not .Values.managed }}`.

- [ ] **Step 3: Validate + commit** — `make helm-lint && make helm-template` (both render with `managed: false`); then `git add infra/helm/postgres infra/helm/redis && git commit -m "infra: helm charts for postgres and redis (managed toggle)"`

---

### Task 8: Helm chart — monitoring umbrella

**Files:** Create `infra/helm/monitoring/{Chart.yaml,values.yaml,templates/{prometheus,grafana,loki,promtail,otel-collector,alertmanager,rules}.yaml,dashboards/spatial-overview.json}`.

- [ ] **Step 1: values.yaml**

```yaml
prometheus: { retention: 15d, scrapeInterval: 15s }
grafana: { adminPasswordSecret: grafana-admin }
loki: { persistence: { size: 30Gi } }
otel: { endpoint: "tempo:4317", sampling: { production: 0.01, staging: 1.0 } }
alertmanager: { slackWebhookSecret: alertmanager-slack }
```

- [ ] **Step 2: Write templates** — each is a Deployment+Service+ConfigMap shape. Key details:

```yaml
# === templates/prometheus.yaml === (prom/prometheus:v2.52.0, configmap scraping pods with prometheus.io/scrape annotation, ADR-019)
# === templates/grafana.yaml === (grafana/grafana:11.0.0, configmap datasources → prometheus:9090 + loki:3100, dashboards from ConfigMap mounted at /etc/grafana/provisioning/dashboards)
# === templates/loki.yaml === (grafana/loki:3.0.0 :3100, 30Gi PVC)
# === templates/promtail.yaml === (DaemonSet grafana/promtail:3.0.0, volumes /var/lib/docker/containers → /var/lib/docker/containers:ro, config → loki:3100)
# === templates/alertmanager.yaml === (prom/alertmanager:v0.27.0 :9093, slack webhook URL from Secret)
# === templates/otel-collector.yaml === (otel/opentelemetry-collector-contrib:0.103.0 :4317, config: receivers otlp/grpc 0.0.0.0:4317, processors batch, exporters otlp → .Values.otel.endpoint)
# === templates/rules.yaml === (PrometheusRule CRD; spec.groups from ADR-019 + §9)
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata: { name: {{ .Release.Name }}-alerts }
spec:
  groups:
    - name: spatial-server
      rules:
        - { alert: ConnectionDropRateHigh, expr: 'rate(gateway_connections_total[5m]) - on() rate(gateway_connections_active[5m]) > 10', for: 2m, labels: { severity: warning } }
        - { alert: HeartbeatFailures, expr: 'rate(game_server_heartbeat_missed_total[1m]) > 0', for: 15s, labels: { severity: critical } }
        - { alert: EntityCountSpike, expr: 'deriv(game_server_entities_total[1m]) > 3', for: 1m, labels: { severity: warning } }
        - { alert: TickDurationCritical, expr: 'game_server_tick_duration_ms > 100', for: 10s, labels: { severity: critical } }
```

- [ ] **Step 3: Dashboard** — `dashboards/spatial-overview.json`: Grafana dashboard JSON array with 4 panels: `gateway_connections_active` (timeseries), `game_server_entities_total` (timeseries), `game_server_tick_duration_ms` (quantile: p50/p95/p99), `room_service_rpc_duration_ms` (p95 per method). Datasource: Prometheus.

- [ ] **Step 4: Validate + commit** — `make helm-lint && make helm-template`; then `git add infra/helm/monitoring && git commit -m "infra: monitoring chart — prometheus/grafana/loki/promtail/otel/alertmanager (ADR-019)"`

---

### Task 9: K3s cluster manifests + docker-compose.staging.yml

**Files:** Create `infra/k3s/{namespace,priority-classes,lease-rbac,ingress,network-policies}.yaml`, `deploy/docker-compose/docker-compose.staging.yml`.

- [ ] **Step 1: Write K3s manifests**

```yaml
# === namespace.yaml ===
apiVersion: v1; kind: Namespace; metadata: { name: spatial-server, labels: { name: spatial-server } }
# === priority-classes.yaml === (evict monitoring before gameplay)
---
apiVersion: scheduling.k8s.io/v1; kind: PriorityClass; metadata: { name: game-server-critical }; value: 1000000; globalDefault: false
---
apiVersion: scheduling.k8s.io/v1; kind: PriorityClass; metadata: { name: room-service-high }; value: 900000; globalDefault: false
# === lease-rbac.yaml === (ADR-011: Room Service Lease leader election via coordination.k8s.io/leases)
---
apiVersion: v1; kind: ServiceAccount; metadata: { name: room-service, namespace: spatial-server }
---
apiVersion: rbac.authorization.k8s.io/v1; kind: Role; metadata: { name: room-service-lease, namespace: spatial-server }
rules: [{ apiGroups: ["coordination.k8s.io"], resources: ["leases"], verbs: ["get","list","watch","create","update","patch"] }]
---
apiVersion: rbac.authorization.k8s.io/v1; kind: RoleBinding; metadata: { name: room-service-lease, namespace: spatial-server }
roleRef: { apiGroup: rbac.authorization.k8s.io, kind: Role, name: room-service-lease }
subjects: [{ kind: ServiceAccount, name: room-service, namespace: spatial-server }]
# === ingress.yaml === (Traefik, routes /ws → gateway-ws-port, TLS cert ref is placeholder)
apiVersion: networking.k8s.io/v1; kind: Ingress; metadata: { name: spatial-gateway, namespace: spatial-server, annotations: { kubernetes.io/ingress.class: traefik } }
spec:
  rules: [{ host: gateway.spatial.example.com, http: { paths: [{ path: /ws, pathType: Prefix, backend: { service: { name: gateway, port: { number: 8080 } } } }] } }]
  tls: [{ hosts: [gateway.spatial.example.com], secretName: gateway-tls }]  # cert-manager in Phase 6
# === network-policies.yaml === (ADR-018: default-deny-ingress, datastores-private allows room-service+game-server→postgres:5432, redis:6379 only)
apiVersion: networking.k8s.io/v1; kind: NetworkPolicy; metadata: { name: default-deny-ingress, namespace: spatial-server }
spec: { podSelector: {}, policyTypes: [Ingress] }
---
apiVersion: networking.k8s.io/v1; kind: NetworkPolicy; metadata: { name: allow-datastores, namespace: spatial-server }
spec:
  podSelector: { matchLabels: { app: postgres } }
  ingress: [{ from: [{ podSelector: { matchExpressions: [{ key: app, operator: In, values: [room-service, game-server] }] } }], ports: [{ port: 5432 }] }]
---
apiVersion: networking.k8s.io/v1; kind: NetworkPolicy; metadata: { name: allow-redis, namespace: spatial-server }
spec: { podSelector: { matchLabels: { app: redis } }, ingress: [{ from: [{ podSelector: {} }], ports: [{ port: 6379 }] }] }
```

- [ ] **Step 2: docker-compose.staging.yml** (overlay; use `docker compose -f docker-compose.yml -f docker-compose.staging.yml up -d`)

```yaml
services:
  prometheus:   { image: prom/prometheus:v2.52.0,   ports: ["9090:9090"], volumes: ["./prometheus.yml:/etc/prometheus/prometheus.yml"] }
  grafana:      { image: grafana/grafana:11.0.0,    ports: ["3000:3000"], environment: { GF_SECURITY_ADMIN_PASSWORD: admin } }
  loki:         { image: grafana/loki:3.0.0,        ports: ["3100:3100"] }
  promtail:     { image: grafana/promtail:3.0.0,    volumes: ["/var/lib/docker/containers:/var/lib/docker/containers:ro"] }
  alertmanager: { image: prom/alertmanager:v0.27.0, ports: ["9093:9093"] }
```

- [ ] **Step 3: Validate + commit** — `make k3s-dry-run` (all .yaml files pass) + `docker compose -f deploy/docker-compose/docker-compose.yml -f deploy/docker-compose/docker-compose.staging.yml config >/dev/null`; then `git add infra/k3s deploy/docker-compose/docker-compose.staging.yml && git commit -m "infra: k3s manifests and staging compose overlay (ADR-018, ADR-019)"`

---

### Task 10: OpenTelemetry tracing (Go TDD) + CI/CD pipelines

**Files:** Create `pkg/observability/{otel,grpc,otel_test}.go`; Modify `apps/{gateway,room-service,game-server}/main.go`; Create `.github/workflows/{release-images,helm-package,terraform-plan}.yml`.

- [ ] **Step 1: Write failing test** `pkg/observability/otel_test.go`:

```go
package observability
import ("context"; "testing")
func TestInitTracer_ReturnsShutdownFunc(t *testing.T) {
	shutdown, err := InitTracer(context.Background(), "gateway", "localhost:4317", 1.0)
	if err != nil { t.Fatalf("InitTracer: %v", err) }; if shutdown == nil { t.Fatal("shutdown nil") }
	if err := shutdown(context.Background()); err != nil { t.Fatalf("shutdown: %v", err) }
}
```

- [ ] **Step 2: Verify fail** — `go test ./pkg/observability/... -run TestInitTracer -v` → FAIL (`InitTracer` undefined).

- [ ] **Step 3: Implement otel.go + grpc.go**

```go
// === otel.go ===
package observability
import ("context"; "fmt"; "time"
	"go.opentelemetry.io/otel"; "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"; sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"google.golang.org/grpc"; "google.golang.org/grpc/credentials/insecure")
func InitTracer(ctx context.Context, service, collectorAddr string, ratio float64) (func(context.Context) error, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second); defer cancel()
	conn, err := grpc.NewClient(collectorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil { return nil, fmt.Errorf("dial otel collector %s: %w", collectorAddr, err) }
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil { return nil, fmt.Errorf("create otlp exporter: %w", err) }
	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(service)))
	if err != nil { return nil, fmt.Errorf("create resource: %w", err) }
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))))
	otel.SetTracerProvider(tp); return tp.Shutdown, nil
}
// === grpc.go === (gRPC interceptors propagating W3C traceparent via gRPC metadata)
package observability
import ("go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"; "google.golang.org/grpc")
func ServerStatsHandler() grpc.StatsHandler { return otelgrpc.NewServerHandler() }
func ClientStatsHandler() grpc.StatsHandler { return otelgrpc.NewClientHandler() }
func WithTraceContext() grpc.DialOption { return grpc.WithStatsHandler(ClientStatsHandler()) }
```

Run `go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@latest go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc@latest && go mod tidy`.

- [ ] **Step 4: Verify pass** — `go test ./pkg/observability/... -race -v` → PASS.

- [ ] **Step 5: Wire OTel into 3 mains** — `apps/gateway/main.go`: add `observability.WithTraceContext()` to the room-service gRPC dial, call `observability.InitTracer(ctx, "gateway", otelAddr, sampling)` at startup, propagate `trace_id` + `request_id` into slog context. `apps/room-service/main.go`: server `grpc.NewServer(..., grpc.StatsHandler(observability.ServerStatsHandler()))`, service name `"room-service"`. `apps/game-server/main.go`: same server handler, service `"game-server"`. OTel addresses come from config (default disabled when unset).

- [ ] **Step 6: Build + vet** — `go build ./... && go vet ./pkg/observability/... ./apps/...`.

- [ ] **Step 7: Write CI workflows** — 3 new files in `.github/workflows/`:

```yaml
# === release-images.yml === (on push: main + tag v*): checkout, setup-buildx, login ghcr via GITHUB_TOKEN, per svc in [gateway,room-service,game-server]: docker buildx build -f build/docker/${svc}.Dockerfile -t ghcr.io/thaolaptrinh/spatial-${svc}:${GITHUB_REF##*/} --push .
# === helm-package.yml === (on tag v*): setup-helm, for c in infra/helm/*/: helm lint && helm package -d .artifacts/; login ghcr; for p in .artifacts/*.tgz: helm push ${p} oci://ghcr.io/thaolaptrinh/charts
# === terraform-plan.yml === (on PR touching infra/terraform/): terraform fmt -check -recursive -chdir=infra/terraform/environments/staging; init -backend=false; validate; plan -lock=false -input=false (continue-on-error: true)
```

- [ ] **Step 8: Verify + commit** — `for f in .github/workflows/{release-images,helm-package,terraform-plan}.yml; do python3 -c "import yaml; yaml.safe_load(open('$f'))"; done` → no parse errors; then `git add pkg/observability apps/gateway/main.go apps/room-service/main.go apps/game-server/main.go .github/workflows/{release-images,helm-package,terraform-plan}.yml && git commit -m "feat: otel distributed tracing + ci image/helm/terraform pipelines (ADR-019)"`

---

## Self-Review Checklist

**Spec coverage:** infra/+docs/Makefile (T1); TF providers/VPC (T2); TF k3s/node_pool+cloud-init (T3); TF rds/elasticache/dns+staging env (T4); Helm gateway (T5); Helm room-service+game-server (T6); Helm postgres+redis managed toggle (T7); Helm monitoring+rules+dashboards (T8); K3s manifests+staging compose overlay (T9); OTel tracing (Go TDD)+3 CI/CD workflows (T10). All 10 spec sections mapped.

**Placeholder scan:** No "TBD"/"TODO"/"implement later"; all infra code blocks are concrete YAML/HCL (compact `;` one-liners semantically equivalent to expanded form); validation commands replace Go tests for non-Go tasks; OTel task uses real TDD (fail→impl→pass); inline `# === path ===` separators denote separate files.

**Type/path consistency:** env convention `SPATIAL_`+`__` matches existing compose (`SPATIAL_REDIS__ADDR`, `SPATIAL_GATEWAY__JWT_SECRET`); module path `github.com/thaolaptrinh/spatial-server`; images `ghcr.io/thaolaptrinh/spatial-*` consistent across charts + `release-images.yml`; chart dir names match ADR-014 `infra/helm/<service>` layout; `terraform-validate` uses `-backend=false` (S3 state needs creds). Network policies use `matchExpressions`, `operator: In` — valid K8s API. `templates/` YAML uses `{{ .Release.Name }}` prefix for resource uniqueness.

---

## Execution Handoff

Plan complete. Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task (10 tasks), sequential, review between tasks.

**2. Inline Execution** — execute tasks in this session with checkpoints.

Which approach?
