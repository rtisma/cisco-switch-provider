package mock

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// MockSwitch represents a mock Cisco switch for testing
type MockSwitch struct {
	listener net.Listener
	config   *ssh.ServerConfig
	vlans    map[int]*VLAN
	ifaces   map[string]*Interface
	svis     map[int]*SVI
	hostname string
	mu       sync.RWMutex
	running  bool
}

// VLAN represents a VLAN in the mock switch
type VLAN struct {
	ID    int
	Name  string
	State string
}

// Interface represents an interface in the mock switch
type Interface struct {
	Name        string
	Description string
	Mode        string
	AccessVLAN  int
	TrunkVLANs  []int
	NativeVLAN  int
	AdminState  string
}

// SVI represents a Switch Virtual Interface
type SVI struct {
	VlanID      int
	IPAddress   string
	SubnetMask  string
	Description string
	AdminState  string
	DHCPServers []string
}

// NewMockSwitch creates a new mock switch
func NewMockSwitch(hostname, username, password, enablePassword string) (*MockSwitch, error) {
	// Create SSH server config
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == username && string(pass) == password {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected for %q", c.User())
		},
	}

	// Generate SSH key
	privateKey, err := generateSSHKey()
	if err != nil {
		return nil, err
	}
	config.AddHostKey(privateKey)

	ms := &MockSwitch{
		config:   config,
		vlans:    make(map[int]*VLAN),
		ifaces:   make(map[string]*Interface),
		svis:     make(map[int]*SVI),
		hostname: hostname,
	}

	// Add default VLAN 1
	ms.vlans[1] = &VLAN{ID: 1, Name: "default", State: "active"}

	return ms, nil
}

// Start starts the mock SSH server
func (ms *MockSwitch) Start(port int) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}

	ms.listener = listener
	ms.running = true

	go ms.acceptConnections()

	return nil
}

// Stop stops the mock SSH server
func (ms *MockSwitch) Stop() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.listener != nil {
		ms.running = false
		return ms.listener.Close()
	}
	return nil
}

// GetAddress returns the listening address
func (ms *MockSwitch) GetAddress() string {
	if ms.listener != nil {
		return ms.listener.Addr().String()
	}
	return ""
}

// GetSVICount returns the number of SVIs (for testing)
func (ms *MockSwitch) GetSVICount() int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return len(ms.svis)
}

// GetVLANCount returns the number of VLANs (for testing)
func (ms *MockSwitch) GetVLANCount() int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return len(ms.vlans)
}

func (ms *MockSwitch) acceptConnections() {
	for ms.running {
		conn, err := ms.listener.Accept()
		if err != nil {
			if ms.running {
				log.Printf("Failed to accept connection: %v", err)
			}
			continue
		}

		go ms.handleConnection(conn)
	}
}

func (ms *MockSwitch) handleConnection(netConn net.Conn) {
	defer netConn.Close()

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(netConn, ms.config)
	if err != nil {
		log.Printf("Failed to handshake: %v", err)
		return
	}
	defer sshConn.Close()

	// Discard global requests
	go ssh.DiscardRequests(reqs)

	// Handle channels
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			log.Printf("Could not accept channel: %v", err)
			continue
		}

		go ms.handleSession(channel, requests)
	}
}

func (ms *MockSwitch) handleSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()

	// Handle session requests
	for req := range requests {
		switch req.Type {
		case "pty-req":
			req.Reply(true, nil)
		case "shell":
			req.Reply(true, nil)
			go ms.runShell(channel)
		default:
			req.Reply(false, nil)
		}
	}
}

func (ms *MockSwitch) runShell(channel ssh.Channel) {
	mode := "user"
	configContext := ""

	// Send initial prompt
	ms.writePrompt(channel, mode, configContext)

	buffer := make([]byte, 4096)
	cmdBuffer := ""

	for {
		n, err := channel.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Printf("Read error: %v", err)
			}
			return
		}

		input := string(buffer[:n])

		for _, ch := range input {
			if ch == '\r' || ch == '\n' {
				if cmdBuffer != "" {
					// Echo command
					channel.Write([]byte(cmdBuffer + "\r\n"))

					// Process command
					newMode, newContext := ms.processCommand(channel, strings.TrimSpace(cmdBuffer), mode, configContext)
					mode = newMode
					configContext = newContext

					cmdBuffer = ""
					ms.writePrompt(channel, mode, configContext)
				}
			} else if ch == 127 || ch == 8 { // Backspace
				if len(cmdBuffer) > 0 {
					cmdBuffer = cmdBuffer[:len(cmdBuffer)-1]
				}
			} else if ch >= 32 && ch < 127 { // Printable characters
				cmdBuffer += string(ch)
			}
		}
	}
}

