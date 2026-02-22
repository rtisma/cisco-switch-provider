package client

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// VLANInfo represents VLAN information from "show vlan" output
type VLANInfo struct {
	ID     int
	Name   string
	State  string
	Ports  []string
	Exists bool
}

// InterfaceInfo represents interface configuration information
type InterfaceInfo struct {
	Name        string
	Description string
	Mode        string // "access" or "trunk"
	AccessVLAN  int
	TrunkVLANs  []int
	NativeVLAN  int
	AdminState  string // "up" or "down"
	Exists      bool
}

// SVIInfo represents SVI (VLAN interface) information
type SVIInfo struct {
	VlanID      int
	IPAddress   string
	SubnetMask  string
	Description string
	AdminState  string
	DHCPServers []string
	Exists      bool
}

// ParseShowVlan parses output from "show vlan id X" or "show vlan brief"
func ParseShowVlan(output string, vlanID int) (*VLANInfo, error) {
	lines := splitLines(output)

	vlan := &VLANInfo{
		ID:     vlanID,
		Exists: false,
	}

	// Check if VLAN doesn't exist
	if containsString(output, "VLAN does not exist") ||
		containsString(output, "not found") {
		return vlan, nil
	}

	// Parse VLAN table
	// Format:
	// VLAN Name                             Status    Ports
	// ---- -------------------------------- --------- -------------------------------
	// 100  Sales_VLAN                       active    Gi1/0/1, Gi1/0/2

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip header lines
		if containsString(line, "VLAN") && containsString(line, "Name") {
			continue
		}
		if containsString(line, "----") {
			continue
		}

		// Parse VLAN line
		fields := splitByWhitespace(line)
		if len(fields) < 2 {
			continue
		}

		// Check if first field is our VLAN ID
		id, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		if id == vlanID {
			vlan.Exists = true
			vlan.Name = fields[1]

			// Status is usually the next field
			if len(fields) > 2 {
				vlan.State = fields[2]
			}

			// Ports might be on the same line or next lines
			if len(fields) > 3 {
				ports := strings.Join(fields[3:], " ")
				vlan.Ports = parsePortList(ports)
			}

			// Check subsequent lines for port continuation
			for j := i + 1; j < len(lines); j++ {
				nextLine := strings.TrimSpace(lines[j])
				if nextLine == "" || startsWithDigit(nextLine) {
					break
				}
				vlan.Ports = append(vlan.Ports, parsePortList(nextLine)...)
			}

			break
		}
	}

	return vlan, nil
}

// ParseInterfaceConfig parses "show running-config interface X" output
func ParseInterfaceConfig(output, interfaceName string) (*InterfaceInfo, error) {
	iface := &InterfaceInfo{
		Name:   interfaceName,
		Exists: false,
		Mode:   "access", // Default
	}

	// Check if interface doesn't exist
	if containsString(output, "Invalid input") ||
		containsString(output, "does not exist") {
		return iface, nil
	}

	lines := splitLines(output)

	// Check if we have interface config
	hasConfig := false
	for _, line := range lines {
		if containsString(line, "interface "+interfaceName) ||
			containsString(line, "interface ") {
			hasConfig = true
			iface.Exists = true
			break
		}
	}

	if !hasConfig {
		return iface, nil
	}

	// Parse configuration lines
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "description ") {
			iface.Description = strings.TrimPrefix(line, "description ")
		} else if containsString(line, "switchport mode access") {
			iface.Mode = "access"
		} else if containsString(line, "switchport mode trunk") {
			iface.Mode = "trunk"
		} else if strings.HasPrefix(line, "switchport access vlan ") {
			vlanStr := strings.TrimPrefix(line, "switchport access vlan ")
			if vlan, err := strconv.Atoi(strings.TrimSpace(vlanStr)); err == nil {
				iface.AccessVLAN = vlan
			}
		} else if strings.HasPrefix(line, "switchport trunk native vlan ") {
			vlanStr := strings.TrimPrefix(line, "switchport trunk native vlan ")
			if vlan, err := strconv.Atoi(strings.TrimSpace(vlanStr)); err == nil {
				iface.NativeVLAN = vlan
			}
		} else if strings.HasPrefix(line, "switchport trunk allowed vlan ") {
			vlanStr := strings.TrimPrefix(line, "switchport trunk allowed vlan ")
			iface.TrunkVLANs = parseVLANList(vlanStr)
		} else if line == "shutdown" {
			iface.AdminState = "down"
		} else if line == "no shutdown" {
			iface.AdminState = "up"
		}
	}

	// Default admin state
	if iface.AdminState == "" {
		iface.AdminState = "up"
	}

	return iface, nil
}

