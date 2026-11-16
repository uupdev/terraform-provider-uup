package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Project is a Project.
type Project struct {
	ID                   string `json:"id"`
	OrganizationID       string `json:"organizationId"`
	Name                 string `json:"name"`
	Timezone             string `json:"timezone"`
	DefaultLocationCount int64  `json:"defaultLocationCount"`
}

// Location is a place a Monitor can be checked from.
type Location struct {
	ID      string  `json:"id"`
	City    string  `json:"city"`
	Country *string `json:"country"`
}

// Channel is where a Notification rule sends its messages.
type Channel struct {
	ID      string  `json:"id"`
	Type    string  `json:"type"`
	Name    string  `json:"name"`
	Address *string `json:"address"`
	State   string  `json:"state"`
}

// Rule is a Notification rule.
type Rule struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	IsDefault            bool      `json:"isDefault"`
	EventTypes           *[]string `json:"eventTypes"`
	ChannelIDs           []string  `json:"channelIds"`
	WarningsDuringOutage bool      `json:"warningsDuringOutage"`
	Repeat               string    `json:"repeat"`
}

// RuleBody is a create's or an edit's body. EventTypes is always sent:
// null means every event.
type RuleBody struct {
	Name                 string    `json:"name"`
	EventTypes           *[]string `json:"eventTypes"`
	ChannelIDs           []string  `json:"channelIds"`
	WarningsDuringOutage *bool     `json:"warningsDuringOutage,omitempty"`
	Repeat               *string   `json:"repeat,omitempty"`
}

// Monitor is a Monitor and its Checks.
type Monitor struct {
	ID           string   `json:"id"`
	ProjectID    string   `json:"projectId"`
	Type         string   `json:"type"`
	Name         string   `json:"name"`
	Description  *string  `json:"description"`
	Target       string   `json:"target"`
	Locations    []string `json:"locations"`
	State        string   `json:"state"`
	OnStatusPage bool     `json:"onStatusPage"`
	RuleIDs      []string `json:"ruleIds"`
	Checks       []Check  `json:"checks"`
}

// Check is one question a Monitor asks of its target.
type Check struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Enabled     bool            `json:"enabled"`
	IntervalSec int64           `json:"intervalSec"`
	Settings    json.RawMessage `json:"settings"`
}

// CheckOf returns the Monitor's Check of this type, or nil.
func (m *Monitor) CheckOf(checkType string) *Check {
	for i := range m.Checks {
		if m.Checks[i].Type == checkType {
			return &m.Checks[i]
		}
	}
	return nil
}

// Settings is the union of every Check's settings, told apart by Kind. Only
// the fields of its kind are set. Secrets come back sealed: a header's value
// and the body as `{ "value": null, "sealed": true }`, the auth as its kind.
type Settings struct {
	Kind string `json:"kind"`

	// http
	Method          string          `json:"method"`
	Headers         []SealedHeader  `json:"headers"`
	Body            json.RawMessage `json:"body"`
	Auth            *SealedAuth     `json:"auth"`
	ExpectedStatus  []string        `json:"expectedStatus"`
	FollowRedirects *bool           `json:"followRedirects"`
	Assertions      []Assertion     `json:"assertions"`

	// http, tcp, ping
	TimeoutMs *int64 `json:"timeoutMs"`

	// tcp, ping
	Host string `json:"host"`
	Port *int64 `json:"port"`

	// ping
	PacketCount      *int64 `json:"packetCount"`
	MaxPacketLossPct *int64 `json:"maxPacketLossPct"`

	// heartbeat
	ExpectedIntervalSec *int64  `json:"expectedIntervalSec"`
	Cron                *string `json:"cron"`
	GraceSec            *int64  `json:"graceSec"`
	URL                 *string `json:"url"`

	// performance
	ThresholdMs *int64 `json:"thresholdMs"`
}

// SealedHeader is a header's name; its value is never shown.
type SealedHeader struct {
	Name string `json:"name"`
}

// SealedAuth is the auth's kind; its credentials are never shown.
type SealedAuth struct {
	Kind string `json:"kind"`
}

// SettingsOf reads a Check's settings, or nil when it has none.
func (c *Check) SettingsOf() (*Settings, error) {
	if c == nil || len(c.Settings) == 0 || string(c.Settings) == "null" {
		return nil, nil
	}
	var settings Settings
	if err := json.Unmarshal(c.Settings, &settings); err != nil {
		return nil, fmt.Errorf("reading the %s Check's settings: %w", c.Type, err)
	}
	return &settings, nil
}

// Header is a header an HTTP Monitor sends.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Auth is the credentials an HTTP Monitor sends.
type Auth struct {
	Kind     string  `json:"kind"`
	Token    *string `json:"token,omitempty"`
	Username *string `json:"username,omitempty"`
	Password *string `json:"password,omitempty"`
}

