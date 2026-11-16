package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uupdev/terraform-provider-uup/internal/client"
)

// ---- uup_project ----

// NewProjectDataSource is uup_project.
func NewProjectDataSource() datasource.DataSource {
	return &projectDataSource{}
}

type projectDataSource struct {
	client *client.Client
}

type projectModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Timezone             types.String `tfsdk:"timezone"`
	DefaultLocationCount types.Int64  `tfsdk:"default_location_count"`
}

func (d *projectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (d *projectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Project by its `id` or its `name`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The Project's identifier, `prj_…`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The Project's name.",
				Optional:            true,
				Computed:            true,
			},
			"timezone": schema.StringAttribute{
				MarkdownDescription: "The IANA timezone its days are cut by.",
				Computed:            true,
			},
			"default_location_count": schema.Int64Attribute{
				MarkdownDescription: "How many locations a new Monitor is checked from when it names none.",
				Computed:            true,
			},
		},
	}
}

func (d *projectDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *projectDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureDataSource(req, resp)
}

func (d *projectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model projectModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var found *client.Project
	if known(model.ID) {
		project, err := d.client.GetProject(ctx, model.ID.ValueString())
		if err != nil {
			apiError(&resp.Diagnostics, "Could not read the Project", err)
			return
		}
		found = project
	} else {
		projects, err := d.client.ListProjects(ctx)
		if err != nil {
			apiError(&resp.Diagnostics, "Could not list Projects", err)
			return
		}
		for i := range projects {
			if projects[i].Name == model.Name.ValueString() {
				found = &projects[i]
				break
			}
		}
		if found == nil {
			resp.Diagnostics.AddError("No such Project", fmt.Sprintf("This token reaches no Project named %q.", model.Name.ValueString()))
			return
		}
	}

	model.ID = types.StringValue(found.ID)
	model.Name = types.StringValue(found.Name)
	model.Timezone = types.StringValue(found.Timezone)
	model.DefaultLocationCount = types.Int64Value(found.DefaultLocationCount)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// ---- uup_channel ----

// NewChannelDataSource is uup_channel.
func NewChannelDataSource() datasource.DataSource {
	return &channelDataSource{}
}

type channelDataSource struct {
	client *client.Client
}

type channelModel struct {
	ProjectID types.String `tfsdk:"project_id"`
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	State     types.String `tfsdk:"state"`
}

func (d *channelDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_channel"
}

func (d *channelDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Channel by its `id` or its `name`. Channels are added and verified in the dashboard, " +
			"because most need a code or a link confirmed by a person.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The Project the Channel belongs to, `prj_…`.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The Channel's identifier, `chn_…`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The Channel's name.",
				Optional:            true,
				Computed:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Its type, such as `email`, `slack`, `discord`, `telegram`, `msteams`, `inbox`, `sms` or `whatsapp`.",
				Computed:            true,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: "`active`, `awaiting_link`, `awaiting_code` or `disabled`.",
				Computed:            true,
			},
		},
	}
}

func (d *channelDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
	}
}

func (d *channelDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureDataSource(req, resp)
}

