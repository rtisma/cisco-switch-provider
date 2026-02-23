package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/example-org/terraform-provider-cisco/internal/provider/client"
	"github.com/example-org/terraform-provider-cisco/internal/provider/resources"
)

// Ensure CiscoProvider satisfies various provider interfaces
var _ provider.Provider = &CiscoProvider{}

// CiscoProvider defines the provider implementation
type CiscoProvider struct {
	version string
}

// CiscoProviderModel describes the provider data model
type CiscoProviderModel struct {
	Host           types.String `tfsdk:"host"`
	Port           types.Int64  `tfsdk:"port"`
	Username       types.String `tfsdk:"username"`
	Password       types.String `tfsdk:"password"`
	EnablePassword types.String `tfsdk:"enable_password"`
	SSHTimeout     types.Int64  `tfsdk:"ssh_timeout"`
	CommandTimeout types.Int64  `tfsdk:"command_timeout"`
}

// New creates a new provider instance
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &CiscoProvider{
			version: version,
		}
	}
}

// Metadata returns the provider metadata
func (p *CiscoProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "cisco"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data
func (p *CiscoProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Terraform provider for managing Cisco WS-C3650 switches via SSH CLI automation",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Description: "Hostname or IP address of the Cisco switch",
				Required:    true,
			},
			"port": schema.Int64Attribute{
				Description: "SSH port (default: 22)",
				Optional:    true,
			},
			"username": schema.StringAttribute{
				Description: "SSH username for authentication",
				Required:    true,
			},
			"password": schema.StringAttribute{
				Description: "SSH password for authentication",
				Required:    true,
				Sensitive:   true,
			},
			"enable_password": schema.StringAttribute{
				Description: "Enable mode password (if different from SSH password)",
				Optional:    true,
				Sensitive:   true,
			},
			"ssh_timeout": schema.Int64Attribute{
				Description: "SSH connection timeout in seconds (default: 30)",
				Optional:    true,
			},
			"command_timeout": schema.Int64Attribute{
				Description: "Command execution timeout in seconds (default: 10)",
				Optional:    true,
			},
		},
	}
}

// Configure prepares a Cisco client for data sources and resources
func (p *CiscoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config CiscoProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Validate required fields
	if config.Host.IsNull() || config.Host.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing Host",
			"The provider cannot create the Cisco client as there is a missing or empty value for the host. "+
				"Set the host value in the provider configuration.",
		)
	}

	if config.Username.IsNull() || config.Username.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing Username",
			"The provider cannot create the Cisco client as there is a missing or empty value for the username. "+
				"Set the username value in the provider configuration.",
		)
	}

	if config.Password.IsNull() || config.Password.ValueString() == "" {
		resp.Diagnostics.AddError(
			"Missing Password",
			"The provider cannot create the Cisco client as there is a missing or empty value for the password. "+
				"Set the password value in the provider configuration.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Set defaults
	port := int(22)
	if !config.Port.IsNull() {
		port = int(config.Port.ValueInt64())
	}

	sshTimeout := 30
	if !config.SSHTimeout.IsNull() {
		sshTimeout = int(config.SSHTimeout.ValueInt64())
	}

	commandTimeout := 10
	if !config.CommandTimeout.IsNull() {
		commandTimeout = int(config.CommandTimeout.ValueInt64())
	}

	enablePassword := ""
	if !config.EnablePassword.IsNull() {
		enablePassword = config.EnablePassword.ValueString()
	}

	// Create client configuration
	clientConfig := client.Config{
		Host:           config.Host.ValueString(),
		Port:           port,
		Username:       config.Username.ValueString(),
		Password:       config.Password.ValueString(),
		EnablePassword: enablePassword,
		SSHTimeout:     time.Duration(sshTimeout) * time.Second,
		CommandTimeout: time.Duration(commandTimeout) * time.Second,
	}

	// Create and connect the client
	ciscoClient := client.NewClient(clientConfig)

	// Test connection
	err := ciscoClient.Connect()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Connect to Cisco Switch",
			fmt.Sprintf("An error occurred while connecting to the Cisco switch: %s", err.Error()),
		)
		return
	}

	// Make the client available to resources and data sources
	resp.DataSourceData = ciscoClient
	resp.ResourceData = ciscoClient
}

// Resources defines the resources implemented in the provider
func (p *CiscoProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewVlanResource,
		resources.NewInterfaceResource,
		resources.NewSVIResource,
		resources.NewInterfaceIPResource,
		resources.NewDHCPPoolResource,
		resources.NewACLPolicyResource,
		resources.NewACLRuleResource,
		resources.NewSNMPCommunityResource,
		resources.NewSNMPResource,
	}
}

// DataSources defines the data sources implemented in the provider
func (p *CiscoProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		// No data sources yet
	}
}
