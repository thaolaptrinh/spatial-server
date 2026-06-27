terraform {
  required_version = ">= 1.7.0"
  required_providers {
    cloudinit = {
      source  = "hashicorp/cloudinit"
      version = "~> 2.3"
    }
  }
}
