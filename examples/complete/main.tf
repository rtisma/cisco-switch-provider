# Example Terraform configuration for Cisco Switch Provider

terraform {
  required_providers {
    cisco = {
      source = "registry.terraform.io/example-org/cisco"
    }
  }
}

# Configure the Cisco Provider
provider "cisco" {
  host            = "192.168.1.1"
  username        = var.cisco_username
  password        = var.cisco_password
  enable_password = var.cisco_enable_password
  port            = 22
  ssh_timeout     = 30
  command_timeout = 10
}

# Variables for sensitive data
variable "cisco_username" {
  description = "Cisco switch SSH username"
  type        = string
  sensitive   = true
}

variable "cisco_password" {
  description = "Cisco switch SSH password"
  type        = string
  sensitive   = true
}

variable "cisco_enable_password" {
  description = "Cisco switch enable password"
  type        = string
  sensitive   = true
}

# Create VLANs
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

# Configure inter-VLAN routing (SVIs)
resource "cisco_svi" "sales_gateway" {
  vlan_id     = cisco_vlan.sales.vlan_id
  ip_address  = "192.168.100.1"
  subnet_mask = "255.255.255.0"
  description = "Sales VLAN Gateway"
  enabled     = true
}

resource "cisco_svi" "engineering_gateway" {
  vlan_id     = cisco_vlan.engineering.vlan_id
  ip_address  = "192.168.200.1"
  subnet_mask = "255.255.255.0"
  description = "Engineering VLAN Gateway"
  enabled     = true
}

# Configure access ports
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

# Configure trunk port for uplink to core switch
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

# Configure management interface IP
resource "cisco_interface_ip" "management" {
  interface   = "Vlan${cisco_vlan.management.vlan_id}"
  ip_address  = "192.168.10.10"
  subnet_mask = "255.255.255.0"
}

# Output the created VLAN IDs
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
