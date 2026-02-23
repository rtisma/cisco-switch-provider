package client

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Prompt patterns for different CLI modes
	userPromptRegex       = regexp.MustCompile(`\w+>\s*$`)
	privilegedPromptRegex = regexp.MustCompile(`\w+#\s*$`)
	configPromptRegex     = regexp.MustCompile(`\w+\(config\)#\s*$`)
	configIfPromptRegex   = regexp.MustCompile(`\w+\(config-if\)#\s*$`)
	configVlanPromptRegex = regexp.MustCompile(`\w+\(config-vlan\)#\s*$`)
	configDhcpPromptRegex    = regexp.MustCompile(`\w+\(dhcp-config\)#\s*$`)
	configExtNaclPromptRegex = regexp.MustCompile(`\w+\(config-ext-nacl\)#\s*$`)
	configStdNaclPromptRegex = regexp.MustCompile(`\w+\(config-std-nacl\)#\s*$`)
	hostnameRegex            = regexp.MustCompile(`^(\w+)[>#]`)
)

// hasPrompt checks if the output ends with a CLI prompt
func (c *Client) hasPrompt(output string) bool {
	// Get the last line
	lines := splitLines(output)
	if len(lines) == 0 {
		return false
	}
	lastLine := lines[len(lines)-1]

	// Check against all prompt patterns
	return userPromptRegex.MatchString(lastLine) ||
		privilegedPromptRegex.MatchString(lastLine) ||
		configPromptRegex.MatchString(lastLine) ||
		configIfPromptRegex.MatchString(lastLine) ||
		configVlanPromptRegex.MatchString(lastLine) ||
		configDhcpPromptRegex.MatchString(lastLine) ||
		configExtNaclPromptRegex.MatchString(lastLine) ||
		configStdNaclPromptRegex.MatchString(lastLine)
}

// detectModeFromPrompt detects the CLI mode and hostname from prompt output
func (c *Client) detectModeFromPrompt(output string) (CLIMode, string) {
	lines := splitLines(output)
	if len(lines) == 0 {
		return ModeUnknown, ""
	}

	lastLine := lines[len(lines)-1]
	lastLine = strings.TrimSpace(lastLine)

	// Extract hostname
	hostname := ""
	if matches := hostnameRegex.FindStringSubmatch(lastLine); len(matches) > 1 {
		hostname = matches[1]
	}

	// Detect mode
	mode := ModeUnknown
	switch {
	case configExtNaclPromptRegex.MatchString(lastLine):
		mode = ModeConfigNacl
	case configStdNaclPromptRegex.MatchString(lastLine):
		mode = ModeConfigNacl
	case configDhcpPromptRegex.MatchString(lastLine):
		mode = ModeConfigDhcp
	case configVlanPromptRegex.MatchString(lastLine):
		mode = ModeConfigVlan
	case configIfPromptRegex.MatchString(lastLine):
		mode = ModeConfigIf
	case configPromptRegex.MatchString(lastLine):
		mode = ModeConfig
	case privilegedPromptRegex.MatchString(lastLine):
		mode = ModePrivileged
	case userPromptRegex.MatchString(lastLine):
		mode = ModeUser
	}

	return mode, hostname
}

// enterPrivilegedMode enters privileged exec mode from user mode
func (c *Client) enterPrivilegedMode() error {
	if c.currentMode == ModePrivileged {
		return nil
	}

	if c.currentMode != ModeUser {
		return fmt.Errorf("cannot enter privileged mode from %s mode", c.currentMode)
	}

	// Send enable command
	output, err := c.sendCommand("enable")
	if err != nil {
		return fmt.Errorf("failed to send enable command: %w", err)
	}

	// Check if password is required
	if containsString(output, "Password:") {
		if c.config.EnablePassword == "" {
			return fmt.Errorf("enable password required but not provided")
		}

		// Send password
		if _, err := c.stdin.Write([]byte(c.config.EnablePassword + "\n")); err != nil {
			return fmt.Errorf("failed to send enable password: %w", err)
		}

		// Read response
		output, err = c.readUntilPrompt()
		if err != nil {
			return fmt.Errorf("failed to read after enable password: %w", err)
		}
	}

	// Update current mode
	mode, _ := c.detectModeFromPrompt(output)
	if mode != ModePrivileged {
		return fmt.Errorf("failed to enter privileged mode, got mode: %s", mode)
	}

	c.currentMode = ModePrivileged
	return nil
}

// enterConfigMode enters global configuration mode
func (c *Client) enterConfigMode() error {
	if c.currentMode == ModeConfig {
		return nil
	}

	// Ensure we're in privileged mode first
	if c.currentMode != ModePrivileged {
		if err := c.returnToPrivilegedMode(); err != nil {
			return err
		}
	}

	// Enter config mode
	_, err := c.sendCommand("configure terminal")
	if err != nil {
		return fmt.Errorf("failed to enter config mode: %w", err)
	}

	// Mode is updated automatically by sendCommand
	// Verify we're in config mode
	if c.currentMode != ModeConfig {
		return fmt.Errorf("failed to enter config mode, got mode: %s", c.currentMode)
	}

	return nil
}

// returnToPrivilegedMode returns to privileged exec mode from any config mode
func (c *Client) returnToPrivilegedMode() error {
	if c.currentMode == ModePrivileged {
		return nil
	}

	// Exit from current mode(s) using "end" command
	_, err := c.sendCommand("end")
	if err != nil {
		return fmt.Errorf("failed to exit to privileged mode: %w", err)
	}

	// Mode is updated automatically by sendCommand
	// Verify we're in privileged mode
	if c.currentMode != ModePrivileged {
		// Try one more time with multiple "exit" commands
		for i := 0; i < 5; i++ {
			_, err = c.sendCommand("exit")
			if err != nil {
				continue
			}
			if c.currentMode == ModePrivileged {
				break
			}
		}

		if c.currentMode != ModePrivileged {
			return fmt.Errorf("failed to return to privileged mode, stuck in: %s", c.currentMode)
		}
	}

	return nil
}

// cleanOutput removes command echo and prompt from command output
func (c *Client) cleanOutput(output, command string) string {
	lines := splitLines(output)
	if len(lines) == 0 {
		return ""
	}

	// Remove first line if it's the command echo
	start := 0
	if len(lines) > 0 && containsString(lines[0], command) {
		start = 1
	}

	// Remove last line if it's the prompt
	end := len(lines)
	if end > 0 && c.hasPrompt(lines[end-1]) {
		end--
	}

	// Join remaining lines
	result := ""
	for i := start; i < end; i++ {
		if lines[i] != "" {
			result += lines[i] + "\n"
		}
	}

	return strings.TrimSpace(result)
}

// GetCurrentMode returns the current CLI mode
func (c *Client) GetCurrentMode() CLIMode {
	return c.currentMode
}