func (ms *MockSwitch) writePrompt(channel ssh.Channel, mode, context string) {
	var prompt string
	switch mode {
	case "user":
		prompt = fmt.Sprintf("%s> ", ms.hostname)
	case "privileged":
		prompt = fmt.Sprintf("%s# ", ms.hostname)
	case "config":
		if context != "" {
			// Format context for prompt display
			displayContext := context
			if strings.HasPrefix(context, "config-vlan:") {
				displayContext = "config-vlan"
			} else if strings.HasPrefix(context, "config-if:") {
				displayContext = "config-if"
			}
			prompt = fmt.Sprintf("%s(%s)# ", ms.hostname, displayContext)
		} else {
			prompt = fmt.Sprintf("%s(config)# ", ms.hostname)
		}
	}
	channel.Write([]byte(prompt))
}

func (ms *MockSwitch) processCommand(channel ssh.Channel, cmd, mode, context string) (string, string) {
	cmd = strings.TrimSpace(cmd)

	// Handle empty commands
	if cmd == "" {
		return mode, context
	}

	// Mode transitions
	if cmd == "enable" && mode == "user" {
		return "privileged", ""
	}

	if cmd == "configure terminal" && mode == "privileged" {
		return "config", ""
	}

	if (cmd == "end" || cmd == "exit") && mode == "config" {
		return "privileged", ""
	}

	if cmd == "exit" && mode == "privileged" {
		return "user", ""
	}

	// Terminal commands
	if cmd == "terminal length 0" {
		return mode, context
	}

	// Show commands
	if strings.HasPrefix(cmd, "show ") {
		ms.handleShowCommand(channel, cmd)
		return mode, context
	}

	// Config mode commands
	if mode == "config" {
		return ms.handleConfigCommand(channel, cmd, context)
	}

	// Unknown command
	channel.Write([]byte("% Invalid input detected\r\n"))
	return mode, context
}

func (ms *MockSwitch) handleShowCommand(channel ssh.Channel, cmd string) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if strings.HasPrefix(cmd, "show vlan id ") {
		vlanIDStr := strings.TrimPrefix(cmd, "show vlan id ")
		vlanID, err := strconv.Atoi(vlanIDStr)
		if err == nil {
			if vlan, exists := ms.vlans[vlanID]; exists {
				output := fmt.Sprintf("VLAN Name                             Status    Ports\r\n")
				output += fmt.Sprintf("---- -------------------------------- --------- -------------------------------\r\n")
				output += fmt.Sprintf("%-4d %-32s %-9s\r\n", vlan.ID, vlan.Name, vlan.State)
				channel.Write([]byte(output))
			} else {
				channel.Write([]byte(fmt.Sprintf("VLAN %d does not exist\r\n", vlanID)))
			}
		}
	} else if strings.HasPrefix(cmd, "show running-config interface ") {
		ifaceName := strings.TrimPrefix(cmd, "show running-config interface ")
		ifaceName = strings.TrimSpace(ifaceName)

		// Check if it's an SVI
		if strings.HasPrefix(ifaceName, "Vlan") || strings.HasPrefix(ifaceName, "vlan") {
			vlanIDStr := strings.TrimPrefix(strings.ToLower(ifaceName), "vlan")
			vlanIDStr = strings.TrimSpace(vlanIDStr)  // Remove any spaces
			vlanID, err := strconv.Atoi(vlanIDStr)
			if err == nil {
				if svi, exists := ms.svis[vlanID]; exists {
					output := fmt.Sprintf("Building configuration...\r\n\r\n")
					output += fmt.Sprintf("Current configuration : 100 bytes\r\n!\r\n")
					output += fmt.Sprintf("interface Vlan%d\r\n", svi.VlanID)
					if svi.Description != "" {
						output += fmt.Sprintf(" description %s\r\n", svi.Description)
					}
					output += fmt.Sprintf(" ip address %s %s\r\n", svi.IPAddress, svi.SubnetMask)
					for _, dhcpServer := range svi.DHCPServers {
						output += fmt.Sprintf(" ip helper-address %s\r\n", dhcpServer)
					}
					if svi.AdminState == "down" {
						output += fmt.Sprintf(" shutdown\r\n")
					} else {
						output += fmt.Sprintf(" no shutdown\r\n")
					}
					output += fmt.Sprintf("end\r\n")
					channel.Write([]byte(output))
				} else {
					channel.Write([]byte(fmt.Sprintf("%%Interface Vlan%d does not exist\r\n", vlanID)))
				}
			}
		} else {
			// Regular interface
			if iface, exists := ms.ifaces[ifaceName]; exists {
				output := fmt.Sprintf("Building configuration...\r\n\r\n")
				output += fmt.Sprintf("Current configuration : 200 bytes\r\n!\r\n")
				output += fmt.Sprintf("interface %s\r\n", iface.Name)
				if iface.Description != "" {
					output += fmt.Sprintf(" description %s\r\n", iface.Description)
				}
				output += fmt.Sprintf(" switchport mode %s\r\n", iface.Mode)
				if iface.Mode == "access" {
					output += fmt.Sprintf(" switchport access vlan %d\r\n", iface.AccessVLAN)
				} else if iface.Mode == "trunk" {
					vlanList := ""
					for i, v := range iface.TrunkVLANs {
						if i > 0 {
							vlanList += ","
						}
						vlanList += fmt.Sprintf("%d", v)
					}
					output += fmt.Sprintf(" switchport trunk allowed vlan %s\r\n", vlanList)
					if iface.NativeVLAN != 1 {
						output += fmt.Sprintf(" switchport trunk native vlan %d\r\n", iface.NativeVLAN)
					}
				}
				if iface.AdminState == "down" {
					output += fmt.Sprintf(" shutdown\r\n")
				} else {
					output += fmt.Sprintf(" no shutdown\r\n")
				}
				output += fmt.Sprintf("end\r\n")
				channel.Write([]byte(output))
			} else {
				channel.Write([]byte(fmt.Sprintf("%%Interface %s does not exist\r\n", ifaceName)))
			}
		}
	}
}

