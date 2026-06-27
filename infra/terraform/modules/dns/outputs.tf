output "record_name" {
  value = cloudflare_record.gateway.hostname
}

output "record_id" {
  value = cloudflare_record.gateway.id
}
