package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
)

var _ resource.Resource = &SVIResource{}
var _ resource.ResourceWithImportState = &SVIResource{}

func NewSVIResource() resource.Resource {
	return &SVIResource{}
}

type SVIResource struct {
	client *client.Client
}

type SVIResourceModel struct {
	VlanID     types.Int64  `tfsdk:"vlan_id"`
	IPAddress  types.String `tfsdk:"ip_address"`
	SubnetMask types.String `tfsdk:"subnet_mask"`
	Description types.String `tfsdk:"description"`
	Enabled    types.Bool   `tfsdk:"enabled"`
}

func (r *SVIResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_svi"
}

func (r *SVIResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Switch Virtual Interface (SVI) for inter-VLAN routing",
		Attributes: map[string]schema.Attribute{
			"vlan_id": schema.Int64Attribute{
				Description: "VLAN ID for the SVI (1-4094)",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"ip_address": schema.StringAttribute{
				Description: "IP address for the SVI",
				Required:    true,
			},
			"subnet_mask": schema.StringAttribute{
				Description: "Subnet mask for the SVI",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "Interface description",
				Optional:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Administrative state (true = no shutdown, false = shutdown)",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
		},
	}
}

func (r *SVIResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *SVIResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SVIResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vlanID := int(data.VlanID.ValueInt64())

	// Build configuration commands
	commands := []string{
		fmt.Sprintf("interface vlan %d", vlanID),
	}

	if !data.Description.IsNull() && data.Description.ValueString() != "" {
		commands = append(commands, fmt.Sprintf("description %s", data.Description.ValueString()))
	}

	commands = append(commands, fmt.Sprintf("ip address %s %s",
		data.IPAddress.ValueString(),
		data.SubnetMask.ValueString()))

	if data.Enabled.ValueBool() {
		commands = append(commands, "no shutdown")
	} else {
		commands = append(commands, "shutdown")
	}

	commands = append(commands, "end")

	// Execute configuration
	err := r.client.ExecuteConfigCommands(commands)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating SVI",
			fmt.Sprintf("Could not create SVI for VLAN %d: %s", vlanID, err.Error()),
		)
		return
	}

	// Read back to verify
	sviInfo, err := r.readSVI(vlanID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading SVI",
			fmt.Sprintf("Could not read SVI for VLAN %d after creation: %s", vlanID, err.Error()),
		)
		return
	}

	if !sviInfo.Exists {
		resp.Diagnostics.AddError(
			"SVI Not Found",
			fmt.Sprintf("SVI for VLAN %d was not found after creation", vlanID),
		)
		return
	}

	// Update state with actual values
	r.updateModelFromInfo(&data, sviInfo)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SVIResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SVIResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vlanID := int(data.VlanID.ValueInt64())

	// Read SVI from switch
	sviInfo, err := r.readSVI(vlanID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading SVI",
			fmt.Sprintf("Could not read SVI for VLAN %d: %s", vlanID, err.Error()),
		)
		return
	}

	if !sviInfo.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state
	r.updateModelFromInfo(&data, sviInfo)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SVIResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SVIResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vlanID := int(data.VlanID.ValueInt64())

	// Build configuration commands
	commands := []string{
		fmt.Sprintf("interface vlan %d", vlanID),
	}

	if !data.Description.IsNull() && data.Description.ValueString() != "" {
		commands = append(commands, fmt.Sprintf("description %s", data.Description.ValueString()))
	}

	commands = append(commands, fmt.Sprintf("ip address %s %s",
		data.IPAddress.ValueString(),
		data.SubnetMask.ValueString()))

	if data.Enabled.ValueBool() {
		commands = append(commands, "no shutdown")
	} else {
		commands = append(commands, "shutdown")
	}

	commands = append(commands, "end")

	// Execute configuration
	err := r.client.ExecuteConfigCommands(commands)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating SVI",
			fmt.Sprintf("Could not update SVI for VLAN %d: %s", vlanID, err.Error()),
		)
		return
	}

	// Read back to verify
	sviInfo, err := r.readSVI(vlanID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading SVI",
			fmt.Sprintf("Could not read SVI for VLAN %d after update: %s", vlanID, err.Error()),
		)
		return
	}

	// Update state
	r.updateModelFromInfo(&data, sviInfo)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SVIResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SVIResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vlanID := int(data.VlanID.ValueInt64())

	// Delete SVI
	commands := []string{
		fmt.Sprintf("no interface vlan %d", vlanID),
		"end",
	}

	err := r.client.ExecuteConfigCommands(commands)
	if err != nil {
		// Check if error is because SVI doesn't exist
		if containsError(err.Error(), "does not exist") {
			return
		}

		resp.Diagnostics.AddError(
			"Error Deleting SVI",
			fmt.Sprintf("Could not delete SVI for VLAN %d: %s", vlanID, err.Error()),
		)
		return
	}
}

func (r *SVIResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import expects VLAN ID as string (e.g., "100")
	vlanID, err := strconv.Atoi(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Import ID must be a valid VLAN ID number: %s", err.Error()),
		)
		return
	}

	// Read the SVI
	sviInfo, err := r.readSVI(vlanID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading SVI",
			fmt.Sprintf("Could not read SVI for VLAN %d: %s", vlanID, err.Error()),
		)
		return
	}

	if !sviInfo.Exists {
		resp.Diagnostics.AddError(
			"SVI Not Found",
			fmt.Sprintf("SVI for VLAN %d does not exist on the switch", vlanID),
		)
		return
	}

	// Set state
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vlan_id"), vlanID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip_address"), sviInfo.IPAddress)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("subnet_mask"), sviInfo.SubnetMask)...)
	if sviInfo.Description != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("description"), sviInfo.Description)...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("enabled"), sviInfo.AdminState == "up")...)
}

// Helper methods

func (r *SVIResource) readSVI(vlanID int) (*client.SVIInfo, error) {
	output, err := r.client.ExecuteCommand(fmt.Sprintf("show running-config interface vlan %d", vlanID))
	if err != nil {
		return nil, err
	}

	return client.ParseSVIConfig(output, vlanID)
}

func (r *SVIResource) updateModelFromInfo(data *SVIResourceModel, info *client.SVIInfo) {
	data.IPAddress = types.StringValue(info.IPAddress)
	data.SubnetMask = types.StringValue(info.SubnetMask)
	if info.Description != "" {
		data.Description = types.StringValue(info.Description)
	}
	data.Enabled = types.BoolValue(info.AdminState == "up")
}
