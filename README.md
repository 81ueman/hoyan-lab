# Hoyan-Style WAN Verifier Lab

This repository contains a medium-size WAN sandbox and verifier inspired by the
Hoyan papers. It is a partial reference implementation for study and
experimentation, not the production Hoyan system and not an implementation
published by the paper authors.

Related papers:

- Fangdan Ye et al., "Accuracy, Scalability, Coverage: A Practical
  Configuration Verifier on a Global WAN", SIGCOMM 2020.
  - DOI: https://doi.org/10.1145/3387514.3406217
  - SIGCOMM 2020 program: https://conferences.sigcomm.org/sigcomm/2020/program.html
  - PDF copy: https://ennanzhai.github.io/pub/hoyan-sigcomm20.pdf
- Yifei Yuan et al., "New Evolution of Hoyan: Enhancing Scalability, Usability,
  and Accuracy for Alibaba's Global WAN Verification", SIGCOMM 2025.
  - DOI: https://doi.org/10.1145/3718958.3754343
  - SIGCOMM 2025 program: https://conferences.sigcomm.org/sigcomm/2025/program/papers-info/
  - PDF copy: https://ennanzhai.github.io/pub/sigcomm25-hoyan2.pdf

The lab uses containerlab for the runnable topology and a Go verifier for
offline route, packet, and failure reachability checks.

The verifier treats the selected lab directory as the source of truth.
`labs/base-wan` is the default lab. Each lab's `hoyan.clab.yml` provides
containerlab inventory and physical links; its FRR, cEOS, and SR Linux startup
configs provide interfaces, BGP ASN, router-id, neighbors, and advertised
prefixes. Containers use containerlab's default `clab-<lab-name>-<node-name>`
names. The verifier builds a Hoyan-style network model from those configs: each
device has control-plane and data-plane pipelines made of ingress policy, route
selector, and egress policy. BGP route updates populate an extended RIB with
topology conditions, and the FIB is derived from the ranked RIB rules.

## Scenario Labs

Hoyan stores runnable inputs under `labs/<name>/`. A scenario lab contains:

```text
labs/<name>/
  hoyan.clab.yml
  configs/
  intent/hoyan.yml
  lab.yml
  README.md
```

`lab.yml` documents the lab name, description, NOS mix, supported checks, and
features under test. The initial scenario set is:

- `labs/base-wan`: standard multi-region WAN with FRR, cEOS, and SR Linux.
- `labs/acl-semantics`: ACL permit/deny/default-action and packet class checks.
- `labs/recursive-nexthop`: recursive BGP next-hop and RIB/FIB diagnostics.

Each scenario topology also uses a unique containerlab `name:` and management
network, so parallel scenario deploys do not reuse the same `clab-...` container
names or Docker network.

Use `--lab` to select a scenario. When `--lab` is set, model and live commands
default to `<lab>/hoyan.clab.yml`; `hoyan intent verify --lab ...` reads
`<lab>/intent/hoyan.yml`. Explicit `--topology` flags override topology
defaults for commands that use a containerlab file.
Without `--lab`, commands use `labs/base-wan`.

```bash
go run ./cmd/hoyan labs list
go run ./cmd/hoyan labs describe base-wan
go run ./cmd/hoyan labs check
go run ./cmd/hoyan labs check base-wan recursive-nexthop
go run ./cmd/hoyan intent verify --lab labs/base-wan --format json
go run ./cmd/hoyan live check --lab labs/base-wan
go run ./cmd/hoyan compare rib --lab labs/recursive-nexthop
go run ./cmd/hoyan compare fib --lab labs/recursive-nexthop
go run ./cmd/hoyan model packet-classes --lab labs/acl-semantics --prefix 10.4.0.0/16
```

## Intent Verify

```bash
go run ./cmd/hoyan intent verify --lab labs/base-wan --format json
```

Use strict config mode when the containerlab topology and startup configs are
the verification source of truth and unsupported parser syntax should fail the
run instead of being reported as warnings:

```bash
go run ./cmd/hoyan live check --strict-config
go run ./cmd/hoyan compare rib --strict-config
go run ./cmd/hoyan model rib --strict-config
```

