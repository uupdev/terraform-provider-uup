# Changelog

## 0.1.0

First release.

- Resource `uup_monitor`: `http`, `tcp`, `ping` and `heartbeat` Monitors, with performance, certificate and domain Checks, pause, and assigned Notification rules. Runs the Validation a `ping` Monitor, or an `http` Monitor with assertions, needs before it is saved.
- Resource `uup_notification_rule`.
- Data sources `uup_project`, `uup_channel`, `uup_notification_rule` and `uup_locations`.
- `terraform import` for both resources, as `<project_id>/<id>`.
