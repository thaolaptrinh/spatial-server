output "server_private_ip" { value = aws_instance.k3s_server.private_ip }
output "server_id"         { value = aws_instance.k3s_server.id }