Strict config errors include the vendor, config file, line number, raw
statement, and unsupported reason so CI logs point at the config syntax that
needs parser support or an intentional non-strict run.

Checks are defined in each lab's `intent/hoyan.yml`:

- RIB/FIB fact assertions over modeled rows
- packet reachability assertions with `packet.from`, `packet.to`, protocol, and destination port
- scenario failure constraints such as one failed core link

`--format json` emits a structured `hoyan.intent.report/v1` report with
`summary` and `results`. Packet results include `actual.reachable`, the modeled
path when available, and machine-readable failure counterexamples when a
failure scenario breaks an expected reachable flow.

Data-plane policies are parsed from the device startup configs.
Linux/FRR data-plane ACLs are stored as nftables rulesets under
`configs/frr/<node>/nftables.conf`; `hoyan live check` builds the local
`hoyan-frr-nftables:10.6.1` image and applies those rulesets after deploy.
The parser normalizes device ACLs into `model.ACL` plus `ACLBinding` records
before data-plane simulation. ACL rules are evaluated in sequence order with
first-match semantics, and both `permit` and `deny` are explicit actions.
When an ACL is bound to an interface and no rule matches, the model applies
the ACL's default action. cEOS IPv4 ACLs and SR Linux IPv4 ACL filters use an
implicit default deny unless an explicit permit rule matches. FRR/Linux
nftables ACLs use the chain policy; the current lab's nftables chain has
`policy accept`, so unmatched packets are permitted. `model.ACL` is the single
data-plane policy IR for parsed configs, manually constructed topologies, and
packet reachability inspection; the earlier deny-only `model.Policy`
compatibility path has been removed. Full vendor ACL grammar, stateful
firewall/conntrack, NAT, PBR, and QoS are intentionally outside the current
model.

Failure search for packet scenarios is symbolic. The intent verifier builds a
symbolic `NOT(reachable)` goal for the solver and reports the failing link or
node set when a scenario such as `failures.max: 1` invalidates an expected
reachable flow.

## Intent DSL and Fact Tables

`hoyan intent` provides the first `version: hoyan/v1` intent DSL for modeled
RIB/FIB and packet checks. Intent files define snapshots, scenarios, variables,
failure constraints, and assertions over deterministic fact rows or modeled
packet reachability:

```bash
go run ./cmd/hoyan intent validate --file testdata/intent/minimal.yml
go run ./cmd/hoyan intent expand --file testdata/intent/forall.yml --format json
go run ./cmd/hoyan intent verify --file testdata/intent/rib-fib-basic.yml --format json
go run ./cmd/hoyan intent verify --lab labs/base-wan --format json
```

The MVP supports scalar/list `vars`, `forall` expansion, `table: rib` and
`table: fib`, simple `where` selectors such as `device`, `device_in`, `prefix`,
`selected`, and `installed`, plus `exists` and `count` assertions. Packet
intents use `table: packet`, a `packet` block, and `assert.reachable`:

```yaml
scenarios:
  one-core-link-failure:
    snapshot: current
    failures:
      max: 1
      include_link_roles: [core]

intents:
  - name: customers-https-allowed
    check:
      table: packet
      scenario: one-core-link-failure
      packet:
        from: cust-bj
        to: 10.4.1.10
        protocol: tcp
        dst_port: 443
      assert:
        reachable: true
```

For example, `labs/base-wan/intent/hoyan.yml` checks that `10.4.0.0/16` is
visible in the modeled RIB on edge routers, HTTP to `10.4.1.10` is denied from
customers, and HTTPS is allowed. The JSON report uses
`hoyan.intent.report/v1` with deterministic result ordering so CI can compare
summary fields or individual result names.

Use `hoyan facts` when you want the modeled RIB/FIB fact tables directly:

```bash
go run ./cmd/hoyan facts rib --lab labs/base-wan --format json
go run ./cmd/hoyan facts fib --lab labs/base-wan --format json
```

These facts are built offline from the lab topology and startup configs via the
same control-plane and data-plane model used by `hoyan model`. Live-device
collection and additional RCL-style workflows are intentionally kept separate
from offline intent verification.

## Compare Modeled BGP RIBs With Live Nodes

