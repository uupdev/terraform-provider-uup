package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uupdev/terraform-provider-uup/internal/client"
)

// clientFrom takes the provider's client from a resource's or data source's
// Configure request. It is nil before the provider is configured.
func clientFrom(data any, diags *diag.Diagnostics) *client.Client {
	if data == nil {
		return nil
	}
	c, ok := data.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("Expected *client.Client, got %T.", data))
		return nil
	}
	return c
}

func configureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *client.Client {
	return clientFrom(req.ProviderData, &resp.Diagnostics)
}

func configureDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *client.Client {
	return clientFrom(req.ProviderData, &resp.Diagnostics)
}

// splitImportID reads `<project_id>/<id>`.
func splitImportID(id, what string, diags *diag.Diagnostics) (string, string, bool) {
	parts := strings.Split(id, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		diags.AddError(
			"Unexpected import ID",
			fmt.Sprintf("Import a %s as <project_id>/<%s_id>, such as prj_abc/%s. Got %q.", what, what, examplePrefix(what), id),
		)
		return "", "", false
	}
	return parts[0], parts[1], true
}

func examplePrefix(what string) string {
	if what == "rule" {
		return "rul_def"
	}
	return "mon_def"
}

func importInto(ctx context.Context, id, what string, resp *resource.ImportStateResponse) {
	projectID, ownID, ok := splitImportID(id, what, &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), ownID)...)
}

// stringsOf reads a set or list of strings. Null and unknown read as nil.
func stringsOf(ctx context.Context, value interface {
	IsNull() bool
	IsUnknown() bool
	ElementsAs(context.Context, any, bool) diag.Diagnostics
}, diags *diag.Diagnostics) []string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(value.ElementsAs(ctx, &out, false)...)
	return out
}

func setOf(items []string) types.Set {
	values := make([]attr.Value, 0, len(items))
	for _, item := range items {
		values = append(values, types.StringValue(item))
	}
	return types.SetValueMust(types.StringType, values)
}

func listOf(items []string) types.List {
	values := make([]attr.Value, 0, len(items))
	for _, item := range items {
		values = append(values, types.StringValue(item))
	}
	return types.ListValueMust(types.StringType, values)
}

func sorted(items []string) []string {
	out := append([]string(nil), items...)
	sort.Strings(out)
	return out
}

// known says whether a value was set or worked out, not left to the API.
func known(value attr.Value) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func stringPtr(value types.String) *string {
	if !known(value) {
		return nil
	}
	v := value.ValueString()
	return &v
}

func int64Ptr(value types.Int64) *int64 {
	if !known(value) {
		return nil
	}
	v := value.ValueInt64()
	return &v
}

func boolPtr(value types.Bool) *bool {
	if !known(value) {
		return nil
	}
	v := value.ValueBool()
	return &v
}

func ptr[T any](v T) *T {
	return &v
}

func apiError(diags *diag.Diagnostics, summary string, err error) {
	diags.AddError(summary, err.Error())
}

// joinTicks lists values for a description, as `a`, `b`, `c`.
func joinTicks(values []string) string {
	return strings.Join(values, "`, `")
}
