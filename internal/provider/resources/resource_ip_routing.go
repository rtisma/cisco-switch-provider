package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
)

var _ resource.Resource = &IPRoutingResource{}

func NewIPRoutingResource() resource.Resource {
	return &IPRoutingResource{}
}

type IPRoutingResource struct {
	client *client.Client
}

type IPRoutingResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Enabled types.Bool   `tfsdk:"enabled"`
}

func (r *IPRoutingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_routing"
}

func (r *IPRoutingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the global 'ip routing' setting on a Cisco switch. " +
			"Enable this to allow the switch to perform Layer 3 inter-VLAN routing. " +
			"Only one cisco_ip_routing resource should exist per switch.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Always \"ip_routing\" — this is a singleton resource.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether IP routing is enabled on the switch (default: true).",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
		},
	}
}

func (r *IPRoutingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *IPRoutingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data IPRoutingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	if err := r.client.ExecuteConfigCommands(r.buildCommands(data.Enabled.ValueBool())); err != nil {
		resp.Diagnostics.AddError(
			"Error Configuring IP Routing",
			fmt.Sprintf("Could not configure ip routing: %s", err.Error()),
		)
		return
	}

	data.ID = types.StringValue("ip_routing")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IPRoutingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data IPRoutingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	enabled, err := r.readIPRouting()
	if err != nil {
		resp.Diagnostics.AddError("Error Reading IP Routing",
			fmt.Sprintf("Could not read ip routing state: %s", err.Error()))
		return
	}

	data.ID = types.StringValue("ip_routing")
	data.Enabled = types.BoolValue(enabled)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IPRoutingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data IPRoutingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	if err := r.client.ExecuteConfigCommands(r.buildCommands(data.Enabled.ValueBool())); err != nil {
		resp.Diagnostics.AddError(
			"Error Updating IP Routing",
			fmt.Sprintf("Could not update ip routing: %s", err.Error()),
		)
		return
	}

	data.ID = types.StringValue("ip_routing")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *IPRoutingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	r.client.Lock()
	defer r.client.Unlock()

	if err := r.client.ExecuteConfigCommands([]string{"no ip routing", "end"}); err != nil {
		resp.Diagnostics.AddError(
			"Error Removing IP Routing",
			fmt.Sprintf("Could not disable ip routing: %s", err.Error()),
		)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (r *IPRoutingResource) buildCommands(enabled bool) []string {
	if enabled {
		return []string{"ip routing", "end"}
	}
	return []string{"no ip routing", "end"}
}

func (r *IPRoutingResource) readIPRouting() (bool, error) {
	output, err := r.client.ExecuteCommand("show running-config | include ip routing")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "ip routing" {
			return true, nil
		}
	}
	return false, nil
}
