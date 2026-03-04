package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
)

var _ resource.Resource = &StaticRouteResource{}
var _ resource.ResourceWithImportState = &StaticRouteResource{}

func NewStaticRouteResource() resource.Resource {
	return &StaticRouteResource{}
}

type StaticRouteResource struct {
	client *client.Client
}

type StaticRouteResourceModel struct {
	Network       types.String `tfsdk:"network"`
	Mask          types.String `tfsdk:"mask"`
	NextHop       types.String `tfsdk:"next_hop"`
	AdminDistance types.Int64  `tfsdk:"admin_distance"`
}

func (r *StaticRouteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_static_route"
}

func (r *StaticRouteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a static IP route on a Cisco switch (ip route). " +
			"Use network=\"0.0.0.0\" mask=\"0.0.0.0\" for the default route.",
		Attributes: map[string]schema.Attribute{
			"network": schema.StringAttribute{
				Description: "Destination network address (e.g. \"0.0.0.0\" for a default route). Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mask": schema.StringAttribute{
				Description: "Destination subnet mask (e.g. \"0.0.0.0\" for a default route). Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"next_hop": schema.StringAttribute{
				Description: "Next-hop IP address. Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"admin_distance": schema.Int64Attribute{
				Description: "Administrative distance (1–255, default 1). Only the distance can be updated in-place; all other attributes force replacement.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(1),
			},
		},
	}
}

func (r *StaticRouteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *StaticRouteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data StaticRouteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.ExecuteConfigCommands(r.buildCommands(data)); err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Static Route",
			fmt.Sprintf("Could not create static route %s %s via %s: %s",
				data.Network.ValueString(), data.Mask.ValueString(), data.NextHop.ValueString(), err.Error()),
		)
		return
	}

	info, err := r.readRoute(data.Network.ValueString(), data.Mask.ValueString(), data.NextHop.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Static Route",
			fmt.Sprintf("Could not read static route after creation: %s", err.Error()))
		return
	}
	if !info.Exists {
		resp.Diagnostics.AddError("Static Route Not Found",
			"Static route was not found on the switch after creation")
		return
	}
	r.updateModelFromInfo(&data, info)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *StaticRouteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data StaticRouteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	info, err := r.readRoute(data.Network.ValueString(), data.Mask.ValueString(), data.NextHop.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Static Route",
			fmt.Sprintf("Could not read static route: %s", err.Error()))
		return
	}
	if !info.Exists {
		resp.State.RemoveResource(ctx)
		return
	}
	r.updateModelFromInfo(&data, info)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *StaticRouteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data StaticRouteResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Re-issuing ip route with a new distance replaces the existing entry.
	if err := r.client.ExecuteConfigCommands(r.buildCommands(data)); err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Static Route",
			fmt.Sprintf("Could not update static route: %s", err.Error()),
		)
		return
	}

	info, err := r.readRoute(data.Network.ValueString(), data.Mask.ValueString(), data.NextHop.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Static Route",
			fmt.Sprintf("Could not read static route after update: %s", err.Error()))
		return
	}
	r.updateModelFromInfo(&data, info)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *StaticRouteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data StaticRouteResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cmds := r.buildCommands(data)
	cmds[0] = "no " + cmds[0]
	if err := r.client.ExecuteConfigCommands(cmds); err != nil {
		if containsError(err.Error(), "does not exist") {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Static Route",
			fmt.Sprintf("Could not delete static route: %s", err.Error()),
		)
	}
}

func (r *StaticRouteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID format: "network/mask/next_hop"  e.g. "0.0.0.0/0.0.0.0/10.10.99.1"
	parts := splitImportID(req.ID, "/", 3)
	if parts == nil {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			"Import ID must be: network/mask/next_hop  (e.g. 0.0.0.0/0.0.0.0/10.10.99.1)",
		)
		return
	}
	network, mask, nextHop := parts[0], parts[1], parts[2]

	info, err := r.readRoute(network, mask, nextHop)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Static Route",
			fmt.Sprintf("Could not read static route: %s", err.Error()))
		return
	}
	if !info.Exists {
		resp.Diagnostics.AddError("Static Route Not Found",
			fmt.Sprintf("Static route %s %s via %s does not exist", network, mask, nextHop))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("network"), info.Network)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mask"), info.Mask)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("next_hop"), info.NextHop)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("admin_distance"), int64(info.AdminDistance))...)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (r *StaticRouteResource) buildCommands(data StaticRouteResourceModel) []string {
	cmd := fmt.Sprintf("ip route %s %s %s",
		data.Network.ValueString(),
		data.Mask.ValueString(),
		data.NextHop.ValueString(),
	)
	if dist := data.AdminDistance.ValueInt64(); dist != 1 {
		cmd += fmt.Sprintf(" %d", dist)
	}
	return []string{cmd, "end"}
}

func (r *StaticRouteResource) readRoute(network, mask, nextHop string) (*client.StaticRouteInfo, error) {
	output, err := r.client.ExecuteCommand("show running-config | include ip route")
	if err != nil {
		return nil, err
	}
	return client.ParseStaticRoute(output, network, mask, nextHop)
}

func (r *StaticRouteResource) updateModelFromInfo(data *StaticRouteResourceModel, info *client.StaticRouteInfo) {
	data.Network = types.StringValue(info.Network)
	data.Mask = types.StringValue(info.Mask)
	data.NextHop = types.StringValue(info.NextHop)
	data.AdminDistance = types.Int64Value(int64(info.AdminDistance))
}
