terraform {
  cloud {
    organization = "<ORG>"

    workspaces {
      name = "spatial-staging"
    }
  }
}
