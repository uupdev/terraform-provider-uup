# Terraform provider for You Up?

You Up? is an uptime monitoring service that checks your websites, APIs, servers and cron jobs from several locations and alerts you by email, Slack, WhatsApp, SMS and more, only when an outage is confirmed.

This provider manages [You Up?](https://uup.dev/) Monitors and Notification rules as code, with [Terraform](https://www.terraform.io/) or [OpenTofu](https://opentofu.org/). It uses the [You Up? API](https://docs.uup.dev/api/).

- **Resources:** `uup_monitor` (`http`, `tcp`, `ping` and `heartbeat`) and `uup_notification_rule`. Both support `terraform import`.
- **Data sources:** `uup_project`, `uup_channel`, `uup_notification_rule` and `uup_locations`.

## Usage

```terraform
terraform {
  required_providers {
    uup = {
      source = "uupdev/uup"
    }
  }
}

provider "uup" {}

data "uup_project" "main" {
  name = "My project"
}

resource "uup_monitor" "api" {
  project_id   = data.uup_project.main.id
  type         = "http"
  name         = "API health"
  url          = "https://api.example.com/health"
  interval_sec = 60
}

resource "uup_monitor" "backup" {
  project_id = data.uup_project.main.id
  type       = "heartbeat"
  name       = "Nightly backup"
  cron       = "0 3 * * *"
  grace_sec  = 900
}
```

Make an API token of the **Full** kind on the dashboard's **API tokens** screen ([how](https://docs.uup.dev/api/tokens/)), and set it in the `UUP_API_TOKEN` environment variable.

The full documentation is on the [Terraform Registry](https://registry.terraform.io/providers/uupdev/uup/latest/docs) and in [`docs/`](docs/).

## Things to know

- **Channels are managed in the dashboard.** Most Channels need a code or a link that a person confirms, so the API does not create them. Look one up with the `uup_channel` data source and use its `id` in a rule.
- **Secrets are write-only.** You Up? never shows a Monitor's header values, body or credentials again. The provider keeps them in the Terraform state, so protect your state. After an import, they read as empty until the next apply sets them.
- **Some Monitors are tested before they are saved.** A `ping` Monitor, and an `http` Monitor with `assertions`, need one test from one location. The provider runs this test and waits for its answer, which can take a few seconds.
- **The target never changes.** Changing `type`, `url`, `host` or `port` replaces the Monitor. A replaced `heartbeat` Monitor gets a new Heartbeat URL.

## Development

The provider is written in Go with the [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework).

```sh
go build ./...
go test ./...       # runs against an in-memory fake of the API; downloads Terraform
go generate ./...   # rebuilds docs/ from the schema, templates/ and examples/
```

To release, push a tag such as `v0.1.0`. The release workflow builds, signs and publishes it with GoReleaser.

## License

MIT
