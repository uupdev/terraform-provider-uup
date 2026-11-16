package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/uupdev/terraform-provider-uup/internal/client"
)

// ValidationPoll is how often a Validation run is asked for its answer.
// Tests shorten it.
var ValidationPoll = 2 * time.Second

var (
	_ resource.Resource                   = &monitorResource{}
	_ resource.ResourceWithConfigure      = &monitorResource{}
	_ resource.ResourceWithImportState    = &monitorResource{}
	_ resource.ResourceWithValidateConfig = &monitorResource{}
)

// NewMonitorResource is uup_monitor.
func NewMonitorResource() resource.Resource {
	return &monitorResource{}
}

type monitorResource struct {
	client *client.Client
}

type monitorModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.String `tfsdk:"project_id"`
	Type        types.String `tfsdk:"type"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`

	URL  types.String `tfsdk:"url"`
	Host types.String `tfsdk:"host"`
	Port types.Int64  `tfsdk:"port"`

	IntervalSec   types.Int64 `tfsdk:"interval_sec"`
	Locations     types.Set   `tfsdk:"locations"`
	LocationCount types.Int64 `tfsdk:"location_count"`
	TimeoutMs     types.Int64 `tfsdk:"timeout_ms"`

	Method            types.String `tfsdk:"method"`
	Headers           types.Map    `tfsdk:"headers"`
	Body              types.String `tfsdk:"body"`
	BearerToken       types.String `tfsdk:"bearer_token"`
	BasicAuthUsername types.String `tfsdk:"basic_auth_username"`
	BasicAuthPassword types.String `tfsdk:"basic_auth_password"`
	ExpectedStatus    types.List   `tfsdk:"expected_status"`
	FollowRedirects   types.Bool   `tfsdk:"follow_redirects"`
	Assertions        types.List   `tfsdk:"assertions"`

	PacketCount      types.Int64 `tfsdk:"packet_count"`
	MaxPacketLossPct types.Int64 `tfsdk:"max_packet_loss_pct"`

	ExpectedIntervalSec types.Int64  `tfsdk:"expected_interval_sec"`
	Cron                types.String `tfsdk:"cron"`
	GraceSec            types.Int64  `tfsdk:"grace_sec"`
	HeartbeatURL        types.String `tfsdk:"heartbeat_url"`

	PerformanceCheckEnabled types.Bool  `tfsdk:"performance_check_enabled"`
	PerformanceThresholdMs  types.Int64 `tfsdk:"performance_threshold_ms"`
	CertificateCheckEnabled types.Bool  `tfsdk:"certificate_check_enabled"`
	DomainCheckEnabled      types.Bool  `tfsdk:"domain_check_enabled"`

	OnStatusPage types.Bool `tfsdk:"on_status_page"`
	Paused       types.Bool `tfsdk:"paused"`
	RuleIDs      types.Set  `tfsdk:"rule_ids"`
}

type assertionModel struct {
	Kind      types.String `tfsdk:"kind"`
	Op        types.String `tfsdk:"op"`
	Path      types.String `tfsdk:"path"`
	Value     types.String `tfsdk:"value"`
	ValueType types.String `tfsdk:"value_type"`
}

var assertionAttrTypes = map[string]attr.Type{
	"kind":       types.StringType,
	"op":         types.StringType,
	"path":       types.StringType,
	"value":      types.StringType,
	"value_type": types.StringType,
}

const (
	typeHTTP      = "http"
	typeTCP       = "tcp"
	typePing      = "ping"
	typeHeartbeat = "heartbeat"
)

func (r *monitorResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor"
}

func kept() []planmodifier.String { return []planmodifier.String{stringplanmodifier.UseStateForUnknown()} }
func keptInt() []planmodifier.Int64 { return []planmodifier.Int64{int64planmodifier.UseStateForUnknown()} }
func keptBool() []planmodifier.Bool { return []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()} }

