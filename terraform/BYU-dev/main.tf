terraform {
  backend "s3" {
    bucket         = "terraform-state-storage-887007127029"
    dynamodb_table = "terraform-state-lock-887007127029"
    region         = "us-west-2"

    // THIS MUST BE UNIQUE
    key = "dev-central-event-system.tfstate"
  }
}

provider "aws" {
  region = "us-west-2"
}

data "aws_ssm_parameter" "eks_cluster_endpoint" {
  name = "/eks/av-dev-cluster-endpoint"
}

provider "kubernetes" {
  host        = data.aws_ssm_parameter.eks_cluster_endpoint.value
  config_path = "~/.kube/config"
}

data "aws_ssm_parameter" "prd_db_addr" {
  name = "/env/couch-address"
}

data "aws_ssm_parameter" "prd_db_username" {
  name = "/env/couch-username"
}

data "aws_ssm_parameter" "prd_db_password" {
  name = "/env/couch-password"
}
