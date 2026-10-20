provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project   = "gils-garage"
      ManagedBy = "terraform"
      Repo      = var.github_repo
    }
  }
}

# CloudFront only accepts ACM certificates from us-east-1.
provider "aws" {
  alias  = "us_east_1"
  region = "us-east-1"

  default_tags {
    tags = {
      Project   = "gils-garage"
      ManagedBy = "terraform"
      Repo      = var.github_repo
    }
  }
}
