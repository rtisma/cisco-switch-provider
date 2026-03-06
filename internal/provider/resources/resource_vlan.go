package resources

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ resource.Resource = &VlanResource{}
var _ resource.ResourceWithImportState = &VlanResource{}

// NewVlanResource creates a new VLAN resource
func NewVlanResource() resource.Resource {
	return &VlanResource{}
}

// VlanResource defines the resource implementation
type VlanResource struct {
	client *client.Client
}

// VlanResourceModel describes the resource data model
type VlanResourceModel struct {
	VlanID types.Int64  `tfsdk:"vlan_id"`
	Name   types.String `tfsdk:"name"`
	State  types.String `tfsdk:"state"`
}

// Metadata returns the resource type name
func (r *VlanResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vlan"
}

// Schema defines the schema for the resource
func (r *VlanResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a VLAN on a Cisco switch",
		Attributes: map[string]schema.Attribute{
			"vlan_id": schema.Int64Attribute{
				Description: "VLAN ID (1-4094)",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "VLAN name",
				Required:    true,
			},
			"state": schema.StringAttribute{
				Description: "VLAN state: active or suspend (default: active)",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("active"),
			},
		},
	}
}

// Configure adds the provider configured client to the resource
func (r *VlanResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create creates the VLAN on the switch
func (r *VlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data VlanResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	// Validate VLAN ID
	vlanID := int(data.VlanID.ValueInt64())
	if vlanID < 1 || vlanID > 4094 {
		resp.Diagnostics.AddError(
			"Invalid VLAN ID",
			fmt.Sprintf("VLAN ID must be between 1 and 4094, got: %d", vlanID),
		)
		return
	}

	// Build configuration commands
	commands := []string{
		fmt.Sprintf("vlan %d", vlanID),
		fmt.Sprintf("name %s", data.Name.ValueString()),
	}

	// Add state if not default
	if !data.State.IsNull() && data.State.ValueString() != "active" {
		commands = append(commands, fmt.Sprintf("state %s", data.State.ValueString()))
	}

	commands = append(commands, "end")

	// Execute configuration
	err := r.client.ExecuteConfigCommands(commands)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating VLAN",
			fmt.Sprintf("Could not create VLAN %d: %s", vlanID, err.Error()),
		)
		return
	}

	// Read back the VLAN to verify
	vlanInfo, err := r.readVLAN(vlanID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading VLAN",
			fmt.Sprintf("Could not read VLAN %d after creation: %s", vlanID, err.Error()),
		)
		return
	}

	if !vlanInfo.Exists {
		resp.Diagnostics.AddError(
			"VLAN Not Found",
			fmt.Sprintf("VLAN %d was not found after creation", vlanID),
		)
		return
	}

	// Update state with actual values
	data.Name = types.StringValue(vlanInfo.Name)
	data.State = types.StringValue(vlanInfo.State)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read reads the VLAN state from the switch
func (r *VlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data VlanResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vlanID := int(data.VlanID.ValueInt64())

	// Read VLAN from switch
	vlanInfo, err := r.readVLAN(vlanID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading VLAN",
			fmt.Sprintf("Could not read VLAN %d: %s", vlanID, err.Error()),
		)
		return
	}

	if !vlanInfo.Exists {
		// VLAN doesn't exist - remove from state
		resp.State.RemoveResource(ctx)
		return
	}

	// Update state
	data.Name = types.StringValue(vlanInfo.Name)
	data.State = types.StringValue(vlanInfo.State)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update updates the VLAN on the switch
func (r *VlanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data VlanResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	vlanID := int(data.VlanID.ValueInt64())

	// Build configuration commands
	commands := []string{
		fmt.Sprintf("vlan %d", vlanID),
		fmt.Sprintf("name %s", data.Name.ValueString()),
		fmt.Sprintf("state %s", data.State.ValueString()),
		"end",
	}

	// Execute configuration
	err := r.client.ExecuteConfigCommands(commands)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating VLAN",
			fmt.Sprintf("Could not update VLAN %d: %s", vlanID, err.Error()),
		)
		return
	}

	// Read back to verify
	vlanInfo, err := r.readVLAN(vlanID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading VLAN",
			fmt.Sprintf("Could not read VLAN %d after update: %s", vlanID, err.Error()),
		)
		return
	}

	// Update state
	data.Name = types.StringValue(vlanInfo.Name)
	data.State = types.StringValue(vlanInfo.State)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete deletes the VLAN from the switch
func (r *VlanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data VlanResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	vlanID := int(data.VlanID.ValueInt64())

	// Build configuration commands
	commands := []string{
		fmt.Sprintf("no vlan %d", vlanID),
		"end",
	}

	// Execute configuration
	err := r.client.ExecuteConfigCommands(commands)
	if err != nil {
		// Check if error is because VLAN doesn't exist
		if containsError(err.Error(), "does not exist") {
			// VLAN already deleted, that's fine
			return
		}

		resp.Diagnostics.AddError(
			"Error Deleting VLAN",
			fmt.Sprintf("Could not delete VLAN %d: %s", vlanID, err.Error()),
		)
		return
	}
}

// ImportState imports an existing VLAN into Terraform state
func (r *VlanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import expects VLAN ID as string (e.g., "100")
	vlanID, err := strconv.Atoi(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Import ID must be a valid VLAN ID number: %s", err.Error()),
		)
		return
	}

	// Read the VLAN
	vlanInfo, err := r.readVLAN(vlanID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading VLAN",
			fmt.Sprintf("Could not read VLAN %d: %s", vlanID, err.Error()),
		)
		return
	}

	if !vlanInfo.Exists {
		resp.Diagnostics.AddError(
			"VLAN Not Found",
			fmt.Sprintf("VLAN %d does not exist on the switch", vlanID),
		)
		return
	}

	// Set state
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vlan_id"), vlanID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), vlanInfo.Name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("state"), vlanInfo.State)...)
}

// readVLAN reads VLAN information from the switch
func (r *VlanResource) readVLAN(vlanID int) (*client.VLANInfo, error) {
	// Execute show command
	output, err := r.client.ExecuteCommand(fmt.Sprintf("show vlan id %d", vlanID))
	if err != nil {
		return nil, err
	}

	// Parse output
	return client.ParseShowVlan(output, vlanID)
}

func containsError(s, substr string) bool {
	return len(s) >= len(substr) && findInString(s, substr)
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
