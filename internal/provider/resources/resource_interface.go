package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
)

var _ resource.Resource = &InterfaceResource{}
var _ resource.ResourceWithImportState = &InterfaceResource{}

func NewInterfaceResource() resource.Resource {
	return &InterfaceResource{}
}

type InterfaceResource struct {
	client *client.Client
}

type InterfaceResourceModel struct {
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Mode        types.String `tfsdk:"mode"`
	AccessVLAN  types.Int64  `tfsdk:"access_vlan"`
	TrunkVLANs  types.List   `tfsdk:"trunk_vlans"`
	NativeVLAN  types.Int64  `tfsdk:"native_vlan"`
}

func (r *InterfaceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_interface"
}

func (r *InterfaceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a switchport interface on a Cisco switch",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Interface name (e.g., GigabitEthernet1/0/1)",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
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
			"mode": schema.StringAttribute{
				Description: "Switchport mode: access or trunk",
				Required:    true,
			},
			"access_vlan": schema.Int64Attribute{
				Description: "VLAN ID for access mode (required when mode is 'access')",
				Optional:    true,
			},
			"trunk_vlans": schema.ListAttribute{
				Description: "List of allowed VLAN IDs for trunk mode (required when mode is 'trunk')",
				ElementType: types.Int64Type,
				Optional:    true,
			},
			"native_vlan": schema.Int64Attribute{
				Description: "Native VLAN for trunk mode (default: 1)",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(1),
			},
		},
	}
}

func (r *InterfaceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *InterfaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data InterfaceResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate configuration
	if err := r.validateConfig(&data); err != nil {
		resp.Diagnostics.AddError("Invalid Configuration", err.Error())
		return
	}

	// Build configuration commands
	commands := r.buildConfigCommands(&data)

	// Execute configuration
	err := r.client.ExecuteConfigCommands(commands)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Configuring Interface",
			fmt.Sprintf("Could not configure interface %s: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	// Read back to verify
	ifaceInfo, err := r.readInterface(data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Interface",
			fmt.Sprintf("Could not read interface %s after configuration: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	// Update state with actual values
	r.updateModelFromInfo(&data, ifaceInfo)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InterfaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data InterfaceResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read interface from switch
	ifaceInfo, err := r.readInterface(data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Interface",
			fmt.Sprintf("Could not read interface %s: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	if !ifaceInfo.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state
	r.updateModelFromInfo(&data, ifaceInfo)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InterfaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data InterfaceResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate configuration
	if err := r.validateConfig(&data); err != nil {
		resp.Diagnostics.AddError("Invalid Configuration", err.Error())
		return
	}

	// Build configuration commands
	commands := r.buildConfigCommands(&data)

	// Execute configuration
	err := r.client.ExecuteConfigCommands(commands)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Interface",
			fmt.Sprintf("Could not update interface %s: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	// Read back to verify
	ifaceInfo, err := r.readInterface(data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Interface",
			fmt.Sprintf("Could not read interface %s after update: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	// Update state
	r.updateModelFromInfo(&data, ifaceInfo)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InterfaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data InterfaceResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Reset interface to default configuration
	commands := []string{
		fmt.Sprintf("default interface %s", data.Name.ValueString()),
		"end",
	}

	err := r.client.ExecuteConfigCommands(commands)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Resetting Interface",
			fmt.Sprintf("Could not reset interface %s: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}
}

func (r *InterfaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import expects interface name (e.g., "GigabitEthernet1/0/1")
	ifaceName := req.ID

	// Read the interface
	ifaceInfo, err := r.readInterface(ifaceName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Interface",
			fmt.Sprintf("Could not read interface %s: %s", ifaceName, err.Error()),
		)
		return
	}

	if !ifaceInfo.Exists {
		resp.Diagnostics.AddError(
			"Interface Not Found",
			fmt.Sprintf("Interface %s does not exist on the switch", ifaceName),
		)
		return
	}

	// Set state
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), ifaceName)...)
	if ifaceInfo.Description != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("description"), ifaceInfo.Description)...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("enabled"), ifaceInfo.AdminState == "up")...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mode"), ifaceInfo.Mode)...)

	if ifaceInfo.Mode == "access" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("access_vlan"), ifaceInfo.AccessVLAN)...)
	} else if ifaceInfo.Mode == "trunk" {
		// Convert int slice to types.List
		var vlanList []int64
		for _, v := range ifaceInfo.TrunkVLANs {
			vlanList = append(vlanList, int64(v))
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("trunk_vlans"), vlanList)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("native_vlan"), ifaceInfo.NativeVLAN)...)
	}
}

