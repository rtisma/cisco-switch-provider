# cisco-switch-provider – Terraform Provider for Cisco Catalyst Switches

## What this is
A Terraform Plugin Framework provider that configures Cisco IOS switches over SSH using private key authentication.

Module path: `github.com/example-org/terraform-provider-cisco`
Provider type name: `cisco`
Registry source: `registry.terraform.io/example-org/cisco`

## Build & install
```bash
go build -o terraform-provider-cisco
# or:
make install
# Copies binary to:
# ~/.terraform.d/plugins/registry.terraform.io/example-org/cisco/1.0.0/linux_amd64/
```

## Key directories

| Path | Purpose |
|------|---------|
| `internal/provider/provider.go` | Provider entry point; `Resources()` lists all resource constructors |
| `internal/provider/client/client.go` | SSH client struct, `ExecuteCommand`, `ExecuteConfigCommands` (runs `write memory` after every change) |
| `internal/provider/client/ssh.go` | SSH connection setup, private key auth, PTY, shell, `sendCommand`, `readUntilPrompt` |
| `internal/provider/client/session.go` | CLI mode detection, prompt regexes, mode transitions (no enable password logic) |
| `internal/provider/client/parser.go` | All `ParseXxx` functions that read `show running-config` output |
| `internal/provider/client/errors.go` | `IsErrorOutput`, `CiscoError` type |
| `internal/provider/resources/` | One file per resource type |
| `README.md` | Complete user-facing reference — all resources, argument tables, examples, Prometheus integration |

## Provider configuration

```hcl
provider "cisco" {
  host             = "192.168.1.1"
  username         = "admin"
  private_key_path = "~/.ssh/id_rsa"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `host` | yes | Switch IP or hostname |
| `username` | yes | SSH username |
| `private_key_path` | yes | Path to SSH private key file |
| `port` | no (default 22) | SSH port |
| `ssh_timeout` | no (default 30s) | SSH connection timeout |
| `command_timeout` | no (default 10s) | Per-command timeout |

**Password and enable_password are not supported.** The login account must have privilege level 15 so the provider reaches `#` prompt immediately after login (configure via `username admin privilege 15` on the switch).

## Resources implemented

| Resource | File | IOS command(s) |
|----------|------|----------------|
| `cisco_vlan` | resource_vlan.go | `vlan <id>` |
| `cisco_interface` | resource_interface.go | `interface <name>` (switchport access/trunk) |
| `cisco_svi` | resource_svi.go | `ip routing` + `interface vlan <id>` (Layer 3 gateway) |
| `cisco_interface_ip` | resource_interface_ip.go | `ip address` on an interface (management) |
| `cisco_dhcp_pool` | resource_dhcp_pool.go | `ip dhcp pool` (switch as DHCP server) |
| `cisco_dhcp_excluded_range` | resource_dhcp_excluded_range.go | `ip dhcp excluded-address <low> [<high>]` |
| `cisco_dhcp_host` | resource_dhcp_host.go | named DHCP pool with `host` + `hardware-address` (MAC→IP binding) |
| `cisco_acl_rule` | resource_acl_rule.go | **local-only** — no switch writes; `id` = IOS ACE string |
| `cisco_acl_policy` | resource_acl.go | `ip access-list extended/standard` (writes ACL to switch) |
| `cisco_snmp_community` | resource_snmp_community.go | `snmp-server community <name> ro/rw [acl]` |
| `cisco_snmp` | resource_snmp.go | `snmp-server location/contact/host` (singleton) |
| `cisco_static_route` | resource_static_route.go | `ip route <net> <mask> <hop> [<dist>]` |
| `cisco_ip_routing` | resource_ip_routing.go | `ip routing` (singleton, enables Layer 3) |

## Important design decisions & gotchas

### Authentication — SSH private key only
`client/ssh.go` reads the key file at `Config.PrivateKeyPath`, parses it with `ssh.ParsePrivateKey`, and uses `ssh.PublicKeys(signer)` as the sole auth method. There is no password fallback. `Config` has no `Password` or `EnablePassword` fields.

### No enable password
Enable password handling has been removed from `session.go` and `ssh.go`. The provider sends `enable` and expects to receive the `#` prompt directly. The switch account must be configured at privilege level 15 to make this work.

### write memory
`ExecuteConfigCommands` in `client.go` runs `write memory` after **every** successful config change. All Create/Update/Delete operations persist to startup-config automatically. Do not remove this.

