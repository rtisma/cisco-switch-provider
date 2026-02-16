# Resource Documentation

Complete API reference for all resources and data sources in the Cisco Switch Provider.

## Table of Contents

1. [Provider Configuration](#provider-configuration)
2. [cisco_vlan](#cisco_vlan) - VLAN Management
3. [cisco_interface](#cisco_interface) - Switchport Configuration
4. [cisco_svi](#cisco_svi) - Switch Virtual Interface (Inter-VLAN Routing)
5. [cisco_interface_ip](#cisco_interface_ip) - IP Address Assignment
6. [Examples](#examples)
7. [Import](#import)

---

## Provider Configuration

The provider requires connection details to your Cisco switch.

### Schema

```hcl
provider "cisco" {
  host            = string  # Required
  port            = number  # Optional, default: 22
  username        = string  # Required
  password        = string  # Required, sensitive
  enable_password = string  # Optional, sensitive
  ssh_timeout     = number  # Optional, default: 30 (seconds)
  command_timeout = number  # Optional, default: 10 (seconds)
}
```

### Attributes

| Attribute | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `host` | string | **Yes** | - | IP address or hostname of the Cisco switch |
| `port` | number | No | `22` | SSH port number |
| `username` | string | **Yes** | - | SSH username for authentication |
| `password` | string | **Yes** | - | SSH password (marked as sensitive) |
| `enable_password` | string | No | - | Enable mode password if different from SSH password |
| `ssh_timeout` | number | No | `30` | SSH connection timeout in seconds (1-600) |
| `command_timeout` | number | No | `10` | Command execution timeout in seconds (1-600) |

### Example

```hcl
provider "cisco" {
  host            = "192.168.1.1"
  username        = var.cisco_username
  password        = var.cisco_password
  enable_password = var.cisco_enable_password
  port            = 22
  ssh_timeout     = 30
  command_timeout = 10
}

variable "cisco_username" {
  type      = string
  sensitive = true
}

variable "cisco_password" {
  type      = string
  sensitive = true
}

variable "cisco_enable_password" {
  type      = string
  sensitive = true
}
```

---

## cisco_vlan

Manages a VLAN on the Cisco switch. VLANs provide logical separation of network traffic.

### Schema

```hcl
resource "cisco_vlan" "example" {
  vlan_id = number  # Required, forces new resource
  name    = string  # Required
  state   = string  # Optional, default: "active"
}
```

### Attributes

| Attribute | Type | Required | Forces New | Default | Description |
|-----------|------|----------|------------|---------|-------------|
| `vlan_id` | number | **Yes** | **Yes** | - | VLAN ID (1-4094). Changing this creates a new VLAN |
| `name` | string | **Yes** | No | - | VLAN name (1-32 characters) |
| `state` | string | No | No | `"active"` | VLAN state: `"active"` or `"suspend"` |

### Attribute Reference

In addition to arguments, the following attributes are exported:

- `vlan_id` - The VLAN ID
- `name` - The VLAN name
- `state` - The VLAN state

### Cisco Commands

**Create:**
```
configure terminal
vlan <vlan_id>
name <name>
state <state>
end
```

**Read:**
```
show vlan id <vlan_id>
```

**Update:**
```
configure terminal
vlan <vlan_id>
name <new_name>
state <new_state>
end
```

**Delete:**
```
configure terminal
no vlan <vlan_id>
end
```

### Examples

#### Basic VLAN

```hcl
resource "cisco_vlan" "sales" {
  vlan_id = 100
  name    = "Sales_Department"
}
```

#### VLAN with Explicit State

```hcl
resource "cisco_vlan" "quarantine" {
  vlan_id = 999
  name    = "Quarantine_VLAN"
  state   = "suspend"
}
```

#### Multiple VLANs

```hcl
resource "cisco_vlan" "sales" {
  vlan_id = 100
  name    = "Sales"
  state   = "active"
}

resource "cisco_vlan" "engineering" {
  vlan_id = 200
  name    = "Engineering"
  state   = "active"
}

resource "cisco_vlan" "hr" {
  vlan_id = 300
  name    = "Human_Resources"
  state   = "active"
}
```

#### Using Count for Multiple VLANs

```hcl
variable "vlans" {
  type = map(object({
    id   = number
    name = string
  }))
  default = {
    sales  = { id = 100, name = "Sales" }
    eng    = { id = 200, name = "Engineering" }
    hr     = { id = 300, name = "HR" }
  }
}

resource "cisco_vlan" "departments" {
  for_each = var.vlans

  vlan_id = each.value.id
  name    = each.value.name
  state   = "active"
}
```

### Import

VLANs can be imported using the VLAN ID:

```bash
terraform import cisco_vlan.sales 100
```

### Notes

- VLAN 1 (default) cannot be deleted
- Some VLANs are reserved by the switch (e.g., 1002-1005)
- VLAN IDs must be unique on the switch
- Deleting a VLAN removes all port assignments

---

## cisco_interface

Manages a switchport interface configuration on the Cisco switch. Configures ports in either access or trunk mode.

### Schema

```hcl
resource "cisco_interface" "example" {
  name        = string       # Required, forces new resource
  description = string       # Optional
  enabled     = bool         # Optional, default: true
  mode        = string       # Required: "access" or "trunk"
  access_vlan = number       # Required if mode="access"
  trunk_vlans = list(number) # Required if mode="trunk"
  native_vlan = number       # Optional, default: 1
}
```

### Attributes

| Attribute | Type | Required | Forces New | Default | Description |
|-----------|------|----------|------------|---------|-------------|
| `name` | string | **Yes** | **Yes** | - | Interface name (e.g., "GigabitEthernet1/0/1") |
| `description` | string | No | No | - | Interface description (up to 240 characters) |
| `enabled` | bool | No | No | `true` | Administrative state (`true` = no shutdown, `false` = shutdown) |
| `mode` | string | **Yes** | No | - | Switchport mode: `"access"` or `"trunk"` |
| `access_vlan` | number | Conditional | No | - | VLAN for access mode (required when mode is "access") |
| `trunk_vlans` | list(number) | Conditional | No | - | Allowed VLANs for trunk mode (required when mode is "trunk") |
| `native_vlan` | number | No | No | `1` | Native VLAN for trunk mode |

### Attribute Reference

In addition to arguments, the following attributes are exported:

- All input attributes
- Configuration is synced from the switch on each read

### Validation Rules

- `mode` must be either "access" or "trunk"
- If `mode = "access"`, `access_vlan` is required
- If `mode = "trunk"`, `trunk_vlans` is required
- `trunk_vlans` cannot be empty
- VLAN IDs must be between 1 and 4094

### Cisco Commands

**Create/Update (Access Mode):**
```
configure terminal
interface <name>
description <description>
switchport mode access
switchport access vlan <access_vlan>
no shutdown
end
```

**Create/Update (Trunk Mode):**
```
configure terminal
interface <name>
description <description>
switchport mode trunk
switchport trunk allowed vlan <trunk_vlans>
switchport trunk native vlan <native_vlan>
no shutdown
end
```

**Read:**
```
show running-config interface <name>
show interface <name> status
```

**Delete:**
```
configure terminal
default interface <name>
end
```

### Examples

#### Access Port

```hcl
resource "cisco_interface" "desktop" {
  name        = "GigabitEthernet1/0/1"
  description = "Sales Desktop Computer"
  enabled     = true
  mode        = "access"
  access_vlan = cisco_vlan.sales.vlan_id
}
```

#### Disabled Port

```hcl
resource "cisco_interface" "unused_port" {
  name        = "GigabitEthernet1/0/48"
  description = "Unused - Disabled for Security"
  enabled     = false
  mode        = "access"
  access_vlan = 999  # Quarantine VLAN
}
```

#### Trunk Port

```hcl
resource "cisco_interface" "uplink" {
  name        = "GigabitEthernet1/0/24"
  description = "Trunk to Core Switch"
  enabled     = true
  mode        = "trunk"
  trunk_vlans = [
    cisco_vlan.sales.vlan_id,
    cisco_vlan.engineering.vlan_id,
    cisco_vlan.management.vlan_id,
  ]
  native_vlan = 1
}
```

#### Trunk with VLAN Range

```hcl
locals {
  all_vlans = range(10, 100)  # VLANs 10-99
}

resource "cisco_interface" "trunk_all" {
  name        = "GigabitEthernet1/0/23"
  description = "Trunk with All VLANs"
  enabled     = true
  mode        = "trunk"
  trunk_vlans = local.all_vlans
  native_vlan = 10
}
```

#### Dynamic Configuration

```hcl
variable "access_ports" {
  type = map(object({
    interface = string
    vlan_id   = number
    desc      = string
  }))
  default = {
    port1 = { interface = "GigabitEthernet1/0/1", vlan_id = 100, desc = "Sales Desk 1" }
    port2 = { interface = "GigabitEthernet1/0/2", vlan_id = 100, desc = "Sales Desk 2" }
    port3 = { interface = "GigabitEthernet1/0/3", vlan_id = 200, desc = "Eng Desk 1" }
  }
}

resource "cisco_interface" "access_ports" {
  for_each = var.access_ports

  name        = each.value.interface
  description = each.value.desc
  enabled     = true
  mode        = "access"
  access_vlan = each.value.vlan_id
}
```

### Import

Interfaces can be imported using the interface name:

```bash
terraform import cisco_interface.desktop "GigabitEthernet1/0/1"
```

### Notes

- Deleting an interface resource resets it to default configuration
- The interface itself is not removed (cannot be removed from switch)
- Changing the mode requires reconfiguration
- Make sure VLANs exist before assigning them to interfaces

---

## cisco_svi

Manages a Switch Virtual Interface (SVI) for inter-VLAN routing. SVIs provide Layer 3 routing between VLANs.

### Schema

```hcl
resource "cisco_svi" "example" {
  vlan_id     = number # Required, forces new resource
  ip_address  = string # Required
  subnet_mask = string # Required
  description = string # Optional
  enabled     = bool   # Optional, default: true
}
```

### Attributes

| Attribute | Type | Required | Forces New | Default | Description |
|-----------|------|----------|------------|---------|-------------|
| `vlan_id` | number | **Yes** | **Yes** | - | VLAN ID (1-4094). The VLAN must exist |
| `ip_address` | string | **Yes** | No | - | IP address for the SVI (e.g., "192.168.1.1") |
| `subnet_mask` | string | **Yes** | No | - | Subnet mask (e.g., "255.255.255.0") |
| `description` | string | No | No | - | Interface description |
| `enabled` | bool | No | No | `true` | Administrative state (`true` = no shutdown) |

### Attribute Reference

In addition to arguments, the following attributes are exported:

- All input attributes
- Configuration synced from switch

### Validation Rules

- `ip_address` must be a valid IPv4 address
- `subnet_mask` must be a valid subnet mask
- The associated VLAN should exist (not enforced but recommended)

### Cisco Commands

**Create/Update:**
```
configure terminal
interface vlan <vlan_id>
description <description>
ip address <ip_address> <subnet_mask>
no shutdown
end
```

**Read:**
```
show running-config interface vlan <vlan_id>
show ip interface vlan <vlan_id>
```

**Delete:**
```
configure terminal
no interface vlan <vlan_id>
end
```

### Examples

#### Basic SVI

```hcl
resource "cisco_svi" "sales_gateway" {
  vlan_id     = cisco_vlan.sales.vlan_id
  ip_address  = "192.168.100.1"
  subnet_mask = "255.255.255.0"
  description = "Sales VLAN Gateway"
  enabled     = true
}
```

#### Multiple SVIs for Routing

```hcl
# Create VLANs
resource "cisco_vlan" "vlans" {
  for_each = {
    sales = 100
    eng   = 200
    hr    = 300
  }

  vlan_id = each.value
  name    = each.key
}

# Create gateways for each VLAN
resource "cisco_svi" "gateways" {
  for_each = {
    sales = { vlan = 100, ip = "192.168.100.1" }
    eng   = { vlan = 200, ip = "192.168.200.1" }
    hr    = { vlan = 300, ip = "192.168.300.1" }
  }

  vlan_id     = each.value.vlan
  ip_address  = each.value.ip
  subnet_mask = "255.255.255.0"
  description = "${each.key} Gateway"
  enabled     = true

  depends_on = [cisco_vlan.vlans]
}
```

#### SVI with CIDR Notation

```hcl
locals {
  # Convert CIDR to subnet mask
  cidr_to_mask = {
    24 = "255.255.255.0"
    16 = "255.255.0.0"
    8  = "255.0.0.0"
  }
}

resource "cisco_svi" "management" {
  vlan_id     = 1
  ip_address  = "10.0.0.1"
  subnet_mask = local.cidr_to_mask[24]  # /24
  description = "Management Gateway"
}
```

### Import

SVIs can be imported using the VLAN ID:

```bash
terraform import cisco_svi.sales_gateway 100
```

### Notes

- The VLAN must exist before creating the SVI
- Deleting the SVI removes Layer 3 routing for that VLAN
- Multiple SVIs enable inter-VLAN routing
- Requires "ip routing" to be enabled on the switch

---

## cisco_interface_ip

Manages IP address assignment on an interface. Typically used for management interfaces.

### Schema

```hcl
resource "cisco_interface_ip" "example" {
  interface   = string # Required, forces new resource
  ip_address  = string # Optional (mutually exclusive with dhcp)
  subnet_mask = string # Optional (required with ip_address)
  dhcp        = bool   # Optional (mutually exclusive with ip_address)
}
```

### Attributes

| Attribute | Type | Required | Forces New | Default | Description |
|-----------|------|----------|------------|---------|-------------|
| `interface` | string | **Yes** | **Yes** | - | Interface name (e.g., "Vlan1") |
| `ip_address` | string | Conditional | No | - | Static IP address (mutually exclusive with `dhcp`) |
| `subnet_mask` | string | Conditional | No | - | Subnet mask (required when `ip_address` is specified) |
| `dhcp` | bool | Conditional | No | `false` | Use DHCP (mutually exclusive with `ip_address`) |

### Attribute Reference

- All input attributes

### Validation Rules

- Either `ip_address` OR `dhcp` must be specified (but not both)
- If `ip_address` is specified, `subnet_mask` is required
- `ip_address` must be a valid IPv4 address
- `subnet_mask` must be a valid subnet mask

### Cisco Commands

**Static IP:**
```
configure terminal
interface <interface>
ip address <ip_address> <subnet_mask>
no shutdown
end
```

**DHCP:**
```
configure terminal
interface <interface>
ip address dhcp
no shutdown
end
```

**Read:**
```
show running-config interface <interface>
show ip interface <interface>
```

**Delete:**
```
configure terminal
interface <interface>
no ip address
end
```

### Examples

#### Static IP on Management VLAN

```hcl
resource "cisco_interface_ip" "management" {
  interface   = "Vlan1"
  ip_address  = "192.168.1.10"
  subnet_mask = "255.255.255.0"
}
```

#### DHCP on Management Interface

```hcl
resource "cisco_interface_ip" "management_dhcp" {
  interface = "Vlan1"
  dhcp      = true
}
```

#### Static IP with Variables

```hcl
variable "management_config" {
  type = object({
    interface = string
    ip        = string
    mask      = string
  })
  default = {
    interface = "Vlan1"
    ip        = "10.0.1.10"
    mask      = "255.255.255.0"
  }
}

resource "cisco_interface_ip" "mgmt" {
  interface   = var.management_config.interface
  ip_address  = var.management_config.ip
  subnet_mask = var.management_config.mask
}
```

#### Conditional DHCP vs Static

```hcl
variable "use_dhcp" {
  type    = bool
  default = false
}

resource "cisco_interface_ip" "management" {
  interface   = "Vlan1"
  dhcp        = var.use_dhcp
  ip_address  = var.use_dhcp ? null : "192.168.1.10"
  subnet_mask = var.use_dhcp ? null : "255.255.255.0"
}
```

### Import

Interface IPs can be imported using the interface name:

```bash
terraform import cisco_interface_ip.management "Vlan1"
```

### Notes

- Typically used for management interfaces
- Can be used on any L3 interface
- Changing from DHCP to static (or vice versa) updates the configuration
- The interface must exist

---

## Examples

### Complete Network Setup

```hcl
terraform {
  required_providers {
    cisco = {
      source = "registry.terraform.io/example-org/cisco"
    }
  }
}

provider "cisco" {
  host            = var.switch_ip
  username        = var.switch_username
  password        = var.switch_password
  enable_password = var.switch_enable_password
}

# Create VLANs
resource "cisco_vlan" "sales" {
  vlan_id = 100
  name    = "Sales_Department"
}

resource "cisco_vlan" "engineering" {
  vlan_id = 200
  name    = "Engineering"
}

resource "cisco_vlan" "management" {
  vlan_id = 10
  name    = "Management"
}

# Create SVI gateways
resource "cisco_svi" "sales_gateway" {
  vlan_id     = cisco_vlan.sales.vlan_id
  ip_address  = "192.168.100.1"
  subnet_mask = "255.255.255.0"
  description = "Sales Gateway"
}

resource "cisco_svi" "eng_gateway" {
  vlan_id     = cisco_vlan.engineering.vlan_id
  ip_address  = "192.168.200.1"
  subnet_mask = "255.255.255.0"
  description = "Engineering Gateway"
}

# Configure access ports
resource "cisco_interface" "sales_ports" {
  count = 10

  name        = "GigabitEthernet1/0/${count.index + 1}"
  description = "Sales Desktop ${count.index + 1}"
  enabled     = true
  mode        = "access"
  access_vlan = cisco_vlan.sales.vlan_id
}

resource "cisco_interface" "eng_ports" {
  count = 5

  name        = "GigabitEthernet1/0/${count.index + 11}"
  description = "Engineering Desktop ${count.index + 1}"
  enabled     = true
  mode        = "access"
  access_vlan = cisco_vlan.engineering.vlan_id
}

# Configure trunk uplink
resource "cisco_interface" "uplink" {
  name        = "GigabitEthernet1/0/48"
  description = "Trunk to Core Switch"
  enabled     = true
  mode        = "trunk"
  trunk_vlans = [
    cisco_vlan.sales.vlan_id,
    cisco_vlan.engineering.vlan_id,
    cisco_vlan.management.vlan_id,
  ]
  native_vlan = 1
}

# Configure management IP
resource "cisco_interface_ip" "management" {
  interface   = "Vlan${cisco_vlan.management.vlan_id}"
  ip_address  = "192.168.10.10"
  subnet_mask = "255.255.255.0"
}

# Outputs
output "vlan_ids" {
  value = {
    sales       = cisco_vlan.sales.vlan_id
    engineering = cisco_vlan.engineering.vlan_id
    management  = cisco_vlan.management.vlan_id
  }
}

output "gateways" {
  value = {
    sales       = "${cisco_svi.sales_gateway.ip_address}/${cisco_svi.sales_gateway.subnet_mask}"
    engineering = "${cisco_svi.eng_gateway.ip_address}/${cisco_svi.eng_gateway.subnet_mask}"
  }
}
```

---

## Import

All resources support importing existing configuration from the switch.

### Import Syntax

```bash
# VLANs - use VLAN ID
terraform import cisco_vlan.example 100

# Interfaces - use interface name
terraform import cisco_interface.example "GigabitEthernet1/0/1"

# SVIs - use VLAN ID
terraform import cisco_svi.example 100

# Interface IPs - use interface name
terraform import cisco_interface_ip.example "Vlan1"
```

### Import Example Workflow

1. **Create placeholder resource in your .tf file:**

```hcl
resource "cisco_vlan" "existing_vlan" {
  vlan_id = 50
  name    = "placeholder"  # Will be updated from switch
}
```

2. **Import the existing configuration:**

```bash
terraform import cisco_vlan.existing_vlan 50
```

3. **Update your .tf file with actual values:**

```bash
terraform show cisco_vlan.existing_vlan
```

Copy the output values to your .tf file.

4. **Verify:**

```bash
terraform plan
# Should show no changes
```

---

## Data Sources

Currently, this provider does not implement data sources. All resources support import to read existing configuration.

Future versions may include:
- `cisco_vlan` data source
- `cisco_interface` data source

---

## Best Practices

1. **Use Variables for Sensitive Data:**
```hcl
variable "cisco_password" {
  type      = string
  sensitive = true
}
```

2. **Use Depends_on for Ordering:**
```hcl
resource "cisco_svi" "gateway" {
  vlan_id = 100
  # ...
  depends_on = [cisco_vlan.my_vlan]
}
```

3. **Use For_each for Multiple Resources:**
```hcl
resource "cisco_vlan" "departments" {
  for_each = var.departments
  vlan_id  = each.value.id
  name     = each.value.name
}
```

4. **Test in Lab First:**
- Always test configurations on non-production switches
- Use `terraform plan` to review changes
- Have rollback procedures ready

5. **Version Control:**
- Store all .tf files in version control
- Use .tfvars files for environment-specific values
- Never commit credentials

---

## Troubleshooting

See [README.md](README.md) for detailed troubleshooting guide.

Quick tips:
- Check connectivity: `terraform plan`
- Review logs: Enable debug mode
- Validate syntax: `terraform validate`
- Check state: `terraform show`

---

## Additional Resources

- [README.md](README.md) - Full provider documentation
- [SAFETY.md](SAFETY.md) - Safety procedures and testing
- [QUICKSTART.md](QUICKSTART.md) - Getting started guide
- [TESTING.md](TESTING.md) - Testing documentation
- [examples/](examples/) - Example configurations

---

**Last Updated:** February 2026
**Provider Version:** 1.0.0
