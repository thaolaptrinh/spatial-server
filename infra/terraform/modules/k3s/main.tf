variable "subnet_id"     { type = string }
variable "sg_id"         { type = string }
variable "ssh_pub_key"   { type = string }
variable "instance_type" { type = string; default = "t3.medium" }

data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"]
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"]
  }
}

resource "aws_instance" "k3s_server" {
  ami                    = data.aws_ami.ubuntu.id
  instance_type          = var.instance_type
  subnet_id              = var.subnet_id
  vpc_security_group_ids = [var.sg_id]
  user_data = templatefile("${path.module}/../../cloud-init/k3s-server.yaml", {
    server_private_ip = aws_instance.k3s_server.private_ip
    ssh_pub_key       = var.ssh_pub_key
  })
  tags = {
    Name = "spatial-k3s-server"
    Role = "control-plane"
  }
}
