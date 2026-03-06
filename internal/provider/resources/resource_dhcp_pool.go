package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
)

var _ resource.Resource = &DHCPPoolResource{}
var _ resource.ResourceWithImportState = &DHCPPoolResource{}

func NewDHCPPoolResource() resource.Resource {
	return &DHCPPoolResource{}
}

type DHCPPoolResource struct {
	client *client.Client
}

type DHCPPoolResourceModel struct {
	Name          types.String `tfsdk:"name"`
	Network       types.String `tfsdk:"network"`
	SubnetMask    types.String `tfsdk:"subnet_mask"`
	DefaultRouter types.String `tfsdk:"default_router"`
	DNSServers    types.List   `tfsdk:"dns_servers"`
	LeaseDays     types.Int64  `tfsdk:"lease_days"`
	LeaseHours    types.Int64  `tfsdk:"lease_hours"`
	LeaseMinutes  types.Int64  `tfsdk:"lease_minutes"`
	DomainName    types.String `tfsdk:"domain_name"`
}

func (r *DHCPPoolResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dhcp_pool"
}

func (r *DHCPPoolResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a DHCP pool on a Cisco switch (ip dhcp pool)",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "DHCP pool name. Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"network": schema.StringAttribute{
				Description: "Network address for the DHCP pool (e.g., \"192.168.100.0\")",
				Required:    true,
			},
			"subnet_mask": schema.StringAttribute{
				Description: "Subnet mask for the DHCP pool (e.g., \"255.255.255.0\")",
				Required:    true,
			},
			"default_router": schema.StringAttribute{
				Description: "Default gateway IP address offered to DHCP clients",
				Optional:    true,
			},
			"dns_servers": schema.ListAttribute{
				Description: "List of DNS server IP addresses offered to DHCP clients",
				ElementType: types.StringType,
				Optional:    true,
			},
			"lease_days": schema.Int64Attribute{
				Description: "Lease duration in days (default: 1)",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(1),
			},
			"lease_hours": schema.Int64Attribute{
				Description: "Additional lease hours (default: 0)",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"lease_minutes": schema.Int64Attribute{
				Description: "Additional lease minutes (default: 0)",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
			},
			"domain_name": schema.StringAttribute{
				Description: "Domain name offered to DHCP clients",
				Optional:    true,
			},
		},
	}
}

