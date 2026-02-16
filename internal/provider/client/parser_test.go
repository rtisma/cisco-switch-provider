package client

import (
	"testing"
)

func TestParseShowVlan(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		vlanID   int
		expected *VLANInfo
	}{
		{
			name: "existing VLAN",
			output: `VLAN Name                             Status    Ports
---- -------------------------------- --------- -------------------------------
100  Sales_Department                 active    Gi1/0/1, Gi1/0/2`,
			vlanID: 100,
			expected: &VLANInfo{
				ID:     100,
				Name:   "Sales_Department",
				State:  "active",
				Ports:  []string{"Gi1/0/1", "Gi1/0/2"},
				Exists: true,
			},
		},
		{
			name:   "non-existent VLAN",
			output: "VLAN does not exist",
			vlanID: 999,
			expected: &VLANInfo{
				ID:     999,
				Exists: false,
			},
		},
		{
			name: "VLAN with no ports",
			output: `VLAN Name                             Status    Ports
---- -------------------------------- --------- -------------------------------
200  Engineering                      active    `,
			vlanID: 200,
			expected: &VLANInfo{
				ID:     200,
				Name:   "Engineering",
				State:  "active",
				Ports:  []string{},
				Exists: true,
			},
		},
		{
			name: "VLAN in suspend state",
			output: `VLAN Name                             Status    Ports
---- -------------------------------- --------- -------------------------------
300  TestVLAN                         suspend   `,
			vlanID: 300,
			expected: &VLANInfo{
				ID:     300,
				Name:   "TestVLAN",
				State:  "suspend",
				Exists: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseShowVlan(tt.output, tt.vlanID)
			if err != nil {
				t.Fatalf("ParseShowVlan() error = %v", err)
			}

			if result.ID != tt.expected.ID {
				t.Errorf("ID = %v, want %v", result.ID, tt.expected.ID)
			}
			if result.Exists != tt.expected.Exists {
				t.Errorf("Exists = %v, want %v", result.Exists, tt.expected.Exists)
			}
			if result.Exists {
				if result.Name != tt.expected.Name {
					t.Errorf("Name = %v, want %v", result.Name, tt.expected.Name)
				}
				if result.State != tt.expected.State {
					t.Errorf("State = %v, want %v", result.State, tt.expected.State)
				}
				if len(result.Ports) != len(tt.expected.Ports) {
					t.Errorf("Ports count = %v, want %v", len(result.Ports), len(tt.expected.Ports))
				}
			}
		})
	}
}

func TestParseInterfaceConfig(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		ifaceName string
		expected  *InterfaceInfo
	}{
		{
			name: "access port",
			output: `Building configuration...

Current configuration : 200 bytes
!
interface GigabitEthernet1/0/1
 description Sales Desktop
 switchport mode access
 switchport access vlan 100
 no shutdown
end`,
			ifaceName: "GigabitEthernet1/0/1",
			expected: &InterfaceInfo{
				Name:        "GigabitEthernet1/0/1",
				Description: "Sales Desktop",
				Mode:        "access",
				AccessVLAN:  100,
				AdminState:  "up",
				Exists:      true,
			},
		},
		{
			name: "trunk port",
			output: `Building configuration...

Current configuration : 250 bytes
!
interface GigabitEthernet1/0/48
 description Trunk to Core
 switchport mode trunk
 switchport trunk allowed vlan 100,200,300
 switchport trunk native vlan 10
 no shutdown
end`,
			ifaceName: "GigabitEthernet1/0/48",
			expected: &InterfaceInfo{
				Name:        "GigabitEthernet1/0/48",
				Description: "Trunk to Core",
				Mode:        "trunk",
				TrunkVLANs:  []int{100, 200, 300},
				NativeVLAN:  10,
				AdminState:  "up",
				Exists:      true,
			},
		},
		{
			name: "shutdown interface",
			output: `Building configuration...

Current configuration : 150 bytes
!
interface GigabitEthernet1/0/5
 switchport mode access
 switchport access vlan 50
 shutdown
end`,
			ifaceName: "GigabitEthernet1/0/5",
			expected: &InterfaceInfo{
				Name:       "GigabitEthernet1/0/5",
				Mode:       "access",
				AccessVLAN: 50,
				AdminState: "down",
				Exists:     true,
			},
		},
		{
			name:      "non-existent interface",
			output:    "Invalid input detected",
			ifaceName: "GigabitEthernet9/9/9",
			expected: &InterfaceInfo{
				Name:   "GigabitEthernet9/9/9",
				Mode:   "access",
				Exists: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseInterfaceConfig(tt.output, tt.ifaceName)
			if err != nil {
				t.Fatalf("ParseInterfaceConfig() error = %v", err)
			}

			if result.Name != tt.expected.Name {
				t.Errorf("Name = %v, want %v", result.Name, tt.expected.Name)
			}
			if result.Exists != tt.expected.Exists {
				t.Errorf("Exists = %v, want %v", result.Exists, tt.expected.Exists)
			}
			if result.Exists {
				if result.Description != tt.expected.Description {
					t.Errorf("Description = %v, want %v", result.Description, tt.expected.Description)
				}
				if result.Mode != tt.expected.Mode {
					t.Errorf("Mode = %v, want %v", result.Mode, tt.expected.Mode)
				}
				if result.Mode == "access" && result.AccessVLAN != tt.expected.AccessVLAN {
					t.Errorf("AccessVLAN = %v, want %v", result.AccessVLAN, tt.expected.AccessVLAN)
				}
				if result.AdminState != tt.expected.AdminState {
					t.Errorf("AdminState = %v, want %v", result.AdminState, tt.expected.AdminState)
				}
			}
		})
	}
}