## Live Snapshots

Use `hoyan live snapshot` to collect live BGP RIB, all-source route-table, and
installed FIB state once and save it as reusable JSON. This is useful when you
want to iterate on parser, normalizer, or compare logic without collecting the
same device state on every run:

```bash
go run ./cmd/hoyan live snapshot --lab labs/base-wan --output labs/base-wan/snapshots/latest.json
go run ./cmd/hoyan live snapshot --topology labs/base-wan/hoyan.clab.yml --output labs/base-wan/snapshots/live-state.json --raw-dir labs/base-wan/snapshots/raw
```

The snapshot includes the topology hash, referenced config file hashes, the
current git commit when available, collection time, node kinds, normalized RIB
routes, normalized installed FIB routes, and unresolved FIB diagnostics. When
`--raw-dir` is set, raw vendor command output is written beside the snapshot for
parser regression fixtures.

Compare commands can reuse the saved state without connecting to devices:

```bash
go run ./cmd/hoyan compare rib --lab labs/base-wan --snapshot labs/base-wan/snapshots/latest.json
go run ./cmd/hoyan compare fib --lab labs/base-wan --snapshot labs/base-wan/snapshots/latest.json
go run ./cmd/hoyan live check --lab labs/base-wan --snapshot labs/base-wan/snapshots/latest.json --offline
```

By default, snapshot compare warns when the current topology or referenced
configs no longer match the hashes saved in the snapshot. Use
`--snapshot-hash-policy fail` to make a mismatch fail, or `ignore` to skip the
check. `hoyan live check --snapshot` skips RIB/FIB collection; without `--offline` it
still deploys the lab and runs live packet probes.

## Inspect Modeled RIB, FIB, and Symbolic Paths

Use `hoyan model` to inspect the offline model built from the containerlab
topology and device configs without collecting live device state:

```bash
go run ./cmd/hoyan model rib --node bj-edge1
go run ./cmd/hoyan model rib --node bj-edge1 bgp
go run ./cmd/hoyan model rib --node bj-edge1 --prefix 10.4.0.0/16 --format json
go run ./cmd/hoyan model rib --node bj-edge1 connected
go run ./cmd/hoyan model fib --node bj-edge1
go run ./cmd/hoyan model fib --node bj-edge1 --prefix 10.4.0.0/16 --format json
go run ./cmd/hoyan model prefix-classes --prefix 10.4.0.0/16
go run ./cmd/hoyan model prefix-classes --prefix 10.4.0.0/16 --show-predicates
go run ./cmd/hoyan model packet-classes --prefix 10.4.0.0/16 --show-predicates
go run ./cmd/hoyan model symbolic-packet --from cust-bj --to 10.4.1.10 --protocol tcp
go run ./cmd/hoyan model symbolic-route --from bj-edge1 --prefix 10.4.0.0/16 --format json
go run ./cmd/hoyan model symbolic-route --from bj-edge1 --prefix 10.4.0.0/16 --show-conditions
go run ./cmd/hoyan model symbolic-route --from bj-edge1 --prefix 10.4.0.0/16 --show-predicates
```

The default table views keep symbolic conditions hidden so route and prefix
splits stay readable. Add `--show-conditions` to `model rib`, `model fib`,
`model symbolic-packet`, or `model symbolic-route` when you need route
existence, selected-route, install, or reachability conditions. JSON output
still includes condition fields for `jq` or Codex.

The `prefix-classes` view shows the PrefixUniverse classes derived from
advertised route prefixes, prefix-list predicates, policy destination prefixes,
modeled RIB/FIB prefixes, and an optional `--prefix` request predicate.
Matched predicates are hidden in table output by default; add
`--show-predicates` to `model prefix-classes` or `model symbolic-route` when
you need to see which predicates matched each class.
The `packet-classes` view builds HeaderSpace classes over only the header
dimensions that have predicate boundaries: destination prefix class, protocol,
source/destination port, and ingress/egress interface. This keeps packet
predicate inspection tied to PrefixUniverse without creating unused cross
products. Add `--show-predicates` to see the ACL or request predicates that
matched each packet class.
Add `--summary` to `model prefix-classes` to print PrefixUniverse build
statistics, including predicate count, unique predicate count, class count,
build duration, max CIDRs per class, and predicate source categories. Use
`--max-prefix-classes` with `model prefix-classes` to fail early when class
expansion exceeds the requested guard.
`model symbolic-route --prefix` uses the same request-aware PrefixUniverse and
emits one symbolic route result per matching class, including `class_id`,
`space`, and reachable/unreachable conditions. JSON output still includes
`matched_predicates`.
`model symbolic-packet` remains IP-address based.

