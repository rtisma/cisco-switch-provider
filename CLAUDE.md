# cisco-switch-provider – Terraform Provider for Cisco Catalyst Switches

## What this is
A Terraform Plugin Framework provider that configures Cisco IOS switches over SSH.

Module path: `github.com/example-org/terraform-provider-cisco`
Provider type name: `cisco`
Registry source: `registry.terraform.io/example-org/cisco`

## Build & install
```bash
go build -o terraform-provider-cisco
# or:
make install
# Copies binary to:
# ~/.terraform.d/plugins/registry.terraform.io/example-org/cisco/1.0.0/darwin_arm64/
```

## Key directories

| Path | Purpose |
|------|---------|
| `internal/provider/provider.go` | Provider entry point; `Resources()` lists all resource constructors |
| `internal/provider/client/client.go` | SSH client struct, `ExecuteCommand`, `ExecuteConfigCommands` (runs `write memory` after every change) |
| `internal/provider/client/session.go` | CLI mode detection, prompt regexes, mode transitions |
| `internal/provider/client/parser.go` | All `ParseXxx` functions that read `show running-config` output |
| `internal/provider/client/errors.go` | `IsErrorOutput`, `CiscoError` type |
| `internal/provider/resources/` | One file per resource type |
| `README.md` | Complete user-facing reference — all resources, argument tables, examples, Prometheus integration |

## Resources implemented

| Resource | File | IOS command(s) |
|----------|------|----------------|
| `cisco_vlan` | resource_vlan.go | `vlan <id>` |
| `cisco_interface` | resource_interface.go | `interface <name>` (switchport access/trunk) |
| `cisco_svi` | resource_svi.go | `interface vlan <id>` (Layer 3 gateway) |
| `cisco_interface_ip` | resource_interface_ip.go | `ip address` on an interface (management) |
| `cisco_dhcp_pool` | resource_dhcp_pool.go | `ip dhcp pool` (switch as DHCP server) |
| `cisco_dhcp_excluded_range` | resource_dhcp_excluded_range.go | `ip dhcp excluded-address <low> [<high>]` |
| `cisco_dhcp_host` | resource_dhcp_host.go | named DHCP pool with `host` + `hardware-address` (MAC→IP binding) |
| `cisco_acl_rule` | resource_acl_rule.go | **local-only** — no switch writes; `id` = IOS ACE string |
| `cisco_acl_policy` | resource_acl.go | `ip access-list extended/standard` (writes ACL to switch) |
| `cisco_svi` access groups | resource_svi.go | `ip access-group <name> in/out` on SVI |
| `cisco_snmp_community` | resource_snmp_community.go | `snmp-server community <name> ro/rw [acl]` |
| `cisco_snmp` | resource_snmp.go | `snmp-server location/contact/host` (singleton) |
| `cisco_static_route` | resource_static_route.go | `ip route <net> <mask> <hop> [<dist>]` |
| `cisco_ip_routing` | resource_ip_routing.go | `ip routing` (singleton, enables Layer 3) |

## Important design decisions & gotchas

### write memory
`ExecuteConfigCommands` in `client.go` runs `write memory` after **every** successful config change. This means all Create/Update/Delete operations persist to startup-config automatically. Do not remove this — it was deliberately added to prevent config loss on reboot.

### cisco_acl_rule is local-only
`cisco_acl_rule` never touches the switch. Its `id` is the IOS ACE command string (e.g. `"permit ip 192.168.100.0 0.0.0.255 any"`). Every attribute uses `RequiresReplace` so any change creates a new resource with a new id. The `cisco_acl_policy` resource detects the id change and recreates the ACL atomically.

### cisco_acl_policy file naming
The resource is named `cisco_acl_policy` (TypeName = `_acl_policy`) but the file is still `resource_acl.go` and the Go structs are `ACLPolicyResource` / `ACLPolicyResourceModel`. This was renamed from `cisco_acl` mid-session.

### cisco_acl_policy update strategy
On any change to the `rules` list, the entire ACL is deleted (`no ip access-list ...`) and recreated. This guarantees exact rule order and no stale entries. Sequence numbers are derived from list position: index 0 → seq 10, index 1 → seq 20, etc.

### Singleton resources
`cisco_snmp` and `cisco_ip_routing` are singletons — only one should exist per switch. Both set `id = "snmp"` / `id = "ip_routing"` as a static computed value using `stringplanmodifier.UseStateForUnknown()`.

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

## Recent session summary (for continuity)
The following work was completed in the most recent session:

- **cisco_dhcp_pool**: switch acts as DHCP server (`ip dhcp pool`)
- **cisco_acl_rule + cisco_acl_policy**: named ACLs with ordered rule list; `cisco_acl_rule` is local-only, `cisco_acl_policy` writes to switch; renamed from `cisco_acl`
- **cisco_svi**: extended with `dhcp_servers` (relay), `access_group_in`, `access_group_out`
- **cisco_snmp_community + cisco_snmp**: SNMP configuration for Prometheus SNMP Exporter polling and Alertmanager trap integration
- **write memory**: added to `ExecuteConfigCommands` so all changes are persisted automatically
- **README.md**: rewritten as single comprehensive reference with all 9+ resources, argument tables, examples, import syntax, and Prometheus/Alertmanager integration guide
- **provider.go** also registers: `cisco_static_route`, `cisco_dhcp_excluded_range`, `cisco_dhcp_host`, `cisco_ip_routing` (resource files + parser functions already present)