// Helper methods

func (r *InterfaceResource) validateConfig(data *InterfaceResourceModel) error {
	mode := data.Mode.ValueString()

	if mode != "access" && mode != "trunk" {
		return fmt.Errorf("mode must be 'access' or 'trunk', got: %s", mode)
	}

	if mode == "access" {
		if data.AccessVLAN.IsNull() {
			return fmt.Errorf("access_vlan is required when mode is 'access'")
		}
	} else if mode == "trunk" {
		if data.TrunkVLANs.IsNull() {
			return fmt.Errorf("trunk_vlans is required when mode is 'trunk'")
		}
	}

	return nil
}

func (r *InterfaceResource) buildConfigCommands(data *InterfaceResourceModel) []string {
	commands := []string{
		fmt.Sprintf("interface %s", data.Name.ValueString()),
	}

	// Description
	if !data.Description.IsNull() && data.Description.ValueString() != "" {
		commands = append(commands, fmt.Sprintf("description %s", data.Description.ValueString()))
	}

	// Mode
	mode := data.Mode.ValueString()
	commands = append(commands, fmt.Sprintf("switchport mode %s", mode))

	// Mode-specific configuration
	if mode == "access" {
		commands = append(commands, fmt.Sprintf("switchport access vlan %d", data.AccessVLAN.ValueInt64()))
	} else if mode == "trunk" {
		// Build VLAN list
		var vlans []int64
		data.TrunkVLANs.ElementsAs(context.Background(), &vlans, false)

		vlanStrs := make([]string, len(vlans))
		for i, v := range vlans {
			vlanStrs[i] = fmt.Sprintf("%d", v)
		}
		vlanList := strings.Join(vlanStrs, ",")

		commands = append(commands, fmt.Sprintf("switchport trunk allowed vlan %s", vlanList))

		if !data.NativeVLAN.IsNull() && data.NativeVLAN.ValueInt64() != 1 {
			commands = append(commands, fmt.Sprintf("switchport trunk native vlan %d", data.NativeVLAN.ValueInt64()))
		}
	}

	// Admin state
	if data.Enabled.ValueBool() {
		commands = append(commands, "no shutdown")
	} else {
		commands = append(commands, "shutdown")
	}

	commands = append(commands, "end")

	return commands
}

func (r *InterfaceResource) readInterface(name string) (*client.InterfaceInfo, error) {
	output, err := r.client.ExecuteCommand(fmt.Sprintf("show running-config interface %s", name))
	if err != nil {
		return nil, err
	}

	return client.ParseInterfaceConfig(output, name)
}

func (r *InterfaceResource) updateModelFromInfo(data *InterfaceResourceModel, info *client.InterfaceInfo) {
	if info.Description != "" {
		data.Description = types.StringValue(info.Description)
	}
	data.Enabled = types.BoolValue(info.AdminState == "up")
	data.Mode = types.StringValue(info.Mode)

	if info.Mode == "access" {
		data.AccessVLAN = types.Int64Value(int64(info.AccessVLAN))
	} else if info.Mode == "trunk" {
		// Convert int slice to types.List
		var vlanList []int64
		for _, v := range info.TrunkVLANs {
			vlanList = append(vlanList, int64(v))
		}
		listVal, _ := types.ListValueFrom(context.Background(), types.Int64Type, vlanList)
		data.TrunkVLANs = listVal
		data.NativeVLAN = types.Int64Value(int64(info.NativeVLAN))
	}
}