Modeled FIB semantics use reachability OR for explicitly grouped ECMP /
equivalent candidates: entries with the same prefix, rank, and `group_id` do
not suppress each other, and packet reachability may use any live member in the
group. Lower-rank or shorter-prefix candidates remain suppressed while a
higher-priority group is selected. This is a safety-oriented abstraction; it
does not model per-flow hashing or sticky hash buckets. The default BGP
decision process treats routes as equivalent when local-pref, local-origin,
AS-path length, MED, and eBGP/iBGP status tie. FRR currently installs only one
such equivalent route in the modeled FIB, while the generic behavior can keep
multiple equivalent routes. The FIB compare expected-state builder separately
normalizes selected FRR multipath RIB entries into an ECMP next-hop set for
live kernel FIB comparison. cEOS and SR Linux do not currently expose
equivalent FIB install groups in this model.

Modeled packet forwarding treats `FIBEntry.NextHop` as a resolved adjacent
topology node, not as a raw BGP next-hop address. Recursive BGP next-hop
resolution is therefore a control-plane/FIB derivation responsibility before an
entry can be used by the modeled dataplane. This model does not implement full
recursive FIB lookup yet: an address-only next-hop is marked
`unresolved_recursive_next_hop`, and packet reachability reports
`recursive next-hop unresolved` instead of forwarding over an inferred path. If
a selected FIB entry names a topology node that is not adjacent to the current
node, packet reachability reports `next-hop is not adjacent`; if the adjacent
link exists but is failed, it reports `next-hop link is down`. Entries that are
known to resolve through the Docker management/default interface are reported
as `next-hop resolved via management interface`. These terms intentionally
mirror FIB compare diagnostics such as `unresolved_recursive_next_hop` and
management fallback so modeled dataplane output and live FIB warnings describe
the same class of issue.

Connected routes are classified when the model derives routes from interface
addresses. `link` means the interface belongs to a containerlab topology link,
`loopback` means a loopback interface on an infrastructure node, `service`
means a loopback interface on a customer/service/host node, and `host` means a
non-loopback host-length connected prefix. `hoyan model rib --format json` and
`hoyan model fib --format json` include `connected_class` for connected routes.
Live RIB/FIB compare canonicalizes vendor protocol names such as `kernel`,
`local`, `direct`, and `connected` to `connected`; `link`, `loopback`, and
`service` connected routes are compared, while unclassified host connected
routes remain outside the strict compare set.

When running Hoyan from multiple git worktrees, render an isolated topology per
worktree first. The suffix is appended to the lab name and Docker management
network name, derives a separate `172.86.<n>.0/24` management subnet, keeps
containerlab's default naming, and keeps the relative config paths valid from
the selected lab directory. Keep the generated topology in the lab directory
when you want relative config paths to stay readable:

```bash
go run ./cmd/hoyan topology render --lab base-wan --suffix issue-21 --output labs/base-wan/hoyan.issue-21.clab.yml
```

For `-suffix issue-21`, containers use containerlab's default names such as
`clab-hoyan-base-wan-issue-21-bj-edge1`. Use the generated topology with live
commands:

```bash
go run ./cmd/hoyan live check --topology labs/base-wan/hoyan.issue-21.clab.yml
go run ./cmd/hoyan compare rib --topology labs/base-wan/hoyan.issue-21.clab.yml
```

To run the full live integration check, including deploy, BGP convergence wait,
modeled-vs-live RIB comparison for BGP, connected, and static route sources,
and cleanup:

```bash
go run ./cmd/hoyan live check
```

By default, the command polls live BGP RIB state up to five times with a 25s
interval. This keeps polling bounded while leaving enough room for all BGP
sessions to come up after a fresh deploy. If expected routes are still missing
or attributes do not match, it prints modeled-vs-live diffs instead of waiting
for the full timeout:

