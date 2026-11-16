// Package provider is the You Up? Terraform and OpenTofu provider.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uupdev/terraform-provider-uup/internal/client"
)

var _ provider.Provider = &uupProvider{}

// New returns the provider at this version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &uupProvider{version: version}
	}
}

type uupProvider struct {
	version string
}

type providerModel struct {
	APIToken types.String `tfsdk:"api_token"`
	BaseURL  types.String `tfsdk:"base_url"`
}

func (p *uupProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "uup"
	resp.Version = p.version
}

func (p *uupProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage You Up? (uup.dev) Monitors and Notification rules as code.",
		MarkdownDescription: "Manage [You Up?](https://uup.dev) Monitors and Notification rules as code, " +
			"through the public API (`/v1`).\n\n" +
			"Make an API token of the **Full** kind in the dashboard, on the **API tokens** screen, " +
			"and give it to the provider in `api_token` or in the `UUP_API_TOKEN` environment variable.",
		Attributes: map[string]schema.Attribute{
			"api_token": schema.StringAttribute{
				MarkdownDescription: "An API token with **Full** access. Can also be set with the `UUP_API_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"base_url": schema.StringAttribute{
				MarkdownDescription: "The API's address. Defaults to `" + client.DefaultBaseURL + "`. Can also be set with the `UUP_API_URL` environment variable.",
				Optional:            true,
			},
		},
	}
}

func (p *uupProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token := os.Getenv("UUP_API_TOKEN")
	if !config.APIToken.IsNull() && !config.APIToken.IsUnknown() {
		token = config.APIToken.ValueString()
	}
	baseURL := client.DefaultBaseURL
	if env := os.Getenv("UUP_API_URL"); env != "" {
		baseURL = env
	}
	if !config.BaseURL.IsNull() && !config.BaseURL.IsUnknown() {
		baseURL = config.BaseURL.ValueString()
	}

	if token == "" {
		resp.Diagnostics.AddError(
			"No API token",
			"Set api_token in the provider block, or the UUP_API_TOKEN environment variable, "+
				"to an API token of the Full kind. Make one in the You Up? dashboard, on the API tokens screen.",
		)
		return
	}

	c := client.New(baseURL, token, "terraform-provider-uup/"+p.version)
	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *uupProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewMonitorResource,
		NewNotificationRuleResource,
	}
}

func (p *uupProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewProjectDataSource,
		NewChannelDataSource,
		NewNotificationRuleDataSource,
		NewLocationsDataSource,
	}
}
