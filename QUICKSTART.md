# Quick Start Guide

This guide will help you get started with the Cisco Switch Terraform Provider in just a few minutes.

## Prerequisites

1. Go 1.21 or later installed
2. Terraform 1.0 or later installed
3. Access to a Cisco WS-C3650 switch (or compatible) with:
   - SSH enabled
   - Valid credentials
   - Enable mode access

## Step 1: Build and Install the Provider

```bash
# Clone the repository (or navigate to your existing directory)
cd cisco-switch-provider

# Build and install the provider
make install
```

This will build the provider and install it to your local Terraform plugins directory.

## Step 2: Create Your First Configuration

Create a new directory for your Terraform configuration:

```bash
mkdir ~/my-cisco-config
cd ~/my-cisco-config
```

Create a file named `main.tf`:

```hcl
terraform {
  required_providers {
    cisco = {
      source = "registry.terraform.io/example-org/cisco"
    }
  }
}

provider "cisco" {
  host            = "192.168.1.1"  # Change to your switch IP
  username        = "admin"
  password        = "your-password"        # SSH password (required; key-based auth not supported)
  enable_password = "your-enable-password" # Optional: only needed if enable requires a separate password
}

# Create a simple VLAN
resource "cisco_vlan" "test" {
  vlan_id = 100
  name    = "Test_VLAN"
  state   = "active"
}

# Configure an access port
resource "cisco_interface" "test_port" {
  name        = "GigabitEthernet1/0/10"
  description = "Test Port"
  enabled     = true
  mode        = "access"
  access_vlan = cisco_vlan.test.vlan_id
}
```

## Step 3: Initialize Terraform

```bash
terraform init
```

You should see output indicating that the Cisco provider was successfully initialized.

## Step 4: Plan and Apply

Review what Terraform will do:

```bash
terraform plan
```

If everything looks good, apply the configuration:

```bash
terraform apply
```

Type `yes` when prompted to confirm.

## Step 5: Verify on the Switch

Connect to your switch and verify the configuration:

```
switch# show vlan id 100
switch# show running-config interface GigabitEthernet1/0/10
```

## Step 6: Make Changes

Edit your `main.tf` to change the VLAN name:

```hcl
resource "cisco_vlan" "test" {
  vlan_id = 100
  name    = "Modified_Test_VLAN"  # Changed
  state   = "active"
}
```

Apply the change:

```bash
terraform apply
```

Terraform will detect the drift and update the VLAN name.

## Step 7: Import Existing Resources

If you have existing VLANs or interfaces on your switch, you can import them:

```bash
# Import an existing VLAN
terraform import cisco_vlan.existing 50

# Import an existing interface
terraform import cisco_interface.existing_port "GigabitEthernet1/0/5"
```

After importing, add the corresponding resource blocks to your `main.tf`.

## Step 8: Clean Up

When you're done testing, destroy the resources:

```bash
terraform destroy
```

Type `yes` when prompted.

## Using Variables for Sensitive Data

For production use, store credentials in variables instead of hardcoding them:

Create `variables.tf`:

```hcl
variable "switch_ip" {
  description = "Switch IP address"
  type        = string
}

variable "switch_username" {
  description = "Switch username"
  type        = string
  sensitive   = true
}

variable "switch_password" {
  description = "Switch password"
  type        = string
  sensitive   = true
}

variable "enable_password" {
  description = "Enable mode password"
  type        = string
  sensitive   = true
}
```

Update `main.tf`:

```hcl
provider "cisco" {
  host            = var.switch_ip
  username        = var.switch_username
  password        = var.switch_password
  enable_password = var.enable_password
}
```

Create `terraform.tfvars`:

```hcl
switch_ip       = "192.168.1.1"
switch_username = "admin"
switch_password = "your-password"
enable_password = "your-enable-password"
```

**Important:** Add `terraform.tfvars` to your `.gitignore` to avoid committing credentials!

## Next Steps

- Explore the [complete example](examples/complete/main.tf) for more advanced configurations
- Read the [README.md](README.md) for full documentation
- Try creating SVIs for inter-VLAN routing
- Configure trunk ports for uplinks

## Troubleshooting

### "Unable to Connect to Cisco Switch"

- Verify the switch IP is correct and reachable: `ping 192.168.1.1`
- Check that SSH is enabled on the switch
- Verify credentials are correct

### "Enable password required"

- Make sure to provide the `enable_password` in your provider configuration
- If your switch doesn't require an enable password, you can omit this field

### "Command timeout"

- Increase the timeout in the provider configuration:
  ```hcl
  provider "cisco" {
    command_timeout = 30
  }
  ```

### Provider Not Found

- Make sure you ran `make install`
- Check the installation path matches your OS and architecture
- Run `terraform init` again

## Getting Help

- Check the [README.md](README.md) for detailed documentation
- Review the [examples](examples/) directory
- Open an issue on GitHub for bugs or feature requests

Happy automating! 🚀