func (r *monitorResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Monitor: an `http` URL, a `tcp` port, a `ping` host, or a `heartbeat` your own job calls.\n\n" +
			"A `ping` Monitor, and an `http` one with `assertions`, are tested once from one location before they are saved " +
			"(a Validation run). The provider runs it for you and waits for its answer.\n\n" +
			"The type and the target (`url`, `host`, `port`) never change: changing one makes a new Monitor.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The Monitor's identifier, `mon_…`.",
				Computed:            true,
				PlanModifiers:       kept(),
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The Project the Monitor belongs to, `prj_…`.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "`http`, `tcp`, `ping` or `heartbeat`.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.OneOf(typeHTTP, typeTCP, typePing, typeHeartbeat)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The Monitor's name.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 200)},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "A note on the Monitor.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},

			"url": schema.StringAttribute{
				MarkdownDescription: "`http` only: the URL to check.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"host": schema.StringAttribute{
				MarkdownDescription: "`tcp` and `ping`: the host name or address to check.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "`tcp` only: the port to connect to.",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 65535)},
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},

			"interval_sec": schema.Int64Attribute{
				MarkdownDescription: "`http`, `tcp` and `ping`: seconds between checks. Your Plan sets the floor.",
				Optional:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(30)},
			},
			"locations": schema.SetAttribute{
				MarkdownDescription: "`http`, `tcp` and `ping`: the locations that check it, by `id` from the `uup_locations` " +
					"data source. Leave it out to let You Up? choose them.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Validators: []validator.Set{
					setvalidator.SizeBetween(1, 5),
					setvalidator.ConflictsWith(path.MatchRoot("location_count")),
				},
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"location_count": schema.Int64Attribute{
				MarkdownDescription: "`http`, `tcp` and `ping`: how many locations check it, which You Up? chooses. " +
					"Set this or `locations`, not both; with neither, the Project's default count. Read when the Monitor " +
					"is made: changing it later makes a new Monitor.",
				Optional:      true,
				Validators:    []validator.Int64{int64validator.Between(1, 5)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplaceIf(changedAfterCreate, "", "")},
			},
			"timeout_ms": schema.Int64Attribute{
				MarkdownDescription: "`http`, `tcp` and `ping`: how long to wait for an answer, in milliseconds (1000 to 30000).",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.Int64{int64validator.Between(1000, 30000)},
				PlanModifiers:       keptInt(),
			},

			"method": schema.StringAttribute{
				MarkdownDescription: "`http` only: `GET`, `HEAD`, `POST`, `PUT`, `PATCH` or `DELETE`. Defaults to `GET`.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{stringvalidator.OneOf("GET", "HEAD", "POST", "PUT", "PATCH", "DELETE")},
				PlanModifiers:       kept(),
			},
			"headers": schema.MapAttribute{
				MarkdownDescription: "`http` only: headers to send, by name. You Up? never shows their values again, " +
					"so after an import they read as empty until the next apply.",
				ElementType: types.StringType,
				Optional:    true,
				Sensitive:   true,
				Validators:  []validator.Map{mapvalidator.SizeAtMost(20)},
			},
			"body": schema.StringAttribute{
				MarkdownDescription: "`http` only: the request body. Never shown again, like `headers`.",
				Optional:            true,
				Sensitive:           true,
			},
			"bearer_token": schema.StringAttribute{
				MarkdownDescription: "`http` only: a token sent as `Authorization: Bearer`. Never shown again, like `headers`.",
				Optional:            true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 4096),
					stringvalidator.ConflictsWith(path.MatchRoot("basic_auth_username"), path.MatchRoot("basic_auth_password")),
				},
			},
			"basic_auth_username": schema.StringAttribute{
				MarkdownDescription: "`http` only: the user name for HTTP Basic auth. Never shown again, like `headers`.",
				Optional:            true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 256),
					stringvalidator.AlsoRequires(path.MatchRoot("basic_auth_password")),
				},
			},
			"basic_auth_password": schema.StringAttribute{
				MarkdownDescription: "`http` only: the password for HTTP Basic auth. Never shown again, like `headers`.",
				Optional:            true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(1024),
					stringvalidator.AlsoRequires(path.MatchRoot("basic_auth_username")),
				},
			},
			"expected_status": schema.ListAttribute{
				MarkdownDescription: "`http` only: the status codes that count as up, such as `200` or `2xx`. Defaults to `[\"2xx\"]`.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Validators: []validator.List{
					listvalidator.SizeBetween(1, 10),
					listvalidator.ValueStringsAre(stringvalidator.RegexMatches(statusPattern, "must be a status code such as 200, or a class such as 2xx")),
				},
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			},
			"follow_redirects": schema.BoolAttribute{
				MarkdownDescription: "`http` only: whether redirects are followed. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keptBool(),
			},
			"assertions": schema.ListNestedAttribute{
				MarkdownDescription: "`http` only: what the answer's body must hold, up to 10. A Monitor with assertions " +
					"is tested once before it is saved.",
				Optional:   true,
				Validators: []validator.List{listvalidator.SizeBetween(1, 10)},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"kind": schema.StringAttribute{
							MarkdownDescription: "`keyword` looks for text in the body; `json` reads a value at `path`.",
							Required:            true,
							Validators:          []validator.String{stringvalidator.OneOf("keyword", "json")},
						},
						"op": schema.StringAttribute{
							MarkdownDescription: "For `keyword`: `contains` or `not_contains`. For `json`: `eq`, `ne`, `lt`, `lte`, " +
								"`gt`, `gte`, `contains`, `exists` or `not_exists`.",
							Required: true,
							Validators: []validator.String{stringvalidator.OneOf(
								"contains", "not_contains", "eq", "ne", "lt", "lte", "gt", "gte", "exists", "not_exists",
							)},
						},
						"path": schema.StringAttribute{
							MarkdownDescription: "For `json`: where the value is, such as `$.status`.",
							Optional:            true,
						},
						"value": schema.StringAttribute{
							MarkdownDescription: "The text to look for, or the value to compare with. Leave it out for `exists` and `not_exists`.",
							Optional:            true,
						},
						"value_type": schema.StringAttribute{
							MarkdownDescription: "For `json`: how `value` is read: `string` (the default), `number`, `boolean` or `null`.",
							Optional:            true,
							Validators:          []validator.String{stringvalidator.OneOf("string", "number", "boolean", "null")},
						},
					},
				},
			},

			"packet_count": schema.Int64Attribute{
				MarkdownDescription: "`ping` only: packets sent each check, 1 to 10.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.Int64{int64validator.Between(1, 10)},
				PlanModifiers:       keptInt(),
			},
			"max_packet_loss_pct": schema.Int64Attribute{
				MarkdownDescription: "`ping` only: the packet loss, in percent, above which the host counts as down (0 to 99).",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.Int64{int64validator.Between(0, 99)},
				PlanModifiers:       keptInt(),
			},

			"expected_interval_sec": schema.Int64Attribute{
				MarkdownDescription: "`heartbeat` only: how often your job calls, in seconds (60 to 2678400). Set this or `cron`.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.Between(60, 2678400),
					int64validator.ConflictsWith(path.MatchRoot("cron")),
				},
			},
			"cron": schema.StringAttribute{
				MarkdownDescription: "`heartbeat` only: your job's schedule, as a cron expression in the Project's timezone. " +
					"Set this or `expected_interval_sec`.",
				Optional: true,
			},
			"grace_sec": schema.Int64Attribute{
				MarkdownDescription: "`heartbeat` only: how late a call may be, in seconds, before it counts as missed (at least 60).",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.Int64{int64validator.AtLeast(60)},
				PlanModifiers:       keptInt(),
			},
			"heartbeat_url": schema.StringAttribute{
				MarkdownDescription: "`heartbeat` only: the URL your job calls when it succeeds. Anyone holding it can send a Heartbeat.",
				Computed:            true,
				Sensitive:           true,
				PlanModifiers:       kept(),
			},

			"performance_check_enabled": schema.BoolAttribute{
				MarkdownDescription: "`http`, `tcp` and `ping`: whether the performance Check warns when answers are slow. Defaults to `true`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keptBool(),
			},
			"performance_threshold_ms": schema.Int64Attribute{
				MarkdownDescription: "`http`, `tcp` and `ping`: the answer time, in milliseconds, above which the performance Check warns (50 to 30000).",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.Int64{int64validator.Between(50, 30000)},
				PlanModifiers:       keptInt(),
			},
			"certificate_check_enabled": schema.BoolAttribute{
				MarkdownDescription: "`http` only: whether the TLS certificate's expiry is checked. Only an `https` URL has one. " +
					"Defaults to on where it applies.",
				Optional:      true,
				Computed:      true,
				PlanModifiers: keptBool(),
			},
			"domain_check_enabled": schema.BoolAttribute{
				MarkdownDescription: "`http` only: whether the domain's registration expiry is checked. Defaults to on where it applies.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keptBool(),
			},

			"on_status_page": schema.BoolAttribute{
				MarkdownDescription: "Whether the Monitor has a row on the Project's Status page.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keptBool(),
			},
			"paused": schema.BoolAttribute{
				MarkdownDescription: "Whether the Monitor is paused. A paused Monitor is not checked and sends nothing.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       keptBool(),
			},
			"rule_ids": schema.SetAttribute{
				MarkdownDescription: "The Notification rules that hear about this Monitor, `rul_…`. A new Monitor gets the " +
					"Project's Default rule when this is left out. An empty set assigns none.",
				ElementType:   types.StringType,
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

// changedAfterCreate makes a new Monitor for a changed location_count, but
// not for one that was only unknown to the state, as after an import.
func changedAfterCreate(_ context.Context, req planmodifier.Int64Request, resp *int64planmodifier.RequiresReplaceIfFuncResponse) {
	resp.RequiresReplace = !req.StateValue.IsNull() && !req.PlanValue.Equal(req.StateValue)
}

var statusPattern = regexp.MustCompile(`^[1-5](?:[0-9]{2}|xx)$`)

// fieldsOf lists the attributes each type takes. Any other set attribute is
// refused, so a typo in the type is caught before the API is called.
var fieldsOf = map[string]map[string]bool{
	typeHTTP: set("url", "interval_sec", "locations", "location_count", "timeout_ms", "method", "headers", "body",
		"bearer_token", "basic_auth_username", "basic_auth_password", "expected_status", "follow_redirects", "assertions",
		"performance_check_enabled", "performance_threshold_ms", "certificate_check_enabled", "domain_check_enabled"),
	typeTCP: set("host", "port", "interval_sec", "locations", "location_count", "timeout_ms",
		"performance_check_enabled", "performance_threshold_ms"),
	typePing: set("host", "interval_sec", "locations", "location_count", "timeout_ms", "packet_count", "max_packet_loss_pct",
		"performance_check_enabled", "performance_threshold_ms"),
	typeHeartbeat: set("expected_interval_sec", "cron", "grace_sec"),
}

var requiredOf = map[string][]string{
	typeHTTP:      {"url", "interval_sec"},
	typeTCP:       {"host", "port", "interval_sec"},
	typePing:      {"host", "interval_sec"},
	typeHeartbeat: {},
}

func set(names ...string) map[string]bool {
	out := map[string]bool{}
	for _, name := range names {
		out[name] = true
	}
	return out
}

// typedFields are every attribute that belongs to some types and not others.
var typedFields = func() []string {
	seen := map[string]bool{}
	var out []string
	for _, fields := range fieldsOf {
		for name := range fields {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	return sorted(out)
}()

func (r *monitorResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config monitorModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || !known(config.Type) {
		return
	}
	kind := config.Type.ValueString()
	allowed, ok := fieldsOf[kind]
	if !ok {
		return
	}
	values := attributesOf(config)

	for _, name := range typedFields {
		if !allowed[name] && !values[name].IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Not for this type",
				fmt.Sprintf("A %s Monitor does not take %s.", kind, name))
		}
	}

	for _, name := range requiredOf[kind] {
		if values[name].IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Missing attribute",
				fmt.Sprintf("A %s Monitor needs %s.", kind, name))
		}
	}

	if kind == typeHeartbeat {
		if config.ExpectedIntervalSec.IsNull() && config.Cron.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("expected_interval_sec"), "Missing attribute",
				"A heartbeat Monitor needs expected_interval_sec or cron.")
		}
	}

	if kind == typeHTTP {
		for i, one := range assertionsOf(ctx, config.Assertions, &resp.Diagnostics) {
			at := path.Root("assertions").AtListIndex(i)
			k, op := one.Kind.ValueString(), one.Op.ValueString()
			if !known(one.Kind) || !known(one.Op) {
				continue
			}
			switch k {
			case "keyword":
				if op != "contains" && op != "not_contains" {
					resp.Diagnostics.AddAttributeError(at.AtName("op"), "Not a keyword op", "A keyword assertion takes contains or not_contains.")
				}
				if one.Value.IsNull() {
					resp.Diagnostics.AddAttributeError(at.AtName("value"), "Missing attribute", "A keyword assertion needs value.")
				}
				if !one.Path.IsNull() || !one.ValueType.IsNull() {
					resp.Diagnostics.AddAttributeError(at, "Not for a keyword", "A keyword assertion takes no path or value_type.")
				}
			case "json":
				if op == "not_contains" {
					resp.Diagnostics.AddAttributeError(at.AtName("op"), "Not a json op", "A json assertion does not take not_contains.")
				}
				if one.Path.IsNull() {
					resp.Diagnostics.AddAttributeError(at.AtName("path"), "Missing attribute", "A json assertion needs path.")
				}
			}
		}
	}
}

