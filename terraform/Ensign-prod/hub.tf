module "event_hub" {
  //source = "github.com/byuoitav/terraform//modules/kubernetes-deployment"
  source = "github.com/byuoitav/terraform-pod-deployment//modules/kubernetes-deployment"

  // required
  name           = "event-hub-prd"
  image          = "byuoitav/central-event-hub"
  image_version  = "latest"
  container_port = 7100
  repo_url       = "https://github.com/byuoitav/central-event-hub"
  cluster        = "av-prd"
  environment    = "production"
  route53_domain = "av.ensign.edu"

  // optional
  public_urls    = ["event-hub-prd.av.ensign.edu"]
  private        = true
  container_env  = {}
  container_args = []
  health_check   = false
}
