package client

import (
	"fmt"
)

// CiscoError represents an error from Cisco CLI operations
type CiscoError struct {
	Operation string
	Command   string
	Output    string
	Err       error
}

func (e *CiscoError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("cisco %s error: %s (command: %q, output: %q)",
			e.Operation, e.Err.Error(), e.Command, e.Output)
	}
	return fmt.Sprintf("cisco %s error: %s (command: %q)",
		e.Operation, e.Output, e.Command)
}

func (e *CiscoError) Unwrap() error {
	return e.Err
}

// Common error patterns in Cisco IOS output
var errorPatterns = []string{
	"Invalid input detected",
	"Incomplete command",
	"% Ambiguous command",
	"Command rejected",
	"% Access denied",
	"% Bad IP address",
	"% VLAN does not exist",
	"% Interface does not exist",
	"% Cannot delete VLAN",
}

// IsErrorOutput checks if the command output contains an error
func IsErrorOutput(output string) (bool, string) {
	for _, pattern := range errorPatterns {
		if containsString(output, pattern) {
			return true, pattern
		}
	}
	return false, ""
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
