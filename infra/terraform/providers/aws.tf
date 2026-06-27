provider "aws" {
  region = var.aws_region
  default_tags {
    tags = {
      Project   = "spatial-server"
      ManagedBy = "terraform"
    }
  }
}

variable "aws_region" {
  type    = string
  default = "us-east-1"
}