```bash
go run ./cmd/hoyan live check --max-polls 5 --poll-interval 25s
```

After BGP converges, `hoyan live check` collects the first-class RIB route-table
view for non-BGP sources and compares modeled BGP, connected, and static routes.
The output includes a source summary such as `bgp=10, connected=4, static=2`.
BGP RIB comparison is exact on prefixes, paths, best flag, valid flag,
next-hop, AS path, origin, local-pref, and MED.

For debugging, keep the lab running if the comparison fails:

```bash
go run ./cmd/hoyan live check --keep-on-failure
```

To keep the lab running even on success:

```bash
go run ./cmd/hoyan live check --skip-destroy
```

If the lab is already deployed, compare the modeled RIB with live nodes
directly. This compares BGP, connected, and static route sources:

```bash
go run ./cmd/hoyan compare rib
```

To compare the no-failure modeled FIB with live installed Linux kernel routes,
run:

```bash
go run ./cmd/hoyan compare fib
```

`hoyan compare fib` normalizes modeled BGP, next-hop static, Null0/blackhole, and
comparable connected FIB entries with live installed FIB entries by node, VRF,
AFI, source protocol, prefix, and next-hop set. FRR `Null0`, cEOS
`Null0`/discard, and SR Linux blackhole/discard routes are canonicalized as
source protocol `blackhole` with no next-hop, and modeled FIB JSON marks them
with `discard: true`. Packet reachability reports these as `discard route
selected`: the route exists and explicitly drops traffic. This is distinct from
`no forwarding route`, where no selected FIB candidate matches the packet, and
`selected route has no next-hop`, where a selected non-discard route is missing
forwarding next-hop metadata. When a local blackhole static and a BGP `network`
route use the same prefix, RIB compare keeps both sources as separate entries,
while FIB compare expects the local blackhole install and does not require a
same-prefix local BGP forwarding entry. BGP aggregate routes are modeled as BGP
control-plane advertisements; they are not treated as local blackhole/discard
FIB entries unless the device also has an explicit discard route. A comparable
live BGP route must have a next-hop that resolves to a topology data-plane
interface; if the kernel route falls back to a
management/default interface such as `eth0`, or the recursive next-hop cannot be
mapped to a topology link, the route is reported as
`unresolved_or_mgmt_fallback`, `unresolved_recursive_next_hop`, or
`topology_interface_missing`. By default these unresolved live routes are
warnings and are excluded from the strict set comparison, because they are live
installed routes whose forwarding cannot be verified against the topology. Use
`go run ./cmd/hoyan compare fib --unresolved-policy fail` to make them fail the
run, or `--unresolved-policy ignore` to keep the exclusion silent. It reports
missing routes, unexpected routes, missing next-hops, and unexpected next-hops,
including ECMP group differences. Live collectors currently use:

```bash
docker exec -i <frr-node> ip -j route show table main
docker exec -i <ceos-node> Cli -p 15 -c "show ip route vrf default | json"
docker exec -i <srlinux-node> sr_cli --output-format json --pagination off -- show network-instance default route-table ipv4-unicast summary
docker exec -i <srlinux-node> sr_cli --output-format json --pagination off -- show network-instance default route-table ipv4-unicast prefix <prefix> detail
```

`hoyan live check` runs the same comparison after BGP RIB convergence by default:

```bash
go run ./cmd/hoyan live check
```

Use `--no-check-fib` to skip the installed FIB comparison for a quick
control-plane/dataplane-only run. `hoyan live check` uses the same unresolved-route
policy with `--fib-unresolved-policy warn|fail|ignore`; the default is `warn`.

Limitations: the modeled side uses the no-failure installed FIB only, Linux
kernel BGP routes are the FRR source of truth, cEOS compares programmed routes
from EOS route JSON, SR Linux compares active route-table entries from
`ipv4-unicast summary` and uses per-prefix `detail` output for SR Linux BGP and
static next-hop peer gateway addresses. Protocol/metric/preference fields are
normalized for inspection but the first comparison target is protocol plus
prefix plus next-hop address/interface set, default routes and unclassified
host connected routes are out of scope, and hardware ASIC FIB or per-flow ECMP
hashing is not verified. BGP routes whose live next-hop cannot be mapped to a
topology data-plane interface are diagnostics rather than silent skips.

