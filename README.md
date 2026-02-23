# Terraform Provider for Cisco WS-C3650 Switches

Manages Cisco WS-C3650 switches (and compatible IOS devices) through SSH CLI automation, enabling infrastructure-as-code for devices without a REST API. Every change is automatically saved to startup-config (`write memory`) so configuration survives reboots.

## Requirements

- Terraform >= 1.0
- Go >= 1.21 (to build from source)
- Cisco WS-C3650 or compatible IOS switch with SSH access and enable-mode privileges

## Installation

```bash
git clone https://github.com/example-org/cisco-switch-provider.git
cd cisco-switch-provider
go build -o terraform-provider-cisco

mkdir -p ~/.terraform.d/plugins/registry.terraform.io/example-org/cisco/1.0.0/darwin_arm64/
cp terraform-provider-cisco ~/.terraform.d/plugins/registry.terraform.io/example-org/cisco/1.0.0/darwin_arm64/
chmod +x ~/.terraform.d/plugins/registry.terraform.io/example-org/cisco/1.0.0/darwin_arm64/terraform-provider-cisco
```

Adjust the path for your OS/architecture (e.g. `linux_amd64`, `darwin_amd64`).

---

## Provider Configuration

```hcl
terraform {
  required_providers {
    cisco = {
      source = "registry.terraform.io/example-org/cisco"
    }
  }
}

provider "cisco" {
  host            = "192.168.1.1"
  username        = var.cisco_username
  password        = var.cisco_password
  enable_password = var.cisco_enable_password  # optional
}
```

| Argument | Type | Required | Default | Description |
|---|---|---|---|---|
| `host` | string | yes | — | IP address or hostname of the switch |
| `username` | string | yes | — | SSH username |
| `password` | string | yes | — | SSH password. Only password auth is supported; SSH key auth is not. |
| `enable_password` | string | no | — | Enable mode password, if different from the SSH password |
| `port` | number | no | `22` | SSH port |
| `ssh_timeout` | number | no | `30` | SSH connection timeout in seconds |
| `command_timeout` | number | no | `10` | Per-command timeout in seconds |

---

## Resources

