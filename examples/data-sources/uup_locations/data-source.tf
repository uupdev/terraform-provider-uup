# Check a Monitor from every location.
data "uup_locations" "all" {}

resource "uup_monitor" "site" {
  project_id   = data.uup_project.main.id
  type         = "http"
  name         = "Site"
  url          = "https://example.com"
  interval_sec = 60
  locations    = slice(data.uup_locations.all.ids, 0, min(5, length(data.uup_locations.all.ids)))
}
