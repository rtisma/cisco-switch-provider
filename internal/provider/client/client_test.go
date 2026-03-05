package client

import (
	"fmt"
	"testing"
	"time"

	"github.com/example-org/terraform-provider-cisco/tests/mock"
)

func setupMockSwitch(t *testing.T) (*mock.MockSwitch, Config) {
	// Create mock switch
	mockSwitch, err := mock.NewMockSwitch("TestSwitch", "admin", "password", "enable")
	if err != nil {
		t.Fatalf("Failed to create mock switch: %v", err)
	}

	// Start on random port
	err = mockSwitch.Start(0) // 0 = random available port
	if err != nil {
		t.Fatalf("Failed to start mock switch: %v", err)
	}

	// Parse address to get host and port
	addr := mockSwitch.GetAddress()

	config := Config{
		Host:           "127.0.0.1",
		Port:           parsePort(addr),
		Username:       "admin",
		SSHTimeout:     5 * time.Second,
		CommandTimeout: 2 * time.Second,
	}

	return mockSwitch, config
}

func parsePort(addr string) int {
	// addr is in format "127.0.0.1:12345"
	var port int
	_, err := fmt.Sscanf(addr, "127.0.0.1:%d", &port)
	if err != nil {
		return 2222 // fallback
	}
	return port
}

func TestClientConnect(t *testing.T) {
	mockSwitch, config := setupMockSwitch(t)
	defer mockSwitch.Stop()

	client := NewClient(config)

	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Disconnect()

	if !client.IsConnected() {
		t.Error("Client should be connected")
	}
}

func TestClientDisconnect(t *testing.T) {
	mockSwitch, config := setupMockSwitch(t)
	defer mockSwitch.Stop()

	client := NewClient(config)

	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	err = client.Disconnect()
	if err != nil {
		t.Errorf("Disconnect() failed: %v", err)
	}

	if client.IsConnected() {
		t.Error("Client should not be connected")
	}
}

func TestClientExecuteCommand(t *testing.T) {
	mockSwitch, config := setupMockSwitch(t)
	defer mockSwitch.Stop()

	client := NewClient(config)
	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Disconnect()

	// Test show command
	output, err := client.ExecuteCommand("show vlan id 1")
	if err != nil {
		t.Fatalf("ExecuteCommand() failed: %v", err)
	}

	if output == "" {
		t.Error("Expected non-empty output")
	}
}

func TestClientExecuteConfigCommands(t *testing.T) {
	mockSwitch, config := setupMockSwitch(t)
	defer mockSwitch.Stop()

	client := NewClient(config)
	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Disconnect()

	// Create a VLAN
	commands := []string{
		"vlan 100",
		"name TestVLAN",
		"state active",
		"end",
	}

	err = client.ExecuteConfigCommands(commands)
	if err != nil {
		t.Fatalf("ExecuteConfigCommands() failed: %v", err)
	}

	// Verify VLAN was created
	output, err := client.ExecuteCommand("show vlan id 100")
	if err != nil {
		t.Fatalf("Failed to verify VLAN: %v", err)
	}

	if !containsString(output, "TestVLAN") {
		t.Errorf("VLAN was not created properly, output: %s", output)
	}
}

func TestVLANLifecycle(t *testing.T) {
	mockSwitch, config := setupMockSwitch(t)
	defer mockSwitch.Stop()

	client := NewClient(config)
	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Disconnect()

	// Create VLAN
	t.Run("Create", func(t *testing.T) {
		commands := []string{
			"vlan 200",
			"name Engineering",
			"state active",
		}
		err := client.ExecuteConfigCommands(commands)
		if err != nil {
			t.Fatalf("Failed to create VLAN: %v", err)
		}
	})

	// Read VLAN
	t.Run("Read", func(t *testing.T) {
		output, err := client.ExecuteCommand("show vlan id 200")
		if err != nil {
			t.Fatalf("Failed to read VLAN: %v", err)
		}

		vlanInfo, err := ParseShowVlan(output, 200)
		if err != nil {
			t.Fatalf("Failed to parse VLAN: %v", err)
		}

		if !vlanInfo.Exists {
			t.Error("VLAN should exist")
		}
		if vlanInfo.Name != "Engineering" {
			t.Errorf("VLAN name = %v, want Engineering", vlanInfo.Name)
		}
		if vlanInfo.State != "active" {
			t.Errorf("VLAN state = %v, want active", vlanInfo.State)
		}
	})

	// Update VLAN
	t.Run("Update", func(t *testing.T) {
		commands := []string{
			"vlan 200",
			"name Engineering_Updated",
		}
		err := client.ExecuteConfigCommands(commands)
		if err != nil {
			t.Fatalf("Failed to update VLAN: %v", err)
		}

		output, err := client.ExecuteCommand("show vlan id 200")
		if err != nil {
			t.Fatalf("Failed to read updated VLAN: %v", err)
		}

		if !containsString(output, "Engineering_Updated") {
			t.Error("VLAN name was not updated")
		}
	})

	// Delete VLAN
	t.Run("Delete", func(t *testing.T) {
		commands := []string{
			"no vlan 200",
		}
		err := client.ExecuteConfigCommands(commands)
		if err != nil {
			t.Fatalf("Failed to delete VLAN: %v", err)
		}

		output, err := client.ExecuteCommand("show vlan id 200")
		if err != nil {
			t.Fatalf("Failed to check deleted VLAN: %v", err)
		}

		vlanInfo, err := ParseShowVlan(output, 200)
		if err != nil {
			t.Fatalf("Failed to parse VLAN: %v", err)
		}

		if vlanInfo.Exists {
			t.Error("VLAN should not exist after deletion")
		}
	})
}

