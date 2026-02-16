# Safety Guide for Cisco Switch Provider

## ⚠️ IMPORTANT: Network Safety First

This Terraform provider makes **REAL CHANGES** to your network infrastructure. Before using it on production hardware, please read and follow this safety guide carefully.

## Testing Strategy: Test Before You Deploy

### Phase 1: Unit Tests (SAFE - No Hardware Required)

Run all unit tests with the mock SSH server:

```bash
./test.sh
```

This will:
- Test all parser functions
- Test command building logic
- Test error detection
- Test CRUD operations with a mock switch
- **Zero risk** - no real hardware touched

✅ **All tests must pass before proceeding to Phase 2**

### Phase 2: Lab Testing (LOW RISK - Test Hardware)

Use a **non-production** switch in a lab environment:

1. **Set up isolated test switch**:
   - Use a switch that is NOT connected to production
   - Disconnect all critical uplinks
   - Document the original configuration
   - Take a backup: `copy running-config backup-config`

2. **Start with read-only operations**:
   ```hcl
   # Test connection first
   provider "cisco" {
     host     = "192.168.1.100"  # Test switch only!
     username = "admin"
     password = var.password
   }

   # No resources yet - just test connectivity
   ```

3. **Run terraform plan (read-only)**:
   ```bash
   terraform init
   terraform plan  # This only reads, doesn't change anything
   ```

4. **Test VLAN creation on unused VLAN ID**:
   ```hcl
   resource "cisco_vlan" "test" {
     vlan_id = 999  # Use unused VLAN ID
     name    = "SafetyTest"
   }
   ```

5. **Verify manually after each apply**:
   ```bash
   terraform apply
   # Then SSH to switch and verify:
   # show vlan id 999
   # show running-config
   ```

6. **Test terraform destroy**:
   ```bash
   terraform destroy
   # Verify VLAN was removed
   ```

### Phase 3: Staging Testing (MEDIUM RISK - Staging Environment)

Test in your staging/pre-production environment:

1. **Use a staging switch that mirrors production**
2. **Test during maintenance window**
3. **Have rollback plan ready**
4. **Test all resource types**:
   - VLANs
   - Interface configurations
   - SVIs (inter-VLAN routing)
   - IP addressing

### Phase 4: Production Deployment (HIGH RISK - Production Environment)

**Only proceed if:**
- ✅ All unit tests pass
- ✅ Lab testing completed successfully
- ✅ Staging testing completed successfully
- ✅ You have a tested rollback procedure
- ✅ You have a maintenance window scheduled
- ✅ You have backups of current configuration

## Safety Checklist Before Production Use

### Pre-Deployment Checklist

- [ ] All unit tests pass (`./test.sh`)
- [ ] Tested on non-production hardware
- [ ] Configuration backup taken (`copy running-config tftp://...`)
- [ ] Rollback procedure documented and tested
- [ ] Change window scheduled
- [ ] Team notified of changes
- [ ] Monitoring in place to detect issues
- [ ] Emergency contact list ready

### Configuration Safety

- [ ] Use Terraform workspaces to separate environments
- [ ] Never hardcode credentials (use variables)
- [ ] Store state files securely
- [ ] Use version control for all .tf files
- [ ] Use `terraform plan` before every `apply`
- [ ] Review plan output carefully before applying

### Network Safety

- [ ] Start with non-critical VLANs
- [ ] Avoid managing VLANs currently in use
- [ ] Test during maintenance windows
- [ ] Have console access to switch (not just SSH)
- [ ] Don't manage the management VLAN initially
- [ ] Keep manual access available (console cable)

## Safe Usage Patterns

### Pattern 1: Incremental Changes

```hcl
# Start small - one VLAN
resource "cisco_vlan" "test" {
  vlan_id = 100
  name    = "Test"
}
```

Apply and verify before adding more resources.

### Pattern 2: Read-Only Verification

Use `terraform plan` extensively:

```bash
# This is safe - only reads current state
terraform plan

# Review the plan carefully before applying
terraform apply
```

### Pattern 3: Import Existing Configuration

Import existing resources before managing them:

```bash
# Import existing VLAN 50
terraform import cisco_vlan.existing 50

# Import existing interface
terraform import cisco_interface.existing "GigabitEthernet1/0/1"
```

