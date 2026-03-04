package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
)

var _ datasource.DataSource = &InterfacesDataSource{}
var _ datasource.DataSourceWithConfigure = &InterfacesDataSource{}

func NewInterfacesDataSource() datasource.DataSource {
	return &InterfacesDataSource{}
}

type InterfacesDataSource struct {
	client *client.Client
}

type InterfacesDataSourceModel struct {
	Interfaces []InterfaceStatusModel `tfsdk:"interfaces"`
}

type InterfaceStatusModel struct {
	Name        types.String `tfsdk:"name"`
	ShortName   types.String `tfsdk:"short_name"`
	Description types.String `tfsdk:"description"`
	Status      types.String `tfsdk:"status"`
	Vlan        types.String `tfsdk:"vlan"`
	Duplex      types.String `tfsdk:"duplex"`
	Speed       types.String `tfsdk:"speed"`
	MediaType   types.String `tfsdk:"media_type"`
}

func (d *InterfacesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_interfaces"
}

func (d *InterfacesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads all switchport interfaces from the Cisco switch via 'show interfaces status'.",
		Attributes: map[string]schema.Attribute{
			"interfaces": schema.ListNestedAttribute{
				Computed:    true,
				Description: "List of interfaces reported by 'show interfaces status'.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:    true,
							Description: "Full interface name (e.g., GigabitEthernet1/0/1).",
						},
						"short_name": schema.StringAttribute{
							Computed:    true,
							Description: "Abbreviated interface name as shown on the switch (e.g., Gi1/0/1).",
						},
						"description": schema.StringAttribute{
							Computed:    true,
							Description: "Port description/name.",
						},
						"status": schema.StringAttribute{
							Computed:    true,
							Description: "Port status: connected, notconnect, disabled, err-disabled, etc.",
						},
						"vlan": schema.StringAttribute{
							Computed:    true,
							Description: "Access VLAN ID, 'trunk', or 'routed'.",
						},
						"duplex": schema.StringAttribute{
							Computed:    true,
							Description: "Duplex mode (e.g., a-full, full, auto).",
						},
						"speed": schema.StringAttribute{
							Computed:    true,
							Description: "Port speed (e.g., a-100, 1000, auto).",
						},
						"media_type": schema.StringAttribute{
							Computed:    true,
							Description: "Interface media type (e.g., 10/100/1000BaseTX, SFP).",
						},
					},
				},
			},
		},
	}
}

func (d *InterfacesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}

	d.client = c
}

func (d *InterfacesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	output, err := d.client.ExecuteCommand("show interfaces status")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Interfaces",
			fmt.Sprintf("Could not run 'show interfaces status': %s", err),
		)
		return
	}

	infos, err := client.ParseShowInterfacesStatus(output)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Parsing Interface Status",
			fmt.Sprintf("Could not parse 'show interfaces status' output: %s", err),
		)
		return
	}

	var state InterfacesDataSourceModel
	for _, info := range infos {
		state.Interfaces = append(state.Interfaces, InterfaceStatusModel{
			Name:        types.StringValue(info.Name),
			ShortName:   types.StringValue(info.ShortName),
			Description: types.StringValue(info.Description),
			Status:      types.StringValue(info.Status),
			Vlan:        types.StringValue(info.Vlan),
			Duplex:      types.StringValue(info.Duplex),
			Speed:       types.StringValue(info.Speed),
			MediaType:   types.StringValue(info.MediaType),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
