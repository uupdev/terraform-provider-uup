data "uup_channel" "oncall" {
  project_id = data.uup_project.main.id
  name       = "On-call Slack"
}
