terraform {
  backend "s3" {
    bucket         = "terraform-state-storage-975050263614"
    dynamodb_table = "terraform-state-lock-975050263614"
    region         = "us-west-2"

    // THIS MUST BE UNIQUE
    key = "ensign-dev-central-event-system.tfstate"
  }
}

provider "aws" {
  region = "us-west-2"
}

data "aws_ssm_parameter" "eks_cluster_endpoint" {
  name = "/eks/ensign-av-dev-cluster-endpoint"
}

provider "kubernetes" {
  host        = data.aws_ssm_parameter.eks_cluster_endpoint.value
  config_path = "~/.kube/config"
}

data "aws_ssm_parameter" "prd_db_addr" {
  name = "/env/ensign-dev-couch-address"
}

data "aws_ssm_parameter" "prd_db_username" {
  name = "/env/ensign-dev-couch-username"
}

data "aws_ssm_parameter" "prd_db_password" {
  name = "/env/ensign-dev-couch-password"
}