func TestParseSVIConfig(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		vlanID   int
		expected *SVIInfo
	}{
		{
			name: "active SVI",
			output: `Building configuration...

Current configuration : 100 bytes
!
interface Vlan100
 description Sales VLAN Gateway
 ip address 192.168.100.1 255.255.255.0
 no shutdown
end`,
			vlanID: 100,
			expected: &SVIInfo{
				VlanID:      100,
				IPAddress:   "192.168.100.1",
				SubnetMask:  "255.255.255.0",
				Description: "Sales VLAN Gateway",
				AdminState:  "up",
				Exists:      true,
			},
		},
		{
			name: "shutdown SVI",
			output: `Building configuration...

Current configuration : 80 bytes
!
interface Vlan200
 ip address 10.0.0.1 255.255.255.0
 shutdown
end`,
			vlanID: 200,
			expected: &SVIInfo{
				VlanID:     200,
				IPAddress:  "10.0.0.1",
				SubnetMask: "255.255.255.0",
				AdminState: "down",
				Exists:     true,
			},
		},
		{
			name:   "non-existent SVI",
			output: "Invalid input detected",
			vlanID: 999,
			expected: &SVIInfo{
				VlanID: 999,
				Exists: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSVIConfig(tt.output, tt.vlanID)
			if err != nil {
				t.Fatalf("ParseSVIConfig() error = %v", err)
			}

			if result.VlanID != tt.expected.VlanID {
				t.Errorf("VlanID = %v, want %v", result.VlanID, tt.expected.VlanID)
			}
			if result.Exists != tt.expected.Exists {
				t.Errorf("Exists = %v, want %v", result.Exists, tt.expected.Exists)
			}
			if result.Exists {
				if result.IPAddress != tt.expected.IPAddress {
					t.Errorf("IPAddress = %v, want %v", result.IPAddress, tt.expected.IPAddress)
				}
				if result.SubnetMask != tt.expected.SubnetMask {
					t.Errorf("SubnetMask = %v, want %v", result.SubnetMask, tt.expected.SubnetMask)
				}
				if result.Description != tt.expected.Description {
					t.Errorf("Description = %v, want %v", result.Description, tt.expected.Description)
				}
				if result.AdminState != tt.expected.AdminState {
					t.Errorf("AdminState = %v, want %v", result.AdminState, tt.expected.AdminState)
				}
			}
		})
	}
}

func TestParseVLANList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []int
	}{
		{
			name:     "single VLAN",
			input:    "100",
			expected: []int{100},
		},
		{
			name:     "multiple VLANs",
			input:    "100,200,300",
			expected: []int{100, 200, 300},
		},
		{
			name:     "VLAN range",
			input:    "10-15",
			expected: []int{10, 11, 12, 13, 14, 15},
		},
		{
			name:     "mixed",
			input:    "10,20-22,30",
			expected: []int{10, 20, 21, 22, 30},
		},
		{
			name:     "with spaces",
			input:    " 10 , 20 - 22 , 30 ",
			expected: []int{10, 20, 21, 22, 30},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseVLANList(tt.input)

			if len(result) != len(tt.expected) {
				t.Fatalf("length = %v, want %v", len(result), len(tt.expected))
			}

			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("result[%d] = %v, want %v", i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestIsErrorOutput(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "invalid input",
			output:      "% Invalid input detected at '^' marker.",
			expectError: true,
			errorMsg:    "Invalid input detected",
		},
		{
			name:        "incomplete command",
			output:      "% Incomplete command.",
			expectError: true,
			errorMsg:    "Incomplete command",
		},
		{
			name:        "ambiguous command",
			output:      "% Ambiguous command:  \"sh vl\"",
			expectError: true,
			errorMsg:    "% Ambiguous command",
		},
		{
			name:        "access denied",
			output:      "% Access denied",
			expectError: true,
			errorMsg:    "% Access denied",
		},
		{
			name:        "no error",
			output:      "VLAN Name                             Status    Ports\n100  Sales active",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isError, msg := IsErrorOutput(tt.output)

			if isError != tt.expectError {
				t.Errorf("IsErrorOutput() = %v, want %v", isError, tt.expectError)
			}

			if tt.expectError && msg != tt.errorMsg {
				t.Errorf("error message = %v, want %v", msg, tt.errorMsg)
			}
		})
	}
}