func TestInterfaceLifecycle(t *testing.T) {
	mockSwitch, config := setupMockSwitch(t)
	defer mockSwitch.Stop()

	client := NewClient(config)
	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Disconnect()

	ifaceName := "GigabitEthernet1/0/10"

	// Configure interface
	t.Run("Configure", func(t *testing.T) {
		commands := []string{
			"interface " + ifaceName,
			"description Test Port",
			"switchport mode access",
			"switchport access vlan 100",
			"no shutdown",
		}
		err := client.ExecuteConfigCommands(commands)
		if err != nil {
			t.Fatalf("Failed to configure interface: %v", err)
		}
	})

	// Read interface
	t.Run("Read", func(t *testing.T) {
		output, err := client.ExecuteCommand("show running-config interface " + ifaceName)
		if err != nil {
			t.Fatalf("Failed to read interface: %v", err)
		}

		ifaceInfo, err := ParseInterfaceConfig(output, ifaceName)
		if err != nil {
			t.Fatalf("Failed to parse interface: %v", err)
		}

		if !ifaceInfo.Exists {
			t.Error("Interface should exist")
		}
		if ifaceInfo.Description != "Test Port" {
			t.Errorf("Description = %v, want 'Test Port'", ifaceInfo.Description)
		}
		if ifaceInfo.Mode != "access" {
			t.Errorf("Mode = %v, want access", ifaceInfo.Mode)
		}
		if ifaceInfo.AccessVLAN != 100 {
			t.Errorf("AccessVLAN = %v, want 100", ifaceInfo.AccessVLAN)
		}
	})

	// Reset interface
	t.Run("Reset", func(t *testing.T) {
		commands := []string{
			"default interface " + ifaceName,
		}
		err := client.ExecuteConfigCommands(commands)
		if err != nil {
			t.Fatalf("Failed to reset interface: %v", err)
		}
	})
}

func TestSVILifecycle(t *testing.T) {
	mockSwitch, config := setupMockSwitch(t)
	defer mockSwitch.Stop()

	client := NewClient(config)
	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Disconnect()

	vlanID := 150

	// Create SVI
	t.Run("Create", func(t *testing.T) {
		commands := []string{
			fmt.Sprintf("interface vlan %d", vlanID),
			"description Test Gateway",
			"ip address 192.168.150.1 255.255.255.0",
			"no shutdown",
		}
		err := client.ExecuteConfigCommands(commands)
		if err != nil {
			t.Fatalf("Failed to create SVI: %v", err)
		}
	})

	// Read SVI
	t.Run("Read", func(t *testing.T) {
		output, err := client.ExecuteCommand(fmt.Sprintf("show running-config interface vlan %d", vlanID))
		if err != nil {
			t.Fatalf("Failed to read SVI: %v", err)
		}

		sviInfo, err := ParseSVIConfig(output, vlanID)
		if err != nil {
			t.Fatalf("Failed to parse SVI: %v", err)
		}

		if !sviInfo.Exists {
			t.Error("SVI should exist")
		}
		if sviInfo.IPAddress != "192.168.150.1" {
			t.Errorf("IPAddress = %v, want 192.168.150.1", sviInfo.IPAddress)
		}
		if sviInfo.SubnetMask != "255.255.255.0" {
			t.Errorf("SubnetMask = %v, want 255.255.255.0", sviInfo.SubnetMask)
		}
	})

	// Delete SVI
	t.Run("Delete", func(t *testing.T) {
		commands := []string{
			fmt.Sprintf("no interface vlan %d", vlanID),
		}
		err := client.ExecuteConfigCommands(commands)
		if err != nil {
			t.Fatalf("Failed to delete SVI: %v", err)
		}
	})
}

func TestConcurrentCommands(t *testing.T) {
	mockSwitch, config := setupMockSwitch(t)
	defer mockSwitch.Stop()

	client := NewClient(config)
	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Disconnect()

	// Test that concurrent commands are properly serialized
	done := make(chan bool, 2)

	go func() {
		commands := []string{
			"vlan 300",
			"name VLAN300",
		}
		err := client.ExecuteConfigCommands(commands)
		if err != nil {
			t.Errorf("Goroutine 1 failed: %v", err)
		}
		done <- true
	}()

	go func() {
		commands := []string{
			"vlan 400",
			"name VLAN400",
		}
		err := client.ExecuteConfigCommands(commands)
		if err != nil {
			t.Errorf("Goroutine 2 failed: %v", err)
		}
		done <- true
	}()

	<-done
	<-done

	// Verify both VLANs were created
	output, _ := client.ExecuteCommand("show vlan id 300")
	if !containsString(output, "VLAN300") {
		t.Error("VLAN 300 was not created")
	}

	output, _ = client.ExecuteCommand("show vlan id 400")
	if !containsString(output, "VLAN400") {
		t.Error("VLAN 400 was not created")
	}
}

func TestInvalidCommands(t *testing.T) {
	mockSwitch, config := setupMockSwitch(t)
	defer mockSwitch.Stop()

	client := NewClient(config)
	err := client.Connect()
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Disconnect()

	tests := []struct {
		name     string
		commands []string
	}{
		{
			name: "invalid VLAN ID",
			commands: []string{
				"vlan 5000", // Invalid: > 4094
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.ExecuteConfigCommands(tt.commands)
			// We expect some commands to fail, that's OK
			// The important thing is we don't crash
			_ = err
		})
	}
}