This prevents Terraform from trying to recreate resources.

### Pattern 4: Use Terraform's -target Flag

Apply changes to specific resources only:

```bash
# Only affect this one VLAN
terraform apply -target=cisco_vlan.sales

# Avoid blast radius of changes
```

## What Could Go Wrong (and How to Avoid It)

### Risk: Misconfigured VLAN

**Problem**: Wrong VLAN ID assigned to interface
**Impact**: Devices lose network connectivity
**Prevention**:
- Test on unused interfaces first
- Use `terraform plan` to review changes
- Have console access for recovery

### Risk: Accidental Deletion

**Problem**: `terraform destroy` removes critical VLANs
**Impact**: Network outage
**Prevention**:
- Use lifecycle `prevent_destroy` for critical resources
- Review destroy plan carefully
- Keep backups

Example protection:
```hcl
resource "cisco_vlan" "critical" {
  vlan_id = 1
  name    = "default"

  lifecycle {
    prevent_destroy = true  # Terraform will refuse to destroy this
  }
}
```

### Risk: Configuration Drift

**Problem**: Manual changes conflict with Terraform
**Impact**: Terraform may revert manual emergency fixes
**Prevention**:
- Use `terraform refresh` to sync state
- Document all manual changes
- Import manual changes into Terraform

### Risk: Provider Bugs

**Problem**: Bug in provider causes incorrect configuration
**Impact**: Unpredictable switch behavior
**Prevention**:
- Run full test suite before use
- Test in lab environment first
- Report bugs before production use
- Keep backups

## Emergency Rollback Procedures

### If Terraform Apply Fails

1. **Don't panic** - switch is likely still functional
2. Check switch status: `show running-config`
3. If configuration is broken:
   ```
   switch# configure replace backup-config force
   ```

### If Configuration Is Wrong But Applied Successfully

1. Revert Terraform changes:
   ```bash
   git revert <commit>
   terraform apply
   ```

2. Or manually fix on switch:
   ```
   switch# configure terminal
   switch(config)# [corrective commands]
   switch(config)# end
   switch# copy running-config startup-config
   ```

### If Connectivity Lost

1. **Use console cable** (this is why you have one!)
2. Review recent changes:
   ```
   show archive config differences
   ```
3. Restore backup:
   ```
   configure replace flash:backup-config
   ```

## Best Practices

### DO:

✅ Test thoroughly before production
✅ Use version control for all Terraform files
✅ Review every `terraform plan` output
✅ Keep configuration backups
✅ Have console access available
✅ Start with non-critical resources
✅ Use `terraform import` for existing resources
✅ Schedule changes during maintenance windows
✅ Document all changes
✅ Have rollback procedures ready

### DON'T:

❌ Skip testing phase
❌ Apply changes without reviewing plan
❌ Hardcode credentials
❌ Make changes during business hours (initially)
❌ Manage critical VLANs without testing
❌ Forget to backup configuration
❌ Rely only on SSH access
❌ Make changes without team notification
❌ Use on production without lab testing

## Testing Sequence

Follow this sequence before production use:

1. **Run unit tests**: `./test.sh` ✅
2. **Test on isolated switch**: 1-2 hours ✅
3. **Create/read/update/delete test VLAN**: Verify manually ✅
4. **Test interface configuration**: On unused port ✅
5. **Test rollback procedure**: Practice emergency recovery ✅
6. **Staging environment test**: Full configuration ✅
7. **Production pilot**: One non-critical VLAN ✅
8. **Gradual expansion**: Add more resources incrementally ✅

## Support and Issues

If you encounter issues:

1. Check test results: `./test.sh`
2. Review TROUBLESHOOTING section in README.md
3. Check switch logs: `show logging`
4. Report bugs with:
   - Test results
   - Switch model and IOS version
   - Terraform version
   - Provider version
   - Anonymized configuration

## Remember

🔐 **Security**: Never commit credentials to version control

🧪 **Testing**: Test, test, test before production

📋 **Documentation**: Document everything you do

💾 **Backups**: Always have a backup plan

🚨 **Emergency Access**: Keep console access available

⏰ **Timing**: Use maintenance windows for changes

👥 **Communication**: Notify team before changes

🔄 **Reversibility**: Always have a rollback plan

---

**When in doubt, test more. Your network depends on it!**
