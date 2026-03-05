# Complete example for the Cisco Switch Terraform Provider

terraform {
  required_providers {
    cisco = {
      source = "registry.terraform.io/example-org/cisco"
    }
  }
}

provider "cisco" {
  host             = "192.168.1.1"
  username         = var.cisco_username
  private_key_path = var.cisco_private_key_path
  port             = 22
  ssh_timeout      = 30
  command_timeout  = 10
}

variable "cisco_username" {
  description = "SSH username for the Cisco switch"
  type        = string
}

variable "cisco_private_key_path" {
  description = "Path to the SSH private key file used to authenticate with the switch"
  type        = string
  default     = "~/.ssh/id_rsa"
}

# ── VLANs ────────────────────────────────────────────────────────────────────

resource "cisco_vlan" "sales" {
  vlan_id = 100
  name    = "Sales_Department"
  state   = "active"
}

resource "cisco_vlan" "engineering" {
  vlan_id = 200
  name    = "Engineering"
  state   = "active"
}

resource "cisco_vlan" "management" {
  vlan_id = 10
  name    = "Management"
  state   = "active"
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
# cisco_svi automatically issues "ip routing" before configuring the interface,
# so no depends_on or cisco_ip_routing resource is required.

resource "cisco_svi" "sales_gateway" {
  vlan_id         = cisco_vlan.sales.vlan_id
  ip_address      = "192.168.100.1"
  subnet_mask     = "255.255.255.0"
  description     = "Sales VLAN Gateway"
  enabled         = true
  access_group_in = cisco_acl_policy.inter_vlan.name
}

resource "cisco_svi" "engineering_gateway" {
  vlan_id     = cisco_vlan.engineering.vlan_id
  ip_address  = "192.168.200.1"
  subnet_mask = "255.255.255.0"
  description = "Engineering VLAN Gateway"
  enabled     = true
}

# ── DHCP (switch acts as server) ─────────────────────────────────────────────

resource "cisco_dhcp_excluded_range" "sales_reserved" {
  low_address  = "192.168.100.1"
  high_address = "192.168.100.10"
}

resource "cisco_dhcp_pool" "sales" {
  name           = "SALES"
  network        = "192.168.100.0"
  subnet_mask    = "255.255.255.0"
  default_router = "192.168.100.1"
  dns_servers    = ["8.8.8.8", "8.8.4.4"]
  lease_days     = 1
}

# ── Switchports ───────────────────────────────────────────────────────────────

resource "cisco_interface" "sales_port_1" {
  name        = "GigabitEthernet1/0/1"
  description = "Sales Desktop 1"
  enabled     = true
  mode        = "access"
  access_vlan = cisco_vlan.sales.vlan_id
}

resource "cisco_interface" "sales_port_2" {
  name        = "GigabitEthernet1/0/2"
  description = "Sales Desktop 2"
  enabled     = true
  mode        = "access"
  access_vlan = cisco_vlan.sales.vlan_id
}

resource "cisco_interface" "engineering_port_1" {
  name        = "GigabitEthernet1/0/3"
  description = "Engineering Desktop 1"
  enabled     = true
  mode        = "access"
  access_vlan = cisco_vlan.engineering.vlan_id
}

resource "cisco_interface" "trunk_uplink" {
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

# ── Management ────────────────────────────────────────────────────────────────

resource "cisco_interface_ip" "management" {
  interface   = "Vlan${cisco_vlan.management.vlan_id}"
  ip_address  = "192.168.10.10"
  subnet_mask = "255.255.255.0"
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

# ── Outputs ───────────────────────────────────────────────────────────────────

output "sales_vlan_id" {
  value       = cisco_vlan.sales.vlan_id
  description = "Sales VLAN ID"
}

output "engineering_vlan_id" {
  value       = cisco_vlan.engineering.vlan_id
  description = "Engineering VLAN ID"
}

output "sales_gateway" {
  value       = "${cisco_svi.sales_gateway.ip_address}/${cisco_svi.sales_gateway.subnet_mask}"
  description = "Sales VLAN gateway address"
}

output "engineering_gateway" {
  value       = "${cisco_svi.engineering_gateway.ip_address}/${cisco_svi.engineering_gateway.subnet_mask}"
  description = "Engineering VLAN gateway address"
}
