package resources

// cisco_acl_rule is a purely local resource: it stores a single ACE (access control entry)
// definition and exposes it as a human-readable ID (the IOS ACE command string).
// It never writes to the switch — rules are applied in order by cisco_acl.
//
// Because the ID encodes the rule content, every attribute uses RequiresReplace.
// Changing any attribute destroys this resource and creates a new one with a new ID,
// which cisco_acl detects as a change to its rules list.

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &ACLRuleResource{}

func NewACLRuleResource() resource.Resource {
	return &ACLRuleResource{}
}

// ACLRuleResource has no client — it never touches the switch.
type ACLRuleResource struct{}

type ACLRuleResourceModel struct {
	Action              types.String `tfsdk:"action"`
	Protocol            types.String `tfsdk:"protocol"`
	Source              types.String `tfsdk:"source"`
	SourceWildcard      types.String `tfsdk:"source_wildcard"`
	Destination         types.String `tfsdk:"destination"`
	DestinationWildcard types.String `tfsdk:"destination_wildcard"`
	SrcPort             types.String `tfsdk:"src_port"`
	DstPort             types.String `tfsdk:"dst_port"`
	Log                 types.Bool   `tfsdk:"log"`
	ID                  types.String `tfsdk:"id"`
}

func (r *ACLRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acl_rule"
}

func (r *ACLRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rr := planmodifier.String(stringplanmodifier.RequiresReplace())
	rb := planmodifier.Bool(boolplanmodifier.RequiresReplace())

	resp.Schema = schema.Schema{
		Description: "Defines a single ACE (access control entry). Does not write to the switch — reference this resource's ID in cisco_acl.rules to apply it. The ID is the IOS ACE command string, making it human-readable in terraform plan. Any attribute change forces a new resource (and therefore a new ID), which cisco_acl detects as a rule change.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The IOS ACE command string (e.g. \"permit ip 192.168.100.0 0.0.0.255 any\"). Use this as an entry in cisco_acl.rules.",
			},
			"action": schema.StringAttribute{
				Description: "\"permit\" to allow matching traffic, \"deny\" to block it.",
				Required:    true,
				PlanModifiers: []planmodifier.String{rr},
			},
			"protocol": schema.StringAttribute{
				Description: "IP protocol: \"ip\" (all), \"tcp\", \"udp\", or \"icmp\". Default: \"ip\".",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("ip"),
				PlanModifiers: []planmodifier.String{rr},
			},
			"source": schema.StringAttribute{
				Description: "Source address: \"any\", a host IP (e.g. \"10.0.0.1\"), or a network IP (e.g. \"192.168.100.0\" — pair with source_wildcard).",
				Required:    true,
				PlanModifiers: []planmodifier.String{rr},
			},
			"source_wildcard": schema.StringAttribute{
				Description: "Wildcard mask for the source network (e.g. \"0.0.0.255\" for /24). Omit when source is \"any\" or a single host.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{rr},
			},
			"destination": schema.StringAttribute{
				Description: "Destination address, same format as source. Required for extended ACLs.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{rr},
			},
			"destination_wildcard": schema.StringAttribute{
				Description: "Wildcard mask for the destination network.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{rr},
			},
			"src_port": schema.StringAttribute{
				Description: "Source port match expression for TCP/UDP (e.g. \"eq 80\", \"range 8000 8080\").",
				Optional:    true,
				PlanModifiers: []planmodifier.String{rr},
			},
			"dst_port": schema.StringAttribute{
				Description: "Destination port match expression for TCP/UDP (e.g. \"eq 443\", \"eq www\").",
				Optional:    true,
				PlanModifiers: []planmodifier.String{rr},
			},
			"log": schema.BoolAttribute{
				Description: "Log packets that match this rule. Default: false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{rb},
			},
		},
	}
}

// Configure is a no-op: this resource needs no provider client.
func (r *ACLRuleResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {
}

func (r *ACLRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ACLRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = types.StringValue(aceFragment(data))
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read is a no-op: the rule lives only in Terraform state.
func (r *ACLRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Nothing to read from the switch — state is always the source of truth.
}

// Update is never called because every attribute uses RequiresReplace.
func (r *ACLRuleResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

// Delete is a no-op: removing a rule only changes Terraform state.
func (r *ACLRuleResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// aceFragment builds the IOS ACE command string (without a sequence number prefix).
// This is used as the resource ID and as the value referenced in cisco_acl.rules.
func aceFragment(d ACLRuleResourceModel) string {
	ace := fmt.Sprintf("%s %s %s",
		d.Action.ValueString(),
		d.Protocol.ValueString(),
		aclAddrSpec(d.Source.ValueString(), d.SourceWildcard.ValueString()),
	)

	if v := d.SrcPort.ValueString(); v != "" {
		ace += " " + v
	}

	if v := d.Destination.ValueString(); v != "" {
		ace += " " + aclAddrSpec(d.Destination.ValueString(), d.DestinationWildcard.ValueString())
	}

	if v := d.DstPort.ValueString(); v != "" {
		ace += " " + v
	}

	if d.Log.ValueBool() {
		ace += " log"
	}
	return ace
}

// aclAddrSpec converts an address + optional wildcard into an IOS ACL address specifier.
func aclAddrSpec(addr, wildcard string) string {
	if addr == "any" {
		return "any"
	}
	if wildcard == "" || wildcard == "0.0.0.0" {
		return "host " + addr
	}
	return addr + " " + wildcard
}
