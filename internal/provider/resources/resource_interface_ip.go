package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
)

var _ resource.Resource = &InterfaceIPResource{}
var _ resource.ResourceWithImportState = &InterfaceIPResource{}

func NewInterfaceIPResource() resource.Resource {
	return &InterfaceIPResource{}
}

type InterfaceIPResource struct {
	client *client.Client
}

type InterfaceIPResourceModel struct {
	Interface  types.String `tfsdk:"interface"`
	IPAddress  types.String `tfsdk:"ip_address"`
	SubnetMask types.String `tfsdk:"subnet_mask"`
	DHCP       types.Bool   `tfsdk:"dhcp"`
}

func (r *InterfaceIPResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_interface_ip"
}

func (r *InterfaceIPResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages IP address assignment on an interface (typically for management)",
		Attributes: map[string]schema.Attribute{
			"interface": schema.StringAttribute{
				Description: "Interface name (e.g., Vlan1)",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ip_address": schema.StringAttribute{
				Description: "Static IP address (mutually exclusive with dhcp)",
				Optional:    true,
			},
			"subnet_mask": schema.StringAttribute{
				Description: "Subnet mask (required with ip_address)",
				Optional:    true,
			},
			"dhcp": schema.BoolAttribute{
				Description: "Use DHCP for IP address (mutually exclusive with ip_address)",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
		},
	}
}

func (r *InterfaceIPResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *InterfaceIPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data InterfaceIPResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	// Validate configuration
	if err := r.validateConfig(&data); err != nil {
		resp.Diagnostics.AddError("Invalid Configuration", err.Error())
		return
	}

	// Build configuration commands
	commands := []string{
		fmt.Sprintf("interface %s", data.Interface.ValueString()),
	}

	if data.DHCP.ValueBool() {
		commands = append(commands, "ip address dhcp")
	} else {
		commands = append(commands, fmt.Sprintf("ip address %s %s",
			data.IPAddress.ValueString(),
			data.SubnetMask.ValueString()))
	}

	commands = append(commands, "no shutdown", "end")

	// Execute configuration
	err := r.client.ExecuteConfigCommands(commands)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Configuring Interface IP",
			fmt.Sprintf("Could not configure IP on interface %s: %s",
				data.Interface.ValueString(), err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InterfaceIPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data InterfaceIPResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read interface configuration
	output, err := r.client.ExecuteCommand(fmt.Sprintf("show running-config interface %s",
		data.Interface.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Interface",
			fmt.Sprintf("Could not read interface %s: %s",
				data.Interface.ValueString(), err.Error()),
		)
		return
	}

	// Check if interface has IP configuration
	hasIP := false
	isDHCP := false

	lines := splitConfigLines(output)
	for _, line := range lines {
		if containsSubstring(line, "ip address dhcp") {
			hasIP = true
			isDHCP = true
			break
		} else if containsSubstring(line, "ip address ") {
			hasIP = true
			// Could parse the IP here if needed
			break
		}
	}

	if !hasIP {
		resp.State.RemoveResource(ctx)
		return
	}

	data.DHCP = types.BoolValue(isDHCP)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InterfaceIPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data InterfaceIPResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	// Validate configuration
	if err := r.validateConfig(&data); err != nil {
		resp.Diagnostics.AddError("Invalid Configuration", err.Error())
		return
	}

	// Build configuration commands
	commands := []string{
		fmt.Sprintf("interface %s", data.Interface.ValueString()),
	}

	if data.DHCP.ValueBool() {
		commands = append(commands, "ip address dhcp")
	} else {
		commands = append(commands, fmt.Sprintf("ip address %s %s",
			data.IPAddress.ValueString(),
			data.SubnetMask.ValueString()))
	}

	commands = append(commands, "end")

	// Execute configuration
	err := r.client.ExecuteConfigCommands(commands)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Interface IP",
			fmt.Sprintf("Could not update IP on interface %s: %s",
				data.Interface.ValueString(), err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *InterfaceIPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data InterfaceIPResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	// Remove IP address from interface
	commands := []string{
		fmt.Sprintf("interface %s", data.Interface.ValueString()),
		"no ip address",
		"end",
	}

	err := r.client.ExecuteConfigCommands(commands)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Removing Interface IP",
			fmt.Sprintf("Could not remove IP from interface %s: %s",
				data.Interface.ValueString(), err.Error()),
		)
		return
	}
}

func (r *InterfaceIPResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import expects interface name (e.g., "Vlan1")
	resource.ImportStatePassthroughID(ctx, path.Root("interface"), req, resp)
}

// Helper methods

func (r *InterfaceIPResource) validateConfig(data *InterfaceIPResourceModel) error {
	isDHCP := data.DHCP.ValueBool()
	hasStatic := !data.IPAddress.IsNull() && data.IPAddress.ValueString() != ""

	if isDHCP && hasStatic {
		return fmt.Errorf("ip_address and dhcp are mutually exclusive")
	}

	if !isDHCP && !hasStatic {
		return fmt.Errorf("either ip_address or dhcp must be specified")
	}

	if hasStatic && (data.SubnetMask.IsNull() || data.SubnetMask.ValueString() == "") {
		return fmt.Errorf("subnet_mask is required when ip_address is specified")
	}

	return nil
}

func splitConfigLines(s string) []string {
	var lines []string
	current := ""
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, current)
			current = ""
		} else if s[i] == '\r' {
			continue
		} else {
			current += string(s[i])
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findInStr(s, substr)
}

func findInStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
