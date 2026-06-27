variable "subnet_ids"      { type = list(string) }
variable "vpc_id"          { type = string }
variable "allowed_sg_ids"  { type = list(string) }
variable "instance_class"  { type = string; default = "db.t4g.medium" }
variable "db_password"     { type = string; sensitive = true }

resource "aws_db_subnet_group" "this" {
  name       = "spatial"
  subnet_ids = var.subnet_ids
}

resource "aws_security_group" "rds" {
  name   = "spatial-rds"
  vpc_id = var.vpc_id
  ingress {
    from_port       = 5432
    to_port         = 5432
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

resource "aws_db_instance" "postgres" {
  identifier             = "spatial-postgres"
  engine                 = "postgres"
  engine_version         = "16"
  instance_class         = var.instance_class
  allocated_storage      = 50
  db_name                = "spatial"
  username               = "spatial"
  password               = var.db_password
  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  storage_encrypted      = true
  skip_final_snapshot    = false
}

output "endpoint" { value = aws_db_instance.postgres.endpoint }
output "sg_id"    { value = aws_security_group.rds.id }
