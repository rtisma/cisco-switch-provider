# Resource Documentation

Complete API reference for all resources and data sources in the Cisco Switch Provider.

## Table of Contents

1. [Provider Configuration](#provider-configuration)
2. [cisco_vlan](#cisco_vlan) - VLAN Management
3. [cisco_interface](#cisco_interface) - Switchport Configuration
4. [cisco_svi](#cisco_svi) - Switch Virtual Interface (Inter-VLAN Routing)
5. [cisco_interface_ip](#cisco_interface_ip) - IP Address Assignment
6. [cisco_dhcp_pool](#cisco_dhcp_pool) - DHCP Server Pool
7. [cisco_acl](#cisco_acl) - Named IP Access List
8. [cisco_acl_rule](#cisco_acl_rule) - ACL Rule (ordered ACE)
9. [Examples](#examples)
10. [Import](#import)

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
| `password` | string | **Yes** | - | SSH password for authentication (marked as sensitive). This is the only supported auth method; SSH key-based auth is not currently supported. |
| `enable_password` | string | No | - | Enable mode password if different from SSH password |
| `ssh_timeout` | number | No | `30` | SSH connection timeout in seconds |
| `command_timeout` | number | No | `10` | Command execution timeout in seconds |

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
  vlan_id      = number       # Required, forces new resource
  ip_address   = string       # Required
  subnet_mask  = string       # Required
  description  = string       # Optional
  enabled      = bool         # Optional, default: true
  dhcp_servers = list(string) # Optional - DHCP relay servers
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
| `dhcp_servers` | list(string) | No | No | - | DHCP relay server IP addresses (configures `ip helper-address`) |
| `access_group_in` | string | No | No | - | ACL name to apply inbound (`ip access-group <name> in`) |
| `access_group_out` | string | No | No | - | ACL name to apply outbound (`ip access-group <name> out`) |

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
ip helper-address <dhcp_server>   (repeated for each dhcp_servers entry)
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

#### SVI with DHCP Relay

```hcl
resource "cisco_svi" "sales_gateway" {
  vlan_id      = cisco_vlan.sales.vlan_id
  ip_address   = "192.168.100.1"
  subnet_mask  = "255.255.255.0"
  description  = "Sales VLAN Gateway"
  enabled      = true
  dhcp_servers = ["10.0.0.10", "10.0.0.11"]  # ip helper-address entries
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

## cisco_dhcp_pool

Configures the switch to act as a DHCP server for a subnet using `ip dhcp pool`. Each pool manages IP allocation for one network.

> **DHCP server vs. DHCP relay**: Use `cisco_dhcp_pool` when the switch itself serves DHCP. To forward DHCP requests to an external server, use `dhcp_servers` on `cisco_svi` instead.

### Schema

```hcl
resource "cisco_dhcp_pool" "example" {
  name           = string       # Required, forces new resource
  network        = string       # Required
  subnet_mask    = string       # Required
  default_router = string       # Optional
  dns_servers    = list(string) # Optional
  lease_days     = number       # Optional, default: 1
  lease_hours    = number       # Optional, default: 0
  lease_minutes  = number       # Optional, default: 0
  domain_name    = string       # Optional
}
```

### Attributes

| Attribute | Type | Required | Forces New | Default | Description |
|-----------|------|----------|------------|---------|-------------|
| `name` | string | **Yes** | **Yes** | - | Pool name (used in `ip dhcp pool <name>`) |
| `network` | string | **Yes** | No | - | Network address for the pool (e.g., `"192.168.100.0"`) |
| `subnet_mask` | string | **Yes** | No | - | Subnet mask (e.g., `"255.255.255.0"`) |
| `default_router` | string | No | No | - | Default gateway IP offered to clients |
| `dns_servers` | list(string) | No | No | - | DNS server IPs offered to clients (up to 8) |
| `lease_days` | number | No | No | `1` | Lease duration in days |
| `lease_hours` | number | No | No | `0` | Additional lease hours |
| `lease_minutes` | number | No | No | `0` | Additional lease minutes |
| `domain_name` | string | No | No | - | Domain name offered to clients |

### Cisco Commands

**Create/Update:**
```
configure terminal
ip dhcp pool <name>
 network <network> <subnet_mask>
 default-router <default_router>
 dns-server <dns_servers...>
 lease <lease_days> <lease_hours> <lease_minutes>
 domain-name <domain_name>
end
```

**Read:**
```
show running-config | section ip dhcp pool <name>
```

**Delete:**
```
configure terminal
no ip dhcp pool <name>
end
```

### Examples

#### Basic Pool

```hcl
resource "cisco_dhcp_pool" "sales" {
  name        = "SALES"
  network     = "192.168.100.0"
  subnet_mask = "255.255.255.0"
}
```

#### Full Configuration

```hcl
resource "cisco_dhcp_pool" "sales" {
  name           = "SALES"
  network        = "192.168.100.0"
  subnet_mask    = "255.255.255.0"
  default_router = "192.168.100.1"
  dns_servers    = ["8.8.8.8", "8.8.4.4"]
  lease_days     = 1
  lease_hours    = 12
  domain_name    = "sales.example.com"
}
```

#### Multiple Pools (one per VLAN)

```hcl
resource "cisco_vlan" "sales" {
  vlan_id = 100
  name    = "Sales"
}

resource "cisco_svi" "sales_gw" {
  vlan_id     = cisco_vlan.sales.vlan_id
  ip_address  = "192.168.100.1"
  subnet_mask = "255.255.255.0"
}

resource "cisco_dhcp_pool" "sales" {
  name           = "SALES"
  network        = "192.168.100.0"
  subnet_mask    = "255.255.255.0"
  default_router = cisco_svi.sales_gw.ip_address
  dns_servers    = ["8.8.8.8"]
  lease_days     = 1

  depends_on = [cisco_svi.sales_gw]
}
```

### Import

DHCP pools can be imported using the pool name:

```bash
terraform import cisco_dhcp_pool.sales "SALES"
```

### Notes

- Pool names are case-sensitive on Cisco IOS
- Ensure `ip routing` is enabled if using SVIs for inter-VLAN routing alongside DHCP
- Use `ip dhcp excluded-address` (configured manually or via a future resource) to reserve addresses for static assignments such as the gateway itself
- Deleting the pool stops DHCP service for that subnet immediately

---

## cisco_acl

Creates a named IP access list on the switch. The ACL itself is just a named container — add rules with `cisco_acl_rule` and apply it to an SVI with `access_group_in` or `access_group_out`.

### Schema

```hcl
resource "cisco_acl" "example" {
  name = string  # Required, forces new resource
  type = string  # Required, forces new resource: "standard" or "extended"
}
```

### Attributes

| Attribute | Type | Required | Forces New | Description |
|-----------|------|----------|------------|-------------|
| `name` | string | **Yes** | **Yes** | ACL name |
| `type` | string | **Yes** | **Yes** | `"standard"` (source-only) or `"extended"` (src, dst, and ports) |

### Cisco Commands

**Create:**
```
configure terminal
ip access-list <type> <name>
end
```

**Delete:**
```
configure terminal
no ip access-list <type> <name>
end
```

### Import

Import ID format: `<type>/<name>`

```bash
terraform import cisco_acl.example "extended/INTER-VLAN-POLICY"
```

---

## cisco_acl_rule

Adds a single ACE (access control entry) to a named ACL. The `sequence` number is the ordering mechanism — lower sequence numbers are evaluated first. Changing `acl_name` or `sequence` forces a new resource.

### Schema

```hcl
resource "cisco_acl_rule" "example" {
  acl_name             = string  # Required, forces new resource
  sequence             = number  # Required, forces new resource
  action               = string  # Required: "permit" or "deny"
  protocol             = string  # Optional, default: "ip"
  source               = string  # Required
  source_wildcard      = string  # Optional
  destination          = string  # Optional (required for extended ACLs)
  destination_wildcard = string  # Optional
  src_port             = string  # Optional (TCP/UDP only)
  dst_port             = string  # Optional (TCP/UDP only)
  log                  = bool    # Optional, default: false
}
```

### Attributes

| Attribute | Type | Required | Forces New | Default | Description |
|-----------|------|----------|------------|---------|-------------|
| `acl_name` | string | **Yes** | **Yes** | - | ACL this rule belongs to |
| `sequence` | number | **Yes** | **Yes** | - | Rule evaluation order (lower = first) |
| `action` | string | **Yes** | No | - | `"permit"` or `"deny"` |
| `protocol` | string | No | No | `"ip"` | `"ip"`, `"tcp"`, `"udp"`, `"icmp"` |
| `source` | string | **Yes** | No | - | `"any"`, host IP, or network IP (use with `source_wildcard`) |
| `source_wildcard` | string | No | No | - | Wildcard mask for network source (e.g. `"0.0.0.255"`) |
| `destination` | string | No | No | - | `"any"`, host IP, or network IP |
| `destination_wildcard` | string | No | No | - | Wildcard mask for network destination |
| `src_port` | string | No | No | - | Source port expression, TCP/UDP only (e.g. `"eq 1024"`, `"range 8000 8080"`) |
| `dst_port` | string | No | No | - | Destination port expression, TCP/UDP only (e.g. `"eq 443"`, `"eq www"`) |
| `log` | bool | No | No | `false` | Log packets that match this rule |

### Address Format

| You want to match | `source` / `destination` | `source_wildcard` / `destination_wildcard` |
|---|---|---|
| All traffic | `"any"` | (omit) |
| Single host | `"10.0.0.5"` | (omit) |
| /24 subnet | `"192.168.100.0"` | `"0.0.0.255"` |
| /16 subnet | `"10.10.0.0"` | `"0.0.255.255"` |

### Port Expression Format (TCP/UDP)

| You want to match | `dst_port` / `src_port` |
|---|---|
| Exactly port 443 | `"eq 443"` |
| Well-known name | `"eq www"` or `"eq https"` |
| Ports 8000–8080 | `"range 8000 8080"` |
| Ports below 1024 | `"lt 1024"` |
| Not port 23 | `"neq 23"` |

### Cisco Commands

**Create/Update:**
```
configure terminal
ip access-list <extended|standard> <acl_name>
 <sequence> <action> <protocol> <src-spec> [src-port] <dst-spec> [dst-port] [log]
end
```

**Delete rule:**
```
configure terminal
ip access-list <extended|standard> <acl_name>
 no <sequence>
end
```

### Examples

#### Block one VLAN from reaching another

```hcl
resource "cisco_acl" "vlan_isolation" {
  name = "VLAN-ISOLATION"
  type = "extended"
}

# Block VLAN 100 (Sales) from VLAN 300 (Finance)
resource "cisco_acl_rule" "block_sales_to_finance" {
  acl_name             = cisco_acl.vlan_isolation.name
  sequence             = 10
  action               = "deny"
  protocol             = "ip"
  source               = "192.168.100.0"
  source_wildcard      = "0.0.0.255"
  destination          = "192.168.300.0"
  destination_wildcard = "0.0.0.255"
  log                  = true
}

# Allow all other traffic
resource "cisco_acl_rule" "permit_rest" {
  acl_name    = cisco_acl.vlan_isolation.name
  sequence    = 999
  action      = "permit"
  protocol    = "ip"
  source      = "any"
  destination = "any"
}

# Apply inbound on the Sales SVI
resource "cisco_svi" "sales_gw" {
  vlan_id         = cisco_vlan.sales.vlan_id
  ip_address      = "192.168.100.1"
  subnet_mask     = "255.255.255.0"
  access_group_in = cisco_acl.vlan_isolation.name

  depends_on = [cisco_acl_rule.block_sales_to_finance, cisco_acl_rule.permit_rest]
}
```

#### Allow only specific ports between VLANs

```hcl
resource "cisco_acl" "web_only" {
  name = "WEB-ONLY"
  type = "extended"
}

# Allow HTTP from Users VLAN to Server VLAN
resource "cisco_acl_rule" "allow_http" {
  acl_name             = cisco_acl.web_only.name
  sequence             = 10
  action               = "permit"
  protocol             = "tcp"
  source               = "192.168.50.0"
  source_wildcard      = "0.0.0.255"
  destination          = "192.168.10.0"
  destination_wildcard = "0.0.0.255"
  dst_port             = "eq 80"
}

# Allow HTTPS
resource "cisco_acl_rule" "allow_https" {
  acl_name             = cisco_acl.web_only.name
  sequence             = 20
  action               = "permit"
  protocol             = "tcp"
  source               = "192.168.50.0"
  source_wildcard      = "0.0.0.255"
  destination          = "192.168.10.0"
  destination_wildcard = "0.0.0.255"
  dst_port             = "eq 443"
}

# Deny everything else
resource "cisco_acl_rule" "deny_all" {
  acl_name    = cisco_acl.web_only.name
  sequence    = 999
  action      = "deny"
  protocol    = "ip"
  source      = "any"
  destination = "any"
  log         = true
}
```

### Import

Import ID format: `<acl_name>/<sequence>`

```bash
terraform import cisco_acl_rule.example "INTER-VLAN-POLICY/10"
```

### Notes

- Rules are evaluated in ascending sequence order; the first match wins
- Cisco IOS has an implicit `deny any any` at the end of every ACL — add an explicit deny rule with `log = true` to make dropped traffic visible
- Sequence numbers can have gaps (10, 20, 30) to allow future insertion without recreating resources
- An applied ACL with no rules drops all traffic (only the implicit deny remains)
- For inter-VLAN filtering, apply the ACL `in` on the source VLAN SVI

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

# DHCP pools - use pool name
terraform import cisco_dhcp_pool.example "SALES"

# ACLs - use "<type>/<name>"
terraform import cisco_acl.example "extended/INTER-VLAN-POLICY"

# ACL rules - use "<acl_name>/<sequence>"
terraform import cisco_acl_rule.example "INTER-VLAN-POLICY/10"
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
