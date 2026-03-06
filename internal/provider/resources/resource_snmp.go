package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
)

var _ resource.Resource = &SNMPResource{}

func NewSNMPResource() resource.Resource {
	return &SNMPResource{}
}

type SNMPResource struct {
	client *client.Client
}

type SNMPResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Location      types.String `tfsdk:"location"`
	Contact       types.String `tfsdk:"contact"`
	TrapCommunity types.String `tfsdk:"trap_community"`
	TrapVersion   types.String `tfsdk:"trap_version"`
	TrapHosts     types.List   `tfsdk:"trap_hosts"`
}

func (r *SNMPResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_snmp"
}

func (r *SNMPResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages global SNMP server settings: system location, contact, and trap destinations. Use cisco_snmp_community for community strings. Only one cisco_snmp resource should exist per switch.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Always \"snmp\" — this is a singleton resource.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"location": schema.StringAttribute{
				Description: "Physical location of the switch (snmp-server location). Shown in SNMP system MIB.",
				Optional:    true,
			},
			"contact": schema.StringAttribute{
				Description: "Contact information for the switch administrator (snmp-server contact). Shown in SNMP system MIB.",
				Optional:    true,
			},
			"trap_community": schema.StringAttribute{
				Description: "Community string to use when sending SNMP traps. Required when trap_hosts is set.",
				Optional:    true,
			},
			"trap_version": schema.StringAttribute{
				Description: "SNMP version for traps: \"1\" or \"2c\" (default: \"2c\"). SNMPv2c traps carry more information and are preferred for Alertmanager.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("2c"),
			},
			"trap_hosts": schema.ListAttribute{
				Description: "IP addresses of trap receivers (snmp-server host). Point these at your Alertmanager SNMP trap receiver or an snmptrapd instance that forwards to Alertmanager.",
				ElementType: types.StringType,
				Optional:    true,
			},
		},
	}
}

func (r *SNMPResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SNMPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SNMPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	commands, err := r.buildApplyCommands(ctx, data)
	if err != nil {
		resp.Diagnostics.AddError("Error Building SNMP Commands", err.Error())
		return
	}

	if err := r.client.ExecuteConfigCommands(commands); err != nil {
		resp.Diagnostics.AddError("Error Creating SNMP Config", err.Error())
		return
	}

	data.ID = types.StringValue("snmp")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SNMPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SNMPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	info, err := r.readSNMP()
	if err != nil {
		resp.Diagnostics.AddError("Error Reading SNMP Config", err.Error())
		return
	}
	if !info.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	r.updateModelFromInfo(ctx, &data, info)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SNMPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SNMPResourceModel
	var state SNMPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	// Build cleanup commands for anything being removed or changed.
	cleanup := r.buildCleanupCommands(ctx, plan, state)

	// Build apply commands for the new desired state.
	apply, err := r.buildApplyCommands(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error Building SNMP Commands", err.Error())
		return
	}

	// Merge: cleanup first, then apply, with a single "end" at the tail.
	all := append(cleanup, apply[:len(apply)-1]...) // strip "end" from apply
	all = append(all, "end")

	if err := r.client.ExecuteConfigCommands(all); err != nil {
		resp.Diagnostics.AddError("Error Updating SNMP Config", err.Error())
		return
	}

	plan.ID = types.StringValue("snmp")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SNMPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SNMPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	commands := []string{}

	if !data.Location.IsNull() && data.Location.ValueString() != "" {
		commands = append(commands, "no snmp-server location")
	}
	if !data.Contact.IsNull() && data.Contact.ValueString() != "" {
		commands = append(commands, "no snmp-server contact")
	}

	var hosts []types.String
	if !data.TrapHosts.IsNull() {
		data.TrapHosts.ElementsAs(context.Background(), &hosts, false)
	}
	for _, h := range hosts {
		commands = append(commands, fmt.Sprintf("no snmp-server host %s", h.ValueString()))
	}

	if len(commands) == 0 {
		return
	}
	commands = append(commands, "end")

	if err := r.client.ExecuteConfigCommands(commands); err != nil {
		resp.Diagnostics.AddError("Error Deleting SNMP Config", err.Error())
	}
}