func (ms *MockSwitch) handleConfigCommand(channel ssh.Channel, cmd, context string) (string, string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	// VLAN commands
	if strings.HasPrefix(cmd, "vlan ") {
		vlanIDStr := strings.TrimPrefix(cmd, "vlan ")
		vlanID, err := strconv.Atoi(vlanIDStr)
		if err == nil && vlanID >= 1 && vlanID <= 4094 {
			if _, exists := ms.vlans[vlanID]; !exists {
				ms.vlans[vlanID] = &VLAN{ID: vlanID, Name: fmt.Sprintf("VLAN%04d", vlanID), State: "active"}
			}
			return "config", fmt.Sprintf("config-vlan:%d", vlanID)
		}
	}

	if strings.HasPrefix(cmd, "no vlan ") {
		vlanIDStr := strings.TrimPrefix(cmd, "no vlan ")
		vlanID, err := strconv.Atoi(vlanIDStr)
		if err == nil {
			delete(ms.vlans, vlanID)
		}
		return "config", ""
	}

	if strings.HasPrefix(context, "config-vlan:") {
		vlanIDStr := strings.TrimPrefix(context, "config-vlan:")
		vlanID, _ := strconv.Atoi(vlanIDStr)

		if vlan, exists := ms.vlans[vlanID]; exists {
			if strings.HasPrefix(cmd, "name ") {
				name := strings.TrimPrefix(cmd, "name ")
				vlan.Name = name
			}
			if strings.HasPrefix(cmd, "state ") {
				state := strings.TrimPrefix(cmd, "state ")
				vlan.State = state
			}
		}

		if cmd == "exit" {
			return "config", ""
		}
		return "config", context
	}

	// Interface commands
	if strings.HasPrefix(cmd, "interface ") {
		ifaceName := strings.TrimPrefix(cmd, "interface ")
		ifaceName = strings.TrimSpace(ifaceName)

		// Check if it's an SVI
		if strings.HasPrefix(ifaceName, "Vlan") || strings.HasPrefix(ifaceName, "vlan") {
			vlanIDStr := strings.TrimPrefix(strings.ToLower(ifaceName), "vlan")
			vlanIDStr = strings.TrimSpace(vlanIDStr)  // Remove any spaces
			vlanID, err := strconv.Atoi(vlanIDStr)
			if err == nil {
				if _, exists := ms.svis[vlanID]; !exists {
					ms.svis[vlanID] = &SVI{VlanID: vlanID, AdminState: "up"}
				}
				return "config", fmt.Sprintf("config-if:vlan%d", vlanID)
			}
		} else {
			// Regular interface
			if _, exists := ms.ifaces[ifaceName]; !exists {
				ms.ifaces[ifaceName] = &Interface{
					Name:       ifaceName,
					Mode:       "access",
					AccessVLAN: 1,
					AdminState: "up",
					NativeVLAN: 1,
				}
			}
			return "config", fmt.Sprintf("config-if:%s", ifaceName)
		}
	}

	if strings.HasPrefix(cmd, "no interface ") {
		ifaceName := strings.TrimPrefix(cmd, "no interface ")
		ifaceName = strings.TrimSpace(ifaceName)

		if strings.HasPrefix(ifaceName, "Vlan") || strings.HasPrefix(ifaceName, "vlan") {
			vlanIDStr := strings.TrimPrefix(strings.ToLower(ifaceName), "vlan")
			vlanID, _ := strconv.Atoi(vlanIDStr)
			delete(ms.svis, vlanID)
		} else {
			delete(ms.ifaces, ifaceName)
		}
		return "config", ""
	}

	if strings.HasPrefix(cmd, "default interface ") {
		ifaceName := strings.TrimPrefix(cmd, "default interface ")
		delete(ms.ifaces, strings.TrimSpace(ifaceName))
		return "config", ""
	}

	// Interface config commands
	if strings.HasPrefix(context, "config-if:") {
		ifaceName := strings.TrimPrefix(context, "config-if:")

		// Check if it's an SVI
		if strings.HasPrefix(ifaceName, "vlan") {
			vlanIDStr := strings.TrimPrefix(ifaceName, "vlan")
			vlanID, _ := strconv.Atoi(vlanIDStr)
			if svi, exists := ms.svis[vlanID]; exists {
				ms.handleSVIConfigCommand(svi, cmd)
			}
		} else {
			// Regular interface
			if iface, exists := ms.ifaces[ifaceName]; exists {
				ms.handleInterfaceConfigCommand(iface, cmd)
			}
		}

		if cmd == "exit" {
			return "config", ""
		}
		return "config", context
	}

	return "config", context
}