func (r *DHCPPoolResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DHCPPoolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DHCPPoolResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	commands := r.buildCommands(ctx, data)

	if err := r.client.ExecuteConfigCommands(commands); err != nil {
		resp.Diagnostics.AddError(
			"Error Creating DHCP Pool",
			fmt.Sprintf("Could not create DHCP pool %q: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	// Read back to populate computed fields
	poolInfo, err := r.readPool(data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading DHCP Pool",
			fmt.Sprintf("Could not read DHCP pool %q after creation: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	if !poolInfo.Exists {
		resp.Diagnostics.AddError(
			"DHCP Pool Not Found",
			fmt.Sprintf("DHCP pool %q was not found after creation", data.Name.ValueString()),
		)
		return
	}

	r.updateModelFromInfo(ctx, &data, poolInfo)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DHCPPoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DHCPPoolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolInfo, err := r.readPool(data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading DHCP Pool",
			fmt.Sprintf("Could not read DHCP pool %q: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	if !poolInfo.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	r.updateModelFromInfo(ctx, &data, poolInfo)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DHCPPoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DHCPPoolResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	commands := r.buildCommands(ctx, data)

	if err := r.client.ExecuteConfigCommands(commands); err != nil {
		resp.Diagnostics.AddError(
			"Error Updating DHCP Pool",
			fmt.Sprintf("Could not update DHCP pool %q: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	poolInfo, err := r.readPool(data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading DHCP Pool",
			fmt.Sprintf("Could not read DHCP pool %q after update: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	r.updateModelFromInfo(ctx, &data, poolInfo)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DHCPPoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DHCPPoolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	commands := []string{
		fmt.Sprintf("no ip dhcp pool %s", data.Name.ValueString()),
		"end",
	}

	if err := r.client.ExecuteConfigCommands(commands); err != nil {
		if containsError(err.Error(), "does not exist") {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting DHCP Pool",
			fmt.Sprintf("Could not delete DHCP pool %q: %s", data.Name.ValueString(), err.Error()),
		)
	}
}

func (r *DHCPPoolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	poolInfo, err := r.readPool(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading DHCP Pool",
			fmt.Sprintf("Could not read DHCP pool %q: %s", req.ID, err.Error()),
		)
		return
	}

	if !poolInfo.Exists {
		resp.Diagnostics.AddError(
			"DHCP Pool Not Found",
			fmt.Sprintf("DHCP pool %q does not exist on the switch", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), poolInfo.Name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("network"), poolInfo.Network)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("subnet_mask"), poolInfo.SubnetMask)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("default_router"), poolInfo.DefaultRouter)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("lease_days"), int64(poolInfo.LeaseDays))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("lease_hours"), int64(poolInfo.LeaseHours))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("lease_minutes"), int64(poolInfo.LeaseMinutes))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain_name"), poolInfo.DomainName)...)

	if len(poolInfo.DNSServers) > 0 {
		dnsElements := make([]types.String, len(poolInfo.DNSServers))
		for i, s := range poolInfo.DNSServers {
			dnsElements[i] = types.StringValue(s)
		}
		dnsList, _ := types.ListValueFrom(ctx, types.StringType, dnsElements)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("dns_servers"), dnsList)...)
	}
}

// buildCommands constructs the IOS config commands for creating/updating a DHCP pool.
func (r *DHCPPoolResource) buildCommands(ctx context.Context, data DHCPPoolResourceModel) []string {
	// "ip dhcp pool NAME" enters dhcp-config sub-mode; subsequent lines are pool sub-commands.
	commands := []string{
		fmt.Sprintf("ip dhcp pool %s", data.Name.ValueString()),
		fmt.Sprintf("network %s %s", data.Network.ValueString(), data.SubnetMask.ValueString()),
	}

	if !data.DefaultRouter.IsNull() && data.DefaultRouter.ValueString() != "" {
		commands = append(commands, fmt.Sprintf("default-router %s", data.DefaultRouter.ValueString()))
	}

	if !data.DNSServers.IsNull() && len(data.DNSServers.Elements()) > 0 {
		var servers []types.String
		data.DNSServers.ElementsAs(ctx, &servers, false)
		serverStrs := make([]string, len(servers))
		for i, s := range servers {
			serverStrs[i] = s.ValueString()
		}
		commands = append(commands, fmt.Sprintf("dns-server %s", joinStrings(serverStrs, " ")))
	}

	commands = append(commands, fmt.Sprintf("lease %d %d %d",
		data.LeaseDays.ValueInt64(),
		data.LeaseHours.ValueInt64(),
		data.LeaseMinutes.ValueInt64(),
	))

	if !data.DomainName.IsNull() && data.DomainName.ValueString() != "" {
		commands = append(commands, fmt.Sprintf("domain-name %s", data.DomainName.ValueString()))
	}

	commands = append(commands, "end")
	return commands
}

func (r *DHCPPoolResource) readPool(name string) (*client.DHCPPoolInfo, error) {
	output, err := r.client.ExecuteCommand(fmt.Sprintf("show running-config | section ip dhcp pool %s", name))
	if err != nil {
		return nil, err
	}
	return client.ParseDHCPPool(output, name)
}

func (r *DHCPPoolResource) updateModelFromInfo(ctx context.Context, data *DHCPPoolResourceModel, info *client.DHCPPoolInfo) {
	data.Network = types.StringValue(info.Network)
	data.SubnetMask = types.StringValue(info.SubnetMask)
	data.LeaseDays = types.Int64Value(int64(info.LeaseDays))
	data.LeaseHours = types.Int64Value(int64(info.LeaseHours))
	data.LeaseMinutes = types.Int64Value(int64(info.LeaseMinutes))

	if info.DefaultRouter != "" {
		data.DefaultRouter = types.StringValue(info.DefaultRouter)
	}

	if info.DomainName != "" {
		data.DomainName = types.StringValue(info.DomainName)
	}

	if len(info.DNSServers) > 0 {
		dnsElements := make([]types.String, len(info.DNSServers))
		for i, s := range info.DNSServers {
			dnsElements[i] = types.StringValue(s)
		}
		dnsList, _ := types.ListValueFrom(ctx, types.StringType, dnsElements)
		data.DNSServers = dnsList
	}
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
