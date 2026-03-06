package client

import (
	"fmt"
	"strings"
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
	PrivateKeyPath string
	SSHTimeout     time.Duration
	CommandTimeout time.Duration
}

// Client represents a Cisco IOS CLI client.
//
// opMu serialises full resource operations (Create/Update/Delete) so that
// commands from different Terraform goroutines never interleave on the switch.
// mu serialises individual SSH command reads/writes within an operation.
// Always acquire opMu before mu — they are never held in the reverse order.
type Client struct {
	config         Config
	sshClient      *ssh.Client
	session        *ssh.Session
	stdin          sshWriter
	stdout         sshReader
	currentMode    CLIMode
	hostname       string
	opMu           sync.Mutex // operation-level: held for the duration of a resource Create/Update/Delete
	mu             sync.Mutex // command-level: held while sending a single SSH command
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

// Lock acquires the operation-level mutex. Every resource Create/Update/Delete
// must call Lock at the start and defer Unlock so that operations are
// serialised end-to-end. This prevents interleaved SSH traffic on the switch.
func (c *Client) Lock() {
	c.opMu.Lock()
}

// Unlock releases the operation-level mutex.
func (c *Client) Unlock() {
	c.opMu.Unlock()
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

// reconnect drops the current session and establishes a fresh one.
// Must be called with c.mu held.
func (c *Client) reconnect() error {
	c.disconnect()
	if err := c.connect(); err != nil {
		return fmt.Errorf("reconnect: %w", err)
	}
	if err := c.enterPrivilegedMode(); err != nil {
		c.disconnect()
		return fmt.Errorf("reconnect: %w", err)
	}
	c.connected = true
	return nil
}

// isConnectionError returns true for errors that indicate the SSH session has dropped.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "eof") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "read error")
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

// executeCommand is the low-level command sender. Must be called with c.mu held.
func (c *Client) executeCommand(command string) (string, error) {
	if !c.connected {
		return "", fmt.Errorf("not connected")
	}
	return c.sendCommand(command)
}

// ExecuteCommand executes a single show/exec command in privileged mode.
// Retries once on connection errors.
func (c *Client) ExecuteCommand(command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for attempt := 0; attempt <= 1; attempt++ {
		if !c.connected {
			return "", fmt.Errorf("not connected")
		}

		if c.currentMode != ModePrivileged {
			if err := c.returnToPrivilegedMode(); err != nil {
				if attempt == 0 && isConnectionError(err) {
					if reconnErr := c.reconnect(); reconnErr != nil {
						return "", reconnErr
					}
					continue
				}
				return "", err
			}
		}

		output, err := c.sendCommand(command)
		if err != nil {
			if attempt == 0 && isConnectionError(err) {
				if reconnErr := c.reconnect(); reconnErr != nil {
					return "", reconnErr
				}
				continue
			}
			return "", &CiscoError{Operation: "execute", Command: command, Err: err}
		}

		if isError, errMsg := IsErrorOutput(output); isError {
			return output, &CiscoError{Operation: "execute", Command: command, Output: errMsg}
		}

		return output, nil
	}

	return "", fmt.Errorf("failed after reconnect")
}

// ExecuteConfigCommands executes a series of IOS config-mode commands.
// Changes are NOT saved to startup-config automatically — the caller must
// apply a cisco_write_memory resource to persist them.
// Retries once on connection errors.
func (c *Client) ExecuteConfigCommands(commands []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for attempt := 0; attempt <= 1; attempt++ {
		if !c.connected {
			return fmt.Errorf("not connected")
		}

		if err := c.runConfigCommands(commands); err != nil {
			if attempt == 0 && isConnectionError(err) {
				if reconnErr := c.reconnect(); reconnErr != nil {
					return reconnErr
				}
				continue
			}
			return err
		}

		return nil
	}

	return fmt.Errorf("failed after reconnect")
}

// runConfigCommands enters config mode, sends commands, and returns to
// privileged mode. Must be called with c.mu held.
func (c *Client) runConfigCommands(commands []string) error {
	if err := c.enterConfigMode(); err != nil {
		return err
	}

	for _, cmd := range commands {
		output, err := c.sendCommand(cmd)
		if err != nil {
			c.returnToPrivilegedMode()
			return &CiscoError{Operation: "config", Command: cmd, Err: err}
		}

		if isError, errMsg := IsErrorOutput(output); isError {
			c.returnToPrivilegedMode()
			return &CiscoError{Operation: "config", Command: cmd, Output: errMsg}
		}
	}

	return c.returnToPrivilegedMode()
}

// WriteMemory saves the running-config to startup-config ("write memory").
// This is the only way changes are persisted. Call it via the
// cisco_write_memory resource after all other resources have been applied.
func (c *Client) WriteMemory() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected")
	}
	return c.writeMemory()
}

// writeMemory is the internal implementation. Must be called with c.mu held.
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
