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

var _ resource.Resource = &SNMPCommunityResource{}
var _ resource.ResourceWithImportState = &SNMPCommunityResource{}

func NewSNMPCommunityResource() resource.Resource {
	return &SNMPCommunityResource{}
}

type SNMPCommunityResource struct {
	client *client.Client
}

type SNMPCommunityResourceModel struct {
	Name   types.String `tfsdk:"name"`
	Access types.String `tfsdk:"access"`
	ACL    types.String `tfsdk:"acl"`
}

func (r *SNMPCommunityResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_snmp_community"
}

func (r *SNMPCommunityResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a single SNMP community string on the switch. Create one cisco_snmp_community per community you want to define (e.g. a read-only community for Prometheus polling).",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "Community string value. Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"access": schema.StringAttribute{
				Description: "Access level: \"ro\" (read-only, for Prometheus polling) or \"rw\" (read-write).",
				Required:    true,
			},
			"acl": schema.StringAttribute{
				Description: "Optional standard ACL name or number to restrict which hosts may use this community string.",
				Optional:    true,
			},
		},
	}
}

func (r *SNMPCommunityResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SNMPCommunityResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SNMPCommunityResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	if err := r.client.ExecuteConfigCommands(r.buildCommands(data)); err != nil {
		resp.Diagnostics.AddError("Error Creating SNMP Community",
			fmt.Sprintf("Could not create SNMP community %q: %s", data.Name.ValueString(), err.Error()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SNMPCommunityResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SNMPCommunityResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	info, err := r.readCommunity(data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading SNMP Community",
			fmt.Sprintf("Could not read SNMP community %q: %s", data.Name.ValueString(), err.Error()))
		return
	}
	if !info.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	data.Access = types.StringValue(info.Access)
	if info.ACL != "" {
		data.ACL = types.StringValue(info.ACL)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SNMPCommunityResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data SNMPCommunityResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	if err := r.client.ExecuteConfigCommands(r.buildCommands(data)); err != nil {
		resp.Diagnostics.AddError("Error Updating SNMP Community",
			fmt.Sprintf("Could not update SNMP community %q: %s", data.Name.ValueString(), err.Error()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SNMPCommunityResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SNMPCommunityResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	commands := []string{
		fmt.Sprintf("no snmp-server community %s", data.Name.ValueString()),
		"end",
	}
	if err := r.client.ExecuteConfigCommands(commands); err != nil {
		if containsError(err.Error(), "does not exist") {
			return
		}
		resp.Diagnostics.AddError("Error Deleting SNMP Community",
			fmt.Sprintf("Could not delete SNMP community %q: %s", data.Name.ValueString(), err.Error()))
	}
}

func (r *SNMPCommunityResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	info, err := r.readCommunity(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading SNMP Community",
			fmt.Sprintf("Could not read SNMP community %q: %s", req.ID, err.Error()))
		return
	}
	if !info.Exists {
		resp.Diagnostics.AddError("SNMP Community Not Found",
			fmt.Sprintf("SNMP community %q does not exist on the switch", req.ID))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), info.Name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("access"), info.Access)...)
	if info.ACL != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("acl"), info.ACL)...)
	}
}

func (r *SNMPCommunityResource) buildCommands(data SNMPCommunityResourceModel) []string {
	cmd := fmt.Sprintf("snmp-server community %s %s", data.Name.ValueString(), data.Access.ValueString())
	if !data.ACL.IsNull() && data.ACL.ValueString() != "" {
		cmd += " " + data.ACL.ValueString()
	}
	return []string{cmd, "end"}
}

func (r *SNMPCommunityResource) readCommunity(name string) (*client.SNMPCommunityInfo, error) {
	output, err := r.client.ExecuteCommand("show running-config | include snmp-server community")
	if err != nil {
		return nil, err
	}
	return client.ParseSNMPCommunity(output, name)
}
