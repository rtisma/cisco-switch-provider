package client

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// CLIMode represents the current CLI mode
type CLIMode int

const (
	ModeUnknown CLIMode = iota
	ModeUser          // Router>
	ModePrivileged    // Router#
	ModeConfig        // Router(config)#
	ModeConfigIf      // Router(config-if)#
	ModeConfigVlan    // Router(config-vlan)#
	ModeConfigDhcp    // Router(dhcp-config)#
	ModeConfigNacl    // Router(config-ext-nacl)# or Router(config-std-nacl)#
)

// String returns the string representation of the CLI mode
func (m CLIMode) String() string {
	switch m {
	case ModeUser:
		return "user"
	case ModePrivileged:
		return "privileged"
	case ModeConfig:
		return "config"
	case ModeConfigIf:
		return "config-if"
	case ModeConfigVlan:
		return "config-vlan"
	case ModeConfigDhcp:
		return "dhcp-config"
	case ModeConfigNacl:
		return "config-nacl"
	default:
		return "unknown"
	}
}

// Config holds the configuration for the Cisco client
type Config struct {
	Host           string
	Port           int
	Username       string
	Password       string
	EnablePassword string
	SSHTimeout     time.Duration
	CommandTimeout time.Duration
}

// Client represents a Cisco IOS CLI client
type Client struct {
	config         Config
	sshClient      *ssh.Client
	session        *ssh.Session
	stdin          sshWriter
	stdout         sshReader
	currentMode    CLIMode
	hostname       string
	mu             sync.Mutex
	connected      bool
	commandTimeout time.Duration
}

type sshWriter interface {
	Write([]byte) (int, error)
}

type sshReader interface {
	Read([]byte) (int, error)
}

// NewClient creates a new Cisco client
func NewClient(config Config) *Client {
	// Set defaults
	if config.Port == 0 {
		config.Port = 22
	}
	if config.SSHTimeout == 0 {
		config.SSHTimeout = 30 * time.Second
	}
	if config.CommandTimeout == 0 {
		config.CommandTimeout = 10 * time.Second
	}

	return &Client{
		config:         config,
		currentMode:    ModeUnknown,
		commandTimeout: config.CommandTimeout,
	}
}

// Connect establishes SSH connection and initializes the session
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	if err := c.connect(); err != nil {
		return &CiscoError{
			Operation: "connect",
			Err:       err,
		}
	}

	// Enter privileged mode
	if err := c.enterPrivilegedMode(); err != nil {
		c.disconnect()
		return err
	}

	c.connected = true
	return nil
}

// Disconnect closes the SSH connection
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.disconnect()
}

func (c *Client) disconnect() error {
	c.connected = false
	c.currentMode = ModeUnknown

	if c.session != nil {
		c.session.Close()
		c.session = nil
	}

	if c.sshClient != nil {
		c.sshClient.Close()
		c.sshClient = nil
	}

	return nil
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// GetHostname returns the detected hostname of the switch
func (c *Client) GetHostname() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hostname
}

// executeCommand executes a command and returns the output
// This is a low-level method that should be called with the mutex held
func (c *Client) executeCommand(command string) (string, error) {
	if !c.connected {
		return "", fmt.Errorf("not connected")
	}

	return c.sendCommand(command)
}

// ExecuteCommand executes a command in privileged mode
func (c *Client) ExecuteCommand(command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return "", fmt.Errorf("not connected")
	}

	// Ensure we're in privileged mode
	if c.currentMode != ModePrivileged {
		if err := c.returnToPrivilegedMode(); err != nil {
			return "", err
		}
	}

	output, err := c.sendCommand(command)
	if err != nil {
		return "", &CiscoError{
			Operation: "execute",
			Command:   command,
			Err:       err,
		}
	}

	// Check for errors in output
	if isError, errMsg := IsErrorOutput(output); isError {
		return output, &CiscoError{
			Operation: "execute",
			Command:   command,
			Output:    errMsg,
		}
	}

	return output, nil
}

// ExecuteConfigCommands executes a series of commands in config mode and then
// saves the running configuration to startup-config via "write memory" so that
// changes survive a reboot.
func (c *Client) ExecuteConfigCommands(commands []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}

	// Enter config mode
	if err := c.enterConfigMode(); err != nil {
		return err
	}

	// Execute each command
	for _, cmd := range commands {
		output, err := c.sendCommand(cmd)
		if err != nil {
			c.returnToPrivilegedMode()
			return &CiscoError{
				Operation: "config",
				Command:   cmd,
				Err:       err,
			}
		}

		// Check for errors in output
		if isError, errMsg := IsErrorOutput(output); isError {
			c.returnToPrivilegedMode()
			return &CiscoError{
				Operation: "config",
				Command:   cmd,
				Output:    errMsg,
			}
		}
	}

	// Return to privileged mode
	if err := c.returnToPrivilegedMode(); err != nil {
		return err
	}

	// Persist changes to startup-config so they survive a reboot.
	return c.writeMemory()
}

// writeMemory saves running-config to startup-config ("write memory").
// Must be called with c.mu held and while in privileged mode.
func (c *Client) writeMemory() error {
	output, err := c.sendCommand("write memory")
	if err != nil {
		return fmt.Errorf("write memory failed: %w", err)
	}
	if isError, errMsg := IsErrorOutput(output); isError {
		return fmt.Errorf("write memory error: %s", errMsg)
	}
	return nil
}
