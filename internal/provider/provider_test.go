package provider

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"uup": providerserver.NewProtocol6WithError(New("test")()),
}

func init() {
	ValidationPoll = 10 * time.Millisecond
}

func providerBlock(f *fakeAPI) string {
	return fmt.Sprintf(`
provider "uup" {
  api_token = %q
  base_url  = %q
}
`, fakeToken, f.URL())
}

// Secrets the API never shows, which an import reads as empty.
var secrets = []string{"headers", "body", "bearer_token", "basic_auth_username", "basic_auth_password"}

func TestHTTPMonitor(t *testing.T) {
	f := newFakeAPI(t)
	first := providerBlock(f) + `
resource "uup_monitor" "site" {
  project_id   = "prj_test"
  type         = "http"
  name         = "Site"
  url          = "https://example.com/health"
  interval_sec = 60
  locations    = ["de", "us"]

  headers      = { "X-Check" = "secret-1" }
  bearer_token = "token-1"
  expected_status = ["200", "3xx"]

  assertions = [
    { kind = "keyword", op = "contains", value = "ok" },
    { kind = "json", op = "eq", path = "$.code", value = "200", value_type = "number" },
    { kind = "json", op = "exists", path = "$.version" },
  ]

  performance_threshold_ms = 1500
}
`
	second := providerBlock(f) + `
resource "uup_monitor" "site" {
  project_id   = "prj_test"
  type         = "http"
  name         = "Site, renamed"
  description  = "The public site"
  url          = "https://example.com/health"
  interval_sec = 120
  locations    = ["de", "us"]

  headers      = { "X-Check" = "secret-2" }
  bearer_token = "token-1"
  expected_status = ["200", "3xx"]

  assertions = [
    { kind = "keyword", op = "contains", value = "ok" },
    { kind = "json", op = "eq", path = "$.code", value = "200", value_type = "number" },
    { kind = "json", op = "exists", path = "$.version" },
  ]

  performance_threshold_ms = 1500
  domain_check_enabled     = false
  paused                   = true
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: first,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestMatchResourceAttr("uup_monitor.site", "id", regexp.MustCompile(`^mon_`)),
					resource.TestCheckResourceAttr("uup_monitor.site", "method", "GET"),
					resource.TestCheckResourceAttr("uup_monitor.site", "follow_redirects", "true"),
					resource.TestCheckResourceAttr("uup_monitor.site", "timeout_ms", "10000"),
					resource.TestCheckResourceAttr("uup_monitor.site", "headers.X-Check", "secret-1"),
					resource.TestCheckResourceAttr("uup_monitor.site", "assertions.1.value", "200"),
					resource.TestCheckResourceAttr("uup_monitor.site", "assertions.1.value_type", "number"),
					resource.TestCheckNoResourceAttr("uup_monitor.site", "assertions.2.value"),
					resource.TestCheckResourceAttr("uup_monitor.site", "certificate_check_enabled", "true"),
					resource.TestCheckResourceAttr("uup_monitor.site", "performance_check_enabled", "true"),
					resource.TestCheckResourceAttr("uup_monitor.site", "performance_threshold_ms", "1500"),
					resource.TestCheckResourceAttr("uup_monitor.site", "rule_ids.#", "1"),
					resource.TestCheckResourceAttr("uup_monitor.site", "paused", "false"),
					resource.TestCheckNoResourceAttr("uup_monitor.site", "heartbeat_url"),
				),
			},
			{
				// A changed header value is a changed setting: the provider
				// tests it again before the edit, or the fake refuses it.
				Config: second,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uup_monitor.site", "name", "Site, renamed"),
					resource.TestCheckResourceAttr("uup_monitor.site", "description", "The public site"),
					resource.TestCheckResourceAttr("uup_monitor.site", "interval_sec", "120"),
					resource.TestCheckResourceAttr("uup_monitor.site", "headers.X-Check", "secret-2"),
					resource.TestCheckResourceAttr("uup_monitor.site", "domain_check_enabled", "false"),
					resource.TestCheckResourceAttr("uup_monitor.site", "paused", "true"),
				),
			},
			{
				ResourceName:            "uup_monitor.site",
				ImportState:             true,
				ImportStateIdFunc:       importID("uup_monitor.site"),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: secrets,
			},
		},
	})
}

func TestTCPPingAndHeartbeatMonitors(t *testing.T) {
	f := newFakeAPI(t)
	config := providerBlock(f) + `
resource "uup_monitor" "db" {
  project_id     = "prj_test"
  type           = "tcp"
  name           = "Database"
  host           = "db.example.com"
  port           = 5432
  interval_sec   = 60
  location_count = 3
}

resource "uup_monitor" "router" {
  project_id   = "prj_test"
  type         = "ping"
  name         = "Router"
  host         = "router.example.com"
  interval_sec = 60
  packet_count = 5
}

resource "uup_monitor" "backup" {
  project_id            = "prj_test"
  type                  = "heartbeat"
  name                  = "Nightly backup"
  expected_interval_sec = 86400
  grace_sec             = 600
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uup_monitor.db", "locations.#", "3"),
					resource.TestCheckResourceAttr("uup_monitor.db", "timeout_ms", "10000"),
					resource.TestCheckNoResourceAttr("uup_monitor.db", "certificate_check_enabled"),
					resource.TestCheckResourceAttr("uup_monitor.router", "packet_count", "5"),
					resource.TestCheckResourceAttr("uup_monitor.router", "max_packet_loss_pct", "0"),
					resource.TestMatchResourceAttr("uup_monitor.backup", "heartbeat_url", regexp.MustCompile(`^https://api\.uup\.dev/hb/mon_`)),
					resource.TestCheckResourceAttr("uup_monitor.backup", "grace_sec", "600"),
					resource.TestCheckNoResourceAttr("uup_monitor.backup", "interval_sec"),
					resource.TestCheckNoResourceAttr("uup_monitor.backup", "performance_check_enabled"),
				),
			},
			{
				ResourceName:      "uup_monitor.db",
				ImportState:       true,
				ImportStateIdFunc: importID("uup_monitor.db"),
				ImportStateVerify: true,
				// Read only when the Monitor is made.
				ImportStateVerifyIgnore: []string{"location_count"},
			},
			{
				ResourceName:      "uup_monitor.router",
				ImportState:       true,
				ImportStateIdFunc: importID("uup_monitor.router"),
				ImportStateVerify: true,
			},
			{
				ResourceName:      "uup_monitor.backup",
				ImportState:       true,
				ImportStateIdFunc: importID("uup_monitor.backup"),
				ImportStateVerify: true,
			},
		},
	})
}

