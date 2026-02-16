package client

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// connect establishes the SSH connection and session
func (c *Client) connect() error {
	// Create SSH client config
	config := &ssh.ClientConfig{
		User: c.config.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(c.config.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // In production, should verify host key
		Timeout:         c.config.SSHTimeout,
	}

	// Connect to the switch
	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
	conn, err := net.DialTimeout("tcp", addr, c.config.SSHTimeout)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create SSH connection: %w", err)
	}

	c.sshClient = ssh.NewClient(sshConn, chans, reqs)

	// Create SSH session
	session, err := c.sshClient.NewSession()
	if err != nil {
		c.sshClient.Close()
		c.sshClient = nil
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Request PTY (pseudo-terminal)
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // Disable echo
		ssh.TTY_OP_ISPEED: 14400, // Input speed
		ssh.TTY_OP_OSPEED: 14400, // Output speed
	}

	if err := session.RequestPty("vt100", 60, 200, modes); err != nil {
		session.Close()
		c.sshClient.Close()
		c.sshClient = nil
		return fmt.Errorf("failed to request PTY: %w", err)
	}

	// Get stdin/stdout pipes
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		c.sshClient.Close()
		c.sshClient = nil
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		c.sshClient.Close()
		c.sshClient = nil
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Start shell
	if err := session.Shell(); err != nil {
		session.Close()
		c.sshClient.Close()
		c.sshClient = nil
		return fmt.Errorf("failed to start shell: %w", err)
	}

	c.session = session
	c.stdin = stdin
	c.stdout = stdout

	// Wait for initial prompt to detect mode and hostname
	output, err := c.readUntilPrompt()
	if err != nil {
		return fmt.Errorf("failed to read initial prompt: %w", err)
	}

	// Detect initial mode and hostname
	mode, hostname := c.detectModeFromPrompt(output)
	c.currentMode = mode
	c.hostname = hostname

	// Disable paging
	if err := c.disablePaging(); err != nil {
		return fmt.Errorf("failed to disable paging: %w", err)
	}

	return nil
}

// disablePaging disables paging (--More-- prompts) for the session
func (c *Client) disablePaging() error {
	// Send "terminal length 0" command to disable paging
	var cmd string
	switch c.currentMode {
	case ModeUser:
		// In user mode, we need to enable first
		cmd = "enable\n"
		if _, err := c.stdin.Write([]byte(cmd)); err != nil {
			return err
		}

		// Wait for password prompt or privileged prompt
		output, err := c.readUntilPrompt()
		if err != nil {
			return err
		}

		// Check if we need to enter enable password
		if containsString(output, "Password:") {
			if c.config.EnablePassword == "" {
				return fmt.Errorf("enable password required but not provided")
			}
			cmd = c.config.EnablePassword + "\n"
			if _, err := c.stdin.Write([]byte(cmd)); err != nil {
				return err
			}
			if _, err := c.readUntilPrompt(); err != nil {
				return err
			}
		}

		// Update mode
		c.currentMode = ModePrivileged

	case ModePrivileged:
		// Already in privileged mode
	default:
		// Return to privileged mode first
		if err := c.returnToPrivilegedMode(); err != nil {
			return err
		}
	}

	// Now disable paging
	cmd = "terminal length 0\n"
	if _, err := c.stdin.Write([]byte(cmd)); err != nil {
		return err
	}

	_, err := c.readUntilPrompt()
	return err
}

// sendCommand sends a command and waits for the prompt
func (c *Client) sendCommand(command string) (string, error) {
	// Send command
	cmd := command + "\n"
	if _, err := c.stdin.Write([]byte(cmd)); err != nil {
		return "", fmt.Errorf("failed to write command: %w", err)
	}

	// Read output until prompt
	output, err := c.readUntilPrompt()
	if err != nil {
		return "", fmt.Errorf("failed to read output: %w", err)
	}

	// Update current mode based on the prompt we received
	mode, _ := c.detectModeFromPrompt(output)
	c.currentMode = mode

	// Clean up output (remove command echo and trailing prompt)
	cleaned := c.cleanOutput(output, command)

	return cleaned, nil
}

// readUntilPrompt reads from stdout until a prompt is detected
func (c *Client) readUntilPrompt() (string, error) {
	var output []byte
	buf := make([]byte, 4096)
	timeout := time.After(c.commandTimeout)

	for {
		select {
		case <-timeout:
			return string(output), fmt.Errorf("timeout waiting for prompt")
		default:
			// Set read deadline
			if session, ok := interface{}(c.session).(*ssh.Session); ok {
				_ = session
			}

			n, err := c.stdout.Read(buf)
			if err != nil {
				return string(output), fmt.Errorf("read error: %w", err)
			}

			if n > 0 {
				output = append(output, buf[:n]...)

				// Check if we have a prompt
				if c.hasPrompt(string(output)) {
					return string(output), nil
				}

				// Check for --More-- pagination
				if containsString(string(output), "--More--") {
					// Send space to continue
					if _, err := c.stdin.Write([]byte(" ")); err != nil {
						return string(output), err
					}
					// Remove the --More-- prompt from output
					output = []byte(removeMore(string(output)))
					continue
				}
			}

			// Small delay to avoid busy waiting
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// removeMore removes --More-- prompts and related ANSI codes from output
func removeMore(s string) string {
	// This is a simple implementation - in production would use regex
	// to handle ANSI escape sequences properly
	result := ""
	lines := splitLines(s)
	for _, line := range lines {
		if !containsString(line, "--More--") {
			result += line + "\n"
		}
	}
	return result
}

func splitLines(s string) []string {
	var lines []string
	current := ""
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, current)
			current = ""
		} else if s[i] == '\r' {
			// Skip carriage returns
			continue
		} else {
			current += string(s[i])
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