// ParseSVIConfig parses "show running-config interface vlan X" output
func ParseSVIConfig(output string, vlanID int) (*SVIInfo, error) {
	svi := &SVIInfo{
		VlanID: vlanID,
		Exists: false,
	}

	// Check if SVI doesn't exist
	if containsString(output, "Invalid input") ||
		containsString(output, "does not exist") {
		return svi, nil
	}

	lines := splitLines(output)

	// Check if we have SVI config
	interfaceName := fmt.Sprintf("Vlan%d", vlanID)
	hasConfig := false
	for _, line := range lines {
		if containsString(line, "interface "+interfaceName) ||
			containsString(line, "interface Vlan") {
			hasConfig = true
			svi.Exists = true
			break
		}
	}

	if !hasConfig {
		return svi, nil
	}

	// Parse IP address using regex
	ipRegex := regexp.MustCompile(`ip address (\d+\.\d+\.\d+\.\d+) (\d+\.\d+\.\d+\.\d+)`)
	helperRegex := regexp.MustCompile(`ip helper-address (\d+\.\d+\.\d+\.\d+)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "description ") {
			svi.Description = strings.TrimPrefix(line, "description ")
		} else if matches := ipRegex.FindStringSubmatch(line); len(matches) == 3 {
			svi.IPAddress = matches[1]
			svi.SubnetMask = matches[2]
		} else if matches := helperRegex.FindStringSubmatch(line); len(matches) == 2 {
			svi.DHCPServers = append(svi.DHCPServers, matches[1])
		} else if line == "shutdown" {
			svi.AdminState = "down"
		} else if line == "no shutdown" {
			svi.AdminState = "up"
		}
	}

	// Default admin state
	if svi.AdminState == "" {
		svi.AdminState = "up"
	}

	return svi, nil
}

// Helper functions

func splitByWhitespace(s string) []string {
	var fields []string
	current := ""
	inSpace := false

	for _, ch := range s {
		if ch == ' ' || ch == '\t' {
			if !inSpace && current != "" {
				fields = append(fields, current)
				current = ""
			}
			inSpace = true
		} else {
			current += string(ch)
			inSpace = false
		}
	}

	if current != "" {
		fields = append(fields, current)
	}

	return fields
}

func parsePortList(s string) []string {
	var ports []string
	s = strings.TrimSpace(s)

	// Split by comma
	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			ports = append(ports, part)
		}
	}

	return ports
}

func parseVLANList(s string) []int {
	var vlans []int
	s = strings.TrimSpace(s)

	// Handle "add" prefix
	s = strings.TrimPrefix(s, "add ")

	// Split by comma
	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Handle ranges (e.g., "10-20")
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) == 2 {
				start, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
				end, err2 := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
				if err1 == nil && err2 == nil {
					for i := start; i <= end; i++ {
						vlans = append(vlans, i)
					}
				}
			}
		} else {
			// Single VLAN
			if vlan, err := strconv.Atoi(part); err == nil {
				vlans = append(vlans, vlan)
			}
		}
	}

	return vlans
}

func startsWithDigit(s string) bool {
	if len(s) == 0 {
		return false
	}
	return s[0] >= '0' && s[0] <= '9'
}
