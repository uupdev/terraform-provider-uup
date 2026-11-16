package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeAPI is a small `/v1` in memory, as strict as the real one where the
// provider's logic depends on it: secrets come back sealed, a ping Monitor
// and an http one with assertions need a Validation run, the target never
// changes, and the Default rule is never deleted.
type fakeAPI struct {
	t        *testing.T
	mu       sync.Mutex
	server   *httptest.Server
	next     int
	monitors map[string]*fakeMonitor
	rules    map[string]map[string]any
	runs     map[string]string // validation id → hash of what it tested
	calls    []string
}

type fakeMonitor struct {
	view    map[string]any // what GET answers
	secrets string         // the request with its secrets, as the hash sees it
}

const (
	fakeProject = "prj_test"
	fakeToken   = "uup_test_token"
	defaultRule = "rul_default"
	fakeChannel = "chn_email"
)

func newFakeAPI(t *testing.T) *fakeAPI {
	f := &fakeAPI{
		t:        t,
		monitors: map[string]*fakeMonitor{},
		rules: map[string]map[string]any{
			defaultRule: {
				"id": defaultRule, "name": "Default", "isDefault": true, "eventTypes": nil,
				"channelIds": []string{fakeChannel}, "warningsDuringOutage": false, "repeat": "1h",
				"usedBy": 0, "createdAt": "2026-11-16T00:00:00Z",
			},
		},
		runs: map[string]string{},
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeAPI) URL() string { return f.server.URL + "/v1" }

func (f *fakeAPI) id(prefix string) string {
	f.next++
	return fmt.Sprintf("%s_%d", prefix, f.next)
}

func refuse(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": message}})
}

func answer(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if r.Header.Get("Authorization") != "Bearer "+fakeToken {
		refuse(w, 401, "unauthenticated", "No working API token.")
		return
	}
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)

	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1"), "/"), "/")
	route := r.Method + " " + strings.Join(shape(parts), "/")

	switch route {
	case "GET projects":
		answer(w, 200, map[string]any{"data": []any{f.project()}, "next": nil})
	case "GET projects/:id":
		if parts[1] != fakeProject {
			refuse(w, 404, "not_found", "No such Project.")
			return
		}
		answer(w, 200, f.project())
	case "GET locations":
		answer(w, 200, map[string]any{"data": []any{
			map[string]any{"id": "de", "city": "Frankfurt", "country": "DE"},
			map[string]any{"id": "fi", "city": "Helsinki", "country": "FI"},
			map[string]any{"id": "us", "city": "New York", "country": "US"},
		}})
	case "GET projects/:id/channels":
		answer(w, 200, map[string]any{"data": []any{map[string]any{
			"id": fakeChannel, "type": "email", "name": "Team email", "address": nil, "state": "active",
		}}, "next": nil})
	case "GET projects/:id/rules":
		var all []any
		for _, id := range sortedKeys(f.rules) {
			all = append(all, f.rules[id])
		}
		answer(w, 200, map[string]any{"data": all, "next": nil})
	case "POST projects/:id/rules":
		id := f.id("rul")
		rule := map[string]any{
			"id": id, "name": body["name"], "isDefault": false, "eventTypes": body["eventTypes"],
			"channelIds": body["channelIds"], "warningsDuringOutage": orDefault(body["warningsDuringOutage"], false),
			"repeat": orDefault(body["repeat"], "1h"), "usedBy": 0, "createdAt": "2026-11-16T00:00:00Z",
		}
		f.rules[id] = rule
		answer(w, 201, rule)
	case "PATCH projects/:id/rules/:id":
		rule, ok := f.rules[parts[3]]
		if !ok {
			refuse(w, 404, "not_found", "No such rule.")
			return
		}
		for key, value := range body {
			rule[key] = value
		}
		answer(w, 200, rule)
	case "DELETE projects/:id/rules/:id":
		if parts[3] == defaultRule {
			refuse(w, 409, "default_rule_is_fixed", "The Default rule stays.")
			return
		}
		delete(f.rules, parts[3])
		answer(w, 204, nil)
	case "POST projects/:id/validations":
		id := f.id("val")
		kind, _ := orDefault(body["type"], "http").(string)
		f.runs[id] = hashOf(withDefaults(kind, body))
		answer(w, 202, map[string]any{"id": id, "state": "pending", "location": "de"})
	case "GET projects/:id/validations/:id":
		answer(w, 200, map[string]any{"id": parts[3], "state": "ok", "location": "de", "statusCode": 200, "replies": 3})
	case "POST projects/:id/monitors":
		f.createMonitor(w, body)
	case "GET projects/:id/monitors/:id":
		m, ok := f.monitors[parts[3]]
		if !ok {
			refuse(w, 404, "not_found", "No such Monitor.")
			return
		}
		answer(w, 200, m.view)
	case "PATCH projects/:id/monitors/:id":
		f.editMonitor(w, parts[3], body)
	case "DELETE projects/:id/monitors/:id":
		delete(f.monitors, parts[3])
		answer(w, 204, nil)
	case "POST projects/:id/monitors/:id/pause", "POST projects/:id/monitors/:id/resume":
		m, ok := f.monitors[parts[3]]
		if !ok {
			refuse(w, 404, "not_found", "No such Monitor.")
			return
		}
		if parts[4] == "pause" {
			m.view["state"] = "paused"
		} else {
			m.view["state"] = "active"
		}
		answer(w, 200, m.view)
	case "PUT projects/:id/monitors/:id/rules":
		m, ok := f.monitors[parts[3]]
		if !ok {
			refuse(w, 404, "not_found", "No such Monitor.")
			return
		}
		m.view["ruleIds"] = body["ruleIds"]
		answer(w, 200, map[string]any{"ruleIds": body["ruleIds"]})
	default:
		f.t.Errorf("the fake API has no route %s", route)
		refuse(w, 404, "not_found", "No such route.")
	}
}

