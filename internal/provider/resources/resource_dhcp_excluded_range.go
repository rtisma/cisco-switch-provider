package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
)

var _ resource.Resource = &DHCPExcludedRangeResource{}
var _ resource.ResourceWithImportState = &DHCPExcludedRangeResource{}

func NewDHCPExcludedRangeResource() resource.Resource {
	return &DHCPExcludedRangeResource{}
}

type DHCPExcludedRangeResource struct {
	client *client.Client
}

type DHCPExcludedRangeResourceModel struct {
	LowAddress  types.String `tfsdk:"low_address"`
	HighAddress types.String `tfsdk:"high_address"`
}

func (r *DHCPExcludedRangeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dhcp_excluded_range"
}

func (r *DHCPExcludedRangeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a DHCP excluded-address range on a Cisco switch " +
			"(ip dhcp excluded-address). Addresses in the excluded range are never " +
			"offered as DHCP leases. Changing either address forces a new resource.",
		Attributes: map[string]schema.Attribute{
			"low_address": schema.StringAttribute{
				Description: "First (lowest) IP address in the excluded range. Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"high_address": schema.StringAttribute{
				Description: "Last (highest) IP address in the excluded range. When omitted, only low_address is excluded. " +
					"Computed: after apply this is always set (equals low_address for single-address exclusions). " +
					"Changing this forces a new resource.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *DHCPExcludedRangeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DHCPExcludedRangeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DHCPExcludedRangeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	low := data.LowAddress.ValueString()
	high := data.HighAddress.ValueString()
	if high == "" {
		high = low
	}

	if err := r.client.ExecuteConfigCommands(r.buildCmd(low, high)); err != nil {
		resp.Diagnostics.AddError(
			"Error Creating DHCP Excluded Range",
			fmt.Sprintf("Could not exclude DHCP range %s–%s: %s", low, high, err.Error()),
		)
		return
	}

	info, err := r.readExcludedRange(low, high)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading DHCP Excluded Range",
			fmt.Sprintf("Could not read excluded range after creation: %s", err.Error()))
		return
	}
	if !info.Exists {
		resp.Diagnostics.AddError("DHCP Excluded Range Not Found",
			fmt.Sprintf("Excluded range %s–%s was not found after creation", low, high))
		return
	}

	data.LowAddress = types.StringValue(info.LowAddress)
	data.HighAddress = types.StringValue(info.HighAddress)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DHCPExcludedRangeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DHCPExcludedRangeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	low := data.LowAddress.ValueString()
	high := data.HighAddress.ValueString()
	if high == "" {
		high = low
	}

	info, err := r.readExcludedRange(low, high)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading DHCP Excluded Range",
			fmt.Sprintf("Could not read excluded range %s–%s: %s", low, high, err.Error()))
		return
	}
	if !info.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	data.LowAddress = types.StringValue(info.LowAddress)
	data.HighAddress = types.StringValue(info.HighAddress)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update is never called because all attributes use RequiresReplace.
func (r *DHCPExcludedRangeResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *DHCPExcludedRangeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data DHCPExcludedRangeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	low := data.LowAddress.ValueString()
	high := data.HighAddress.ValueString()

	cmds := r.buildCmd(low, high)
	cmds[0] = "no " + cmds[0]
	if err := r.client.ExecuteConfigCommands(cmds); err != nil {
		if containsError(err.Error(), "does not exist") || containsError(err.Error(), "not found") {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting DHCP Excluded Range",
			fmt.Sprintf("Could not remove excluded range %s–%s: %s", low, high, err.Error()),
		)
	}
}

func (r *DHCPExcludedRangeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID: "<low>-<high>" for a range, or "<low>" for a single address.
	// IPv4 addresses never contain hyphens, so splitting on "-" is unambiguous.
	var low, high string
	if idx := strings.Index(req.ID, "-"); idx > 0 {
		low = req.ID[:idx]
		high = req.ID[idx+1:]
	} else {
		low = req.ID
		high = low
	}

	info, err := r.readExcludedRange(low, high)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading DHCP Excluded Range",
			fmt.Sprintf("Could not read excluded range %s–%s: %s", low, high, err.Error()))
		return
	}
	if !info.Exists {
		resp.Diagnostics.AddError("DHCP Excluded Range Not Found",
			fmt.Sprintf("Excluded range %s–%s does not exist on the switch", low, high))
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("low_address"), info.LowAddress)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("high_address"), info.HighAddress)...)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// buildCmd returns the ip dhcp excluded-address command + "end".
func (r *DHCPExcludedRangeResource) buildCmd(low, high string) []string {
	cmd := fmt.Sprintf("ip dhcp excluded-address %s", low)
	if high != "" && high != low {
		cmd += " " + high
	}
	return []string{cmd, "end"}
}

func (r *DHCPExcludedRangeResource) readExcludedRange(low, high string) (*client.DHCPExcludedRangeInfo, error) {
	output, err := r.client.ExecuteCommand("show running-config | include ip dhcp excluded")
	if err != nil {
		return nil, err
	}
	return client.ParseDHCPExcludedRange(output, low, high)
}
