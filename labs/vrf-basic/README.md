# VRF Basic

FRR/Linux-only lab for static and connected routing across two isolated VRFs.

- `tenant-a` and `tenant-b` use overlapping link and service prefixes.
- `tenant-b` owns an additional service prefix that must not be reachable from `tenant-a`.
- No BGP, OSPF, route leaking, cEOS, or SR Linux is used.