// shape names a path's parts, with identifiers as `:id`.
func shape(parts []string) []string {
	out := make([]string, len(parts))
	for i, part := range parts {
		if i%2 == 1 {
			out[i] = ":id"
		} else {
			out[i] = part
		}
	}
	return out
}

func (f *fakeAPI) project() map[string]any {
	return map[string]any{
		"id": fakeProject, "organizationId": "org_test", "name": "Test project", "timezone": "Europe/Istanbul",
		"defaultLocationCount": 2, "suspendedAt": nil, "createdAt": "2026-11-16T00:00:00Z",
	}
}

func orDefault(value, fallback any) any {
	if value == nil {
		return fallback
	}
	return value
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// settingsOf is what a Validation run is matched by: the type, the target
// and the probed settings, secrets included.
func settingsOf(kind string, all map[string]any) map[string]any {
	keep := map[string][]string{
		"http": {"url", "request", "assertions"},
		"ping": {"host", "packetCount", "maxPacketLossPct", "timeoutMs"},
	}[kind]
	out := map[string]any{"type": kind}
	for _, key := range keep {
		out[key] = all[key]
	}
	return out
}

func hashOf(body map[string]any) string {
	kind, _ := body["type"].(string)
	encoded, _ := json.Marshal(settingsOf(kind, body))
	return string(encoded)
}

func needsRun(kind string, assertions any) bool {
	if kind == "ping" {
		return true
	}
	list, _ := assertions.([]any)
	return kind == "http" && len(list) > 0
}

// checkRun refuses a create or an edit whose settings need a run it did not
// name, or name one of other settings.
func (f *fakeAPI) checkRun(w http.ResponseWriter, held map[string]any, validationID any) bool {
	kind, _ := held["type"].(string)
	if !needsRun(kind, held["assertions"]) {
		return true
	}
	id, _ := validationID.(string)
	tested, ok := f.runs[id]
	if !ok {
		refuse(w, 422, "validation_required", "Test these settings first.")
		return false
	}
	if tested != hashOf(held) {
		refuse(w, 422, "validation_mismatch", "The run tested other settings.")
		return false
	}
	return true
}

func withDefaults(kind string, body map[string]any) map[string]any {
	held := map[string]any{}
	for key, value := range body {
		held[key] = value
	}
	held["type"] = kind
	switch kind {
	case "http":
		request, _ := held["request"].(map[string]any)
		if request == nil {
			request = map[string]any{}
		}
		for key, value := range map[string]any{
			"method": "GET", "headers": []any{}, "body": nil, "auth": nil,
			"expectedStatus": []any{"2xx"}, "followRedirects": true, "timeoutMs": float64(10000),
		} {
			if _, ok := request[key]; !ok {
				request[key] = value
			}
		}
		held["request"] = request
		if held["assertions"] == nil {
			held["assertions"] = []any{}
		}
	case "tcp":
		held["timeoutMs"] = orDefault(held["timeoutMs"], float64(10000))
	case "ping":
		held["packetCount"] = orDefault(held["packetCount"], float64(3))
		held["maxPacketLossPct"] = orDefault(held["maxPacketLossPct"], float64(0))
		held["timeoutMs"] = orDefault(held["timeoutMs"], float64(5000))
	case "heartbeat":
		held["graceSec"] = orDefault(held["graceSec"], float64(300))
	}
	return held
}

func (f *fakeAPI) createMonitor(w http.ResponseWriter, body map[string]any) {
	kind, _ := orDefault(body["type"], "http").(string)
	held := withDefaults(kind, body)
	if !f.checkRun(w, held, body["validationId"]) {
		return
	}
	delete(held, "validationId")

	id := f.id("mon")
	locations := []any{}
	if kind != "heartbeat" {
		if asked, ok := held["locations"].([]any); ok {
			locations = asked
		} else {
			count := 2
			if asked, ok := held["locationCount"].(float64); ok {
				count = int(asked)
			}
			locations = []any{"de", "fi", "us"}[:count]
		}
	}
	held["locations"] = locations
	delete(held, "locationCount")

	view := map[string]any{
		"id": id, "projectId": fakeProject, "type": kind, "name": held["name"],
		"description": held["description"], "state": "active", "status": "pending",
		"onStatusPage": orDefault(held["onStatusPage"], false), "ruleIds": []any{defaultRule},
		"pausedAt": nil, "suspendedAt": nil, "createdAt": "2026-11-16T00:00:00Z", "standIns": map[string]any{},
	}
	switches, _ := held["checks"].(map[string]any)
	f.monitors[id] = &fakeMonitor{view: view}
	f.render(id, held, switches)
	answer(w, 201, view)
}

func (f *fakeAPI) editMonitor(w http.ResponseWriter, id string, body map[string]any) {
	m, ok := f.monitors[id]
	if !ok {
		refuse(w, 404, "not_found", "No such Monitor.")
		return
	}
	held := map[string]any{}
	_ = json.Unmarshal([]byte(m.secrets), &held)
	before := hashOf(held)
	for key, value := range body {
		if key == "checks" || key == "validationId" {
			continue
		}
		held[key] = value
	}
	held = withDefaults(held["type"].(string), held)
	if hashOf(held) != before && !f.checkRun(w, held, body["validationId"]) {
		return
	}
	switches, _ := body["checks"].(map[string]any)
	f.render(id, held, switches)
	answer(w, 200, m.view)
}

// render writes what GET answers from the held settings, sealing secrets.
func (f *fakeAPI) render(id string, held map[string]any, switches map[string]any) {
	m := f.monitors[id]
	encoded, _ := json.Marshal(held)
	m.secrets = string(encoded)
	kind := held["type"].(string)
	m.view["name"] = held["name"]
	m.view["description"] = held["description"]
	if value, ok := held["onStatusPage"]; ok {
		m.view["onStatusPage"] = value
	}
	m.view["locations"] = held["locations"]

	old := map[string]map[string]any{}
	if checks, ok := m.view["checks"].([]any); ok {
		for _, one := range checks {
			check := one.(map[string]any)
			old[check["type"].(string)] = check
		}
	}
	interval := held["intervalSec"]
	var checks []any
	check := func(checkType string, enabled bool, settings any) {
		if previous, ok := old[checkType]; ok && switches[checkType] == nil {
			enabled = previous["enabled"].(bool)
			if checkType == "performance" && settings == nil {
				settings = previous["settings"]
			}
		}
		if asked, ok := switches[checkType].(map[string]any); ok {
			if on, ok := asked["enabled"].(bool); ok {
				enabled = on
			}
		}
		checks = append(checks, map[string]any{
			"id": "chk_" + checkType + "_" + id, "type": checkType, "enabled": enabled, "intervalSec": interval,
			"status": "pending", "snoozedUntil": nil, "settings": settings, "notApplicable": nil,
			"heartbeatDeadline": nil, "openIncident": nil, "createdAt": "2026-11-16T00:00:00Z",
		})
	}

	switch kind {
	case "http":
		request := held["request"].(map[string]any)
		var headers []any
		for _, one := range request["headers"].([]any) {
			headers = append(headers, map[string]any{"name": one.(map[string]any)["name"], "value": nil, "sealed": true})
		}
		if headers == nil {
			headers = []any{}
		}
		var sealedBody any
		if request["body"] != nil {
			sealedBody = map[string]any{"value": nil, "sealed": true}
		}
		var auth any
		if asked, ok := request["auth"].(map[string]any); ok {
			auth = map[string]any{"kind": asked["kind"]}
		}
		m.view["target"] = held["url"]
		check("uptime", true, map[string]any{
			"kind": "http", "method": request["method"], "headers": headers, "body": sealedBody, "auth": auth,
			"expectedStatus": request["expectedStatus"], "followRedirects": request["followRedirects"],
			"timeoutMs": request["timeoutMs"], "assertions": held["assertions"],
		})
		https := strings.HasPrefix(held["url"].(string), "https://")
		check("certificate", https, map[string]any{"kind": "certificate"})
		check("domain", true, map[string]any{"kind": "domain"})
	case "tcp":
		m.view["target"] = fmt.Sprintf("%s:%v", held["host"], held["port"])
		check("uptime", true, map[string]any{"kind": "tcp", "host": held["host"], "port": held["port"], "timeoutMs": held["timeoutMs"]})
	case "ping":
		m.view["target"] = held["host"]
		check("uptime", true, map[string]any{
			"kind": "ping", "host": held["host"], "packetCount": held["packetCount"],
			"maxPacketLossPct": held["maxPacketLossPct"], "timeoutMs": held["timeoutMs"],
		})
	case "heartbeat":
		m.view["target"] = held["name"]
		interval = orDefault(held["expectedIntervalSec"], float64(3600))
		check("uptime", true, map[string]any{
			"kind": "heartbeat", "expectedIntervalSec": held["expectedIntervalSec"], "cron": held["cron"],
			"graceSec": held["graceSec"], "url": "https://api.uup.dev/hb/" + id,
		})
	}
	if kind != "heartbeat" {
		var threshold any
		if asked, ok := switches["performance"].(map[string]any); ok {
			threshold = asked["thresholdMs"]
		}
		var settings any
		if threshold != nil {
			settings = map[string]any{"kind": "performance", "thresholdMs": threshold}
		} else if _, ok := old["performance"]; !ok {
			settings = map[string]any{"kind": "performance", "thresholdMs": float64(2000)}
		}
		check("performance", true, settings)
	}
	m.view["checks"] = checks
}
