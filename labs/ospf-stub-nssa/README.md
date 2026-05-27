# OSPF Stub/NSSA Lab

This lab validates the supported FRR OSPFv2 area-type subset.

Topology:

```text
normal area 0       stub area 1
r1 -------- r2 ---------------- r3
            |
            | NSSA area 2
            r4
```

`r2` is the ABR. `r1` redistributes a static blackhole route from normal area 0. `r4` redistributes a static blackhole route from NSSA area 2.

Expected behavior:

- `r3` receives a stub default route from `r2`.
- `r4` receives an NSSA default route from `r2`.
- `r3` and `r4` do not receive `r1`'s normal external route.
- `r1` receives `r4`'s NSSA external route after ABR translation.

Supported syntax used here:

```frr
router ospf
 area AREA stub
 area AREA nssa default-information-originate
 redistribute static
```
