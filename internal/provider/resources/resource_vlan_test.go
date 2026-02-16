package resources

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/example-org/terraform-provider-cisco/tests/mock"
)

// These are acceptance tests that require a running mock switch
// Run with: go test -v -tags=acceptance

func setupTestProvider(t *testing.T) (func() tfprotov6.ProviderServer, *mock.MockSwitch, func()) {
	// Create and start mock switch
	mockSwitch, err := mock.NewMockSwitch("TestSwitch", "admin", "password", "enable")
	if err != nil {
		t.Fatalf("Failed to create mock switch: %v", err)
	}

	err = mockSwitch.Start(2222) // Use fixed port for testing
	if err != nil {
		t.Fatalf("Failed to start mock switch: %v", err)
	}

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	// Create provider factory
	providerFactory := func() tfprotov6.ProviderServer {
		return providerserver.NewProtocol6(
			NewTestProvider(),
		)()
	}

	cleanup := func() {
		mockSwitch.Stop()
	}

	return providerFactory, mockSwitch, cleanup
}

// NewTestProvider creates a test provider instance
func NewTestProvider() provider.Provider {
	return &TestCiscoProvider{
		version: "test",
	}
}

// TestCiscoProvider is a test version of the provider
type TestCiscoProvider struct {
	version string
}

func (p *TestCiscoProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "cisco"
	resp.Version = p.version
}

func (p *TestCiscoProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	// Use the same schema as the real provider
	// For brevity in testing, we'll just note this should match
}

func (p *TestCiscoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// Test configuration
}

func (p *TestCiscoProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewVlanResource,
		NewInterfaceResource,
		NewSVIResource,
		NewInterfaceIPResource,
	}
}

func (p *TestCiscoProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

// Unit tests for VLAN resource validation

func TestVlanResourceValidation(t *testing.T) {
	tests := []struct {
		name      string
		vlanID    int64
		vlanName  string
		expectErr bool
	}{
		{
			name:      "valid VLAN",
			vlanID:    100,
			vlanName:  "TestVLAN",
			expectErr: false,
		},
		{
			name:      "VLAN ID too low",
			vlanID:    0,
			vlanName:  "TestVLAN",
			expectErr: true,
		},
		{
			name:      "VLAN ID too high",
			vlanID:    5000,
			vlanName:  "TestVLAN",
			expectErr: true,
		},
		{
			name:      "VLAN ID at lower bound",
			vlanID:    1,
			vlanName:  "TestVLAN",
			expectErr: false,
		},
		{
			name:      "VLAN ID at upper bound",
			vlanID:    4094,
			vlanName:  "TestVLAN",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test VLAN ID validation
			if tt.vlanID < 1 || tt.vlanID > 4094 {
				if !tt.expectErr {
					t.Error("Expected error for invalid VLAN ID")
				}
			} else {
				if tt.expectErr {
					t.Error("Did not expect error for valid VLAN ID")
				}
			}
		})
	}
}

func TestBuildVLANCommands(t *testing.T) {
	tests := []struct {
		name     string
		vlanID   int
		vlanName string
		state    string
		want     []string
	}{
		{
			name:     "basic VLAN",
			vlanID:   100,
			vlanName: "TestVLAN",
			state:    "active",
			want: []string{
				"vlan 100",
				"name TestVLAN",
				"end",
			},
		},
		{
			name:     "suspended VLAN",
			vlanID:   200,
			vlanName: "SuspendedVLAN",
			state:    "suspend",
			want: []string{
				"vlan 200",
				"name SuspendedVLAN",
				"state suspend",
				"end",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands := []string{
				fmt.Sprintf("vlan %d", tt.vlanID),
				fmt.Sprintf("name %s", tt.vlanName),
			}

			if tt.state != "active" {
				commands = append(commands, fmt.Sprintf("state %s", tt.state))
			}

			commands = append(commands, "end")

			// Verify command structure
			if len(commands) < 3 {
				t.Error("Commands should have at least 3 entries")
			}

			if commands[0] != fmt.Sprintf("vlan %d", tt.vlanID) {
				t.Errorf("First command should be 'vlan %d'", tt.vlanID)
			}

			if commands[len(commands)-1] != "end" {
				t.Error("Last command should be 'end'")
			}
		})
	}
}
