data "uup_project" "main" {
  name = "My project"
}

# An HTTP Monitor checked from two locations, with an assertion on the body.
resource "uup_monitor" "api" {
  project_id   = data.uup_project.main.id
  type         = "http"
  name         = "API health"
  url          = "https://api.example.com/health"
  interval_sec = 60
  locations    = ["de", "us"]

  headers         = { "X-Health-Key" = var.health_key }
  expected_status = ["200"]

  assertions = [
    { kind = "json", op = "eq", path = "$.status", value = "ok" },
  ]

  performance_threshold_ms = 1500
}

# A TCP port.
resource "uup_monitor" "database" {
  project_id     = data.uup_project.main.id
  type           = "tcp"
  name           = "Database"
  host           = "db.example.com"
  port           = 5432
  interval_sec   = 60
  location_count = 3
}

# A host that answers ping.
resource "uup_monitor" "router" {
  project_id   = data.uup_project.main.id
  type         = "ping"
  name         = "Office router"
  host         = "203.0.113.10"
  interval_sec = 120
}

# A Heartbeat your nightly job calls when it succeeds.
resource "uup_monitor" "backup" {
  project_id = data.uup_project.main.id
  type       = "heartbeat"
  name       = "Nightly backup"
  cron       = "0 3 * * *"
  grace_sec  = 900
}

output "backup_heartbeat_url" {
  value     = uup_monitor.backup.heartbeat_url
  sensitive = true
}

variable "health_key" {
  type      = string
  sensitive = true
}
