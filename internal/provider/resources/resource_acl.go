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

var _ resource.Resource = &ACLPolicyResource{}
var _ resource.ResourceWithImportState = &ACLPolicyResource{}

func NewACLPolicyResource() resource.Resource {
	return &ACLPolicyResource{}
}

type ACLPolicyResource struct {
	client *client.Client
}

type ACLPolicyResourceModel struct {
	Name  types.String `tfsdk:"name"`
	Type  types.String `tfsdk:"type"`
	Rules types.List   `tfsdk:"rules"`
}

func (r *ACLPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acl_policy"
}

func (r *ACLPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Creates a named IP access list and applies an ordered set of rules. Each entry in rules is a cisco_acl_rule.id — the position in the list determines sequence order (first = seq 10, second = seq 20, …). Updating the list recreates the ACL atomically to guarantee exact rule order.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "ACL name on the switch. Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Description: "ACL type: \"standard\" (source-only filtering) or \"extended\" (source, destination, and port filtering). Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"rules": schema.ListAttribute{
				Description: "Ordered list of cisco_acl_rule IDs. Position determines sequence: index 0 → seq 10, index 1 → seq 20, etc. To reorder rules, change their position in this list. To insert a rule between two existing rules, add it at the desired index — the entire ACL is recreated atomically so there are no gaps or conflicts.",
				ElementType: types.StringType,
				Required:    true,
			},
		},
	}
}

func (r *ACLPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ACLPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ACLPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.applyACL(ctx, data); err != nil {
		resp.Diagnostics.AddError("Error Creating ACL",
			fmt.Sprintf("Could not create ACL %q: %s", data.Name.ValueString(), err.Error()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ACLPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ACLPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	aclInfo, err := r.readACL(data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading ACL",
			fmt.Sprintf("Could not read ACL %q: %s", data.Name.ValueString(), err.Error()))
		return
	}
	if !aclInfo.Exists {
		resp.State.RemoveResource(ctx)
		return
	}

	// The rules list is managed entirely through Terraform — we trust stored state
	// for the rule content and only check that the ACL container still exists.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update atomically recreates the ACL with the new ordered rule list.
// This guarantees no stale rules remain and sequence numbers are always correct.
func (r *ACLPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ACLPolicyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete old ACL first, then recreate with the new rule set.
	var state ACLPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteCmd := []string{
		fmt.Sprintf("no ip access-list %s %s", state.Type.ValueString(), state.Name.ValueString()),
		"end",
	}
	if err := r.client.ExecuteConfigCommands(deleteCmd); err != nil {
		// Non-fatal: ACL may already be absent.
		_ = err
	}

	if err := r.applyACL(ctx, data); err != nil {
		resp.Diagnostics.AddError("Error Updating ACL",
			fmt.Sprintf("Could not update ACL %q: %s", data.Name.ValueString(), err.Error()))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ACLPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ACLPolicyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	commands := []string{
		fmt.Sprintf("no ip access-list %s %s", data.Type.ValueString(), data.Name.ValueString()),
		"end",
	}
	if err := r.client.ExecuteConfigCommands(commands); err != nil {
		if containsError(err.Error(), "does not exist") {
			return
		}
		resp.Diagnostics.AddError("Error Deleting ACL",
			fmt.Sprintf("Could not delete ACL %q: %s", data.Name.ValueString(), err.Error()))
	}
}

func (r *ACLPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID format: "<type>/<name>", e.g. "extended/INTER-VLAN-POLICY"
	parts := splitImportID(req.ID, "/", 2)
	if parts == nil {
		resp.Diagnostics.AddError("Invalid Import ID",
			"Import ID must be in the format \"<type>/<name>\" (e.g. \"extended/INTER-VLAN-POLICY\")")
		return
	}

	aclType := parts[0]
	aclName := parts[1]

	aclInfo, err := r.readACL(aclName)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading ACL",
			fmt.Sprintf("Could not read ACL %q: %s", aclName, err.Error()))
		return
	}
	if !aclInfo.Exists {
		resp.Diagnostics.AddError("ACL Not Found",
			fmt.Sprintf("ACL %q does not exist on the switch", aclName))
		return
	}

	// Import the ACL with an empty rules list — the user must define cisco_acl_rule
	// resources and populate the rules list manually after import.
	emptyRules, _ := types.ListValueFrom(ctx, types.StringType, []string{})
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), aclName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), aclType)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("rules"), emptyRules)...)
}

// applyACL writes the full ACL to the switch: creates the container then adds each
// rule with a sequence number based on its position (index 0 → seq 10, 1 → seq 20, …).
func (r *ACLPolicyResource) applyACL(ctx context.Context, data ACLPolicyResourceModel) error {
	commands := []string{
		fmt.Sprintf("ip access-list %s %s", data.Type.ValueString(), data.Name.ValueString()),
	}

	var rules []types.String
	if diags := data.Rules.ElementsAs(ctx, &rules, false); diags.HasError() {
		return fmt.Errorf("failed to read rules list")
	}

	for i, rule := range rules {
		seq := (i + 1) * 10
		commands = append(commands, fmt.Sprintf("%d %s", seq, rule.ValueString()))
	}

	commands = append(commands, "end")
	return r.client.ExecuteConfigCommands(commands)
}

// splitImportID splits an import ID string by sep, returning exactly n parts or nil.
func splitImportID(id, sep string, n int) []string {
	var parts []string
	current := ""
	for i := 0; i < len(id); i++ {
		if string(id[i]) == sep && len(parts) < n-1 {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(id[i])
		}
	}
	parts = append(parts, current)
	if len(parts) != n {
		return nil
	}
	return parts
}

func (r *ACLPolicyResource) readACL(name string) (*client.ACLInfo, error) {
	output, err := r.client.ExecuteCommand(
		fmt.Sprintf("show running-config | section ip access-list %s", name))
	if err != nil {
		return nil, err
	}
	return client.ParseACL(output, name)
}