func (d *channelDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model channelModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	channels, err := d.client.ListChannels(ctx, model.ProjectID.ValueString())
	if err != nil {
		apiError(&resp.Diagnostics, "Could not list Channels", err)
		return
	}

	var found *client.Channel
	for i := range channels {
		if (known(model.ID) && channels[i].ID == model.ID.ValueString()) ||
			(known(model.Name) && channels[i].Name == model.Name.ValueString()) {
			found = &channels[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("No such Channel", "The Project has no Channel with this id or name.")
		return
	}

	model.ID = types.StringValue(found.ID)
	model.Name = types.StringValue(found.Name)
	model.Type = types.StringValue(found.Type)
	model.State = types.StringValue(found.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// ---- uup_notification_rule ----

// NewNotificationRuleDataSource is the uup_notification_rule data source.
func NewNotificationRuleDataSource() datasource.DataSource {
	return &ruleDataSource{}
}

type ruleDataSource struct {
	client *client.Client
}

type ruleLookupModel struct {
	ProjectID            types.String `tfsdk:"project_id"`
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Default              types.Bool   `tfsdk:"default"`
	EventTypes           types.Set    `tfsdk:"event_types"`
	ChannelIDs           types.Set    `tfsdk:"channel_ids"`
	WarningsDuringOutage types.Bool   `tfsdk:"warnings_during_outage"`
	Repeat               types.String `tfsdk:"repeat"`
}

func (d *ruleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_rule"
}

func (d *ruleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Notification rule by its `id` or its `name`, or the Project's Default rule with `default = true`.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				MarkdownDescription: "The Project the rule belongs to, `prj_…`.",
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The rule's identifier, `rul_…`.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The rule's name.",
				Optional:            true,
				Computed:            true,
			},
			"default": schema.BoolAttribute{
				MarkdownDescription: "Set to `true` to find the Project's Default rule.",
				Optional:            true,
				Computed:            true,
			},
			"event_types": schema.SetAttribute{
				MarkdownDescription: "The events it sends, or null for every event.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"channel_ids": schema.SetAttribute{
				MarkdownDescription: "Its Channels, `chn_…`.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"warnings_during_outage": schema.BoolAttribute{
				MarkdownDescription: "Whether warnings are sent while an outage is open.",
				Computed:            true,
			},
			"repeat": schema.StringAttribute{
				MarkdownDescription: "How often an open Incident is sent again, or `off`.",
				Computed:            true,
			},
		},
	}
}

func (d *ruleDataSource) ConfigValidators(_ context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{
		datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"), path.MatchRoot("default")),
	}
}

func (d *ruleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureDataSource(req, resp)
}

func (d *ruleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model ruleLookupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rules, err := d.client.ListRules(ctx, model.ProjectID.ValueString())
	if err != nil {
		apiError(&resp.Diagnostics, "Could not list Notification rules", err)
		return
	}

	var found *client.Rule
	for i := range rules {
		if (known(model.ID) && rules[i].ID == model.ID.ValueString()) ||
			(known(model.Name) && rules[i].Name == model.Name.ValueString()) ||
			(model.Default.ValueBool() && rules[i].IsDefault) {
			found = &rules[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("No such Notification rule", "The Project has no rule with this id or name.")
		return
	}

	model.ID = types.StringValue(found.ID)
	model.Name = types.StringValue(found.Name)
	model.Default = types.BoolValue(found.IsDefault)
	if found.EventTypes == nil {
		model.EventTypes = types.SetNull(types.StringType)
	} else {
		model.EventTypes = setOf(*found.EventTypes)
	}
	model.ChannelIDs = setOf(found.ChannelIDs)
	model.WarningsDuringOutage = types.BoolValue(found.WarningsDuringOutage)
	model.Repeat = types.StringValue(found.Repeat)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// ---- uup_locations ----

// NewLocationsDataSource is uup_locations.
func NewLocationsDataSource() datasource.DataSource {
	return &locationsDataSource{}
}

type locationsDataSource struct {
	client *client.Client
}

type locationsModel struct {
	IDs       types.List      `tfsdk:"ids"`
	Locations []locationModel `tfsdk:"locations"`
}

type locationModel struct {
	ID      types.String `tfsdk:"id"`
	City    types.String `tfsdk:"city"`
	Country types.String `tfsdk:"country"`
}

func (d *locationsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_locations"
}

func (d *locationsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The locations a Monitor can be checked from. A Monitor's `locations` names them by `id`.",
		Attributes: map[string]schema.Attribute{
			"ids": schema.ListAttribute{
				MarkdownDescription: "Every location's `id`, such as `de`.",
				ElementType:         types.StringType,
				Computed:            true,
			},
			"locations": schema.ListNestedAttribute{
				MarkdownDescription: "Every location.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":      schema.StringAttribute{Computed: true, MarkdownDescription: "What a Monitor's `locations` names."},
						"city":    schema.StringAttribute{Computed: true, MarkdownDescription: "The city."},
						"country": schema.StringAttribute{Computed: true, MarkdownDescription: "The country, as an ISO 3166-1 alpha-2 code."},
					},
				},
			},
		},
	}
}

func (d *locationsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureDataSource(req, resp)
}

func (d *locationsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	locations, err := d.client.ListLocations(ctx)
	if err != nil {
		apiError(&resp.Diagnostics, "Could not list locations", err)
		return
	}

	model := locationsModel{Locations: []locationModel{}}
	ids := make([]string, 0, len(locations))
	for _, one := range locations {
		ids = append(ids, one.ID)
		model.Locations = append(model.Locations, locationModel{
			ID:      types.StringValue(one.ID),
			City:    types.StringValue(one.City),
			Country: types.StringPointerValue(one.Country),
		})
	}
	model.IDs = listOf(ids)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
