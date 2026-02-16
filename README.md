# Terraform Provider for Cisco WS-C3650 Switches

A Terraform provider for managing Cisco WS-C3650 switches through SSH CLI automation. This provider enables infrastructure-as-code management of VLANs, interfaces, inter-VLAN routing, and IP addressing on Cisco IOS switches that don't have REST API support.

## Features

- **VLAN Management**: Create, update, and delete VLANs with full lifecycle management
- **Interface Configuration**: Configure switchports in access or trunk mode
- **Inter-VLAN Routing**: Manage Switch Virtual Interfaces (SVIs) for Layer 3 routing
- **IP Address Management**: Configure static or DHCP IP addresses on interfaces
- **State Management**: Full Terraform state support with drift detection
- **Import Support**: Import existing switch configurations into Terraform

## Requirements

- Terraform >= 1.0
- Go >= 1.21 (for building from source)
- Cisco WS-C3650 switch (or compatible IOS device)
- SSH access with enable mode privileges

## Installation

### From Source

```bash
git clone https://github.com/example-org/cisco-switch-provider.git
cd cisco-switch-provider
go build -o terraform-provider-cisco
```

### Install the Provider

```bash
# Create the provider directory
mkdir -p ~/.terraform.d/plugins/registry.terraform.io/example-org/cisco/1.0.0/darwin_arm64/

# Copy the binary
cp terraform-provider-cisco ~/.terraform.d/plugins/registry.terraform.io/example-org/cisco/1.0.0/darwin_arm64/

# Make it executable
chmod +x ~/.terraform.d/plugins/registry.terraform.io/example-org/cisco/1.0.0/darwin_arm64/terraform-provider-cisco
```

Note: Adjust the path based on your OS and architecture (e.g., `linux_amd64`, `darwin_amd64`).

## Usage

### Provider Configuration

```hcl
terraform {
  required_providers {
    cisco = {
      source = "registry.terraform.io/example-org/cisco"
    }
  }
}

provider "cisco" {
  host            = "192.168.1.1"      # Switch IP or hostname
  username        = var.cisco_username  # SSH username
  password        = var.cisco_password  # SSH password (sensitive)
  enable_password = var.cisco_enable_password # Enable mode password (sensitive)
  port            = 22                  # SSH port (default: 22)
  ssh_timeout     = 30                  # Connection timeout in seconds
  command_timeout = 10                  # Command timeout in seconds
}
```

### Resources

#### cisco_vlan

Manages a VLAN on the switch.

```hcl
resource "cisco_vlan" "sales" {
  vlan_id = 100
  name    = "Sales_Department"
  state   = "active"  # active or suspend (default: active)
}
```

**Arguments:**
- `vlan_id` (Required, Number) - VLAN ID (1-4094). Changing this forces a new resource.
- `name` (Required, String) - VLAN name
- `state` (Optional, String) - VLAN state: "active" or "suspend" (default: "active")

**Import:**
```bash
terraform import cisco_vlan.sales 100
```

#### cisco_interface

Manages a switchport interface configuration.

```hcl
# Access port example
resource "cisco_interface" "desktop_port" {
  name        = "GigabitEthernet1/0/1"
  description = "Desktop Computer"
  enabled     = true
  mode        = "access"
  access_vlan = cisco_vlan.sales.vlan_id
}

# Trunk port example
resource "cisco_interface" "uplink" {
  name        = "GigabitEthernet1/0/48"
  description = "Trunk to Core Switch"
  enabled     = true
  mode        = "trunk"
  trunk_vlans = [100, 200, 300]
  native_vlan = 1
}
```

**Arguments:**
- `name` (Required, String) - Interface name (e.g., "GigabitEthernet1/0/1"). Changing this forces a new resource.
- `description` (Optional, String) - Interface description
- `enabled` (Optional, Boolean) - Administrative state (true = no shutdown, false = shutdown). Default: true
- `mode` (Required, String) - Switchport mode: "access" or "trunk"
- `access_vlan` (Optional, Number) - VLAN for access mode (required when mode is "access")
- `trunk_vlans` (Optional, List of Numbers) - Allowed VLANs for trunk mode (required when mode is "trunk")
- `native_vlan` (Optional, Number) - Native VLAN for trunk mode (default: 1)

**Import:**
```bash
terraform import cisco_interface.desktop_port "GigabitEthernet1/0/1"
```

#### cisco_svi

Manages a Switch Virtual Interface (SVI) for inter-VLAN routing.

```hcl
resource "cisco_svi" "sales_gateway" {
  vlan_id     = cisco_vlan.sales.vlan_id
  ip_address  = "192.168.100.1"
  subnet_mask = "255.255.255.0"
  description = "Sales VLAN Gateway"
  enabled     = true
}
```

