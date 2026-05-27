# VRF BGP Basic

FRR/Linux-only lab for BGP routing across two isolated VRFs.

- `r1` peers with `r2` in `tenant-a` and `r3` in `tenant-b`.
- `tenant-a` and `tenant-b` use overlapping link and service prefixes.
- `tenant-b` owns an additional BGP service prefix that must not be reachable from `tenant-a`.
- No route leaking, OSPF, cEOS, or SR Linux is used.
