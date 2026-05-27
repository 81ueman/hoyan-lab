# OSPF Broadcast

This lab models an OSPF broadcast multi-access segment. Routers `r1`, `r2`, and `r3` share `198.51.100.0/29` through `sw1`; `r4` hangs off `r3` over a point-to-point link.

The lab is intended to verify that the control-plane model can form adjacencies across a shared segment, learn loopbacks through that segment, and keep route, packet, failure, RIB, FIB, and live-check behavior aligned with FRR.
