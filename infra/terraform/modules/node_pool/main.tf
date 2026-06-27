variable "name"              { type = string }
variable "instance_type"     { type = string; default = "t3.large" }
variable "desired"           { type = number; default = 2 }
variable "min_size"          { type = number; default = 1 }
variable "max_size"          { type = number; default = 6 }
variable "subnet_ids"        { type = list(string) }
variable "sg_id"             { type = string }
variable "server_private_ip" { type = string }
variable "k3s_token"         { type = string }
variable "ssh_pub_key"       { type = string }

data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"]
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"]
  }
}

resource "aws_key_pair" "this" {
  key_name   = "${var.name}-key"
  public_key = var.ssh_pub_key
}

resource "aws_launch_template" "agent" {
  name_prefix   = "${var.name}-"
  image_id      = data.aws_ami.ubuntu.id
  instance_type = var.instance_type
  key_name      = aws_key_pair.this.key_name
  user_data = base64encode(templatefile("${path.module}/../../cloud-init/k3s-agent.yaml", {
    server_private_ip = var.server_private_ip
    k3s_token         = var.k3s_token
    ssh_pub_key       = var.ssh_pub_key
  }))
}

resource "aws_autoscaling_group" "agents" {
  name                = var.name
  desired_capacity    = var.desired
  min_size            = var.min_size
  max_size            = var.max_size
  vpc_zone_identifier = var.subnet_ids
  launch_template {
    id      = aws_launch_template.agent.id
    version = "$Latest"
  }
}