### cisco_svi auto-enables ip routing
`cisco_svi` prepends `"ip routing"` to its config command list in both `Create` and `Update`. This means ip routing is always enabled before the SVI is configured, regardless of Terraform resource declaration order. No `depends_on` is needed. IOS ignores the command if routing is already enabled.

### cisco_acl_rule is local-only
`cisco_acl_rule` never touches the switch. Its `id` is the IOS ACE command string. Every attribute uses `RequiresReplace` so any change creates a new resource with a new id. The `cisco_acl_policy` resource detects the id change and recreates the ACL atomically.

### cisco_acl_policy file naming
The resource is named `cisco_acl_policy` (TypeName = `_acl_policy`) but the file is `resource_acl.go` and the Go structs are `ACLPolicyResource` / `ACLPolicyResourceModel`.

### cisco_acl_policy update strategy
On any change to the `rules` list, the entire ACL is deleted (`no ip access-list ...`) and recreated. Sequence numbers are derived from list position: index 0 → seq 10, index 1 → seq 20, etc.

### Singleton resources
`cisco_snmp` and `cisco_ip_routing` are singletons — only one should exist per switch. Both set a static computed `id` using `stringplanmodifier.UseStateForUnknown()`.

### CLI sub-modes
`session.go` must recognise every IOS sub-mode prompt or `readUntilPrompt` will hang. Current regexes cover:
- `(config)#`, `(config-if)#`, `(config-vlan)#`
- `(dhcp-config)#` — for `ip dhcp pool`
- `(config-ext-nacl)#`, `(config-std-nacl)#` — for `ip access-list`

If you add a resource that enters a new sub-mode, add the corresponding regex to `session.go` (`hasPrompt` and `detectModeFromPrompt`).

### Parser conventions
- Read with `show running-config | include <keyword>` for flat global commands (SNMP, static routes, DHCP excluded)
- Read with `show running-config | section <keyword>` for block config (DHCP pool, ACL)
- Read with `show running-config interface <name>` for interface blocks (SVI, switchport)
- `normalizeMAC(mac)` converts Cisco dotted-hex (`xxxx.xxxx.xxxx`) → lowercase colon format (`xx:xx:xx:xx:xx:xx`)

## Parser functions (client/parser.go)

| Function | Input command | Returns |
|----------|--------------|---------|
| `ParseShowVlan` | `show vlan id <id>` | `*VLANInfo` |
| `ParseInterfaceConfig` | `show running-config interface <name>` | `*InterfaceInfo` |
| `ParseSVIConfig` | `show running-config interface vlan <id>` | `*SVIInfo` |
| `ParseACL` | `show running-config \| section ip access-list <name>` | `*ACLInfo` |
| `ParseDHCPPool` | `show running-config \| section ip dhcp pool <name>` | `*DHCPPoolInfo` |
| `ParseDHCPExcludedRange` | `show running-config \| include ip dhcp excluded` | `*DHCPExcludedRangeInfo` |
| `ParseDHCPHostPool` | `show running-config \| section ip dhcp pool <name>` | `*DHCPHostInfo` |
| `ParseSNMPCommunity` | `show running-config \| include snmp-server community` | `*SNMPCommunityInfo` |
| `ParseSNMP` | `show running-config \| include snmp-server` | `*SNMPInfo` |
| `ParseStaticRoute` | `show running-config \| include ip route` | `*StaticRouteInfo` |

## Coding conventions
- All resources follow the same CRUD + `ImportState` pattern
- `buildCommands()` helper returns IOS commands slice; last element is always `"end"`
- Delete: use `no <full-command>`; check `containsError(err, "does not exist")` for idempotent deletes
- Optional cleared fields: emit `no <field>` in `Update` when old value was set and new value is empty
- `containsError(msg, substr)` — helper in `resource_vlan.go` (resources package), used across all files
- `splitImportID(id, sep, n)` — helper in `resource_acl.go` for multi-part import IDs (e.g. `"extended/ACL-NAME"`)

## Adding a new resource (checklist)
1. Add `XxxInfo` struct + `ParseXxx()` in `client/parser.go`
2. Create `resources/resource_xxx.go` with `Metadata` / `Schema` / `Configure` / `Create` / `Read` / `Update` / `Delete` / `ImportState`
3. Register `resources.NewXxxResource` in `provider.go` → `Resources()` slice
4. Add to the resource table in `README.md`
5. `go build ./...` to verify