func TestRulesAndDataSources(t *testing.T) {
	f := newFakeAPI(t)
	config := providerBlock(f) + `
data "uup_project" "main" {
  name = "Test project"
}

data "uup_channel" "email" {
  project_id = data.uup_project.main.id
  name       = "Team email"
}

data "uup_notification_rule" "default" {
  project_id = data.uup_project.main.id
  default    = true
}

data "uup_locations" "all" {}

resource "uup_notification_rule" "outages" {
  project_id  = data.uup_project.main.id
  name        = "Outages only"
  event_types = ["uptime.failed", "uptime.recovered"]
  channel_ids = [data.uup_channel.email.id]
  repeat      = "30m"
}

resource "uup_monitor" "api" {
  project_id   = data.uup_project.main.id
  type         = "http"
  name         = "API"
  url          = "https://api.example.com"
  interval_sec = 30
  locations    = data.uup_locations.all.ids
  rule_ids     = [uup_notification_rule.outages.id, data.uup_notification_rule.default.id]
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.uup_project.main", "id", "prj_test"),
					resource.TestCheckResourceAttr("data.uup_channel.email", "id", "chn_email"),
					resource.TestCheckResourceAttr("data.uup_notification_rule.default", "id", "rul_default"),
					resource.TestCheckResourceAttr("data.uup_locations.all", "ids.#", "3"),
					resource.TestCheckResourceAttr("uup_notification_rule.outages", "warnings_during_outage", "false"),
					resource.TestCheckResourceAttr("uup_notification_rule.outages", "is_default", "false"),
					resource.TestCheckResourceAttr("uup_monitor.api", "rule_ids.#", "2"),
					resource.TestCheckResourceAttr("uup_monitor.api", "locations.#", "3"),
				),
			},
			{
				ResourceName:      "uup_notification_rule.outages",
				ImportState:       true,
				ImportStateIdFunc: importID("uup_notification_rule.outages"),
				ImportStateVerify: true,
			},
		},
	})
}

func TestConfigRefusals(t *testing.T) {
	f := newFakeAPI(t)
	cases := map[string]struct {
		body string
		want string
	}{
		"a port on an http Monitor": {
			body: `type = "http"
  url = "https://example.com"
  interval_sec = 60
  port = 443`,
			want: `A http Monitor does not take port`,
		},
		"a tcp Monitor with no port": {
			body: `type = "tcp"
  host = "db.example.com"
  interval_sec = 60`,
			want: `A tcp Monitor needs port`,
		},
		"a heartbeat with no schedule": {
			body: `type = "heartbeat"`,
			want: `needs expected_interval_sec or cron`,
		},
		"a keyword with a path": {
			body: `type = "http"
  url = "https://example.com"
  interval_sec = 60
  assertions = [{ kind = "keyword", op = "contains", value = "ok", path = "$.x" }]`,
			want: `A keyword assertion takes no path`,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories,
				Steps: []resource.TestStep{{
					Config: providerBlock(f) + fmt.Sprintf(`
resource "uup_monitor" "bad" {
  project_id = "prj_test"
  name       = "Bad"
  %s
}
`, c.body),
					ExpectError: regexp.MustCompile(regexp.QuoteMeta(c.want)),
				}},
			})
		})
	}
}

func importID(name string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		found, ok := s.RootModule().Resources[name]
		if !ok {
			return "", fmt.Errorf("no %s in the state", name)
		}
		return found.Primary.Attributes["project_id"] + "/" + found.Primary.ID, nil
	}
}
