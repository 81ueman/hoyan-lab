# OSPF VRF

FRR/Linux-only lab for OSPF processes scoped to non-default VRFs.

- `r1` runs separate OSPF processes in `tenant-a` and `tenant-b`.
- `r2` advertises a `tenant-a` service prefix.
- `r3` advertises a `tenant-b` service prefix.
- The two OSPF domains use independent RIB/FIB tables, so service routes do not leak between VRFs.
