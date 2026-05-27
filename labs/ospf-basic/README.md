# OSPF Basic Lab

This lab validates the supported multi-vendor OSPFv2 subset in Hoyan.

Topology:

```text
      cost 10
r1(FRR) --- r2(cEOS)
|           |
| cost 1    | cost 1
|           |
r4(FRR) --- r3(SR Linux)
      cost 1
```

Each router advertises its loopback in area 0:

- r1: `10.255.1.1/32`
- r2: `10.255.2.2/32`
- r3: `10.255.3.3/32`
- r4: `10.255.4.4/32`

The direct `r1-r2` link has OSPF cost 10. The other three links have cost 1, so `r1` reaches `r2`'s loopback through `r4-r3-r2` while all links are up. If `r1-r4` fails, the model and live FRR state should fall back to the direct `r1-r2` link.

Supported FRR/cEOS syntax used here:

```frr
router ospf
 ospf router-id A.B.C.D
 network A.B.C.D/M area AREA
 passive-interface IFNAME
interface IFNAME
 ip ospf area AREA
 ip ospf cost COST
 ip ospf network point-to-point
 ip ospf hello-interval 1
 ip ospf dead-interval 3
```

Redistribution is intentionally not used and remains unsupported for OSPF in this lab.

Supported SR Linux syntax used here:

```text
set / network-instance default protocols ospf instance default router-id A.B.C.D
set / network-instance default protocols ospf instance default area AREA interface IFNAME admin-state enable
set / network-instance default protocols ospf instance default area AREA interface IFNAME interface-type point-to-point
set / network-instance default protocols ospf instance default area AREA interface IFNAME metric COST
set / network-instance default protocols ospf instance default area AREA interface IFNAME passive true
```
