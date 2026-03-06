package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
)

var _ resource.Resource = &WriteMemoryResource{}

func NewWriteMemoryResource() resource.Resource {
	return &WriteMemoryResource{}
}

type WriteMemoryResource struct {
	client *client.Client
}

type WriteMemoryResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Triggers types.Map    `tfsdk:"triggers"`
}

func (r *WriteMemoryResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_write_memory"
}

func (r *WriteMemoryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Saves the running-config to startup-config ('write memory'). " +
			"Place this resource at the end of your configuration with depends_on pointing " +
			"to all other cisco_* resources. Changes to triggers force a re-run.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Always \"write_memory\".",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"triggers": schema.MapAttribute{
				Description: "Map of arbitrary key/value pairs. Any change forces write memory to run again. " +
					"Use this to re-save after specific resource changes.",
				ElementType: types.StringType,
				Optional:    true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *WriteMemoryResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *WriteMemoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data WriteMemoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.client.Lock()
	defer r.client.Unlock()

	if err := r.client.WriteMemory(); err != nil {
		resp.Diagnostics.AddError("Error Writing Memory", err.Error())
		return
	}

	data.ID = types.StringValue("write_memory")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read is a no-op — write memory is an action, not persistent state.
func (r *WriteMemoryResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

// Update is never called because triggers uses RequiresReplace.
func (r *WriteMemoryResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

// Delete is a no-op — there is nothing to undo.
func (r *WriteMemoryResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
