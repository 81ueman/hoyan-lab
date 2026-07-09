# BGP + OSPF IGP Lab

## Topology

```
edge1 --- core1 --- core2 --- edge2
```

- **edge1**, **edge2**: iBGP edge routers (AS 65000)
- **core1**, **core2**: Core routers running OSPF as IGP

## Design

- OSPF (area 0) runs on all four routers as the IGP, providing reachability for loopback interfaces.
- iBGP peering (AS 65000) is established between edge1 and edge2 using their loopback addresses (10.255.1.1 and 10.255.4.4).
- OSPF provides recursive next-hop resolution for the BGP routes.
- Each edge router advertises its own loopback prefix via BGP: 10.255.1.1/32 (edge1) and 10.255.4.1/32 (edge2).
- The core routers (core1, core2) do not run BGP but carry the BGP routes via OSPF-provided next-hop resolution.

## Verification

```bash
# Show the model RIB
go run ./cmd/hoyan model rib --lab labs/bgp-ospf-igp

# Verify intents
go run ./cmd/hoyan intent verify --lab labs/bgp-ospf-igp --format json
```

## Intent Tests

The intents verify:
1. BGP routes exist on edge routers (from iBGP advertisements)
2. OSPF routes exist on all routers (the IGP)
3. Edge routers receive BGP routes from their peer
4. Core routers have OSPF routes to edge loopbacks (providing BGP next-hop resolution)
