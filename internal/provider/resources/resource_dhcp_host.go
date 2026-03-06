package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
)

var _ resource.Resource = &DHCPHostResource{}
var _ resource.ResourceWithImportState = &DHCPHostResource{}

func NewDHCPHostResource() resource.Resource {
	return &DHCPHostResource{}
}

type DHCPHostResource struct {
	client *client.Client
}

type DHCPHostResourceModel struct {
	PoolName        types.String `tfsdk:"pool_name"`
	IPAddress       types.String `tfsdk:"ip_address"`
	SubnetMask      types.String `tfsdk:"subnet_mask"`
	HardwareAddress types.String `tfsdk:"hardware_address"`
	ClientName      types.String `tfsdk:"client_name"`
	DefaultRouter   types.String `tfsdk:"default_router"`
}

func (r *DHCPHostResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dhcp_host"
}

func (r *DHCPHostResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a DHCP host binding (static MAC → IP reservation) implemented as a named DHCP pool " +
			"with a 'host' sub-command. The switch always offers the reserved IP to the client with the given MAC address.",
		Attributes: map[string]schema.Attribute{
			"pool_name": schema.StringAttribute{
				Description: "Name of the DHCP host pool on the switch. Must be unique. Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ip_address": schema.StringAttribute{
				Description: "IP address to reserve for this host.",
				Required:    true,
			},
			"subnet_mask": schema.StringAttribute{
				Description: "Subnet mask for the reserved address (e.g. \"255.255.255.0\").",
				Required:    true,
			},
			"hardware_address": schema.StringAttribute{
				Description: "Client MAC address in colon-separated notation (e.g. \"aa:bb:cc:dd:ee:ff\", case-insensitive). " +
					"The provider normalises to lowercase colon format on read.",
				Required: true,
			},
			"client_name": schema.StringAttribute{
				Description: "Optional client hostname stored in the DHCP pool.",
				Optional:    true,
			},
			"default_router": schema.StringAttribute{
				Description: "Optional default gateway offered specifically to this host. " +
					"When set it overrides any default-router set by a network pool for this host.",
				Optional: true,
			},
		},
	}
}

func (r *DHCPHostResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DHCPHostResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DHCPHostResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	if err := r.client.ExecuteConfigCommands(r.buildCommands(data)); err != nil {
		resp.Diagnostics.AddError(
			"Error Creating DHCP Host Binding",
			fmt.Sprintf("Could not create DHCP host pool %q: %s", data.PoolName.ValueString(), err.Error()),
		)
		return
	}

	info, err := r.readHostPool(data.PoolName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading DHCP Host Binding",
			fmt.Sprintf("Could not read pool %q after creation: %s", data.PoolName.ValueString(), err.Error()))
		return
	}
	if !info.Exists {
		resp.Diagnostics.AddError("DHCP Host Binding Not Found",
			fmt.Sprintf("Pool %q was not found after creation", data.PoolName.ValueString()))
		return
	}
	r.updateModelFromInfo(&data, info)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DHCPHostResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DHCPHostResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	info, err := r.readHostPool(data.PoolName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading DHCP Host Binding",
			fmt.Sprintf("Could not read pool %q: %s", data.PoolName.ValueString(), err.Error()))
		return
	}
	if !info.Exists {
		resp.State.RemoveResource(ctx)
		return
	}
	r.updateModelFromInfo(&data, info)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DHCPHostResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state DHCPHostResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	commands := []string{
		fmt.Sprintf("ip dhcp pool %s", plan.PoolName.ValueString()),
		fmt.Sprintf("host %s %s", plan.IPAddress.ValueString(), plan.SubnetMask.ValueString()),
		fmt.Sprintf("hardware-address %s", plan.HardwareAddress.ValueString()),
	}

	// Clear optional fields that were previously set but are now removed.
	oldClientName := state.ClientName.ValueString()
	newClientName := plan.ClientName.ValueString()
	if oldClientName != "" && newClientName == "" {
		commands = append(commands, "no client-name")
	} else if newClientName != "" {
		commands = append(commands, fmt.Sprintf("client-name %s", newClientName))
	}

	oldDefaultRouter := state.DefaultRouter.ValueString()
	newDefaultRouter := plan.DefaultRouter.ValueString()
	if oldDefaultRouter != "" && newDefaultRouter == "" {
		commands = append(commands, "no default-router")
	} else if newDefaultRouter != "" {
		commands = append(commands, fmt.Sprintf("default-router %s", newDefaultRouter))
	}

	commands = append(commands, "end")

	if err := r.client.ExecuteConfigCommands(commands); err != nil {
		resp.Diagnostics.AddError(
			"Error Updating DHCP Host Binding",
			fmt.Sprintf("Could not update pool %q: %s", plan.PoolName.ValueString(), err.Error()),
		)
		return
	}

	info, err := r.readHostPool(plan.PoolName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading DHCP Host Binding",
			fmt.Sprintf("Could not read pool %q after update: %s", plan.PoolName.ValueString(), err.Error()))
		return
	}
	r.updateModelFromInfo(&plan, info)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DHCPHostResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DHCPHostResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	cmds := []string{
		fmt.Sprintf("no ip dhcp pool %s", data.PoolName.ValueString()),
		"end",
	}
	if err := r.client.ExecuteConfigCommands(cmds); err != nil {
		if containsError(err.Error(), "does not exist") {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting DHCP Host Binding",
			fmt.Sprintf("Could not delete pool %q: %s", data.PoolName.ValueString(), err.Error()),
		)
	}
}

