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
	VlanID          int
	IPAddress       string
	SubnetMask      string
	Description     string
	AdminState      string
	DHCPServers     []string
	AccessGroupIn   string
	AccessGroupOut  string
	Exists          bool
}

// ACLInfo represents a named IP access list
type ACLInfo struct {
	Name   string
	Type   string // "standard" or "extended"
	Exists bool
}

// ACLRuleInfo represents a single ACE (access control entry) in a named ACL
type ACLRuleInfo struct {
	Sequence            int
	Action              string // "permit" or "deny"
	Protocol            string // "ip", "tcp", "udp", "icmp", etc.
	Source              string // "any", "host x.x.x.x", or "x.x.x.x"
	SourceWildcard      string // wildcard mask, empty when source is "any" or a host
	Destination         string // same format as Source
	DestinationWildcard string
	SrcPort             string // e.g. "eq 80", "range 8000 8080" (TCP/UDP only)
	DstPort             string // same
	Log                 bool
	Exists              bool
}

// DHCPPoolInfo represents DHCP pool configuration
type DHCPPoolInfo struct {
	Name          string
	Network       string
	SubnetMask    string
	DefaultRouter string
	DNSServers    []string
	LeaseDays     int
	LeaseHours    int
	LeaseMinutes  int
	DomainName    string
	Exists        bool
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
	accessGroupRegex := regexp.MustCompile(`ip access-group (\S+) (in|out)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "description ") {
			svi.Description = strings.TrimPrefix(line, "description ")
		} else if matches := ipRegex.FindStringSubmatch(line); len(matches) == 3 {
			svi.IPAddress = matches[1]
			svi.SubnetMask = matches[2]
		} else if matches := helperRegex.FindStringSubmatch(line); len(matches) == 2 {
			svi.DHCPServers = append(svi.DHCPServers, matches[1])
		} else if matches := accessGroupRegex.FindStringSubmatch(line); len(matches) == 3 {
			if matches[2] == "in" {
				svi.AccessGroupIn = matches[1]
			} else {
				svi.AccessGroupOut = matches[1]
			}
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

// ParseACL parses "show running-config | section ip access-list NAME" to check ACL existence and type.
func ParseACL(output, name string) (*ACLInfo, error) {
	acl := &ACLInfo{Name: name, Exists: false}

	for _, line := range splitLines(output) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ip access-list extended "+name) {
			acl.Type = "extended"
			acl.Exists = true
			return acl, nil
		}
		if strings.HasPrefix(trimmed, "ip access-list standard "+name) {
			acl.Type = "standard"
			acl.Exists = true
			return acl, nil
		}
	}
	return acl, nil
}

// ParseACLRule parses a specific sequence entry from ACL config output.
// output is from "show running-config | section ip access-list NAME".
func ParseACLRule(output, aclName string, sequence int) (*ACLRuleInfo, error) {
	rule := &ACLRuleInfo{Sequence: sequence, Exists: false}

	seqStr := strconv.Itoa(sequence)
	inACL := false

	for _, line := range splitLines(output) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Detect our ACL header
		if strings.HasPrefix(trimmed, "ip access-list extended "+aclName) ||
			strings.HasPrefix(trimmed, "ip access-list standard "+aclName) {
			inACL = true
			continue
		}

		// Stop if we enter another top-level config block
		if !strings.HasPrefix(line, " ") && line != "" && inACL {
			break
		}

		if !inACL {
			continue
		}

		// Look for our sequence number
		tokens := splitByWhitespace(trimmed)
		if len(tokens) < 2 {
			continue
		}
		if tokens[0] != seqStr {
			continue
		}

		// Found our rule — parse it
		parsed := parseACLTokens(tokens)
		if parsed != nil {
			parsed.Sequence = sequence
			parsed.Exists = true
			return parsed, nil
		}
	}
	return rule, nil
}

// parseACLTokens parses the token list for a single ACE starting after the sequence number.
// tokens[0] is the sequence, tokens[1] is action, etc.
func parseACLTokens(tokens []string) *ACLRuleInfo {
	if len(tokens) < 3 {
		return nil
	}

	rule := &ACLRuleInfo{}
	idx := 1 // skip sequence

	rule.Action = tokens[idx]
	idx++

	rule.Protocol = tokens[idx]
	idx++

	// Source address spec
	src, srcWild, consumed := parseAddrSpec(tokens[idx:])
	rule.Source = src
	rule.SourceWildcard = srcWild
	idx += consumed

	// Optional source port (TCP/UDP only, before destination)
	if isPortOperator(rule.Protocol, tokens, idx) {
		portSpec, n := buildPortSpec(tokens[idx:])
		rule.SrcPort = portSpec
		idx += n
	}

	// Destination address spec (extended ACLs only)
	if idx < len(tokens) && tokens[idx] != "log" {
		dst, dstWild, consumed := parseAddrSpec(tokens[idx:])
		rule.Destination = dst
		rule.DestinationWildcard = dstWild
		idx += consumed
	}

	// Optional destination port
	if isPortOperator(rule.Protocol, tokens, idx) {
		portSpec, n := buildPortSpec(tokens[idx:])
		rule.DstPort = portSpec
		idx += n
	}

	// Log keyword
	if idx < len(tokens) && tokens[idx] == "log" {
		rule.Log = true
	}

	return rule
}

// parseAddrSpec reads "any", "host <ip>", or "<ip> <wildcard>" from the token slice.
// Returns the address, wildcard, and number of tokens consumed.
func parseAddrSpec(tokens []string) (addr, wildcard string, consumed int) {
	if len(tokens) == 0 {
		return "", "", 0
	}

	if tokens[0] == "any" {
		return "any", "", 1
	}

	if tokens[0] == "host" && len(tokens) >= 2 {
		return tokens[1], "", 2
	}

	// IP address — next token is a wildcard if it looks like one
	ip := tokens[0]
	if len(tokens) >= 2 && looksLikeIPv4(tokens[1]) {
		return ip, tokens[1], 2
	}
	return ip, "", 1
}

// isPortOperator returns true when the token at idx is a port match keyword (eq, lt, gt, range, neq)
// and the protocol supports ports.
func isPortOperator(protocol string, tokens []string, idx int) bool {
	if protocol != "tcp" && protocol != "udp" {
		return false
	}
	if idx >= len(tokens) {
		return false
	}
	op := tokens[idx]
	return op == "eq" || op == "lt" || op == "gt" || op == "neq" || op == "range"
}

// buildPortSpec reads the port operator and operand(s) from the token slice.
func buildPortSpec(tokens []string) (spec string, consumed int) {
	if len(tokens) == 0 {
		return "", 0
	}
	op := tokens[0]
	if op == "range" && len(tokens) >= 3 {
		return "range " + tokens[1] + " " + tokens[2], 3
	}
	if len(tokens) >= 2 {
		return op + " " + tokens[1], 2
	}
	return op, 1
}

// looksLikeIPv4 returns true if s is a dotted-quad (used to distinguish wildcards from keywords).
func looksLikeIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
	}
	return true
}

// BuildACLRuleCommand assembles the IOS ACE string for a given rule (without the sequence prefix).
func BuildACLRuleCommand(rule *ACLRuleInfo) string {
	cmd := fmt.Sprintf("%d %s %s %s",
		rule.Sequence,
		rule.Action,
		rule.Protocol,
		buildACLAddrSpec(rule.Source, rule.SourceWildcard),
	)

	if rule.SrcPort != "" {
		cmd += " " + rule.SrcPort
	}

	if rule.Destination != "" {
		cmd += " " + buildACLAddrSpec(rule.Destination, rule.DestinationWildcard)
	}

	if rule.DstPort != "" {
		cmd += " " + rule.DstPort
	}

	if rule.Log {
		cmd += " log"
	}
	return cmd
}

// buildACLAddrSpec formats an address + optional wildcard into an IOS ACL address specifier.
func buildACLAddrSpec(addr, wildcard string) string {
	if addr == "any" {
		return "any"
	}
	if wildcard == "" || wildcard == "0.0.0.0" {
		return "host " + addr
	}
	return addr + " " + wildcard
}

// ParseDHCPPool parses output from "show running-config | section ip dhcp pool NAME"
func ParseDHCPPool(output, poolName string) (*DHCPPoolInfo, error) {
	pool := &DHCPPoolInfo{
		Name:   poolName,
		Exists: false,
	}

	if containsString(output, "Invalid input detected") ||
		containsString(output, "% Invalid") {
		return pool, nil
	}

	lines := splitLines(output)

	inPool := false
	for _, line := range lines {
		// Detect pool header (may have leading spaces from section output)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "ip dhcp pool ") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "ip dhcp pool "))
			if name == poolName {
				inPool = true
				pool.Exists = true
			} else {
				inPool = false
			}
			continue
		}

		if !inPool {
			continue
		}

		// Sub-commands are indented; parse them trimmed
		if strings.HasPrefix(trimmed, "network ") {
			parts := splitByWhitespace(strings.TrimPrefix(trimmed, "network "))
			if len(parts) >= 2 {
				pool.Network = parts[0]
				pool.SubnetMask = parts[1]
			}
		} else if strings.HasPrefix(trimmed, "default-router ") {
			pool.DefaultRouter = strings.TrimSpace(strings.TrimPrefix(trimmed, "default-router "))
		} else if strings.HasPrefix(trimmed, "dns-server ") {
			pool.DNSServers = splitByWhitespace(strings.TrimPrefix(trimmed, "dns-server "))
		} else if strings.HasPrefix(trimmed, "lease ") {
			parts := splitByWhitespace(strings.TrimPrefix(trimmed, "lease "))
			if len(parts) >= 1 {
				pool.LeaseDays, _ = strconv.Atoi(parts[0])
			}
			if len(parts) >= 2 {
				pool.LeaseHours, _ = strconv.Atoi(parts[1])
			}
			if len(parts) >= 3 {
				pool.LeaseMinutes, _ = strconv.Atoi(parts[2])
			}
		} else if strings.HasPrefix(trimmed, "domain-name ") {
			pool.DomainName = strings.TrimSpace(strings.TrimPrefix(trimmed, "domain-name "))
		}
	}

	return pool, nil
}

// SNMPCommunityInfo represents a single SNMP community string
type SNMPCommunityInfo struct {
	Name   string
	Access string // "ro" or "rw"
	ACL    string // optional standard ACL name/number restricting access
	Exists bool
}

// SNMPInfo represents global SNMP server settings
type SNMPInfo struct {
	Location      string
	Contact       string
	TrapCommunity string
	TrapVersion   string   // "1" or "2c"
	TrapHosts     []string // IP addresses of trap receivers
	Exists        bool
}

// ParseSNMPCommunity parses "show running-config | include snmp-server community"
// output to find a specific community by name.
func ParseSNMPCommunity(output, name string) (*SNMPCommunityInfo, error) {
	community := &SNMPCommunityInfo{Name: name}
	// Matches: snmp-server community <name> <ro|rw> [<acl>]
	re := regexp.MustCompile(`snmp-server community (\S+)\s+(ro|rw)(?:\s+(\S+))?`)
	for _, line := range splitLines(output) {
		if matches := re.FindStringSubmatch(strings.TrimSpace(line)); len(matches) >= 3 {
			if matches[1] == name {
				community.Exists = true
				community.Access = matches[2]
				if len(matches) >= 4 {
					community.ACL = matches[3]
				}
				return community, nil
			}
		}
	}
	return community, nil
}

// ParseSNMP parses "show running-config | include snmp-server" output for global SNMP settings.
func ParseSNMP(output string) (*SNMPInfo, error) {
	info := &SNMPInfo{TrapVersion: "2c"}

	locationRe := regexp.MustCompile(`snmp-server location\s+(.+)`)
	contactRe  := regexp.MustCompile(`snmp-server contact\s+(.+)`)
	// matches: snmp-server host <ip> [version <ver>] <community>
	hostRe := regexp.MustCompile(`snmp-server host\s+(\S+)(?:\s+version\s+(1|2c|3))?\s+(\S+)`)

	for _, line := range splitLines(output) {
		line = strings.TrimSpace(line)
		if matches := locationRe.FindStringSubmatch(line); len(matches) == 2 {
			info.Location = strings.TrimSpace(matches[1])
			info.Exists = true
		} else if matches := contactRe.FindStringSubmatch(line); len(matches) == 2 {
			info.Contact = strings.TrimSpace(matches[1])
			info.Exists = true
		} else if matches := hostRe.FindStringSubmatch(line); len(matches) >= 4 {
			info.TrapHosts = append(info.TrapHosts, matches[1])
			if matches[2] != "" {
				info.TrapVersion = matches[2]
			}
			info.TrapCommunity = matches[3]
			info.Exists = true
		}
	}

	return info, nil
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

// ── Static routes ─────────────────────────────────────────────────────────────

// StaticRouteInfo represents a single static IP route.
type StaticRouteInfo struct {
	Network       string
	Mask          string
	NextHop       string
	AdminDistance int
	Exists        bool
}

// ParseStaticRoute parses "show running-config | include ip route" output and
// returns the route entry that matches the given network, mask, and next-hop.
// AdminDistance defaults to 1 (the IOS default) when no explicit distance is found.
func ParseStaticRoute(output, network, mask, nextHop string) (*StaticRouteInfo, error) {
	route := &StaticRouteInfo{
		Network:       network,
		Mask:          mask,
		NextHop:       nextHop,
		AdminDistance: 1,
	}
	re := regexp.MustCompile(`^ip route\s+(\S+)\s+(\S+)\s+(\S+)(?:\s+(\d+))?`)
	for _, line := range splitLines(output) {
		matches := re.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) < 4 {
			continue
		}
		if matches[1] == network && matches[2] == mask && matches[3] == nextHop {
			route.Exists = true
			if len(matches) == 5 && matches[4] != "" {
				route.AdminDistance, _ = strconv.Atoi(matches[4])
			}
			return route, nil
		}
	}
	return route, nil
}

// ── DHCP excluded ranges ──────────────────────────────────────────────────────

// DHCPExcludedRangeInfo represents a DHCP excluded-address range.
type DHCPExcludedRangeInfo struct {
	LowAddress  string
	HighAddress string // always set; equals LowAddress for single-address exclusions
	Exists      bool
}

// ParseDHCPExcludedRange parses "show running-config | include ip dhcp excluded"
// and returns the exclusion entry that matches lowAddress and highAddress.
// Pass highAddress == "" to match a single-address exclusion (high == low).
func ParseDHCPExcludedRange(output, lowAddress, highAddress string) (*DHCPExcludedRangeInfo, error) {
	if highAddress == "" {
		highAddress = lowAddress
	}
	info := &DHCPExcludedRangeInfo{LowAddress: lowAddress, HighAddress: highAddress}

	re := regexp.MustCompile(`^ip dhcp excluded-address\s+(\S+)(?:\s+(\S+))?`)
	for _, line := range splitLines(output) {
		matches := re.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) < 2 {
			continue
		}
		foundLow := matches[1]
		foundHigh := foundLow // IOS may omit the high address for single exclusions
		if len(matches) == 3 && matches[2] != "" {
			foundHigh = matches[2]
		}
		if foundLow == lowAddress && foundHigh == highAddress {
			info.LowAddress = foundLow
			info.HighAddress = foundHigh
			info.Exists = true
			return info, nil
		}
	}
	return info, nil
}

// ── DHCP host bindings ────────────────────────────────────────────────────────

// DHCPHostInfo represents a DHCP host-binding pool (MAC → static IP).
type DHCPHostInfo struct {
	PoolName        string
	IPAddress       string
	SubnetMask      string
	HardwareAddress string // normalised to lowercase colon format: xx:xx:xx:xx:xx:xx
	ClientName      string
	DefaultRouter   string
	Exists          bool
}

// ParseDHCPHostPool parses "show running-config | section ip dhcp pool NAME"
// output for a host-binding pool (contains "host" and "hardware-address").
func ParseDHCPHostPool(output, poolName string) (*DHCPHostInfo, error) {
	info := &DHCPHostInfo{PoolName: poolName}

	if containsString(output, "Invalid input detected") ||
		containsString(output, "% Invalid") {
		return info, nil
	}

	inPool := false
	for _, line := range splitLines(output) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "ip dhcp pool ") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "ip dhcp pool "))
			inPool = (name == poolName)
			if inPool {
				info.Exists = true
			}
			continue
		}

		if !inPool {
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "host "):
			parts := splitByWhitespace(strings.TrimPrefix(trimmed, "host "))
			if len(parts) >= 2 {
				info.IPAddress = parts[0]
				info.SubnetMask = parts[1]
			}
		case strings.HasPrefix(trimmed, "hardware-address "):
			// IOS may append the hardware type (e.g. "ethernet"); take only the MAC token.
			raw := splitByWhitespace(strings.TrimPrefix(trimmed, "hardware-address "))[0]
			info.HardwareAddress = normalizeMAC(raw)
		case strings.HasPrefix(trimmed, "client-name "):
			info.ClientName = strings.TrimSpace(strings.TrimPrefix(trimmed, "client-name "))
		case strings.HasPrefix(trimmed, "default-router "):
			info.DefaultRouter = strings.TrimSpace(strings.TrimPrefix(trimmed, "default-router "))
		}
	}
	return info, nil
}

// normalizeMAC converts a MAC address to lowercase colon-separated format
// (xx:xx:xx:xx:xx:xx). Accepts Cisco dotted-hex (xxxx.xxxx.xxxx) or colon
// notation as input.
func normalizeMAC(mac string) string {
	mac = strings.ToLower(mac)
	clean := strings.NewReplacer(".", "", ":", "").Replace(mac)
	if len(clean) != 12 {
		return mac // unrecognised format – return as-is
	}
	return clean[0:2] + ":" + clean[2:4] + ":" + clean[4:6] + ":" +
		clean[6:8] + ":" + clean[8:10] + ":" + clean[10:12]
}
