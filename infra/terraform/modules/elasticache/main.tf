variable "subnet_ids"      { type = list(string) }
variable "vpc_id"          { type = string }
variable "allowed_sg_ids"  { type = list(string) }
variable "node_type"       { type = string; default = "cache.t4g.small" }

resource "aws_elasticache_subnet_group" "this" {
  name       = "spatial-redis"
  subnet_ids = var.subnet_ids
}

resource "aws_security_group" "redis" {
  name   = "spatial-redis"
  vpc_id = var.vpc_id
  ingress {
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = var.allowed_sg_ids
  }
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_elasticache_replication_group" "redis" {
  replication_group_id = "spatial-redis"
  description          = "spatial Redis HA"

  node_type               = var.node_type
  port                    = 6379
  parameter_group_name    = "default.redis7"
  engine_version          = "7"
  number_cache_clusters   = 2
  subnet_group_name        = aws_elasticache_subnet_group.this.name
  security_group_ids       = [aws_security_group.redis.id]
  automatic_failover      = true
  transit_encryption_enabled = true
}

output "primary_endpoint" { value = aws_elasticache_replication_group.redis.primary_endpoint_address }