- [cisco_vlan](#cisco_vlan)
- [cisco_interface](#cisco_interface)
- [cisco_svi](#cisco_svi)
- [cisco_interface_ip](#cisco_interface_ip)
- [cisco_dhcp_pool](#cisco_dhcp_pool)
- [cisco_acl_rule](#cisco_acl_rule)
- [cisco_acl_policy](#cisco_acl_policy)
- [cisco_snmp_community](#cisco_snmp_community)
- [cisco_snmp](#cisco_snmp)

---

### cisco_vlan

Creates and manages a VLAN.

```hcl
resource "cisco_vlan" "sales" {
  vlan_id = 100
  name    = "Sales"
}
```

| Argument | Type | Required | Default | Description |
|---|---|---|---|---|
| `vlan_id` | number | **yes** | — | VLAN ID (1–4094). Changing forces a new resource. |
| `name` | string | **yes** | — | VLAN name |
| `state` | string | no | `"active"` | `"active"` or `"suspend"` |

```bash
terraform import cisco_vlan.sales 100
```

---

### cisco_interface

Configures a switchport as an access or trunk port.

```hcl
# Access port
resource "cisco_interface" "desktop" {
  name        = "GigabitEthernet1/0/1"
  description = "Sales Desktop"
  mode        = "access"
  access_vlan = cisco_vlan.sales.vlan_id
}

# Trunk uplink
resource "cisco_interface" "uplink" {
  name        = "GigabitEthernet1/0/48"
  description = "Core Switch Uplink"
  mode        = "trunk"
  trunk_vlans = [100, 200, 300]
  native_vlan = 1
}
```

| Argument | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | **yes** | — | Interface name (e.g. `"GigabitEthernet1/0/1"`). Changing forces a new resource. |
| `mode` | string | **yes** | — | `"access"` or `"trunk"` |
| `access_vlan` | number | no* | — | VLAN ID for the port. Required when `mode = "access"`. |
| `trunk_vlans` | list(number) | no* | — | Allowed VLAN IDs. Required when `mode = "trunk"`. |
| `description` | string | no | — | Interface description |
| `enabled` | bool | no | `true` | `true` = no shutdown, `false` = shutdown |
| `native_vlan` | number | no | `1` | Native VLAN for trunk mode |

```bash
terraform import cisco_interface.desktop "GigabitEthernet1/0/1"
```

---

### cisco_svi

Creates a Switch Virtual Interface (SVI) — the Layer 3 gateway for a VLAN. Supports DHCP relay and ACL filtering.

```hcl
# Basic gateway
resource "cisco_svi" "sales_gw" {
  vlan_id     = cisco_vlan.sales.vlan_id
  ip_address  = "192.168.100.1"
  subnet_mask = "255.255.255.0"
  description = "Sales VLAN Gateway"
}

# Gateway with DHCP relay and inbound ACL
resource "cisco_svi" "eng_gw" {
  vlan_id          = cisco_vlan.engineering.vlan_id
  ip_address       = "192.168.200.1"
  subnet_mask      = "255.255.255.0"
  dhcp_servers     = ["10.0.0.1", "10.0.0.2"]
  access_group_in  = cisco_acl_policy.inter_vlan.name
}
```

| Argument | Type | Required | Default | Description |
|---|---|---|---|---|
| `vlan_id` | number | **yes** | — | VLAN ID (1–4094). Changing forces a new resource. |
| `ip_address` | string | **yes** | — | IP address for the SVI |
| `subnet_mask` | string | **yes** | — | Subnet mask |
| `description` | string | no | — | Interface description |
| `enabled` | bool | no | `true` | `true` = no shutdown, `false` = shutdown |
| `dhcp_servers` | list(string) | no | — | DHCP relay server IPs (`ip helper-address`). Forwards DHCP requests to an external server instead of the switch serving them. |
| `access_group_in` | string | no | — | `cisco_acl_policy` name to apply inbound (`ip access-group <name> in`) |
| `access_group_out` | string | no | — | `cisco_acl_policy` name to apply outbound (`ip access-group <name> out`) |

```bash
terraform import cisco_svi.sales_gw 100
```

---

### cisco_interface_ip

Assigns a static or DHCP-obtained IP address to an interface. Typically used for the management VLAN.

```hcl
# Static management IP
resource "cisco_interface_ip" "mgmt" {
  interface   = "Vlan1"
  ip_address  = "192.168.1.10"
  subnet_mask = "255.255.255.0"
}

# DHCP-assigned management IP
resource "cisco_interface_ip" "mgmt_dhcp" {
  interface = "Vlan1"
  dhcp      = true
}
```

| Argument | Type | Required | Default | Description |
|---|---|---|---|---|
| `interface` | string | **yes** | — | Interface name (e.g. `"Vlan1"`). Changing forces a new resource. |
| `ip_address` | string | no* | — | Static IP address. Mutually exclusive with `dhcp`. |
| `subnet_mask` | string | no* | — | Subnet mask. Required when `ip_address` is set. |
| `dhcp` | bool | no | `false` | Obtain IP via DHCP. Mutually exclusive with `ip_address`. |

```bash
terraform import cisco_interface_ip.mgmt "Vlan1"
```

---

### cisco_dhcp_pool

Configures the switch as a DHCP server for a subnet. Use `dhcp_servers` on `cisco_svi` instead if you want to relay DHCP to an external server.

```hcl
resource "cisco_dhcp_pool" "sales" {
  name           = "SALES"
  network        = "192.168.100.0"
  subnet_mask    = "255.255.255.0"
  default_router = "192.168.100.1"
  dns_servers    = ["8.8.8.8", "8.8.4.4"]
  lease_days     = 1
  domain_name    = "sales.example.com"
}
```

| Argument | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | **yes** | — | Pool name. Changing forces a new resource. |
| `network` | string | **yes** | — | Network address (e.g. `"192.168.100.0"`) |
| `subnet_mask` | string | **yes** | — | Subnet mask |
| `default_router` | string | no | — | Default gateway IP offered to clients |
| `dns_servers` | list(string) | no | — | DNS server IPs offered to clients |
| `lease_days` | number | no | `1` | Lease duration in days |
| `lease_hours` | number | no | `0` | Additional lease hours |
| `lease_minutes` | number | no | `0` | Additional lease minutes |
| `domain_name` | string | no | — | Domain name offered to clients |

```bash
terraform import cisco_dhcp_pool.sales "SALES"
```

---

### cisco_acl_rule

Defines a single ACE (access control entry). **This resource is local-only — it never writes to the switch.** Its `id` is the IOS ACE command string (e.g. `"permit ip 192.168.100.0 0.0.0.255 any"`), which is referenced in order inside `cisco_acl_policy.rules`. Any attribute change forces a new resource and a new `id`, which the policy automatically picks up.

```hcl
resource "cisco_acl_rule" "allow_sales_to_eng" {
  action               = "permit"
  protocol             = "ip"
  source               = "192.168.100.0"
  source_wildcard      = "0.0.0.255"
  destination          = "192.168.200.0"
  destination_wildcard = "0.0.0.255"
}

resource "cisco_acl_rule" "allow_https_out" {
  action      = "permit"
  protocol    = "tcp"
  source      = "192.168.100.0"
  source_wildcard = "0.0.0.255"
  destination = "any"
  dst_port    = "eq 443"
}

resource "cisco_acl_rule" "deny_all" {
  action      = "deny"
  protocol    = "ip"
  source      = "any"
  destination = "any"
  log         = true
}
```

| Argument | Type | Required | Default | Description |
|---|---|---|---|---|
| `action` | string | **yes** | — | `"permit"` or `"deny"`. Changing forces a new resource. |
| `source` | string | **yes** | — | `"any"`, a host IP, or a network IP (pair with `source_wildcard`). Changing forces a new resource. |
| `protocol` | string | no | `"ip"` | `"ip"`, `"tcp"`, `"udp"`, or `"icmp"`. Changing forces a new resource. |
| `source_wildcard` | string | no | — | Wildcard mask for the source network. Omit for `"any"` or a single host. Changing forces a new resource. |
| `destination` | string | no | — | Same format as `source`. Required for extended ACLs. Changing forces a new resource. |
| `destination_wildcard` | string | no | — | Wildcard mask for the destination. Changing forces a new resource. |
| `src_port` | string | no | — | Source port match for TCP/UDP (e.g. `"eq 80"`, `"range 8000 8080"`). Changing forces a new resource. |
| `dst_port` | string | no | — | Destination port match for TCP/UDP (e.g. `"eq 443"`). Changing forces a new resource. |
| `log` | bool | no | `false` | Log matching packets. Changing forces a new resource. |

**Computed attribute:** `id` — the IOS ACE string. Reference this in `cisco_acl_policy.rules`.

---

### cisco_acl_policy

Creates a named ACL on the switch. Rules are applied in the order they appear in the `rules` list — list position determines IOS sequence number (index 0 → seq 10, index 1 → seq 20, etc.). Any change to the list atomically recreates the ACL to guarantee exact order.

```hcl
resource "cisco_acl_policy" "inter_vlan" {
  name = "INTER-VLAN-POLICY"
  type = "extended"

  rules = [
    cisco_acl_rule.allow_sales_to_eng.id,
    cisco_acl_rule.allow_https_out.id,
    cisco_acl_rule.deny_all.id,  # must be last
  ]
}

# Attach the policy to an SVI
resource "cisco_svi" "sales_gw" {
  vlan_id         = cisco_vlan.sales.vlan_id
  ip_address      = "192.168.100.1"
  subnet_mask     = "255.255.255.0"
  access_group_in = cisco_acl_policy.inter_vlan.name
}
```

| Argument | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | **yes** | — | ACL name on the switch. Changing forces a new resource. |
| `type` | string | **yes** | — | `"standard"` (source-only) or `"extended"` (source, destination, ports). Changing forces a new resource. |
| `rules` | list(string) | **yes** | — | Ordered list of `cisco_acl_rule.id` values. Position = sequence order. |

```bash
# Import format: "<type>/<name>"
terraform import cisco_acl_policy.inter_vlan "extended/INTER-VLAN-POLICY"
# rules will be empty after import — repopulate with cisco_acl_rule resources.
```

---

### cisco_snmp_community

Manages a single SNMP community string. Create one resource per community needed. Use `access = "ro"` for Prometheus SNMP Exporter polling.

```hcl
# Read-only community for Prometheus polling
resource "cisco_snmp_community" "readonly" {
  name   = "MONITOR"
  access = "ro"
}

# Read-only community restricted to specific hosts via ACL
resource "cisco_snmp_community" "readonly_restricted" {
  name   = "MONITOR"
  access = "ro"
  acl    = "SNMP-HOSTS"  # standard ACL number or name
}
```

| Argument | Type | Required | Default | Description |
|---|---|---|---|---|
| `name` | string | **yes** | — | Community string value. Changing forces a new resource. |
| `access` | string | **yes** | — | `"ro"` (read-only, for Prometheus) or `"rw"` (read-write) |
| `acl` | string | no | — | Standard ACL name or number restricting which source IPs may use this community |

```bash
terraform import cisco_snmp_community.readonly "MONITOR"
```

---

### cisco_snmp

Manages global SNMP settings: system location, contact, and trap destinations. Only one of this resource should exist per switch.

```hcl
resource "cisco_snmp" "main" {
  location       = "DC1, Rack 3, Unit 12"
  contact        = "NOC <noc@example.com>"
  trap_community = "MONITOR"
  trap_version   = "2c"
  trap_hosts     = ["10.0.0.50"]  # snmptrapd or Alertmanager SNMP receiver
}
```

| Argument | Type | Required | Default | Description |
|---|---|---|---|---|
| `location` | string | no | — | Physical location string (`snmp-server location`) |
| `contact` | string | no | — | Administrator contact (`snmp-server contact`) |
| `trap_community` | string | no | — | Community string to use when sending traps. Required if `trap_hosts` is set. |
| `trap_version` | string | no | `"2c"` | SNMP version for traps: `"1"` or `"2c"` |
| `trap_hosts` | list(string) | no | — | IPs of trap receivers (`snmp-server host`). Setting this also enables `snmp-server enable traps`. |

**Computed attribute:** `id` — always `"snmp"` (singleton resource, no import needed).

---

## Prometheus + Alertmanager Integration

The recommended observability stack for this switch provider:

```
Switch ──SNMP poll──▶ Prometheus SNMP Exporter ──▶ Prometheus ──▶ Alertmanager
Switch ──SNMP trap──▶ snmptrapd                ──▶ Alertmanager
```

**Switch configuration:**

```hcl
resource "cisco_snmp_community" "prom" {
  name   = "MONITOR"
  access = "ro"
}

resource "cisco_snmp" "main" {
  location       = "DC1 Rack 3"
  contact        = "ops@example.com"
  trap_community = "MONITOR"
  trap_hosts     = ["10.0.0.50"]
}
```

**Prometheus SNMP Exporter — `snmp.yml`:**

```yaml
modules:
  cisco_switch:
    walk:
      - sysUpTime
      - ifTable
      - ifXTable
      - ciscoMemoryPoolTable
      - cpmCPUTotalTable
    auth:
      community: MONITOR
      version: 2
```

**Prometheus — `prometheus.yml` scrape config:**

```yaml
scrape_configs:
  - job_name: cisco_switch
    static_configs:
      - targets: ["192.168.1.1"]  # switch IP
    metrics_path: /snmp
    params:
      module: [cisco_switch]
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: localhost:9116  # SNMP Exporter
```

**Alert rules:**

```yaml
groups:
  - name: cisco_switch
    rules:
      - alert: InterfaceDown
        expr: ifOperStatus{job="cisco_switch"} == 2
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Interface {{ $labels.ifDescr }} is down on {{ $labels.instance }}"

      - alert: HighCPU
        expr: cpmCPUTotal5min{job="cisco_switch"} > 80
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Switch CPU > 80% on {{ $labels.instance }}"

      - alert: HighMemoryUsage
        expr: (ciscoMemoryPoolUsed / (ciscoMemoryPoolUsed + ciscoMemoryPoolFree)) * 100 > 85
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Switch memory > 85% on {{ $labels.instance }}"
```

---

## Complete Example

```hcl
# ── VLANs ────────────────────────────────────────────────────────────────────

resource "cisco_vlan" "sales" {
  vlan_id = 100
  name    = "Sales"
}

resource "cisco_vlan" "engineering" {
  vlan_id = 200
  name    = "Engineering"
}

# ── Switchports ───────────────────────────────────────────────────────────────

resource "cisco_interface" "sales_port" {
  name        = "GigabitEthernet1/0/1"
  description = "Sales Desktop"
  mode        = "access"
  access_vlan = cisco_vlan.sales.vlan_id
}

resource "cisco_interface" "uplink" {
  name        = "GigabitEthernet1/0/48"
  description = "Core Switch Uplink"
  mode        = "trunk"
  trunk_vlans = [cisco_vlan.sales.vlan_id, cisco_vlan.engineering.vlan_id]
}

# ── ACL ───────────────────────────────────────────────────────────────────────

resource "cisco_acl_rule" "sales_to_eng" {
  action               = "permit"
  protocol             = "ip"
  source               = "192.168.100.0"
  source_wildcard      = "0.0.0.255"
  destination          = "192.168.200.0"
  destination_wildcard = "0.0.0.255"
}

resource "cisco_acl_rule" "deny_all" {
  action      = "deny"
  protocol    = "ip"
  source      = "any"
  destination = "any"
  log         = true
}

resource "cisco_acl_policy" "inter_vlan" {
  name  = "INTER-VLAN"
  type  = "extended"
  rules = [
    cisco_acl_rule.sales_to_eng.id,
    cisco_acl_rule.deny_all.id,
  ]
}

# ── SVIs (Layer 3 gateways) ───────────────────────────────────────────────────

resource "cisco_svi" "sales_gw" {
  vlan_id          = cisco_vlan.sales.vlan_id
  ip_address       = "192.168.100.1"
  subnet_mask      = "255.255.255.0"
  description      = "Sales Gateway"
  dhcp_servers     = ["10.0.0.1"]
  access_group_in  = cisco_acl_policy.inter_vlan.name
}

resource "cisco_svi" "eng_gw" {
  vlan_id     = cisco_vlan.engineering.vlan_id
  ip_address  = "192.168.200.1"
  subnet_mask = "255.255.255.0"
  description = "Engineering Gateway"
}

# ── Management ────────────────────────────────────────────────────────────────

resource "cisco_interface_ip" "mgmt" {
  interface   = "Vlan1"
  ip_address  = "192.168.1.10"
  subnet_mask = "255.255.255.0"
}

# ── DHCP (switch acts as server) ─────────────────────────────────────────────

resource "cisco_dhcp_pool" "sales" {
  name           = "SALES"
  network        = "192.168.100.0"
  subnet_mask    = "255.255.255.0"
  default_router = "192.168.100.1"
  dns_servers    = ["8.8.8.8", "8.8.4.4"]
  lease_days     = 1
}

# ── SNMP / Observability ──────────────────────────────────────────────────────

resource "cisco_snmp_community" "readonly" {
  name   = "MONITOR"
  access = "ro"
}

resource "cisco_snmp" "main" {
  location       = "DC1, Rack 3"
  contact        = "ops@example.com"
  trap_community = "MONITOR"
  trap_hosts     = ["10.0.0.50"]
}
```

---

## Notes

- **Authentication:** Password-based SSH only. SSH key auth is not supported.
- **Persistence:** Every apply runs `write memory` automatically — changes always survive a reboot.
- **Operations:** Single SSH session; all operations are sequential.
- **IOS version:** Tested on IOS 15.x.
- **Security:** Use `sensitive = true` for password variables. The provider uses `ssh.InsecureIgnoreHostKey()` — configure proper host key verification in production environments.

## License

MIT License
