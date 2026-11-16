package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uupdev/terraform-provider-uup/internal/client"
)

// EventTypes are the events a Notification rule can send.
var EventTypes = []string{
	"uptime.failed", "uptime.recovered",
	"certificate.failed", "certificate.recovered",
	"domain.failed", "domain.recovered",
	"certificate.expiring", "domain.expiring", "domain.unreadable",
	"performance.degraded",
	"uptime.family_failing", "uptime.regional_outage",
}

// Repeats are how often an open Incident is sent again.
var Repeats = []string{"off", "30m", "1h", "3h", "6h"}

var (
	_ resource.Resource                = &ruleResource{}
	_ resource.ResourceWithConfigure   = &ruleResource{}
	_ resource.ResourceWithImportState = &ruleResource{}
)

// NewNotificationRuleResource is uup_notification_rule.
func NewNotificationRuleResource() resource.Resource {
	return &ruleResource{}
}

type ruleResource struct {
	client *client.Client
}

type ruleModel struct {
	ID                   types.String `tfsdk:"id"`
	ProjectID            types.String `tfsdk:"project_id"`
	Name                 types.String `tfsdk:"name"`
	EventTypes           types.Set    `tfsdk:"event_types"`
	ChannelIDs           types.Set    `tfsdk:"channel_ids"`
	WarningsDuringOutage types.Bool   `tfsdk:"warnings_during_outage"`
	Repeat               types.String `tfsdk:"repeat"`
	IsDefault            types.Bool   `tfsdk:"is_default"`
}

func (r *ruleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_rule"
}

func (r *ruleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Notification rule: which events go to which Channels. " +
			"A rule reaches no Monitor until it is assigned to one, with `rule_ids` on `uup_monitor`.\n\n" +
			"Channels are added and verified in the dashboard. Look one up with the `uup_channel` data source.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The rule's identifier, `rul_…`.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The Project the rule belongs to, `prj_…`. Changing it makes a new rule.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The rule's name, unique in the Project.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 100)},
			},
			"event_types": schema.SetAttribute{
				MarkdownDescription: "The events it sends. Leave it out to send every event, those added later included. " +
					"One of: `" + joinTicks(EventTypes) + "`.",
				ElementType: types.StringType,
				Optional:    true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.OneOf(EventTypes...)),
				},
			},
			"channel_ids": schema.SetAttribute{
				MarkdownDescription: "The Channels it sends to, `chn_…`.",
				ElementType:         types.StringType,
				Required:            true,
				Validators:          []validator.Set{setvalidator.SizeBetween(1, 100)},
			},
			"warnings_during_outage": schema.BoolAttribute{
				MarkdownDescription: "Whether warnings are sent while an outage is open. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"repeat": schema.StringAttribute{
				MarkdownDescription: "How often an open Incident is sent again: `off`, `30m`, `1h`, `3h` or `6h`. Defaults to `1h`.",
				Optional:            true,
				Computed:            true,
				Validators:          []validator.String{stringvalidator.OneOf(Repeats...)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"is_default": schema.BoolAttribute{
				MarkdownDescription: "Whether this is the Project's Default rule. The Default rule is made with the Project " +
					"and cannot be deleted, so it can be imported and changed but not created.",
				Computed:      true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *ruleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureResource(req, resp)
}

func ruleBodyOf(ctx context.Context, plan ruleModel, diags *diag.Diagnostics) client.RuleBody {
	body := client.RuleBody{
		Name:                 plan.Name.ValueString(),
		ChannelIDs:           sorted(stringsOf(ctx, plan.ChannelIDs, diags)),
		WarningsDuringOutage: boolPtr(plan.WarningsDuringOutage),
		Repeat:               stringPtr(plan.Repeat),
	}
	if known(plan.EventTypes) {
		events := sorted(stringsOf(ctx, plan.EventTypes, diags))
		body.EventTypes = &events
	}
	return body
}

func (r *ruleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ruleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := ruleBodyOf(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	made, err := r.client.CreateRule(ctx, plan.ProjectID.ValueString(), body)
	if err != nil {
		apiError(&resp.Diagnostics, "Could not create the Notification rule", err)
		return
	}

	fillRule(&plan, made)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ruleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ruleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	found, err := r.client.GetRule(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiError(&resp.Diagnostics, "Could not read the Notification rule", err)
		return
	}

	fillRule(&state, found)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ruleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ruleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := ruleBodyOf(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	changed, err := r.client.EditRule(ctx, state.ProjectID.ValueString(), state.ID.ValueString(), body)
	if err != nil {
		apiError(&resp.Diagnostics, "Could not change the Notification rule", err)
		return
	}

	plan.ID = state.ID
	fillRule(&plan, changed)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ruleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ruleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.IsDefault.ValueBool() {
		resp.Diagnostics.AddWarning(
			"The Default rule stays",
			"The Default rule cannot be deleted, so it was only removed from the Terraform state. It still exists in You Up?.",
		)
		return
	}

	err := r.client.DeleteRule(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		apiError(&resp.Diagnostics, "Could not delete the Notification rule", err)
	}
}

func (r *ruleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importInto(ctx, req.ID, "rule", resp)
}

func fillRule(model *ruleModel, rule *client.Rule) {
	model.ID = types.StringValue(rule.ID)
	model.Name = types.StringValue(rule.Name)
	if rule.EventTypes == nil {
		model.EventTypes = types.SetNull(types.StringType)
	} else {
		model.EventTypes = setOf(*rule.EventTypes)
	}
	model.ChannelIDs = setOf(rule.ChannelIDs)
	model.WarningsDuringOutage = types.BoolValue(rule.WarningsDuringOutage)
	model.Repeat = types.StringValue(rule.Repeat)
	model.IsDefault = types.BoolValue(rule.IsDefault)
}
