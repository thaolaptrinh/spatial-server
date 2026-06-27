terraform {
  backend "s3" {
    bucket         = "spatial-tfstate"
    key            = "staging/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "spatial-tf-locks"
    encrypt        = true
  }
}