func (ms *MockSwitch) handleInterfaceConfigCommand(iface *Interface, cmd string) {
	if strings.HasPrefix(cmd, "description ") {
		iface.Description = strings.TrimPrefix(cmd, "description ")
	} else if cmd == "switchport mode access" {
		iface.Mode = "access"
	} else if cmd == "switchport mode trunk" {
		iface.Mode = "trunk"
	} else if strings.HasPrefix(cmd, "switchport access vlan ") {
		vlanStr := strings.TrimPrefix(cmd, "switchport access vlan ")
		vlan, _ := strconv.Atoi(vlanStr)
		iface.AccessVLAN = vlan
	} else if strings.HasPrefix(cmd, "switchport trunk allowed vlan ") {
		vlanStr := strings.TrimPrefix(cmd, "switchport trunk allowed vlan ")
		iface.TrunkVLANs = parseVLANList(vlanStr)
	} else if strings.HasPrefix(cmd, "switchport trunk native vlan ") {
		vlanStr := strings.TrimPrefix(cmd, "switchport trunk native vlan ")
		vlan, _ := strconv.Atoi(vlanStr)
		iface.NativeVLAN = vlan
	} else if cmd == "shutdown" {
		iface.AdminState = "down"
	} else if cmd == "no shutdown" {
		iface.AdminState = "up"
	}
}

func (ms *MockSwitch) handleSVIConfigCommand(svi *SVI, cmd string) {
	if strings.HasPrefix(cmd, "description ") {
		svi.Description = strings.TrimPrefix(cmd, "description ")
	} else if strings.HasPrefix(cmd, "ip address ") {
		// Parse "ip address 192.168.1.1 255.255.255.0"
		re := regexp.MustCompile(`ip address (\S+) (\S+)`)
		matches := re.FindStringSubmatch(cmd)
		if len(matches) == 3 {
			svi.IPAddress = matches[1]
			svi.SubnetMask = matches[2]
		}
	} else if strings.HasPrefix(cmd, "ip helper-address ") {
		// Parse "ip helper-address 10.0.0.1"
		re := regexp.MustCompile(`ip helper-address (\S+)`)
		matches := re.FindStringSubmatch(cmd)
		if len(matches) == 2 {
			svi.DHCPServers = append(svi.DHCPServers, matches[1])
		}
	} else if cmd == "shutdown" {
		svi.AdminState = "down"
	} else if cmd == "no shutdown" {
		svi.AdminState = "up"
	}
}

func parseVLANList(s string) []int {
	var vlans []int
	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) == 2 {
				start, _ := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
				end, _ := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
				for i := start; i <= end; i++ {
					vlans = append(vlans, i)
				}
			}
		} else {
			vlan, _ := strconv.Atoi(part)
			if vlan > 0 {
				vlans = append(vlans, vlan)
			}
		}
	}
	return vlans
}

func generateSSHKey() (ssh.Signer, error) {
	// Generate a temporary RSA key for testing
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	return ssh.NewSignerFromKey(key)
}
