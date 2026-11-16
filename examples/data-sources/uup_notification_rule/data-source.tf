# The Project's Default rule, which every new Monitor gets.
data "uup_notification_rule" "default" {
  project_id = data.uup_project.main.id
  default    = true
}
