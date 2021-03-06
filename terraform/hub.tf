module "event_hub" {
  source = "github.com/byuoitav/terraform//modules/kubernetes-deployment"

  // required
  name           = "event-hub"
  image          = "byuoitav/central-event-hub"
  image_version  = "latest"
  container_port = 7100
  repo_url       = "https://github.com/byuoitav/central-event-hub"

  // optional
  public_urls    = ["event-hub.av.byu.edu"]
  private        = true
  container_env  = {}
  container_args = []
  health_check   = false
}