// Request is how an HTTP Monitor asks. Nil fields are left to the API.
type Request struct {
	Method          *string  `json:"method,omitempty"`
	Headers         []Header `json:"headers"`
	Body            *string  `json:"body"`
	Auth            *Auth    `json:"auth"`
	ExpectedStatus  []string `json:"expectedStatus,omitempty"`
	FollowRedirects *bool    `json:"followRedirects,omitempty"`
	TimeoutMs       *int64   `json:"timeoutMs,omitempty"`
}

// Assertion is what an HTTP Monitor's answer must hold. Value is any JSON
// value: a string, a number, a boolean or null.
type Assertion struct {
	Kind  string           `json:"kind"`
	Op    string           `json:"op"`
	Path  *string          `json:"path,omitempty"`
	Value *json.RawMessage `json:"value,omitempty"`
}

// Switch turns a Check beside uptime on or off.
type Switch struct {
	Enabled     *bool  `json:"enabled,omitempty"`
	ThresholdMs *int64 `json:"thresholdMs,omitempty"`
}

// Switches are the Checks beside uptime a create or an edit turns.
type Switches struct {
	Performance *Switch `json:"performance,omitempty"`
	Certificate *Switch `json:"certificate,omitempty"`
	Domain      *Switch `json:"domain,omitempty"`
}

// MonitorBody is a create's or an edit's body. A nil field is left out:
// on a create the API's default applies, on an edit it stays as it is.
type MonitorBody struct {
	Type                *string      `json:"type,omitempty"`
	Name                *string      `json:"name,omitempty"`
	Description         *NullString  `json:"description,omitempty"`
	URL                 *string      `json:"url,omitempty"`
	Host                *string      `json:"host,omitempty"`
	Port                *int64       `json:"port,omitempty"`
	IntervalSec         *int64       `json:"intervalSec,omitempty"`
	Locations           []string     `json:"locations,omitempty"`
	LocationCount       *int64       `json:"locationCount,omitempty"`
	Request             *Request     `json:"request,omitempty"`
	Assertions          *[]Assertion `json:"assertions,omitempty"`
	TimeoutMs           *int64       `json:"timeoutMs,omitempty"`
	PacketCount         *int64       `json:"packetCount,omitempty"`
	MaxPacketLossPct    *int64       `json:"maxPacketLossPct,omitempty"`
	ExpectedIntervalSec *int64       `json:"expectedIntervalSec,omitempty"`
	Cron                *string      `json:"cron,omitempty"`
	GraceSec            *int64       `json:"graceSec,omitempty"`
	OnStatusPage        *bool        `json:"onStatusPage,omitempty"`
	ValidationID        *string      `json:"validationId,omitempty"`
	Checks              *Switches    `json:"checks,omitempty"`
}

// NullString is a string that is sent as JSON null when Null is set.
type NullString struct {
	Value string
	Null  bool
}

