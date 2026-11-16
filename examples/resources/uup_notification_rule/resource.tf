data "uup_project" "main" {
  name = "My project"
}

data "uup_channel" "oncall" {
  project_id = data.uup_project.main.id
  name       = "On-call Slack"
}

resource "uup_notification_rule" "outages" {
  project_id  = data.uup_project.main.id
  name        = "Outages to on-call"
  event_types = ["uptime.failed", "uptime.recovered"]
  channel_ids = [data.uup_channel.oncall.id]
  repeat      = "30m"
}

# Assign the rule to a Monitor.
resource "uup_monitor" "api" {
  project_id   = data.uup_project.main.id
  type         = "http"
  name         = "API"
  url          = "https://api.example.com/health"
  interval_sec = 60
  rule_ids     = [uup_notification_rule.outages.id]
}
