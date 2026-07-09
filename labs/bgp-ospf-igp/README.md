# BGP + OSPF IGP Lab

## Topology

```
edge1 (AS 65001) --- core1 (AS 65000) --- core2 (AS 65000) --- edge2 (AS 65002)
```

## Design

- **OSPF** (area 0) runs on all four routers as the IGP, providing reachability for loopbacks and connected interfaces.
- **BGP** uses a hybrid AS design:
  - `edge1` (AS 65001) establishes eBGP with `core1` (AS 65000) over their direct link.
  - `core1` and `core2` (both AS 65000) establish iBGP between themselves.
  - `core2` (AS 65000) establishes eBGP with `edge2` (AS 65002) over their direct link.
  - Routes propagate: `edge1 → core1 → core2 → edge2` and vice versa.
- **Next-hop resolution**: OSPF provides IGP reachability for all link and loopback addresses, enabling BGP next-hop resolution.
- Each edge router advertises its own loopback prefix via BGP:
  - `edge1` → `10.255.1.1/32`
  - `edge2` → `10.255.4.4/32`

> **Note on iBGP**: The original spec called for direct iBGP peering between edge1 and edge2. The model enforces iBGP split-horizon (routes learned from iBGP are not re-advertised to other iBGP peers) and does not support multihop BGP sessions between non-directly-connected peers. The hybrid eBGP/iBGP design achieves the same cross-protocol verification goal: BGP routes traverse the network, OSPF provides the IGP underlay, and both protocols' routes coexist in every router's RIB.

## Verification

```bash
# Show the model RIB
go run ./cmd/hoyan model rib --lab labs/bgp-ospf-igp

# Verify intents
go run ./cmd/hoyan intent verify --lab labs/bgp-ospf-igp --format json
```

## Intent Tests

The intents verify:
1. **OSPF routes** exist on all routers (IGP backbone)
2. **BGP routes** exist on all routers (propagated via BGP)
3. **Cross-protocol**: each router has both OSPF and BGP routes for the same prefixes
4. **Edge-to-edge**: edge routers receive BGP routes from their remote peer
5. **Core underlay**: core routers have OSPF routes providing BGP next-hop resolution