func (r *DHCPHostResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID is the pool name.
	info, err := r.readHostPool(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading DHCP Host Binding",
			fmt.Sprintf("Could not read pool %q: %s", req.ID, err.Error()))
		return
	}
	if !info.Exists {
		resp.Diagnostics.AddError("DHCP Host Binding Not Found",
			fmt.Sprintf("Pool %q does not exist on the switch", req.ID))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_name"), info.PoolName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip_address"), info.IPAddress)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("subnet_mask"), info.SubnetMask)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("hardware_address"), info.HardwareAddress)...)
	if info.ClientName != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("client_name"), info.ClientName)...)
	}
	if info.DefaultRouter != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("default_router"), info.DefaultRouter)...)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (r *DHCPHostResource) buildCommands(data DHCPHostResourceModel) []string {
	cmds := []string{
		fmt.Sprintf("ip dhcp pool %s", data.PoolName.ValueString()),
		fmt.Sprintf("host %s %s", data.IPAddress.ValueString(), data.SubnetMask.ValueString()),
		fmt.Sprintf("hardware-address %s", data.HardwareAddress.ValueString()),
	}
	if !data.ClientName.IsNull() && data.ClientName.ValueString() != "" {
		cmds = append(cmds, fmt.Sprintf("client-name %s", data.ClientName.ValueString()))
	}
	if !data.DefaultRouter.IsNull() && data.DefaultRouter.ValueString() != "" {
		cmds = append(cmds, fmt.Sprintf("default-router %s", data.DefaultRouter.ValueString()))
	}
	return append(cmds, "end")
}

func (r *DHCPHostResource) readHostPool(name string) (*client.DHCPHostInfo, error) {
	output, err := r.client.ExecuteCommand(
		fmt.Sprintf("show running-config | section ip dhcp pool %s", name))
	if err != nil {
		return nil, err
	}
	return client.ParseDHCPHostPool(output, name)
}

func (r *DHCPHostResource) updateModelFromInfo(data *DHCPHostResourceModel, info *client.DHCPHostInfo) {
	data.IPAddress = types.StringValue(info.IPAddress)
	data.SubnetMask = types.StringValue(info.SubnetMask)
	data.HardwareAddress = types.StringValue(info.HardwareAddress)
	if info.ClientName != "" {
		data.ClientName = types.StringValue(info.ClientName)
	} else {
		data.ClientName = types.StringNull()
	}
	if info.DefaultRouter != "" {
		data.DefaultRouter = types.StringValue(info.DefaultRouter)
	} else {
		data.DefaultRouter = types.StringNull()
	}
}
