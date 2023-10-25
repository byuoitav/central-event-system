module "event_repeater" {
  //source = "github.com/byuoitav/terraform//modules/kubernetes-deployment"
  source = "github.com/byuoitav/terraform-pod-deployment//modules/kubernetes-deployment"


  // required
  name           = "event-repeater-dev"
  image          = "byuoitav/central-event-repeater"
  image_version  = "latest"
  container_port = 7101
  repo_url       = "https://github.com/byuoitav/central-event-hub"
  cluster        = "av-dev"
  environment    = "dev"
  route53_domain = "avdev.byu.edu"


  // optional
  public_urls = ["event-repeater-dev.avdev.byu.edu"]
  private     = true
  container_env = {
    "DB_ADDRESS"       = "https://${data.aws_ssm_parameter.prd_db_addr.value}",
    "DB_USERNAME"      = data.aws_ssm_parameter.prd_db_username.value,
    "DB_PASSWORD"      = data.aws_ssm_parameter.prd_db_password.value,
    "HUB_ADDRESS"      = "ws://event-hub-dev"
    "STOP_REPLICATION" = "true"
    "SYSTEM_ID"        = "aws-repeater-system"
    "VERSION"          = "0.1.0"
  }
  container_args = []
  health_check   = false
}