// buildApplyCommands builds the commands to apply the desired state.
// The last element is always "end".
func (r *SNMPResource) buildApplyCommands(ctx context.Context, data SNMPResourceModel) ([]string, error) {
	var commands []string

	if !data.Location.IsNull() && data.Location.ValueString() != "" {
		commands = append(commands, fmt.Sprintf("snmp-server location %s", data.Location.ValueString()))
	}
	if !data.Contact.IsNull() && data.Contact.ValueString() != "" {
		commands = append(commands, fmt.Sprintf("snmp-server contact %s", data.Contact.ValueString()))
	}

	if !data.TrapHosts.IsNull() && len(data.TrapHosts.Elements()) > 0 {
		if data.TrapCommunity.IsNull() || data.TrapCommunity.ValueString() == "" {
			return nil, fmt.Errorf("trap_community is required when trap_hosts is set")
		}
		var hosts []types.String
		data.TrapHosts.ElementsAs(ctx, &hosts, false)
		version := data.TrapVersion.ValueString()
		community := data.TrapCommunity.ValueString()
		for _, h := range hosts {
			commands = append(commands, fmt.Sprintf("snmp-server host %s version %s %s",
				h.ValueString(), version, community))
		}
		commands = append(commands, "snmp-server enable traps")
	}

	if len(commands) == 0 {
		return []string{"end"}, nil
	}
	commands = append(commands, "end")
	return commands, nil
}

// buildCleanupCommands returns commands to remove settings that are being cleared or changed.
// "end" is NOT appended — it will be added by the caller.
func (r *SNMPResource) buildCleanupCommands(ctx context.Context, plan, state SNMPResourceModel) []string {
	var commands []string

	// Remove location if it was set and is now cleared
	oldLoc := state.Location.ValueString()
	newLoc := plan.Location.ValueString()
	if oldLoc != "" && newLoc == "" {
		commands = append(commands, "no snmp-server location")
	}

	// Remove contact if it was set and is now cleared
	oldCon := state.Contact.ValueString()
	newCon := plan.Contact.ValueString()
	if oldCon != "" && newCon == "" {
		commands = append(commands, "no snmp-server contact")
	}

	// Remove old trap hosts not present in the new plan
	var oldHosts []types.String
	if !state.TrapHosts.IsNull() {
		state.TrapHosts.ElementsAs(ctx, &oldHosts, false)
	}
	var newHosts []types.String
	if !plan.TrapHosts.IsNull() {
		plan.TrapHosts.ElementsAs(ctx, &newHosts, false)
	}
	newHostSet := make(map[string]bool)
	for _, h := range newHosts {
		newHostSet[h.ValueString()] = true
	}
	for _, h := range oldHosts {
		if !newHostSet[h.ValueString()] {
			commands = append(commands, fmt.Sprintf("no snmp-server host %s", h.ValueString()))
		}
	}

	return commands
}

func (r *SNMPResource) readSNMP() (*client.SNMPInfo, error) {
	output, err := r.client.ExecuteCommand("show running-config | include snmp-server")
	if err != nil {
		return nil, err
	}
	return client.ParseSNMP(output)
}

func (r *SNMPResource) updateModelFromInfo(ctx context.Context, data *SNMPResourceModel, info *client.SNMPInfo) {
	data.ID = types.StringValue("snmp")
	if info.Location != "" {
		data.Location = types.StringValue(info.Location)
	}
	if info.Contact != "" {
		data.Contact = types.StringValue(info.Contact)
	}
	if info.TrapCommunity != "" {
		data.TrapCommunity = types.StringValue(info.TrapCommunity)
	}
	if info.TrapVersion != "" {
		data.TrapVersion = types.StringValue(info.TrapVersion)
	}
	if len(info.TrapHosts) > 0 {
		elems := make([]types.String, len(info.TrapHosts))
		for i, h := range info.TrapHosts {
			elems[i] = types.StringValue(h)
		}
		list, _ := types.ListValueFrom(ctx, types.StringType, elems)
		data.TrapHosts = list
	}
}