The live comparison reads BGP table state from FRR, cEOS, and SR Linux nodes,
not kernel routes, installed route tables, or dataplane forwarding state. It
collects:

```bash
docker exec -i <frr-node> vtysh -c "show ip bgp json"
docker exec -i <ceos-node> Cli -p 15 -c "show ip bgp | json"
docker exec -i <srlinux-node> sr_cli --output-format json --pagination off -- show network-instance default protocols bgp routes ipv4 summary
docker exec -i <srlinux-node> sr_cli --output-format json --pagination off -- show network-instance default protocols bgp routes ipv4 prefix <prefix> detail
```

Routes are normalized by node, network-instance, AFI, prefix, and BGP path.
Vendor-specific formatting differences such as local next-hop representation,
origin spelling, and omitted default local-pref are normalized before exact
comparison.

Vendor-specific BGP RIB behavior is modeled where it affects live comparison.
cEOS can keep paths with unresolved next-hops in the BGP table as invalid
paths, and SR Linux can retain AS-loop paths as invalid BGP RIB entries. Those
paths are not used for forwarding, but they are represented in the modeled BGP
RIB so live table comparison stays aligned with the devices.

### Modeled BGP Decision Process

`DefaultBGPDecisionProcess` is a Hoyan model approximation, not a complete
vendor implementation. It currently orders candidate BGP routes as follows:

1. Higher local-pref.
2. Locally originated route.
3. Shorter AS path.
4. Lower origin-code preference: IGP, then EGP, then incomplete.
5. Lower MED. The default model preserves the historical approximation and
   compares MED across neighboring ASNs; `BGPDecisionOptions.AlwaysCompareMED`
   documents this knob and can be disabled for same-neighbor-AS-only MED tests.
6. eBGP over iBGP.
7. Shorter modeled path length.
8. Stable lexical tie-break over modeled path nodes.

The modeled path length and lexical tie-break are deterministic simulator
tie-breaks, not vendor bestpath rules. FRR currently uses the same route
attributes but keeps a vendor-specific reverse lexical tie-break so live RIB
comparison remains stable for this lab.

Known unsupported or approximated bestpath knobs:

- Weight: unsupported.
- IGP cost to next-hop: unsupported.
- Router-id tie-break: documented in `BGPDecisionOptions`, unsupported until
  modeled routes carry router-id attributes.
- Originator-id and cluster-list length: unsupported.
- Deterministic MED: documented in `BGPDecisionOptions`, unsupported.
- Always-compare-MED: documented in `BGPDecisionOptions`; the default model
  currently uses the always-compare approximation for backward compatibility.
- Compare-routerid: documented in `BGPDecisionOptions`, unsupported.
- Multipath / ECMP install policy: documented in `BGPDecisionOptions`; route
  equivalence and FIB install semantics are tracked separately in #65.
- Vendor-specific invalid route retention: partially modeled in device
  behavior for cEOS unresolved next-hops and SR Linux AS-loop paths.
- Vendor-specific route-map / policy side effects: only the parsed policy
  actions represented in the model are applied.

## Deploy

```bash
containerlab deploy --reconfigure
```

Destroy with:

```bash
containerlab destroy --cleanup
```

Useful FRR checks:

```bash
docker exec -it clab-hoyan-base-wan-bj-edge1 vtysh -c "show ip bgp summary"
docker exec -it clab-hoyan-base-wan-bj-edge1 vtysh -c "show ip route bgp"
docker exec -it clab-hoyan-base-wan-cust-bj ping -c 3 10.4.1.10
```

## Z3

The default verifier backend enumerates small failure sets so normal tests work
without native libraries. A Z3-backed solver is available behind the `z3` build
tag and uses cgo against `libz3`.

On Debian:

```bash
sudo apt-get update
sudo apt-get install -y libz3-dev
go test -tags z3 ./...
```

## Tests

```bash
go test ./...
go test -tags z3 ./...
```