// MarshalJSON sends the string, or null.
func (n NullString) MarshalJSON() ([]byte, error) {
	if n.Null {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

// ValidationBody is the settings a Validation run tests, as the create will
// send them, less the name.
type ValidationBody struct {
	Type             string       `json:"type"`
	URL              *string      `json:"url,omitempty"`
	Host             *string      `json:"host,omitempty"`
	IntervalSec      int64        `json:"intervalSec"`
	Locations        []string     `json:"locations,omitempty"`
	LocationCount    *int64       `json:"locationCount,omitempty"`
	Request          *Request     `json:"request,omitempty"`
	Assertions       *[]Assertion `json:"assertions,omitempty"`
	PacketCount      *int64       `json:"packetCount,omitempty"`
	MaxPacketLossPct *int64       `json:"maxPacketLossPct,omitempty"`
	TimeoutMs        *int64       `json:"timeoutMs,omitempty"`
}

// Validation is a Validation run.
type Validation struct {
	ID         string  `json:"id"`
	State      string  `json:"state"`
	Location   string  `json:"location"`
	StatusCode *int64  `json:"statusCode"`
	ErrorKind  *string `json:"errorKind"`
	Replies    *int64  `json:"replies"`
}

// ListProjects reads every Project the token reaches.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	return list[Project](ctx, c, "/projects")
}

// GetProject reads one Project.
func (c *Client) GetProject(ctx context.Context, projectID string) (*Project, error) {
	var out Project
	if err := c.Do(ctx, http.MethodGet, "/projects/"+esc(projectID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListLocations reads every location a Monitor can be checked from.
func (c *Client) ListLocations(ctx context.Context) ([]Location, error) {
	var out struct {
		Data []Location `json:"data"`
	}
	if err := c.Do(ctx, http.MethodGet, "/locations", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// ListChannels reads a Project's Channels.
func (c *Client) ListChannels(ctx context.Context, projectID string) ([]Channel, error) {
	return list[Channel](ctx, c, "/projects/"+esc(projectID)+"/channels")
}

// ListRules reads a Project's Notification rules.
func (c *Client) ListRules(ctx context.Context, projectID string) ([]Rule, error) {
	return list[Rule](ctx, c, "/projects/"+esc(projectID)+"/rules")
}

// GetRule reads one rule from the Project's list, or a 404 Error.
func (c *Client) GetRule(ctx context.Context, projectID, ruleID string) (*Rule, error) {
	rules, err := c.ListRules(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].ID == ruleID {
			return &rules[i], nil
		}
	}
	return nil, &Error{Status: http.StatusNotFound, Code: "not_found", Message: "No such rule."}
}

// CreateRule makes a rule.
func (c *Client) CreateRule(ctx context.Context, projectID string, body RuleBody) (*Rule, error) {
	var out Rule
	err := c.Do(ctx, http.MethodPost, "/projects/"+esc(projectID)+"/rules", nil, body, &out)
	return &out, err
}

// EditRule changes a rule.
func (c *Client) EditRule(ctx context.Context, projectID, ruleID string, body RuleBody) (*Rule, error) {
	var out Rule
	err := c.Do(ctx, http.MethodPatch, "/projects/"+esc(projectID)+"/rules/"+esc(ruleID), nil, body, &out)
	return &out, err
}

// DeleteRule deletes a rule.
func (c *Client) DeleteRule(ctx context.Context, projectID, ruleID string) error {
	return c.Do(ctx, http.MethodDelete, "/projects/"+esc(projectID)+"/rules/"+esc(ruleID), nil, nil, nil)
}

func monitorPath(projectID, monitorID string) string {
	return "/projects/" + esc(projectID) + "/monitors/" + esc(monitorID)
}

// GetMonitor reads one Monitor.
func (c *Client) GetMonitor(ctx context.Context, projectID, monitorID string) (*Monitor, error) {
	var out Monitor
	if err := c.Do(ctx, http.MethodGet, monitorPath(projectID, monitorID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateMonitor makes a Monitor.
func (c *Client) CreateMonitor(ctx context.Context, projectID string, body MonitorBody) (*Monitor, error) {
	var out Monitor
	err := c.Do(ctx, http.MethodPost, "/projects/"+esc(projectID)+"/monitors", nil, body, &out)
	return &out, err
}

// EditMonitor changes a Monitor.
func (c *Client) EditMonitor(ctx context.Context, projectID, monitorID string, body MonitorBody) (*Monitor, error) {
	var out Monitor
	err := c.Do(ctx, http.MethodPatch, monitorPath(projectID, monitorID), nil, body, &out)
	return &out, err
}

// DeleteMonitor deletes a Monitor.
func (c *Client) DeleteMonitor(ctx context.Context, projectID, monitorID string) error {
	return c.Do(ctx, http.MethodDelete, monitorPath(projectID, monitorID), nil, nil, nil)
}

// PauseMonitor pauses a Monitor.
func (c *Client) PauseMonitor(ctx context.Context, projectID, monitorID string) error {
	return c.Do(ctx, http.MethodPost, monitorPath(projectID, monitorID)+"/pause", nil, nil, nil)
}

// ResumeMonitor resumes a paused Monitor.
func (c *Client) ResumeMonitor(ctx context.Context, projectID, monitorID string) error {
	return c.Do(ctx, http.MethodPost, monitorPath(projectID, monitorID)+"/resume", nil, nil, nil)
}

// AssignRules sets the rules that hear about a Monitor.
func (c *Client) AssignRules(ctx context.Context, projectID, monitorID string, ruleIDs []string) error {
	body := struct {
		RuleIDs []string `json:"ruleIds"`
	}{RuleIDs: ruleIDs}
	if body.RuleIDs == nil {
		body.RuleIDs = []string{}
	}
	return c.Do(ctx, http.MethodPut, monitorPath(projectID, monitorID)+"/rules", nil, body, nil)
}

// Validate starts a Validation run of these settings and waits until a
// location answers it, or the context ends.
func (c *Client) Validate(ctx context.Context, projectID string, body ValidationBody, every time.Duration) (*Validation, error) {
	var run Validation
	path := "/projects/" + esc(projectID) + "/validations"
	if err := c.Do(ctx, http.MethodPost, path, nil, body, &run); err != nil {
		return nil, err
	}
	for run.State == "pending" {
		if err := wait(ctx, every); err != nil {
			return nil, fmt.Errorf("waiting for Validation run %s: %w", run.ID, err)
		}
		if err := c.Do(ctx, http.MethodGet, path+"/"+esc(run.ID), nil, nil, &run); err != nil {
			return nil, err
		}
	}
	return &run, nil
}
