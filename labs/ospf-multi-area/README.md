# OSPF Multi-Area Lab

This lab validates FRR OSPFv2 ABR behavior and inter-area route modeling.

Topology:

```text
area 1          area 0          area 2
r1 ---- r2 ---- r3 ---- r4
       ABR \
            \___ r5 ---- r6
                 ABR   area 3
```

Loopback advertisements:

- r1: `10.255.1.1/32` in area 1
- r2: `10.255.2.2/32` in area 0
- r3: `10.255.3.3/32` in area 0
- r4: `10.255.4.4/32` in area 2
- r5: `10.255.5.5/32` in area 0
- r6: `10.255.6.6/32` in area 3

`r2` connects area 1 to the backbone, `r3` connects area 2, and `r5`
connects area 3. Routes between non-backbone areas, such as `r1` to `r4`
or `r6`, must be modeled and normalized as inter-area OSPF routes.

Redistribution, stub areas, NSSA, and OSPFv3 are intentionally not used.