func (r *monitorResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureResource(req, resp)
}

func (r *monitorResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importInto(ctx, req.ID, "monitor", resp)
}

// ---- Create, Read, Update, Delete ----

func (r *monitorResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := plan.ProjectID.ValueString()
	kind := plan.Type.ValueString()

	body := r.createBody(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if needsValidation(kind, body.Assertions) {
		run := r.validate(ctx, projectID, plan, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		body.ValidationID = &run.ID
	}

	made, err := r.client.CreateMonitor(ctx, projectID, body)
	if err != nil {
		apiError(&resp.Diagnostics, "Could not create the Monitor", err)
		return
	}
	plan.ID = types.StringValue(made.ID)
	// Save the Monitor now, so a failure below leaves it in the state to be
	// fixed or destroyed, not made a second time.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), made.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), kind)...)

	if known(plan.RuleIDs) {
		ruleIDs := sorted(stringsOf(ctx, plan.RuleIDs, &resp.Diagnostics))
		if err := r.client.AssignRules(ctx, projectID, made.ID, ruleIDs); err != nil {
			apiError(&resp.Diagnostics, "Made the Monitor, but could not assign its rules", err)
			return
		}
	}
	if plan.Paused.ValueBool() {
		if err := r.client.PauseMonitor(ctx, projectID, made.ID); err != nil {
			apiError(&resp.Diagnostics, "Made the Monitor, but could not pause it", err)
			return
		}
	}

	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, err := r.client.GetMonitor(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "Could not read the Monitor", err)
		return
	}

	fillMonitor(ctx, &state, found, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *monitorResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state monitorModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := state.ProjectID.ValueString()
	monitorID := state.ID.ValueString()
	plan.ID = state.ID

	body, settingsChanged := r.editBody(ctx, plan, state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if settingsChanged && needsValidation(plan.Type.ValueString(), r.assertionsBody(ctx, plan, &resp.Diagnostics)) {
		run := r.validate(ctx, projectID, plan, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		body.ValidationID = &run.ID
	}

	if !reflect.DeepEqual(body, client.MonitorBody{}) {
		if _, err := r.client.EditMonitor(ctx, projectID, monitorID, body); err != nil {
			apiError(&resp.Diagnostics, "Could not change the Monitor", err)
			return
		}
	}

	if known(plan.RuleIDs) && !plan.RuleIDs.Equal(state.RuleIDs) {
		ruleIDs := sorted(stringsOf(ctx, plan.RuleIDs, &resp.Diagnostics))
		if err := r.client.AssignRules(ctx, projectID, monitorID, ruleIDs); err != nil {
			apiError(&resp.Diagnostics, "Could not assign the Monitor's rules", err)
			return
		}
	}

	if known(plan.Paused) && !plan.Paused.Equal(state.Paused) {
		var err error
		if plan.Paused.ValueBool() {
			err = r.client.PauseMonitor(ctx, projectID, monitorID)
		} else {
			err = r.client.ResumeMonitor(ctx, projectID, monitorID)
		}
		if err != nil {
			apiError(&resp.Diagnostics, "Could not pause or resume the Monitor", err)
			return
		}
	}

	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *monitorResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteMonitor(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		apiError(&resp.Diagnostics, "Could not delete the Monitor", err)
	}
}

// readInto reads the Monitor back over model, keeping the secrets the API
// never shows.
func (r *monitorResource) readInto(ctx context.Context, model *monitorModel, diags *diag.Diagnostics) {
	found, err := r.client.GetMonitor(ctx, model.ProjectID.ValueString(), model.ID.ValueString())
	if err != nil {
		apiError(diags, "Could not read the Monitor back", err)
		return
	}
	fillMonitor(ctx, model, found, diags)
}

// ---- Validation ----

// needsValidation says whether a create of these settings needs a
// Validation run: a ping Monitor always, an http one with assertions.
func needsValidation(kind string, assertions *[]client.Assertion) bool {
	if kind == typePing {
		return true
	}
	return kind == typeHTTP && assertions != nil && len(*assertions) > 0
}

// validate runs a Validation of the planned settings and waits for it.
func (r *monitorResource) validate(ctx context.Context, projectID string, plan monitorModel, diags *diag.Diagnostics) *client.Validation {
	kind := plan.Type.ValueString()
	body := client.ValidationBody{
		Type:        kind,
		IntervalSec: plan.IntervalSec.ValueInt64(),
		Locations:   sorted(stringsOf(ctx, plan.Locations, diags)),
	}
	if len(body.Locations) == 0 {
		body.LocationCount = int64Ptr(plan.LocationCount)
	}
	switch kind {
	case typeHTTP:
		body.URL = stringPtr(plan.URL)
		body.Request = r.requestBody(ctx, plan, diags)
		body.Assertions = r.assertionsBody(ctx, plan, diags)
	case typePing:
		body.Host = stringPtr(plan.Host)
		body.PacketCount = int64Ptr(plan.PacketCount)
		body.MaxPacketLossPct = int64Ptr(plan.MaxPacketLossPct)
		body.TimeoutMs = int64Ptr(plan.TimeoutMs)
	}
	if diags.HasError() {
		return nil
	}

	run, err := r.client.Validate(ctx, projectID, body, ValidationPoll)
	if err != nil {
		apiError(diags, "Could not test the Monitor's settings", err)
		return nil
	}

	if run.State != "ok" {
		detail := fmt.Sprintf("The Validation run from %s answered %q", run.Location, run.State)
		if run.ErrorKind != nil {
			detail += fmt.Sprintf(" (%s)", *run.ErrorKind)
		}
		if run.StatusCode != nil {
			detail += fmt.Sprintf(", HTTP %d", *run.StatusCode)
		}
		diags.AddWarning("The target did not pass its test", detail+
			". An http Monitor is saved anyway, so it can watch a target that is down right now. "+
			"A ping Monitor needs at least one reply.")
	}
	return run
}

// ---- Bodies ----

func (r *monitorResource) createBody(ctx context.Context, plan monitorModel, diags *diag.Diagnostics) client.MonitorBody {
	kind := plan.Type.ValueString()
	body := client.MonitorBody{
		Type:         &kind,
		Name:         stringPtr(plan.Name),
		OnStatusPage: boolPtr(plan.OnStatusPage),
	}
	if known(plan.Description) {
		body.Description = &client.NullString{Value: plan.Description.ValueString()}
	}

	switch kind {
	case typeHeartbeat:
		body.ExpectedIntervalSec = int64Ptr(plan.ExpectedIntervalSec)
		body.Cron = stringPtr(plan.Cron)
		body.GraceSec = int64Ptr(plan.GraceSec)
		return body
	case typeHTTP:
		body.URL = stringPtr(plan.URL)
		body.Request = r.requestBody(ctx, plan, diags)
		body.Assertions = r.assertionsBody(ctx, plan, diags)
	case typeTCP:
		body.Host = stringPtr(plan.Host)
		body.Port = int64Ptr(plan.Port)
		body.TimeoutMs = int64Ptr(plan.TimeoutMs)
	case typePing:
		body.Host = stringPtr(plan.Host)
		body.PacketCount = int64Ptr(plan.PacketCount)
		body.MaxPacketLossPct = int64Ptr(plan.MaxPacketLossPct)
		body.TimeoutMs = int64Ptr(plan.TimeoutMs)
	}

	body.IntervalSec = int64Ptr(plan.IntervalSec)
	if locations := sorted(stringsOf(ctx, plan.Locations, diags)); len(locations) > 0 {
		body.Locations = locations
	} else {
		body.LocationCount = int64Ptr(plan.LocationCount)
	}
	body.Checks = switchesOf(plan, kind, true)
	return body
}

// switchesOf turns the Checks beside uptime as the plan asks. On a create,
// a performance switch must say whether it is on, so a threshold alone
// keeps it on.
func switchesOf(plan monitorModel, kind string, creating bool) *client.Switches {
	var out client.Switches
	if known(plan.PerformanceCheckEnabled) || known(plan.PerformanceThresholdMs) {
		on := boolPtr(plan.PerformanceCheckEnabled)
		if on == nil && creating {
			on = ptr(true)
		}
		out.Performance = &client.Switch{Enabled: on, ThresholdMs: int64Ptr(plan.PerformanceThresholdMs)}
	}
	if kind == typeHTTP {
		if known(plan.CertificateCheckEnabled) {
			out.Certificate = &client.Switch{Enabled: boolPtr(plan.CertificateCheckEnabled)}
		}
		if known(plan.DomainCheckEnabled) {
			out.Domain = &client.Switch{Enabled: boolPtr(plan.DomainCheckEnabled)}
		}
	}
	if out == (client.Switches{}) {
		return nil
	}
	return &out
}

// requestBody is the whole request an http Monitor sends. Its secrets come
// from the plan, since the API never shows them.
func (r *monitorResource) requestBody(ctx context.Context, plan monitorModel, diags *diag.Diagnostics) *client.Request {
	request := client.Request{
		Method:          stringPtr(plan.Method),
		Headers:         []client.Header{},
		Body:            stringPtr(plan.Body),
		FollowRedirects: boolPtr(plan.FollowRedirects),
		TimeoutMs:       int64Ptr(plan.TimeoutMs),
	}
	if known(plan.Headers) {
		values := map[string]string{}
		diags.Append(plan.Headers.ElementsAs(ctx, &values, false)...)
		names := make([]string, 0, len(values))
		for name := range values {
			names = append(names, name)
		}
		for _, name := range sorted(names) {
			request.Headers = append(request.Headers, client.Header{Name: name, Value: values[name]})
		}
	}
	if known(plan.ExpectedStatus) {
		request.ExpectedStatus = stringsOf(ctx, plan.ExpectedStatus, diags)
	}
	switch {
	case known(plan.BearerToken):
		request.Auth = &client.Auth{Kind: "bearer", Token: stringPtr(plan.BearerToken)}
	case known(plan.BasicAuthUsername):
		request.Auth = &client.Auth{
			Kind:     "basic",
			Username: stringPtr(plan.BasicAuthUsername),
			Password: ptr(plan.BasicAuthPassword.ValueString()),
		}
	}
	return &request
}

func (r *monitorResource) assertionsBody(ctx context.Context, plan monitorModel, diags *diag.Diagnostics) *[]client.Assertion {
	out := []client.Assertion{}
	for i, one := range assertionsOf(ctx, plan.Assertions, diags) {
		assertion := client.Assertion{Kind: one.Kind.ValueString(), Op: one.Op.ValueString(), Path: stringPtr(one.Path)}
		raw, err := assertionValue(one)
		if err != nil {
			diags.AddAttributeError(path.Root("assertions").AtListIndex(i).AtName("value"), "Unreadable value", err.Error())
			continue
		}
		assertion.Value = raw
		out = append(out, assertion)
	}
	return &out
}

// assertionValue writes an assertion's value as the JSON value its
// value_type names.
func assertionValue(one assertionModel) (*json.RawMessage, error) {
	valueType := one.ValueType.ValueString()
	if valueType == "null" {
		raw := json.RawMessage("null")
		return &raw, nil
	}
	if one.Value.IsNull() {
		return nil, nil
	}
	text := one.Value.ValueString()
	var encoded []byte
	var err error
	switch valueType {
	case "number":
		var number float64
		number, err = strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", text)
		}
		encoded, err = json.Marshal(number)
	case "boolean":
		var flag bool
		flag, err = strconv.ParseBool(text)
		if err != nil {
			return nil, fmt.Errorf("%q is not true or false", text)
		}
		encoded, err = json.Marshal(flag)
	default:
		encoded, err = json.Marshal(text)
	}
	if err != nil {
		return nil, err
	}
	raw := json.RawMessage(encoded)
	return &raw, nil
}

// editBody is the edit of what changed between state and plan, and whether
// the uptime settings a Validation run tests changed.
func (r *monitorResource) editBody(ctx context.Context, plan, state monitorModel, diags *diag.Diagnostics) (client.MonitorBody, bool) {
	var body client.MonitorBody
	kind := plan.Type.ValueString()
	changed := func(a, b attr.Value) bool { return known(a) && !a.Equal(b) }

	if changed(plan.Name, state.Name) {
		body.Name = stringPtr(plan.Name)
	}
	if !plan.Description.Equal(state.Description) {
		body.Description = &client.NullString{Value: plan.Description.ValueString(), Null: plan.Description.IsNull()}
	}
	if changed(plan.OnStatusPage, state.OnStatusPage) {
		body.OnStatusPage = boolPtr(plan.OnStatusPage)
	}

	if kind == typeHeartbeat {
		if !plan.ExpectedIntervalSec.Equal(state.ExpectedIntervalSec) || !plan.Cron.Equal(state.Cron) {
			body.ExpectedIntervalSec = int64Ptr(plan.ExpectedIntervalSec)
			body.Cron = stringPtr(plan.Cron)
		}
		if changed(plan.GraceSec, state.GraceSec) {
			body.GraceSec = int64Ptr(plan.GraceSec)
		}
		return body, false
	}

	if changed(plan.IntervalSec, state.IntervalSec) {
		body.IntervalSec = int64Ptr(plan.IntervalSec)
	}
	if changed(plan.Locations, state.Locations) {
		body.Locations = sorted(stringsOf(ctx, plan.Locations, diags))
	}

	settingsChanged := false
	switch kind {
	case typeHTTP:
		requestChanged := changed(plan.Method, state.Method) ||
			!plan.Headers.Equal(state.Headers) ||
			!plan.Body.Equal(state.Body) ||
			!plan.BearerToken.Equal(state.BearerToken) ||
			!plan.BasicAuthUsername.Equal(state.BasicAuthUsername) ||
			!plan.BasicAuthPassword.Equal(state.BasicAuthPassword) ||
			changed(plan.ExpectedStatus, state.ExpectedStatus) ||
			changed(plan.FollowRedirects, state.FollowRedirects) ||
			changed(plan.TimeoutMs, state.TimeoutMs)
		assertionsChanged := !plan.Assertions.Equal(state.Assertions)
		if requestChanged || assertionsChanged {
			body.Request = r.requestBody(ctx, plan, diags)
			body.Assertions = r.assertionsBody(ctx, plan, diags)
			settingsChanged = true
		}
	case typeTCP:
		if changed(plan.TimeoutMs, state.TimeoutMs) {
			body.TimeoutMs = int64Ptr(plan.TimeoutMs)
		}
	case typePing:
		if changed(plan.PacketCount, state.PacketCount) ||
			changed(plan.MaxPacketLossPct, state.MaxPacketLossPct) ||
			changed(plan.TimeoutMs, state.TimeoutMs) {
			body.PacketCount = int64Ptr(plan.PacketCount)
			body.MaxPacketLossPct = int64Ptr(plan.MaxPacketLossPct)
			body.TimeoutMs = int64Ptr(plan.TimeoutMs)
			settingsChanged = true
		}
	}

	var switches client.Switches
	if changed(plan.PerformanceCheckEnabled, state.PerformanceCheckEnabled) ||
		changed(plan.PerformanceThresholdMs, state.PerformanceThresholdMs) {
		switches.Performance = &client.Switch{
			Enabled:     boolPtr(plan.PerformanceCheckEnabled),
			ThresholdMs: int64Ptr(plan.PerformanceThresholdMs),
		}
	}
	if kind == typeHTTP {
		if changed(plan.CertificateCheckEnabled, state.CertificateCheckEnabled) {
			switches.Certificate = &client.Switch{Enabled: boolPtr(plan.CertificateCheckEnabled)}
		}
		if changed(plan.DomainCheckEnabled, state.DomainCheckEnabled) {
			switches.Domain = &client.Switch{Enabled: boolPtr(plan.DomainCheckEnabled)}
		}
	}
	if switches != (client.Switches{}) {
		body.Checks = &switches
	}

	return body, settingsChanged
}

// ---- Reading back ----

// fillMonitor writes what the API says of the Monitor over model. The
// target never changes, so the target model holds is kept as written; the
// secrets the API never shows are kept from model, or read as empty.
func fillMonitor(ctx context.Context, model *monitorModel, m *client.Monitor, diags *diag.Diagnostics) {
	model.ID = types.StringValue(m.ID)
	model.ProjectID = types.StringValue(m.ProjectID)
	model.Type = types.StringValue(m.Type)
	model.Name = types.StringValue(m.Name)
	model.Description = types.StringPointerValue(m.Description)
	model.OnStatusPage = types.BoolValue(m.OnStatusPage)
	model.Paused = types.BoolValue(m.State == "paused")
	model.RuleIDs = setOf(m.RuleIDs)

	uptime := m.CheckOf("uptime")
	settings, err := uptime.SettingsOf()
	if err != nil {
		diags.AddError("Unreadable Monitor", err.Error())
		return
	}
	if settings == nil {
		settings = &client.Settings{}
	}

	// Everything typed starts empty, and the Monitor's own type fills its part.
	nullString, nullInt, nullBool := types.StringNull(), types.Int64Null(), types.BoolNull()
	model.Locations = types.SetNull(types.StringType)
	model.IntervalSec = nullInt
	model.TimeoutMs = nullInt
	model.Method = nullString
	model.ExpectedStatus = types.ListNull(types.StringType)
	model.FollowRedirects = nullBool
	model.PacketCount = nullInt
	model.MaxPacketLossPct = nullInt
	model.ExpectedIntervalSec = nullInt
	model.Cron = nullString
	model.GraceSec = nullInt
	model.HeartbeatURL = nullString
	model.PerformanceCheckEnabled = nullBool
	model.PerformanceThresholdMs = nullInt
	model.CertificateCheckEnabled = nullBool
	model.DomainCheckEnabled = nullBool
	if m.Type != typeHTTP {
		model.URL = nullString
		model.Headers = types.MapNull(types.StringType)
		model.Body = nullString
		model.BearerToken = nullString
		model.BasicAuthUsername = nullString
		model.BasicAuthPassword = nullString
		model.Assertions = types.ListNull(types.ObjectType{AttrTypes: assertionAttrTypes})
	}
	if m.Type != typeTCP && m.Type != typePing {
		model.Host = nullString
	}
	if m.Type != typeTCP {
		model.Port = nullInt
	}

	if m.Type == typeHeartbeat {
		model.ExpectedIntervalSec = types.Int64PointerValue(settings.ExpectedIntervalSec)
		model.Cron = types.StringPointerValue(settings.Cron)
		model.GraceSec = types.Int64PointerValue(settings.GraceSec)
		model.HeartbeatURL = types.StringPointerValue(settings.URL)
		return
	}

	model.Locations = setOf(m.Locations)
	if uptime != nil {
		model.IntervalSec = types.Int64Value(uptime.IntervalSec)
	}
	model.TimeoutMs = types.Int64PointerValue(settings.TimeoutMs)

	switch m.Type {
	case typeHTTP:
		if !known(model.URL) {
			model.URL = types.StringValue(m.Target)
		}
		model.Method = types.StringValue(settings.Method)
		model.ExpectedStatus = listOf(settings.ExpectedStatus)
		model.FollowRedirects = types.BoolPointerValue(settings.FollowRedirects)
		fillSecrets(ctx, model, settings, diags)
		model.Assertions = assertionsFrom(model.Assertions, settings.Assertions, diags)
	case typeTCP:
		if !known(model.Host) {
			model.Host = types.StringValue(settings.Host)
		}
		model.Port = types.Int64PointerValue(settings.Port)
	case typePing:
		if !known(model.Host) {
			model.Host = types.StringValue(settings.Host)
		}
		model.PacketCount = types.Int64PointerValue(settings.PacketCount)
		model.MaxPacketLossPct = types.Int64PointerValue(settings.MaxPacketLossPct)
	}

	if check := m.CheckOf("performance"); check != nil {
		model.PerformanceCheckEnabled = types.BoolValue(check.Enabled)
		if performance, err := check.SettingsOf(); err == nil && performance != nil {
			model.PerformanceThresholdMs = types.Int64PointerValue(performance.ThresholdMs)
		}
	}
	if m.Type == typeHTTP {
		if check := m.CheckOf("certificate"); check != nil {
			model.CertificateCheckEnabled = types.BoolValue(check.Enabled)
		} else {
			model.CertificateCheckEnabled = types.BoolValue(false)
		}
		if check := m.CheckOf("domain"); check != nil {
			model.DomainCheckEnabled = types.BoolValue(check.Enabled)
		} else {
			model.DomainCheckEnabled = types.BoolValue(false)
		}
	}
}

// fillSecrets keeps the header values, body and credentials model holds
// where the API says they are still set, and reads them as empty where
// model has none, as after an import.
func fillSecrets(ctx context.Context, model *monitorModel, settings *client.Settings, diags *diag.Diagnostics) {
	held := map[string]string{}
	if known(model.Headers) {
		diags.Append(model.Headers.ElementsAs(ctx, &held, false)...)
	}
	if len(settings.Headers) == 0 {
		if !(known(model.Headers) && len(held) == 0) {
			model.Headers = types.MapNull(types.StringType)
		}
	} else {
		values := map[string]attr.Value{}
		for _, header := range settings.Headers {
			values[header.Name] = types.StringValue(held[header.Name])
		}
		model.Headers = types.MapValueMust(types.StringType, values)
	}

	if len(settings.Body) == 0 || string(settings.Body) == "null" {
		model.Body = types.StringNull()
	} else if !known(model.Body) {
		model.Body = types.StringValue("")
	}

	kind := ""
	if settings.Auth != nil {
		kind = settings.Auth.Kind
	}
	keepOrEmpty := func(value types.String) types.String {
		if known(value) {
			return value
		}
		return types.StringValue("")
	}
	switch kind {
	case "bearer":
		model.BearerToken = keepOrEmpty(model.BearerToken)
		model.BasicAuthUsername = types.StringNull()
		model.BasicAuthPassword = types.StringNull()
	case "basic":
		model.BearerToken = types.StringNull()
		model.BasicAuthUsername = keepOrEmpty(model.BasicAuthUsername)
		model.BasicAuthPassword = keepOrEmpty(model.BasicAuthPassword)
	default:
		model.BearerToken = types.StringNull()
		model.BasicAuthUsername = types.StringNull()
		model.BasicAuthPassword = types.StringNull()
	}
}

func assertionsOf(ctx context.Context, list types.List, diags *diag.Diagnostics) []assertionModel {
	if !known(list) {
		return nil
	}
	var out []assertionModel
	diags.Append(list.ElementsAs(ctx, &out, false)...)
	return out
}

// assertionsFrom reads the API's assertions, as the model writes them. A
// value_type is named only where `string` would not read it the same way.
func assertionsFrom(held types.List, assertions []client.Assertion, diags *diag.Diagnostics) types.List {
	elementType := types.ObjectType{AttrTypes: assertionAttrTypes}
	if len(assertions) == 0 {
		if known(held) && len(held.Elements()) == 0 {
			return held
		}
		return types.ListNull(elementType)
	}

	heldTypes := map[int]types.String{}
	if known(held) {
		for i, element := range held.Elements() {
			if object, ok := element.(basetypes.ObjectValue); ok {
				if valueType, ok := object.Attributes()["value_type"].(types.String); ok {
					heldTypes[i] = valueType
				}
			}
		}
	}

	values := make([]attr.Value, 0, len(assertions))
	for i, one := range assertions {
		value, valueType := types.StringNull(), types.StringNull()
		if one.Value != nil {
			var decoded any
			if err := json.Unmarshal(*one.Value, &decoded); err != nil {
				diags.AddError("Unreadable assertion", err.Error())
				return types.ListNull(elementType)
			}
			switch typed := decoded.(type) {
			case nil:
				if one.Op != "exists" && one.Op != "not_exists" {
					valueType = types.StringValue("null")
				}
			case string:
				value = types.StringValue(typed)
				if held, ok := heldTypes[i]; ok && held.ValueString() == "string" {
					valueType = held
				}
			case bool:
				value = types.StringValue(strconv.FormatBool(typed))
				valueType = types.StringValue("boolean")
			case float64:
				value = types.StringValue(strconv.FormatFloat(typed, 'f', -1, 64))
				valueType = types.StringValue("number")
			}
		}
		object, d := types.ObjectValue(assertionAttrTypes, map[string]attr.Value{
			"kind":       types.StringValue(one.Kind),
			"op":         types.StringValue(one.Op),
			"path":       types.StringPointerValue(one.Path),
			"value":      value,
			"value_type": valueType,
		})
		diags.Append(d...)
		values = append(values, object)
	}
	list, d := types.ListValue(elementType, values)
	diags.Append(d...)
	return list
}

// attributesOf names the model's values by their attribute names.
func attributesOf(m monitorModel) map[string]attr.Value {
	out := map[string]attr.Value{}
	v := reflect.ValueOf(m)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		if value, ok := v.Field(i).Interface().(attr.Value); ok {
			out[t.Field(i).Tag.Get("tfsdk")] = value
		}
	}
	return out
}