**Arguments:**
- `vlan_id` (Required, Number) - VLAN ID for the SVI (1-4094). Changing this forces a new resource.
- `ip_address` (Required, String) - IP address for the SVI
- `subnet_mask` (Required, String) - Subnet mask
- `description` (Optional, String) - Interface description
- `enabled` (Optional, Boolean) - Administrative state (default: true)

**Import:**
```bash
terraform import cisco_svi.sales_gateway 100
```

#### cisco_interface_ip

Manages IP address assignment on an interface (typically for management).

```hcl
# Static IP example
resource "cisco_interface_ip" "management" {
  interface   = "Vlan1"
  ip_address  = "192.168.1.10"
  subnet_mask = "255.255.255.0"
}

# DHCP example
resource "cisco_interface_ip" "management_dhcp" {
  interface = "Vlan1"
  dhcp      = true
}
```

**Arguments:**
- `interface` (Required, String) - Interface name (e.g., "Vlan1"). Changing this forces a new resource.
- `ip_address` (Optional, String) - Static IP address (mutually exclusive with dhcp)
- `subnet_mask` (Optional, String) - Subnet mask (required with ip_address)
- `dhcp` (Optional, Boolean) - Use DHCP for IP address (mutually exclusive with ip_address). Default: false

**Import:**
```bash
terraform import cisco_interface_ip.management "Vlan1"
```

## Complete Example

See the [examples/complete](examples/complete/) directory for a full working example that demonstrates:
- Creating multiple VLANs
- Configuring inter-VLAN routing with SVIs
- Setting up access ports
- Configuring trunk ports
- Managing management interface IP

## How It Works

### SSH CLI Automation

The provider connects to Cisco switches via SSH and executes IOS CLI commands to manage configuration:

1. **Connection**: Establishes SSH connection with PTY (pseudo-terminal) support
2. **Authentication**: Authenticates with username/password and enters enable mode
3. **Command Execution**: Executes configuration commands in the appropriate CLI mode
4. **Output Parsing**: Parses command output to maintain Terraform state
5. **State Management**: Reads current configuration to detect drift

### CLI Mode Management

The provider automatically handles Cisco IOS CLI modes:
- **User Mode** (`Router>`) → Initial login
- **Privileged Mode** (`Router#`) → After `enable` command
- **Config Mode** (`Router(config)#`) → For configuration changes
- **Interface/VLAN Config** → Sub-configuration modes

### Error Handling

The provider detects and reports common Cisco IOS errors:
- Invalid input detected
- Incomplete commands
- Access denied
- Non-existent VLANs/interfaces

## Limitations

- **Sequential Operations**: Single SSH session means operations are sequential (not parallel)
- **No Auto-Save**: Configuration changes are not automatically saved to startup-config
- **No Rollback**: Partial failures require manual recovery
- **IOS Version**: Tested on IOS 15.x; other versions may have different command output formats

## Security Considerations

- Use `sensitive = true` for passwords in variable definitions
- Never commit credentials to version control
- Consider using Terraform Cloud/Enterprise for secure variable storage
- The provider uses `ssh.InsecureIgnoreHostKey()` - in production, implement proper host key verification

## Development

### Building

```bash
go build -o terraform-provider-cisco
```

### Testing

```bash
go test ./...
```

### Mock SSH Server

The `tests/mock` directory contains a mock SSH server for testing without real hardware.

## Architecture

```
cisco-switch-provider/
├── main.go                           # Provider entry point
├── internal/provider/
│   ├── provider.go                   # Provider configuration
│   ├── client/                       # SSH client layer
│   │   ├── client.go                 # Main client struct
│   │   ├── ssh.go                    # SSH connection management
│   │   ├── session.go                # CLI mode handling
│   │   ├── parser.go                 # Output parsing
│   │   └── errors.go                 # Error handling
│   └── resources/                    # Terraform resources
│       ├── resource_vlan.go
│       ├── resource_interface.go
│       ├── resource_svi.go
│       └── resource_interface_ip.go
└── examples/                         # Usage examples
```

## Troubleshooting

### Connection Issues

```
Error: Unable to Connect to Cisco Switch
```

**Solution**: Verify:
- Switch IP/hostname is correct and reachable
- SSH is enabled on the switch (`ip ssh version 2`)
- Firewall rules allow SSH traffic
- Username and password are correct

### Authentication Issues

```
Error: enable password required but not provided
```

**Solution**: Provide the enable password in the provider configuration:
```hcl
provider "cisco" {
  enable_password = var.cisco_enable_password
}
```

### Command Timeouts

```
Error: timeout waiting for prompt
```

**Solution**: Increase command timeout:
```hcl
provider "cisco" {
  command_timeout = 30  # Increase from default 10 seconds
}
```

## Contributing

Contributions are welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

MIT License - see LICENSE file for details

## Author

Created by example-org

## Acknowledgments

- HashiCorp Terraform Plugin Framework
- Go SSH library (golang.org/x/crypto/ssh)
